package content

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

func setupContentTestDB(t *testing.T) *gorm.DB {
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

func seedContentPermission(t *testing.T, db *gorm.DB, documentID, userID, createdBy uuid.UUID, role string) {
	t.Helper()
	permission := models.DocumentPermission{
		ID:         uuid.New(),
		DocumentID: documentID,
		UserID:     userID,
		Role:       role,
		CreatedBy:  createdBy,
	}
	if err := db.Create(&permission).Error; err != nil {
		t.Fatalf("create document permission: %v", err)
	}
}

func seedContentBlob(t *testing.T, db *gorm.DB, objectKey string, mimeType string, size int64, hash string) models.BlobObject {
	t.Helper()
	blob := models.BlobObject{
		ID:              uuid.New(),
		SHA256:          hash,
		Size:            size,
		MimeType:        mimeType,
		StorageProvider: "local",
		ObjectKey:       objectKey,
		URL:             "http://example.test/" + objectKey,
		Status:          "ready",
	}
	if err := db.Create(&blob).Error; err != nil {
		t.Fatalf("create blob: %v", err)
	}
	return blob
}

func seedDocumentForContent(t *testing.T, db *gorm.DB, ownerID uuid.UUID, title, contentJSON string) (uuid.UUID, uuid.UUID) {
	t.Helper()

	doc := models.Document{
		ID:           uuid.New(),
		OwnerUserID:  ownerID,
		Title:        title,
		Excerpt:      "seed",
		DocumentType: "rich_text",
		EditorType:   "tiptap",
		CreatedBy:    ownerID,
		UpdatedBy:    ownerID,
	}
	if err := db.Create(&doc).Error; err != nil {
		t.Fatalf("create document: %v", err)
	}

	docContent := models.DocumentBody{
		ID:             uuid.New(),
		DocumentID:     doc.ID,
		ContentJSON:    contentJSON,
		PlainText:      "seed",
		ContentVersion: 1,
		UpdatedBy:      ownerID,
	}
	if err := db.Create(&docContent).Error; err != nil {
		t.Fatalf("create document content: %v", err)
	}

	return doc.ID, docContent.ID
}

func TestGetContent_DeniesCrossUserAccess(t *testing.T) {
	db := setupContentTestDB(t)
	ownerID := uuid.New()
	attackerID := uuid.New()
	docID, _ := seedDocumentForContent(t, db, ownerID, "owner-doc", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"secret"}]}]}`)

	if _, err := GetContent(attackerID, docID); err == nil {
		t.Fatal("expected cross-user get content to fail")
	}
}

func TestGetContent_AllowsViewerPermission(t *testing.T) {
	db := setupContentTestDB(t)
	ownerID := uuid.New()
	viewerID := uuid.New()
	docID, _ := seedDocumentForContent(t, db, ownerID, "owner-doc", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"shared"}]}]}`)
	seedContentPermission(t, db, docID, viewerID, ownerID, "viewer")

	result, err := GetContent(viewerID, docID)
	if err != nil {
		t.Fatalf("expected shared viewer access, got %v", err)
	}
	if result.DocumentID != docID {
		t.Fatalf("unexpected document result: %+v", result)
	}
}

