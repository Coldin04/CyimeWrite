package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/auth"
	"github.com/gofiber/fiber/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const testJWTSecret = "test-secret-please-rotate-aaaaaaaaaaaaaaaa"

func signTestJWT(t *testing.T, userID uuid.UUID) string {
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
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("failed to sign test JWT: %v", err)
	}
	return token
}

func newProtectedTestApp() *fiber.App {
	app := fiber.New()
	app.Get("/api/v1/media/assets", Protected(), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})
	app.Delete("/api/v1/media/assets/:id", Protected(), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})
	app.Get("/api/v1/media/assets/:id/content", ProtectedMediaContent(), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})
	app.Get("/api/v1/media/assets/:id/thumbnail", ProtectedMediaContent(), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	})
	return app
}

func TestProtectedRejectsMediaCookieOnGenericMediaRoutes(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", testJWTSecret)
	app := newProtectedTestApp()
	token := signTestJWT(t, uuid.New())

	tests := []struct {
		name   string
		method string
		target string
	}{
		{name: "list assets", method: http.MethodGet, target: "/api/v1/media/assets"},
		{name: "delete asset", method: http.MethodDelete, target: "/api/v1/media/assets/" + uuid.NewString()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, nil)
			req.AddCookie(&http.Cookie{Name: "cyime_media_access_token", Value: token, Path: "/api/v1/media"})

			resp, err := app.Test(req, -1)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			if resp.StatusCode != http.StatusUnauthorized {
				t.Fatalf("expected media-cookie-only request to be rejected with 401, got %d", resp.StatusCode)
			}
		})
	}
}

func TestProtectedMediaContentAcceptsMediaCookieOnlyForReadContentRoutes(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", testJWTSecret)
	app := newProtectedTestApp()
	token := signTestJWT(t, uuid.New())

	tests := []struct {
		name   string
		target string
	}{
		{name: "content", target: "/api/v1/media/assets/" + uuid.NewString() + "/content"},
		{name: "thumbnail", target: "/api/v1/media/assets/" + uuid.NewString() + "/thumbnail"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			req.AddCookie(&http.Cookie{Name: "cyime_media_access_token", Value: token, Path: "/api/v1/media"})

			resp, err := app.Test(req, -1)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			if resp.StatusCode != http.StatusNoContent {
				t.Fatalf("expected media cookie to be accepted on read content route, got %d", resp.StatusCode)
			}
		})
	}
}

func TestProtectedStillAcceptsAuthorizationBearer(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", testJWTSecret)
	app := newProtectedTestApp()
	token := signTestJWT(t, uuid.New())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/assets", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected bearer token to be accepted, got %d", resp.StatusCode)
	}
}
