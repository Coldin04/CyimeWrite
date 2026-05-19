package admin

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/config"
	"g.co1d.in/Coldin04/Cyime/server/internal/content"
	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/media"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	userpkg "g.co1d.in/Coldin04/Cyime/server/internal/user"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	defaultUserListLimit = 20
	maxUserListLimit     = 50
	maxUserListOffset    = 10000
	maxUserSearchLength  = 100
	maxDocumentQuota     = 100000
)

type OverviewStats struct {
	UserCount  int64
	AdminCount int64
}

type ListUsersParams struct {
	Limit  int
	Offset int
	Query  string
}

type UserListItem struct {
	User                 models.User
	ActiveDocumentCount  int64
	TrashedDocumentCount int64
	EffectiveQuota       *int
}

type UserListResult struct {
	Items      []UserListItem
	HasMore    bool
	NextOffset int
	Total      int64
}

type UpdateDocumentQuotaInput struct {
	DocumentQuotaMode string
	DocumentQuota     *int
}

type UpdateUserEmailInput struct {
	Email string
}

type ListUserSessionsParams struct {
	Limit  int
	Offset int
}

type AdminSessionItem struct {
	ID          uuid.UUID
	DeviceLabel string
	UserAgent   string
	LastSeenAt  time.Time
	ExpiresAt   time.Time
	CreatedAt   time.Time
}

type AdminSessionListResult struct {
	Items      []AdminSessionItem
	HasMore    bool
	NextOffset int
	Total      int64
}

type ListUserMediaParams struct {
	Limit  int
	Offset int
	Query  string
	Kind   string
	Status string
}

type documentCountRow struct {
	OwnerUserID  uuid.UUID
	ActiveCount  int64
	TrashedCount int64
}

func GetOverviewStats() (*OverviewStats, error) {
	var userCount int64
	if err := database.DB.Model(&models.User{}).Count(&userCount).Error; err != nil {
		return nil, err
	}

	var adminCount int64
	if err := database.DB.Model(&models.User{}).
		Where("admin_role = ?", models.AdminRoleAdmin).
		Count(&adminCount).Error; err != nil {
		return nil, err
	}

	return &OverviewStats{
		UserCount:  userCount,
		AdminCount: adminCount,
	}, nil
}

func ListUsers(params ListUsersParams) (*UserListResult, error) {
	normalized := normalizeListUsersParams(params)

	baseQuery := database.DB.Model(&models.User{})
	if normalized.Query != "" {
		like := "%" + strings.ToLower(normalized.Query) + "%"
		baseQuery = baseQuery.Where(
			"LOWER(COALESCE(email, '')) LIKE ? OR LOWER(COALESCE(display_name, '')) LIKE ?",
			like,
			like,
		)
	}

	var total int64
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, err
	}

	var users []models.User
	if err := baseQuery.
		Order("created_at DESC").
		Order("id DESC").
		Limit(normalized.Limit + 1).
		Offset(normalized.Offset).
		Find(&users).Error; err != nil {
		return nil, err
	}

	hasMore := len(users) > normalized.Limit
	if hasMore {
		users = users[:normalized.Limit]
	}

	countsByUserID, err := loadDocumentCountsByUserID(userIDs(users))
	if err != nil {
		return nil, err
	}
	globalDocumentQuota, err := GlobalDocumentQuota()
	if err != nil {
		return nil, err
	}

	items := make([]UserListItem, 0, len(users))
	for _, currentUser := range users {
		counts := countsByUserID[currentUser.ID]
		items = append(items, UserListItem{
			User:                 currentUser,
			ActiveDocumentCount:  counts.ActiveCount,
			TrashedDocumentCount: counts.TrashedCount,
			EffectiveQuota:       resolveEffectiveDocumentQuota(currentUser, globalDocumentQuota),
		})
	}

	nextOffset := normalized.Offset + len(items)
	return &UserListResult{
		Items:      items,
		HasMore:    hasMore,
		NextOffset: nextOffset,
		Total:      total,
	}, nil
}

