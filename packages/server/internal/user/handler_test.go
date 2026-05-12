package user

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/media"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"g.co1d.in/Coldin04/Cyime/server/internal/securevalue"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupUserTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	t.Setenv("APP_ENCRYPTION_KEY", "f3a4d6e7c1b2a8d9e0f1a2b3c4d5e6f70a1b2c3d")
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.UserImageBedConfig{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	database.DB = db
	return db
}

func seedUser(t *testing.T, db *gorm.DB) models.User {
	t.Helper()
	name := "Coldin"
	email := "coldin@example.com"
	user := models.User{
		ID:          uuid.New(),
		Email:       &email,
		DisplayName: &name,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func newUserTestApp(userID uuid.UUID) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("userId", userID.String())
		return c.Next()
	})
	app.Get("/user/me", GetMe)
	app.Get("/user/image-beds/providers", ListImageBedProvidersHandler)
	app.Get("/user/image-beds", ListImageBedConfigsHandler)
	app.Post("/user/image-beds", CreateImageBedConfigHandler)
	app.Put("/user/image-beds/:id", UpdateImageBedConfigHandler)
	app.Delete("/user/image-beds/:id", DeleteImageBedConfigHandler)
	app.Put("/user/profile", UpdateProfileHandler)
	app.Post("/user/avatar", UploadAvatarHandler)
	app.Put("/user/avatar/github", UpdateGitHubAvatarHandler)
	app.Get("/api/v1/user/avatar/content", GetAvatarContentHandler)
	return app
}

func multipartAvatarRequest(t *testing.T, method string, path string, fieldName string, filename string, contentType string, data []byte) *http.Request {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if contentType != "" {
		req.Header.Set("X-Test-Content-Type", contentType)
	}
	return req
}

