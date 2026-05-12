package user

import (
	"context"
	"errors"

	"g.co1d.in/Coldin04/Cyime/server/internal/imagebeds"
	"g.co1d.in/Coldin04/Cyime/server/internal/media"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UserResponseDTO defines the data structure for the user profile response.
// This prevents leaking unwanted or sensitive fields from the database model.
type UserResponseDTO struct {
	ID          uuid.UUID `json:"id"`
	Email       *string   `json:"email"`
	DisplayName *string   `json:"displayName"`
	AvatarURL   *string   `json:"avatarUrl"`
}

type OverviewResponseDTO struct {
	ActiveDocumentCount  int64 `json:"activeDocumentCount"`
	TrashedDocumentCount int64 `json:"trashedDocumentCount"`
	DocumentLimit        *int  `json:"documentLimit"`
	Unlimited            bool  `json:"unlimited"`
}

type UpdateProfileRequest struct {
	DisplayName string `json:"displayName"`
}

type UpdateGitHubAvatarRequest struct {
	Username string `json:"username"`
}

type ImageBedConfigDTO struct {
	ID           uuid.UUID         `json:"id"`
	Name         string            `json:"name"`
	ProviderType string            `json:"providerType"`
	BaseURL      string            `json:"baseUrl"`
	APIToken     string            `json:"apiToken"`
	HasAPIToken  bool              `json:"hasApiToken"`
	IsEnabled    bool              `json:"isEnabled"`
	StorageID    int               `json:"storageId,omitempty"`
	StrategyID   string            `json:"strategyId,omitempty"`
	FieldValues  map[string]string `json:"fieldValues,omitempty"`
}

type UpsertImageBedConfigRequest struct {
	Name         string            `json:"name"`
	ProviderType string            `json:"providerType"`
	BaseURL      string            `json:"baseUrl"`
	APIToken     string            `json:"apiToken"`
	IsEnabled    bool              `json:"isEnabled"`
	StorageID    int               `json:"storageId"`
	StrategyID   string            `json:"strategyId"`
	FieldValues  map[string]string `json:"fieldValues"`
}

type ImageBedProviderFieldDTO struct {
	Key            string `json:"key"`
	Type           string `json:"type"`
	Label          string `json:"label"`
	LabelKey       string `json:"labelKey,omitempty"`
	Placeholder    string `json:"placeholder"`
	PlaceholderKey string `json:"placeholderKey,omitempty"`
	HelpText       string `json:"helpText,omitempty"`
	HelpTextKey    string `json:"helpTextKey,omitempty"`
	InputMode      string `json:"inputMode,omitempty"`
	Required       bool   `json:"required"`
}

type ImageBedProviderDTO struct {
	ProviderType string                     `json:"providerType"`
	DisplayName  string                     `json:"displayName"`
	Description  string                     `json:"description"`
	Fields       []ImageBedProviderFieldDTO `json:"fields"`
}

// GetMe handles the GET /api/v1/user/me request.
// It relies on the Protected middleware to have already validated the JWT
// and placed the userId in the context.
func GetMe(c *fiber.Ctx) error {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized: Invalid token context.",
		})
	}

	user, err := GetUserByID(userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "User not found.",
		})
	}

	response, err := toUserResponseDTO(c, user)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(response)
}

// GetOverview handles GET /api/v1/user/overview.
func GetOverview(c *fiber.Ctx) error {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized: Invalid token context.",
		})
	}

	stats, err := GetOverviewStats(userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to load overview.",
		})
	}

	return c.JSON(OverviewResponseDTO{
		ActiveDocumentCount:  stats.ActiveDocumentCount,
		TrashedDocumentCount: stats.TrashedDocumentCount,
		DocumentLimit:        stats.DocumentLimit,
		Unlimited:            stats.Unlimited,
	})
}

func ListImageBedConfigsHandler(c *fiber.Ctx) error {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"error": "Unauthorized: Invalid token context.",
		})
	}

	items, err := ListImageBedConfigs(userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to load image bed configs.",
		})
	}

	result := make([]ImageBedConfigDTO, 0, len(items))
	for _, item := range items {
		result = append(result, imageBedConfigToDTO(item))
	}

	return c.JSON(fiber.Map{"items": result})
}

func ListImageBedProvidersHandler(c *fiber.Ctx) error {
	providers := imagebeds.ListProviders()
	items := make([]ImageBedProviderDTO, 0, len(providers))
	for _, provider := range providers {
		fieldItems := make([]ImageBedProviderFieldDTO, 0, len(provider.Fields))
		for _, field := range provider.Fields {
			fieldItems = append(fieldItems, ImageBedProviderFieldDTO{
				Key:            field.Key,
				Type:           field.Type,
				Label:          field.Label,
				LabelKey:       field.LabelKey,
				Placeholder:    field.Placeholder,
				PlaceholderKey: field.PlaceholderKey,
				HelpText:       field.HelpText,
				HelpTextKey:    field.HelpTextKey,
				InputMode:      field.InputMode,
				Required:       field.Required,
			})
		}

		items = append(items, ImageBedProviderDTO{
			ProviderType: provider.ProviderType,
			DisplayName:  provider.DisplayName,
			Description:  provider.Description,
			Fields:       fieldItems,
		})
	}

	return c.JSON(fiber.Map{"items": items})
}

