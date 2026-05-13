package workspace

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/acl"
	"g.co1d.in/Coldin04/Cyime/server/internal/content"
	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupWorkspaceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	t.Setenv("SMTP_ENABLED", "true")

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	if err := db.AutoMigrate(
		&models.User{},
		&models.UserImageBedConfig{},
		&models.Folder{},
		&models.Document{},
		&models.DocumentBody{},
		&models.DocumentAssetRef{},
		&models.DocumentPermission{},
		&models.DocumentImageTargetPreference{},
		&models.DocumentInvite{},
		&models.Notification{},
		&models.BlobObject{},
		&models.Asset{},
	); err != nil {
		t.Fatalf("auto migrate: %v", err)
	}

	database.DB = db
	return db
}

func seedVerifiedUser(t *testing.T, db *gorm.DB, userID uuid.UUID, email string) {
	t.Helper()
	var count int64
	if err := db.Model(&models.User{}).Where("id = ?", userID).Count(&count).Error; err != nil {
		t.Fatalf("count user: %v", err)
	}
	if count > 0 {
		return
	}
	normalizedEmail := email
	now := time.Now()
	user := models.User{
		ID:              userID,
		Email:           &normalizedEmail,
		EmailVerified:   true,
		EmailVerifiedAt: &now,
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
}

func seedDocumentForWorkspace(t *testing.T, db *gorm.DB, ownerID uuid.UUID, title string) uuid.UUID {
	t.Helper()
	seedVerifiedUser(t, db, ownerID, ownerID.String()+"@example.com")

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

	content := models.DocumentBody{
		ID:             uuid.New(),
		DocumentID:     doc.ID,
		ContentJSON:    `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"seed"}]}]}`,
		PlainText:      "seed",
		ContentVersion: 1,
		UpdatedBy:      ownerID,
	}
	if err := db.Create(&content).Error; err != nil {
		t.Fatalf("create document content: %v", err)
	}

	return doc.ID
}

func seedWorkspacePermission(t *testing.T, db *gorm.DB, documentID, userID, createdBy uuid.UUID, role string) {
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

func seedWorkspaceAsset(t *testing.T, db *gorm.DB, ownerID, documentID uuid.UUID, filename string) uuid.UUID {
	t.Helper()

	blob := models.BlobObject{
		ID:              uuid.New(),
		SHA256:          uuid.NewString(),
		Size:            int64(len(filename) + 10),
		MimeType:        "image/png",
		StorageProvider: "local",
		ObjectKey:       "objects/" + uuid.NewString(),
		URL:             "https://example.com/" + filename,
		Status:          "ready",
		ThumbnailStatus: "skipped",
	}
	if err := db.Create(&blob).Error; err != nil {
		t.Fatalf("create blob: %v", err)
	}

	asset := models.Asset{
		ID:             uuid.New(),
		OwnerUserID:    ownerID,
		DocumentID:     &documentID,
		BlobID:         blob.ID,
		Kind:           "image",
		Filename:       filename,
		URL:            blob.URL,
		Visibility:     "private",
		Status:         "ready",
		ReferenceCount: 1,
		CreatedBy:      ownerID,
	}
	if err := db.Create(&asset).Error; err != nil {
		t.Fatalf("create asset: %v", err)
	}

	ref := models.DocumentAssetRef{
		ID:          uuid.New(),
		DocumentID:  documentID,
		AssetID:     asset.ID,
		OwnerUserID: ownerID,
		RefType:     "editor_content",
	}
	if err := db.Create(&ref).Error; err != nil {
		t.Fatalf("create asset ref: %v", err)
	}

	return asset.ID
}

func TestGetFile_Document_DeniesCrossUserAccess(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	attackerID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "owner-doc")

	if _, err := GetFile(attackerID, docID, "document"); err == nil {
		t.Fatal("expected cross-user file access to fail")
	}
}

func TestGetFile_Document_AllowsSharedViewer(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	viewerID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")
	seedWorkspacePermission(t, db, docID, viewerID, ownerID, "viewer")

	item, err := GetFile(viewerID, docID, "document")
	if err != nil {
		t.Fatalf("expected shared viewer access, got error: %v", err)
	}
	if item == nil || item.ID != docID {
		t.Fatalf("unexpected document item: %+v", item)
	}
}

func TestGetFiles_DocumentsSortByNameUsesTitleColumn(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	seedDocumentForWorkspace(t, db, ownerID, "Beta")
	seedDocumentForWorkspace(t, db, ownerID, "Alpha")

	response, err := GetFiles(ownerID, nil, 50, 0, "name", "asc", "documents")
	if err != nil {
		t.Fatalf("get document files: %v", err)
	}
	if len(response.Items) != 2 {
		t.Fatalf("expected 2 documents, got %+v", response.Items)
	}
	if response.Items[0].Name != "Alpha" || response.Items[1].Name != "Beta" {
		t.Fatalf("expected documents sorted by title, got %q then %q", response.Items[0].Name, response.Items[1].Name)
	}
}

func TestGetFiles_FoldersWithoutParentReturnsOnlyRootFolders(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	seedVerifiedUser(t, db, ownerID, "owner@example.com")

	rootFolder := models.Folder{
		ID:          uuid.New(),
		OwnerUserID: ownerID,
		Name:        "Root",
		CreatedBy:   ownerID,
		UpdatedBy:   ownerID,
	}
	if err := db.Create(&rootFolder).Error; err != nil {
		t.Fatalf("create root folder: %v", err)
	}
	childFolder := models.Folder{
		ID:          uuid.New(),
		OwnerUserID: ownerID,
		ParentID:    &rootFolder.ID,
		Name:        "Child",
		CreatedBy:   ownerID,
		UpdatedBy:   ownerID,
	}
	if err := db.Create(&childFolder).Error; err != nil {
		t.Fatalf("create child folder: %v", err)
	}

	response, err := GetFiles(ownerID, nil, 50, 0, "name", "asc", "folders")
	if err != nil {
		t.Fatalf("get folder files: %v", err)
	}
	if len(response.Items) != 1 {
		t.Fatalf("expected only root folder, got %+v", response.Items)
	}
	if response.Items[0].ID != rootFolder.ID {
		t.Fatalf("expected root folder %s, got %s", rootFolder.ID, response.Items[0].ID)
	}
}

func TestMoveDocument_DeniesCrossUserAccess(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	attackerID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "owner-doc")

	if _, err := MoveDocument(attackerID, docID, nil); err == nil {
		t.Fatal("expected cross-user move to fail")
	}
}

