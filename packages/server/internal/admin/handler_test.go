package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAdminTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&models.Document{},
		&models.UserSession{},
		&models.UserRefreshToken{},
		&models.BlobObject{},
		&models.Asset{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	database.DB = db
	return db
}

func seedAdminTestUser(t *testing.T, db *gorm.DB, email string, quotaMode string, quota *int) models.User {
	t.Helper()
	name := strings.Split(email, "@")[0]
	user := models.User{
		ID:                uuid.New(),
		Email:             &email,
		DisplayName:       &name,
		DocumentQuotaMode: quotaMode,
		DocumentQuota:     quota,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func seedOwnedDocument(t *testing.T, db *gorm.DB, ownerID uuid.UUID, title string, deleted bool) {
	t.Helper()
	doc := models.Document{
		ID:          uuid.New(),
		OwnerUserID: ownerID,
		Title:       title,
		CreatedBy:   ownerID,
		UpdatedBy:   ownerID,
	}
	if err := db.Create(&doc).Error; err != nil {
		t.Fatalf("create document: %v", err)
	}
	if deleted {
		if err := db.Delete(&models.Document{}, "id = ?", doc.ID).Error; err != nil {
			t.Fatalf("delete document: %v", err)
		}
	}
}

func newAdminTestApp(adminUserID uuid.UUID) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("userId", adminUserID.String())
		return c.Next()
	})
	app.Get("/admin/overview", GetOverviewHandler)
	app.Get("/admin/users", ListUsersHandler)
	app.Get("/admin/users/:id", GetUserHandler)
	app.Get("/admin/users/:id/sessions", ListUserSessionsHandler)
	app.Get("/admin/users/:id/media", ListUserMediaHandler)
	app.Delete("/admin/users/:id/sessions/:sessionId", RevokeUserSessionHandler)
	app.Put("/admin/users/:id/email", UpdateUserEmailHandler)
	app.Post("/admin/users/:id/verify-email", VerifyUserEmailHandler)
	app.Put("/admin/users/:id/document-quota", UpdateUserDocumentQuotaHandler)
	return app
}

