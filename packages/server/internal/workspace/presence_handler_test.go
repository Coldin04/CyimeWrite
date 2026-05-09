package workspace

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func resetPresenceTestState(t *testing.T) {
	t.Helper()

	presenceMu.Lock()
	presenceStore = map[uuid.UUID]map[string]presenceEntry{}
	presenceMu.Unlock()

	presenceAuthMu.Lock()
	presenceAuthCache = map[uuid.UUID]map[uuid.UUID]time.Time{}
	presenceAuthMu.Unlock()
}

func newPresenceTestApp(userID uuid.UUID) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("userId", userID.String())
		return c.Next()
	})
	app.Put("/documents/:id/presence", HeartbeatDocumentPresenceHandler)
	app.Get("/documents/:id/presence", GetDocumentPresenceHandler)
	return app
}

func putPresenceHeartbeat(t *testing.T, app *fiber.App, documentID uuid.UUID, body string, sessionHeader string) *http.Response {
	t.Helper()

	req := httptest.NewRequest(http.MethodPut, "/documents/"+documentID.String()+"/presence", bytes.NewBufferString(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if sessionHeader != "" {
		req.Header.Set(presenceSessionIDHeader, sessionHeader)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func TestUpdatePresenceRejectsInvalidAndCapsUserSessions(t *testing.T) {
	resetPresenceTestState(t)
	documentID := uuid.New()
	userID := uuid.New()

	if _, _, err := updatePresence(documentID, userID, "not-a-uuid"); err == nil {
		t.Fatal("expected invalid UUID session ID to be rejected")
	}
	if _, _, err := updatePresence(documentID, userID, strings.Repeat("a", 1024)); err == nil {
		t.Fatal("expected oversized session ID to be rejected")
	}
	if connectedCount, uniqueUserCount := readPresence(documentID); connectedCount != 0 || uniqueUserCount != 0 {
		t.Fatalf("invalid sessions were stored: connected=%d unique=%d", connectedCount, uniqueUserCount)
	}

	firstSessionID := uuid.NewString()
	for i := 0; i < presenceMaxSessionsPerUserDocument; i++ {
		sessionID := firstSessionID
		if i > 0 {
			sessionID = uuid.NewString()
		}
		connectedCount, uniqueUserCount, err := updatePresence(documentID, userID, sessionID)
		if err != nil {
			t.Fatalf("session %d should be accepted: %v", i+1, err)
		}
		if connectedCount != i+1 || uniqueUserCount != 1 {
			t.Fatalf("unexpected counts after session %d: connected=%d unique=%d", i+1, connectedCount, uniqueUserCount)
		}
	}

	connectedCount, uniqueUserCount, err := updatePresence(documentID, userID, uuid.NewString())
	if err == nil {
		t.Fatal("expected per-user presence session cap to reject a new session")
	}
	if connectedCount != presenceMaxSessionsPerUserDocument || uniqueUserCount != 1 {
		t.Fatalf("unexpected counts after capped session: connected=%d unique=%d", connectedCount, uniqueUserCount)
	}

	connectedCount, uniqueUserCount, err = updatePresence(documentID, userID, firstSessionID)
	if err != nil {
		t.Fatalf("refreshing an existing capped session should be accepted: %v", err)
	}
	if connectedCount != presenceMaxSessionsPerUserDocument || uniqueUserCount != 1 {
		t.Fatalf("unexpected counts after refresh: connected=%d unique=%d", connectedCount, uniqueUserCount)
	}

	connectedCount, uniqueUserCount, err = updatePresence(documentID, uuid.New(), firstSessionID)
	if err != nil {
		t.Fatalf("a different user should be allowed to use the same UUID session value: %v", err)
	}
	if connectedCount != presenceMaxSessionsPerUserDocument+1 || uniqueUserCount != 2 {
		t.Fatalf("session keys are not scoped per user: connected=%d unique=%d", connectedCount, uniqueUserCount)
	}
}

func TestHeartbeatDocumentPresenceHandlerValidatesSessionID(t *testing.T) {
	resetPresenceTestState(t)
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	documentID := seedDocumentForWorkspace(t, db, ownerID, "presence-doc")
	app := newPresenceTestApp(ownerID)

	invalidResp := putPresenceHeartbeat(t, app, documentID, `{"sessionId":"not-a-uuid"}`, "")
	if invalidResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected invalid session ID to return 400, got %d", invalidResp.StatusCode)
	}
	if connectedCount, uniqueUserCount := readPresence(documentID); connectedCount != 0 || uniqueUserCount != 0 {
		t.Fatalf("invalid heartbeat was stored: connected=%d unique=%d", connectedCount, uniqueUserCount)
	}

	oversizedResp := putPresenceHeartbeat(t, app, documentID, `{"sessionId":"`+strings.Repeat("a", presenceMaxRequestBodyBytes)+`"}`, "")
	if oversizedResp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected oversized heartbeat body to return 413, got %d", oversizedResp.StatusCode)
	}

	validSessionID := uuid.NewString()
	validResp := putPresenceHeartbeat(t, app, documentID, "", validSessionID)
	if validResp.StatusCode != http.StatusOK {
		t.Fatalf("expected valid header session ID to return 200, got %d", validResp.StatusCode)
	}

	var body documentPresenceResponse
	if err := json.NewDecoder(validResp.Body).Decode(&body); err != nil {
		t.Fatalf("decode presence response: %v", err)
	}
	if body.ConnectedCount != 1 || body.UniqueUserCount != 1 {
		t.Fatalf("unexpected presence response: %+v", body)
	}
}