func TestBatchMoveFiles_DeniesSharedEditorForDocumentMove(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	editorID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")
	seedWorkspacePermission(t, db, docID, editorID, ownerID, "editor")

	resp, err := BatchMoveFiles(editorID, []ItemToMove{
		{ID: docID, Type: "document"},
	}, nil)
	if err != nil {
		t.Fatalf("batch move failed unexpectedly: %v", err)
	}
	if resp.Success {
		t.Fatal("expected batch move to report partial/failed result")
	}
	if resp.MovedCount != 0 {
		t.Fatalf("expected zero moved items, got %d", resp.MovedCount)
	}
	if len(resp.FailedItems) != 1 {
		t.Fatalf("expected one failed item, got %+v", resp.FailedItems)
	}
}

func TestBatchMoveFiles_MixedOwnedAndForeignDocuments_OnlyMovesOwned(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	otherUserID := uuid.New()

	ownerFolderID := uuid.New()
	if err := db.Create(&models.Folder{
		ID:          ownerFolderID,
		OwnerUserID: ownerID,
		Name:        "owner-folder",
		CreatedBy:   ownerID,
		UpdatedBy:   ownerID,
	}).Error; err != nil {
		t.Fatalf("create owner folder: %v", err)
	}

	otherFolderID := uuid.New()
	if err := db.Create(&models.Folder{
		ID:          otherFolderID,
		OwnerUserID: otherUserID,
		Name:        "other-folder",
		CreatedBy:   otherUserID,
		UpdatedBy:   otherUserID,
	}).Error; err != nil {
		t.Fatalf("create other folder: %v", err)
	}

	ownedDocID := seedDocumentForWorkspace(t, db, ownerID, "owned-doc")
	foreignDocID := seedDocumentForWorkspace(t, db, otherUserID, "foreign-doc")

	if err := db.Model(&models.Document{}).Where("id = ?", ownedDocID).Update("folder_id", ownerFolderID).Error; err != nil {
		t.Fatalf("attach owned doc: %v", err)
	}
	if err := db.Model(&models.Document{}).Where("id = ?", foreignDocID).Update("folder_id", otherFolderID).Error; err != nil {
		t.Fatalf("attach foreign doc: %v", err)
	}

	resp, err := BatchMoveFiles(ownerID, []ItemToMove{
		{ID: ownedDocID, Type: "document"},
		{ID: foreignDocID, Type: "document"},
	}, nil)
	if err != nil {
		t.Fatalf("batch move failed unexpectedly: %v", err)
	}
	if resp.Success {
		t.Fatal("expected partial success because one document is unauthorized")
	}
	if resp.MovedCount != 1 {
		t.Fatalf("expected one moved item, got %d", resp.MovedCount)
	}
	if len(resp.FailedItems) != 1 {
		t.Fatalf("expected one failed item, got %+v", resp.FailedItems)
	}

	var ownedDoc models.Document
	if err := db.First(&ownedDoc, "id = ?", ownedDocID).Error; err != nil {
		t.Fatalf("load owned doc: %v", err)
	}
	if ownedDoc.FolderID != nil {
		t.Fatalf("expected owned doc moved to root, got folder %v", *ownedDoc.FolderID)
	}

	var foreignDoc models.Document
	if err := db.First(&foreignDoc, "id = ?", foreignDocID).Error; err != nil {
		t.Fatalf("load foreign doc: %v", err)
	}
	if foreignDoc.FolderID == nil || *foreignDoc.FolderID != otherFolderID {
		t.Fatalf("expected foreign doc unchanged, got %+v", foreignDoc.FolderID)
	}
}

