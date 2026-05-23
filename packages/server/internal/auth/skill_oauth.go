package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/apitoken"
	"g.co1d.in/Coldin04/Cyime/server/internal/config"
	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	skillOAuthCodeLifetime  = 10 * time.Minute
	skillOAuthTokenLifetime = 90 * 24 * time.Hour
)

var defaultSkillOAuthScopes = []string{
	apitoken.ScopeWorkspaceRead,
	apitoken.ScopeWorkspaceWrite,
	apitoken.ScopeDocumentRead,
	apitoken.ScopeDocumentWrite,
	apitoken.ScopeFileMove,
	apitoken.ScopeFileCopy,
}

type skillOAuthTokenRequest struct {
	GrantType    string `json:"grant_type" form:"grant_type"`
	Code         string `json:"code" form:"code"`
	RedirectURI  string `json:"redirect_uri" form:"redirect_uri"`
	CodeVerifier string `json:"code_verifier" form:"code_verifier"`
	ClientID     string `json:"client_id" form:"client_id"`
}

// SkillOAuthAuthorize starts the browser authorization-code flow used by skill
// clients to obtain a scoped Cyime API token without manual copy from settings.
func SkillOAuthAuthorize(c *fiber.Ctx) error {
	redirectURI := strings.TrimSpace(c.Query("redirect_uri"))
	if err := validateSkillOAuthRedirectURI(redirectURI); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}

	responseType := strings.TrimSpace(c.Query("response_type"))
	if responseType != "" && responseType != "code" {
		return redirectOAuthError(c, redirectURI, c.Query("state"), "unsupported_response_type", "response_type must be code")
	}

	scopes, err := normalizeSkillOAuthScopes(c.Query("scope"))
	if err != nil {
		return redirectOAuthError(c, redirectURI, c.Query("state"), "invalid_scope", err.Error())
	}

	codeChallenge := strings.TrimSpace(c.Query("code_challenge"))
	codeChallengeMethod := strings.TrimSpace(c.Query("code_challenge_method"))
	if codeChallenge != "" {
		if codeChallengeMethod == "" {
			codeChallengeMethod = "plain"
		}
		if !isSupportedCodeChallengeMethod(codeChallengeMethod) {
			return redirectOAuthError(c, redirectURI, c.Query("state"), "invalid_request", "unsupported code_challenge_method")
		}
	}

	userID, authenticated, err := currentBrowserUserID(c)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	if !authenticated {
		return redirectToSkillOAuthLogin(c)
	}

	code, err := generateSkillOAuthCode()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to generate authorization code"})
	}
	scopesJSON, err := apitoken.EncodeScopes(scopes)
	if err != nil {
		return redirectOAuthError(c, redirectURI, c.Query("state"), "invalid_scope", err.Error())
	}

	row := models.SkillOAuthCode{
		UserID:              userID,
		ClientID:            strings.TrimSpace(c.Query("client_id")),
		RedirectURI:         redirectURI,
		CodeHash:            hashSkillOAuthCode(code),
		CodeChallenge:       codeChallenge,
		CodeChallengeMethod: codeChallengeMethod,
		Scopes:              scopesJSON,
		ExpiresAt:           time.Now().Add(skillOAuthCodeLifetime),
	}
	if err := database.DB.Create(&row).Error; err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "failed to persist authorization code"})
	}

	return redirectOAuthCode(c, redirectURI, code, c.Query("state"), scopes)
}

