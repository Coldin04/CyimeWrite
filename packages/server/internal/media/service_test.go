package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupMediaTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	t.Setenv("APP_ENCRYPTION_KEY", "test-app-encryption-key")
	// Keep tests deterministic: avoid async thumbnail writes racing on SQLite locks.
	t.Setenv("MEDIA_THUMBNAIL_SOURCE_MAX_BYTES", "1")
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.UserImageBedConfig{}, &models.Document{}, &models.DocumentPermission{}, &models.DocumentImageTargetPreference{}, &models.BlobObject{}, &models.Asset{}, &models.DocumentAssetRef{}, &models.AssetGCJob{}, &models.BlobGCJob{}); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}
	database.DB = db
	return db
}

func seedBlob(t *testing.T, db *gorm.DB, objectKey string, mimeType string, size int64, hash string) models.BlobObject {
	t.Helper()
	blob := models.BlobObject{
		ID:              uuid.New(),
		SHA256:          hash,
		Size:            size,
		MimeType:        mimeType,
		StorageProvider: "local",
		ObjectKey:       objectKey,
		URL:             "http://example.test/" + filepath.Base(objectKey),
		Status:          "ready",
	}
	if err := db.Create(&blob).Error; err != nil {
		t.Fatalf("create blob: %v", err)
	}
	return blob
}

func seedOwnedDocument(t *testing.T, db *gorm.DB, userID uuid.UUID) uuid.UUID {
	return seedOwnedDocumentWithImageTarget(t, db, userID, "")
}

func seedOwnedDocumentWithImageTarget(t *testing.T, db *gorm.DB, userID uuid.UUID, preferredImageTargetID string) uuid.UUID {
	t.Helper()
	doc := models.Document{
		ID:                     uuid.New(),
		OwnerUserID:            userID,
		Title:                  "doc",
		DocumentType:           "rich_text",
		PreferredImageTargetID: preferredImageTargetID,
		EditorType:             "tiptap",
		CreatedBy:              userID,
		UpdatedBy:              userID,
	}
	if err := db.Create(&doc).Error; err != nil {
		t.Fatalf("create document: %v", err)
	}
	return doc.ID
}

func seedDocumentPermission(t *testing.T, db *gorm.DB, documentID, userID, createdBy uuid.UUID, role string) {
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

func makeFileHeader(t *testing.T, fieldName, filename string, content []byte) *multipart.FileHeader {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(fieldName, filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	req := httptest.NewRequest("POST", "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(8 << 20); err != nil {
		t.Fatalf("parse multipart form: %v", err)
	}
	return req.MultipartForm.File[fieldName][0]
}

func TestBuildBlobThumbnailRejectsOversizedPNGBeforeDecode(t *testing.T) {
	pngBytes := makePNGHeaderOnly(t, blobThumbnailMaxSourceEdge+1, blobThumbnailMaxSourceEdge+1)

	_, _, _, _, err := buildBlobThumbnail(pngBytes, "image/png")
	if err == nil {
		t.Fatalf("expected oversized PNG to be rejected")
	}
	if !strings.Contains(err.Error(), "exceed maximum edge") {
		t.Fatalf("expected maximum edge error, got %v", err)
	}
}

func TestShouldGenerateBlobThumbnailDoesNotRetryTerminalStatuses(t *testing.T) {
	for _, status := range []string{blobThumbnailStatusFailed, blobThumbnailStatusSkipped} {
		blob := models.BlobObject{MimeType: "image/png", ThumbnailStatus: status}
		if shouldGenerateBlobThumbnail(blob) {
			t.Fatalf("expected status %q not to be scheduled", status)
		}
	}

	if !shouldGenerateBlobThumbnail(models.BlobObject{MimeType: "image/png", ThumbnailStatus: blobThumbnailStatusPending}) {
		t.Fatalf("expected pending image thumbnail to be scheduled")
	}
}

func makePNGHeaderOnly(t *testing.T, width, height int) []byte {
	t.Helper()
	var out bytes.Buffer
	out.Write([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})

	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], uint32(width))
	binary.BigEndian.PutUint32(ihdr[4:8], uint32(height))
	ihdr[8] = 8 // bit depth
	ihdr[9] = 6 // RGBA color type
	writePNGChunk(&out, "IHDR", ihdr)
	return out.Bytes()
}

