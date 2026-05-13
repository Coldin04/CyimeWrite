package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"g.co1d.in/Coldin04/Cyime/server/internal/securevalue"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"golang.org/x/oauth2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func setupAuthTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// LoadJWTSecret enforces a minimum length and rejects known defaults; this
	// value is long enough and not on the blocklist so the auth handlers can
	// construct a TokenService without touching the operator-facing env.
	t.Setenv("JWT_SECRET_KEY", "test-secret-please-rotate-aaaaaaaaaaaaaaaa")
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.AuthProvider{}, &models.UserSession{}, &models.UserRefreshToken{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	database.DB = db
	tokenService = nil
	return db
}

func TestDecryptClientSecret_RejectsPlaintext(t *testing.T) {
	t.Setenv("APP_ENCRYPTION_KEY", "f3a4d6e7c1b2a8d9e0f1a2b3c4d5e6f70a1b2c3d")

	if _, err := decryptClientSecret("plaintext-secret"); err == nil {
		t.Fatal("expected plaintext client secret to be rejected")
	}
}

func TestDecryptClientSecret_DecryptsEncryptedValue(t *testing.T) {
	t.Setenv("APP_ENCRYPTION_KEY", "f3a4d6e7c1b2a8d9e0f1a2b3c4d5e6f70a1b2c3d")

	encrypted, err := securevalue.EncryptString("client-secret")
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	decrypted, err := decryptClientSecret(encrypted)
	if err != nil {
		t.Fatalf("decrypt secret: %v", err)
	}
	if decrypted != "client-secret" {
		t.Fatalf("decrypted secret = %q, want client-secret", decrypted)
	}
}

