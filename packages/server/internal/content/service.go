package content

import (
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/acl"
	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const defaultContentJSON = `{"type":"doc","content":[{"type":"paragraph"}]}`

const MaxContentJSONBytes = 2 * 1024 * 1024
const maxWorkspaceStorageBytesPerUser = 50 * 1024 * 1024

const (
	editorContentRefType = "editor_content"
	assetStatusReady     = "ready"
	assetStatusPending   = "pending_delete"
	assetDeleteDelay     = 24 * time.Hour
)

var assetContentPathPattern = regexp.MustCompile(`/api/v1/media/assets/([0-9a-fA-F-]{36})/content(?:\?.*)?$`)

// GetContentResult represents the current content of a document.
type GetContentResult struct {
	ID             uuid.UUID       `json:"id"`
	DocumentID     uuid.UUID       `json:"documentId"`
	ContentJSON    json.RawMessage `json:"contentJson"`
	PlainText      string          `json:"plainText"`
	ContentVersion int64           `json:"contentVersion"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

// UpdateContentResult represents the result of updating document content.
type UpdateContentResult struct {
	Success        bool      `json:"success"`
	ContentVersion int64     `json:"contentVersion"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// DocumentBodyPatch carries optional extra fields that should be written
// alongside canonical content during a single transactional save.
type DocumentBodyPatch struct {
	YjsState       *string
	YjsStateVector *string
	YjsVersion     *int64
}

// GetContent retrieves the current content of a document.
func GetContent(userID uuid.UUID, documentID uuid.UUID) (*GetContentResult, error) {
	if _, err := acl.CanReadDocument(database.DB, userID, documentID); err != nil {
		return nil, ErrDocumentNotFoundOrUnauthorized
	}

	var content models.DocumentBody
	result := database.DB.Where("document_id = ?", documentID).First(&content)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return nil, ErrDocumentContentNotFound
		}
		return nil, result.Error
	}

	return &GetContentResult{
		ID:             content.ID,
		DocumentID:     content.DocumentID,
		ContentJSON:    json.RawMessage(content.ContentJSON),
		PlainText:      content.PlainText,
		ContentVersion: content.ContentVersion,
		CreatedAt:      content.CreatedAt,
		UpdatedAt:      content.UpdatedAt,
	}, nil
}

// UpdateContent updates the current content of a document in place.
func UpdateContent(userID uuid.UUID, documentID uuid.UUID, contentJSONRaw []byte) (*UpdateContentResult, error) {
	var result *UpdateContentResult
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		document, err := acl.CanEditDocument(tx, userID, documentID)
		if err != nil {
			return ErrDocumentNotFoundOrUnauthorized
		}

		result, err = PersistCanonicalContent(tx, document, userID, contentJSONRaw, nil)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// PersistCanonicalContent updates the canonical JSON-backed document state and
// any optional companion fields in a single transaction.
func PersistCanonicalContent(
	tx *gorm.DB,
	document *models.Document,
	userID uuid.UUID,
	contentJSONRaw []byte,
	patch *DocumentBodyPatch,
) (*UpdateContentResult, error) {
	contentJSON, err := normalizeContentJSON(contentJSONRaw)
	if err != nil {
		return nil, err
	}
	if err := ensureWorkspaceStorageWithinLimitForContentUpdate(tx, document.OwnerUserID, document.ID, contentJSON); err != nil {
		return nil, err
	}
	assetIDs, err := extractAssetIDsFromContentJSON(contentJSON)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	plainText := toPlainText(contentJSON)
	excerpt := buildExcerpt(plainText)
	bodyUpdates := map[string]any{
		"content_json":    contentJSON,
		"plain_text":      plainText,
		"updated_by":      userID,
		"content_version": gorm.Expr("content_version + 1"),
		"updated_at":      now,
	}
	if patch != nil {
		if patch.YjsState != nil {
			bodyUpdates["yjs_state"] = *patch.YjsState
		}
		if patch.YjsStateVector != nil {
			bodyUpdates["yjs_state_vector"] = *patch.YjsStateVector
		}
		if patch.YjsVersion != nil {
			bodyUpdates["yjs_version"] = *patch.YjsVersion
		}
	}

	var contentVersion int64
	result := tx.Model(&models.DocumentBody{}).
		Where("document_id = ?", document.ID).
		Updates(bodyUpdates)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		contentRecord := &models.DocumentBody{
			ID:             uuid.New(),
			DocumentID:     document.ID,
			ContentJSON:    contentJSON,
			PlainText:      plainText,
			ContentVersion: 1,
			YjsVersion:     1,
			UpdatedBy:      userID,
		}
		if patch != nil {
			if patch.YjsState != nil {
				contentRecord.YjsState = *patch.YjsState
			}
			if patch.YjsStateVector != nil {
				contentRecord.YjsStateVector = *patch.YjsStateVector
			}
			if patch.YjsVersion != nil {
				contentRecord.YjsVersion = *patch.YjsVersion
			}
		}
		if err := tx.Create(contentRecord).Error; err != nil {
			return nil, err
		}
		contentVersion = contentRecord.ContentVersion
	}

	if contentVersion == 0 {
		var body models.DocumentBody
		if err := tx.Where("document_id = ?", document.ID).First(&body).Error; err != nil {
			return nil, err
		}
		contentVersion = body.ContentVersion
	}

	if err := syncDocumentAssetRefs(tx, document.OwnerUserID, document.ID, assetIDs, now); err != nil {
		return nil, err
	}

	if err := tx.Model(document).Updates(map[string]any{
		"excerpt":    excerpt,
		"updated_at": now,
		"updated_by": userID,
	}).Error; err != nil {
		return nil, err
	}

	return &UpdateContentResult{
		Success:        true,
		ContentVersion: contentVersion,
		UpdatedAt:      now,
	}, nil
}

