package user

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"mime/multipart"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"g.co1d.in/Coldin04/Cyime/server/internal/config"
	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/imagebeds"
	"g.co1d.in/Coldin04/Cyime/server/internal/media"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"g.co1d.in/Coldin04/Cyime/server/internal/securevalue"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var githubUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37})$`)

// OverviewStats stores the lightweight numbers shown in the user overview panel.
type OverviewStats struct {
	ActiveDocumentCount  int64
	TrashedDocumentCount int64
	DocumentLimit        *int
	Unlimited            bool
}

type ImageBedConfigErrorCode string

const (
	ImageBedConfigErrNameRequired        ImageBedConfigErrorCode = "image_bed_name_required"
	ImageBedConfigErrNameTooLong         ImageBedConfigErrorCode = "image_bed_name_too_long"
	ImageBedConfigErrUnsupportedProvider ImageBedConfigErrorCode = "image_bed_unsupported_provider"
	ImageBedConfigErrFieldRequired       ImageBedConfigErrorCode = "image_bed_field_required"
	ImageBedConfigErrFieldInvalid        ImageBedConfigErrorCode = "image_bed_field_invalid"
)

type ImageBedConfigError struct {
	Code    ImageBedConfigErrorCode
	Field   string
	Message string
}

func (e *ImageBedConfigError) Error() string {
	return e.Message
}

func newImageBedConfigError(code ImageBedConfigErrorCode, field string, message string) error {
	return &ImageBedConfigError{
		Code:    code,
		Field:   field,
		Message: message,
	}
}

type ImageBedProviderField = imagebeds.ProviderField
type ImageBedProvider = imagebeds.Provider

func GetUserByID(userID uuid.UUID) (*models.User, error) {
	var user models.User
	if err := database.DB.First(&user, "id = ?", userID).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func getUserByIDWithDB(db *gorm.DB, userID uuid.UUID) (*models.User, error) {
	var user models.User
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

// GetEffectiveDocumentQuota resolves the effective document limit for one user.
// 优先使用用户自己的配额；如果用户没有单独配置，则回退到全局默认值；都没有时表示无限制。
func GetEffectiveDocumentQuota(userID uuid.UUID) (*int, error) {
	return GetEffectiveDocumentQuotaWithDB(database.DB, userID)
}

// GetEffectiveDocumentQuotaWithDB resolves the effective document limit using the
// provided handle so callers inside a transaction do not deadlock by re-entering
// the global single-connection SQLite pool.
func GetEffectiveDocumentQuotaWithDB(db *gorm.DB, userID uuid.UUID) (*int, error) {
	currentUser, err := getUserByIDWithDB(db, userID)
	if err != nil {
		return nil, err
	}
	if currentUser.DocumentQuota != nil {
		return currentUser.DocumentQuota, nil
	}

	return config.GetOptionalNonNegativeInt("DEFAULT_DOCUMENT_QUOTA")
}

// GetOverviewStats returns overview document counts for the current user.
func GetOverviewStats(userID uuid.UUID) (*OverviewStats, error) {
	limit, err := GetEffectiveDocumentQuota(userID)
	if err != nil {
		return nil, err
	}

	var activeCount int64
	if err := database.DB.Model(&models.Document{}).
		Where("owner_user_id = ? AND deleted_at IS NULL", userID).
		Count(&activeCount).Error; err != nil {
		return nil, err
	}

	var trashedCount int64
	if err := database.DB.Unscoped().Model(&models.Document{}).
		Where("owner_user_id = ? AND deleted_at IS NOT NULL", userID).
		Count(&trashedCount).Error; err != nil {
		return nil, err
	}

	return &OverviewStats{
		ActiveDocumentCount:  activeCount,
		TrashedDocumentCount: trashedCount,
		DocumentLimit:        limit,
		Unlimited:            limit == nil,
	}, nil
}

func UpdateProfile(userID uuid.UUID, displayName string) (*models.User, error) {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return nil, ErrDisplayNameRequired
	}
	if len([]rune(displayName)) > 80 {
		return nil, ErrDisplayNameTooLong
	}

	if err := database.DB.Model(&models.User{}).
		Where("id = ?", userID).
		Update("display_name", displayName).Error; err != nil {
		return nil, err
	}

	return GetUserByID(userID)
}

type ImageBedConfig struct {
	ID           uuid.UUID
	Name         string
	ProviderType string
	BaseURL      string
	HasAPIToken  bool
	IsEnabled    bool
	StorageID    int
	StrategyID   string
	FieldValues  map[string]string
}

type UpsertImageBedConfigInput struct {
	Name              string
	ProviderType      string
	BaseURL           string
	APIToken          string
	EncryptedAPIToken string
	IsEnabled         bool
	StorageID         int
	StrategyID        string
	FieldValues       map[string]string
}

type imageBedConfigPayload struct {
	Fields map[string]string `json:"fields,omitempty"`
}

func ListImageBedConfigs(userID uuid.UUID) ([]ImageBedConfig, error) {
	var rows []models.UserImageBedConfig
	if err := database.DB.
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("created_at ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	items := make([]ImageBedConfig, 0, len(rows))
	for _, row := range rows {
		item, err := imageBedModelToConfig(row)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func CreateImageBedConfig(userID uuid.UUID, input UpsertImageBedConfigInput) (*ImageBedConfig, error) {
	normalized, err := normalizeImageBedConfigInput(input)
	if err != nil {
		return nil, err
	}

	row := models.UserImageBedConfig{
		UserID:       userID,
		Name:         normalized.Name,
		ProviderType: normalized.ProviderType,
		BaseURL:      stringPtrOrNil(normalized.BaseURL),
		APIToken:     stringPtrOrNil(normalized.EncryptedAPIToken),
		ConfigJSON:   stringPtrOrNil(buildImageBedConfigJSON(stripStoredFieldValues(normalized.FieldValues))),
		IsEnabled:    normalized.IsEnabled,
	}
	if err := database.DB.Create(&row).Error; err != nil {
		return nil, err
	}

	config, err := imageBedModelToConfig(row)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func UpdateImageBedConfig(userID uuid.UUID, configID uuid.UUID, input UpsertImageBedConfigInput) (*ImageBedConfig, error) {
	var row models.UserImageBedConfig
	if err := database.DB.
		Where("id = ? AND user_id = ? AND deleted_at IS NULL", configID, userID).
		First(&row).Error; err != nil {
		return nil, err
	}

	// Secrets are not returned to the client. Allow leaving token empty to keep existing value.
	if strings.TrimSpace(input.APIToken) == "" && strings.TrimSpace(input.EncryptedAPIToken) == "" {
		input.EncryptedAPIToken = trimStringPtr(row.APIToken)
	}
	// Allow leaving baseUrl empty to keep existing value.
	if strings.TrimSpace(input.BaseURL) == "" {
		input.BaseURL = trimStringPtr(row.BaseURL)
	}

	normalized, err := normalizeImageBedConfigInput(input)
	if err != nil {
		return nil, err
	}

	updates := map[string]any{
		"name":          normalized.Name,
		"provider_type": normalized.ProviderType,
		"base_url":      nullableTrimmedString(normalized.BaseURL),
		"api_token":     nullableTrimmedString(normalized.EncryptedAPIToken),
		"config_json":   nullableTrimmedString(buildImageBedConfigJSON(stripStoredFieldValues(normalized.FieldValues))),
		"is_enabled":    normalized.IsEnabled,
	}
	if err := database.DB.Model(&row).Updates(updates).Error; err != nil {
		return nil, err
	}

	if err := database.DB.
		Where("id = ? AND user_id = ? AND deleted_at IS NULL", configID, userID).
		First(&row).Error; err != nil {
		return nil, err
	}

	config, err := imageBedModelToConfig(row)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func DeleteImageBedConfig(userID uuid.UUID, configID uuid.UUID) error {
	result := database.DB.
		Where("id = ? AND user_id = ? AND deleted_at IS NULL", configID, userID).
		Delete(&models.UserImageBedConfig{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func GetImageBedConfigByID(userID uuid.UUID, configID uuid.UUID) (*ImageBedConfig, error) {
	var row models.UserImageBedConfig
	if err := database.DB.
		Where("id = ? AND user_id = ? AND deleted_at IS NULL", configID, userID).
		First(&row).Error; err != nil {
		return nil, err
	}

	config, err := imageBedModelToConfig(row)
	if err != nil {
		return nil, err
	}
	return &config, nil
}

func normalizeImageBedConfigInput(input UpsertImageBedConfigInput) (*UpsertImageBedConfigInput, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, newImageBedConfigError(ImageBedConfigErrNameRequired, "name", "image bed name is required")
	}
	if len([]rune(name)) > 120 {
		return nil, newImageBedConfigError(ImageBedConfigErrNameTooLong, "name", "image bed name is too long")
	}

	providerType := strings.TrimSpace(input.ProviderType)
	provider, ok := imagebeds.GetProviderByType(providerType)
	if !ok {
		return nil, newImageBedConfigError(ImageBedConfigErrUnsupportedProvider, "providerType", "unsupported image bed provider")
	}

	fieldValues := normalizeFieldValues(input)
	if strings.TrimSpace(fieldValues["apiToken"]) == "" && strings.TrimSpace(input.EncryptedAPIToken) != "" {
		fieldValues["apiToken"] = "(configured)"
	}

	for _, field := range provider.Fields {
		value := strings.TrimSpace(fieldValues[field.Key])
		if field.Required && value == "" {
			return nil, newImageBedConfigError(ImageBedConfigErrFieldRequired, field.Key, fmt.Sprintf("%s is required", field.Key))
		}
		if value == "" {
			continue
		}
		switch field.Type {
		case "url":
			parsed, err := url.Parse(value)
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
				return nil, newImageBedConfigError(ImageBedConfigErrFieldInvalid, field.Key, fmt.Sprintf("invalid url for %s", field.Key))
			}
			fieldValues[field.Key] = strings.TrimRight(parsed.String(), "/")
		case "number":
			num, err := strconv.Atoi(value)
			if err != nil || num <= 0 {
				return nil, newImageBedConfigError(ImageBedConfigErrFieldInvalid, field.Key, fmt.Sprintf("invalid number for %s", field.Key))
			}
			fieldValues[field.Key] = strconv.Itoa(num)
		default:
			fieldValues[field.Key] = value
		}
	}

	input.Name = name
	input.ProviderType = providerType
	input.FieldValues = fieldValues
	input.BaseURL = strings.TrimSpace(fieldValues["baseUrl"])
	input.APIToken = strings.TrimSpace(fieldValues["apiToken"])
	if input.APIToken != "" && input.APIToken != "(configured)" {
		encryptedToken, err := securevalue.EncryptString(input.APIToken)
		if err != nil {
			return nil, err
		}
		input.EncryptedAPIToken = encryptedToken
	}
	input.StrategyID = strings.TrimSpace(fieldValues["strategyId"])
	input.StorageID = 0
	if storageIDRaw := strings.TrimSpace(fieldValues["storageId"]); storageIDRaw != "" {
		storageID, _ := strconv.Atoi(storageIDRaw)
		input.StorageID = storageID
	}
	return &input, nil
}

func imageBedModelToConfig(row models.UserImageBedConfig) (ImageBedConfig, error) {
	fieldValues := parseImageBedConfigJSON(row.ConfigJSON)
	baseURL := trimStringPtr(row.BaseURL)
	hasAPIToken := strings.TrimSpace(trimStringPtr(row.APIToken)) != ""
	if baseURL != "" && strings.TrimSpace(fieldValues["baseUrl"]) == "" {
		fieldValues["baseUrl"] = baseURL
	}

	storageID := 0
	if storageIDRaw := strings.TrimSpace(fieldValues["storageId"]); storageIDRaw != "" {
		parsed, err := strconv.Atoi(storageIDRaw)
		if err == nil {
			storageID = parsed
		}
	}
	strategyID := strings.TrimSpace(fieldValues["strategyId"])

	return ImageBedConfig{
		ID:           row.ID,
		Name:         row.Name,
		ProviderType: row.ProviderType,
		BaseURL:      baseURL,
		HasAPIToken:  hasAPIToken,
		IsEnabled:    row.IsEnabled,
		StorageID:    storageID,
		StrategyID:   strategyID,
		FieldValues:  fieldValues,
	}, nil
}

func parseImageBedConfigJSON(value *string) map[string]string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return map[string]string{}
	}

	var payload imageBedConfigPayload
	if err := json.Unmarshal([]byte(*value), &payload); err == nil && len(payload.Fields) > 0 {
		return normalizeFieldValueMap(payload.Fields)
	}

	var legacy map[string]any
	if err := json.Unmarshal([]byte(*value), &legacy); err != nil {
		return map[string]string{}
	}

	fieldValues := map[string]string{}
	if fieldsRaw, ok := legacy["fields"]; ok {
		if fields, ok := fieldsRaw.(map[string]any); ok {
			for key, raw := range fields {
				stringified := strings.TrimSpace(fmt.Sprintf("%v", raw))
				if stringified != "" {
					fieldValues[key] = stringified
				}
			}
		}
	}
	if storageRaw, ok := legacy["storageId"]; ok {
		stringified := strings.TrimSpace(fmt.Sprintf("%v", storageRaw))
		stringified = strings.TrimSuffix(stringified, ".0")
		if stringified != "" {
			fieldValues["storageId"] = stringified
		}
	}
	if strategyRaw, ok := legacy["strategyId"]; ok {
		stringified := strings.TrimSpace(fmt.Sprintf("%v", strategyRaw))
		if stringified != "" {
			fieldValues["strategyId"] = stringified
		}
	}
	return normalizeFieldValueMap(fieldValues)
}

func buildImageBedConfigJSON(fieldValues map[string]string) string {
	normalized := normalizeFieldValueMap(fieldValues)
	if len(normalized) == 0 {
		return ""
	}

	payload, err := json.Marshal(imageBedConfigPayload{
		Fields: normalized,
	})
	if err != nil {
		return ""
	}
	return string(payload)
}

func normalizeFieldValues(input UpsertImageBedConfigInput) map[string]string {
	values := normalizeFieldValueMap(input.FieldValues)

	if trimmed := strings.TrimSpace(input.BaseURL); trimmed != "" {
		values["baseUrl"] = trimmed
	}
	if trimmed := strings.TrimSpace(input.APIToken); trimmed != "" {
		values["apiToken"] = trimmed
	}
	if input.StorageID > 0 {
		values["storageId"] = strconv.Itoa(input.StorageID)
	}
	if trimmed := strings.TrimSpace(input.StrategyID); trimmed != "" {
		values["strategyId"] = trimmed
	}

	return values
}

func stripStoredFieldValues(input map[string]string) map[string]string {
	values := normalizeFieldValueMap(input)
	delete(values, "apiToken")
	delete(values, "baseUrl")
	return values
}

func normalizeFieldValueMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return map[string]string{}
	}

	values := make(map[string]string, len(input))
	for key, value := range input {
		trimmedKey := strings.TrimSpace(key)
		trimmedValue := strings.TrimSpace(value)
		if trimmedKey == "" || trimmedValue == "" {
			continue
		}
		values[trimmedKey] = trimmedValue
	}
	return values
}

func ResolveAvatarURL(baseURL string, user *models.User) (*string, error) {
	avatarURL := trimStringPtr(user.AvatarURL)
	avatarObjectKey := trimStringPtr(user.AvatarObjectKey)
	if avatarURL == "" {
		return nil, nil
	}
	if avatarObjectKey == "" {
		return &avatarURL, nil
	}

	resolved := strings.TrimRight(baseURL, "/") + "/api/v1/user/avatar/content"
	return &resolved, nil
}

func UpdateAvatarWithUpload(ctx context.Context, userID uuid.UUID, fileHeader *multipart.FileHeader) (*models.User, error) {
	currentUser, err := GetUserByID(userID)
	if err != nil {
		return nil, err
	}

	uploadResult, err := media.UploadUserAvatar(ctx, userID, fileHeader)
	if err != nil {
		return nil, err
	}

	oldObjectKey := trimStringPtr(currentUser.AvatarObjectKey)
	newURL := uploadResult.URL
	newObjectKey := uploadResult.ObjectKey
	if err := database.DB.Model(&models.User{}).
		Where("id = ?", userID).
		Updates(map[string]any{
			"avatar_url":        newURL,
			"avatar_object_key": newObjectKey,
		}).Error; err != nil {
		_ = media.DeleteStoredObject(ctx, newObjectKey)
		return nil, err
	}

	if oldObjectKey != "" && oldObjectKey != newObjectKey {
		if err := media.DeleteStoredObject(ctx, oldObjectKey); err != nil {
			log.Printf("[user.avatar] cleanup old uploaded avatar failed user=%s objectKey=%q err=%v", userID, oldObjectKey, err)
		}
	}

	return GetUserByID(userID)
}

func UpdateAvatarWithGitHub(ctx context.Context, userID uuid.UUID, username string) (*models.User, error) {
	currentUser, err := GetUserByID(userID)
	if err != nil {
		return nil, err
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return nil, ErrGitHubUsernameRequired
	}
	if !githubUsernamePattern.MatchString(username) {
		return nil, ErrGitHubUsernameInvalid
	}

	avatarURL := fmt.Sprintf("https://github.com/%s.png", username)
	oldObjectKey := trimStringPtr(currentUser.AvatarObjectKey)
	if err := database.DB.Model(&models.User{}).
		Where("id = ?", userID).
		Updates(map[string]any{
			"avatar_url":        avatarURL,
			"avatar_object_key": nil,
		}).Error; err != nil {
		return nil, err
	}

	if oldObjectKey != "" {
		if err := media.DeleteStoredObject(ctx, oldObjectKey); err != nil {
			log.Printf("[user.avatar] cleanup replaced uploaded avatar failed user=%s objectKey=%q err=%v", userID, oldObjectKey, err)
		}
	}

	return GetUserByID(userID)
}

func trimStringPtr(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func nullableTrimmedString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

func stringPtrOrNil(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