// SkillOAuthToken exchanges a short-lived authorization code for a scoped Cyime
// API token. The returned access_token is used as Authorization: Bearer <token>.
func SkillOAuthToken(c *fiber.Ctx) error {
	req, err := parseSkillOAuthTokenRequest(c)
	if err != nil {
		return oauthTokenError(c, fiber.StatusBadRequest, "invalid_request", "invalid token request")
	}

	if req.GrantType != "" && req.GrantType != "authorization_code" {
		return oauthTokenError(c, fiber.StatusBadRequest, "unsupported_grant_type", "grant_type must be authorization_code")
	}
	if strings.TrimSpace(req.Code) == "" || strings.TrimSpace(req.RedirectURI) == "" {
		return oauthTokenError(c, fiber.StatusBadRequest, "invalid_request", "code and redirect_uri are required")
	}

	var codeRow models.SkillOAuthCode
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.
			Where("code_hash = ? AND used_at IS NULL AND expires_at > ?", hashSkillOAuthCode(req.Code), time.Now()).
			First(&codeRow).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return fiber.NewError(fiber.StatusBadRequest, "invalid_grant")
			}
			return err
		}

		if codeRow.RedirectURI != strings.TrimSpace(req.RedirectURI) {
			return fiber.NewError(fiber.StatusBadRequest, "invalid_grant")
		}
		if strings.TrimSpace(codeRow.ClientID) != "" && strings.TrimSpace(req.ClientID) != "" && strings.TrimSpace(codeRow.ClientID) != strings.TrimSpace(req.ClientID) {
			return fiber.NewError(fiber.StatusBadRequest, "invalid_grant")
		}
		if !verifyPKCE(codeRow.CodeChallenge, codeRow.CodeChallengeMethod, req.CodeVerifier) {
			return fiber.NewError(fiber.StatusBadRequest, "invalid_grant")
		}

		now := time.Now()
		result := tx.Model(&models.SkillOAuthCode{}).
			Where("id = ? AND used_at IS NULL", codeRow.ID).
			Update("used_at", &now)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fiber.NewError(fiber.StatusBadRequest, "invalid_grant")
		}
		return nil
	})
	if err != nil {
		if e, ok := err.(*fiber.Error); ok && e.Message == "invalid_grant" {
			return oauthTokenError(c, fiber.StatusBadRequest, "invalid_grant", "authorization code is invalid, expired, or already used")
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	scopes, err := apitoken.DecodeScopes(codeRow.Scopes)
	if err != nil {
		return oauthTokenError(c, fiber.StatusBadRequest, "invalid_scope", err.Error())
	}
	expiresAt := time.Now().Add(skillOAuthTokenLifetime)
	tokenName := "Cyime Workspace Skill OAuth"
	if clientID := strings.TrimSpace(codeRow.ClientID); clientID != "" {
		tokenName = fmt.Sprintf("%s (%s)", tokenName, clientID)
	}

	created, err := apitoken.CreateToken(codeRow.UserID, apitoken.CreateTokenInput{
		Name:      tokenName,
		Scopes:    scopes,
		ExpiresAt: &expiresAt,
	})
	if err != nil {
		return oauthTokenError(c, fiber.StatusBadRequest, "invalid_scope", err.Error())
	}

	return c.JSON(fiber.Map{
		"access_token": created.Token,
		"token_type":   "Bearer",
		"scope":        strings.Join(scopes, " "),
		"expires_in":   int(time.Until(expiresAt).Seconds()),
	})
}

func currentBrowserUserID(c *fiber.Ctx) (uuid.UUID, bool, error) {
	rawRefreshToken := c.Cookies("cyime_refresh_token")
	if strings.TrimSpace(rawRefreshToken) == "" {
		return uuid.Nil, false, nil
	}

	var row models.UserRefreshToken
	if err := database.DB.
		Select("user_refresh_tokens.user_id").
		Joins("JOIN user_sessions ON user_sessions.id = user_refresh_tokens.session_id").
		Where("user_refresh_tokens.token_hash = ? AND user_refresh_tokens.expires_at > ? AND user_sessions.revoked_at IS NULL", hashRefreshToken(rawRefreshToken), time.Now()).
		First(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return uuid.Nil, false, nil
		}
		return uuid.Nil, false, err
	}
	return row.UserID, true, nil
}

func redirectToSkillOAuthLogin(c *fiber.Ctx) error {
	returnTo := getAPIBaseURL() + c.OriginalURL()
	loginURL, err := url.Parse(config.GetPublicBaseURL() + "/login")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "invalid public base URL"})
	}
	query := loginURL.Query()
	query.Set("return_to", returnTo)
	loginURL.RawQuery = query.Encode()
	return c.Redirect(loginURL.String(), fiber.StatusTemporaryRedirect)
}

