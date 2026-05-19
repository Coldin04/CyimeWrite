package media

import (
	"context"
	"errors"
	"log"
	"strings"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

type AssetURLResponse struct {
	AssetID   uuid.UUID `json:"assetId"`
	URL       string    `json:"url"`
	ExpiresAt string    `json:"expiresAt"`
}

type UploadAssetResponse struct {
	ID              uuid.UUID `json:"id"`
	AssetID         uuid.UUID `json:"assetId"`
	DocumentID      uuid.UUID `json:"documentId"`
	Kind            string    `json:"kind"`
	Filename        string    `json:"filename"`
	MimeType        string    `json:"mimeType"`
	FileSize        int64     `json:"fileSize"`
	StorageProvider string    `json:"storageProvider"`
	ObjectKey       string    `json:"objectKey"`
	URL             string    `json:"url"`
	ExpiresAt       string    `json:"expiresAt,omitempty"`
	Visibility      string    `json:"visibility"`
}

type UploadDocumentImageResponse struct {
	TargetID  string     `json:"targetId"`
	Mode      string     `json:"mode"`
	URL       string     `json:"url"`
	AssetID   *uuid.UUID `json:"assetId,omitempty"`
	ExpiresAt string     `json:"expiresAt,omitempty"`
}

type AssetReferencesResponse struct {
	AssetID        uuid.UUID                `json:"assetId"`
	ReferenceCount int                      `json:"referenceCount"`
	Documents      []AssetReferenceDocument `json:"documents"`
}

type AssetListResponse struct {
	Items   []AssetListItem `json:"items"`
	HasMore bool            `json:"hasMore"`
	Total   int64           `json:"total"`
}

type SharedAssetListResponse struct {
	Items   []SharedAssetListItem `json:"items"`
	HasMore bool                  `json:"hasMore"`
	Total   int64                 `json:"total"`
}

type ResolveAssetURLsRequest struct {
	AssetIDs []string `json:"assetIds"`
}

type ResolveAssetURLItem struct {
	AssetID   uuid.UUID `json:"assetId"`
	URL       string    `json:"url,omitempty"`
	ExpiresAt string    `json:"expiresAt,omitempty"`
	Error     string    `json:"error,omitempty"`
	Code      string    `json:"code,omitempty"`
}

type ResolveAssetURLsResponse struct {
	Items []ResolveAssetURLItem `json:"items"`
}

func GetAssetURLHandler(c *fiber.Ctx) error {
	userIDStr, ok := c.Locals("userId").(string)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error:   "Unauthorized",
			Message: "Invalid user context",
		})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid user id",
		})
	}

	assetID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid asset id",
		})
	}

	resolvedURL, expiresAt, err := ResolveAccessibleAssetReadURL(c.UserContext(), c.BaseURL(), userID, assetID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "Not Found",
			Message: err.Error(),
		})
	}
	formattedExpiresAt := ""
	if !expiresAt.IsZero() {
		formattedExpiresAt = expiresAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	return c.JSON(AssetURLResponse{
		AssetID:   assetID,
		URL:       resolvedURL,
		ExpiresAt: formattedExpiresAt,
	})
}

func ResolveAssetURLsHandler(c *fiber.Ctx) error {
	userIDStr, ok := c.Locals("userId").(string)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error:   "Unauthorized",
			Message: "Invalid user context",
		})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid user id",
		})
	}

	var req ResolveAssetURLsRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid request body",
		})
	}

	if len(req.AssetIDs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "assetIds is required",
		})
	}

	if len(req.AssetIDs) > 200 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "assetIds exceeds max size 200",
		})
	}

	items := make([]ResolveAssetURLItem, 0, len(req.AssetIDs))
	for _, rawID := range req.AssetIDs {
		assetID, parseErr := uuid.Parse(rawID)
		if parseErr != nil {
			items = append(items, ResolveAssetURLItem{
				Error:   "Invalid asset id",
				Code:    "INVALID_ASSET_ID",
				AssetID: uuid.Nil,
			})
			continue
		}

		resolvedURL, expiresAt, resolveErr := ResolveAccessibleAssetReadURL(c.UserContext(), c.BaseURL(), userID, assetID)
		if resolveErr != nil {
			items = append(items, ResolveAssetURLItem{
				AssetID: assetID,
				Error:   "Asset not found or access denied",
				Code:    "ASSET_NOT_FOUND_OR_FORBIDDEN",
			})
			continue
		}

		item := ResolveAssetURLItem{
			AssetID: assetID,
			URL:     resolvedURL,
		}
		if !expiresAt.IsZero() {
			item.ExpiresAt = expiresAt.UTC().Format("2006-01-02T15:04:05Z")
		}
		items = append(items, item)
	}

	return c.JSON(ResolveAssetURLsResponse{Items: items})
}