func CreateImageBedConfigHandler(c *fiber.Ctx) error {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid user ID format in token.",
		})
	}

	var req UpsertImageBedConfigRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body.",
		})
	}

	config, err := CreateImageBedConfig(userID, UpsertImageBedConfigInput{
		Name:         req.Name,
		ProviderType: req.ProviderType,
		BaseURL:      req.BaseURL,
		APIToken:     req.APIToken,
		IsEnabled:    req.IsEnabled,
		StorageID:    req.StorageID,
		StrategyID:   req.StrategyID,
		FieldValues:  req.FieldValues,
	})
	if err != nil {
		var configErr *ImageBedConfigError
		if errors.As(err, &configErr) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": configErr.Message})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(fiber.StatusCreated).JSON(imageBedConfigToDTO(*config))
}

func UpdateImageBedConfigHandler(c *fiber.Ctx) error {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid user ID format in token.",
		})
	}

	configID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid image bed config id."})
	}

	var req UpsertImageBedConfigRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body.",
		})
	}

	config, err := UpdateImageBedConfig(userID, configID, UpsertImageBedConfigInput{
		Name:         req.Name,
		ProviderType: req.ProviderType,
		BaseURL:      req.BaseURL,
		APIToken:     req.APIToken,
		IsEnabled:    req.IsEnabled,
		StorageID:    req.StorageID,
		StrategyID:   req.StrategyID,
		FieldValues:  req.FieldValues,
	})
	if err != nil {
		var configErr *ImageBedConfigError
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Image bed config not found."})
		case errors.As(err, &configErr):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": configErr.Message})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	}

	return c.JSON(imageBedConfigToDTO(*config))
}

func DeleteImageBedConfigHandler(c *fiber.Ctx) error {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid user ID format in token.",
		})
	}

	configID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid image bed config id."})
	}

	if err := DeleteImageBedConfig(userID, configID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Image bed config not found."})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true})
}

func UpdateProfileHandler(c *fiber.Ctx) error {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid user ID format in token.",
		})
	}

	var req UpdateProfileRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body.",
		})
	}

	user, err := UpdateProfile(userID, req.DisplayName)
	if err != nil {
		switch {
		case errors.Is(err, ErrDisplayNameRequired), errors.Is(err, ErrDisplayNameTooLong):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	}

	response, err := toUserResponseDTO(c, user)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(response)
}

func UploadAvatarHandler(c *fiber.Ctx) error {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID format in token."})
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file is required"})
	}

	user, err := UpdateAvatarWithUpload(context.Background(), userID, fileHeader)
	if err != nil {
		var unsupportedAvatar *media.UnsupportedAvatarFileTypeError
		switch {
		case errors.Is(err, media.ErrFileRequired):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		default:
			if errors.Is(err, media.ErrAvatarFileTooLarge) {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
			}
			if errors.As(err, &unsupportedAvatar) {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
			}
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	}

	response, err := toUserResponseDTO(c, user)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(response)
}

func UpdateGitHubAvatarHandler(c *fiber.Ctx) error {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid user ID format in token."})
	}

	var req UpdateGitHubAvatarRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body."})
	}

	user, err := UpdateAvatarWithGitHub(context.Background(), userID, req.Username)
	if err != nil {
		switch {
		case errors.Is(err, ErrGitHubUsernameRequired), errors.Is(err, ErrGitHubUsernameInvalid):
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
	}

	response, err := toUserResponseDTO(c, user)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(response)
}

func GetAvatarContentHandler(c *fiber.Ctx) error {
	userID, err := getUserIDFromContext(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "Unauthorized: Invalid token context."})
	}

	user, err := GetUserByID(userID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "User not found"})
	}
	objectKey := trimStringPtr(user.AvatarObjectKey)
	if objectKey == "" {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "Avatar not found"})
	}

	if err := media.InitStorageProviderForAvatarRead(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}
	obj, err := media.GetStoredObject(context.Background(), objectKey)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	// No defer Close — fasthttp closes the stream after flushing the
	// response. A deferred Close here would fire before the reader is
	// actually drained and the handler would emit a half-read body or
	// "file already closed" errors in tests.

	c.Set("Content-Type", obj.ContentType)
	c.Set("Cache-Control", "private, max-age=60")
	// Stream the avatar body straight to the response instead of buffering
	// it in memory (previous io.ReadAll + c.Send peak = 2 × avatar size).
	return c.SendStream(obj.Body)
}

func imageBedConfigToDTO(config ImageBedConfig) ImageBedConfigDTO {
	return ImageBedConfigDTO{
		ID:           config.ID,
		Name:         config.Name,
		ProviderType: config.ProviderType,
		BaseURL:      config.BaseURL,
		APIToken:     "",
		HasAPIToken:  config.HasAPIToken,
		IsEnabled:    config.IsEnabled,
		StorageID:    config.StorageID,
		StrategyID:   config.StrategyID,
		FieldValues:  config.FieldValues,
	}
}

func getUserIDFromContext(c *fiber.Ctx) (uuid.UUID, error) {
	userIdStr, ok := c.Locals("userId").(string)
	if !ok {
		return uuid.Nil, fiber.ErrUnauthorized
	}

	return uuid.Parse(userIdStr)
}

func toUserResponseDTO(c *fiber.Ctx, user *models.User) (UserResponseDTO, error) {
	avatarURL, err := ResolveAvatarURL(c.BaseURL(), user)
	if err != nil {
		return UserResponseDTO{}, err
	}
	return UserResponseDTO{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		AvatarURL:   avatarURL,
	}, nil
}