func TestUpdateContent_DeniesCrossUserAccessAndKeepsData(t *testing.T) {
	db := setupContentTestDB(t)
	ownerID := uuid.New()
	attackerID := uuid.New()
	docID, contentID := seedDocumentForContent(t, db, ownerID, "owner-doc", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"before"}]}]}`)

	if _, err := UpdateContent(attackerID, docID, []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"hacked"}]}]}`)); err == nil {
		t.Fatal("expected cross-user update content to fail")
	}

	var got models.DocumentBody
	if err := db.First(&got, "id = ?", contentID).Error; err != nil {
		t.Fatalf("load content: %v", err)
	}
	expected := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"before"}]}]}`
	if got.ContentJSON != expected {
		t.Fatalf("expected content unchanged, got: %q", got.ContentJSON)
	}
	if got.UpdatedBy != ownerID {
		t.Fatalf("expected updated_by unchanged, got: %s", got.UpdatedBy)
	}
}

func TestUpdateContent_AllowsEditorPermission(t *testing.T) {
	db := setupContentTestDB(t)
	ownerID := uuid.New()
	editorID := uuid.New()
	docID, _ := seedDocumentForContent(t, db, ownerID, "owner-doc", `{"type":"doc","content":[{"type":"paragraph"}]}`)
	seedContentPermission(t, db, docID, editorID, ownerID, "editor")

	blob := seedContentBlob(t, db, "owner/shared.png", "image/png", 12, "hash-shared")
	asset := models.Asset{
		ID:             uuid.New(),
		OwnerUserID:    ownerID,
		BlobID:         blob.ID,
		Kind:           "image",
		Filename:       "shared.png",
		URL:            blob.URL,
		Visibility:     "private",
		Status:         "ready",
		ReferenceCount: 0,
		CreatedBy:      ownerID,
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}

	payload := []byte(fmt.Sprintf(`{"type":"doc","content":[{"type":"image","attrs":{"assetId":"%s"}}]}`, asset.ID))
	if _, err := UpdateContent(editorID, docID, payload); err != nil {
		t.Fatalf("expected shared editor update to succeed, got %v", err)
	}

	var ref models.DocumentAssetRef
	if err := db.First(&ref, "document_id = ? AND asset_id = ?", docID, asset.ID).Error; err != nil {
		t.Fatalf("load ref: %v", err)
	}
	if ref.OwnerUserID != ownerID {
		t.Fatalf("expected ref owner to stay document owner, got %s", ref.OwnerUserID)
	}
}

func TestUpdateContent_DeniesEditorWhenCollaborationDisabled(t *testing.T) {
	t.Setenv("COLLABORATION_ENABLED", "false")

	db := setupContentTestDB(t)
	ownerID := uuid.New()
	editorID := uuid.New()
	docID, _ := seedDocumentForContent(t, db, ownerID, "owner-doc", `{"type":"doc","content":[{"type":"paragraph"}]}`)
	seedContentPermission(t, db, docID, editorID, ownerID, "editor")

	if _, err := UpdateContent(editorID, docID, []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"blocked"}]}]}`)); err == nil {
		t.Fatal("expected editor update to fail when collaboration is disabled")
	}
}

