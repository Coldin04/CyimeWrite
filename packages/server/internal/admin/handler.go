package admin

import (
	"errors"
	"strconv"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/media"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	userpkg "g.co1d.in/Coldin04/Cyime/server/internal/user"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OverviewResponseDTO struct {
	UserCount           int64 `json:"userCount"`
	AdminCount          int64 `json:"adminCount"`
	GlobalDocumentQuota *int  `json:"globalDocumentQuota"`
	GlobalUnlimited     bool  `json:"globalUnlimited"`
}

type UserListItemDTO struct {
	ID                     uuid.UUID              `json:"id"`
	Email                  *string                `json:"email"`
	EmailVerified          bool                   `json:"emailVerified"`
	DisplayName            *string                `json:"displayName"`
	AvatarURL              *string                `json:"avatarUrl"`
	AdminAccess            userpkg.AdminAccessDTO `json:"adminAccess"`
	DocumentQuotaMode      string                 `json:"documentQuotaMode"`
	DocumentQuota          *int                   `json:"documentQuota"`
	EffectiveDocumentQuota *int                   `json:"effectiveDocumentQuota"`
	Unlimited              bool                   `json:"unlimited"`
	ActiveDocumentCount    int64                  `json:"activeDocumentCount"`
	TrashedDocumentCount   int64                  `json:"trashedDocumentCount"`
	UsedDocumentCount      int64                  `json:"usedDocumentCount"`
}

type UserListResponseDTO struct {
	Items               []UserListItemDTO `json:"items"`
	HasMore             bool              `json:"hasMore"`
	NextOffset          int               `json:"nextOffset"`
	Total               int64             `json:"total"`
	GlobalDocumentQuota *int              `json:"globalDocumentQuota"`
	GlobalUnlimited     bool              `json:"globalUnlimited"`
}

type UpdateUserDocumentQuotaRequest struct {
	DocumentQuotaMode string `json:"documentQuotaMode"`
	DocumentQuota     *int   `json:"documentQuota"`
}

type UpdateUserEmailRequest struct {
	Email string `json:"email"`
}

type AdminSessionItemDTO struct {
	ID          uuid.UUID `json:"id"`
	DeviceLabel string    `json:"deviceLabel"`
	UserAgent   string    `json:"userAgent"`
	LastSeenAt  string    `json:"lastSeenAt"`
	ExpiresAt   string    `json:"expiresAt"`
	CreatedAt   string    `json:"createdAt"`
}

type AdminSessionListResponseDTO struct {
	Items      []AdminSessionItemDTO `json:"items"`
	HasMore    bool                  `json:"hasMore"`
	NextOffset int                   `json:"nextOffset"`
	Total      int64                 `json:"total"`
}

type AdminMediaListResponseDTO struct {
	Items   []AdminMediaItemDTO `json:"items"`
	HasMore bool                `json:"hasMore"`
	Total   int64               `json:"total"`
}

type AdminMediaItemDTO struct {
	ID             uuid.UUID `json:"id"`
	Kind           string    `json:"kind"`
	Filename       string    `json:"filename"`
	MimeType       string    `json:"mimeType"`
	FileSize       int64     `json:"fileSize"`
	ThumbnailURL   string    `json:"thumbnailUrl,omitempty"`
	Visibility     string    `json:"visibility"`
	Status         string    `json:"status"`
	ReferenceCount int       `json:"referenceCount"`
	Deletable      bool      `json:"deletable"`
	CreatedAt      string    `json:"createdAt"`
	UpdatedAt      string    `json:"updatedAt"`
}

func GetOverviewHandler(c *fiber.Ctx) error {
	stats, err := GetOverviewStats()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load admin overview."})
	}

	globalDocumentQuota, err := GlobalDocumentQuota()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load global quota."})
	}

	return c.JSON(OverviewResponseDTO{
		UserCount:           stats.UserCount,
		AdminCount:          stats.AdminCount,
		GlobalDocumentQuota: globalDocumentQuota,
		GlobalUnlimited:     globalDocumentQuota == nil,
	})
}