func UpdateUserDocumentQuota(targetUserID uuid.UUID, input UpdateDocumentQuotaInput) (*models.User, error) {
	normalized, err := normalizeUpdateDocumentQuotaInput(input)
	if err != nil {
		return nil, err
	}

	var currentUser models.User
	if err := database.DB.First(&currentUser, "id = ?", targetUserID).Error; err != nil {
		return nil, err
	}

	updates := map[string]any{
		"document_quota_mode": normalized.DocumentQuotaMode,
		"document_quota":      normalized.DocumentQuota,
		"updated_at":          time.Now(),
	}
	if err := database.DB.Model(&currentUser).Updates(updates).Error; err != nil {
		return nil, err
	}

	return userpkg.GetUserByID(targetUserID)
}

func UpdateUserEmail(targetUserID uuid.UUID, input UpdateUserEmailInput) (*models.User, error) {
	normalized, err := normalizeUpdateUserEmailInput(input)
	if err != nil {
		return nil, err
	}

	err = database.DB.Transaction(func(tx *gorm.DB) error {
		var currentUser models.User
		if err := tx.First(&currentUser, "id = ?", targetUserID).Error; err != nil {
			return err
		}

		var existing models.User
		err := tx.Select("id").
			Where("email = ? AND id <> ?", *normalized, targetUserID).
			First(&existing).Error
		switch {
		case err == nil:
			return ErrEmailAlreadyInUse
		case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
			return err
		}

		updates := map[string]any{
			"email":             normalized,
			"email_verified":    false,
			"email_verified_at": nil,
			"updated_at":        time.Now(),
		}
		if err := tx.Model(&currentUser).Updates(updates).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return userpkg.GetUserByID(targetUserID)
}

func VerifyUserEmail(targetUserID uuid.UUID) (*models.User, error) {
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		var currentUser models.User
		if err := tx.First(&currentUser, "id = ?", targetUserID).Error; err != nil {
			return err
		}
		if currentUser.Email == nil || strings.TrimSpace(*currentUser.Email) == "" {
			return ErrEmailNotSet
		}

		now := time.Now()
		updates := map[string]any{
			"email_verified":    true,
			"email_verified_at": &now,
			"updated_at":        now,
		}
		if err := tx.Model(&currentUser).Updates(updates).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return userpkg.GetUserByID(targetUserID)
}

func GetUser(targetUserID uuid.UUID) (*UserListItem, error) {
	var currentUser models.User
	if err := database.DB.First(&currentUser, "id = ?", targetUserID).Error; err != nil {
		return nil, err
	}

	countsByUserID, err := loadDocumentCountsByUserID([]uuid.UUID{targetUserID})
	if err != nil {
		return nil, err
	}

	globalDocumentQuota, err := GlobalDocumentQuota()
	if err != nil {
		return nil, err
	}

	counts := countsByUserID[targetUserID]
	return &UserListItem{
		User:                 currentUser,
		ActiveDocumentCount:  counts.ActiveCount,
		TrashedDocumentCount: counts.TrashedCount,
		EffectiveQuota:       resolveEffectiveDocumentQuota(currentUser, globalDocumentQuota),
	}, nil
}

func normalizeListUsersParams(params ListUsersParams) ListUsersParams {
	limit := params.Limit
	if limit <= 0 {
		limit = defaultUserListLimit
	}
	if limit > maxUserListLimit {
		limit = maxUserListLimit
	}

	offset := params.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > maxUserListOffset {
		offset = maxUserListOffset
	}

	query := strings.TrimSpace(params.Query)
	if len([]rune(query)) > maxUserSearchLength {
		query = string([]rune(query)[:maxUserSearchLength])
	}

	return ListUsersParams{
		Limit:  limit,
		Offset: offset,
		Query:  query,
	}
}

func normalizeUpdateDocumentQuotaInput(input UpdateDocumentQuotaInput) (*UpdateDocumentQuotaInput, error) {
	mode := strings.TrimSpace(input.DocumentQuotaMode)
	switch mode {
	case models.DocumentQuotaModeInherit:
		if input.DocumentQuota != nil {
			return nil, ErrDocumentQuotaMustBeEmpty
		}
		input.DocumentQuota = nil
	case models.DocumentQuotaModeUnlimited:
		if input.DocumentQuota != nil {
			return nil, ErrDocumentQuotaMustBeEmpty
		}
		input.DocumentQuota = nil
	case models.DocumentQuotaModeCustom:
		if input.DocumentQuota == nil {
			return nil, ErrDocumentQuotaRequired
		}
		if *input.DocumentQuota < 0 {
			return nil, ErrDocumentQuotaInvalid
		}
		if *input.DocumentQuota > maxDocumentQuota {
			return nil, ErrDocumentQuotaTooLarge
		}
	default:
		return nil, ErrDocumentQuotaModeInvalid
	}

	input.DocumentQuotaMode = mode
	return &input, nil
}

func normalizeUpdateUserEmailInput(input UpdateUserEmailInput) (*string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(input.Email))
	if trimmed == "" {
		return nil, ErrEmailRequired
	}
	parsed, err := mail.ParseAddress(trimmed)
	if err != nil || !strings.EqualFold(parsed.Address, trimmed) {
		return nil, ErrEmailInvalid
	}
	return &trimmed, nil
}

func loadDocumentCountsByUserID(ids []uuid.UUID) (map[uuid.UUID]documentCountRow, error) {
	if len(ids) == 0 {
		return map[uuid.UUID]documentCountRow{}, nil
	}

	var rows []documentCountRow
	if err := database.DB.Unscoped().
		Model(&models.Document{}).
		Select(
			"owner_user_id, "+
				"SUM(CASE WHEN deleted_at IS NULL THEN 1 ELSE 0 END) AS active_count, "+
				"SUM(CASE WHEN deleted_at IS NOT NULL THEN 1 ELSE 0 END) AS trashed_count",
		).
		Where("owner_user_id IN ?", ids).
		Group("owner_user_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	result := make(map[uuid.UUID]documentCountRow, len(rows))
	for _, row := range rows {
		result[row.OwnerUserID] = row
	}
	return result, nil
}

func userIDs(users []models.User) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(users))
	for _, currentUser := range users {
		ids = append(ids, currentUser.ID)
	}
	return ids
}