func TestListUsersHandler_ReturnsPagedUsersAndCounts(t *testing.T) {
	db := setupAdminTestDB(t)
	t.Setenv("DEFAULT_DOCUMENT_QUOTA", "9")

	adminRole := models.AdminRoleAdmin
	adminEmail := "admin@local.dev"
	adminUser := models.User{
		ID:                uuid.New(),
		Email:             &adminEmail,
		AdminRole:         &adminRole,
		DocumentQuotaMode: models.DocumentQuotaModeInherit,
	}
	if err := db.Create(&adminUser).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	customQuota := 3
	firstUser := seedAdminTestUser(t, db, "alpha@example.com", models.DocumentQuotaModeCustom, &customQuota)
	secondUser := seedAdminTestUser(t, db, "bravo@example.com", models.DocumentQuotaModeInherit, nil)
	seedOwnedDocument(t, db, firstUser.ID, "active", false)
	seedOwnedDocument(t, db, firstUser.ID, "trashed", true)
	seedOwnedDocument(t, db, secondUser.ID, "active-2", false)

	app := newAdminTestApp(adminUser.ID)
	req := httptest.NewRequest(http.MethodGet, "/admin/users?limit=2&offset=0&q=example", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload UserListResponseDTO
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.GlobalDocumentQuota == nil || *payload.GlobalDocumentQuota != 9 {
		t.Fatalf("unexpected global quota: %+v", payload.GlobalDocumentQuota)
	}
	if len(payload.Items) != 2 {
		t.Fatalf("expected 2 users, got %d", len(payload.Items))
	}

	var foundCustom bool
	for _, item := range payload.Items {
		if item.ID != firstUser.ID {
			continue
		}
		foundCustom = true
		if item.DocumentQuotaMode != models.DocumentQuotaModeCustom {
			t.Fatalf("expected custom quota mode, got %q", item.DocumentQuotaMode)
		}
		if item.DocumentQuota == nil || *item.DocumentQuota != 3 {
			t.Fatalf("unexpected custom quota: %+v", item.DocumentQuota)
		}
		if item.EffectiveDocumentQuota == nil || *item.EffectiveDocumentQuota != 3 {
			t.Fatalf("unexpected effective quota: %+v", item.EffectiveDocumentQuota)
		}
		if item.ActiveDocumentCount != 1 || item.TrashedDocumentCount != 1 || item.UsedDocumentCount != 2 {
			t.Fatalf("unexpected document counts: %+v", item)
		}
	}
	if !foundCustom {
		t.Fatalf("expected custom user in payload: %+v", payload.Items)
	}
}

func TestListUsersHandler_DefaultPagination(t *testing.T) {
	db := setupAdminTestDB(t)

	adminRole := models.AdminRoleAdmin
	adminEmail := "admin@local.dev"
	adminUser := models.User{
		ID:                uuid.New(),
		Email:             &adminEmail,
		AdminRole:         &adminRole,
		DocumentQuotaMode: models.DocumentQuotaModeInherit,
	}
	if err := db.Create(&adminUser).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	for i := 1; i <= 26; i++ {
		email := "page-user-" + strconv.Itoa(i) + "@example.com"
		seedAdminTestUser(t, db, email, models.DocumentQuotaModeInherit, nil)
	}

	app := newAdminTestApp(adminUser.ID)

	firstReq := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	firstResp, err := app.Test(firstReq, -1)
	if err != nil {
		t.Fatalf("first page request failed: %v", err)
	}
	if firstResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", firstResp.StatusCode)
	}

	var firstPage UserListResponseDTO
	if err := json.NewDecoder(firstResp.Body).Decode(&firstPage); err != nil {
		t.Fatalf("decode first page: %v", err)
	}
	if len(firstPage.Items) != 20 || !firstPage.HasMore || firstPage.NextOffset != 20 {
		t.Fatalf("unexpected first page payload: %+v", firstPage)
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/admin/users?offset=20", nil)
	secondResp, err := app.Test(secondReq, -1)
	if err != nil {
		t.Fatalf("second page request failed: %v", err)
	}
	if secondResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", secondResp.StatusCode)
	}

	var secondPage UserListResponseDTO
	if err := json.NewDecoder(secondResp.Body).Decode(&secondPage); err != nil {
		t.Fatalf("decode second page: %v", err)
	}
	if len(secondPage.Items) != 7 || secondPage.HasMore || secondPage.NextOffset != 27 {
		t.Fatalf("unexpected second page payload: %+v", secondPage)
	}
}