func ListUsersHandler(c *fiber.Ctx) error {
	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	result, err := ListUsers(ListUsersParams{
		Limit:  limit,
		Offset: offset,
		Query:  c.Query("q"),
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load users."})
	}

	globalDocumentQuota, err := GlobalDocumentQuota()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load global quota."})
	}

	items := make([]UserListItemDTO, 0, len(result.Items))
	for _, item := range result.Items {
		dto, err := userListItemToDTO(c.BaseURL(), item)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		items = append(items, dto)
	}

	return c.JSON(UserListResponseDTO{
		Items:               items,
		HasMore:             result.HasMore,
		NextOffset:          result.NextOffset,
		Total:               result.Total,
		GlobalDocumentQuota: globalDocumentQuota,
		GlobalUnlimited:     globalDocumentQuota == nil,
	})
}

func GetUserHandler(c *fiber.Ctx) error {
	targetUserID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user id."})
	}

	item, err := GetUser(targetUserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found."})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load user detail."})
	}

	dto, err := userListItemToDTO(c.BaseURL(), *item)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(dto)
}

func ListUserSessionsHandler(c *fiber.Ctx) error {
	targetUserID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user id."})
	}

	limit, _ := strconv.Atoi(c.Query("limit"))
	offset, _ := strconv.Atoi(c.Query("offset"))
	result, err := ListUserSessions(targetUserID, ListUserSessionsParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found."})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load user sessions."})
	}

	items := make([]AdminSessionItemDTO, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, AdminSessionItemDTO{
			ID:          item.ID,
			DeviceLabel: item.DeviceLabel,
			UserAgent:   item.UserAgent,
			LastSeenAt:  item.LastSeenAt.UTC().Format(time.RFC3339),
			ExpiresAt:   item.ExpiresAt.UTC().Format(time.RFC3339),
			CreatedAt:   item.CreatedAt.UTC().Format(time.RFC3339),
		})
	}

	return c.JSON(AdminSessionListResponseDTO{
		Items:      items,
		HasMore:    result.HasMore,
		NextOffset: result.NextOffset,
		Total:      result.Total,
	})
}

func RevokeUserSessionHandler(c *fiber.Ctx) error {
	adminUserIDStr, ok := c.Locals("userId").(string)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	adminUserID, err := uuid.Parse(adminUserIDStr)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}

	targetUserID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user id."})
	}

	sessionID, err := uuid.Parse(c.Params("sessionId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid session id."})
	}

	if err := RevokeUserSession(adminUserID, targetUserID, sessionID); err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Session not found."})
		case errors.Is(err, ErrCannotRevokeOwnSession):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to revoke user session."})
		}
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func ListUserMediaHandler(c *fiber.Ctx) error {
	targetUserID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user id."})
	}

	result, err := ListUserMedia(targetUserID, ListUserMediaParams{
		Limit:  c.QueryInt("limit", 20),
		Offset: c.QueryInt("offset", 0),
		Query:  c.Query("q"),
		Kind:   c.Query("kind"),
		Status: c.Query("status"),
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found."})
		case errors.Is(err, media.ErrInvalidAssetStatus), errors.Is(err, media.ErrInvalidAssetKind):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to load user media."})
		}
	}

	items := make([]AdminMediaItemDTO, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, AdminMediaItemDTO{
			ID:             item.ID,
			Kind:           item.Kind,
			Filename:       item.Filename,
			MimeType:       item.MimeType,
			FileSize:       item.FileSize,
			ThumbnailURL:   item.ThumbnailURL,
			Visibility:     item.Visibility,
			Status:         item.Status,
			ReferenceCount: item.ReferenceCount,
			Deletable:      item.Deletable,
			CreatedAt:      item.CreatedAt.UTC().Format(time.RFC3339),
			UpdatedAt:      item.UpdatedAt.UTC().Format(time.RFC3339),
		})
	}

	return c.JSON(AdminMediaListResponseDTO{
		Items:   items,
		HasMore: result.HasMore,
		Total:   result.Total,
	})
}

