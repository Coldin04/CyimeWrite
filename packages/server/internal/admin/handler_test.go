package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

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
	if err := db.AutoMigrate(&models.User{}, &models.Document{}); err != nil {
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
