package admin

import (
	"errors"
	"strconv"

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
		DisplayName:            updatedUser.DisplayName,
		AvatarURL:              avatarURL,
		AdminAccess:            userpkg.BuildAdminAccessDTO(updatedUser),
		DocumentQuotaMode:      normalizeDocumentQuotaMode(updatedUser.DocumentQuotaMode),
		DocumentQuota:          updatedUser.DocumentQuota,
		EffectiveDocumentQuota: effectiveQuota,
		Unlimited:              effectiveQuota == nil,
	})
}

func userListItemToDTO(baseURL string, item UserListItem) (UserListItemDTO, error) {
	avatarURL, err := userpkg.ResolveAvatarURL(baseURL, &item.User)
	if err != nil {
		return UserListItemDTO{}, err
	}

	return UserListItemDTO{
		ID:                     item.User.ID,
		Email:                  item.User.Email,
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