func TestUpdateProfileHandler_UpdatesDisplayName(t *testing.T) {
	db := setupUserTestDB(t)
	user := seedUser(t, db)

	app := newUserTestApp(user.ID)
	req := httptest.NewRequest(http.MethodPut, "/user/profile", strings.NewReader(`{"displayName":"  New Name  "}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload UserResponseDTO
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.DisplayName == nil || *payload.DisplayName != "New Name" {
		t.Fatalf("unexpected displayName: %+v", payload.DisplayName)
	}
}

func TestCreateImageBedConfigHandler_StoresConfig(t *testing.T) {
	db := setupUserTestDB(t)
	user := seedUser(t, db)

	app := newUserTestApp(user.ID)
	req := httptest.NewRequest(
		http.MethodPost,
		"/user/image-beds",
		strings.NewReader(`{"name":"Blog","providerType":"lsky","baseUrl":"https://img.example.com","apiToken":"lsky-token","isEnabled":true,"storageId":3,"strategyId":"posters"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var payload ImageBedConfigDTO
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Name != "Blog" || payload.ProviderType != "lsky" || payload.BaseURL != "https://img.example.com" || payload.APIToken != "" || !payload.HasAPIToken || payload.StorageID != 3 || payload.StrategyID != "posters" {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	var stored models.UserImageBedConfig
	if err := db.First(&stored, "id = ?", payload.ID).Error; err != nil {
		t.Fatalf("load stored config: %v", err)
	}
	if stored.APIToken == nil || *stored.APIToken == "lsky-token" {
		t.Fatalf("expected encrypted token in database")
	}
}

func TestCreateImageBedConfigHandler_StoresImgBBConfig(t *testing.T) {
	db := setupUserTestDB(t)
	user := seedUser(t, db)

	app := newUserTestApp(user.ID)
	req := httptest.NewRequest(
		http.MethodPost,
		"/user/image-beds",
		strings.NewReader(`{"name":"ImgBB","providerType":"imgbb","apiToken":"imgbb-key","isEnabled":true}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var payload ImageBedConfigDTO
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Name != "ImgBB" || payload.ProviderType != "imgbb" || payload.APIToken != "" || !payload.HasAPIToken {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestListImageBedConfigsHandler_DoesNotReturnToken(t *testing.T) {
	db := setupUserTestDB(t)
	user := seedUser(t, db)
	encryptedToken, err := securevalue.EncryptString("secret-token")
	if err != nil {
		t.Fatalf("encrypt token: %v", err)
	}
	configJSON := `{"fields":{"storageId":"3"}}`
	if err := db.Create(&models.UserImageBedConfig{
		ID:           uuid.New(),
		UserID:       user.ID,
		Name:         "Encrypted",
		ProviderType: "imgbb",
		APIToken:     &encryptedToken,
		ConfigJSON:   &configJSON,
		IsEnabled:    true,
	}).Error; err != nil {
		t.Fatalf("create image bed config: %v", err)
	}

	app := newUserTestApp(user.ID)
	req := httptest.NewRequest(http.MethodGet, "/user/image-beds", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload struct {
		Items []ImageBedConfigDTO `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Items) != 1 || payload.Items[0].APIToken != "" || !payload.Items[0].HasAPIToken {
		t.Fatalf("unexpected payload: %+v", payload.Items)
	}
}

func TestListImageBedProvidersHandler_ReturnsBuiltins(t *testing.T) {
	db := setupUserTestDB(t)
	user := seedUser(t, db)

	app := newUserTestApp(user.ID)
	req := httptest.NewRequest(http.MethodGet, "/user/image-beds/providers", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload struct {
		Items []ImageBedProviderDTO `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Items) < 2 {
		t.Fatalf("expected built-in providers, got %+v", payload.Items)
	}
}

func TestUploadAvatarHandler_StoresAvatarAndUpdatesUser(t *testing.T) {
	db := setupUserTestDB(t)
	user := seedUser(t, db)

	rootDir := t.TempDir()
	t.Setenv("MEDIA_STORAGE_PROVIDER", "local")
	t.Setenv("MEDIA_LOCAL_ROOT_DIR", rootDir)
	t.Setenv("MEDIA_LOCAL_BASE_URL", "/media-files")
	t.Setenv("JWT_SECRET_KEY", "test-secret")
	media.ResetStorageProviderForTesting()

	app := newUserTestApp(user.ID)
	req := multipartAvatarRequest(t, http.MethodPost, "/user/avatar", "file", "avatar.png", "image/png", []byte("fake-png"))
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload UserResponseDTO
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.AvatarURL == nil || !strings.HasSuffix(*payload.AvatarURL, "/api/v1/user/avatar/content") {
		t.Fatalf("unexpected avatarUrl: %+v", payload.AvatarURL)
	}
	if strings.Contains(*payload.AvatarURL, "token=") {
		t.Fatalf("avatarUrl must not expose bearer tokens: %q", *payload.AvatarURL)
	}

	var updated models.User
	if err := db.First(&updated, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	if updated.AvatarObjectKey == nil || strings.TrimSpace(*updated.AvatarObjectKey) == "" {
		t.Fatalf("expected avatar object key persisted")
	}
	storedPath := filepath.Join(rootDir, filepath.FromSlash(*updated.AvatarObjectKey))
	if _, err := os.Stat(storedPath); err != nil {
		t.Fatalf("expected avatar file stored, stat err=%v", err)
	}
}

func TestUploadAvatarHandler_RejectsTooLargeAvatar(t *testing.T) {
	db := setupUserTestDB(t)
	user := seedUser(t, db)

	rootDir := t.TempDir()
	t.Setenv("MEDIA_STORAGE_PROVIDER", "local")
	t.Setenv("MEDIA_LOCAL_ROOT_DIR", rootDir)
	t.Setenv("MEDIA_LOCAL_BASE_URL", "/media-files")
	t.Setenv("MEDIA_AVATAR_MAX_BYTES", "4")
	t.Setenv("JWT_SECRET_KEY", "test-secret")
	media.ResetStorageProviderForTesting()

	app := newUserTestApp(user.ID)
	req := multipartAvatarRequest(t, http.MethodPost, "/user/avatar", "file", "avatar.png", "image/png", []byte("12345678"))
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	var payload map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.Contains(payload["error"], "avatar file too large") {
		t.Fatalf("unexpected error payload: %+v", payload)
	}
}

func TestUpdateGitHubAvatarHandler_ClearsStoredObjectKeyAndDeletesOldUpload(t *testing.T) {
	db := setupUserTestDB(t)
	user := seedUser(t, db)

	rootDir := t.TempDir()
	t.Setenv("MEDIA_STORAGE_PROVIDER", "local")
	t.Setenv("MEDIA_LOCAL_ROOT_DIR", rootDir)
	t.Setenv("MEDIA_LOCAL_BASE_URL", "/media-files")
	t.Setenv("JWT_SECRET_KEY", "test-secret")
	media.ResetStorageProviderForTesting()

	oldObjectKey := "avatars/old.png"
	oldPath := filepath.Join(rootDir, filepath.FromSlash(oldObjectKey))
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o755); err != nil {
		t.Fatalf("mkdir old avatar dir: %v", err)
	}
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old avatar: %v", err)
	}
	oldURL := "/media-files/" + oldObjectKey
	if err := db.Model(&models.User{}).
		Where("id = ?", user.ID).
		Updates(map[string]any{
			"avatar_url":        oldURL,
			"avatar_object_key": oldObjectKey,
		}).Error; err != nil {
		t.Fatalf("seed old avatar: %v", err)
	}

	app := newUserTestApp(user.ID)
	req := httptest.NewRequest(http.MethodPut, "/user/avatar/github", strings.NewReader(`{"username":"octocat"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload UserResponseDTO
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.AvatarURL == nil || *payload.AvatarURL != "https://github.com/octocat.png" {
		t.Fatalf("unexpected github avatar url: %+v", payload.AvatarURL)
	}

	var updated models.User
	if err := db.First(&updated, "id = ?", user.ID).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	if updated.AvatarObjectKey != nil {
		t.Fatalf("expected avatar object key cleared, got %q", *updated.AvatarObjectKey)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected old uploaded avatar deleted, stat err=%v", err)
	}
}

func TestGetAvatarContentHandler_RejectsQueryTokenWithoutAuthenticatedContext(t *testing.T) {
	db := setupUserTestDB(t)
	user := seedUser(t, db)

	t.Setenv("JWT_SECRET_KEY", "test-secret")
	objectKey := user.ID.String() + "/avatars/avatar.png"
	urlValue := "/media-files/" + objectKey
	if err := db.Model(&models.User{}).
		Where("id = ?", user.ID).
		Updates(map[string]any{
			"avatar_url":        urlValue,
			"avatar_object_key": objectKey,
		}).Error; err != nil {
		t.Fatalf("seed avatar object: %v", err)
	}

	tokenService, err := media.NewTokenService()
	if err != nil {
		t.Fatalf("new token service: %v", err)
	}
	token, _, err := tokenService.IssueAvatarReadToken(user.ID, objectKey)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	app := fiber.New()
	app.Get("/api/v1/user/avatar/content", GetAvatarContentHandler)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user/avatar/content?token="+token, nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestGetAvatarContentHandler_ReturnsCurrentUsersUploadedAvatar(t *testing.T) {
	db := setupUserTestDB(t)
	user := seedUser(t, db)

	rootDir := t.TempDir()
	t.Setenv("MEDIA_STORAGE_PROVIDER", "local")
	t.Setenv("MEDIA_LOCAL_ROOT_DIR", rootDir)
	t.Setenv("MEDIA_LOCAL_BASE_URL", "/media-files")
	t.Setenv("JWT_SECRET_KEY", "test-secret")
	media.ResetStorageProviderForTesting()

	objectKey := user.ID.String() + "/avatars/avatar.png"
	filePath := filepath.Join(rootDir, filepath.FromSlash(objectKey))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir avatar dir: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("avatar-bytes"), 0o644); err != nil {
		t.Fatalf("write avatar: %v", err)
	}
	urlValue := "/media-files/" + objectKey
	if err := db.Model(&models.User{}).
		Where("id = ?", user.ID).
		Updates(map[string]any{
			"avatar_url":        urlValue,
			"avatar_object_key": objectKey,
		}).Error; err != nil {
		t.Fatalf("seed avatar object: %v", err)
	}

	app := newUserTestApp(user.ID)
	meReq := httptest.NewRequest(http.MethodGet, "/user/me", nil)
	meResp, err := app.Test(meReq, -1)
	if err != nil {
		t.Fatalf("get me failed: %v", err)
	}
	var mePayload UserResponseDTO
	if err := json.NewDecoder(meResp.Body).Decode(&mePayload); err != nil {
		t.Fatalf("decode me payload: %v", err)
	}
	if mePayload.AvatarURL == nil {
		t.Fatalf("expected resolved avatar url")
	}

	if strings.Contains(*mePayload.AvatarURL, "token=") {
		t.Fatalf("avatarUrl must not expose bearer tokens: %q", *mePayload.AvatarURL)
	}

	contentReq := httptest.NewRequest(http.MethodGet, *mePayload.AvatarURL, nil)
	contentResp, err := app.Test(contentReq, -1)
	if err != nil {
		t.Fatalf("get avatar content failed: %v", err)
	}
	if contentResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", contentResp.StatusCode)
	}
	body, err := io.ReadAll(contentResp.Body)
	if err != nil {
		t.Fatalf("read content body: %v", err)
	}
	if string(body) != "avatar-bytes" {
		t.Fatalf("unexpected avatar content: %q", string(body))
	}
}
