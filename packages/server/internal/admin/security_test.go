package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/auth"
	"g.co1d.in/Coldin04/Cyime/server/internal/middleware"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const adminSecurityTestJWTSecret = "admin-security-test-secret-aaaaaaaaaaaa"

func signAdminSecurityTestJWT(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	claims := auth.JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "Cyime",
			Subject:   userID.String(),
		},
		UserID: userID,
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(adminSecurityTestJWTSecret))
	if err != nil {
		t.Fatalf("failed to sign test JWT: %v", err)
	}
	return token
}

func newSecuredAdminTestApp() *fiber.App {
	app := fiber.New()
	group := app.Group("/api/v1/admin", middleware.Protected(), middleware.RequireAdmin())
	group.Get("/overview", GetOverviewHandler)
	group.Get("/users", ListUsersHandler)
	group.Get("/users/:id", GetUserHandler)
	group.Get("/users/:id/sessions", ListUserSessionsHandler)
	group.Delete("/users/:id/sessions/:sessionId", RevokeUserSessionHandler)
	group.Get("/users/:id/media", ListUserMediaHandler)
	group.Put("/users/:id/email", UpdateUserEmailHandler)
	group.Post("/users/:id/verify-email", VerifyUserEmailHandler)
	group.Put("/users/:id/document-quota", UpdateUserDocumentQuotaHandler)
	return app
}

func TestAdminRoutesRejectUnauthorizedAndNonAdminUsers(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", adminSecurityTestJWTSecret)
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

	nonAdminUser := seedAdminTestUser(t, db, "member@example.com", models.DocumentQuotaModeInherit, nil)
	targetUser := seedAdminTestUser(t, db, "target@example.com", models.DocumentQuotaModeInherit, nil)
	targetSession := models.UserSession{
		ID:          uuid.New(),
		UserID:      targetUser.ID,
		UserAgent:   "test-agent",
		DeviceLabel: "Chrome · Linux",
		LastSeenAt:  time.Now(),
	}
	if err := db.Create(&targetSession).Error; err != nil {
		t.Fatalf("create target session: %v", err)
	}
	targetToken := models.UserRefreshToken{
		ID:        uuid.New(),
		UserID:    targetUser.ID,
		SessionID: targetSession.ID,
		TokenHash: "security-test-token",
		ExpiresAt: time.Now().Add(time.Hour),
	}
	if err := db.Create(&targetToken).Error; err != nil {
		t.Fatalf("create target token: %v", err)
	}

	app := newSecuredAdminTestApp()

	tests := []struct {
		name          string
		method        string
		target        string
		body          string
		allowedStatus int
	}{
		{name: "overview", method: http.MethodGet, target: "/api/v1/admin/overview"},
		{name: "list users", method: http.MethodGet, target: "/api/v1/admin/users"},
		{name: "get user", method: http.MethodGet, target: "/api/v1/admin/users/" + targetUser.ID.String()},
		{name: "get sessions", method: http.MethodGet, target: "/api/v1/admin/users/" + targetUser.ID.String() + "/sessions"},
		{
			name:          "revoke session",
			method:        http.MethodDelete,
			target:        "/api/v1/admin/users/" + targetUser.ID.String() + "/sessions/" + targetSession.ID.String(),
			allowedStatus: http.StatusNoContent,
		},
		{name: "get media", method: http.MethodGet, target: "/api/v1/admin/users/" + targetUser.ID.String() + "/media"},
		{
			name:   "update email",
			method: http.MethodPut,
			target: "/api/v1/admin/users/" + targetUser.ID.String() + "/email",
			body:   `{"email":"verified@example.com"}`,
		},
		{
			name:   "verify email",
			method: http.MethodPost,
			target: "/api/v1/admin/users/" + targetUser.ID.String() + "/verify-email",
		},
		{
			name:   "update user quota",
			method: http.MethodPut,
			target: "/api/v1/admin/users/" + targetUser.ID.String() + "/document-quota",
			body:   `{"documentQuotaMode":"inherit","documentQuota":null}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+" unauthorized", func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := app.Test(req, -1)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("expected 401, got %d", resp.StatusCode)
			}
		})

		t.Run(tt.name+" forbidden", func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer "+signAdminSecurityTestJWT(t, nonAdminUser.ID))
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := app.Test(req, -1)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("expected 403, got %d", resp.StatusCode)
			}
		})

		t.Run(tt.name+" allowed", func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			req.Header.Set("Authorization", "Bearer "+signAdminSecurityTestJWT(t, adminUser.ID))
			if tt.body != "" {
				req.Header.Set("Content-Type", "application/json")
			}
			resp, err := app.Test(req, -1)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			expectedStatus := tt.allowedStatus
			if expectedStatus == 0 {
				expectedStatus = http.StatusOK
			}
			if resp.StatusCode != expectedStatus {
				t.Fatalf("expected %d, got %d", expectedStatus, resp.StatusCode)
			}
		})
	}
}
