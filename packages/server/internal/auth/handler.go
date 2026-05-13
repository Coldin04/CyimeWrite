package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"g.co1d.in/Coldin04/Cyime/server/internal/securevalue"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

var tokenService *TokenService
var tokenServiceMu sync.Mutex

func getTokenService() (*TokenService, error) {
	tokenServiceMu.Lock()
	defer tokenServiceMu.Unlock()

	if tokenService != nil {
		return tokenService, nil
	}
	svc, err := NewTokenService()
	if err != nil {
		return nil, err
	}
	tokenService = svc
	return tokenService, nil
}

// Shared struct to store user info from any provider
type UserProfile struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	Picture       string
}

// ProviderInfo represents the data sent to the frontend for a login provider.
type ProviderInfo struct {
	Name        string  `json:"name"`
	DisplayName *string `json:"displayName,omitempty"`
	Icon        string  `json:"icon"`
	SSOUrl      string  `json:"ssoUrl"`
}

type SessionListResponse struct {
	Items []SessionResponseDTO `json:"items"`
}

type SessionResponseDTO struct {
	ID          string    `json:"id"`
	DeviceLabel string    `json:"deviceLabel"`
	UserAgent   string    `json:"userAgent"`
	LastSeenAt  time.Time `json:"lastSeenAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
	CreatedAt   time.Time `json:"createdAt"`
	Current     bool      `json:"current"`
}

func getAPIBaseURL() string {
	baseURL := strings.TrimSpace(os.Getenv("API_BASE_URL"))
	if baseURL == "" {
		port := strings.TrimSpace(os.Getenv("PORT"))
		if port == "" {
			port = "8080"
		}
		port = strings.TrimPrefix(port, ":")
		baseURL = fmt.Sprintf("http://localhost:%s", port)
	}

	return strings.TrimRight(baseURL, "/")
}

// GetAuthConfig is the handler for GET /api/v1/auth/config
func GetAuthConfig(c *fiber.Ctx) error {
	var dbProviders []models.AuthProvider
	if err := database.DB.Where("is_active = ?", true).Find(&dbProviders).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "数据库查询失败"})
	}

	responseProviders := make([]ProviderInfo, 0, len(dbProviders))
	for _, p := range dbProviders {
		var iconURL string
		if p.IconURL != nil {
			iconURL = *p.IconURL
		}
		responseProviders = append(responseProviders, ProviderInfo{
			Name:        p.Name,
			DisplayName: p.DisplayName,
			Icon:        iconURL,
			SSOUrl:      getAPIBaseURL() + "/api/v1/auth/login/" + p.Name,
		})
	}

	return c.JSON(fiber.Map{
		"providers": responseProviders,
	})
}

// AuthLogin initiates the OIDC/OAuth2 login flow.
func AuthLogin(c *fiber.Ctx) error {
	providerName := c.Params("provider")
	ctx := c.Context()

	var dbProvider models.AuthProvider
	if err := database.DB.Where("name = ? AND is_active = ?", providerName, true).First(&dbProvider).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "提供的认证商不存在或未激活"})
	}

	endpoint, err := getEndpointFromProvider(ctx, &dbProvider)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	clientSecret, err := decryptClientSecret(dbProvider.ClientSecretEncrypted)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to decrypt auth provider secret"})
	}

	oauth2Config := oauth2.Config{
		ClientID:     dbProvider.ClientID,
		ClientSecret: clientSecret,
		RedirectURL:  fmt.Sprintf("%s/api/v1/auth/callback/%s", getAPIBaseURL(), providerName),
		Endpoint:     endpoint,
		Scopes:       strings.Split(dbProvider.Scopes, " "),
	}

	state, err := generateState(c)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to initialize oauth state"})
	}
	return c.Redirect(oauth2Config.AuthCodeURL(state), fiber.StatusTemporaryRedirect)
}

// AuthCallback handles the callback from the OIDC/OAuth2 provider.
func AuthCallback(c *fiber.Ctx) error {
	svc, err := getTokenService()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	providerName := c.Params("provider")
	ctx := c.Context()

	if err := verifyState(c); err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	var dbProvider models.AuthProvider
	if err := database.DB.Where("name = ? AND is_active = ?", providerName, true).First(&dbProvider).Error; err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "提供的认证商不存在或未激活"})
	}

	endpoint, err := getEndpointFromProvider(ctx, &dbProvider)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	clientSecret, err := decryptClientSecret(dbProvider.ClientSecretEncrypted)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to decrypt auth provider secret"})
	}

	oauth2Config := oauth2.Config{
		ClientID:     dbProvider.ClientID,
		ClientSecret: clientSecret,
		RedirectURL:  fmt.Sprintf("%s/api/v1/auth/callback/%s", getAPIBaseURL(), providerName),
		Endpoint:     endpoint,
		Scopes:       strings.Split(dbProvider.Scopes, " "),
	}

	code := c.Query("code")
	oauth2Token, err := oauth2Config.Exchange(ctx, code)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "无法交换授权码: " + err.Error()})
	}

	userProfile, err := getUserProfile(ctx, &dbProvider, &oauth2Config, oauth2Token)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// --- Transactional User & Token Handling ---
	var accessToken, refreshToken string
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		// Step 1: Find or create the user.
		user, txErr := findOrCreateUser(tx, providerName, userProfile)
		if txErr != nil {
			return txErr
		}

		// Step 2: Generate and persist tokens for the user.
		accessToken, refreshToken, txErr = svc.GenerateAndPersistTokens(tx, user, c.Get("User-Agent"))
		if txErr != nil {
			return txErr
		}

		return nil
	})

	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// Step 3: Deliver tokens to the client and redirect.
	return svc.DeliverTokensAndRedirect(c, accessToken, refreshToken)
}

// HandleRefresh handles the token refresh endpoint by delegating to the token service.
func HandleRefresh(c *fiber.Ctx) error {
	svc, err := getTokenService()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return svc.HandleRefresh(c)
}

// HandleLogout handles the user logout process.
func HandleLogout(c *fiber.Ctx) error {
	// Get the refresh token from the secure cookie.
	rawRefreshToken := c.Cookies("cyime_refresh_token")

	// If the cookie is not present, there's nothing to do.
	// The user is already effectively logged out from the server's perspective.
	if rawRefreshToken != "" {
		svc, err := getTokenService()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		// We don't need to block on the result.
		// Fire-and-forget the revocation. The most important part is clearing the client-side cookie.
		_ = svc.RevokeRefreshToken(rawRefreshToken)
	}

	// Instruct the browser to clear the refresh token cookie.
	// This is the most critical step for the client-side.
	// We only need to provide the path that the cookie was set with.
	c.ClearCookie("cyime_refresh_token", "/api/v1/auth")
	c.ClearCookie("cyime_media_access_token", "/api/v1/media")

	return c.SendStatus(fiber.StatusNoContent)
}

// HandleListSessions returns active sessions for the current user.
func HandleListSessions(c *fiber.Ctx) error {
	svc, err := getTokenService()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	userID, err := parseUserIDFromContext(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	items, err := svc.ListUserSessions(userID, c.Cookies("cyime_refresh_token"))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	responseItems := make([]SessionResponseDTO, 0, len(items))
	for _, item := range items {
		responseItems = append(responseItems, SessionResponseDTO{
			ID:          item.ID.String(),
			DeviceLabel: item.DeviceLabel,
			UserAgent:   item.UserAgent,
			LastSeenAt:  item.LastSeenAt,
			ExpiresAt:   item.ExpiresAt,
			CreatedAt:   item.CreatedAt,
			Current:     item.Current,
		})
	}

	return c.JSON(SessionListResponse{Items: responseItems})
}

// HandleRevokeSession revokes one session by id.
func HandleRevokeSession(c *fiber.Ctx) error {
	svc, err := getTokenService()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	userID, err := parseUserIDFromContext(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	sessionID, err := parseSessionID(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	isCurrent, err := svc.RevokeSession(userID, sessionID, c.Cookies("cyime_refresh_token"))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "会话不存在"})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	if isCurrent {
		c.ClearCookie("cyime_refresh_token", "/api/v1/auth")
		c.ClearCookie("cyime_media_access_token", "/api/v1/media")
	}

	return c.SendStatus(fiber.StatusNoContent)
}

// HandleRevokeOtherSessions revokes all sessions except the current one.
func HandleRevokeOtherSessions(c *fiber.Ctx) error {
	svc, err := getTokenService()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	userID, err := parseUserIDFromContext(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": err.Error()})
	}

	revokedCount, err := svc.RevokeOtherSessions(userID, c.Cookies("cyime_refresh_token"))
	if err != nil {
		if e, ok := err.(*fiber.Error); ok {
			return c.Status(e.Code).JSON(fiber.Map{"error": e.Message})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"revokedCount": revokedCount})
}

// --- Helper Functions ---

// findOrCreateUser finds an existing user based on provider info or creates a new one.
// It must be run within a transaction.
func findOrCreateUser(tx *gorm.DB, providerName string, userProfile *UserProfile) (*models.User, error) {
	var identity models.UserIdentityProvider

	// 1. Find identity by provider and provider's user ID
	err := tx.Preload("User").Where("provider_name = ? AND provider_user_id = ?", providerName, userProfile.Subject).First(&identity).Error

	if err == nil {
		// Identity found, return the associated user
		return &identity.User, nil
	}

	if err != gorm.ErrRecordNotFound {
		// A different database error occurred
		return nil, fmt.Errorf("查询身份提供商信息失败: %w", err)
	}

	normalizedEmail := normalizeEmail(userProfile.Email)
	if !userProfile.EmailVerified {
		normalizedEmail = nil
	}
	var existingByEmail models.User
	if normalizedEmail != nil {
		queryErr := tx.Where("email = ?", *normalizedEmail).First(&existingByEmail).Error
		switch {
		case queryErr == nil:
			if !userProfile.EmailVerified {
				return nil, fmt.Errorf("该邮箱已存在，请先在个人中心绑定登录方式")
			}
			if err := tx.Create(&models.UserIdentityProvider{
				UserID:         existingByEmail.ID,
				ProviderName:   providerName,
				ProviderUserID: userProfile.Subject,
			}).Error; err != nil {
				return nil, fmt.Errorf("关联身份提供商失败: %w", err)
			}
			return &existingByEmail, nil
		case errors.Is(queryErr, gorm.ErrRecordNotFound):
		default:
			return nil, fmt.Errorf("查询邮箱信息失败: %w", queryErr)
		}
	}

	// 2. Identity not found, so we create a new user and a new identity.
	newUser := models.User{
		Email:         normalizedEmail,
		EmailVerified: userProfile.EmailVerified,
		DisplayName:   optionalStringPtr(userProfile.Name),
		AvatarURL:     optionalStringPtr(userProfile.Picture),
	}
	if userProfile.EmailVerified && normalizedEmail != nil {
		now := time.Now()
		newUser.EmailVerifiedAt = &now
	}
	if err := tx.Create(&newUser).Error; err != nil {
		return nil, fmt.Errorf("创建新用户失败: %w", err)
	}

	newIdentity := models.UserIdentityProvider{
		UserID:         newUser.ID,
		ProviderName:   providerName,
		ProviderUserID: userProfile.Subject,
	}
	if err := tx.Create(&newIdentity).Error; err != nil {
		return nil, fmt.Errorf("关联新身份提供商失败: %w", err)
	}

	// We need to return the user that was just created.
	// To be safe and ensure all default values (like CreatedAt) are loaded, we can reload it.
	var createdUser models.User
	if err := tx.First(&createdUser, newUser.ID).Error; err != nil {
		return nil, fmt.Errorf("无法重新加载创建的用户: %w", err)
	}

	return &createdUser, nil
}

func parseUserIDFromContext(c *fiber.Ctx) (uuid.UUID, error) {
	userIDStr, ok := c.Locals("userId").(string)
	if !ok || strings.TrimSpace(userIDStr) == "" {
		return uuid.Nil, fmt.Errorf("missing user id in context")
	}
	return uuid.Parse(userIDStr)
}

func parseSessionID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, fmt.Errorf("无效的会话 id")
	}
	return id, nil
}

func getEndpointFromProvider(ctx context.Context, provider *models.AuthProvider) (oauth2.Endpoint, error) {
	switch provider.ProtocolType {
	case "oidc":
		if provider.IssuerURL == nil || *provider.IssuerURL == "" {
			return oauth2.Endpoint{}, fmt.Errorf("OIDC提供商 '%s' 缺少issuer_url", provider.Name)
		}
		oidcProvider, err := oidc.NewProvider(ctx, *provider.IssuerURL)
		if err != nil {
			return oauth2.Endpoint{}, fmt.Errorf("无法连接到OIDC提供商 '%s'", provider.Name)
		}
		return oidcProvider.Endpoint(), nil
	case "oauth2":
		if provider.AuthURL == nil || *provider.AuthURL == "" || provider.TokenURL == nil || *provider.TokenURL == "" {
			return oauth2.Endpoint{}, fmt.Errorf("OAuth2提供商 '%s' 缺少auth_url或token_url", provider.Name)
		}
		return oauth2.Endpoint{
			AuthURL:  *provider.AuthURL,
			TokenURL: *provider.TokenURL,
		}, nil
	default:
		return oauth2.Endpoint{}, fmt.Errorf("未知的协议类型: '%s'", provider.ProtocolType)
	}
}

// generateState creates a cryptographically random OAuth/OIDC state value,
// stores it in a cookie scoped to the auth endpoints, and returns the value
// for insertion into the authorization URL. The previous version ignored
// errors from crypto/rand.Read and omitted the Secure flag; both gaps are
// closed here.
func generateState(c *fiber.Ctx) (string, error) {
	stateBytes := make([]byte, 32)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", fmt.Errorf("generate oauth state: %w", err)
	}
	state := base64.URLEncoding.EncodeToString(stateBytes)
	c.Cookie(&fiber.Cookie{
		Name:     "oidc_state",
		Value:    state,
		Expires:  time.Now().Add(10 * time.Minute),
		HTTPOnly: true,
		Secure:   c.Protocol() == "https",
		SameSite: "Lax",
		// Scope to the auth endpoints so the cookie isn't sent on unrelated
		// paths. The login and callback URLs both live under /api/v1/auth.
		Path: "/api/v1/auth",
	})
	return state, nil
}

// verifyState checks the callback state against the cookie using a
// constant-time comparison and, on success, immediately invalidates the
// cookie so it cannot be replayed on a second callback.
func verifyState(c *fiber.Ctx) error {
	stateFromCookie := c.Cookies("oidc_state")
	stateFromQuery := c.Query("state")
	if stateFromCookie == "" || stateFromQuery == "" {
		return errors.New("无效的 state 参数")
	}
	if subtle.ConstantTimeCompare([]byte(stateFromCookie), []byte(stateFromQuery)) != 1 {
		return errors.New("无效的 state 参数")
	}
	// Expire the cookie now that it has been consumed.
	c.Cookie(&fiber.Cookie{
		Name:     "oidc_state",
		Value:    "",
		Expires:  time.Now().Add(-time.Hour),
		HTTPOnly: true,
		Secure:   c.Protocol() == "https",
		SameSite: "Lax",
		Path:     "/api/v1/auth",
	})
	return nil
}

func getUserProfile(ctx context.Context, provider *models.AuthProvider, oauth2Config *oauth2.Config, token *oauth2.Token) (*UserProfile, error) {
	var userProfile UserProfile

	switch provider.ProtocolType {
	case "oidc":
		rawIDToken, ok := token.Extra("id_token").(string)
		if !ok {
			return nil, fmt.Errorf("无法从令牌中获取 id_token")
		}
		oidcProvider, err := oidc.NewProvider(ctx, *provider.IssuerURL)
		if err != nil {
			return nil, fmt.Errorf("无法连接到 OIDC 提供商")
		}
		idToken, err := oidcProvider.Verifier(&oidc.Config{ClientID: provider.ClientID}).Verify(ctx, rawIDToken)
		if err != nil {
			return nil, fmt.Errorf("无效的 id_token")
		}
		var claims struct {
			Email         string `json:"email"`
			EmailVerified bool   `json:"email_verified"`
			Name          string `json:"name"`
			Picture       string `json:"picture"`
			Subject       string `json:"sub"`
		}
		if err := idToken.Claims(&claims); err != nil {
			return nil, fmt.Errorf("无法解析 id_token 的 claims")
		}
		userProfile = UserProfile{
			Subject:       claims.Subject,
			Email:         claims.Email,
			EmailVerified: claims.EmailVerified,
			Name:          claims.Name,
			Picture:       claims.Picture,
		}

	case "oauth2":
		if provider.UserInfoURL == nil || *provider.UserInfoURL == "" {
			return nil, fmt.Errorf("OAuth2提供商缺少user_info_url")
		}
		client := oauth2Config.Client(ctx, token)
		resp, err := client.Get(*provider.UserInfoURL)
		if err != nil {
			return nil, fmt.Errorf("无法获取用户信息")
		}
		defer resp.Body.Close()

		// NOTE: This part is still provider-specific because each provider has a different user info response structure.
		// A more advanced implementation might use a plugin system or field mapping in the DB.
		// For now, a switch on the name is a reasonable compromise.
		if provider.Name == "github" {
			var ghUser struct {
				ID     int64  `json:"id"`
				Login  string `json:"login"`
				Name   string `json:"name"`
				Email  string `json:"email"`
				Avatar string `json:"avatar_url"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&ghUser); err != nil {
				return nil, fmt.Errorf("无法解析GitHub用户信息")
			}
			// Use login as name if name is empty
			userName := ghUser.Name
			if userName == "" {
				userName = ghUser.Login
			}
			userEmail, emailVerified, err := fetchGitHubPrimaryEmail(ctx, oauth2Config, token)
			if err != nil {
				return nil, err
			}
			userProfile = UserProfile{
				Subject:       fmt.Sprintf("%d", ghUser.ID),
				Email:         userEmail,
				EmailVerified: emailVerified,
				Name:          userName,
				Picture:       ghUser.Avatar,
			}
		} else if provider.Name == "google" {
			var googleUser struct {
				ID            string `json:"id"`
				Email         string `json:"email"`
				VerifiedEmail bool   `json:"verified_email"`
				Name          string `json:"name"`
				Picture       string `json:"picture"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&googleUser); err != nil {
				return nil, fmt.Errorf("无法解析Google用户信息")
			}

			userProfile = UserProfile{
				Subject:       strings.TrimSpace(googleUser.ID),
				Email:         strings.TrimSpace(googleUser.Email),
				EmailVerified: googleUser.VerifiedEmail,
				Name:          strings.TrimSpace(googleUser.Name),
				Picture:       strings.TrimSpace(googleUser.Picture),
			}
		} else {
			return nil, fmt.Errorf("未实现对 '%s' 的用户信息解析", provider.Name)
		}
	}

	if userProfile.Subject == "" {
		return nil, fmt.Errorf("未能获取到任何用户信息")
	}
	return &userProfile, nil
}

func fetchGitHubPrimaryEmail(ctx context.Context, oauth2Config *oauth2.Config, token *oauth2.Token) (string, bool, error) {
	client := oauth2Config.Client(ctx, token)
	resp, err := client.Get("https://api.github.com/user/emails")
	if err != nil {
		return "", false, fmt.Errorf("无法获取 GitHub 邮箱信息: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", false, fmt.Errorf("无法获取 GitHub 邮箱信息: status %d, body: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&emails); err != nil {
		return "", false, fmt.Errorf("无法解析 GitHub 邮箱信息: %w", err)
	}

	for _, item := range emails {
		email := strings.TrimSpace(item.Email)
		if item.Primary && item.Verified && email != "" {
			return email, true, nil
		}
	}
	for _, item := range emails {
		email := strings.TrimSpace(item.Email)
		if item.Verified && email != "" {
			return email, true, nil
		}
	}

	return "", false, nil
}

func optionalStringPtr(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeEmail(value string) *string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// decryptClientSecret decrypts an OAuth provider's encrypted client secret.
func decryptClientSecret(encrypted string) (string, error) {
	return securevalue.DecryptString(encrypted)
}