func TestDeleteFile_Document_DeniesCrossUserAccessAndKeepsRow(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	attackerID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "owner-doc")

	if err := DeleteFile(attackerID, docID, "document"); err == nil {
		t.Fatal("expected cross-user delete to fail")
	}

	var got models.Document
	if err := db.First(&got, "id = ?", docID).Error; err != nil {
		t.Fatalf("load document: %v", err)
	}
	if got.DeletedAt.Valid {
		t.Fatal("expected document to remain undeleted")
	}
}

func TestGetPublicDocument_PrivateDocumentNotExposed(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "private-doc")

	item, err := GetPublicDocument(docID, nil)
	if err == nil || item != nil {
		t.Fatalf("expected private doc to be hidden, got item=%+v err=%v", item, err)
	}
}

func TestGetPublicDocument_PublicDocumentReadable(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "public-doc")

	if err := db.Model(&models.Document{}).Where("id = ?", docID).Update("public_access", PublicAccessGlobal).Error; err != nil {
		t.Fatalf("set public_access: %v", err)
	}

	item, err := GetPublicDocument(docID, nil)
	if err != nil {
		t.Fatalf("expected public doc readable, got err=%v", err)
	}
	if item == nil || item.PublicAccess == nil || *item.PublicAccess != PublicAccessGlobal {
		t.Fatalf("expected public access in response, got item=%+v", item)
	}
}

func TestGetPublicDocument_AuthenticatedRequiresLogin(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	readerID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "auth-doc")
	seedVerifiedUser(t, db, readerID, readerID.String()+"@example.com")

	if err := db.Model(&models.Document{}).Where("id = ?", docID).Update("public_access", PublicAccessAuthenticated).Error; err != nil {
		t.Fatalf("set public_access: %v", err)
	}

	if _, err := GetPublicDocument(docID, nil); err == nil {
		t.Fatal("expected unauthenticated access to fail")
	}

	item, err := GetPublicDocument(docID, &readerID)
	if err != nil {
		t.Fatalf("expected authenticated read success, got err=%v", err)
	}
	if item == nil {
		t.Fatal("expected document item")
	}
}

func TestUpdateDocumentPublicAccess_DeniesNonOwner(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	editorID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "public-control")
	seedWorkspacePermission(t, db, docID, editorID, ownerID, "editor")

	if err := UpdateDocumentPublicAccess(editorID, docID, PublicAccessGlobal); err == nil {
		t.Fatal("expected non-owner to be denied public-access update")
	}
}

func TestUpdateDocumentImageTarget_DeniesCrossUserAccess(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	attackerID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "owner-doc")
	config := models.UserImageBedConfig{
		ID:           uuid.New(),
		UserID:       attackerID,
		Name:         "attacker bed",
		ProviderType: "see",
		APIToken:     stringPtr("token"),
		IsEnabled:    true,
	}
	if err := db.Create(&config).Error; err != nil {
		t.Fatalf("create image bed config: %v", err)
	}

	if err := UpdateDocumentImageTarget(attackerID, docID, config.ID.String()); err == nil {
		t.Fatal("expected cross-user image target update to fail")
	}
}