func GlobalDocumentQuota() (*int, error) {
	return config.GetOptionalNonNegativeInt("DEFAULT_DOCUMENT_QUOTA")
}

func resolveEffectiveDocumentQuota(currentUser models.User, globalDocumentQuota *int) *int {
	switch strings.TrimSpace(currentUser.DocumentQuotaMode) {
	case models.DocumentQuotaModeUnlimited:
		return nil
	case models.DocumentQuotaModeCustom:
		return currentUser.DocumentQuota
	}

	if currentUser.DocumentQuota != nil {
		return currentUser.DocumentQuota
	}

	return globalDocumentQuota
}

func ensureUserExists(tx *gorm.DB, targetUserID uuid.UUID) error {
	var currentUser models.User
	return tx.Select("id").First(&currentUser, "id = ?", targetUserID).Error
}

func ListUserSessions(targetUserID uuid.UUID, params ListUserSessionsParams) (*AdminSessionListResult, error) {
	normalizedLimit := params.Limit
	if normalizedLimit <= 0 {
		normalizedLimit = 10
	}
	if normalizedLimit > 50 {
		normalizedLimit = 50
	}
	normalizedOffset := params.Offset
	if normalizedOffset < 0 {
		normalizedOffset = 0
	}
	if normalizedOffset > maxUserListOffset {
		normalizedOffset = maxUserListOffset
	}

	var currentUser models.User
	if err := database.DB.Select("id").First(&currentUser, "id = ?", targetUserID).Error; err != nil {
		return nil, err
	}

	now := time.Now()
	activeSessionsQuery := func() *gorm.DB {
		return database.DB.Table("user_sessions AS s").
			Select("s.id, MAX(s.last_seen_at) AS last_seen_at").
			Joins("JOIN user_refresh_tokens AS rt ON rt.session_id = s.id AND rt.user_id = s.user_id").
			Where("s.user_id = ? AND s.revoked_at IS NULL AND s.deleted_at IS NULL AND rt.expires_at > ?", targetUserID, now).
			Group("s.id")
	}

	var total int64
	if err := database.DB.Table("(?) AS active_sessions", activeSessionsQuery()).
		Count(&total).Error; err != nil {
		return nil, err
	}

	var sessionIDs []uuid.UUID
	if err := database.DB.Table("(?) AS active_sessions", activeSessionsQuery()).
		Select("id").
		Order("last_seen_at DESC").
		Order("id DESC").
		Limit(normalizedLimit + 1).
		Offset(normalizedOffset).
		Scan(&sessionIDs).Error; err != nil {
		return nil, err
	}

	hasMore := len(sessionIDs) > normalizedLimit
	if hasMore {
		sessionIDs = sessionIDs[:normalizedLimit]
	}
	if len(sessionIDs) == 0 {
		return &AdminSessionListResult{
			Items:      []AdminSessionItem{},
			HasMore:    false,
			NextOffset: normalizedOffset,
			Total:      total,
		}, nil
	}

	var sessions []models.UserSession
	if err := database.DB.
		Where("user_id = ? AND id IN ?", targetUserID, sessionIDs).
		Find(&sessions).Error; err != nil {
		return nil, err
	}

	sessionsByID := make(map[uuid.UUID]models.UserSession, len(sessions))
	for _, session := range sessions {
		sessionsByID[session.ID] = session
	}

	var refreshTokens []models.UserRefreshToken
	if err := database.DB.
		Where("user_id = ? AND session_id IN ? AND expires_at > ?", targetUserID, sessionIDs, now).
		Order("expires_at DESC").
		Find(&refreshTokens).Error; err != nil {
		return nil, err
	}

	expiresAtBySessionID := make(map[uuid.UUID]time.Time, len(refreshTokens))
	for _, token := range refreshTokens {
		if _, exists := expiresAtBySessionID[token.SessionID]; !exists {
			expiresAtBySessionID[token.SessionID] = token.ExpiresAt
		}
	}

	items := make([]AdminSessionItem, 0, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		session, ok := sessionsByID[sessionID]
		if !ok {
			continue
		}
		expiresAt, ok := expiresAtBySessionID[sessionID]
		if !ok {
			continue
		}
		items = append(items, AdminSessionItem{
			ID:          session.ID,
			DeviceLabel: session.DeviceLabel,
			UserAgent:   session.UserAgent,
			LastSeenAt:  session.LastSeenAt,
			ExpiresAt:   expiresAt,
			CreatedAt:   session.CreatedAt,
		})
	}

	return &AdminSessionListResult{
		Items:      items,
		HasMore:    hasMore,
		NextOffset: normalizedOffset + len(items),
		Total:      total,
	}, nil
}

