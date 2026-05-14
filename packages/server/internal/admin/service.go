package admin

import (
	"strings"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/config"
	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	userpkg "g.co1d.in/Coldin04/Cyime/server/internal/user"
	"github.com/google/uuid"
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

	items := make([]UserListItem, 0, len(users))
	for _, currentUser := range users {
		effectiveQuota, err := userpkg.GetEffectiveDocumentQuota(currentUser.ID)
		if err != nil {
			return nil, err
		}
		counts := countsByUserID[currentUser.ID]
		items = append(items, UserListItem{
			User:                 currentUser,
			ActiveDocumentCount:  counts.ActiveCount,
			TrashedDocumentCount: counts.TrashedCount,
			EffectiveQuota:       effectiveQuota,
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

func GetUser(targetUserID uuid.UUID) (*UserListItem, error) {
	var currentUser models.User
	if err := database.DB.First(&currentUser, "id = ?", targetUserID).Error; err != nil {
		return nil, err
	}

	countsByUserID, err := loadDocumentCountsByUserID([]uuid.UUID{targetUserID})
	if err != nil {
		return nil, err
	}

	effectiveQuota, err := userpkg.GetEffectiveDocumentQuota(targetUserID)
	if err != nil {
		return nil, err
	}

	counts := countsByUserID[targetUserID]
	return &UserListItem{
		User:                 currentUser,
		ActiveDocumentCount:  counts.ActiveCount,
		TrashedDocumentCount: counts.TrashedCount,
		EffectiveQuota:       effectiveQuota,
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