func ListAssetsHandler(c *fiber.Ctx) error {
	userIDStr, ok := c.Locals("userId").(string)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error:   "Unauthorized",
			Message: "Invalid user context",
		})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid user id",
		})
	}

	result, err := ListOwnedAssets(ListAssetsRequest{
		UserID: userID,
		Kind:   c.Query("kind"),
		Status: c.Query("status"),
		Query:  c.Query("q"),
		Limit:  c.QueryInt("limit", 20),
		Offset: c.QueryInt("offset", 0),
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidAssetStatus), errors.Is(err, ErrInvalidAssetKind):
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Bad Request",
				Message: err.Error(),
			})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "Internal Server Error",
				Message: err.Error(),
			})
		}
	}

	for i := range result.Items {
		thumbnailURL, _, thumbErr := resolveListThumbnailURL(c.UserContext(), c.BaseURL(), userID, result.Items[i].ID, result.Items[i].Visibility, result.Items[i].ObjectKey, result.Items[i].MimeType, result.Items[i].ThumbnailObjectKey, result.Items[i].ThumbnailMimeType, result.Items[i].ThumbnailStatus)
		if thumbErr == nil {
			result.Items[i].ThumbnailURL = thumbnailURL
		}
	}

	return c.JSON(AssetListResponse{
		Items:   result.Items,
		HasMore: result.HasMore,
		Total:   result.Total,
	})
}

func ListSharedAssetsHandler(c *fiber.Ctx) error {
	userIDStr, ok := c.Locals("userId").(string)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error:   "Unauthorized",
			Message: "Invalid user context",
		})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid user id",
		})
	}

	result, err := ListSharedEditableAssets(ListAssetsRequest{
		UserID: userID,
		Kind:   c.Query("kind"),
		Status: c.Query("status"),
		Query:  c.Query("q"),
		Limit:  c.QueryInt("limit", 20),
		Offset: c.QueryInt("offset", 0),
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidAssetStatus), errors.Is(err, ErrInvalidAssetKind):
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Bad Request",
				Message: err.Error(),
			})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "Internal Server Error",
				Message: err.Error(),
			})
		}
	}

	for i := range result.Items {
		thumbnailURL, _, thumbErr := resolveListThumbnailURL(c.UserContext(), c.BaseURL(), userID, result.Items[i].ID, result.Items[i].Visibility, result.Items[i].ObjectKey, result.Items[i].MimeType, result.Items[i].ThumbnailObjectKey, result.Items[i].ThumbnailMimeType, result.Items[i].ThumbnailStatus)
		if thumbErr == nil {
			result.Items[i].ThumbnailURL = thumbnailURL
		}
	}

	return c.JSON(SharedAssetListResponse{
		Items:   result.Items,
		HasMore: result.HasMore,
		Total:   result.Total,
	})
}

func resolveListThumbnailURL(ctx context.Context, baseURL string, userID uuid.UUID, assetID uuid.UUID, visibility, objectKey, mimeType, thumbnailObjectKey, thumbnailMimeType, thumbnailStatus string) (string, time.Time, error) {
	asset := models.Asset{
		ID:         assetID,
		Visibility: visibility,
	}
	if thumbnailStatus == blobThumbnailStatusReady && thumbnailObjectKey != "" {
		return resolveAssetObjectURL(ctx, baseURL, userID, asset, thumbnailObjectKey, thumbnailMimeType, "/thumbnail")
	}
	return resolveAssetObjectURL(ctx, baseURL, userID, asset, objectKey, mimeType, "/content")
}