func RevokeUserSession(adminUserID, targetUserID, sessionID uuid.UUID) error {
	if adminUserID == targetUserID {
		return ErrCannotRevokeOwnSession
	}

	now := time.Now()
	return database.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.UserSession{}).
			Where("id = ? AND user_id = ? AND revoked_at IS NULL", sessionID, targetUserID).
			Updates(map[string]any{
				"revoked_at": &now,
				"updated_at": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}

		return tx.Unscoped().
			Where("user_id = ? AND session_id = ?", targetUserID, sessionID).
			Delete(&models.UserRefreshToken{}).Error
	})
}

func ListUserMedia(targetUserID uuid.UUID, params ListUserMediaParams) (*media.ListAssetsResult, error) {
	if err := ensureUserExists(database.DB, targetUserID); err != nil {
		return nil, err
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	offset := params.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > maxUserListOffset {
		offset = maxUserListOffset
	}
	query := strings.TrimSpace(params.Query)
	if len([]rune(query)) > maxUserSearchLength {
		query = string([]rune(query)[:maxUserSearchLength])
	}

	return media.ListOwnedAssets(media.ListAssetsRequest{
		UserID: targetUserID,
		Kind:   strings.TrimSpace(params.Kind),
		Status: strings.TrimSpace(params.Status),
		Query:  query,
		Limit:  limit,
		Offset: offset,
	})
}

func PurgeUserMedia(targetUserID uuid.UUID) error {
	if err := ensureUserExists(database.DB, targetUserID); err != nil {
		return err
	}

	var assets []models.Asset
	if err := database.DB.Where("owner_user_id = ? AND deleted_at IS NULL", targetUserID).Find(&assets).Error; err != nil {
		return err
	}

	var deleteErrors []error
	for _, asset := range assets {
		if err := media.DeleteOwnedUnusedAsset(context.Background(), targetUserID, asset.ID); err != nil {
			deleteErrors = append(deleteErrors, fmt.Errorf("delete asset %s: %w", asset.ID, err))
		}
	}
	return errors.Join(deleteErrors...)
}

func PurgeUserDocuments(targetUserID uuid.UUID) error {
	if err := ensureUserExists(database.DB, targetUserID); err != nil {
		return err
	}

	return database.DB.Transaction(func(tx *gorm.DB) error {
		return purgeUserDocumentsTx(tx, targetUserID)
	})
}

func UnregisterUser(adminUserID, targetUserID uuid.UUID) error {
	if adminUserID == targetUserID {
		return ErrCannotUnregisterSelf
	}

	var user models.User
	if err := database.DB.First(&user, "id = ?", targetUserID).Error; err != nil {
		return err
	}

	if err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := purgeUserDocumentsTx(tx, targetUserID); err != nil {
			return err
		}

		return nil
	}); err != nil {
		return err
	}

	if err := PurgeUserMedia(targetUserID); err != nil {
		return err
	}

	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", targetUserID).Delete(&models.UserRefreshToken{}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", targetUserID).Delete(&models.UserSession{}).Error; err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", targetUserID).Delete(&models.UserImageBedConfig{}).Error; err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", targetUserID).Delete(&models.UserIdentityProvider{}).Error; err != nil {
			return err
		}

		if err := tx.Where("user_id = ?", targetUserID).Delete(&models.Notification{}).Error; err != nil {
			return err
		}

		if err := tx.Delete(&user).Error; err != nil {
			return err
		}

		return nil
	})
}

// purgeUserDocumentsTx permanently deletes all content and metadata for documents/folders owned by user
func purgeUserDocumentsTx(tx *gorm.DB, targetUserID uuid.UUID) error {
	var documents []models.Document
	if err := tx.Where("owner_user_id = ?", targetUserID).Find(&documents).Error; err != nil {
		return err
	}

	for _, doc := range documents {
		if err := content.PermanentDeleteContentByDocumentID(tx, targetUserID, doc.ID); err != nil {
			return err
		}
		if err := tx.Where("document_id = ?", doc.ID).Delete(&models.DocumentPermission{}).Error; err != nil {
			return err
		}
		if err := tx.Where("document_id = ?", doc.ID).Delete(&models.DocumentInvite{}).Error; err != nil {
			return err
		}
		if err := tx.Where("document_id = ?", doc.ID).Delete(&models.DocumentImageTargetPreference{}).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Delete(&doc).Error; err != nil {
			return err
		}
	}

	if err := tx.Unscoped().Where("owner_user_id = ?", targetUserID).Delete(&models.Folder{}).Error; err != nil {
		return err
	}

	return nil
}