func writePNGChunk(out *bytes.Buffer, chunkType string, data []byte) {
	_ = binary.Write(out, binary.BigEndian, uint32(len(data)))
	out.WriteString(chunkType)
	out.Write(data)
	crc := crc32.NewIEEE()
	_, _ = crc.Write([]byte(chunkType))
	_, _ = crc.Write(data)
	_ = binary.Write(out, binary.BigEndian, crc.Sum32())
}

func TestUploadDocumentAsset_DeduplicatesByHashAndSize(t *testing.T) {
	db := setupMediaTestDB(t)
	userID := uuid.New()
	docID := seedOwnedDocument(t, db, userID)

	t.Setenv("MEDIA_STORAGE_PROVIDER", "local")
	t.Setenv("MEDIA_LOCAL_ROOT_DIR", t.TempDir())
	storageProvider = nil

	headerA := makeFileHeader(t, "file", "photo.png", []byte("same-content"))
	first, err := UploadDocumentAsset(context.Background(), UploadAssetRequest{
		DocumentID: docID,
		UserID:     userID,
		FileHeader: headerA,
		Visibility: "private",
	})
	if err != nil {
		t.Fatalf("first upload: %v", err)
	}

	headerB := makeFileHeader(t, "file", "duplicate.png", []byte("same-content"))
	second, err := UploadDocumentAsset(context.Background(), UploadAssetRequest{
		DocumentID: docID,
		UserID:     userID,
		FileHeader: headerB,
		Visibility: "private",
	})
	if err != nil {
		t.Fatalf("second upload: %v", err)
	}

	if first.Asset.ID != second.Asset.ID {
		t.Fatalf("expected deduplicated asset id, got %s and %s", first.Asset.ID, second.Asset.ID)
	}

	var asset models.Asset
	if err := db.First(&asset, "id = ?", first.Asset.ID).Error; err != nil {
		t.Fatalf("load asset: %v", err)
	}
	if asset.BlobID == uuid.Nil {
		t.Fatalf("expected uploaded asset to reference blob")
	}
	if asset.ReferenceCount != 0 {
		t.Fatalf("expected reference_count=0 before save-time sync, got %d", asset.ReferenceCount)
	}
}

