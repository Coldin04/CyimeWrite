package apitoken

import (
	"fmt"
	"testing"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAPITokenTestDB(t *testing.T) *gorm.DB {
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

func TestUpdateTokenRenamesAndReplacesScopes(t *testing.T) {
	db := setupAPITokenTestDB(t)
	userID := uuid.New()
	if err := db.Create(&models.User{ID: userID}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	created, err := CreateToken(userID, CreateTokenInput{
		Name:      "before",
		Scopes:    []string{ScopeWorkspaceRead},
		ExpiresAt: &expiresAt,
	})
	if err != nil {
		t.Fatalf("CreateToken returned error: %v", err)
	}

	updated, err := UpdateToken(userID, created.ID, UpdateTokenInput{
		Name:   "after",
		Scopes: []string{ScopeWorkspaceRead, ScopeDocumentWrite},
	})
	if err != nil {
		t.Fatalf("UpdateToken returned error: %v", err)
	}
	if updated.Name != "after" {
		t.Fatalf("name = %q, want after", updated.Name)
	}
	if updated.ExpiresAt == nil || !updated.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expiration changed: got %v, want %v", updated.ExpiresAt, expiresAt)
	}
	if !HasScopes(updated.Scopes, ScopeWorkspaceRead, ScopeDocumentWrite) || HasScopes(updated.Scopes, ScopeFileCopy) {
		t.Fatalf("unexpected scopes: %#v", updated.Scopes)
	}

	authenticated, err := Authenticate(created.Token, "127.0.0.1")
	if err != nil {
		t.Fatalf("Authenticate returned error after update: %v", err)
	}
	if authenticated.TokenID != created.ID {
		t.Fatalf("authenticated token id = %s, want %s", authenticated.TokenID, created.ID)
	}
	if !HasScopes(authenticated.Scopes, ScopeWorkspaceRead, ScopeDocumentWrite) {
		t.Fatalf("authenticated scopes were not updated: %#v", authenticated.Scopes)
	}
}
