package apitoken

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const tokenPrefix = "cyime_sk_"

var (
	ErrTokenNameRequired = errors.New("token name is required")
	ErrTokenNameTooLong  = errors.New("token name is too long")
	ErrInvalidExpiry     = errors.New("token expiry must be in the future")
	ErrTokenUnauthorized = errors.New("invalid or expired API token")
	ErrTokenRevoked      = errors.New("API token has been revoked")
)

type TokenInfo struct {
	ID          uuid.UUID  `json:"id"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"tokenPrefix"`
	Scopes      []string   `json:"scopes"`
	LastUsedAt  *time.Time `json:"lastUsedAt,omitempty"`
	LastUsedIP  string     `json:"lastUsedIp,omitempty"`
	ExpiresAt   *time.Time `json:"expiresAt,omitempty"`
	RevokedAt   *time.Time `json:"revokedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

type CreatedToken struct {
	TokenInfo
	Token string `json:"token"`
}

type CreateTokenInput struct {
	Name      string
	Scopes    []string
	ExpiresAt *time.Time
}

type UpdateTokenInput struct {
	Name   string
	Scopes []string
}

type AuthenticatedToken struct {
	UserID    uuid.UUID
	TokenID   uuid.UUID
	Scopes    []string
	ExpiresAt *time.Time
}

func ListTokens(userID uuid.UUID) ([]TokenInfo, error) {
	var rows []models.ApiToken
	if err := database.DB.
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	items := make([]TokenInfo, 0, len(rows))
	for _, row := range rows {
		item, err := tokenToInfo(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func CreateToken(userID uuid.UUID, input CreateTokenInput) (*CreatedToken, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrTokenNameRequired
	}
	if len([]rune(name)) > 120 {
		return nil, ErrTokenNameTooLong
	}
	if input.ExpiresAt != nil && !input.ExpiresAt.After(time.Now()) {
		return nil, ErrInvalidExpiry
	}

	scopesJSON, err := EncodeScopes(input.Scopes)
	if err != nil {
		return nil, err
	}

	rawToken, err := generateRawToken()
	if err != nil {
		return nil, err
	}

	row := models.ApiToken{
		UserID:      userID,
		Name:        name,
		TokenPrefix: displayPrefix(rawToken),
		TokenHash:   hashToken(rawToken),
		Scopes:      scopesJSON,
		ExpiresAt:   input.ExpiresAt,
	}
	if err := database.DB.Create(&row).Error; err != nil {
		return nil, err
	}

	info, err := tokenToInfo(row)
	if err != nil {
		return nil, err
	}
	return &CreatedToken{
		TokenInfo: info,
		Token:     rawToken,
	}, nil
}

func UpdateToken(userID uuid.UUID, tokenID uuid.UUID, input UpdateTokenInput) (*TokenInfo, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, ErrTokenNameRequired
	}
	if len([]rune(name)) > 120 {
		return nil, ErrTokenNameTooLong
	}

	scopesJSON, err := EncodeScopes(input.Scopes)
	if err != nil {
		return nil, err
	}

	updates := map[string]any{
		"name":   name,
		"scopes": scopesJSON,
	}

	result := database.DB.Model(&models.ApiToken{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL AND deleted_at IS NULL", tokenID, userID).
		Updates(updates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	var row models.ApiToken
	if err := database.DB.Where("id = ? AND user_id = ?", tokenID, userID).First(&row).Error; err != nil {
		return nil, err
	}
	info, err := tokenToInfo(row)
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func RevokeToken(userID uuid.UUID, tokenID uuid.UUID) error {
	now := time.Now()
	result := database.DB.Model(&models.ApiToken{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", tokenID, userID).
		Update("revoked_at", now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func DeleteRevokedToken(userID uuid.UUID, tokenID uuid.UUID) error {
	result := database.DB.
		Where("id = ? AND user_id = ? AND revoked_at IS NOT NULL AND deleted_at IS NULL", tokenID, userID).
		Delete(&models.ApiToken{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func Authenticate(rawToken string, ip string) (*AuthenticatedToken, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" || !strings.HasPrefix(rawToken, tokenPrefix) {
		return nil, ErrTokenUnauthorized
	}

	now := time.Now()
	var row models.ApiToken
	if err := database.DB.
		Where("token_hash = ? AND deleted_at IS NULL", hashToken(rawToken)).
		First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTokenUnauthorized
		}
		return nil, err
	}

	if row.RevokedAt != nil {
		return nil, ErrTokenRevoked
	}
	if row.ExpiresAt != nil && !row.ExpiresAt.After(now) {
		return nil, ErrTokenUnauthorized
	}

	scopes, err := DecodeScopes(row.Scopes)
	if err != nil {
		return nil, err
	}

	updates := map[string]any{
		"last_used_at": now,
		"last_used_ip": strings.TrimSpace(ip),
	}
	_ = database.DB.Model(&models.ApiToken{}).Where("id = ?", row.ID).Updates(updates).Error

	return &AuthenticatedToken{
		UserID:    row.UserID,
		TokenID:   row.ID,
		Scopes:    scopes,
		ExpiresAt: row.ExpiresAt,
	}, nil
}

func generateRawToken() (string, error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", err
	}
	return tokenPrefix + base64.RawURLEncoding.EncodeToString(randomBytes), nil
}

func hashToken(rawToken string) string {
	sum := sha256.Sum256([]byte(rawToken))
	return hex.EncodeToString(sum[:])
}

func displayPrefix(rawToken string) string {
	if len(rawToken) <= 18 {
		return rawToken
	}
	return rawToken[:18]
}

func tokenToInfo(row models.ApiToken) (TokenInfo, error) {
	scopes, err := DecodeScopes(row.Scopes)
	if err != nil {
		return TokenInfo{}, fmt.Errorf("invalid token scopes: %w", err)
	}
	return TokenInfo{
		ID:          row.ID,
		Name:        row.Name,
		TokenPrefix: row.TokenPrefix,
		Scopes:      scopes,
		LastUsedAt:  row.LastUsedAt,
		LastUsedIP:  row.LastUsedIP,
		ExpiresAt:   row.ExpiresAt,
		RevokedAt:   row.RevokedAt,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}, nil
}