func TestUpdateDocumentImageTarget_AllowsSharedEditorToSetPersonalPreference(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	editorID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")
	seedWorkspacePermission(t, db, docID, editorID, ownerID, "editor")

	ownerConfig := models.UserImageBedConfig{
		ID:           uuid.New(),
		UserID:       ownerID,
		Name:         "owner bed",
		ProviderType: "see",
		APIToken:     stringPtr("owner-token"),
		IsEnabled:    true,
	}
	if err := db.Create(&ownerConfig).Error; err != nil {
		t.Fatalf("create owner config: %v", err)
	}
	if err := db.Model(&models.Document{}).Where("id = ?", docID).Update("preferred_image_target_id", ownerConfig.ID.String()).Error; err != nil {
		t.Fatalf("set owner preferred target: %v", err)
	}

	editorConfig := models.UserImageBedConfig{
		ID:           uuid.New(),
		UserID:       editorID,
		Name:         "editor bed",
		ProviderType: "see",
		APIToken:     stringPtr("editor-token"),
		IsEnabled:    true,
	}
	if err := db.Create(&editorConfig).Error; err != nil {
		t.Fatalf("create editor config: %v", err)
	}

	if err := UpdateDocumentImageTarget(editorID, docID, editorConfig.ID.String()); err != nil {
		t.Fatalf("expected shared editor to set personal image target: %v", err)
	}

	var doc models.Document
	if err := db.First(&doc, "id = ?", docID).Error; err != nil {
		t.Fatalf("load document: %v", err)
	}
	if doc.PreferredImageTargetID != ownerConfig.ID.String() {
		t.Fatalf("expected document default target unchanged, got %s", doc.PreferredImageTargetID)
	}

	var preference models.DocumentImageTargetPreference
	if err := db.Where("document_id = ? AND user_id = ?", docID, editorID).First(&preference).Error; err != nil {
		t.Fatalf("load personal image target preference: %v", err)
	}
	if preference.TargetID != editorConfig.ID.String() {
		t.Fatalf("expected personal target %s, got %s", editorConfig.ID, preference.TargetID)
	}
}

func TestUpdateDocumentTitle_AllowsEditor(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	editorID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")
	seedWorkspacePermission(t, db, docID, editorID, ownerID, "editor")

	if err := UpdateDocumentTitle(editorID, docID, "updated-by-editor"); err != nil {
		t.Fatalf("expected editor title update success: %v", err)
	}

	var doc models.Document
	if err := db.First(&doc, "id = ?", docID).Error; err != nil {
		t.Fatalf("load document: %v", err)
	}
	if doc.Title != "updated-by-editor" {
		t.Fatalf("expected updated title, got %s", doc.Title)
	}
}

func TestUpdateDocumentTitle_DeniesViewer(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	viewerID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")
	seedWorkspacePermission(t, db, docID, viewerID, ownerID, "viewer")

	if err := UpdateDocumentTitle(viewerID, docID, "viewer-should-fail"); err == nil {
		t.Fatal("expected viewer title update to fail")
	}
}

func TestUpdateDocumentManualExcerpt_AllowsCollaborator(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	collaboratorID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")
	seedWorkspacePermission(t, db, docID, collaboratorID, ownerID, "collaborator")

	manualExcerpt, excerpt, err := UpdateDocumentManualExcerpt(collaboratorID, docID, "手动介绍")
	if err != nil {
		t.Fatalf("expected collaborator manual excerpt update success: %v", err)
	}
	if manualExcerpt != "手动介绍" {
		t.Fatalf("expected returned manual excerpt, got %q", manualExcerpt)
	}
	if excerpt != "手动介绍" {
		t.Fatalf("expected returned excerpt to be manual text, got %q", excerpt)
	}

	var doc models.Document
	if err := db.First(&doc, "id = ?", docID).Error; err != nil {
		t.Fatalf("load document: %v", err)
	}
	if doc.ManualExcerpt != "手动介绍" {
		t.Fatalf("expected manual excerpt saved, got %q", doc.ManualExcerpt)
	}
}

func TestUpdateDocumentManualExcerpt_DeniesEditor(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	editorID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")
	seedWorkspacePermission(t, db, docID, editorID, ownerID, "editor")

	if _, _, err := UpdateDocumentManualExcerpt(editorID, docID, "editor-should-fail"); err == nil {
		t.Fatal("expected editor manual excerpt update to fail")
	}
}

