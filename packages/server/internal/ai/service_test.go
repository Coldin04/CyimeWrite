package ai

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupAIServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.AutoMigrate(
		&models.User{},
		&models.Document{},
		&models.Folder{},
		&models.DocumentBody{},
		&models.DocumentPermission{},
		&models.BlobObject{},
		&models.Asset{},
		&models.DocumentAssetRef{},
		&models.AssetGCJob{},
		&models.BlobGCJob{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	database.DB = db
	return db
}

func seedAIMarkdownDocument(t *testing.T, db *gorm.DB, ownerID uuid.UUID, title string, markdown string, version int64) uuid.UUID {
	t.Helper()

	if err := db.Create(&models.User{ID: ownerID}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	contentJSON, err := markdownToContentJSON(markdown)
	if err != nil {
		t.Fatalf("markdownToContentJSON: %v", err)
	}

	document := models.Document{
		ID:           uuid.New(),
		OwnerUserID:  ownerID,
		Title:        title,
		DocumentType: "rich_text",
		EditorType:   "tiptap",
		CreatedBy:    ownerID,
		UpdatedBy:    ownerID,
	}
	if err := db.Create(&document).Error; err != nil {
		t.Fatalf("create document: %v", err)
	}

	body := models.DocumentBody{
		ID:             uuid.New(),
		DocumentID:     document.ID,
		ContentJSON:    string(contentJSON),
		PlainText:      "seed",
		ContentVersion: version,
		UpdatedBy:      ownerID,
	}
	if err := db.Create(&body).Error; err != nil {
		t.Fatalf("create document body: %v", err)
	}

	return document.ID
}

func TestGetMarkdownContentReturnsMarkdown(t *testing.T) {
	db := setupAIServiceTestDB(t)
	ownerID := uuid.New()
	docID := seedAIMarkdownDocument(t, db, ownerID, "notes", "# Notes\n\nHello Cyime", 3)

	result, err := GetMarkdownContent(ownerID, docID)
	if err != nil {
		t.Fatalf("GetMarkdownContent returned error: %v", err)
	}
	if result.Format != "markdown" {
		t.Fatalf("format = %q, want markdown", result.Format)
	}
	if result.Version != 3 {
		t.Fatalf("version = %d, want 3", result.Version)
	}
	if !strings.Contains(result.Content, "# Notes") || !strings.Contains(result.Content, "Hello Cyime") {
		t.Fatalf("unexpected markdown content:\n%s", result.Content)
	}
}

func TestUpdateMarkdownContentWritesMarkdown(t *testing.T) {
	db := setupAIServiceTestDB(t)
	ownerID := uuid.New()
	docID := seedAIMarkdownDocument(t, db, ownerID, "notes", "# Before\n\nold", 1)

	baseVersion := int64(1)
	result, err := UpdateMarkdownContent(ownerID, docID, "# After\n\nnew", &baseVersion)
	if err != nil {
		t.Fatalf("UpdateMarkdownContent returned error: %v", err)
	}
	if !result.Success {
		t.Fatal("expected update success")
	}
	if result.Version != 2 {
		t.Fatalf("version = %d, want 2", result.Version)
	}

	stored, err := GetMarkdownContent(ownerID, docID)
	if err != nil {
		t.Fatalf("GetMarkdownContent after update returned error: %v", err)
	}
	if !strings.Contains(stored.Content, "# After") || !strings.Contains(stored.Content, "new") {
		t.Fatalf("updated markdown not stored:\n%s", stored.Content)
	}

	var body models.DocumentBody
	if err := db.Where("document_id = ?", docID).First(&body).Error; err != nil {
		t.Fatalf("load body: %v", err)
	}
	if body.ContentVersion != 2 {
		t.Fatalf("stored content_version = %d, want 2", body.ContentVersion)
	}
}

func TestUpdateMarkdownContentRejectsVersionConflict(t *testing.T) {
	db := setupAIServiceTestDB(t)
	ownerID := uuid.New()
	docID := seedAIMarkdownDocument(t, db, ownerID, "notes", "# Original\n\nkeep", 5)

	baseVersion := int64(4)
	if _, err := UpdateMarkdownContent(ownerID, docID, "# Stale\n\nlost update", &baseVersion); !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("error = %v, want ErrVersionConflict", err)
	}

	current, err := GetMarkdownContent(ownerID, docID)
	if err != nil {
		t.Fatalf("GetMarkdownContent after conflict returned error: %v", err)
	}
	if !strings.Contains(current.Content, "# Original") || strings.Contains(current.Content, "Stale") {
		t.Fatalf("conflicting update changed content:\n%s", current.Content)
	}
}