func TestUpdateContent_SyncsDocumentAssetRefsAndAssetState(t *testing.T) {
	db := setupContentTestDB(t)
	ownerID := uuid.New()
	docID, _ := seedDocumentForContent(t, db, ownerID, "owner-doc", `{"type":"doc","content":[{"type":"paragraph"}]}`)

	blobA := seedContentBlob(t, db, "owner/a.png", "image/png", 10, "hash-a")
	assetA := models.Asset{
		ID:             uuid.New(),
		OwnerUserID:    ownerID,
		BlobID:         blobA.ID,
		Kind:           "image",
		Filename:       "a.png",
		URL:            blobA.URL,
		Visibility:     "private",
		Status:         "ready",
		ReferenceCount: 0,
		CreatedBy:      ownerID,
	}
	blobB := seedContentBlob(t, db, "owner/b.png", "image/png", 11, "hash-b")
	assetB := models.Asset{
		ID:             uuid.New(),
		OwnerUserID:    ownerID,
		BlobID:         blobB.ID,
		Kind:           "image",
		Filename:       "b.png",
		URL:            blobB.URL,
		Visibility:     "private",
		Status:         "ready",
		ReferenceCount: 0,
		CreatedBy:      ownerID,
	}
	if err := db.Create(&assetA).Error; err != nil {
		t.Fatalf("create assetA: %v", err)
	}
	if err := db.Create(&assetB).Error; err != nil {
		t.Fatalf("create assetB: %v", err)
	}

	firstPayload := []byte(fmt.Sprintf(`{"type":"doc","content":[{"type":"image","attrs":{"src":"http://localhost/api/v1/media/assets/%s/content?token=x","assetId":"%s"}}]}`, assetA.ID, assetA.ID))
	if _, err := UpdateContent(ownerID, docID, firstPayload); err != nil {
		t.Fatalf("first update: %v", err)
	}

	var refs []models.DocumentAssetRef
	if err := db.Order("asset_id asc").Find(&refs).Error; err != nil {
		t.Fatalf("load refs: %v", err)
	}
	if len(refs) != 1 || refs[0].AssetID != assetA.ID {
		t.Fatalf("expected only assetA ref, got %+v", refs)
	}

	var gotAssetA models.Asset
	if err := db.First(&gotAssetA, "id = ?", assetA.ID).Error; err != nil {
		t.Fatalf("load assetA: %v", err)
	}
	if gotAssetA.ReferenceCount != 1 || gotAssetA.Status != "ready" {
		t.Fatalf("expected assetA ready with ref=1, got status=%s ref=%d", gotAssetA.Status, gotAssetA.ReferenceCount)
	}

	secondPayload := []byte(fmt.Sprintf(`{"type":"doc","content":[{"type":"image","attrs":{"src":"http://localhost/api/v1/media/assets/%s/content","assetId":"%s"}}]}`, assetB.ID, assetB.ID))
	if _, err := UpdateContent(ownerID, docID, secondPayload); err != nil {
		t.Fatalf("second update: %v", err)
	}

	refs = nil
	if err := db.Order("asset_id asc").Find(&refs).Error; err != nil {
		t.Fatalf("reload refs: %v", err)
	}
	if len(refs) != 1 || refs[0].AssetID != assetB.ID {
		t.Fatalf("expected only assetB ref after replacement, got %+v", refs)
	}

	if err := db.First(&gotAssetA, "id = ?", assetA.ID).Error; err != nil {
		t.Fatalf("reload assetA: %v", err)
	}
	if gotAssetA.ReferenceCount != 0 || gotAssetA.Status != "pending_delete" {
		t.Fatalf("expected assetA pending_delete with ref=0, got status=%s ref=%d", gotAssetA.Status, gotAssetA.ReferenceCount)
	}

	var deleteJobs []models.AssetGCJob
	if err := db.Where("asset_id = ? AND job_type = ?", assetA.ID, "delete").Find(&deleteJobs).Error; err != nil {
		t.Fatalf("load delete jobs: %v", err)
	}
	if len(deleteJobs) != 1 || deleteJobs[0].Status != "pending" {
		t.Fatalf("expected one pending delete job for assetA, got %+v", deleteJobs)
	}
}

func TestUpdateContent_RejectsForeignAssetReference(t *testing.T) {
	db := setupContentTestDB(t)
	ownerID := uuid.New()
	otherUserID := uuid.New()
	docID, _ := seedDocumentForContent(t, db, ownerID, "owner-doc", `{"type":"doc","content":[{"type":"paragraph"}]}`)

	foreignBlob := seedContentBlob(t, db, "other/foreign.png", "image/png", 99, "hash-foreign")
	foreignAsset := models.Asset{
		ID:          uuid.New(),
		OwnerUserID: otherUserID,
		BlobID:      foreignBlob.ID,
		Kind:        "image",
		Filename:    "foreign.png",
		URL:         foreignBlob.URL,
		Visibility:  "private",
		Status:      "ready",
		CreatedBy:   otherUserID,
	}
	if err := db.Create(&foreignAsset).Error; err != nil {
		t.Fatalf("create foreign asset: %v", err)
	}

	payload := []byte(fmt.Sprintf(`{"type":"doc","content":[{"type":"image","attrs":{"assetId":"%s"}}]}`, foreignAsset.ID))
	if _, err := UpdateContent(ownerID, docID, payload); err == nil || err.Error() != "content references invalid assets" {
		t.Fatalf("expected invalid asset error, got: %v", err)
	}
}