func TestUpdateDocumentManualExcerpt_AllowsOwnerWhenCollaborationDisabled(t *testing.T) {
	t.Setenv("COLLABORATION_ENABLED", "false")

	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "owner-doc")

	manualExcerpt, excerpt, err := UpdateDocumentManualExcerpt(ownerID, docID, "owner-manual-excerpt")
	if err != nil {
		t.Fatalf("expected owner manual excerpt update success when collaboration disabled: %v", err)
	}
	if manualExcerpt != "owner-manual-excerpt" {
		t.Fatalf("expected returned manual excerpt, got %q", manualExcerpt)
	}
	if excerpt != "owner-manual-excerpt" {
		t.Fatalf("expected returned excerpt to be manual text, got %q", excerpt)
	}

	var doc models.Document
	if err := db.First(&doc, "id = ?", docID).Error; err != nil {
		t.Fatalf("load document: %v", err)
	}
	if doc.ManualExcerpt != "owner-manual-excerpt" {
		t.Fatalf("expected manual excerpt saved, got %q", doc.ManualExcerpt)
	}
}

func TestShareDocument_AllowsOwnerToGrantEditor(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	targetUserID := uuid.New()
	seedVerifiedUser(t, db, targetUserID, targetUserID.String()+"@example.com")
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")

	result, err := ShareDocument(ownerID, docID, targetUserID, "editor")
	if err != nil {
		t.Fatalf("share document: %v", err)
	}
	if len(result.Members) != 2 {
		t.Fatalf("expected owner + one member, got %+v", result.Members)
	}
}

func TestShareDocument_RevivesSoftDeletedPermission(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	targetUserID := uuid.New()
	seedVerifiedUser(t, db, targetUserID, targetUserID.String()+"@example.com")
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")

	if _, err := ShareDocument(ownerID, docID, targetUserID, "viewer"); err != nil {
		t.Fatalf("share document: %v", err)
	}
	if _, err := RemoveDocumentMember(ownerID, docID, targetUserID); err != nil {
		t.Fatalf("remove member: %v", err)
	}
	if _, err := ShareDocument(ownerID, docID, targetUserID, "editor"); err != nil {
		t.Fatalf("re-share document: %v", err)
	}

	var permission models.DocumentPermission
	if err := db.Unscoped().First(&permission, "document_id = ? AND user_id = ?", docID, targetUserID).Error; err != nil {
		t.Fatalf("load permission: %v", err)
	}
	if permission.DeletedAt.Valid {
		t.Fatalf("expected permission revived")
	}
	if permission.Role != "editor" {
		t.Fatalf("expected role updated, got %s", permission.Role)
	}
}

func TestAcceptDocumentInviteRejectsInviteAfterInviterRevoked(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	collaboratorID := uuid.New()
	inviteeID := uuid.New()
	seedVerifiedUser(t, db, collaboratorID, "collaborator@example.com")
	seedVerifiedUser(t, db, inviteeID, "invitee@example.com")
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")
	seedWorkspacePermission(t, db, docID, collaboratorID, ownerID, acl.RoleCollaborator)

	if _, err := InviteDocumentByEmail(collaboratorID, docID, "invitee@example.com", acl.RoleEditor); err != nil {
		t.Fatalf("invite document: %v", err)
	}

	var invite models.DocumentInvite
	if err := db.Where("document_id = ? AND inviter_user_id = ? AND invitee_user_id = ?", docID, collaboratorID, inviteeID).First(&invite).Error; err != nil {
		t.Fatalf("load invite: %v", err)
	}

	if _, err := RemoveDocumentMember(ownerID, docID, collaboratorID); err != nil {
		t.Fatalf("remove collaborator: %v", err)
	}

	err := AcceptDocumentInvite(inviteeID, invite.ID)
	if !errors.Is(err, ErrInviteInvalidStatus) {
		t.Fatalf("expected invalid invite status after inviter revocation, got %v", err)
	}

	var permissionCount int64
	if err := db.Model(&models.DocumentPermission{}).
		Where("document_id = ? AND user_id = ? AND deleted_at IS NULL", docID, inviteeID).
		Count(&permissionCount).Error; err != nil {
		t.Fatalf("count invitee permissions: %v", err)
	}
	if permissionCount != 0 {
		t.Fatalf("expected no invitee permission, got %d", permissionCount)
	}

	var updatedInvite models.DocumentInvite
	if err := db.First(&updatedInvite, "id = ?", invite.ID).Error; err != nil {
		t.Fatalf("reload invite: %v", err)
	}
	if updatedInvite.Status != documentInviteStatusCanceled {
		t.Fatalf("expected invite canceled, got %s", updatedInvite.Status)
	}
}