func ensureWorkspaceStorageWithinLimitForContentUpdate(tx *gorm.DB, ownerUserID, documentID uuid.UUID, nextContentJSON string) error {
	var folderDescriptionBytes int64
	if err := tx.Unscoped().Model(&models.Folder{}).
		Select("COALESCE(SUM(LENGTH(description)), 0)").
		Where("owner_user_id = ?", ownerUserID).
		Scan(&folderDescriptionBytes).Error; err != nil {
		return err
	}

	var documentContentBytes int64
	if err := tx.Raw(`
		SELECT COALESCE(SUM(LENGTH(document_bodies.content_json)), 0)
		FROM document_bodies
		JOIN documents ON documents.id = document_bodies.document_id
		WHERE documents.owner_user_id = ?
	`, ownerUserID).Scan(&documentContentBytes).Error; err != nil {
		return err
	}

	var currentDocumentContentBytes int64
	if err := tx.Raw(`
		SELECT COALESCE(LENGTH(content_json), 0)
		FROM document_bodies
		WHERE document_id = ?
		LIMIT 1
	`, documentID).Scan(&currentDocumentContentBytes).Error; err != nil {
		return err
	}

	projectedTotal := folderDescriptionBytes + documentContentBytes - currentDocumentContentBytes + int64(len(nextContentJSON))
	if projectedTotal > maxWorkspaceStorageBytesPerUser {
		return ErrWorkspaceStorageQuotaExceeded
	}

	return nil
}

// CreateInitialContent creates the first content row for a document.
func CreateInitialContent(tx *gorm.DB, documentID, userID uuid.UUID, contentJSONRaw string) error {
	contentJSON, err := normalizeContentJSON([]byte(contentJSONRaw))
	if err != nil {
		return err
	}
	assetIDs, err := extractAssetIDsFromContentJSON(contentJSON)
	if err != nil {
		return err
	}

	contentRecord := &models.DocumentBody{
		ID:             uuid.New(),
		DocumentID:     documentID,
		ContentJSON:    contentJSON,
		PlainText:      toPlainText(contentJSON),
		ContentVersion: 1,
		UpdatedBy:      userID,
	}

	if err := tx.Create(contentRecord).Error; err != nil {
		return err
	}

	var document models.Document
	if err := tx.Where("id = ?", documentID).First(&document).Error; err != nil {
		return err
	}

	return syncDocumentAssetRefs(tx, document.OwnerUserID, documentID, assetIDs, time.Now())
}

// DeleteContentByDocumentID soft deletes the content row for a document.
//
// The ACL check mirrors the caller's earlier check as defense in depth. The
// previous implementation swallowed every error (including raw DB failures)
// as nil, which left orphaned document_bodies rows whenever the defense-in-
// depth lookup itself hit a transient DB problem. We now treat
// acl.ErrDocumentNotFoundOrForbidden as a benign no-op and propagate every
// other error.
func DeleteContentByDocumentID(tx *gorm.DB, userID, documentID uuid.UUID) error {
	document, _, err := acl.CanAccessDocumentOwnerOnly(tx, userID, documentID)
	if err != nil {
		if errors.Is(err, acl.ErrDocumentNotFoundOrForbidden) {
			return nil
		}
		return err
	}

	if err := tx.Where("document_id = ?", documentID).Delete(&models.DocumentBody{}).Error; err != nil {
		return err
	}

	return removeDocumentAssetRefs(tx, document.OwnerUserID, documentID, time.Now())
}