func TestUpdateContent_RejectsWhenWorkspaceStorageQuotaWouldBeExceeded(t *testing.T) {
	db := setupContentTestDB(t)
	ownerID := uuid.New()
	docID, contentID := seedDocumentForContent(t, db, ownerID, "owner-doc", `{"type":"doc","content":[{"type":"paragraph"}]}`)

	largeDescription := strings.Repeat("d", maxWorkspaceStorageBytesPerUser-40)
	folder := models.Folder{
		ID:          uuid.New(),
		OwnerUserID: ownerID,
		Name:        "large",
		Description: &largeDescription,
		CreatedBy:   ownerID,
		UpdatedBy:   ownerID,
	}
	if err := db.Create(&folder).Error; err != nil {
		t.Fatalf("create folder: %v", err)
	}

	payload := []byte(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"this update should exceed workspace quota"}]}]}`)
	if _, err := UpdateContent(ownerID, docID, payload); !errors.Is(err, ErrWorkspaceStorageQuotaExceeded) {
		t.Fatalf("expected workspace quota error, got: %v", err)
	}

	var got models.DocumentBody
	if err := db.First(&got, "id = ?", contentID).Error; err != nil {
		t.Fatalf("reload content: %v", err)
	}
	expected := `{"type":"doc","content":[{"type":"paragraph"}]}`
	if got.ContentJSON != expected {
		t.Fatalf("expected original content unchanged, got %q", got.ContentJSON)
	}
}

func TestDeleteAndRestoreContent_ReconcilesDocumentAssetRefs(t *testing.T) {
	db := setupContentTestDB(t)
	ownerID := uuid.New()
	docID, _ := seedDocumentForContent(t, db, ownerID, "owner-doc", `{"type":"doc","content":[{"type":"paragraph"}]}`)

	blob := seedContentBlob(t, db, "owner/asset.png", "image/png", 42, "hash-asset")
	asset := models.Asset{
		ID:             uuid.New(),
		OwnerUserID:    ownerID,
		BlobID:         blob.ID,
		Kind:           "image",
		Filename:       "asset.png",
		URL:            blob.URL,
		Visibility:     "private",
		Status:         "ready",
		ReferenceCount: 0,
		CreatedBy:      ownerID,
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}

	payload := []byte(fmt.Sprintf(`{"type":"doc","content":[{"type":"image","attrs":{"assetId":"%s"}}]}`, asset.ID))
	if _, err := UpdateContent(ownerID, docID, payload); err != nil {
		t.Fatalf("update content: %v", err)
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		return DeleteContentByDocumentID(tx, ownerID, docID)
	}); err != nil {
		t.Fatalf("delete content: %v", err)
	}

	var refCount int64
	if err := db.Model(&models.DocumentAssetRef{}).Where("document_id = ?", docID).Count(&refCount).Error; err != nil {
		t.Fatalf("count refs after delete: %v", err)
	}
	if refCount != 0 {
		t.Fatalf("expected refs removed after delete, got %d", refCount)
	}

	var gotAsset models.Asset
	if err := db.First(&gotAsset, "id = ?", asset.ID).Error; err != nil {
		t.Fatalf("load asset after delete: %v", err)
	}
	if gotAsset.ReferenceCount != 0 || gotAsset.Status != "pending_delete" {
		t.Fatalf("expected asset pending_delete after content delete, got status=%s ref=%d", gotAsset.Status, gotAsset.ReferenceCount)
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Model(&models.Document{}).Where("id = ?", docID).Update("deleted_at", nil).Error; err != nil {
			return err
		}
		return RestoreContentByDocumentID(tx, ownerID, docID)
	}); err != nil {
		t.Fatalf("restore content: %v", err)
	}

	if err := db.Model(&models.DocumentAssetRef{}).Where("document_id = ?", docID).Count(&refCount).Error; err != nil {
		t.Fatalf("count refs after restore: %v", err)
	}
	if refCount != 1 {
		t.Fatalf("expected refs restored, got %d", refCount)
	}

	if err := db.First(&gotAsset, "id = ?", asset.ID).Error; err != nil {
		t.Fatalf("load asset after restore: %v", err)
	}
	if gotAsset.ReferenceCount != 1 || gotAsset.Status != "ready" {
		t.Fatalf("expected asset ready after restore, got status=%s ref=%d", gotAsset.Status, gotAsset.ReferenceCount)
	}
}