func TestGetAuthConfig_ReturnsDisplayNameWhenConfigured(t *testing.T) {
	db := setupAuthTestDB(t)
	displayName := "GitHub"
	iconURL := "https://github.com/fluidicon.png"
	provider := models.AuthProvider{
		ID:                    uuid.New(),
		Name:                  "github",
		DisplayName:           &displayName,
		ProtocolType:          "oauth2",
		ClientID:              "client-id",
		ClientSecretEncrypted: "secret",
		IconURL:               &iconURL,
		Scopes:                "read:user user:email",
		IsActive:              true,
	}
	if err := db.Create(&provider).Error; err != nil {
		t.Fatalf("create provider: %v", err)
	}

	app := fiber.New()
	app.Get("/api/v1/auth/config", GetAuthConfig)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/config", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload struct {
		Providers []ProviderInfo `json:"providers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(payload.Providers))
	}
	if payload.Providers[0].DisplayName == nil || *payload.Providers[0].DisplayName != "GitHub" {
		t.Fatalf("expected displayName GitHub, got %+v", payload.Providers[0].DisplayName)
	}
}

func newAuthTestApp(userID uuid.UUID) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("userId", userID.String())
		return c.Next()
	})
	app.Get("/auth/sessions", HandleListSessions)
	app.Delete("/auth/sessions/others", HandleRevokeOtherSessions)
	app.Delete("/auth/sessions/:id", HandleRevokeSession)
	return app
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func seedSessionWithToken(t *testing.T, db *gorm.DB, userID uuid.UUID, userAgent string, lastSeenAt time.Time, rawRefreshToken string) models.UserSession {
	t.Helper()
	session := models.UserSession{
		ID:          uuid.New(),
		UserID:      userID,
		UserAgent:   userAgent,
		DeviceLabel: buildDeviceLabel(userAgent),
		LastSeenAt:  lastSeenAt,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	token := models.UserRefreshToken{
		ID:        uuid.New(),
		UserID:    userID,
		SessionID: session.ID,
		TokenHash: hashToken(rawRefreshToken),
		ExpiresAt: time.Now().Add(24 * time.Hour),
		CreatedAt: lastSeenAt,
	}
	if err := db.Create(&token).Error; err != nil {
		t.Fatalf("create refresh token: %v", err)
	}
	return session
}

func TestHandleListSessions_ReturnsCurrentAndOtherSessions(t *testing.T) {
	db := setupAuthTestDB(t)
	userID := uuid.New()
	email := "coldin@example.com"
	if err := db.Create(&models.User{ID: userID, Email: &email}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	currentSession := seedSessionWithToken(t, db, userID, "Mozilla/5.0 Firefox", time.Now().Add(-time.Hour), "current-token")
	_ = seedSessionWithToken(t, db, userID, "Mozilla/5.0 Chrome", time.Now().Add(-2*time.Hour), "other-token")

	app := newAuthTestApp(userID)
	req := httptest.NewRequest(http.MethodGet, "/auth/sessions", nil)
	req.AddCookie(&http.Cookie{Name: "cyime_refresh_token", Value: "current-token"})
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload SessionListResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Items) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(payload.Items))
	}

	var foundCurrent bool
	for _, item := range payload.Items {
		if item.ID == currentSession.ID.String() {
			foundCurrent = item.Current
		}
	}
	if !foundCurrent {
		t.Fatalf("expected current session to be marked current")
	}
}

func TestHandleRevokeOtherSessions_RevokesOnlyOtherSessions(t *testing.T) {
	db := setupAuthTestDB(t)
	userID := uuid.New()
	email := "coldin@example.com"
	if err := db.Create(&models.User{ID: userID, Email: &email}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	currentSession := seedSessionWithToken(t, db, userID, "Mozilla/5.0 Firefox", time.Now().Add(-time.Hour), "current-token")
	otherSession := seedSessionWithToken(t, db, userID, "Mozilla/5.0 Chrome", time.Now().Add(-2*time.Hour), "other-token")

	app := newAuthTestApp(userID)
	req := httptest.NewRequest(http.MethodDelete, "/auth/sessions/others", nil)
	req.AddCookie(&http.Cookie{Name: "cyime_refresh_token", Value: "current-token"})
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var currentCount int64
	if err := db.Model(&models.UserSession{}).Where("id = ? AND revoked_at IS NULL", currentSession.ID).Count(&currentCount).Error; err != nil {
		t.Fatalf("count current session: %v", err)
	}
	if currentCount != 1 {
		t.Fatalf("expected current session kept")
	}

	var revokedAt models.UserSession
	if err := db.First(&revokedAt, "id = ?", otherSession.ID).Error; err != nil {
		t.Fatalf("load other session: %v", err)
	}
	if revokedAt.RevokedAt == nil {
		t.Fatalf("expected other session revoked")
	}
}

func TestFindOrCreateUser_AllowsMultipleUsersWithoutEmail(t *testing.T) {
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.UserIdentityProvider{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	profileA := &UserProfile{
		Subject: "github-user-a",
		Email:   "",
		Name:    "User A",
	}
	profileB := &UserProfile{
		Subject: "github-user-b",
		Email:   "",
		Name:    "User B",
	}

	userA, err := findOrCreateUser(db, "github", profileA)
	if err != nil {
		t.Fatalf("create user A: %v", err)
	}
	userB, err := findOrCreateUser(db, "github", profileB)
	if err != nil {
		t.Fatalf("create user B: %v", err)
	}

	if userA.Email != nil {
		t.Fatalf("expected user A email to be nil, got %q", *userA.Email)
	}
	if userB.Email != nil {
		t.Fatalf("expected user B email to be nil, got %q", *userB.Email)
	}
	if userA.ID == userB.ID {
		t.Fatalf("expected different users to be created")
	}
}

func TestFindOrCreateUser_MergesVerifiedEmailAcrossProviders(t *testing.T) {
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.UserIdentityProvider{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	email := "same@example.com"
	existing := models.User{
		ID:            uuid.New(),
		Email:         &email,
		EmailVerified: true,
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing user: %v", err)
	}

	profile := &UserProfile{
		Subject:       "oidc-sub-1",
		Email:         "same@example.com",
		EmailVerified: true,
		Name:          "Merged User",
	}

	user, err := findOrCreateUser(db, "oidc", profile)
	if err != nil {
		t.Fatalf("find or create user: %v", err)
	}
	if user.ID != existing.ID {
		t.Fatalf("expected merge to existing user %s, got %s", existing.ID, user.ID)
	}

	var identity models.UserIdentityProvider
	if err := db.Where("provider_name = ? AND provider_user_id = ?", "oidc", "oidc-sub-1").First(&identity).Error; err != nil {
		t.Fatalf("load identity: %v", err)
	}
	if identity.UserID != existing.ID {
		t.Fatalf("expected identity to link to existing user, got %s", identity.UserID)
	}
}

func TestFindOrCreateUser_IgnoresUnverifiedEmailForMergeAndStorage(t *testing.T) {
	dsn := "file:" + uuid.NewString() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.UserIdentityProvider{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	email := "same@example.com"
	existing := models.User{
		ID:            uuid.New(),
		Email:         &email,
		EmailVerified: true,
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing user: %v", err)
	}

	profile := &UserProfile{
		Subject:       "oidc-sub-2",
		Email:         "same@example.com",
		EmailVerified: false,
		Name:          "No Merge",
	}

	user, err := findOrCreateUser(db, "oidc", profile)
	if err != nil {
		t.Fatalf("find or create user: %v", err)
	}
	if user.ID == existing.ID {
		t.Fatalf("expected unverified email not to merge with existing user")
	}
	if user.Email != nil {
		t.Fatalf("expected unverified email not to be stored, got %q", *user.Email)
	}
}

func TestGetUserProfile_ParsesGoogleOAuthUserInfo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"google-user-1","email":"person@example.com","verified_email":false,"name":"Google User","picture":"https://example.com/avatar.png"}`))
	}))
	defer server.Close()

	provider := &models.AuthProvider{
		Name:         "google",
		ProtocolType: "oauth2",
		UserInfoURL:  &server.URL,
		ClientID:     "test-client",
	}
	oauth2Config := &oauth2.Config{}
	token := &oauth2.Token{AccessToken: "token", TokenType: "Bearer"}

	profile, err := getUserProfile(context.Background(), provider, oauth2Config, token)
	if err != nil {
		t.Fatalf("get user profile: %v", err)
	}
	if profile.Subject != "google-user-1" {
		t.Fatalf("expected subject google-user-1, got %q", profile.Subject)
	}
	if profile.Email != "person@example.com" {
		t.Fatalf("expected email person@example.com, got %q", profile.Email)
	}
	if profile.EmailVerified {
		t.Fatalf("expected google email verification flag to be propagated")
	}
	if profile.Name != "Google User" {
		t.Fatalf("expected name Google User, got %q", profile.Name)
	}
	if profile.Picture != "https://example.com/avatar.png" {
		t.Fatalf("expected picture propagated, got %q", profile.Picture)
	}
}

