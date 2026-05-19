package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// JWTClaims represents the claims for our access token
type JWTClaims struct {
	jwt.RegisteredClaims
	UserID uuid.UUID `json:"userId"`
}

type TokenService struct {
	jwtSecret              []byte
	accessTokenLifetime    time.Duration
	refreshTokenLifetime   time.Duration
	refreshTokenByteLength int
}

const mediaAccessTokenCookieName = "cyime_media_access_token"

// SessionInfo is the API-facing session view used by the security page.
type SessionInfo struct {
	ID          uuid.UUID `json:"id"`
	DeviceLabel string    `json:"deviceLabel"`
	UserAgent   string    `json:"userAgent"`
	LastSeenAt  time.Time `json:"lastSeenAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
	CreatedAt   time.Time `json:"createdAt"`
	Current     bool      `json:"current"`
}

// NewTokenService creates a new token service, reading configuration from environment variables.
// JWT_SECRET_KEY is required and validated by LoadJWTSecret; there is no fallback to a hardcoded
// default. Callers should treat any error returned here as a fatal configuration error.
func NewTokenService() (*TokenService, error) {
	secret, err := LoadJWTSecret()
	if err != nil {
		return nil, err
	}

	accessTokenLifetime := 15 * time.Minute
	accessTokenLifetimeMinutes, err := strconv.Atoi(os.Getenv("ACCESS_TOKEN_LIFETIME_MINUTES"))
	if err == nil && accessTokenLifetimeMinutes > 0 {
		accessTokenLifetime = time.Duration(accessTokenLifetimeMinutes) * time.Minute
	}
	accessTokenLifetimeSeconds, err := strconv.Atoi(os.Getenv("ACCESS_TOKEN_LIFETIME_SECONDS"))
	if err == nil && accessTokenLifetimeSeconds > 0 {
		accessTokenLifetime = time.Duration(accessTokenLifetimeSeconds) * time.Second
	}

	refreshTokenLifetimeHours, err := strconv.Atoi(os.Getenv("REFRESH_TOKEN_LIFETIME_HOURS"))
	if err != nil || refreshTokenLifetimeHours <= 0 {
		refreshTokenLifetimeHours = 720 // Default to 30 days (30 * 24)
	}

	return &TokenService{
		jwtSecret:              secret,
		accessTokenLifetime:    accessTokenLifetime,
		refreshTokenLifetime:   time.Duration(refreshTokenLifetimeHours) * time.Hour,
		refreshTokenByteLength: 32,
	}, nil
}

// GenerateAndPersistTokens creates a new logical session plus access/refresh tokens.
// This must be run within a GORM transaction.
func (s *TokenService) GenerateAndPersistTokens(tx *gorm.DB, user *models.User, userAgent string) (accessTokenString, rawRefreshTokenString string, err error) {
	// 1. Generate Access Token
	accessTokenString, err = s.generateAccessToken(user.ID)
	if err != nil {
		return "", "", fmt.Errorf("failed to generate access token: %w", err)
	}

	// 2. Generate Refresh Token
	rawRefreshTokenString, err = s.generateRefreshToken()
	if err != nil {
		return "", "", fmt.Errorf("failed to generate refresh token: %w", err)
	}

	// 3. Create a logical session row for later session management UI.
	now := time.Now()
	session := models.UserSession{
		UserID:      user.ID,
		UserAgent:   userAgent,
		DeviceLabel: buildDeviceLabel(userAgent),
		LastSeenAt:  now,
	}
	if err := tx.Create(&session).Error; err != nil {
		return "", "", fmt.Errorf("failed to persist session: %w", err)
	}

	// 4. Hash and Persist Refresh Token
	refreshTokenHash := sha256.Sum256([]byte(rawRefreshTokenString))
	refreshTokenExpiresAt := now.Add(s.refreshTokenLifetime)

	refreshToken := models.UserRefreshToken{
		UserID:    user.ID,
		SessionID: session.ID,
		TokenHash: hex.EncodeToString(refreshTokenHash[:]),
		ExpiresAt: refreshTokenExpiresAt,
	}

	if err := tx.Create(&refreshToken).Error; err != nil {
		return "", "", fmt.Errorf("failed to persist refresh token: %w", err)
	}

	return accessTokenString, rawRefreshTokenString, nil
}

// DeliverTokensAndRedirect sets the refresh token in a secure cookie and redirects the user.
func (s *TokenService) DeliverTokensAndRedirect(c *fiber.Ctx, accessToken, refreshToken string) error {
	// Set the refresh token cookie
	c.Cookie(&fiber.Cookie{
		Name:     "cyime_refresh_token",
		Value:    refreshToken,
		Expires:  time.Now().Add(s.refreshTokenLifetime),
		HTTPOnly: true,
		Secure:   c.Protocol() == "https", // Only set secure flag if on HTTPS
		SameSite: "Lax",
		Path:     "/api/v1/auth", // Important: Scope cookie to the auth path to prevent it being sent on every request
	})

	// Media content endpoints can be loaded by <img>, so we also provide a scoped access-token cookie.
	c.Cookie(&fiber.Cookie{
		Name:     mediaAccessTokenCookieName,
		Value:    accessToken,
		Expires:  time.Now().Add(s.accessTokenLifetime),
		HTTPOnly: true,
		Secure:   c.Protocol() == "https",
		SameSite: "Lax",
		Path:     "/api/v1/media",
	})

	// 通过将过期时间设置为过去来清除 oidc_state cookie。
	c.Cookie(&fiber.Cookie{
		Name:     "oidc_state",
		Value:    "",
		Expires:  time.Now().Add(-time.Hour), // 设置为一小时前，使其立即过期
		HTTPOnly: true,
		Secure:   c.Protocol() == "https",
		SameSite: "Lax",
		Path:     "/",
	})

	frontendCallbackURL := os.Getenv("FRONTEND_CALLBACK_URL")
	if frontendCallbackURL == "" {
		frontendCallbackURL = "http://localhost:5173/auth/callback" // Default for local dev
	}

	redirectURL := fmt.Sprintf("%s#token=%s", frontendCallbackURL, accessToken)
	return c.Redirect(redirectURL, fiber.StatusTemporaryRedirect)
}

func (s *TokenService) generateAccessToken(userID uuid.UUID) (string, error) {
	claims := JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(s.accessTokenLifetime)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "Cyime",
			Subject:   userID.String(),
		},
		UserID: userID,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

func (s *TokenService) generateRefreshToken() (string, error) {
	b := make([]byte, s.refreshTokenByteLength)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func hashRefreshToken(rawRefreshToken string) string {
	tokenHash := sha256.Sum256([]byte(rawRefreshToken))
	return hex.EncodeToString(tokenHash[:])
}

// buildDeviceLabel keeps设备展示简单可读，不依赖重型 UA 解析库。
func buildDeviceLabel(userAgent string) string {
	if userAgent == "" {
		return "未知设备"
	}

	lower := strings.ToLower(userAgent)
	browser := "未知浏览器"
	platform := "未知系统"

	switch {
	case strings.Contains(lower, "firefox"):
		browser = "Firefox"
	case strings.Contains(lower, "edg"):
		browser = "Edge"
	case strings.Contains(lower, "chrome") && !strings.Contains(lower, "edg"):
		browser = "Chrome"
	case strings.Contains(lower, "safari") && !strings.Contains(lower, "chrome"):
		browser = "Safari"
	}

	switch {
	case strings.Contains(lower, "iphone"):
		platform = "iPhone"
	case strings.Contains(lower, "ipad"):
		platform = "iPad"
	case strings.Contains(lower, "android"):
		platform = "Android"
	case strings.Contains(lower, "mac os x"), strings.Contains(lower, "macintosh"):
		platform = "macOS"
	case strings.Contains(lower, "windows"):
		platform = "Windows"
	case strings.Contains(lower, "linux"):
		platform = "Linux"
	}

	return fmt.Sprintf("%s · %s", browser, platform)
}

// RevokeRefreshToken deletes a refresh token from the database.
func (s *TokenService) RevokeRefreshToken(rawRefreshToken string) error {
	// Delete the token from the database.
	// We use Unscoped() to ensure a hard delete, not a soft delete.
	result := database.DB.Unscoped().Where("token_hash = ?", hashRefreshToken(rawRefreshToken)).Delete(&models.UserRefreshToken{})

	if result.Error != nil {
		return result.Error
	}

	return nil
}

func (s *TokenService) currentSessionIDFromRefreshToken(rawRefreshToken string) (*uuid.UUID, error) {
	if rawRefreshToken == "" {
		return nil, nil
	}

	var token models.UserRefreshToken
	if err := database.DB.
		Select("session_id").
		Where("token_hash = ? AND expires_at > ?", hashRefreshToken(rawRefreshToken), time.Now()).
		First(&token).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &token.SessionID, nil
}

// ListUserSessions returns active sessions for one user.
func (s *TokenService) ListUserSessions(userID uuid.UUID, currentRawRefreshToken string) ([]SessionInfo, error) {
	currentSessionID, err := s.currentSessionIDFromRefreshToken(currentRawRefreshToken)
	if err != nil {
		return nil, err
	}

	var sessions []models.UserSession
	if err := database.DB.
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Order("last_seen_at DESC").
		Find(&sessions).Error; err != nil {
		return nil, err
	}

	var refreshTokens []models.UserRefreshToken
	if err := database.DB.
		Where("user_id = ? AND expires_at > ?", userID, time.Now()).
		Order("created_at DESC").
		Find(&refreshTokens).Error; err != nil {
		return nil, err
	}

	expiresAtBySessionID := make(map[uuid.UUID]time.Time, len(refreshTokens))
	for _, token := range refreshTokens {
		if _, exists := expiresAtBySessionID[token.SessionID]; !exists {
			expiresAtBySessionID[token.SessionID] = token.ExpiresAt
		}
	}

	items := make([]SessionInfo, 0, len(sessions))
	for _, session := range sessions {
		expiresAt, ok := expiresAtBySessionID[session.ID]
		if !ok {
			continue
		}
		isCurrent := currentSessionID != nil && *currentSessionID == session.ID
		items = append(items, SessionInfo{
			ID:          session.ID,
			DeviceLabel: session.DeviceLabel,
			UserAgent:   session.UserAgent,
			LastSeenAt:  session.LastSeenAt,
			ExpiresAt:   expiresAt,
			CreatedAt:   session.CreatedAt,
			Current:     isCurrent,
		})
	}

	return items, nil
}

// RevokeSession revokes one logical session and removes all its refresh tokens.
func (s *TokenService) RevokeSession(userID uuid.UUID, sessionID uuid.UUID, currentRawRefreshToken string) (bool, error) {
	currentSessionID, err := s.currentSessionIDFromRefreshToken(currentRawRefreshToken)
	if err != nil {
		return false, err
	}

	now := time.Now()
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.UserSession{}).
			Where("id = ? AND user_id = ? AND revoked_at IS NULL", sessionID, userID).
			Updates(map[string]any{"revoked_at": &now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		if err := tx.Unscoped().
			Where("user_id = ? AND session_id = ?", userID, sessionID).
			Delete(&models.UserRefreshToken{}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return false, err
	}

	return currentSessionID != nil && *currentSessionID == sessionID, nil
}

// RevokeOtherSessions revokes all sessions except the current one.
func (s *TokenService) RevokeOtherSessions(userID uuid.UUID, currentRawRefreshToken string) (int64, error) {
	currentSessionID, err := s.currentSessionIDFromRefreshToken(currentRawRefreshToken)
	if err != nil {
		return 0, err
	}
	if currentSessionID == nil {
		return 0, fiber.NewError(fiber.StatusUnauthorized, "Current session not found")
	}

	now := time.Now()
	var affected int64
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.UserSession{}).
			Where("user_id = ? AND id <> ? AND revoked_at IS NULL", userID, *currentSessionID).
			Updates(map[string]any{"revoked_at": &now})
		if result.Error != nil {
			return result.Error
		}
		affected = result.RowsAffected

		if err := tx.Unscoped().
			Where("user_id = ? AND session_id <> ?", userID, *currentSessionID).
			Delete(&models.UserRefreshToken{}).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return 0, err
	}

	return affected, nil
}

// HandleRefresh processes a refresh token request, implementing token rotation for security.
func (s *TokenService) HandleRefresh(c *fiber.Ctx) error {
	// 1. Get the refresh token from the secure cookie.
	rawRefreshToken := c.Cookies("cyime_refresh_token")
	if rawRefreshToken == "" {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Refresh token not found"})
	}

	// 2. Hash the incoming token to look it up in the database.
	incomingTokenHashStr := hashRefreshToken(rawRefreshToken)

	var foundToken models.UserRefreshToken
	var newAccessToken string
	var newRefreshToken string

	// 3. Start a transaction for the rotation logic.
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		// Find the token and ensure it's not expired.
		if err := tx.Preload("User").Where("token_hash = ? AND expires_at > ?", incomingTokenHashStr, time.Now()).First(&foundToken).Error; err != nil {
			// If not found (or another DB error), the token is invalid.
			return fiber.NewError(fiber.StatusUnauthorized, "Invalid or expired refresh token")
		}

		// --- TOKEN ROTATION ---
		// Immediately delete the used token.
		if err := tx.Delete(&foundToken).Error; err != nil {
			return fmt.Errorf("failed to delete used refresh token: %w", err)
		}

		// Generate a new access token.
		var err error
		newAccessToken, err = s.generateAccessToken(foundToken.UserID)
		if err != nil {
			return fmt.Errorf("failed to generate new access token: %w", err)
		}

		// Generate and persist a new refresh token.
		newRefreshToken, err = s.generateRefreshToken()
		if err != nil {
			return fmt.Errorf("failed to generate new refresh token: %w", err)
		}

		now := time.Now()
		newRefreshTokenHash := sha256.Sum256([]byte(newRefreshToken))
		newRefreshTokenExpiresAt := now.Add(s.refreshTokenLifetime)

		// 刷新时沿用同一个逻辑会话，只更新最近活跃时间。
		if err := tx.Model(&models.UserSession{}).
			Where("id = ? AND user_id = ? AND revoked_at IS NULL", foundToken.SessionID, foundToken.UserID).
			Updates(map[string]any{
				"last_seen_at": now,
				"user_agent":   c.Get("User-Agent"),
				"device_label": buildDeviceLabel(c.Get("User-Agent")),
			}).Error; err != nil {
			return fmt.Errorf("failed to update session heartbeat: %w", err)
		}

		replacementToken := models.UserRefreshToken{
			UserID:    foundToken.UserID,
			SessionID: foundToken.SessionID,
			TokenHash: hex.EncodeToString(newRefreshTokenHash[:]),
			ExpiresAt: newRefreshTokenExpiresAt,
		}
		if err := tx.Create(&replacementToken).Error; err != nil {
			return fmt.Errorf("failed to persist new refresh token: %w", err)
		}

		return nil // Commit transaction
	})

	if err != nil {
		// If the transaction failed, it's either an internal error or the token was invalid.
		// In either case, clear the potentially invalid cookie on the client.
		c.ClearCookie("cyime_refresh_token")
		// Use the error from fiber.NewError if it exists
		if e, ok := err.(*fiber.Error); ok {
			return c.Status(e.Code).JSON(fiber.Map{"error": e.Message})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	// 5. Send the new refresh token in a new secure cookie.
	c.Cookie(&fiber.Cookie{
		Name:     "cyime_refresh_token",
		Value:    newRefreshToken,
		Expires:  time.Now().Add(s.refreshTokenLifetime),
		HTTPOnly: true,
		Secure:   c.Protocol() == "https",
		SameSite: "Lax",
		Path:     "/api/v1/auth",
	})

	c.Cookie(&fiber.Cookie{
		Name:     mediaAccessTokenCookieName,
		Value:    newAccessToken,
		Expires:  time.Now().Add(s.accessTokenLifetime),
		HTTPOnly: true,
		Secure:   c.Protocol() == "https",
		SameSite: "Lax",
		Path:     "/api/v1/media",
	})

	// 5. Send the new access token in the response body.
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"accessToken": newAccessToken,
	})
}
