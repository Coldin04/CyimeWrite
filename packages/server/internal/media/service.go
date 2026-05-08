package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"mime/multipart"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/acl"
	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"

	_ "image/gif"
)

var (
	thumbnailInlineInit sync.Once
	thumbnailInlineSem  chan struct{}
)

type UploadAssetRequest struct {
	DocumentID uuid.UUID
	UserID     uuid.UUID
	FileHeader *multipart.FileHeader
	Visibility string
}

type UploadAssetResult struct {
	Asset *models.Asset
}

type AssetReferenceDocument struct {
	DocumentID uuid.UUID `json:"documentId"`
	Title      string    `json:"title"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type AssetReferencesResult struct {
	AssetID        uuid.UUID                `json:"assetId"`
	ReferenceCount int                      `json:"referenceCount"`
	Documents      []AssetReferenceDocument `json:"documents"`
}

type ListAssetsRequest struct {
	UserID uuid.UUID
	Kind   string
	Status string
	Query  string
	Limit  int
	Offset int
}

type AssetListItem struct {
	ID                 uuid.UUID  `json:"id"`
	Kind               string     `json:"kind"`
	Filename           string     `json:"filename"`
	MimeType           string     `json:"mimeType"`
	FileSize           int64      `json:"fileSize"`
	ThumbnailURL       string     `json:"thumbnailUrl,omitempty"`
	ObjectKey          string     `json:"-"`
	ThumbnailObjectKey string     `json:"-"`
	ThumbnailMimeType  string     `json:"-"`
	ThumbnailStatus    string     `json:"-"`
	Visibility         string     `json:"visibility"`
	Status             string     `json:"status"`
	ReferenceCount     int        `json:"referenceCount"`
	Deletable          bool       `json:"deletable"`
	DocumentID         *uuid.UUID `json:"documentId,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

type ListAssetsResult struct {
	Items   []AssetListItem `json:"items"`
	HasMore bool            `json:"hasMore"`
	Total   int64           `json:"total"`
}

type SharedAssetDocument struct {
	DocumentID uuid.UUID `json:"documentId"`
	Title      string    `json:"title"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type SharedAssetListItem struct {
	ID                 uuid.UUID             `json:"id"`
	Kind               string                `json:"kind"`
	Filename           string                `json:"filename"`
	MimeType           string                `json:"mimeType"`
	FileSize           int64                 `json:"fileSize"`
	ThumbnailURL       string                `json:"thumbnailUrl,omitempty"`
	ObjectKey          string                `json:"-"`
	ThumbnailObjectKey string                `json:"-"`
	ThumbnailMimeType  string                `json:"-"`
	ThumbnailStatus    string                `json:"-"`
	Visibility         string                `json:"visibility"`
	OwnerUserID        uuid.UUID             `json:"ownerUserId"`
	ReferenceCount     int                   `json:"referenceCount"`
	DocumentCount      int                   `json:"documentCount"`
	Documents          []SharedAssetDocument `json:"documents"`
	CreatedAt          time.Time             `json:"createdAt"`
	UpdatedAt          time.Time             `json:"updatedAt"`
}

type ListSharedAssetsResult struct {
	Items   []SharedAssetListItem `json:"items"`
	HasMore bool                  `json:"hasMore"`
	Total   int64                 `json:"total"`
}

const mediaRefTypeEditorContent = "editor_content"
const blobThumbnailStatusReady = "ready"
const blobThumbnailStatusPending = "pending"
const blobThumbnailStatusFailed = "failed"
const blobThumbnailStatusSkipped = "skipped"
const blobThumbnailMaxEdge = 320
const blobThumbnailMaxSourceEdge = 12000
const blobThumbnailMaxSourcePixels int64 = 50_000_000

type assetBlobRecord struct {
	Asset models.Asset
	Blob  models.BlobObject
}

var storageProvider StorageProvider

var allowedAssetMimeTypes = map[string]struct{}{
	"image/png":  {},
	"image/jpeg": {},
	"image/webp": {},
	"image/gif":  {},
	"video/mp4":  {},
	"video/webm": {},
}

var allowedAssetExtensions = map[string]string{
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".webp": "image/webp",
	".gif":  "image/gif",
	".mp4":  "video/mp4",
	".webm": "video/webm",
}

func initStorageProvider() error {
	if storageProvider != nil {
		return nil
	}
	provider, err := newStorageProviderFromEnv()
	if err != nil {
		return err
	}
	storageProvider = provider
	return nil
}

func GetOwnedAsset(userID, assetID uuid.UUID) (*models.Asset, error) {
	var asset models.Asset
	result := database.DB.Where("id = ? AND owner_user_id = ? AND deleted_at IS NULL", assetID, userID).First(&asset)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrAssetNotFoundOrForbidden
		}
		return nil, result.Error
	}
	return &asset, nil
}

func GetAccessibleAsset(userID, assetID uuid.UUID) (*models.Asset, error) {
	asset, err := GetAssetByID(assetID)
	if err != nil {
		return nil, err
	}
	if asset.OwnerUserID == userID {
		return asset, nil
	}
	if asset.DocumentID != nil {
		if _, err := acl.CanReadDocument(database.DB, userID, *asset.DocumentID); err == nil {
			return asset, nil
		}
	}

	var count int64
	if err := database.DB.
		Table("document_asset_refs AS refs").
		Joins("JOIN documents AS d ON d.id = refs.document_id AND d.deleted_at IS NULL").
		Joins("LEFT JOIN document_permissions AS perms ON perms.document_id = refs.document_id AND perms.user_id = ? AND perms.deleted_at IS NULL", userID).
		Where("refs.asset_id = ? AND refs.ref_type = ? AND refs.deleted_at IS NULL", assetID, "editor_content").
		Where("d.owner_user_id = ? OR perms.role IN ?", userID, acl.AllowedRolesForAction(acl.ActionRead)).
		Count(&count).Error; err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, ErrAssetNotFoundOrForbidden
	}
	return asset, nil
}

func ResolveAccessibleAssetReadURL(ctx context.Context, baseURL string, userID, assetID uuid.UUID) (string, time.Time, error) {
	asset, err := GetAccessibleAsset(userID, assetID)
	if err != nil {
		return "", time.Time{}, err
	}

	record, err := getAssetBlobByID(assetID)
	if err != nil {
		return "", time.Time{}, err
	}
	blob := record.Blob

	return resolveAssetObjectURL(ctx, baseURL, userID, *asset, blob.ObjectKey, blob.MimeType, "/content")
}

func ResolveAccessibleAssetThumbnailURL(ctx context.Context, baseURL string, userID, assetID uuid.UUID) (string, time.Time, error) {
	asset, err := GetAccessibleAsset(userID, assetID)
	if err != nil {
		return "", time.Time{}, err
	}

	record, err := getAssetBlobByID(assetID)
	if err != nil {
		return "", time.Time{}, err
	}
	blob := record.Blob
	if blob.ThumbnailStatus != blobThumbnailStatusReady || strings.TrimSpace(blob.ThumbnailObjectKey) == "" {
		return resolveAssetObjectURL(ctx, baseURL, userID, *asset, blob.ObjectKey, blob.MimeType, "/content")
	}
	return resolveAssetObjectURL(ctx, baseURL, userID, *asset, blob.ThumbnailObjectKey, blob.ThumbnailMimeType, "/thumbnail")
}

func resolveAssetObjectURL(ctx context.Context, baseURL string, _ uuid.UUID, asset models.Asset, objectKey string, contentType string, pathSuffix string) (string, time.Time, error) {
	if err := initStorageProvider(); err != nil {
		return "", time.Time{}, err
	}

	if provider, ok := storageProvider.(PresignedURLProvider); ok && strings.TrimSpace(objectKey) != "" {
		presigned, err := provider.PresignGetObject(ctx, PresignGetObjectInput{
			ObjectKey:   objectKey,
			ExpiresIn:   assetResolveTTLFromEnv(),
			ContentType: contentType,
		})
		if err == nil {
			return presigned.URL, presigned.ExpiresAt, nil
		}
		log.Printf("[media.resolve] presign failed provider=%s asset=%s path=%s fallback=protected_api err=%v", storageProvider.ProviderName(), asset.ID, pathSuffix, err)
	}

	readURL := strings.TrimRight(baseURL, "/") + "/api/v1/media/assets/" + asset.ID.String() + pathSuffix
	if asset.Visibility == "public" {
		return readURL, time.Time{}, nil
	}
	// Fallback path for providers without presign support: serve a stable API URL.
	// Access control is enforced by JWT middleware + per-request ACL checks in handlers.
	return readURL, time.Time{}, nil
}

func ListOwnedAssets(req ListAssetsRequest) (*ListAssetsResult, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	query := database.DB.Model(&models.Asset{}).
		Where("owner_user_id = ?", req.UserID)

	status := strings.TrimSpace(req.Status)
	switch status {
	case "", "all":
	case "ready", "pending_delete", "deleted", "failed":
		if status == "deleted" {
			query = query.Unscoped().Where("owner_user_id = ? AND status = ?", req.UserID, status)
		} else {
			query = query.Where("status = ?", status)
		}
	default:
		return nil, ErrInvalidAssetStatus
	}

	kind := strings.TrimSpace(req.Kind)
	switch kind {
	case "", "all":
	case "image", "video", "file":
		query = query.Where("kind = ?", kind)
	default:
		return nil, ErrInvalidAssetKind
	}

	if q := strings.TrimSpace(req.Query); q != "" {
		like := "%" + q + "%"
		query = query.Where("filename LIKE ?", like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	var assets []models.Asset
	if err := query.
		Order("created_at desc").
		Limit(limit).
		Offset(offset).
		Find(&assets).Error; err != nil {
		return nil, err
	}

	blobMap, err := loadBlobMap(extractBlobIDs(assets))
	if err != nil {
		return nil, err
	}

	items := make([]AssetListItem, 0, len(assets))
	for _, asset := range assets {
		blob, ok := blobMap[asset.BlobID]
		if !ok {
			return nil, fmt.Errorf("blob not found for asset %s", asset.ID)
		}
		items = append(items, AssetListItem{
			ID:                 asset.ID,
			Kind:               asset.Kind,
			Filename:           asset.Filename,
			MimeType:           blob.MimeType,
			FileSize:           blob.Size,
			ObjectKey:          blob.ObjectKey,
			ThumbnailObjectKey: blob.ThumbnailObjectKey,
			ThumbnailMimeType:  blob.ThumbnailMimeType,
			ThumbnailStatus:    blob.ThumbnailStatus,
			Visibility:         asset.Visibility,
			Status:             asset.Status,
			ReferenceCount:     asset.ReferenceCount,
			Deletable:          asset.ReferenceCount == 0 && asset.Status != "deleted",
			DocumentID:         asset.DocumentID,
			CreatedAt:          asset.CreatedAt,
			UpdatedAt:          asset.UpdatedAt,
		})
	}

	return &ListAssetsResult{
		Items:   items,
		HasMore: int64(offset+len(items)) < total,
		Total:   total,
	}, nil
}

func ListSharedEditableAssets(req ListAssetsRequest) (*ListSharedAssetsResult, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	baseQuery := database.DB.
		Table("document_asset_refs AS refs").
		Joins("JOIN document_permissions AS perms ON perms.document_id = refs.document_id AND perms.deleted_at IS NULL").
		Joins("JOIN assets AS a ON a.id = refs.asset_id AND a.deleted_at IS NULL").
		Joins("JOIN documents AS d ON d.id = refs.document_id AND d.deleted_at IS NULL").
		Where("perms.user_id = ? AND perms.role IN ? AND refs.ref_type = ? AND refs.deleted_at IS NULL", req.UserID, acl.AllowedRolesForAction(acl.ActionEdit), mediaRefTypeEditorContent).
		Where("d.owner_user_id <> ?", req.UserID)

	kind := strings.TrimSpace(req.Kind)
	switch kind {
	case "", "all":
	case "image", "video", "file":
		baseQuery = baseQuery.Where("a.kind = ?", kind)
	default:
		return nil, ErrInvalidAssetKind
	}

	if q := strings.TrimSpace(req.Query); q != "" {
		like := "%" + q + "%"
		baseQuery = baseQuery.Where("a.filename LIKE ?", like)
	}

	status := strings.TrimSpace(req.Status)
	switch status {
	case "", "all":
	case "ready", "pending_delete", "failed":
		baseQuery = baseQuery.Where("a.status = ?", status)
	default:
		return nil, ErrInvalidAssetStatus
	}

	var total int64
	if err := baseQuery.
		Select("COUNT(DISTINCT a.id)").
		Count(&total).Error; err != nil {
		return nil, err
	}

	type assetRow struct {
		AssetID        uuid.UUID
		OwnerUserID    uuid.UUID
		BlobID         uuid.UUID
		Kind           string
		Filename       string
		Visibility     string
		ReferenceCount int
		CreatedAt      time.Time
		UpdatedAt      time.Time
	}

	var rows []assetRow
	if err := baseQuery.
		Select("DISTINCT a.id AS asset_id", "a.owner_user_id", "a.blob_id", "a.kind", "a.filename", "a.visibility", "a.reference_count", "a.created_at", "a.updated_at").
		Order("a.updated_at desc").
		Limit(limit).
		Offset(offset).
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	assetIDs := make([]uuid.UUID, 0, len(rows))
	blobIDs := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		assetIDs = append(assetIDs, row.AssetID)
		blobIDs = append(blobIDs, row.BlobID)
	}

	blobMap, err := loadBlobMap(blobIDs)
	if err != nil {
		return nil, err
	}

	type docJoinRow struct {
		AssetID    uuid.UUID
		DocumentID uuid.UUID
		Title      string
		UpdatedAt  time.Time
	}
	var docRows []docJoinRow
	if len(assetIDs) > 0 {
		if err := database.DB.
			Table("document_asset_refs AS refs").
			Select("refs.asset_id", "d.id AS document_id", "d.title", "d.updated_at").
			Joins("JOIN document_permissions AS perms ON perms.document_id = refs.document_id AND perms.deleted_at IS NULL").
			Joins("JOIN documents AS d ON d.id = refs.document_id AND d.deleted_at IS NULL").
			Where("perms.user_id = ? AND perms.role IN ? AND refs.asset_id IN ? AND refs.ref_type = ? AND refs.deleted_at IS NULL", req.UserID, acl.AllowedRolesForAction(acl.ActionEdit), assetIDs, mediaRefTypeEditorContent).
			Order("d.updated_at desc").
			Scan(&docRows).Error; err != nil {
			return nil, err
		}
	}

	docMap := make(map[uuid.UUID][]SharedAssetDocument, len(assetIDs))
	docSeen := make(map[uuid.UUID]map[uuid.UUID]struct{}, len(assetIDs))
	for _, row := range docRows {
		if _, ok := docSeen[row.AssetID]; !ok {
			docSeen[row.AssetID] = map[uuid.UUID]struct{}{}
		}
		if _, exists := docSeen[row.AssetID][row.DocumentID]; exists {
			continue
		}
		docSeen[row.AssetID][row.DocumentID] = struct{}{}
		docMap[row.AssetID] = append(docMap[row.AssetID], SharedAssetDocument{
			DocumentID: row.DocumentID,
			Title:      row.Title,
			UpdatedAt:  row.UpdatedAt,
		})
	}

	items := make([]SharedAssetListItem, 0, len(rows))
	for _, row := range rows {
		blob, ok := blobMap[row.BlobID]
		if !ok {
			return nil, fmt.Errorf("blob not found for asset %s", row.AssetID)
		}
		documents := docMap[row.AssetID]
		items = append(items, SharedAssetListItem{
			ID:                 row.AssetID,
			Kind:               row.Kind,
			Filename:           row.Filename,
			MimeType:           blob.MimeType,
			FileSize:           blob.Size,
			ObjectKey:          blob.ObjectKey,
			ThumbnailObjectKey: blob.ThumbnailObjectKey,
			ThumbnailMimeType:  blob.ThumbnailMimeType,
			ThumbnailStatus:    blob.ThumbnailStatus,
			Visibility:         row.Visibility,
			OwnerUserID:        row.OwnerUserID,
			ReferenceCount:     row.ReferenceCount,
			DocumentCount:      len(documents),
			Documents:          documents,
			CreatedAt:          row.CreatedAt,
			UpdatedAt:          row.UpdatedAt,
		})
	}

	return &ListSharedAssetsResult{
		Items:   items,
		HasMore: int64(offset+len(items)) < total,
		Total:   total,
	}, nil
}

func GetAssetByID(assetID uuid.UUID) (*models.Asset, error) {
	var asset models.Asset
	result := database.DB.Where("id = ? AND deleted_at IS NULL", assetID).First(&asset)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, errors.New("资源不存在")
		}
		return nil, result.Error
	}
	return &asset, nil
}

func getAssetBlobByID(assetID uuid.UUID) (*assetBlobRecord, error) {
	var asset models.Asset
	if err := database.DB.Where("id = ? AND deleted_at IS NULL", assetID).First(&asset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("资源不存在")
		}
		return nil, err
	}

	var blob models.BlobObject
	if err := database.DB.Where("id = ? AND deleted_at IS NULL", asset.BlobID).First(&blob).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("资源物理对象不存在")
		}
		return nil, err
	}

	return &assetBlobRecord{Asset: asset, Blob: blob}, nil
}

func UploadDocumentAsset(ctx context.Context, req UploadAssetRequest) (*UploadAssetResult, error) {
	log.Printf("[media.upload.service] validating document=%s user=%s", req.DocumentID, req.UserID)
	if req.FileHeader == nil {
		return nil, ErrFileRequired
	}
	contentType, ok := normalizeAllowedContentType(
		strings.TrimSpace(req.FileHeader.Header.Get("Content-Type")),
		req.FileHeader.Filename,
	)
	if !ok {
		return nil, &UnsupportedFileTypeError{ContentType: contentType}
	}
	if err := ValidateVisibility(req.Visibility); err != nil {
		return nil, err
	}
	if err := initStorageProvider(); err != nil {
		return nil, err
	}
	document, err := ensureEditableDocument(req.UserID, req.DocumentID)
	if err != nil {
		return nil, err
	}

	file, err := req.FileHeader.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()

	log.Printf("[media.upload.service] reading file filename=%q size=%d", req.FileHeader.Filename, req.FileHeader.Size)
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	fileHash := computeFileHash(fileBytes)
	log.Printf("[media.upload.service] file hash computed hash=%s size=%d", fileHash, req.FileHeader.Size)

	kind := detectKind(req.FileHeader.Header.Get("Content-Type"))
	visibility := req.Visibility
	if visibility == "" {
		visibility = "private"
	}

	blob, err := findBlobByHash(fileHash, req.FileHeader.Size)
	if err != nil {
		return nil, err
	}
	if blob == nil {
		objectKey := buildObjectKey(req.UserID, req.FileHeader.Filename)
		log.Printf("[media.upload.service] putting object key=%q provider=%T", objectKey, storageProvider)
		uploadResult, putErr := storageProvider.PutObject(ctx, PutObjectInput{
			ObjectKey:   objectKey,
			ContentType: contentType,
			Body:        bytes.NewReader(fileBytes),
		})
		if putErr != nil {
			return nil, putErr
		}
		log.Printf("[media.upload.service] object stored bucket=%q provider=%q", uploadResult.Bucket, uploadResult.Provider)

		blob = &models.BlobObject{
			ID:              uuid.New(),
			SHA256:          fileHash,
			Size:            req.FileHeader.Size,
			MimeType:        contentType,
			StorageProvider: uploadResult.Provider,
			Bucket:          uploadResult.Bucket,
			ObjectKey:       objectKey,
			URL:             uploadResult.URL,
			Status:          "ready",
		}
		if err := database.DB.Create(blob).Error; err != nil {
			if !isUniqueConstraintError(err, "blob_objects.sha256", "blob_objects.size") {
				return nil, err
			}
			log.Printf("[media.upload.service] blob create raced for hash=%s size=%d, checking existing rows", fileHash, req.FileHeader.Size)
			blob, err = findBlobByHash(fileHash, req.FileHeader.Size)
			if err != nil {
				return nil, err
			}
			if blob != nil {
				// Active blob already exists; cleanup the duplicate object we just uploaded.
				if deleteErr := storageProvider.DeleteObject(ctx, objectKey); deleteErr != nil {
					log.Printf("[media.upload.service] cleanup duplicate object failed key=%q err=%v", objectKey, deleteErr)
				}
			} else {
				// Unique conflict without an active row usually means a soft-deleted blob exists.
				// Revive that row and repoint it to the newly uploaded object.
				var restored *models.BlobObject
				restoreErr := database.DB.Transaction(func(tx *gorm.DB) error {
					var softDeleted models.BlobObject
					unscopedErr := tx.
						Unscoped().
						Where("sha256 = ? AND size = ?", fileHash, req.FileHeader.Size).
						First(&softDeleted).Error
					if unscopedErr != nil {
						return unscopedErr
					}

					now := time.Now()
					if err := tx.Unscoped().Model(&models.BlobObject{}).
						Where("id = ? AND sha256 = ? AND size = ?", softDeleted.ID, fileHash, req.FileHeader.Size).
						Updates(map[string]any{
							"deleted_at":           nil,
							"status":               "ready",
							"mime_type":            contentType,
							"storage_provider":     uploadResult.Provider,
							"bucket":               uploadResult.Bucket,
							"object_key":           objectKey,
							"thumbnail_object_key": "",
							"thumbnail_mime_type":  "",
							"thumbnail_size":       0,
							"thumbnail_width":      nil,
							"thumbnail_height":     nil,
							"thumbnail_status":     blobThumbnailStatusPending,
							"url":                  uploadResult.URL,
							"updated_at":           now,
						}).Error; err != nil {
						return err
					}

					if err := tx.Model(&models.BlobGCJob{}).
						Where("blob_id = ? AND status = ?", softDeleted.ID, "pending").
						Updates(map[string]any{
							"status":     "cancelled",
							"updated_at": now,
						}).Error; err != nil {
						return err
					}

					var active models.BlobObject
					if err := tx.Where("id = ? AND deleted_at IS NULL", softDeleted.ID).First(&active).Error; err != nil {
						return err
					}
					restored = &active
					return nil
				})
				if restoreErr != nil {
					if errors.Is(restoreErr, gorm.ErrRecordNotFound) {
						if deleteErr := storageProvider.DeleteObject(ctx, objectKey); deleteErr != nil {
							log.Printf("[media.upload.service] cleanup duplicate object after missing blob row failed key=%q err=%v", objectKey, deleteErr)
						}
						return nil, errors.New("blob create conflicted but no existing blob found")
					}
					return nil, restoreErr
				}
				if restored == nil {
					return nil, errors.New("blob create conflicted but restore result missing")
				}
				blob = restored
			}
		}
	}

	if existing, err := findDeduplicatedAsset(document.OwnerUserID, blob.ID); err != nil {
		return nil, err
	} else if existing != nil {
		maybeScheduleBlobThumbnail(*blob, fileBytes)
		log.Printf("[media.upload.service] deduplicated existing asset=%s", existing.ID)
		return &UploadAssetResult{Asset: existing}, nil
	}

	asset := &models.Asset{
		ID:             uuid.New(),
		OwnerUserID:    document.OwnerUserID,
		DocumentID:     &req.DocumentID,
		BlobID:         blob.ID,
		Kind:           kind,
		Filename:       req.FileHeader.Filename,
		URL:            blob.URL,
		Visibility:     visibility,
		Status:         "ready",
		ReferenceCount: 0,
		CreatedBy:      req.UserID,
	}

	if err := database.DB.Create(asset).Error; err != nil {
		if !isUniqueConstraintError(err, "assets.owner_user_id", "assets.blob_id") {
			return nil, err
		}
		log.Printf("[media.upload.service] asset create raced owner=%s blob=%s, reusing existing row", document.OwnerUserID, blob.ID)
		existing, findErr := findDeduplicatedAsset(document.OwnerUserID, blob.ID)
		if findErr != nil {
			return nil, findErr
		}
		if existing != nil {
			maybeScheduleBlobThumbnail(*blob, fileBytes)
			return &UploadAssetResult{Asset: existing}, nil
		}

		// The unique constraint was hit, but we did not find an active (non-deleted) asset.
		// Look for a soft-deleted row and restore it instead of failing the upload.
		var restored *models.Asset
		restoreErr := database.DB.Transaction(func(tx *gorm.DB) error {
			var softDeleted models.Asset
			unscopedErr := tx.
				Unscoped().
				Where("owner_user_id = ? AND blob_id = ?", document.OwnerUserID, blob.ID).
				First(&softDeleted).Error
			if unscopedErr != nil {
				return unscopedErr
			}

			now := time.Now()
			if !softDeleted.DeletedAt.Valid {
				// Row already active (likely a race); still cancel pending GC jobs defensively.
				if err := tx.Model(&models.AssetGCJob{}).
					Where("asset_id = ? AND status = ?", softDeleted.ID, "pending").
					Updates(map[string]any{
						"status":     "cancelled",
						"updated_at": now,
					}).Error; err != nil {
					return err
				}
				if err := ensurePendingBlobDeleteJob(tx, blob.ID, now); err != nil {
					return err
				}
				restored = &softDeleted
				return nil
			}

			if err := tx.Unscoped().Model(&models.Asset{}).
				Where("id = ? AND owner_user_id = ? AND blob_id = ?", softDeleted.ID, document.OwnerUserID, blob.ID).
				Updates(map[string]any{
					"deleted_at":  nil,
					"status":      "ready",
					"document_id": req.DocumentID,
					"kind":        kind,
					"filename":    req.FileHeader.Filename,
					"url":         blob.URL,
					"visibility":  visibility,
					"updated_at":  now,
				}).Error; err != nil {
				return err
			}

			if err := tx.Model(&models.AssetGCJob{}).
				Where("asset_id = ? AND status = ?", softDeleted.ID, "pending").
				Updates(map[string]any{
					"status":     "cancelled",
					"updated_at": now,
				}).Error; err != nil {
				return err
			}

			// If there is a pending delete job for this blob, cancel it now that the blob is referenced again.
			if err := ensurePendingBlobDeleteJob(tx, blob.ID, now); err != nil {
				return err
			}

			var active models.Asset
			if err := tx.Where("id = ? AND deleted_at IS NULL", softDeleted.ID).First(&active).Error; err != nil {
				return err
			}
			restored = &active
			return nil
		})
		if restoreErr != nil {
			if errors.Is(restoreErr, gorm.ErrRecordNotFound) {
				return nil, errors.New("asset create conflicted but no existing asset found")
			}
			return nil, restoreErr
		}

		if restored == nil {
			return nil, errors.New("asset create conflicted but restore result missing")
		}
		maybeScheduleBlobThumbnail(*blob, fileBytes)
		return &UploadAssetResult{Asset: restored}, nil
	}
	maybeScheduleBlobThumbnail(*blob, fileBytes)
	log.Printf("[media.upload.service] asset row created asset=%s blob=%s", asset.ID, blob.ID)
	return &UploadAssetResult{Asset: asset}, nil
}

func normalizeAllowedContentType(contentType string, filename string) (string, bool) {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	if _, ok := allowedAssetMimeTypes[contentType]; ok {
		return contentType, true
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if fallbackType, ok := allowedAssetExtensions[ext]; ok {
		return fallbackType, true
	}

	if contentType != "" {
		return contentType, false
	}
	return ext, false
}

func ensureEditableDocument(userID, documentID uuid.UUID) (*models.Document, error) {
	document, err := acl.CanEditDocument(database.DB, userID, documentID)
	if err != nil {
		return nil, ErrDocumentNotAccessible
	}
	return document, nil
}

func computeFileHash(fileBytes []byte) string {
	hasher := sha256.New()
	_, _ = hasher.Write(fileBytes)
	return hex.EncodeToString(hasher.Sum(nil))
}

func findBlobByHash(fileHash string, fileSize int64) (*models.BlobObject, error) {
	var blob models.BlobObject
	result := database.DB.
		Where("sha256 = ? AND size = ? AND deleted_at IS NULL", fileHash, fileSize).
		First(&blob)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &blob, nil
}

func findDeduplicatedAsset(userID uuid.UUID, blobID uuid.UUID) (*models.Asset, error) {
	var asset models.Asset
	result := database.DB.
		Where("owner_user_id = ? AND blob_id = ? AND deleted_at IS NULL", userID, blobID).
		First(&asset)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, result.Error
	}
	return &asset, nil
}

func isUniqueConstraintError(err error, parts ...string) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "unique constraint failed") && !strings.Contains(msg, "duplicate entry") {
		return false
	}
	for _, part := range parts {
		if !strings.Contains(msg, strings.ToLower(part)) {
			return false
		}
	}
	return true
}

func assetResolveTTLFromEnv() time.Duration {
	ttlSeconds, err := strconv.Atoi(strings.TrimSpace(os.Getenv("MEDIA_ASSET_RESOLVE_TTL_SECONDS")))
	if err != nil || ttlSeconds <= 0 {
		ttlSeconds, err = strconv.Atoi(strings.TrimSpace(os.Getenv("MEDIA_SIGN_TTL_SECONDS")))
	}
	if err != nil || ttlSeconds <= 0 {
		ttlSeconds = defaultSignTTLSeconds
	}
	return time.Duration(ttlSeconds) * time.Second
}

func GetOwnedAssetReferences(userID, assetID uuid.UUID) (*AssetReferencesResult, error) {
	asset, err := GetOwnedAsset(userID, assetID)
	if err != nil {
		return nil, err
	}

	var refs []models.DocumentAssetRef
	if err := database.DB.
		Where("owner_user_id = ? AND asset_id = ? AND ref_type = ?", userID, assetID, "editor_content").
		Find(&refs).Error; err != nil {
		return nil, err
	}

	documents := make([]AssetReferenceDocument, 0, len(refs))
	if len(refs) > 0 {
		documentIDs := make([]uuid.UUID, 0, len(refs))
		for _, ref := range refs {
			documentIDs = append(documentIDs, ref.DocumentID)
		}

		var docs []models.Document
		if err := database.DB.
			Where("owner_user_id = ? AND id IN ? AND deleted_at IS NULL", userID, documentIDs).
			Find(&docs).Error; err != nil {
			return nil, err
		}

		docByID := make(map[uuid.UUID]models.Document, len(docs))
		for _, doc := range docs {
			docByID[doc.ID] = doc
		}

		for _, ref := range refs {
			doc, ok := docByID[ref.DocumentID]
			if !ok {
				continue
			}
			documents = append(documents, AssetReferenceDocument{
				DocumentID: doc.ID,
				Title:      doc.Title,
				UpdatedAt:  doc.UpdatedAt,
			})
		}
	}

	sort.Slice(documents, func(i, j int) bool {
		return documents[i].UpdatedAt.After(documents[j].UpdatedAt)
	})

	return &AssetReferencesResult{
		AssetID:        asset.ID,
		ReferenceCount: len(documents),
		Documents:      documents,
	}, nil
}

func DeleteOwnedUnusedAsset(ctx context.Context, userID, assetID uuid.UUID) error {
	record, err := getOwnedAssetBlob(userID, assetID)
	if err != nil {
		return err
	}
	_ = ctx
	asset := &record.Asset
	if asset.ReferenceCount > 0 {
		return ErrAssetStillReferenced
	}
	if asset.Status == "deleted" {
		return ErrAssetAlreadyDeleted
	}

	now := time.Now()
	return database.DB.Transaction(func(tx *gorm.DB) error {
		var refCount int64
		if err := tx.Model(&models.DocumentAssetRef{}).
			Where("asset_id = ? AND ref_type = ?", assetID, "editor_content").
			Count(&refCount).Error; err != nil {
			return err
		}
		if refCount > 0 {
			return ErrAssetStillReferenced
		}

		if err := tx.Model(&models.Asset{}).
			Where("id = ? AND owner_user_id = ? AND deleted_at IS NULL", assetID, userID).
			Updates(map[string]any{
				"status":          "deleted",
				"reference_count": 0,
				"updated_at":      now,
				"deleted_at":      now,
			}).Error; err != nil {
			return err
		}

		if err := tx.Model(&models.AssetGCJob{}).
			Where("asset_id = ? AND status = ?", assetID, "pending").
			Updates(map[string]any{
				"status":     "cancelled",
				"updated_at": now,
			}).Error; err != nil {
			return err
		}

		return ensurePendingBlobDeleteJob(tx, asset.BlobID, now)
	})
}

func getOwnedAssetBlob(userID, assetID uuid.UUID) (*assetBlobRecord, error) {
	asset, err := GetOwnedAsset(userID, assetID)
	if err != nil {
		return nil, err
	}

	var blob models.BlobObject
	if err := database.DB.Where("id = ? AND deleted_at IS NULL", asset.BlobID).First(&blob).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("资源物理对象不存在")
		}
		return nil, err
	}
	return &assetBlobRecord{Asset: *asset, Blob: blob}, nil
}

func extractBlobIDs(assets []models.Asset) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(assets))
	seen := make(map[uuid.UUID]struct{}, len(assets))
	for _, asset := range assets {
		if _, ok := seen[asset.BlobID]; ok {
			continue
		}
		seen[asset.BlobID] = struct{}{}
		ids = append(ids, asset.BlobID)
	}
	return ids
}

func loadBlobMap(blobIDs []uuid.UUID) (map[uuid.UUID]models.BlobObject, error) {
	if len(blobIDs) == 0 {
		return map[uuid.UUID]models.BlobObject{}, nil
	}
	var blobs []models.BlobObject
	if err := database.DB.Where("id IN ? AND deleted_at IS NULL", blobIDs).Find(&blobs).Error; err != nil {
		return nil, err
	}
	blobMap := make(map[uuid.UUID]models.BlobObject, len(blobs))
	for _, blob := range blobs {
		blobMap[blob.ID] = blob
	}
	return blobMap, nil
}

func ensurePendingBlobDeleteJob(tx *gorm.DB, blobID uuid.UUID, now time.Time) error {
	var activeAssetCount int64
	if err := tx.Model(&models.Asset{}).
		Where("blob_id = ? AND deleted_at IS NULL", blobID).
		Count(&activeAssetCount).Error; err != nil {
		return err
	}
	if activeAssetCount > 0 {
		return tx.Model(&models.BlobGCJob{}).
			Where("blob_id = ? AND job_type = ? AND status = ?", blobID, "delete", "pending").
			Updates(map[string]any{
				"status":     "cancelled",
				"updated_at": now,
			}).Error
	}

	var existing models.BlobGCJob
	err := tx.
		Where("blob_id = ? AND job_type = ? AND status = ?", blobID, "delete", "pending").
		First(&existing).Error
	switch {
	case err == nil:
		delay := blobDeleteDelayFromEnv()
		return tx.Model(&models.BlobGCJob{}).
			Where("id = ?", existing.ID).
			Updates(map[string]any{
				"run_after":  now.Add(delay),
				"updated_at": now,
			}).Error
	case errors.Is(err, gorm.ErrRecordNotFound):
		delay := blobDeleteDelayFromEnv()
		return tx.Create(&models.BlobGCJob{
			ID:       uuid.New(),
			BlobID:   blobID,
			JobType:  "delete",
			Status:   "pending",
			RunAfter: now.Add(delay),
		}).Error
	default:
		return err
	}
}

func buildObjectKey(userID uuid.UUID, filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	day := time.Now().UTC().Format("20060102")
	return fmt.Sprintf("%s/%s/%s%s", userID.String(), day, uuid.NewString(), ext)
}

func buildThumbnailObjectKey(objectKey string, ext string) string {
	base := strings.TrimSuffix(objectKey, filepath.Ext(objectKey))
	return base + "__thumb_sm" + ext
}

func detectKind(contentType string) string {
	if strings.HasPrefix(contentType, "image/") {
		return "image"
	}
	if strings.HasPrefix(contentType, "video/") {
		return "video"
	}
	return "file"
}

func maybeScheduleBlobThumbnail(blob models.BlobObject, sourceBytes []byte) {
	if !shouldGenerateBlobThumbnail(blob) {
		return
	}

	// Avoid unbounded goroutines and large extra allocations.
	// If the inline queue is saturated, let the periodic reconcile pass backfill thumbnails later.
	thumbnailInlineInit.Do(func() {
		thumbnailInlineSem = make(chan struct{}, thumbnailInlineConcurrencyFromEnv())
	})

	// Defensive: never try to thumbnail absurdly large inputs.
	maxBytes := thumbnailSourceMaxBytesFromEnv()
	if blob.Size > maxBytes || int64(len(sourceBytes)) > maxBytes {
		_ = markBlobThumbnailState(blob.ID, blobThumbnailStatusSkipped, map[string]any{
			"thumbnail_object_key": "",
			"thumbnail_mime_type":  "",
			"thumbnail_size":       0,
			"thumbnail_width":      nil,
			"thumbnail_height":     nil,
		})
		return
	}

	select {
	case thumbnailInlineSem <- struct{}{}:
		go func() {
			defer func() { <-thumbnailInlineSem }()
			if err := generateAndStoreBlobThumbnail(context.Background(), blob, sourceBytes); err != nil {
				log.Printf("[media.thumb] blob=%s failed: %v", blob.ID, err)
			}
		}()
	default:
	}
}

func shouldGenerateBlobThumbnail(blob models.BlobObject) bool {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(blob.MimeType)), "image/") {
		return false
	}
	status := strings.ToLower(strings.TrimSpace(blob.ThumbnailStatus))
	if status == blobThumbnailStatusFailed || status == blobThumbnailStatusSkipped {
		return false
	}
	if status == blobThumbnailStatusReady && strings.TrimSpace(blob.ThumbnailObjectKey) != "" {
		return false
	}
	return true
}

func thumbnailInlineConcurrencyFromEnv() int {
	raw := strings.TrimSpace(os.Getenv("MEDIA_THUMBNAIL_INLINE_CONCURRENCY"))
	if raw == "" {
		return 2
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return 2
	}
	if n > 16 {
		return 16
	}
	return n
}

func thumbnailSourceMaxBytesFromEnv() int64 {
	raw := strings.TrimSpace(os.Getenv("MEDIA_THUMBNAIL_SOURCE_MAX_BYTES"))
	if raw == "" {
		return 25 << 20 // 25 MiB
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 {
		return 25 << 20
	}
	// Avoid ridiculous configs.
	if n < (1 << 20) {
		return 1 << 20
	}
	return n
}

func generateAndStoreBlobThumbnail(ctx context.Context, blob models.BlobObject, sourceBytes []byte) error {
	if err := initStorageProvider(); err != nil {
		return err
	}

	thumbData, thumbMimeType, width, height, err := buildBlobThumbnail(sourceBytes, blob.MimeType)
	if err != nil {
		status := blobThumbnailStatusFailed
		if strings.Contains(strings.ToLower(err.Error()), "unsupported image") {
			status = blobThumbnailStatusSkipped
		}
		_ = markBlobThumbnailState(blob.ID, status, map[string]any{
			"thumbnail_object_key": "",
			"thumbnail_mime_type":  "",
			"thumbnail_size":       0,
			"thumbnail_width":      nil,
			"thumbnail_height":     nil,
		})
		return err
	}
	if len(thumbData) == 0 {
		return markBlobThumbnailState(blob.ID, blobThumbnailStatusSkipped, map[string]any{
			"thumbnail_object_key": "",
			"thumbnail_mime_type":  "",
			"thumbnail_size":       0,
			"thumbnail_width":      nil,
			"thumbnail_height":     nil,
		})
	}

	ext := ".jpg"
	if thumbMimeType == "image/png" {
		ext = ".png"
	}
	objectKey := buildThumbnailObjectKey(blob.ObjectKey, ext)
	if _, err := storageProvider.PutObject(ctx, PutObjectInput{
		ObjectKey:   objectKey,
		ContentType: thumbMimeType,
		Body:        bytes.NewReader(thumbData),
	}); err != nil {
		_ = markBlobThumbnailState(blob.ID, blobThumbnailStatusFailed, nil)
		return err
	}

	return database.DB.Model(&models.BlobObject{}).
		Where("id = ? AND deleted_at IS NULL", blob.ID).
		Updates(map[string]any{
			"thumbnail_object_key": objectKey,
			"thumbnail_mime_type":  thumbMimeType,
			"thumbnail_size":       int64(len(thumbData)),
			"thumbnail_width":      width,
			"thumbnail_height":     height,
			"thumbnail_status":     blobThumbnailStatusReady,
		}).Error
}

func markBlobThumbnailState(blobID uuid.UUID, status string, extra map[string]any) error {
	updates := map[string]any{
		"thumbnail_status": status,
	}
	for key, value := range extra {
		updates[key] = value
	}
	return database.DB.Model(&models.BlobObject{}).
		Where("id = ? AND deleted_at IS NULL", blobID).
		Updates(updates).Error
}

func buildBlobThumbnail(sourceBytes []byte, mimeType string) ([]byte, string, *int, *int, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(sourceBytes))
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("unsupported image for thumbnail: %w", err)
	}
	srcW := config.Width
	srcH := config.Height
	if err := validateBlobThumbnailSourceSize(srcW, srcH); err != nil {
		return nil, "", nil, nil, err
	}

	img, _, err := image.Decode(bytes.NewReader(sourceBytes))
	if err != nil {
		return nil, "", nil, nil, fmt.Errorf("unsupported image for thumbnail: %w", err)
	}

	bounds := img.Bounds()
	srcW = bounds.Dx()
	srcH = bounds.Dy()
	if err := validateBlobThumbnailSourceSize(srcW, srcH); err != nil {
		return nil, "", nil, nil, err
	}
	if srcW <= blobThumbnailMaxEdge && srcH <= blobThumbnailMaxEdge {
		return nil, "", nil, nil, nil
	}

	dstW, dstH := fitWithin(srcW, srcH, blobThumbnailMaxEdge)
	thumb := resizeNearest(img, dstW, dstH)

	var encoded bytes.Buffer
	thumbMime := "image/jpeg"
	if imageHasAlpha(thumb) || strings.EqualFold(mimeType, "image/png") {
		thumbMime = "image/png"
		if err := png.Encode(&encoded, thumb); err != nil {
			return nil, "", nil, nil, err
		}
	} else {
		if err := jpeg.Encode(&encoded, thumb, &jpeg.Options{Quality: 80}); err != nil {
			return nil, "", nil, nil, err
		}
	}

	width := dstW
	height := dstH
	return encoded.Bytes(), thumbMime, &width, &height, nil
}

func validateBlobThumbnailSourceSize(width, height int) error {
	if width <= 0 || height <= 0 {
		return errors.New("invalid image size")
	}
	if width > blobThumbnailMaxSourceEdge || height > blobThumbnailMaxSourceEdge {
		return fmt.Errorf("unsupported image for thumbnail: dimensions %dx%d exceed maximum edge %d", width, height, blobThumbnailMaxSourceEdge)
	}
	pixels := int64(width) * int64(height)
	if pixels > blobThumbnailMaxSourcePixels {
		return fmt.Errorf("unsupported image for thumbnail: dimensions %dx%d exceed maximum pixels %d", width, height, blobThumbnailMaxSourcePixels)
	}
	return nil
}

func fitWithin(srcW, srcH, maxEdge int) (int, int) {
	if srcW <= maxEdge && srcH <= maxEdge {
		return srcW, srcH
	}
	if srcW >= srcH {
		dstW := maxEdge
		dstH := max(1, srcH*maxEdge/srcW)
		return dstW, dstH
	}
	dstH := maxEdge
	dstW := max(1, srcW*maxEdge/srcH)
	return dstW, dstH
}

func resizeNearest(src image.Image, dstW, dstH int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	srcBounds := src.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	for y := 0; y < dstH; y++ {
		srcY := srcBounds.Min.Y + y*srcH/dstH
		for x := 0; x < dstW; x++ {
			srcX := srcBounds.Min.X + x*srcW/dstW
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}

func imageHasAlpha(img image.Image) bool {
	bounds := img.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			if alpha := color.NRGBAModel.Convert(img.At(x, y)).(color.NRGBA).A; alpha < 255 {
				return true
			}
		}
	}
	return false
}