// RestoreContentByDocumentID restores the content row for a document.
// See DeleteContentByDocumentID for the rationale around error handling.
func RestoreContentByDocumentID(tx *gorm.DB, userID, documentID uuid.UUID) error {
	document, err := acl.CanAccessDocumentOwnerOnlyUnscoped(tx, userID, documentID)
	if err != nil {
		if errors.Is(err, acl.ErrDocumentNotFoundOrForbidden) {
			return nil
		}
		return err
	}

	if err := tx.Unscoped().
		Model(&models.DocumentBody{}).
		Where("document_id = ?", documentID).
		Update("deleted_at", nil).Error; err != nil {
		return err
	}

	var body models.DocumentBody
	if err := tx.Where("document_id = ?", documentID).First(&body).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}

	assetIDs, err := extractAssetIDsFromContentJSON(body.ContentJSON)
	if err != nil {
		return err
	}

	return syncDocumentAssetRefs(tx, document.OwnerUserID, documentID, assetIDs, time.Now())
}

// PermanentDeleteContentByDocumentID permanently deletes the content row for a document.
// See DeleteContentByDocumentID for the rationale around error handling.
func PermanentDeleteContentByDocumentID(tx *gorm.DB, userID, documentID uuid.UUID) error {
	document, err := acl.CanAccessDocumentOwnerOnlyUnscoped(tx, userID, documentID)
	if err != nil {
		if errors.Is(err, acl.ErrDocumentNotFoundOrForbidden) {
			return nil
		}
		return err
	}

	if err := tx.Unscoped().Where("document_id = ?", documentID).Delete(&models.DocumentBody{}).Error; err != nil {
		return err
	}

	return removeDocumentAssetRefs(tx, document.OwnerUserID, documentID, time.Now())
}

func normalizeContentJSON(raw []byte) (string, error) {
	if len(raw) > MaxContentJSONBytes {
		return "", ErrContentJSONTooLarge
	}
	if len(raw) == 0 {
		return defaultContentJSON, nil
	}
	if !json.Valid(raw) {
		return "", ErrInvalidContentJSON
	}
	return string(raw), nil
}

func toPlainText(contentJSON string) string {
	var node any
	if err := json.Unmarshal([]byte(contentJSON), &node); err != nil {
		return ""
	}

	parts := make([]string, 0, 64)
	collectText(node, &parts)
	return joinWithSpace(parts)
}

func collectText(node any, out *[]string) {
	switch v := node.(type) {
	case map[string]any:
		if text, ok := v["text"].(string); ok {
			*out = append(*out, text)
		}
		if children, ok := v["content"].([]any); ok {
			for _, child := range children {
				collectText(child, out)
			}
		}
	case []any:
		for _, item := range v {
			collectText(item, out)
		}
	}
}

func joinWithSpace(parts []string) string {
	merged := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if merged == "" {
			merged = p
			continue
		}
		merged += " " + p
	}
	return merged
}

func buildExcerpt(plainText string) string {
	text := strings.TrimSpace(plainText)
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) > 100 {
		return string(runes[:100]) + "..."
	}
	return text
}

// BuildExcerptFromContentJSON derives a document excerpt from editor JSON.
func BuildExcerptFromContentJSON(contentJSONRaw string) string {
	contentJSON, err := normalizeContentJSON([]byte(contentJSONRaw))
	if err != nil {
		return ""
	}
	firstParagraph := extractFirstParagraph(contentJSON)
	if firstParagraph != "" {
		return buildExcerpt(firstParagraph)
	}
	return buildExcerpt(toPlainText(contentJSON))
}

func extractFirstParagraph(contentJSON string) string {
	var node map[string]any
	if err := json.Unmarshal([]byte(contentJSON), &node); err != nil {
		return ""
	}

	contentNodes, ok := node["content"].([]any)
	if !ok {
		return ""
	}

	for _, rawBlock := range contentNodes {
		block, ok := rawBlock.(map[string]any)
		if !ok {
			continue
		}

		blockType, _ := block["type"].(string)
		if blockType != "paragraph" && blockType != "heading" {
			continue
		}

		parts := make([]string, 0, 16)
		collectText(block, &parts)
		plain := strings.TrimSpace(joinWithSpace(parts))
		if plain != "" {
			return plain
		}
	}

	return ""
}