func parseSkillOAuthTokenRequest(c *fiber.Ctx) (skillOAuthTokenRequest, error) {
	var req skillOAuthTokenRequest
	if err := c.BodyParser(&req); err != nil {
		return req, err
	}
	if req.Code != "" || req.RedirectURI != "" || req.GrantType != "" {
		return req, nil
	}
	if len(c.Body()) == 0 {
		return req, nil
	}
	return req, json.Unmarshal(c.Body(), &req)
}

func normalizeSkillOAuthScopes(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return append([]string(nil), defaultSkillOAuthScopes...), nil
	}
	return apitoken.NormalizeScopes(strings.Fields(raw))
}

func generateSkillOAuthCode() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func hashSkillOAuthCode(code string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(code)))
	return hex.EncodeToString(sum[:])
}

func isSupportedCodeChallengeMethod(method string) bool {
	switch method {
	case "plain", "S256":
		return true
	default:
		return false
	}
}

func verifyPKCE(challenge, method, verifier string) bool {
	challenge = strings.TrimSpace(challenge)
	if challenge == "" {
		return true
	}
	verifier = strings.TrimSpace(verifier)
	if verifier == "" {
		return false
	}
	if method == "" || method == "plain" {
		return subtle.ConstantTimeCompare([]byte(challenge), []byte(verifier)) == 1
	}
	if method != "S256" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	computed := base64.RawURLEncoding.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(challenge), []byte(computed)) == 1
}

func validateSkillOAuthRedirectURI(raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return fmt.Errorf("redirect_uri is required")
	}
	if isConfiguredSkillOAuthRedirectURI(value) {
		return nil
	}

	parsed, err := url.Parse(value)
	if err != nil || !parsed.IsAbs() {
		return fmt.Errorf("redirect_uri must be an absolute URL")
	}

	switch parsed.Scheme {
	case "http", "https":
		host := parsed.Hostname()
		if host == "localhost" {
			return nil
		}
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			return nil
		}
		if parsed.Scheme == "https" {
			return fmt.Errorf("https redirect_uri must be allowlisted with CYIME_SKILL_OAUTH_REDIRECT_URIS")
		}
	default:
		if parsed.Scheme != "" {
			return nil
		}
	}

	return fmt.Errorf("redirect_uri is not allowed")
}

func isConfiguredSkillOAuthRedirectURI(value string) bool {
	allowed := os.Getenv("CYIME_SKILL_OAUTH_REDIRECT_URIS")
	for _, candidate := range strings.Split(allowed, ",") {
		if strings.TrimSpace(candidate) == value {
			return true
		}
	}
	return false
}

func redirectOAuthCode(c *fiber.Ctx, redirectURI, code, state string, scopes []string) error {
	target, err := url.Parse(redirectURI)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid redirect_uri"})
	}
	query := target.Query()
	query.Set("code", code)
	if strings.TrimSpace(state) != "" {
		query.Set("state", state)
	}
	query.Set("scope", strings.Join(scopes, " "))
	target.RawQuery = query.Encode()
	return c.Redirect(target.String(), fiber.StatusTemporaryRedirect)
}

func redirectOAuthError(c *fiber.Ctx, redirectURI, state, code, description string) error {
	target, err := url.Parse(redirectURI)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": code, "error_description": description})
	}
	query := target.Query()
	query.Set("error", code)
	query.Set("error_description", description)
	if strings.TrimSpace(state) != "" {
		query.Set("state", state)
	}
	target.RawQuery = query.Encode()
	return c.Redirect(target.String(), fiber.StatusTemporaryRedirect)
}

func oauthTokenError(c *fiber.Ctx, status int, code, description string) error {
	return c.Status(status).JSON(fiber.Map{
		"error":             code,
		"error_description": description,
	})
}