func TestListSharedDocuments_ReturnsPermissionedDocs(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	sharedUserID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")
	seedWorkspacePermission(t, db, docID, sharedUserID, ownerID, "viewer")

	result, err := ListSharedDocuments(sharedUserID, 20, 0)
	if err != nil {
		t.Fatalf("list shared documents: %v", err)
	}
	if len(result.Items) != 1 || result.Items[0].DocumentID != docID {
		t.Fatalf("unexpected shared documents: %+v", result.Items)
	}
	if result.Items[0].MyRole != "viewer" {
		t.Fatalf("expected viewer role, got %+v", result.Items[0])
	}
}

func TestListOutgoingSharedDocuments_ReturnsManagedSharedAndPublicDocs(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	sharedUserID := uuid.New()
	docSharedID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")
	docPublicID := seedDocumentForWorkspace(t, db, ownerID, "public-doc")
	docPrivateID := seedDocumentForWorkspace(t, db, ownerID, "private-doc")

	seedWorkspacePermission(t, db, docSharedID, sharedUserID, ownerID, "viewer")
	if err := db.Model(&models.Document{}).Where("id = ?", docPublicID).Update("public_access", PublicAccessAuthenticated).Error; err != nil {
		t.Fatalf("set public access: %v", err)
	}
	if err := db.Model(&models.Document{}).Where("id = ?", docPublicID).Update("updated_at", time.Now().Add(time.Second)).Error; err != nil {
		t.Fatalf("touch public doc: %v", err)
	}
	if err := db.Model(&models.Document{}).Where("id = ?", docPrivateID).Update("updated_at", time.Now().Add(-time.Second)).Error; err != nil {
		t.Fatalf("touch private doc: %v", err)
	}

	result, err := ListOutgoingSharedDocuments(ownerID, 20, 0)
	if err != nil {
		t.Fatalf("list outgoing shared documents: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 outgoing docs, got %+v", result.Items)
	}

	foundShared := false
	foundPublic := false
	for _, item := range result.Items {
		switch item.DocumentID {
		case docSharedID:
			foundShared = true
			if item.SharedMemberCount != 1 {
				t.Fatalf("expected shared member count 1, got %+v", item)
			}
			if item.MyRole != acl.RoleOwner {
				t.Fatalf("expected owner role, got %+v", item)
			}
		case docPublicID:
			foundPublic = true
			if item.PublicAccess != PublicAccessAuthenticated {
				t.Fatalf("expected authenticated public access, got %+v", item)
			}
		case docPrivateID:
			t.Fatalf("unexpected private doc in outgoing list: %+v", item)
		}
	}
	if !foundShared || !foundPublic {
		t.Fatalf("missing expected outgoing docs: %+v", result.Items)
	}
}

func TestLeaveSharedDocument_RemovesPermissionOnly(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	sharedUserID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")
	seedWorkspacePermission(t, db, docID, sharedUserID, ownerID, "editor")

	if err := LeaveSharedDocument(sharedUserID, docID); err != nil {
		t.Fatalf("leave shared document: %v", err)
	}

	var permissionCount int64
	if err := db.Model(&models.DocumentPermission{}).Where("document_id = ? AND user_id = ?", docID, sharedUserID).Count(&permissionCount).Error; err != nil {
		t.Fatalf("count permissions: %v", err)
	}
	if permissionCount != 0 {
		t.Fatalf("expected permission removed, got %d", permissionCount)
	}

	var doc models.Document
	if err := db.First(&doc, "id = ?", docID).Error; err != nil {
		t.Fatalf("load document: %v", err)
	}
	if doc.DeletedAt.Valid {
		t.Fatalf("expected document untouched")
	}
}

func TestCreateDocument_CountsTrashedDocumentsTowardQuota(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "trashed-doc")

	quota := 1
	if err := db.Model(&models.User{}).Where("id = ?", ownerID).Update("document_quota", quota).Error; err != nil {
		t.Fatalf("set document quota: %v", err)
	}
	if err := db.Delete(&models.Document{}, "id = ?", docID).Error; err != nil {
		t.Fatalf("trash document: %v", err)
	}
	if err := db.Delete(&models.DocumentBody{}, "document_id = ?", docID).Error; err != nil {
		t.Fatalf("trash document body: %v", err)
	}

	_, err := CreateDocument(
		ownerID,
		"new-doc",
		`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"new"}]}]}`,
		nil,
		"rich_text",
		"",
	)
	if !errors.Is(err, ErrDocumentQuotaExceeded) {
		t.Fatalf("expected document quota exceeded, got %v", err)
	}
}