func TestUploadDocumentAsset_RevivesSoftDeletedBlobOnUniqueConflict(t *testing.T) {
	db := setupMediaTestDB(t)
	userID := uuid.New()
	docID := seedOwnedDocument(t, db, userID)
	content := []byte("same-content")
	hash := computeFileHash(content)

	blob := seedBlob(t, db, "owner/old.png", "image/png", int64(len(content)), hash)
	if err := db.Model(&models.BlobObject{}).
		Where("id = ?", blob.ID).
		Updates(map[string]any{
			"status":               "deleted",
			"thumbnail_object_key": "owner/old__thumb_sm.png",
			"thumbnail_mime_type":  "image/png",
			"thumbnail_size":       1234,
			"thumbnail_status":     blobThumbnailStatusReady,
		}).Error; err != nil {
		t.Fatalf("mark blob deleted: %v", err)
	}
	if err := db.Delete(&models.BlobObject{ID: blob.ID}).Error; err != nil {
		t.Fatalf("soft delete blob: %v", err)
	}
	if err := db.Create(&models.BlobGCJob{
		ID:       uuid.New(),
		BlobID:   blob.ID,
		JobType:  "delete",
		Status:   "pending",
		RunAfter: time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatalf("create blob gc job: %v", err)
	}

	t.Setenv("MEDIA_STORAGE_PROVIDER", "local")
	t.Setenv("MEDIA_LOCAL_ROOT_DIR", t.TempDir())
	storageProvider = nil

	header := makeFileHeader(t, "file", "photo.png", content)
	result, err := UploadDocumentAsset(context.Background(), UploadAssetRequest{
		DocumentID: docID,
		UserID:     userID,
		FileHeader: header,
		Visibility: "private",
	})
	if err != nil {
		t.Fatalf("upload: %v", err)
	}
	if result.Asset.BlobID != blob.ID {
		t.Fatalf("expected revived blob id=%s, got %s", blob.ID, result.Asset.BlobID)
	}

	var gotBlob models.BlobObject
	if err := db.First(&gotBlob, "id = ?", blob.ID).Error; err != nil {
		t.Fatalf("load revived blob: %v", err)
	}
	if gotBlob.DeletedAt.Valid {
		t.Fatalf("expected deleted_at cleared after blob revive")
	}
	if gotBlob.Status != "ready" {
		t.Fatalf("expected blob status ready, got %s", gotBlob.Status)
	}
	if gotBlob.ThumbnailStatus != blobThumbnailStatusPending {
		t.Fatalf("expected thumbnail status reset to pending, got %s", gotBlob.ThumbnailStatus)
	}
	if gotBlob.ThumbnailObjectKey != "" {
		t.Fatalf("expected thumbnail object key cleared on revive, got %q", gotBlob.ThumbnailObjectKey)
	}

	var gcJob models.BlobGCJob
	if err := db.First(&gcJob, "blob_id = ?", blob.ID).Error; err != nil {
		t.Fatalf("load blob gc job: %v", err)
	}
	if gcJob.Status != "cancelled" {
		t.Fatalf("expected blob gc job cancelled, got %s", gcJob.Status)
	}
}

func TestUploadDocumentAsset_RevivesSoftDeletedAssetOnUniqueConflict(t *testing.T) {
	db := setupMediaTestDB(t)
	userID := uuid.New()
	docID := seedOwnedDocument(t, db, userID)

	t.Setenv("MEDIA_STORAGE_PROVIDER", "local")
	t.Setenv("MEDIA_LOCAL_ROOT_DIR", t.TempDir())
	storageProvider = nil

	headerA := makeFileHeader(t, "file", "photo.png", []byte("same-content"))
	first, err := UploadDocumentAsset(context.Background(), UploadAssetRequest{
		DocumentID: docID,
		UserID:     userID,
		FileHeader: headerA,
		Visibility: "private",
	})
	if err != nil {
		t.Fatalf("first upload: %v", err)
	}

	assetID := first.Asset.ID
	blobID := first.Asset.BlobID
	createdBy := first.Asset.CreatedBy

	// Soft-delete the asset row to simulate user "delete" from media library.
	if err := db.Model(&models.Asset{}).
		Where("id = ?", assetID).
		Updates(map[string]any{
			"status":          "deleted",
			"reference_count": 0,
		}).Error; err != nil {
		t.Fatalf("mark asset deleted: %v", err)
	}
	if err := db.Delete(&models.Asset{ID: assetID}).Error; err != nil {
		t.Fatalf("soft delete asset: %v", err)
	}

	// Create pending GC jobs; revive path should cancel them.
	if err := db.Create(&models.AssetGCJob{
		ID:       uuid.New(),
		AssetID:  assetID,
		JobType:  "delete",
		Status:   "pending",
		RunAfter: time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatalf("create asset gc job: %v", err)
	}
	if err := db.Create(&models.BlobGCJob{
		ID:       uuid.New(),
		BlobID:   blobID,
		JobType:  "delete",
		Status:   "pending",
		RunAfter: time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatalf("create blob gc job: %v", err)
	}

	headerB := makeFileHeader(t, "file", "duplicate.png", []byte("same-content"))
	second, err := UploadDocumentAsset(context.Background(), UploadAssetRequest{
		DocumentID: docID,
		UserID:     userID,
		FileHeader: headerB,
		Visibility: "private",
	})
	if err != nil {
		t.Fatalf("second upload: %v", err)
	}
	if second.Asset.ID != assetID {
		t.Fatalf("expected revived asset id=%s, got %s", assetID, second.Asset.ID)
	}

	var got models.Asset
	if err := db.First(&got, "id = ?", assetID).Error; err != nil {
		t.Fatalf("load revived asset: %v", err)
	}
	if got.Status != "ready" {
		t.Fatalf("expected revived asset status=ready, got %s", got.Status)
	}
	if got.CreatedBy != createdBy {
		t.Fatalf("expected created_by unchanged, got %s want %s", got.CreatedBy, createdBy)
	}
	if got.DeletedAt.Valid {
		t.Fatalf("expected deleted_at cleared after revive")
	}

	var assetJob models.AssetGCJob
	if err := db.First(&assetJob, "asset_id = ?", assetID).Error; err != nil {
		t.Fatalf("load asset gc job: %v", err)
	}
	if assetJob.Status != "cancelled" {
		t.Fatalf("expected asset gc job cancelled, got %s", assetJob.Status)
	}

	var blobJob models.BlobGCJob
	if err := db.First(&blobJob, "blob_id = ?", blobID).Error; err != nil {
		t.Fatalf("load blob gc job: %v", err)
	}
	if blobJob.Status != "cancelled" {
		t.Fatalf("expected blob gc job cancelled, got %s", blobJob.Status)
	}
}

func TestUploadDocumentAsset_AllowsSharedEditorAndUsesOwnerLibrary(t *testing.T) {
	db := setupMediaTestDB(t)
	ownerID := uuid.New()
	editorID := uuid.New()
	docID := seedOwnedDocument(t, db, ownerID)
	seedDocumentPermission(t, db, docID, editorID, ownerID, "editor")

	t.Setenv("MEDIA_STORAGE_PROVIDER", "local")
	t.Setenv("MEDIA_LOCAL_ROOT_DIR", t.TempDir())
	storageProvider = nil

	header := makeFileHeader(t, "file", "photo.png", []byte("shared-content"))
	result, err := UploadDocumentAsset(context.Background(), UploadAssetRequest{
		DocumentID: docID,
		UserID:     editorID,
		FileHeader: header,
		Visibility: "private",
	})
	if err != nil {
		t.Fatalf("upload by shared editor: %v", err)
	}
	if result.Asset.OwnerUserID != ownerID {
		t.Fatalf("expected asset owner to stay document owner, got %s", result.Asset.OwnerUserID)
	}
}

func TestResolveAccessibleAssetReadURL_AllowsSharedEditorBeforeAssetRefsSync(t *testing.T) {
	db := setupMediaTestDB(t)
	ownerID := uuid.New()
	editorID := uuid.New()
	docID := seedOwnedDocument(t, db, ownerID)
	seedDocumentPermission(t, db, docID, editorID, ownerID, "editor")

	blob := seedBlob(t, db, "owner/photo.png", "image/png", 12, "hash-read-url")
	asset := models.Asset{
		ID:          uuid.New(),
		OwnerUserID: ownerID,
		DocumentID:  &docID,
		BlobID:      blob.ID,
		Kind:        "image",
		Filename:    "photo.png",
		URL:         blob.URL,
		Visibility:  "private",
		Status:      "ready",
		CreatedBy:   editorID,
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}

	t.Setenv("MEDIA_STORAGE_PROVIDER", "local")
	t.Setenv("MEDIA_LOCAL_ROOT_DIR", t.TempDir())
	storageProvider = nil

	readURL, _, err := ResolveAccessibleAssetReadURL(context.Background(), "http://localhost:8080", editorID, asset.ID)
	if err != nil {
		t.Fatalf("resolve accessible asset read url for shared editor: %v", err)
	}
	if strings.TrimSpace(readURL) == "" {
		t.Fatal("expected non-empty read url")
	}
}

func TestIsUniqueConstraintError(t *testing.T) {
	if !isUniqueConstraintError(fmt.Errorf("UNIQUE constraint failed: blob_objects.sha256, blob_objects.size"), "blob_objects.sha256", "blob_objects.size") {
		t.Fatalf("expected sqlite unique constraint to match")
	}
	if !isUniqueConstraintError(fmt.Errorf("Error 1062: Duplicate entry 'x' for key 'idx_owner_blob_asset'"), "duplicate") {
		t.Fatalf("expected mysql duplicate entry to match")
	}
	if isUniqueConstraintError(fmt.Errorf("some other error"), "blob_objects.sha256") {
		t.Fatalf("did not expect unrelated error to match")
	}
	if isUniqueConstraintError(errors.New(strings.ToUpper("duplicate entry")), "blob_objects.sha256") {
		t.Fatalf("did not expect missing parts to match")
	}
}

func TestGetOwnedAssetReferences_ReturnsReferencingDocuments(t *testing.T) {
	db := setupMediaTestDB(t)
	userID := uuid.New()
	docID := seedOwnedDocument(t, db, userID)

	blob := seedBlob(t, db, "owner/photo.png", "image/png", 12, "hash-photo")
	asset := models.Asset{
		ID:             uuid.New(),
		OwnerUserID:    userID,
		DocumentID:     &docID,
		BlobID:         blob.ID,
		Filename:       "photo.png",
		Kind:           "image",
		URL:            blob.URL,
		Visibility:     "private",
		Status:         "ready",
		ReferenceCount: 1,
		CreatedBy:      userID,
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}
	if err := db.Create(&models.DocumentAssetRef{
		ID:          uuid.New(),
		DocumentID:  docID,
		AssetID:     asset.ID,
		OwnerUserID: userID,
		RefType:     "editor_content",
	}).Error; err != nil {
		t.Fatalf("create ref: %v", err)
	}

	result, err := GetOwnedAssetReferences(userID, asset.ID)
	if err != nil {
		t.Fatalf("get references: %v", err)
	}
	if result.ReferenceCount != 1 {
		t.Fatalf("expected referenceCount=1, got %d", result.ReferenceCount)
	}
	if len(result.Documents) != 1 || result.Documents[0].DocumentID != docID {
		t.Fatalf("unexpected documents: %+v", result.Documents)
	}
}

func TestDeleteOwnedUnusedAsset_DeletesStorageAndMarksDeleted(t *testing.T) {
	db := setupMediaTestDB(t)
	userID := uuid.New()
	docID := seedOwnedDocument(t, db, userID)

	rootDir := t.TempDir()
	t.Setenv("MEDIA_STORAGE_PROVIDER", "local")
	t.Setenv("MEDIA_LOCAL_ROOT_DIR", rootDir)
	storageProvider = nil

	objectKey := "owner/deletable.png"
	filePath := rootDir + string(os.PathSeparator) + "owner" + string(os.PathSeparator) + "deletable.png"
	if err := os.MkdirAll(rootDir+string(os.PathSeparator)+"owner", 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("data"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	blob := seedBlob(t, db, objectKey, "image/png", 4, "hash-delete")
	asset := models.Asset{
		ID:             uuid.New(),
		OwnerUserID:    userID,
		DocumentID:     &docID,
		BlobID:         blob.ID,
		Kind:           "image",
		Filename:       "deletable.png",
		URL:            blob.URL,
		Visibility:     "private",
		Status:         "pending_delete",
		ReferenceCount: 0,
		CreatedBy:      userID,
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}
	if err := db.Create(&models.AssetGCJob{
		ID:       uuid.New(),
		AssetID:  asset.ID,
		JobType:  "delete",
		Status:   "pending",
		RunAfter: asset.CreatedAt,
	}).Error; err != nil {
		t.Fatalf("create gc job: %v", err)
	}

	if err := DeleteOwnedUnusedAsset(context.Background(), userID, asset.ID); err != nil {
		t.Fatalf("delete unused asset: %v", err)
	}

	var got models.Asset
	if err := db.Unscoped().First(&got, "id = ?", asset.ID).Error; err != nil {
		t.Fatalf("load deleted asset: %v", err)
	}
	if got.Status != "deleted" || got.DeletedAt.Valid == false {
		t.Fatalf("expected deleted asset row, got status=%s deletedAt=%v", got.Status, got.DeletedAt.Valid)
	}

	var job models.AssetGCJob
	if err := db.First(&job, "asset_id = ?", asset.ID).Error; err != nil {
		t.Fatalf("load gc job: %v", err)
	}
	if job.Status != "cancelled" {
		t.Fatalf("expected gc job cancelled, got %s", job.Status)
	}

	var blobJob models.BlobGCJob
	if err := db.First(&blobJob, "blob_id = ?", blob.ID).Error; err != nil {
		t.Fatalf("load blob gc job: %v", err)
	}
	if blobJob.Status != "pending" {
		t.Fatalf("expected blob gc job pending, got %s", blobJob.Status)
	}
	if !blobJob.RunAfter.After(time.Now().Add(23 * time.Hour)) {
		t.Fatalf("expected delayed blob gc schedule, got %s", blobJob.RunAfter)
	}

	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("expected storage object to remain until blob gc, stat err=%v", err)
	}
}

func TestDeleteOwnedUnusedAsset_RejectsReferencedAsset(t *testing.T) {
	db := setupMediaTestDB(t)
	userID := uuid.New()
	docID := seedOwnedDocument(t, db, userID)

	blob := seedBlob(t, db, "owner/used.png", "image/png", 4, "hash-used")
	asset := models.Asset{
		ID:             uuid.New(),
		OwnerUserID:    userID,
		DocumentID:     &docID,
		BlobID:         blob.ID,
		Kind:           "image",
		Filename:       "used.png",
		URL:            blob.URL,
		Visibility:     "private",
		Status:         "ready",
		ReferenceCount: 1,
		CreatedBy:      userID,
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}
	if err := db.Create(&models.DocumentAssetRef{
		ID:          uuid.New(),
		DocumentID:  docID,
		AssetID:     asset.ID,
		OwnerUserID: userID,
		RefType:     "editor_content",
	}).Error; err != nil {
		t.Fatalf("create ref: %v", err)
	}

	if err := DeleteOwnedUnusedAsset(context.Background(), userID, asset.ID); err == nil || !errors.Is(err, ErrAssetStillReferenced) {
		t.Fatalf("expected referenced asset rejection, got %v", err)
	}
}

func TestListOwnedAssets_FiltersAndMarksDeletable(t *testing.T) {
	db := setupMediaTestDB(t)
	userID := uuid.New()
	otherUserID := uuid.New()
	docID := seedOwnedDocument(t, db, userID)

	blobCover := seedBlob(t, db, "owner/cover.png", "image/png", 10, "hash-cover")
	blobClip := seedBlob(t, db, "owner/clip.webm", "video/webm", 20, "hash-clip")
	blobOther := seedBlob(t, db, "other/other.png", "image/png", 30, "hash-other")
	assets := []models.Asset{
		{
			ID:             uuid.New(),
			OwnerUserID:    userID,
			DocumentID:     &docID,
			BlobID:         blobCover.ID,
			Kind:           "image",
			Filename:       "cover.png",
			URL:            blobCover.URL,
			Visibility:     "private",
			Status:         "ready",
			ReferenceCount: 1,
			CreatedBy:      userID,
		},
		{
			ID:             uuid.New(),
			OwnerUserID:    userID,
			DocumentID:     &docID,
			BlobID:         blobClip.ID,
			Kind:           "video",
			Filename:       "clip.webm",
			URL:            blobClip.URL,
			Visibility:     "private",
			Status:         "pending_delete",
			ReferenceCount: 0,
			CreatedBy:      userID,
		},
		{
			ID:             uuid.New(),
			OwnerUserID:    otherUserID,
			BlobID:         blobOther.ID,
			Kind:           "image",
			Filename:       "other.png",
			URL:            blobOther.URL,
			Visibility:     "private",
			Status:         "ready",
			ReferenceCount: 0,
			CreatedBy:      otherUserID,
		},
	}
	for _, asset := range assets {
		if err := db.Create(&asset).Error; err != nil {
			t.Fatalf("create asset %s: %v", asset.Filename, err)
		}
	}

	result, err := ListOwnedAssets(ListAssetsRequest{
		UserID: userID,
		Kind:   "video",
		Status: "pending_delete",
		Query:  "clip",
		Limit:  10,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("list assets: %v", err)
	}

	if len(result.Items) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(result.Items))
	}
	item := result.Items[0]
	if item.Filename != "clip.webm" || item.Kind != "video" || item.Status != "pending_delete" {
		t.Fatalf("unexpected listed asset: %+v", item)
	}
	if !item.Deletable || item.ReferenceCount != 0 {
		t.Fatalf("expected pending_delete asset to be deletable with ref=0, got %+v", item)
	}
	if result.Total != 1 || result.HasMore {
		t.Fatalf("expected total=1 and hasMore=false, got total=%d hasMore=%v", result.Total, result.HasMore)
	}
}

func TestListOwnedAssets_IncludesDeletedWhenRequested(t *testing.T) {
	db := setupMediaTestDB(t)
	userID := uuid.New()
	blob := seedBlob(t, db, "owner/deleted.png", "image/png", 10, "hash-deleted")
	asset := models.Asset{
		ID:             uuid.New(),
		OwnerUserID:    userID,
		BlobID:         blob.ID,
		Kind:           "image",
		Filename:       "deleted.png",
		URL:            blob.URL,
		Visibility:     "private",
		Status:         "deleted",
		ReferenceCount: 0,
		CreatedBy:      userID,
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}
	if err := db.Delete(&asset).Error; err != nil {
		t.Fatalf("soft delete asset: %v", err)
	}

	result, err := ListOwnedAssets(ListAssetsRequest{
		UserID: userID,
		Status: "deleted",
		Limit:  10,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("list deleted assets: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].Status != "deleted" {
		t.Fatalf("unexpected deleted asset list: %+v", result.Items)
	}
	if result.Items[0].Deletable {
		t.Fatalf("deleted asset should not be deletable again")
	}
}

func TestListSharedEditableAssets_ReturnsOnlyEditorScopedManagedAssets(t *testing.T) {
	db := setupMediaTestDB(t)
	ownerID := uuid.New()
	editorID := uuid.New()
	viewerID := uuid.New()
	docID := seedOwnedDocument(t, db, ownerID)
	seedDocumentPermission(t, db, docID, editorID, ownerID, "editor")
	seedDocumentPermission(t, db, docID, viewerID, ownerID, "viewer")

	blob := seedBlob(t, db, "owner/shared-query.png", "image/png", 31, "hash-shared-query")
	asset := models.Asset{
		ID:             uuid.New(),
		OwnerUserID:    ownerID,
		DocumentID:     &docID,
		BlobID:         blob.ID,
		Kind:           "image",
		Filename:       "shared-query.png",
		URL:            blob.URL,
		Visibility:     "private",
		Status:         "ready",
		ReferenceCount: 1,
		CreatedBy:      ownerID,
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}
	if err := db.Create(&models.DocumentAssetRef{
		ID:          uuid.New(),
		DocumentID:  docID,
		AssetID:     asset.ID,
		OwnerUserID: ownerID,
		RefType:     "editor_content",
	}).Error; err != nil {
		t.Fatalf("create ref: %v", err)
	}

	editorResult, err := ListSharedEditableAssets(ListAssetsRequest{
		UserID: editorID,
		Limit:  20,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("list shared editor assets: %v", err)
	}
	if len(editorResult.Items) != 1 || editorResult.Items[0].ID != asset.ID {
		t.Fatalf("unexpected editor shared assets: %+v", editorResult.Items)
	}
	if editorResult.Items[0].DocumentCount != 1 {
		t.Fatalf("expected one shared document, got %+v", editorResult.Items[0])
	}

	viewerResult, err := ListSharedEditableAssets(ListAssetsRequest{
		UserID: viewerID,
		Limit:  20,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("list shared viewer assets: %v", err)
	}
	if len(viewerResult.Items) != 0 {
		t.Fatalf("viewer should not get shared editable assets, got %+v", viewerResult.Items)
	}
}

func TestListOwnedAssets_PaginatesLikeWorkspaceList(t *testing.T) {
	db := setupMediaTestDB(t)
	userID := uuid.New()

	blobA := seedBlob(t, db, "owner/a.png", "image/png", 1, "hash-a-page")
	blobB := seedBlob(t, db, "owner/b.png", "image/png", 1, "hash-b-page")
	blobC := seedBlob(t, db, "owner/c.png", "image/png", 1, "hash-c-page")
	assets := []models.Asset{
		{
			ID:          uuid.New(),
			OwnerUserID: userID,
			BlobID:      blobA.ID,
			Kind:        "image",
			Filename:    "a.png",
			URL:         blobA.URL,
			Visibility:  "private",
			Status:      "ready",
			CreatedBy:   userID,
		},
		{
			ID:          uuid.New(),
			OwnerUserID: userID,
			BlobID:      blobB.ID,
			Kind:        "image",
			Filename:    "b.png",
			URL:         blobB.URL,
			Visibility:  "private",
			Status:      "ready",
			CreatedBy:   userID,
		},
		{
			ID:          uuid.New(),
			OwnerUserID: userID,
			BlobID:      blobC.ID,
			Kind:        "image",
			Filename:    "c.png",
			URL:         blobC.URL,
			Visibility:  "private",
			Status:      "ready",
			CreatedBy:   userID,
		},
	}
	for _, asset := range assets {
		if err := db.Create(&asset).Error; err != nil {
			t.Fatalf("create asset %s: %v", asset.Filename, err)
		}
	}

	result, err := ListOwnedAssets(ListAssetsRequest{
		UserID: userID,
		Limit:  2,
		Offset: 1,
	})
	if err != nil {
		t.Fatalf("list assets with pagination: %v", err)
	}

	if len(result.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(result.Items))
	}
	if result.Total != 3 {
		t.Fatalf("expected total=3, got %d", result.Total)
	}
	if result.HasMore {
		t.Fatalf("expected hasMore=false at offset 1 limit 2, got true")
	}
}