func UpdateUserDocumentQuotaHandler(c *fiber.Ctx) error {
	targetUserID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user id."})
	}

	var req UpdateUserDocumentQuotaRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body."})
	}

	updatedUser, err := UpdateUserDocumentQuota(targetUserID, UpdateDocumentQuotaInput{
		DocumentQuotaMode: req.DocumentQuotaMode,
		DocumentQuota:     req.DocumentQuota,
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found."})
		case errors.Is(err, ErrDocumentQuotaModeInvalid),
			errors.Is(err, ErrDocumentQuotaRequired),
			errors.Is(err, ErrDocumentQuotaInvalid),
			errors.Is(err, ErrDocumentQuotaTooLarge),
			errors.Is(err, ErrDocumentQuotaMustBeEmpty):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update user quota."})
		}
	}

	effectiveQuota, err := userpkg.GetEffectiveDocumentQuota(updatedUser.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to resolve effective quota."})
	}

	avatarURL, err := userpkg.ResolveAvatarURL(c.BaseURL(), updatedUser)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(UserListItemDTO{
		ID:                     updatedUser.ID,
		Email:                  updatedUser.Email,
		EmailVerified:          updatedUser.EmailVerified,
		DisplayName:            updatedUser.DisplayName,
		AvatarURL:              avatarURL,
		AdminAccess:            userpkg.BuildAdminAccessDTO(updatedUser),
		DocumentQuotaMode:      normalizeDocumentQuotaMode(updatedUser.DocumentQuotaMode),
		DocumentQuota:          updatedUser.DocumentQuota,
		EffectiveDocumentQuota: effectiveQuota,
		Unlimited:              effectiveQuota == nil,
	})
}

func UpdateUserEmailHandler(c *fiber.Ctx) error {
	targetUserID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user id."})
	}

	var req UpdateUserEmailRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body."})
	}

	updatedUser, err := UpdateUserEmail(targetUserID, UpdateUserEmailInput{Email: req.Email})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found."})
		case errors.Is(err, ErrEmailRequired),
			errors.Is(err, ErrEmailInvalid),
			errors.Is(err, ErrEmailAlreadyInUse):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to update user email."})
		}
	}

	item, err := GetUser(updatedUser.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to reload user detail."})
	}
	dto, err := userListItemToDTO(c.BaseURL(), *item)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(dto)
}

func VerifyUserEmailHandler(c *fiber.Ctx) error {
	targetUserID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user id."})
	}

	updatedUser, err := VerifyUserEmail(targetUserID)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found."})
		case errors.Is(err, ErrEmailNotSet):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to verify user email."})
		}
	}

	item, err := GetUser(updatedUser.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to reload user detail."})
	}
	dto, err := userListItemToDTO(c.BaseURL(), *item)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(dto)
}

func PurgeUserMediaHandler(c *fiber.Ctx) error {
	targetUserID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user id."})
	}

	if err := PurgeUserMedia(targetUserID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found."})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to purge user media."})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func PurgeUserDocumentsHandler(c *fiber.Ctx) error {
	targetUserID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user id."})
	}

	if err := PurgeUserDocuments(targetUserID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found."})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to purge user documents."})
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func UnregisterUserHandler(c *fiber.Ctx) error {
	adminUserIDStr, ok := c.Locals("userId").(string)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized"})
	}
	adminUserID, _ := uuid.Parse(adminUserIDStr)

	targetUserID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user id."})
	}

	if err := UnregisterUser(adminUserID, targetUserID); err != nil {
		switch {
		case errors.Is(err, ErrCannotUnregisterSelf):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		case errors.Is(err, gorm.ErrRecordNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found."})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to unregister user."})
		}
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func userListItemToDTO(baseURL string, item UserListItem) (UserListItemDTO, error) {
	avatarURL, err := userpkg.ResolveAvatarURL(baseURL, &item.User)
	if err != nil {
		return UserListItemDTO{}, err
	}

	return UserListItemDTO{
		ID:                     item.User.ID,
		Email:                  item.User.Email,
		EmailVerified:          item.User.EmailVerified,
		DisplayName:            item.User.DisplayName,
		AvatarURL:              avatarURL,
		AdminAccess:            userpkg.BuildAdminAccessDTO(&item.User),
		DocumentQuotaMode:      normalizeDocumentQuotaMode(item.User.DocumentQuotaMode),
		DocumentQuota:          item.User.DocumentQuota,
		EffectiveDocumentQuota: item.EffectiveQuota,
		Unlimited:              item.EffectiveQuota == nil,
		ActiveDocumentCount:    item.ActiveDocumentCount,
		TrashedDocumentCount:   item.TrashedDocumentCount,
		UsedDocumentCount:      item.ActiveDocumentCount + item.TrashedDocumentCount,
	}, nil
}

func normalizeDocumentQuotaMode(mode string) string {
	switch mode {
	case models.DocumentQuotaModeCustom:
		return models.DocumentQuotaModeCustom
	case models.DocumentQuotaModeUnlimited:
		return models.DocumentQuotaModeUnlimited
	default:
		return models.DocumentQuotaModeInherit
	}
}