func extractAssetIDsFromContentJSON(contentJSON string) ([]uuid.UUID, error) {
	var node any
	if err := json.Unmarshal([]byte(contentJSON), &node); err != nil {
		return nil, ErrInvalidContentJSON
	}

	seen := make(map[uuid.UUID]struct{})
	assetIDs := make([]uuid.UUID, 0)
	collectAssetIDs(node, seen, &assetIDs)
	return assetIDs, nil
}

func collectAssetIDs(node any, seen map[uuid.UUID]struct{}, out *[]uuid.UUID) {
	switch v := node.(type) {
	case map[string]any:
		if attrs, ok := v["attrs"].(map[string]any); ok {
			if assetID, ok := extractAssetIDFromAttrs(attrs); ok {
				if _, exists := seen[assetID]; !exists {
					seen[assetID] = struct{}{}
					*out = append(*out, assetID)
				}
			}
		}
		if children, ok := v["content"].([]any); ok {
			for _, child := range children {
				collectAssetIDs(child, seen, out)
			}
		}
	case []any:
		for _, item := range v {
			collectAssetIDs(item, seen, out)
		}
	}
}

func extractAssetIDFromAttrs(attrs map[string]any) (uuid.UUID, bool) {
	if rawAssetID, ok := attrs["assetId"].(string); ok {
		if assetID, err := uuid.Parse(rawAssetID); err == nil {
			return assetID, true
		}
	}

	rawSrc, ok := attrs["src"].(string)
	if !ok || rawSrc == "" {
		return uuid.Nil, false
	}

	if parsed, err := url.Parse(rawSrc); err == nil {
		if match := assetContentPathPattern.FindStringSubmatch(parsed.Path); len(match) == 2 {
			if assetID, err := uuid.Parse(match[1]); err == nil {
				return assetID, true
			}
		}
	}
	if match := assetContentPathPattern.FindStringSubmatch(rawSrc); len(match) == 2 {
		if assetID, err := uuid.Parse(match[1]); err == nil {
			return assetID, true
		}
	}

	return uuid.Nil, false
}

func syncDocumentAssetRefs(tx *gorm.DB, ownerUserID, documentID uuid.UUID, assetIDs []uuid.UUID, now time.Time) error {
	validAssetIDs, err := filterOwnedAssetIDs(tx, ownerUserID, assetIDs)
	if err != nil {
		return err
	}
	if len(validAssetIDs) != len(assetIDs) {
		return ErrInvalidContentAssetReferences
	}

	var existingRefs []models.DocumentAssetRef
	if err := tx.
		Where("document_id = ? AND owner_user_id = ? AND ref_type = ?", documentID, ownerUserID, editorContentRefType).
		Find(&existingRefs).Error; err != nil {
		return err
	}

	existingSet := make(map[uuid.UUID]models.DocumentAssetRef, len(existingRefs))
	affectedSet := make(map[uuid.UUID]struct{}, len(validAssetIDs)+len(existingRefs))
	for _, ref := range existingRefs {
		existingSet[ref.AssetID] = ref
		affectedSet[ref.AssetID] = struct{}{}
	}

	desiredSet := make(map[uuid.UUID]struct{}, len(validAssetIDs))
	for _, assetID := range validAssetIDs {
		desiredSet[assetID] = struct{}{}
		affectedSet[assetID] = struct{}{}
		if ref, ok := existingSet[assetID]; ok {
			if err := tx.Model(&models.DocumentAssetRef{}).
				Where("id = ?", ref.ID).
				Update("updated_at", now).Error; err != nil {
				return err
			}
			continue
		}

		if err := tx.Create(&models.DocumentAssetRef{
			ID:          uuid.New(),
			DocumentID:  documentID,
			AssetID:     assetID,
			OwnerUserID: ownerUserID,
			RefType:     editorContentRefType,
			CreatedAt:   now,
			UpdatedAt:   now,
		}).Error; err != nil {
			return err
		}
	}

	for _, ref := range existingRefs {
		if _, keep := desiredSet[ref.AssetID]; keep {
			continue
		}
		if err := tx.Unscoped().Delete(&models.DocumentAssetRef{}, "id = ?", ref.ID).Error; err != nil {
			return err
		}
	}

	affectedAssetIDs := make([]uuid.UUID, 0, len(affectedSet))
	for assetID := range affectedSet {
		affectedAssetIDs = append(affectedAssetIDs, assetID)
	}

	return reconcileAssetRefs(tx, affectedAssetIDs, now)
}

