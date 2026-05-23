package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/apitoken"
	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func newSkillOAuthTestApp() *fiber.App {
	app := fiber.New()
	app.Get("/api/v1/auth/skill/oauth/authorize", SkillOAuthAuthorize)
	app.Post("/api/v1/auth/skill/oauth/token", SkillOAuthToken)
	return app
}

func TestSkillOAuthFlowIssuesAPIToken(t *testing.T) {
	db := setupAuthTestDB(t)
	userID := uuid.New()
	if err := db.Create(&models.User{ID: userID}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	seedSessionWithToken(t, db, userID, "Mozilla/5.0 Chrome", time.Now(), "refresh-token")

	app := newSkillOAuthTestApp()
	redirectURI := "http://127.0.0.1:4173/oauth/callback"
	codeVerifier := "correct-horse-battery-staple"
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", "lobe-skill")
	params.Set("redirect_uri", redirectURI)
	params.Set("state", "opaque-state")
	params.Set("scope", "workspace:read document:read")
	params.Set("code_challenge", pkceS256Challenge(codeVerifier))
	params.Set("code_challenge_method", "S256")

	authorizeReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/skill/oauth/authorize?"+params.Encode(), nil)
	authorizeReq.AddCookie(&http.Cookie{Name: "cyime_refresh_token", Value: "refresh-token"})
	authorizeResp, err := app.Test(authorizeReq, -1)
	if err != nil {
		t.Fatalf("authorize request failed: %v", err)
	}
	if authorizeResp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("authorize status = %d, want %d", authorizeResp.StatusCode, http.StatusTemporaryRedirect)
	}

	location, err := url.Parse(authorizeResp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse authorize location: %v", err)
	}
	if location.Scheme != "http" || location.Host != "127.0.0.1:4173" {
		t.Fatalf("unexpected redirect location: %s", location.String())
	}
	code := location.Query().Get("code")
	if code == "" {
		t.Fatal("authorization redirect did not include code")
	}
	if location.Query().Get("state") != "opaque-state" {
		t.Fatalf("state = %q, want opaque-state", location.Query().Get("state"))
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("code_verifier", codeVerifier)
	form.Set("client_id", "lobe-skill")
	tokenReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/skill/oauth/token", strings.NewReader(form.Encode()))
	tokenReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	tokenResp, err := app.Test(tokenReq, -1)
	if err != nil {
		t.Fatalf("token request failed: %v", err)
	}
	if tokenResp.StatusCode != http.StatusOK {
		t.Fatalf("token status = %d, want 200", tokenResp.StatusCode)
	}

	var tokenPayload struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		Scope       string `json:"scope"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(tokenResp.Body).Decode(&tokenPayload); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if !strings.HasPrefix(tokenPayload.AccessToken, "cyime_sk_") {
		t.Fatalf("access_token prefix = %q", tokenPayload.AccessToken)
	}
	if tokenPayload.TokenType != "Bearer" {
		t.Fatalf("token_type = %q, want Bearer", tokenPayload.TokenType)
	}
	if tokenPayload.ExpiresIn <= 0 {
		t.Fatalf("expires_in = %d, want positive", tokenPayload.ExpiresIn)
	}

	authenticated, err := apitoken.Authenticate(tokenPayload.AccessToken, "127.0.0.1")
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if authenticated.UserID != userID {
		t.Fatalf("authenticated user = %s, want %s", authenticated.UserID, userID)
	}
	if !apitoken.HasScopes(authenticated.Scopes, apitoken.ScopeWorkspaceRead, apitoken.ScopeDocumentRead) {
		t.Fatalf("unexpected token scopes: %#v", authenticated.Scopes)
	}
	if apitoken.HasScopes(authenticated.Scopes, apitoken.ScopeWorkspaceWrite) {
		t.Fatalf("token unexpectedly has workspace write scope: %#v", authenticated.Scopes)
	}

	reuseReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/skill/oauth/token", strings.NewReader(form.Encode()))
	reuseReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	reuseResp, err := app.Test(reuseReq, -1)
	if err != nil {
		t.Fatalf("reuse token request failed: %v", err)
	}
	if reuseResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("reuse status = %d, want %d", reuseResp.StatusCode, http.StatusBadRequest)
	}

	var stored models.ApiToken
	if err := database.DB.First(&stored, "token_hash = ?", hashAPITokenForTest(tokenPayload.AccessToken)).Error; err != nil {
		t.Fatalf("load stored API token: %v", err)
	}
	if stored.ExpiresAt == nil {
		t.Fatal("OAuth-issued API token should have an expiration")
	}
}

func TestSkillOAuthAuthorizeRedirectsAnonymousUserToLogin(t *testing.T) {
	setupAuthTestDB(t)
	t.Setenv("PUBLIC_BASE_URL", "https://cyime.example")

	app := newSkillOAuthTestApp()
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", "lobe-skill")
	params.Set("redirect_uri", "http://127.0.0.1:4173/oauth/callback")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/skill/oauth/authorize?"+params.Encode(), nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("authorize request failed: %v", err)
	}
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusTemporaryRedirect)
	}

	location, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse location: %v", err)
	}
	if location.Scheme != "https" || location.Host != "cyime.example" || location.Path != "/login" {
		t.Fatalf("unexpected login redirect: %s", location.String())
	}
	returnTo := location.Query().Get("return_to")
	if !strings.HasPrefix(returnTo, "http://localhost:8080/api/v1/auth/skill/oauth/authorize?") {
		t.Fatalf("unexpected return_to: %s", returnTo)
	}
}

func pkceS256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func hashAPITokenForTest(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