func GetAssetContentHandler(c *fiber.Ctx) error {
	userIDStr, ok := c.Locals("userId").(string)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error:   "Unauthorized",
			Message: "Invalid user context",
		})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error:   "Unauthorized",
			Message: "Invalid user id",
		})
	}

	assetID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid asset id",
		})
	}

	record, err := getAssetBlobByID(assetID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "Not Found",
			Message: err.Error(),
		})
	}
	asset := record.Asset
	blob := record.Blob

	if asset.Visibility != "public" {
		// Private media requires document/media ACL check per request.
		if _, err := GetAccessibleAsset(userID, asset.ID); err != nil {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "Not Found",
				Message: err.Error(),
			})
		}
	}

	if err := initStorageProvider(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Internal Server Error",
			Message: err.Error(),
		})
	}

	obj, err := storageProvider.GetObject(context.Background(), blob.ObjectKey)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(ErrorResponse{
			Error:   "Media Upstream Error",
			Message: err.Error(),
		})
	}
	// NOTE: do NOT `defer obj.Body.Close()` — fasthttp reads from the
	// stream *after* the handler returns, and will call Close() on the
	// stream itself once it has finished writing the response. A deferred
	// close here fires too early and the reader is dead by the time
	// fasthttp actually serialises the body.

	contentType := blob.MimeType
	if contentType == "" {
		contentType = obj.ContentType
	}
	c.Set("Content-Type", contentType)
	c.Set("Cache-Control", "private, max-age=60")

	// Stream the object body straight to the response. The previous
	// implementation did io.ReadAll(obj.Body) then c.Send(data), which held
	// the full blob (up to 25 MiB for thumbnails, arbitrary for videos) in
	// memory per in-flight request. SendStream hands the reader to fasthttp
	// which writes chunked-transfer output without buffering.
	return c.SendStream(obj.Body)
}

func GetAssetThumbnailHandler(c *fiber.Ctx) error {
	userIDStr, ok := c.Locals("userId").(string)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error:   "Unauthorized",
			Message: "Invalid user context",
		})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error:   "Unauthorized",
			Message: "Invalid user id",
		})
	}

	assetID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid asset id",
		})
	}

	record, err := getAssetBlobByID(assetID)
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "Not Found",
			Message: err.Error(),
		})
	}
	asset := record.Asset
	blob := record.Blob

	if asset.Visibility != "public" {
		// Private media requires document/media ACL check per request.
		if _, err := GetAccessibleAsset(userID, asset.ID); err != nil {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "Not Found",
				Message: err.Error(),
			})
		}
	}

	if err := initStorageProvider(); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Internal Server Error",
			Message: err.Error(),
		})
	}

	objectKey := blob.ObjectKey
	contentType := blob.MimeType
	if blob.ThumbnailStatus == blobThumbnailStatusReady && blob.ThumbnailObjectKey != "" {
		objectKey = blob.ThumbnailObjectKey
		if blob.ThumbnailMimeType != "" {
			contentType = blob.ThumbnailMimeType
		}
	}

	obj, err := storageProvider.GetObject(context.Background(), objectKey)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(ErrorResponse{
			Error:   "Media Upstream Error",
			Message: err.Error(),
		})
	}
	// No defer Close here — fasthttp closes the stream after flushing.
	// See GetAssetContentHandler.

	if contentType == "" {
		contentType = obj.ContentType
	}
	c.Set("Content-Type", contentType)
	c.Set("Cache-Control", "private, max-age=60")

	// See GetAssetContentHandler for the rationale on SendStream.
	return c.SendStream(obj.Body)
}

func GetAssetReferencesHandler(c *fiber.Ctx) error {
	userIDStr, ok := c.Locals("userId").(string)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error:   "Unauthorized",
			Message: "Invalid user context",
		})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid user id",
		})
	}

	assetID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid asset id",
		})
	}

	result, err := GetOwnedAssetReferences(userID, assetID)
	if err != nil {
		if errors.Is(err, ErrAssetNotFoundOrForbidden) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "Not Found",
				Message: err.Error(),
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Internal Server Error",
			Message: err.Error(),
		})
	}

	return c.JSON(AssetReferencesResponse{
		AssetID:        result.AssetID,
		ReferenceCount: result.ReferenceCount,
		Documents:      result.Documents,
	})
}