func TestRestoreTrashedItems_RejectsWhenQuotaAlreadyExceeded(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "trashed-doc")

	quota := 0
	if err := db.Model(&models.User{}).Where("id = ?", ownerID).Update("document_quota", quota).Error; err != nil {
		t.Fatalf("set document quota: %v", err)
	}
	if err := db.Delete(&models.Document{}, "id = ?", docID).Error; err != nil {
		t.Fatalf("trash document: %v", err)
	}
	if err := db.Delete(&models.DocumentBody{}, "document_id = ?", docID).Error; err != nil {
		t.Fatalf("trash document body: %v", err)
	}

	response, err := RestoreTrashedItems(ownerID, []ItemToRestore{{ID: docID, Type: "document"}})
	if err != nil {
		t.Fatalf("restore trashed items: %v", err)
	}
	if response.RestoredCount != 0 {
		t.Fatalf("expected zero restored items, got %d", response.RestoredCount)
	}
	if len(response.FailedItems) != 1 {
		t.Fatalf("expected one failed item, got %+v", response.FailedItems)
	}
	if response.FailedItems[0].Reason != ErrDocumentQuotaExceeded.Error() {
		t.Fatalf("expected quota failure, got %+v", response.FailedItems[0])
	}

	var document models.Document
	if err := db.Unscoped().First(&document, "id = ?", docID).Error; err != nil {
		t.Fatalf("load trashed document: %v", err)
	}
	if !document.DeletedAt.Valid {
		t.Fatal("expected document to remain in trash")
	}
}

func TestRestoreTrashedItems_RejectsRootFolderNameConflict(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	seedVerifiedUser(t, db, ownerID, "owner@example.com")

	trashedFolder := models.Folder{
		ID:          uuid.New(),
		OwnerUserID: ownerID,
		Name:        "Projects",
		CreatedBy:   ownerID,
		UpdatedBy:   ownerID,
	}
	if err := db.Create(&trashedFolder).Error; err != nil {
		t.Fatalf("create trashed folder: %v", err)
	}
	if err := db.Delete(&models.Folder{}, "id = ?", trashedFolder.ID).Error; err != nil {
		t.Fatalf("trash folder: %v", err)
	}

	activeFolder := models.Folder{
		ID:          uuid.New(),
		OwnerUserID: ownerID,
		Name:        "Projects",
		CreatedBy:   ownerID,
		UpdatedBy:   ownerID,
	}
	if err := db.Create(&activeFolder).Error; err != nil {
		t.Fatalf("create active conflicting folder: %v", err)
	}

	response, err := RestoreTrashedItems(ownerID, []ItemToRestore{{ID: trashedFolder.ID, Type: "folder"}})
	if err != nil {
		t.Fatalf("restore trashed folder: %v", err)
	}
	if response.RestoredCount != 0 || len(response.FailedItems) != 1 {
		t.Fatalf("expected one failed restore, got %+v", response)
	}

	var folder models.Folder
	if err := db.Unscoped().First(&folder, "id = ?", trashedFolder.ID).Error; err != nil {
		t.Fatalf("load trashed folder: %v", err)
	}
	if !folder.DeletedAt.Valid {
		t.Fatal("expected folder to remain in trash")
	}
}

func TestRestoreTrashedItems_RejectsRootDocumentTitleConflict(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	trashedDocID := seedDocumentForWorkspace(t, db, ownerID, "Release Notes")
	if err := db.Delete(&models.Document{}, "id = ?", trashedDocID).Error; err != nil {
		t.Fatalf("trash document: %v", err)
	}
	if err := db.Delete(&models.DocumentBody{}, "document_id = ?", trashedDocID).Error; err != nil {
		t.Fatalf("trash document body: %v", err)
	}
	seedDocumentForWorkspace(t, db, ownerID, "Release Notes")

	response, err := RestoreTrashedItems(ownerID, []ItemToRestore{{ID: trashedDocID, Type: "document"}})
	if err != nil {
		t.Fatalf("restore trashed document: %v", err)
	}
	if response.RestoredCount != 0 || len(response.FailedItems) != 1 {
		t.Fatalf("expected one failed restore, got %+v", response)
	}

	var document models.Document
	if err := db.Unscoped().First(&document, "id = ?", trashedDocID).Error; err != nil {
		t.Fatalf("load trashed document: %v", err)
	}
	if !document.DeletedAt.Valid {
		t.Fatal("expected document to remain in trash")
	}
}