func TestUpdateUserDocumentQuotaHandler_SetsUnlimitedMode(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRole := models.AdminRoleAdmin
	adminEmail := "admin@local.dev"
	adminUser := models.User{
		ID:        uuid.New(),
		Email:     &adminEmail,
		AdminRole: &adminRole,
	}
	if err := db.Create(&adminUser).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	quota := 2
	targetUser := seedAdminTestUser(t, db, "member@example.com", models.DocumentQuotaModeCustom, &quota)

	app := newAdminTestApp(adminUser.ID)
	req := httptest.NewRequest(
		http.MethodPut,
		"/admin/users/"+targetUser.ID.String()+"/document-quota",
		strings.NewReader(`{"documentQuotaMode":"unlimited","documentQuota":null}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload UserListItemDTO
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.DocumentQuotaMode != models.DocumentQuotaModeUnlimited {
		t.Fatalf("expected unlimited mode, got %q", payload.DocumentQuotaMode)
	}
	if payload.DocumentQuota != nil || payload.EffectiveDocumentQuota != nil || !payload.Unlimited {
		t.Fatalf("unexpected quota payload: %+v", payload)
	}

	var reloaded models.User
	if err := db.First(&reloaded, "id = ?", targetUser.ID).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if reloaded.DocumentQuotaMode != models.DocumentQuotaModeUnlimited || reloaded.DocumentQuota != nil {
		t.Fatalf("unexpected stored user: %+v", reloaded)
	}
}

func TestGetUserHandler_ReturnsUserDetail(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRole := models.AdminRoleAdmin
	adminEmail := "admin@local.dev"
	adminUser := models.User{
		ID:        uuid.New(),
		Email:     &adminEmail,
		AdminRole: &adminRole,
	}
	if err := db.Create(&adminUser).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	targetUser := seedAdminTestUser(t, db, "detail@example.com", models.DocumentQuotaModeInherit, nil)
	seedOwnedDocument(t, db, targetUser.ID, "detail-doc", false)

	app := newAdminTestApp(adminUser.ID)
	req := httptest.NewRequest(http.MethodGet, "/admin/users/"+targetUser.ID.String(), nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload UserListItemDTO
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ID != targetUser.ID || payload.ActiveDocumentCount != 1 {
		t.Fatalf("unexpected user detail payload: %+v", payload)
	}
}

func TestListUserMediaHandler_OmitsDocumentReferenceFields(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRole := models.AdminRoleAdmin
	adminEmail := "admin@local.dev"
	adminUser := models.User{
		ID:        uuid.New(),
		Email:     &adminEmail,
		AdminRole: &adminRole,
	}
	if err := db.Create(&adminUser).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	targetUser := seedAdminTestUser(t, db, "media-owner@example.com", models.DocumentQuotaModeInherit, nil)
	documentID := uuid.New()
	blob := models.BlobObject{
		ID:              uuid.New(),
		OwnerUserID:     targetUser.ID,
		SHA256:          strings.Repeat("a", 64),
		Size:            2048,
		MimeType:        "image/png",
		StorageProvider: "local",
		ObjectKey:       "media/blob.png",
		URL:             "https://example.com/blob.png",
		Status:          "ready",
		ThumbnailStatus: "ready",
	}
	if err := db.Create(&blob).Error; err != nil {
		t.Fatalf("create blob: %v", err)
	}

	asset := models.Asset{
		ID:             uuid.New(),
		OwnerUserID:    targetUser.ID,
		DocumentID:     &documentID,
		BlobID:         blob.ID,
		Kind:           "image",
		Filename:       "cover.png",
		URL:            "https://example.com/blob.png",
		Visibility:     "private",
		Status:         "ready",
		ReferenceCount: 1,
		CreatedBy:      targetUser.ID,
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}

	app := newAdminTestApp(adminUser.ID)
	req := httptest.NewRequest(http.MethodGet, "/admin/users/"+targetUser.ID.String()+"/media", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	items, ok := payload["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("unexpected items payload: %+v", payload["items"])
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected item payload: %+v", items[0])
	}
	if _, exists := first["documentId"]; exists {
		t.Fatalf("documentId should not be exposed in admin media payload: %+v", first)
	}
}

func TestUpdateUserEmailHandler_ResetsVerification(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRole := models.AdminRoleAdmin
	adminEmail := "admin@local.dev"
	adminUser := models.User{
		ID:        uuid.New(),
		Email:     &adminEmail,
		AdminRole: &adminRole,
	}
	if err := db.Create(&adminUser).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	targetEmail := "member@example.com"
	now := time.Now()
	targetUser := models.User{
		ID:              uuid.New(),
		Email:           &targetEmail,
		EmailVerified:   true,
		EmailVerifiedAt: &now,
	}
	if err := db.Create(&targetUser).Error; err != nil {
		t.Fatalf("create target user: %v", err)
	}

	app := newAdminTestApp(adminUser.ID)
	req := httptest.NewRequest(
		http.MethodPut,
		"/admin/users/"+targetUser.ID.String()+"/email",
		strings.NewReader(`{"email":"next@example.com"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload UserListItemDTO
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Email == nil || *payload.Email != "next@example.com" {
		t.Fatalf("unexpected email payload: %+v", payload.Email)
	}
	if payload.EmailVerified {
		t.Fatalf("expected updated email to be unverified")
	}
}

func TestVerifyUserEmailHandler_MarksUserVerified(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRole := models.AdminRoleAdmin
	adminEmail := "admin@local.dev"
	adminUser := models.User{
		ID:        uuid.New(),
		Email:     &adminEmail,
		AdminRole: &adminRole,
	}
	if err := db.Create(&adminUser).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	targetEmail := "member@example.com"
	targetUser := models.User{
		ID:            uuid.New(),
		Email:         &targetEmail,
		EmailVerified: false,
	}
	if err := db.Create(&targetUser).Error; err != nil {
		t.Fatalf("create target user: %v", err)
	}

	app := newAdminTestApp(adminUser.ID)
	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+targetUser.ID.String()+"/verify-email", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload UserListItemDTO
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.EmailVerified {
		t.Fatalf("expected email to be verified")
	}
}

func TestUpdateUserEmailHandler_RejectsDuplicateEmail(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRole := models.AdminRoleAdmin
	adminEmail := "admin@local.dev"
	adminUser := models.User{
		ID:        uuid.New(),
		Email:     &adminEmail,
		AdminRole: &adminRole,
	}
	if err := db.Create(&adminUser).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	occupiedEmail := "occupied@example.com"
	targetEmail := "member@example.com"
	occupiedUser := models.User{ID: uuid.New(), Email: &occupiedEmail}
	targetUser := models.User{ID: uuid.New(), Email: &targetEmail}
	if err := db.Create(&occupiedUser).Error; err != nil {
		t.Fatalf("create occupied user: %v", err)
	}
	if err := db.Create(&targetUser).Error; err != nil {
		t.Fatalf("create target user: %v", err)
	}

	app := newAdminTestApp(adminUser.ID)
	req := httptest.NewRequest(
		http.MethodPut,
		"/admin/users/"+targetUser.ID.String()+"/email",
		strings.NewReader(`{"email":"occupied@example.com"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestRevokeUserSessionHandler_RemovesRefreshTokens(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRole := models.AdminRoleAdmin
	adminEmail := "admin@local.dev"
	adminUser := models.User{
		ID:        uuid.New(),
		Email:     &adminEmail,
		AdminRole: &adminRole,
	}
	if err := db.Create(&adminUser).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	targetUser := seedAdminTestUser(t, db, "session-owner@example.com", models.DocumentQuotaModeInherit, nil)
	session := models.UserSession{
		ID:          uuid.New(),
		UserID:      targetUser.ID,
		UserAgent:   "test-agent",
		DeviceLabel: "Chrome · Linux",
		LastSeenAt:  time.Now(),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	token := models.UserRefreshToken{
		ID:        uuid.New(),
		UserID:    targetUser.ID,
		SessionID: session.ID,
		TokenHash: "refresh-token-hash",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := db.Create(&token).Error; err != nil {
		t.Fatalf("create refresh token: %v", err)
	}

	app := newAdminTestApp(adminUser.ID)
	req := httptest.NewRequest(
		http.MethodDelete,
		"/admin/users/"+targetUser.ID.String()+"/sessions/"+session.ID.String(),
		nil,
	)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}

	var reloaded models.UserSession
	if err := db.First(&reloaded, "id = ?", session.ID).Error; err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if reloaded.RevokedAt == nil {
		t.Fatalf("expected session to be revoked")
	}

	var tokenCount int64
	if err := db.Model(&models.UserRefreshToken{}).Where("session_id = ?", session.ID).Count(&tokenCount).Error; err != nil {
		t.Fatalf("count refresh tokens: %v", err)
	}
	if tokenCount != 0 {
		t.Fatalf("expected refresh tokens to be deleted, got %d", tokenCount)
	}
}

func TestListUserSessionsHandler_PaginatesActiveSessionsOnly(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRole := models.AdminRoleAdmin
	adminEmail := "admin@local.dev"
	adminUser := models.User{
		ID:        uuid.New(),
		Email:     &adminEmail,
		AdminRole: &adminRole,
	}
	if err := db.Create(&adminUser).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	targetUser := seedAdminTestUser(t, db, "session-page@example.com", models.DocumentQuotaModeInherit, nil)
	now := time.Now()
	for i := 0; i < 25; i++ {
		session := models.UserSession{
			ID:          uuid.New(),
			UserID:      targetUser.ID,
			UserAgent:   "test-agent",
			DeviceLabel: "Chrome · Linux",
			LastSeenAt:  now.Add(-time.Duration(i) * time.Minute),
		}
		if err := db.Create(&session).Error; err != nil {
			t.Fatalf("create active session %d: %v", i, err)
		}
		token := models.UserRefreshToken{
			ID:        uuid.New(),
			UserID:    targetUser.ID,
			SessionID: session.ID,
			TokenHash: "active-token-" + strconv.Itoa(i),
			ExpiresAt: now.Add(time.Hour),
		}
		if err := db.Create(&token).Error; err != nil {
			t.Fatalf("create active token %d: %v", i, err)
		}
		if i < 3 {
			extraToken := models.UserRefreshToken{
				ID:        uuid.New(),
				UserID:    targetUser.ID,
				SessionID: session.ID,
				TokenHash: "extra-active-token-" + strconv.Itoa(i),
				ExpiresAt: now.Add(2 * time.Hour),
			}
			if err := db.Create(&extraToken).Error; err != nil {
				t.Fatalf("create extra active token %d: %v", i, err)
			}
		}
	}

	expiredSession := models.UserSession{
		ID:          uuid.New(),
		UserID:      targetUser.ID,
		UserAgent:   "expired-agent",
		DeviceLabel: "Expired",
		LastSeenAt:  now.Add(time.Minute),
	}
	if err := db.Create(&expiredSession).Error; err != nil {
		t.Fatalf("create expired session: %v", err)
	}
	expiredToken := models.UserRefreshToken{
		ID:        uuid.New(),
		UserID:    targetUser.ID,
		SessionID: expiredSession.ID,
		TokenHash: "expired-token",
		ExpiresAt: now.Add(-time.Hour),
	}
	if err := db.Create(&expiredToken).Error; err != nil {
		t.Fatalf("create expired token: %v", err)
	}

	app := newAdminTestApp(adminUser.ID)

	firstReq := httptest.NewRequest(http.MethodGet, "/admin/users/"+targetUser.ID.String()+"/sessions?limit=10", nil)
	firstResp, err := app.Test(firstReq, -1)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	if firstResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", firstResp.StatusCode)
	}
	var firstPage AdminSessionListResponseDTO
	if err := json.NewDecoder(firstResp.Body).Decode(&firstPage); err != nil {
		t.Fatalf("decode first page: %v", err)
	}
	if len(firstPage.Items) != 10 || !firstPage.HasMore || firstPage.NextOffset != 10 || firstPage.Total != 25 {
		t.Fatalf("unexpected first page: %+v", firstPage)
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/admin/users/"+targetUser.ID.String()+"/sessions?limit=10&offset=10", nil)
	secondResp, err := app.Test(secondReq, -1)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	if secondResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", secondResp.StatusCode)
	}
	var secondPage AdminSessionListResponseDTO
	if err := json.NewDecoder(secondResp.Body).Decode(&secondPage); err != nil {
		t.Fatalf("decode second page: %v", err)
	}
	if len(secondPage.Items) != 10 || !secondPage.HasMore || secondPage.NextOffset != 20 || secondPage.Total != 25 {
		t.Fatalf("unexpected second page: %+v", secondPage)
	}
}

func TestRevokeUserSessionHandler_RejectsSelfTarget(t *testing.T) {
	db := setupAdminTestDB(t)
	adminRole := models.AdminRoleAdmin
	adminEmail := "admin@local.dev"
	adminUser := models.User{
		ID:        uuid.New(),
		Email:     &adminEmail,
		AdminRole: &adminRole,
	}
	if err := db.Create(&adminUser).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}

	session := models.UserSession{
		ID:          uuid.New(),
		UserID:      adminUser.ID,
		UserAgent:   "test-agent",
		DeviceLabel: "Chrome · Linux",
		LastSeenAt:  time.Now(),
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}

	app := newAdminTestApp(adminUser.ID)
	req := httptest.NewRequest(
		http.MethodDelete,
		"/admin/users/"+adminUser.ID.String()+"/sessions/"+session.ID.String(),
		nil,
	)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}

func TestGetOverviewHandler_ReturnsAdminAndGlobalQuota(t *testing.T) {
	db := setupAdminTestDB(t)
	t.Setenv("DEFAULT_DOCUMENT_QUOTA", "12")

	adminRole := models.AdminRoleAdmin
	adminEmail := "admin@local.dev"
	adminUser := models.User{
		ID:        uuid.New(),
		Email:     &adminEmail,
		AdminRole: &adminRole,
	}
	if err := db.Create(&adminUser).Error; err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	seedAdminTestUser(t, db, "member@example.com", models.DocumentQuotaModeInherit, nil)

	app := newAdminTestApp(adminUser.ID)
	req := httptest.NewRequest(http.MethodGet, "/admin/overview", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload OverviewResponseDTO
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.UserCount != 2 || payload.AdminCount != 1 {
		t.Fatalf("unexpected overview payload: %+v", payload)
	}
	if payload.GlobalDocumentQuota == nil || *payload.GlobalDocumentQuota != 12 || payload.GlobalUnlimited {
		t.Fatalf("unexpected global quota payload: %+v", payload)
	}
}