func DeleteAssetHandler(c *fiber.Ctx) error {
	userIDStr, ok := c.Locals("userId").(string)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error:   "Unauthorized",
			Message: "Invalid user context",
		})
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid user id",
		})
	}

	assetID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid asset id",
		})
	}

	if err := DeleteOwnedUnusedAsset(context.Background(), userID, assetID); err != nil {
		switch {
		case errors.Is(err, ErrAssetNotFoundOrForbidden):
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "Not Found",
				Message: err.Error(),
			})
		case errors.Is(err, ErrAssetStillReferenced), errors.Is(err, ErrAssetAlreadyDeleted):
			return c.Status(fiber.StatusConflict).JSON(ErrorResponse{
				Error:   "Conflict",
				Message: err.Error(),
			})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "Internal Server Error",
				Message: err.Error(),
			})
		}
	}

	return c.SendStatus(fiber.StatusNoContent)
}

func ValidateVisibility(visibility string) error {
	switch visibility {
	case "", "private", "public":
		return nil
	default:
		return ErrInvalidVisibility
	}
}

// UploadDocumentAssetHandler handles POST /api/v1/edit/documents/:id/assets
func UploadDocumentAssetHandler(c *fiber.Ctx) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[media.upload] panic: %v", r)
			_ = c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "Upload Failed",
				Message: "upload handler panic",
			})
		}
	}()

	userIDStr, ok := c.Locals("userId").(string)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error:   "Unauthorized",
			Message: "Invalid user context",
		})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid user id",
		})
	}
	documentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid document id",
		})
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "file is required",
		})
	}

	visibility := c.FormValue("visibility")
	log.Printf("[media.upload] start document=%s user=%s filename=%q size=%d visibility=%q", documentID, userID, fileHeader.Filename, fileHeader.Size, visibility)
	result, err := UploadDocumentAsset(context.Background(), UploadAssetRequest{
		DocumentID: documentID,
		UserID:     userID,
		FileHeader: fileHeader,
		Visibility: visibility,
	})
	if err != nil {
		log.Printf("[media.upload] failed document=%s user=%s filename=%q err=%v", documentID, userID, fileHeader.Filename, err)
		status := fiber.StatusInternalServerError
		switch {
		case errors.Is(err, ErrDocumentNotAccessible), errors.Is(err, ErrFileRequired), errors.Is(err, ErrInvalidVisibility):
			status = fiber.StatusBadRequest
		default:
			if errors.Is(err, context.Canceled) {
				status = fiber.StatusRequestTimeout
			}
			var unsupported *UnsupportedFileTypeError
			if errors.As(err, &unsupported) {
				status = fiber.StatusBadRequest
			}
		}
		return c.Status(status).JSON(ErrorResponse{
			Error:   "Upload Failed",
			Message: err.Error(),
		})
	}

	asset := result.Asset
	record, err := getAssetBlobByID(asset.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Internal Server Error",
			Message: err.Error(),
		})
	}
	blob := record.Blob
	docID := documentID
	if asset.DocumentID != nil {
		docID = *asset.DocumentID
	}

	readURL, expiresAtAtTime, err := ResolveAccessibleAssetReadURL(c.UserContext(), c.BaseURL(), userID, asset.ID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Internal Server Error",
			Message: err.Error(),
		})
	}
	expiresAt := ""
	if !expiresAtAtTime.IsZero() {
		expiresAt = expiresAtAtTime.UTC().Format("2006-01-02T15:04:05Z")
	}

	log.Printf("[media.upload] success asset=%s provider=%s objectKey=%q", asset.ID, blob.StorageProvider, blob.ObjectKey)
	return c.Status(fiber.StatusCreated).JSON(UploadAssetResponse{
		ID:              asset.ID,
		AssetID:         asset.ID,
		DocumentID:      docID,
		Kind:            asset.Kind,
		Filename:        asset.Filename,
		MimeType:        blob.MimeType,
		FileSize:        blob.Size,
		StorageProvider: blob.StorageProvider,
		ObjectKey:       blob.ObjectKey,
		URL:             readURL,
		ExpiresAt:       expiresAt,
		Visibility:      asset.Visibility,
	})
}

