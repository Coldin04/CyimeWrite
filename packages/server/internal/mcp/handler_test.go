package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"g.co1d.in/Coldin04/Cyime/server/internal/apitoken"
	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupMCPTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.ApiToken{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	database.DB = db
	return db
}

func createMCPTestToken(t *testing.T, scopes []string) string {
	t.Helper()

	userID := uuid.New()
	if err := database.DB.Create(&models.User{ID: userID}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	created, err := apitoken.CreateToken(userID, apitoken.CreateTokenInput{
		Name:   "mcp-test",
		Scopes: scopes,
	})
	if err != nil {
		t.Fatalf("CreateToken returned error: %v", err)
	}
	return created.Token
}

func newMCPTestApp() *fiber.App {
	app := fiber.New()
	app.Post("/api/v1/mcp", apitoken.Protected(), Handle)
	return app
}

func postMCP(t *testing.T, app *fiber.App, token string, body string) *http.Response {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test returned error: %v", err)
	}
	return resp
}

func TestHandleRequiresAPIToken(t *testing.T) {
	setupMCPTestDB(t)
	app := newMCPTestApp()

	resp := postMCP(t, app, "", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusUnauthorized)
	}
}

func TestHandleToolsList(t *testing.T) {
	setupMCPTestDB(t)
	app := newMCPTestApp()
	token := createMCPTestToken(t, []string{apitoken.ScopeWorkspaceRead})

	resp := postMCP(t, app, token, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}

	var payload rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Error != nil {
		t.Fatalf("unexpected error: %#v", payload.Error)
	}

	raw, err := json.Marshal(payload.Result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if !strings.Contains(string(raw), "cyime_list_files") {
		t.Fatalf("tools/list did not include cyime_list_files: %s", raw)
	}
}

func TestHandleToolsCallChecksScopes(t *testing.T) {
	setupMCPTestDB(t)
	app := newMCPTestApp()
	token := createMCPTestToken(t, []string{apitoken.ScopeWorkspaceRead})

	resp := postMCP(t, app, token, `{
		"jsonrpc": "2.0",
		"id": 2,
		"method": "tools/call",
		"params": {
			"name": "cyime_create_folder",
			"arguments": {"name": "blocked"}
		}
	}`)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}

	var payload rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Error == nil {
		t.Fatal("expected scope error")
	}
	if payload.Error.Code != jsonRPCForbidden {
		t.Fatalf("error code = %d, want %d", payload.Error.Code, jsonRPCForbidden)
	}
}