func stringPtr(value string) *string {
	return &value
}

func seedFolderWithParentForWorkspace(t *testing.T, db *gorm.DB, ownerID uuid.UUID, name string, parentID *uuid.UUID) uuid.UUID {
	t.Helper()
	seedVerifiedUser(t, db, ownerID, ownerID.String()+"@example.com")

	folder := models.Folder{
		ID:          uuid.New(),
		OwnerUserID: ownerID,
		ParentID:    parentID,
		Name:        name,
		CreatedBy:   ownerID,
		UpdatedBy:   ownerID,
	}
	if err := db.Create(&folder).Error; err != nil {
		t.Fatalf("create folder: %v", err)
	}
	return folder.ID
}

func TestCheckCircularDependency_DetectsExistingUnrelatedCycle(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()

	folderA := seedFolderWithParentForWorkspace(t, db, ownerID, "A", nil)
	folderB := seedFolderWithParentForWorkspace(t, db, ownerID, "B", &folderA)
	folderC := seedFolderWithParentForWorkspace(t, db, ownerID, "C", nil)

	if err := db.Model(&models.Folder{}).Where("id = ?", folderA).Update("parent_id", folderB).Error; err != nil {
		t.Fatalf("create existing cycle: %v", err)
	}

	err := checkCircularDependency(db, ownerID, &folderC, &folderA)
	if !errors.Is(err, ErrFolderMoveCycle) {
		t.Fatalf("expected cycle error, got %v", err)
	}
}

func TestDeleteFolderRecursive_DetectsExistingCycle(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()

	folderA := seedFolderWithParentForWorkspace(t, db, ownerID, "A", nil)
	folderB := seedFolderWithParentForWorkspace(t, db, ownerID, "B", &folderA)

	if err := db.Model(&models.Folder{}).Where("id = ?", folderA).Update("parent_id", folderB).Error; err != nil {
		t.Fatalf("create existing cycle: %v", err)
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		return deleteFolderRecursive(tx, ownerID, folderA)
	})
	if err == nil {
		t.Fatal("expected recursive delete to reject folder cycle")
	}
}

func TestMoveFolder_DetectsCycleInTransaction(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()

	folderA := seedFolderWithParentForWorkspace(t, db, ownerID, "A", nil)
	folderB := seedFolderWithParentForWorkspace(t, db, ownerID, "B", &folderA)

	if _, err := MoveFolder(ownerID, folderA, &folderB); !errors.Is(err, ErrFolderMoveCycle) {
		t.Fatalf("expected move cycle error, got %v", err)
	}
}

func TestGetFilesCapsClientLimit(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	seedVerifiedUser(t, db, ownerID, ownerID.String()+"@example.com")

	for i := 0; i < maxFileListLimit+5; i++ {
		folder := models.Folder{
			ID:          uuid.New(),
			OwnerUserID: ownerID,
			Name:        fmt.Sprintf("folder-%03d", i),
			CreatedBy:   ownerID,
			UpdatedBy:   ownerID,
		}
		if err := db.Create(&folder).Error; err != nil {
			t.Fatalf("seed folder %d: %v", i, err)
		}
	}

	response, err := GetFiles(ownerID, nil, maxFileListLimit*1000, 0, "name", "asc", "folders")
	if err != nil {
		t.Fatalf("get files: %v", err)
	}
	if len(response.Items) != maxFileListLimit {
		t.Fatalf("expected capped item count %d, got %d", maxFileListLimit, len(response.Items))
	}
	if !response.HasMore {
		t.Fatalf("expected hasMore when total exceeds capped limit")
	}
}

func TestCreateFolderRejectsOversizedDescription(t *testing.T) {
	setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	description := strings.Repeat("a", maxFolderDescriptionBytes+1)

	_, err := CreateFolder(ownerID, "oversized-description", &description, nil)
	if !errors.Is(err, ErrFolderDescriptionTooLong) {
		t.Fatalf("expected oversized description error, got %v", err)
	}
}

func TestCreateDocumentRejectsOversizedContentJSON(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	seedVerifiedUser(t, db, ownerID, ownerID.String()+"@example.com")
	contentJSON := `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"` + strings.Repeat("a", content.MaxContentJSONBytes) + `"}]}]}`

	_, err := CreateDocument(ownerID, "oversized-content", contentJSON, nil, "rich_text", "")
	if !errors.Is(err, content.ErrContentJSONTooLarge) {
		t.Fatalf("expected oversized content error, got %v", err)
	}
}