func TestFetchGitHubPrimaryEmail_PrefersVerifiedAndMarksVerified(t *testing.T) {
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "https://api.github.com/user/emails" {
				t.Fatalf("unexpected URL: %s", req.URL.String())
			}
			body := `[{"email":"unverified@example.com","primary":true,"verified":false},{"email":"verified@example.com","primary":false,"verified":true}]`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(body)),
				Header:     make(http.Header),
			}, nil
		}),
	})

	email, verified, err := fetchGitHubPrimaryEmail(ctx, &oauth2.Config{}, &oauth2.Token{AccessToken: "token"})
	if err != nil {
		t.Fatalf("fetch github primary email: %v", err)
	}
	if email != "verified@example.com" {
		t.Fatalf("expected verified email, got %q", email)
	}
	if !verified {
		t.Fatalf("expected email to be marked verified")
	}
}

func TestFetchGitHubPrimaryEmail_IgnoresUnverifiedWhenNoVerifiedExists(t *testing.T) {
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := `[{"email":"only-unverified@example.com","primary":true,"verified":false}]`
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(bytes.NewBufferString(body)),
				Header:     make(http.Header),
			}, nil
		}),
	})

	email, verified, err := fetchGitHubPrimaryEmail(ctx, &oauth2.Config{}, &oauth2.Token{AccessToken: "token"})
	if err != nil {
		t.Fatalf("fetch github primary email: %v", err)
	}
	if email != "" {
		t.Fatalf("expected no fallback email, got %q", email)
	}
	if verified {
		t.Fatalf("expected no verified email")
	}
}

func TestGetUserProfile_GitHubIgnoresUnverifiedUserEmailWithoutVerifiedEmailRecord(t *testing.T) {
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.com/github/userinfo":
				body := `{"id":123,"login":"octocat","name":"Octo Cat","email":"victim@example.com","avatar_url":"https://example.com/avatar.png"}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(body)),
					Header:     make(http.Header),
				}, nil
			case "https://api.github.com/user/emails":
				body := `[{"email":"victim@example.com","primary":true,"verified":false}]`
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(bytes.NewBufferString(body)),
					Header:     make(http.Header),
				}, nil
			default:
				t.Fatalf("unexpected URL: %s", req.URL.String())
				return nil, nil
			}
		}),
	})

	userInfoURL := "https://example.com/github/userinfo"
	provider := &models.AuthProvider{
		Name:         "github",
		ProtocolType: "oauth2",
		UserInfoURL:  &userInfoURL,
		ClientID:     "test-client",
	}

	profile, err := getUserProfile(ctx, provider, &oauth2.Config{}, &oauth2.Token{AccessToken: "token"})
	if err != nil {
		t.Fatalf("get user profile: %v", err)
	}
	if profile.Email != "" {
		t.Fatalf("expected github email to be empty without a verified email record, got %q", profile.Email)
	}
	if profile.EmailVerified {
		t.Fatalf("expected github email to remain unverified")
	}
}
