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

func newSkillOAuthTestApp(protectedUserID uuid.UUID) *fiber.App {
	app := fiber.New()
	app.Get("/api/v1/auth/skill/oauth/authorize", SkillOAuthAuthorize)
	protected := func(c *fiber.Ctx) error {
		if protectedUserID == uuid.Nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "test user required"})
		}
		c.Locals("userId", protectedUserID.String())
		return c.Next()
	}
	app.Get("/api/v1/auth/skill/oauth/requests/:id", protected, SkillOAuthGetRequest)
	app.Post("/api/v1/auth/skill/oauth/requests/:id/approve", protected, SkillOAuthApproveRequest)
	app.Post("/api/v1/auth/skill/oauth/requests/:id/deny", protected, SkillOAuthDenyRequest)
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

	t.Setenv("PUBLIC_BASE_URL", "https://cyime.example")

	app := newSkillOAuthTestApp(userID)
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
	if location.Scheme != "https" || location.Host != "cyime.example" || location.Path != "/auth/skill/consent" {
		t.Fatalf("unexpected consent redirect location: %s", location.String())
	}
	requestID := location.Query().Get("request_id")
	if requestID == "" {
		t.Fatal("consent redirect did not include request_id")
	}
	var codeCount int64
	if err := db.Model(&models.SkillOAuthCode{}).Count(&codeCount).Error; err != nil {
		t.Fatalf("count authorization codes: %v", err)
	}
	if codeCount != 0 {
		t.Fatalf("authorization code should not exist before user consent, got %d", codeCount)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/skill/oauth/requests/"+requestID, nil)
	detailResp, err := app.Test(detailReq, -1)
	if err != nil {
		t.Fatalf("request detail failed: %v", err)
	}
	if detailResp.StatusCode != http.StatusOK {
		t.Fatalf("request detail status = %d, want 200", detailResp.StatusCode)
	}
	var detail skillOAuthRequestResponse
	if err := json.NewDecoder(detailResp.Body).Decode(&detail); err != nil {
		t.Fatalf("decode request detail: %v", err)
	}
	if detail.ClientID != "lobe-skill" || detail.RedirectURI != redirectURI {
		t.Fatalf("unexpected request detail: %+v", detail)
	}
	if !apitoken.HasScopes(detail.Scopes, apitoken.ScopeWorkspaceRead, apitoken.ScopeDocumentRead) {
		t.Fatalf("unexpected request scopes: %#v", detail.Scopes)
	}

	approveReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/skill/oauth/requests/"+requestID+"/approve", nil)
	approveResp, err := app.Test(approveReq, -1)
	if err != nil {
		t.Fatalf("approve request failed: %v", err)
	}
	if approveResp.StatusCode != http.StatusOK {
		t.Fatalf("approve status = %d, want 200", approveResp.StatusCode)
	}
	var approvePayload struct {
		RedirectURL string `json:"redirectUrl"`
	}
	if err := json.NewDecoder(approveResp.Body).Decode(&approvePayload); err != nil {
		t.Fatalf("decode approve response: %v", err)
	}

	callbackURL, err := url.Parse(approvePayload.RedirectURL)
	if err != nil {
		t.Fatalf("parse approve redirect URL: %v", err)
	}
	if callbackURL.Scheme != "http" || callbackURL.Host != "127.0.0.1:4173" {
		t.Fatalf("unexpected approve redirect location: %s", callbackURL.String())
	}
	code := callbackURL.Query().Get("code")
	if code == "" {
		t.Fatal("approve redirect did not include code")
	}
	if callbackURL.Query().Get("state") != "opaque-state" {
		t.Fatalf("state = %q, want opaque-state", callbackURL.Query().Get("state"))
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

	app := newSkillOAuthTestApp(uuid.Nil)
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

func TestSkillOAuthDenyReturnsAccessDeniedRedirect(t *testing.T) {
	db := setupAuthTestDB(t)
	t.Setenv("PUBLIC_BASE_URL", "https://cyime.example")
	userID := uuid.New()
	if err := db.Create(&models.User{ID: userID}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	seedSessionWithToken(t, db, userID, "Mozilla/5.0 Chrome", time.Now(), "refresh-token")

	app := newSkillOAuthTestApp(userID)
	redirectURI := "http://127.0.0.1:4173/oauth/callback"
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", "deny-test")
	params.Set("redirect_uri", redirectURI)
	params.Set("state", "deny-state")

	authorizeReq := httptest.NewRequest(http.MethodGet, "/api/v1/auth/skill/oauth/authorize?"+params.Encode(), nil)
	authorizeReq.AddCookie(&http.Cookie{Name: "cyime_refresh_token", Value: "refresh-token"})
	authorizeResp, err := app.Test(authorizeReq, -1)
	if err != nil {
		t.Fatalf("authorize request failed: %v", err)
	}
	location, err := url.Parse(authorizeResp.Header.Get("Location"))
	if err != nil {
		t.Fatalf("parse consent location: %v", err)
	}
	requestID := location.Query().Get("request_id")
	if requestID == "" {
		t.Fatal("consent redirect did not include request_id")
	}

	denyReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/skill/oauth/requests/"+requestID+"/deny", nil)
	denyResp, err := app.Test(denyReq, -1)
	if err != nil {
		t.Fatalf("deny request failed: %v", err)
	}
	if denyResp.StatusCode != http.StatusOK {
		t.Fatalf("deny status = %d, want 200", denyResp.StatusCode)
	}
	var payload struct {
		RedirectURL string `json:"redirectUrl"`
	}
	if err := json.NewDecoder(denyResp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode deny response: %v", err)
	}
	callbackURL, err := url.Parse(payload.RedirectURL)
	if err != nil {
		t.Fatalf("parse deny redirect URL: %v", err)
	}
	if callbackURL.Query().Get("error") != "access_denied" {
		t.Fatalf("error = %q, want access_denied", callbackURL.Query().Get("error"))
	}
	if callbackURL.Query().Get("state") != "deny-state" {
		t.Fatalf("state = %q, want deny-state", callbackURL.Query().Get("state"))
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