func filterOwnedAssetIDs(tx *gorm.DB, userID uuid.UUID, assetIDs []uuid.UUID) ([]uuid.UUID, error) {
	if len(assetIDs) == 0 {
		return nil, nil
	}

	var assets []models.Asset
	if err := tx.
		Where("owner_user_id = ? AND id IN ? AND deleted_at IS NULL", userID, assetIDs).
		Find(&assets).Error; err != nil {
		return nil, err
	}

	validSet := make(map[uuid.UUID]struct{}, len(assets))
	for _, asset := range assets {
		validSet[asset.ID] = struct{}{}
	}

	validAssetIDs := make([]uuid.UUID, 0, len(assetIDs))
	for _, assetID := range assetIDs {
		if _, ok := validSet[assetID]; ok {
			validAssetIDs = append(validAssetIDs, assetID)
		}
	}
	return validAssetIDs, nil
}

func reconcileAssetRefs(tx *gorm.DB, assetIDs []uuid.UUID, now time.Time) error {
	for _, assetID := range assetIDs {
		var refCount int64
		if err := tx.Model(&models.DocumentAssetRef{}).
			Where("asset_id = ? AND ref_type = ?", assetID, editorContentRefType).
			Count(&refCount).Error; err != nil {
			return err
		}

		status := assetStatusReady
		if refCount == 0 {
			status = assetStatusPending
		}

		if err := tx.Model(&models.Asset{}).
			Where("id = ? AND deleted_at IS NULL", assetID).
			Updates(map[string]any{
				"reference_count": int(refCount),
				"status":          status,
				"updated_at":      now,
			}).Error; err != nil {
			return err
		}

		if refCount == 0 {
			if err := ensurePendingDeleteJob(tx, assetID, now); err != nil {
				return err
			}
			continue
		}

		if err := cancelPendingDeleteJobs(tx, assetID, now); err != nil {
			return err
		}
	}

	return nil
}

func ensurePendingDeleteJob(tx *gorm.DB, assetID uuid.UUID, now time.Time) error {
	var existing models.AssetGCJob
	err := tx.
		Where("asset_id = ? AND job_type = ? AND status = ?", assetID, "delete", "pending").
		First(&existing).Error
	switch {
	case err == nil:
		return tx.Model(&models.AssetGCJob{}).
			Where("id = ?", existing.ID).
			Updates(map[string]any{
				"run_after":  now.Add(assetDeleteDelay),
				"updated_at": now,
			}).Error
	case errors.Is(err, gorm.ErrRecordNotFound):
		return tx.Create(&models.AssetGCJob{
			ID:       uuid.New(),
			AssetID:  assetID,
			JobType:  "delete",
			Status:   "pending",
			RunAfter: now.Add(assetDeleteDelay),
		}).Error
	default:
		return err
	}
}

func cancelPendingDeleteJobs(tx *gorm.DB, assetID uuid.UUID, now time.Time) error {
	return tx.Model(&models.AssetGCJob{}).
		Where("asset_id = ? AND job_type = ? AND status = ?", assetID, "delete", "pending").
		Updates(map[string]any{
			"status":     "cancelled",
			"updated_at": now,
		}).Error
}

func removeDocumentAssetRefs(tx *gorm.DB, userID, documentID uuid.UUID, now time.Time) error {
	var refs []models.DocumentAssetRef
	if err := tx.
		Where("document_id = ? AND owner_user_id = ? AND ref_type = ?", documentID, userID, editorContentRefType).
		Find(&refs).Error; err != nil {
		return err
	}
	if len(refs) == 0 {
		return nil
	}

	assetIDs := make([]uuid.UUID, 0, len(refs))
	for _, ref := range refs {
		assetIDs = append(assetIDs, ref.AssetID)
	}

	if err := tx.Unscoped().
		Where("document_id = ? AND owner_user_id = ? AND ref_type = ?", documentID, userID, editorContentRefType).
		Delete(&models.DocumentAssetRef{}).Error; err != nil {
		return err
	}

	return reconcileAssetRefs(tx, assetIDs, now)
}