// UploadDocumentImageHandler handles POST /api/v1/edit/documents/:id/paste-image
func UploadDocumentImageHandler(c *fiber.Ctx) error {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[media.document-image] panic: %v", r)
			_ = c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "Upload Failed",
				Code:    "DOCUMENT_IMAGE_UPLOAD_FAILED",
				Message: "document image upload handler panic",
			})
		}
	}()

	userIDStr, ok := c.Locals("userId").(string)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
			Error:   "Unauthorized",
			Message: "Invalid user context",
		})
	}
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Code:    "DOCUMENT_IMAGE_INVALID_USER_ID",
			Message: "Invalid user id",
		})
	}
	documentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Code:    "DOCUMENT_IMAGE_INVALID_DOCUMENT_ID",
			Message: "Invalid document id",
		})
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Code:    "DOCUMENT_IMAGE_FILE_REQUIRED",
			Message: "file is required",
		})
	}

	log.Printf("[media.document-image] start document=%s user=%s filename=%q size=%d target=%q", documentID, userID, fileHeader.Filename, fileHeader.Size, c.FormValue("targetId"))
	result, err := UploadDocumentImage(c.UserContext(), UploadDocumentImageRequest{
		DocumentID: documentID,
		UserID:     userID,
		FileHeader: fileHeader,
		TargetID:   c.FormValue("targetId"),
	})
	if err != nil {
		status := fiber.StatusInternalServerError
		code := "DOCUMENT_IMAGE_UPLOAD_FAILED"
		var docErr *DocumentImageError
		var unsupported *UnsupportedFileTypeError
		switch {
		case errors.Is(err, ErrDocumentNotAccessible):
			status = fiber.StatusForbidden
			code = "DOCUMENT_IMAGE_FORBIDDEN"
		case errors.Is(err, ErrFileRequired):
			status = fiber.StatusBadRequest
			code = "DOCUMENT_IMAGE_FILE_REQUIRED"
		case errors.As(err, &docErr):
			switch docErr.Code {
			case DocumentImageErrUnsupportedTarget, DocumentImageErrProviderNotFound:
				status = fiber.StatusConflict
				code = "DOCUMENT_IMAGE_TARGET_NOT_SUPPORTED"
			case DocumentImageErrProviderNotReady, DocumentImageErrProviderConfig:
				status = fiber.StatusBadRequest
				code = "DOCUMENT_IMAGE_PROVIDER_NOT_CONFIGURED"
			case DocumentImageErrProviderUploadFail:
				status = fiber.StatusBadGateway
				code = "DOCUMENT_IMAGE_PROVIDER_UPLOAD_FAILED"
			case DocumentImageErrFileTooLarge:
				status = fiber.StatusRequestEntityTooLarge
				code = "DOCUMENT_IMAGE_FILE_TOO_LARGE"
			}
		case errors.Is(err, context.Canceled):
			status = fiber.StatusRequestTimeout
			code = "DOCUMENT_IMAGE_UPLOAD_TIMEOUT"
		case errors.As(err, &unsupported):
			status = fiber.StatusBadRequest
			code = "DOCUMENT_IMAGE_UNSUPPORTED_FILE_TYPE"
		case strings.Contains(err.Error(), "request body too large") || strings.Contains(err.Error(), "413"):
			status = fiber.StatusRequestEntityTooLarge
			code = "DOCUMENT_IMAGE_FILE_TOO_LARGE"
		}
		log.Printf("[media.document-image] failed document=%s user=%s filename=%q target=%q status=%d code=%s err=%v", documentID, userID, fileHeader.Filename, c.FormValue("targetId"), status, code, err)
		return c.Status(status).JSON(ErrorResponse{
			Error:   "Upload Failed",
			Code:    code,
			Message: err.Error(),
		})
	}

	response := UploadDocumentImageResponse{
		TargetID: result.TargetID,
		Mode:     result.Mode,
		URL:      result.URL,
		AssetID:  result.AssetID,
	}

	if result.Mode == documentImageModeManagedAsset && result.AssetID != nil {
		resolvedURL, expiresAt, err := ResolveAccessibleAssetReadURL(c.UserContext(), c.BaseURL(), userID, *result.AssetID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "Internal Server Error",
				Message: err.Error(),
			})
		}
		response.URL = resolvedURL
		if !expiresAt.IsZero() {
			response.ExpiresAt = expiresAt.UTC().Format("2006-01-02T15:04:05Z")
		}
	}

	log.Printf("[media.document-image] success document=%s user=%s target=%s mode=%s", documentID, userID, result.TargetID, result.Mode)
	return c.Status(fiber.StatusCreated).JSON(response)
}
