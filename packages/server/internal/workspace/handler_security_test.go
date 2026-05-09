package workspace

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func newWorkspaceTestApp(userID uuid.UUID) *fiber.App {
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("userId", userID.String())
		return c.Next()
	})
	app.Get("/search", SearchHandler)
	app.Get("/files/:id", GetFileHandler)
	app.Get("/public/documents/:id", GetPublicDocumentHandler)
	app.Get("/shared/summary", SharedDocumentSummaryHandler)
	app.Get("/shared/documents", ListSharedDocumentsHandler)
	app.Get("/shared/outgoing", ListOutgoingSharedDocumentsHandler)
	app.Get("/documents/:id/shares", ListDocumentMembersHandler)
	app.Put("/documents/:id/excerpt", UpdateDocumentExcerptHandler)
	app.Put("/documents/:id/public-access", UpdateDocumentPublicAccessHandler)
	app.Post("/documents/:id/shares", ShareDocumentHandler)
	app.Post("/documents/:id/invites", InviteDocumentByEmailHandler)
	app.Delete("/documents/:id/shares/me", LeaveSharedDocumentHandler)
	app.Delete("/documents/:id/shares/:userId", RemoveDocumentMemberHandler)
	app.Get("/notifications", ListNotificationsHandler)
	app.Delete("/notifications", ClearNotificationsHandler)
	app.Post("/notifications/:id/read", MarkNotificationReadHandler)
	app.Post("/document-invites/:id/accept", AcceptDocumentInviteHandler)
	app.Post("/document-invites/:id/decline", DeclineDocumentInviteHandler)
	app.Delete("/trash", PermanentDeleteHandler)
	app.Delete("/files/:id", DeleteFileHandler)
	app.Post("/files/batch-delete", BatchDeleteHandler)
	app.Post("/files/batch-move", BatchMoveHandler)
	return app
}

func seedFolderForWorkspace(t *testing.T, db *gorm.DB, ownerID uuid.UUID, name string) uuid.UUID {
	t.Helper()

	folder := models.Folder{
		ID:          uuid.New(),
		OwnerUserID: ownerID,
		Name:        name,
		CreatedBy:   ownerID,
		UpdatedBy:   ownerID,
	}
	if err := db.Create(&folder).Error; err != nil {
		t.Fatalf("create folder: %v", err)
	}

	return folder.ID
}

func TestGetFileHandler_Document_CrossUserDenied(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	attackerID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "owner-doc")

	app := newWorkspaceTestApp(attackerID)
	req := httptest.NewRequest(http.MethodGet, "/files/"+docID.String()+"?type=document", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestDeleteFileHandler_Document_CrossUserDeniedAndNotDeleted(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	attackerID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "owner-doc")

	app := newWorkspaceTestApp(attackerID)
	req := httptest.NewRequest(http.MethodDelete, "/files/"+docID.String()+"?type=document", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}

	var got models.Document
	if err := db.First(&got, "id = ?", docID).Error; err != nil {
		t.Fatalf("load document: %v", err)
	}
	if got.DeletedAt.Valid {
		t.Fatal("expected document to remain undeleted")
	}
}

func TestBatchDeleteHandler_Document_CrossUserDeniedAndNotDeleted(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	attackerID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "owner-doc")

	app := newWorkspaceTestApp(attackerID)
	body := bytes.NewBufferString(`{"items":[{"id":"` + docID.String() + `","type":"document"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/files/batch-delete", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode != http.StatusMultiStatus {
		t.Fatalf("expected 207, got %d", resp.StatusCode)
	}

	var got models.Document
	if err := db.First(&got, "id = ?", docID).Error; err != nil {
		t.Fatalf("load document: %v", err)
	}
	if got.DeletedAt.Valid {
		t.Fatal("expected document to remain undeleted")
	}
}

func TestBatchMoveHandler_Document_CrossUserDeniedAndNotMoved(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	attackerID := uuid.New()
	folderID := seedFolderForWorkspace(t, db, ownerID, "owner-folder")
	docID := seedDocumentForWorkspace(t, db, ownerID, "owner-doc")
	if err := db.Model(&models.Document{}).Where("id = ?", docID).Update("folder_id", folderID).Error; err != nil {
		t.Fatalf("attach document to folder: %v", err)
	}

	app := newWorkspaceTestApp(attackerID)
	body := bytes.NewBufferString(`{"items":[{"id":"` + docID.String() + `","type":"document"}],"destinationFolderId":null}`)
	req := httptest.NewRequest(http.MethodPost, "/files/batch-move", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if resp.StatusCode != http.StatusMultiStatus {
		t.Fatalf("expected 207, got %d", resp.StatusCode)
	}

	var got models.Document
	if err := db.First(&got, "id = ?", docID).Error; err != nil {
		t.Fatalf("load document: %v", err)
	}
	if got.FolderID == nil || *got.FolderID != folderID {
		t.Fatal("expected document folder unchanged")
	}
}

func TestListSharedDocumentsHandler_ReturnsSharedDocs(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	sharedUserID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")
	seedWorkspacePermission(t, db, docID, sharedUserID, ownerID, "viewer")

	app := newWorkspaceTestApp(sharedUserID)
	req := httptest.NewRequest(http.MethodGet, "/shared/documents", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload SharedDocumentListResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Items) != 1 || payload.Items[0].DocumentID != docID {
		t.Fatalf("unexpected shared payload: %+v", payload)
	}
}

func TestListOutgoingSharedDocumentsHandler_ReturnsManagedDocs(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	sharedUserID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")
	seedWorkspacePermission(t, db, docID, sharedUserID, ownerID, "viewer")

	app := newWorkspaceTestApp(ownerID)
	req := httptest.NewRequest(http.MethodGet, "/shared/outgoing", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload OutgoingSharedDocumentListResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload.Items) != 1 || payload.Items[0].DocumentID != docID {
		t.Fatalf("unexpected outgoing payload: %+v", payload)
	}
	if payload.Items[0].SharedMemberCount != 1 {
		t.Fatalf("expected sharedMemberCount=1, got %+v", payload.Items[0])
	}
}

func TestListOutgoingSharedDocumentsHandler_DisabledWhenCollaborationOff(t *testing.T) {
	t.Setenv("COLLABORATION_ENABLED", "false")

	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	sharedUserID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")
	seedWorkspacePermission(t, db, docID, sharedUserID, ownerID, "viewer")

	app := newWorkspaceTestApp(ownerID)
	req := httptest.NewRequest(http.MethodGet, "/shared/outgoing", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 when collaboration disabled, got %d", resp.StatusCode)
	}
}

func TestGetFileHandler_SharedViewerReturnsViewerRole(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	viewerID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")
	seedWorkspacePermission(t, db, docID, viewerID, ownerID, "viewer")

	app := newWorkspaceTestApp(viewerID)
	req := httptest.NewRequest(http.MethodGet, "/files/"+docID.String()+"?type=document", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload FileItem
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.MyRole == nil || *payload.MyRole != "viewer" {
		t.Fatalf("expected myRole=viewer, got %+v", payload.MyRole)
	}
}

func TestSharedDocumentSummaryHandler_ReturnsHasSharedDocuments(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	sharedUserID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")
	seedWorkspacePermission(t, db, docID, sharedUserID, ownerID, "viewer")

	app := newWorkspaceTestApp(sharedUserID)
	req := httptest.NewRequest(http.MethodGet, "/shared/summary", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload SharedDocumentSummaryResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.HasSharedDocuments {
		t.Fatal("expected hasSharedDocuments=true")
	}
}

func TestGetPublicDocumentHandler_AuthenticatedAccessRequiresLogin(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	signedInReaderID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "auth-public-doc")
	seedVerifiedUser(t, db, signedInReaderID, signedInReaderID.String()+"@example.com")
	if err := db.Model(&models.Document{}).Where("id = ?", docID).Update("public_access", PublicAccessAuthenticated).Error; err != nil {
		t.Fatalf("set public access: %v", err)
	}

	anonymousApp := fiber.New()
	anonymousApp.Get("/public/documents/:id", GetPublicDocumentHandler)
	anonymousReq := httptest.NewRequest(http.MethodGet, "/public/documents/"+docID.String(), nil)
	anonymousResp, err := anonymousApp.Test(anonymousReq, -1)
	if err != nil {
		t.Fatalf("anonymous request failed: %v", err)
	}
	if anonymousResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected anonymous 401, got %d", anonymousResp.StatusCode)
	}

	signedInApp := newWorkspaceTestApp(signedInReaderID)
	signedInReq := httptest.NewRequest(http.MethodGet, "/public/documents/"+docID.String(), nil)
	signedInResp, err := signedInApp.Test(signedInReq, -1)
	if err != nil {
		t.Fatalf("signed-in request failed: %v", err)
	}
	if signedInResp.StatusCode != http.StatusOK {
		t.Fatalf("expected signed-in 200, got %d", signedInResp.StatusCode)
	}
}

func TestGetPublicDocumentHandler_PublicDocumentAllowsSignedInNonMember(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	otherUserID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "global-public-doc")
	seedVerifiedUser(t, db, otherUserID, otherUserID.String()+"@example.com")
	if err := db.Model(&models.Document{}).Where("id = ?", docID).Update("public_access", PublicAccessGlobal).Error; err != nil {
		t.Fatalf("set public access: %v", err)
	}

	app := newWorkspaceTestApp(otherUserID)
	req := httptest.NewRequest(http.MethodGet, "/public/documents/"+docID.String(), nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestGetPublicDocumentHandler_StillWorksWhenCollaborationDisabled(t *testing.T) {
	t.Setenv("COLLABORATION_ENABLED", "false")

	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	otherUserID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "public-doc-with-collab-off")
	seedVerifiedUser(t, db, otherUserID, otherUserID.String()+"@example.com")
	if err := db.Model(&models.Document{}).Where("id = ?", docID).Update("public_access", PublicAccessGlobal).Error; err != nil {
		t.Fatalf("set public access: %v", err)
	}

	app := newWorkspaceTestApp(otherUserID)
	req := httptest.NewRequest(http.MethodGet, "/public/documents/"+docID.String(), nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 when collaboration disabled, got %d", resp.StatusCode)
	}
}

func TestDocumentSettingsACL_EditorCannotUpdateExcerptOrPublicAccess(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	editorID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "acl-doc")
	seedWorkspacePermission(t, db, docID, editorID, ownerID, "editor")

	app := newWorkspaceTestApp(editorID)

	excerptReq := httptest.NewRequest(http.MethodPut, "/documents/"+docID.String()+"/excerpt", bytes.NewBufferString(`{"excerpt":"manual"}`))
	excerptReq.Header.Set("Content-Type", "application/json")
	excerptResp, err := app.Test(excerptReq, -1)
	if err != nil {
		t.Fatalf("excerpt request failed: %v", err)
	}
	if excerptResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected excerpt update 404, got %d", excerptResp.StatusCode)
	}

	publicReq := httptest.NewRequest(http.MethodPut, "/documents/"+docID.String()+"/public-access", bytes.NewBufferString(`{"publicAccess":"public"}`))
	publicReq.Header.Set("Content-Type", "application/json")
	publicResp, err := app.Test(publicReq, -1)
	if err != nil {
		t.Fatalf("public-access request failed: %v", err)
	}
	if publicResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected public-access update 404, got %d", publicResp.StatusCode)
	}
}

func TestUpdateDocumentPublicAccessHandler_OwnerCanUpdateWhenCollaborationDisabled(t *testing.T) {
	t.Setenv("COLLABORATION_ENABLED", "false")

	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "owner-public-access")

	app := newWorkspaceTestApp(ownerID)
	req := httptest.NewRequest(http.MethodPut, "/documents/"+docID.String()+"/public-access", bytes.NewBufferString(`{"publicAccess":"public"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 when collaboration disabled, got %d", resp.StatusCode)
	}

	var doc models.Document
	if err := db.First(&doc, "id = ?", docID).Error; err != nil {
		t.Fatalf("reload document: %v", err)
	}
	if normalizePublicAccess(doc.PublicAccess) != PublicAccessGlobal {
		t.Fatalf("expected public access to be updated, got %q", doc.PublicAccess)
	}
}

func TestListDocumentMembersHandler_RequiresMemberManagementAccess(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	viewerID := uuid.New()
	editorID := uuid.New()
	collaboratorID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")
	seedWorkspacePermission(t, db, docID, viewerID, ownerID, "viewer")
	seedWorkspacePermission(t, db, docID, editorID, ownerID, "editor")
	seedWorkspacePermission(t, db, docID, collaboratorID, ownerID, "collaborator")

	tests := []struct {
		name       string
		userID     uuid.UUID
		wantStatus int
	}{
		{name: "owner", userID: ownerID, wantStatus: http.StatusOK},
		{name: "collaborator", userID: collaboratorID, wantStatus: http.StatusOK},
		{name: "editor", userID: editorID, wantStatus: http.StatusNotFound},
		{name: "viewer", userID: viewerID, wantStatus: http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := newWorkspaceTestApp(tt.userID)
			req := httptest.NewRequest(http.MethodGet, "/documents/"+docID.String()+"/shares", nil)
			resp, err := app.Test(req, -1)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			if resp.StatusCode != tt.wantStatus {
				t.Fatalf("expected %d, got %d", tt.wantStatus, resp.StatusCode)
			}
		})
	}
}

func TestShareDocumentHandler_CreatesPermission(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	targetUserID := uuid.New()
	seedVerifiedUser(t, db, targetUserID, targetUserID.String()+"@example.com")
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")

	app := newWorkspaceTestApp(ownerID)
	body := bytes.NewBufferString(`{"userId":"` + targetUserID.String() + `","role":"editor"}`)
	req := httptest.NewRequest(http.MethodPost, "/documents/"+docID.String()+"/shares", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestLeaveSharedDocumentHandler_RemovesOnlySelfPermission(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	sharedUserID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")
	seedWorkspacePermission(t, db, docID, sharedUserID, ownerID, "editor")

	app := newWorkspaceTestApp(sharedUserID)
	req := httptest.NewRequest(http.MethodDelete, "/documents/"+docID.String()+"/shares/me", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.StatusCode)
	}
}

func TestPermanentDeleteHandler_EmptyBodyClearsTrashWithoutWhereError(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	userID := uuid.New()
	folderID := seedFolderForWorkspace(t, db, userID, "trash-folder")
	docID := seedDocumentForWorkspace(t, db, userID, "trash-doc")

	if err := db.Model(&models.Document{}).Where("id = ?", docID).Update("folder_id", folderID).Error; err != nil {
		t.Fatalf("attach document to folder: %v", err)
	}
	if err := db.Delete(&models.Document{}, "id = ?", docID).Error; err != nil {
		t.Fatalf("soft delete doc: %v", err)
	}
	if err := db.Delete(&models.Folder{}, "id = ?", folderID).Error; err != nil {
		t.Fatalf("soft delete folder: %v", err)
	}

	app := newWorkspaceTestApp(userID)
	req := httptest.NewRequest(http.MethodDelete, "/trash", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var docCount int64
	if err := db.Unscoped().Model(&models.Document{}).Where("id = ?", docID).Count(&docCount).Error; err != nil {
		t.Fatalf("count doc: %v", err)
	}
	if docCount != 0 {
		t.Fatalf("expected doc removed, count=%d", docCount)
	}
	var folderCount int64
	if err := db.Unscoped().Model(&models.Folder{}).Where("id = ?", folderID).Count(&folderCount).Error; err != nil {
		t.Fatalf("count folder: %v", err)
	}
	if folderCount != 0 {
		t.Fatalf("expected folder removed, count=%d", folderCount)
	}
}

func TestInviteDocumentByEmailHandler_CreatesPermissionAndNotification(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	inviteeID := uuid.New()
	seedVerifiedUser(t, db, ownerID, ownerID.String()+"@example.com")
	seedVerifiedUser(t, db, inviteeID, "invitee@example.com")
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")

	app := newWorkspaceTestApp(ownerID)
	body := bytes.NewBufferString(`{"email":"invitee@example.com","role":"editor"}`)
	req := httptest.NewRequest(http.MethodPost, "/documents/"+docID.String()+"/invites", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var permissionCount int64
	if err := db.Model(&models.DocumentPermission{}).
		Where("document_id = ? AND user_id = ? AND deleted_at IS NULL", docID, inviteeID).
		Count(&permissionCount).Error; err != nil {
		t.Fatalf("count permissions: %v", err)
	}
	if permissionCount != 0 {
		t.Fatalf("expected no active permission before invite acceptance, got %d", permissionCount)
	}

	var notificationCount int64
	if err := db.Model(&models.Notification{}).Where("user_id = ? AND type = ?", inviteeID, "document_invite").Count(&notificationCount).Error; err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if notificationCount != 1 {
		t.Fatalf("expected 1 notification, got %d", notificationCount)
	}
}

func TestDeclineDocumentInviteHandler_RemovesPermission(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	inviteeID := uuid.New()
	seedVerifiedUser(t, db, ownerID, ownerID.String()+"@example.com")
	seedVerifiedUser(t, db, inviteeID, "invitee@example.com")
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")

	ownerApp := newWorkspaceTestApp(ownerID)
	inviteBody := bytes.NewBufferString(`{"email":"invitee@example.com","role":"viewer"}`)
	inviteReq := httptest.NewRequest(http.MethodPost, "/documents/"+docID.String()+"/invites", inviteBody)
	inviteReq.Header.Set("Content-Type", "application/json")
	inviteResp, err := ownerApp.Test(inviteReq, -1)
	if err != nil {
		t.Fatalf("invite request failed: %v", err)
	}
	if inviteResp.StatusCode != http.StatusOK {
		t.Fatalf("expected invite 200, got %d", inviteResp.StatusCode)
	}

	var invite models.DocumentInvite
	if err := db.Where("document_id = ? AND invitee_user_id = ?", docID, inviteeID).First(&invite).Error; err != nil {
		t.Fatalf("load invite: %v", err)
	}

	inviteeApp := newWorkspaceTestApp(inviteeID)
	declineReq := httptest.NewRequest(http.MethodPost, "/document-invites/"+invite.ID.String()+"/decline", nil)
	declineResp, err := inviteeApp.Test(declineReq, -1)
	if err != nil {
		t.Fatalf("decline request failed: %v", err)
	}
	if declineResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected decline 204, got %d", declineResp.StatusCode)
	}

	var permissionCount int64
	if err := db.Model(&models.DocumentPermission{}).
		Where("document_id = ? AND user_id = ? AND deleted_at IS NULL", docID, inviteeID).
		Count(&permissionCount).Error; err != nil {
		t.Fatalf("count permissions: %v", err)
	}
	if permissionCount != 0 {
		t.Fatalf("expected permission removed, got %d", permissionCount)
	}
}

func TestAcceptDocumentInviteHandler_UpdatesInviteStatusAndMarksNotificationRead(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	inviteeID := uuid.New()
	seedVerifiedUser(t, db, ownerID, ownerID.String()+"@example.com")
	seedVerifiedUser(t, db, inviteeID, "invitee@example.com")
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")

	ownerApp := newWorkspaceTestApp(ownerID)
	inviteBody := bytes.NewBufferString(`{"email":"invitee@example.com","role":"collaborator"}`)
	inviteReq := httptest.NewRequest(http.MethodPost, "/documents/"+docID.String()+"/invites", inviteBody)
	inviteReq.Header.Set("Content-Type", "application/json")
	inviteResp, err := ownerApp.Test(inviteReq, -1)
	if err != nil {
		t.Fatalf("invite request failed: %v", err)
	}
	if inviteResp.StatusCode != http.StatusOK {
		t.Fatalf("expected invite 200, got %d", inviteResp.StatusCode)
	}

	var invite models.DocumentInvite
	if err := db.Where("document_id = ? AND invitee_user_id = ?", docID, inviteeID).First(&invite).Error; err != nil {
		t.Fatalf("load invite: %v", err)
	}

	inviteeApp := newWorkspaceTestApp(inviteeID)
	acceptReq := httptest.NewRequest(http.MethodPost, "/document-invites/"+invite.ID.String()+"/accept", nil)
	acceptResp, err := inviteeApp.Test(acceptReq, -1)
	if err != nil {
		t.Fatalf("accept request failed: %v", err)
	}
	if acceptResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected accept 204, got %d", acceptResp.StatusCode)
	}

	var updatedInvite models.DocumentInvite
	if err := db.Where("id = ?", invite.ID).First(&updatedInvite).Error; err != nil {
		t.Fatalf("reload invite: %v", err)
	}
	if updatedInvite.Status != "accepted" {
		t.Fatalf("expected accepted status, got %s", updatedInvite.Status)
	}

	var notification models.Notification
	if err := db.Where("user_id = ? AND type = ?", inviteeID, "document_invite").First(&notification).Error; err != nil {
		t.Fatalf("load notification: %v", err)
	}
	if notification.ReadAt == nil {
		t.Fatalf("expected notification to be marked as read")
	}

	var permission models.DocumentPermission
	if err := db.Where("document_id = ? AND user_id = ? AND deleted_at IS NULL", docID, inviteeID).First(&permission).Error; err != nil {
		t.Fatalf("load permission after accept: %v", err)
	}
	if permission.Role != "collaborator" {
		t.Fatalf("expected collaborator role after accept, got %s", permission.Role)
	}

	// 权限生效校验：接受后可读取该共享文档
	readReq := httptest.NewRequest(http.MethodGet, "/files/"+docID.String()+"?type=document", nil)
	readResp, err := inviteeApp.Test(readReq, -1)
	if err != nil {
		t.Fatalf("read shared file request failed: %v", err)
	}
	if readResp.StatusCode != http.StatusOK {
		t.Fatalf("expected read shared document 200 after accept, got %d", readResp.StatusCode)
	}
}

func TestDeclineDocumentInviteHandler_DeniesSharedDocumentRead(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	inviteeID := uuid.New()
	seedVerifiedUser(t, db, ownerID, ownerID.String()+"@example.com")
	seedVerifiedUser(t, db, inviteeID, "invitee@example.com")
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")

	ownerApp := newWorkspaceTestApp(ownerID)
	inviteBody := bytes.NewBufferString(`{"email":"invitee@example.com","role":"viewer"}`)
	inviteReq := httptest.NewRequest(http.MethodPost, "/documents/"+docID.String()+"/invites", inviteBody)
	inviteReq.Header.Set("Content-Type", "application/json")
	inviteResp, err := ownerApp.Test(inviteReq, -1)
	if err != nil {
		t.Fatalf("invite request failed: %v", err)
	}
	if inviteResp.StatusCode != http.StatusOK {
		t.Fatalf("expected invite 200, got %d", inviteResp.StatusCode)
	}

	var invite models.DocumentInvite
	if err := db.Where("document_id = ? AND invitee_user_id = ?", docID, inviteeID).First(&invite).Error; err != nil {
		t.Fatalf("load invite: %v", err)
	}

	inviteeApp := newWorkspaceTestApp(inviteeID)
	declineReq := httptest.NewRequest(http.MethodPost, "/document-invites/"+invite.ID.String()+"/decline", nil)
	declineResp, err := inviteeApp.Test(declineReq, -1)
	if err != nil {
		t.Fatalf("decline request failed: %v", err)
	}
	if declineResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected decline 204, got %d", declineResp.StatusCode)
	}

	readReq := httptest.NewRequest(http.MethodGet, "/files/"+docID.String()+"?type=document", nil)
	readResp, err := inviteeApp.Test(readReq, -1)
	if err != nil {
		t.Fatalf("read shared file request failed: %v", err)
	}
	if readResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected read shared document 404 after decline, got %d", readResp.StatusCode)
	}
}

func TestClearNotificationsHandler_RemovesAllUserNotifications(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	userID := uuid.New()
	otherUserID := uuid.New()
	seedVerifiedUser(t, db, userID, "user@example.com")
	seedVerifiedUser(t, db, otherUserID, "other@example.com")

	if err := db.Create(&models.Notification{
		ID:       uuid.New(),
		UserID:   userID,
		Type:     "document_invite",
		GroupKey: "doc:test-1",
		DataJSON: `{"inviteId":"a"}`,
	}).Error; err != nil {
		t.Fatalf("create notification 1: %v", err)
	}
	if err := db.Create(&models.Notification{
		ID:       uuid.New(),
		UserID:   userID,
		Type:     "document_invite",
		GroupKey: "doc:test-2",
		DataJSON: `{"inviteId":"b"}`,
	}).Error; err != nil {
		t.Fatalf("create notification 2: %v", err)
	}
	if err := db.Create(&models.Notification{
		ID:       uuid.New(),
		UserID:   otherUserID,
		Type:     "document_invite",
		GroupKey: "doc:test-3",
		DataJSON: `{"inviteId":"c"}`,
	}).Error; err != nil {
		t.Fatalf("create other notification: %v", err)
	}

	app := newWorkspaceTestApp(userID)
	req := httptest.NewRequest(http.MethodDelete, "/notifications", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("clear request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var userCount int64
	if err := db.Model(&models.Notification{}).Where("user_id = ?", userID).Count(&userCount).Error; err != nil {
		t.Fatalf("count user notifications: %v", err)
	}
	if userCount != 0 {
		t.Fatalf("expected user notifications cleared, got %d", userCount)
	}

	var otherCount int64
	if err := db.Model(&models.Notification{}).Where("user_id = ?", otherUserID).Count(&otherCount).Error; err != nil {
		t.Fatalf("count other notifications: %v", err)
	}
	if otherCount != 1 {
		t.Fatalf("expected other user notifications untouched, got %d", otherCount)
	}
}

func TestInviteAcceptNotificationFlow_EndToEnd(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	inviteeID := uuid.New()
	seedVerifiedUser(t, db, ownerID, ownerID.String()+"@example.com")
	seedVerifiedUser(t, db, inviteeID, "invitee@example.com")
	docID := seedDocumentForWorkspace(t, db, ownerID, "flow-doc")

	ownerApp := newWorkspaceTestApp(ownerID)
	inviteBody := bytes.NewBufferString(`{"email":"invitee@example.com","role":"viewer"}`)
	inviteReq := httptest.NewRequest(http.MethodPost, "/documents/"+docID.String()+"/invites", inviteBody)
	inviteReq.Header.Set("Content-Type", "application/json")
	inviteResp, err := ownerApp.Test(inviteReq, -1)
	if err != nil {
		t.Fatalf("invite request failed: %v", err)
	}
	if inviteResp.StatusCode != http.StatusOK {
		t.Fatalf("expected invite 200, got %d", inviteResp.StatusCode)
	}

	inviteeApp := newWorkspaceTestApp(inviteeID)
	listUnreadReq := httptest.NewRequest(http.MethodGet, "/notifications?unread=1", nil)
	listUnreadResp, err := inviteeApp.Test(listUnreadReq, -1)
	if err != nil {
		t.Fatalf("list unread notifications failed: %v", err)
	}
	if listUnreadResp.StatusCode != http.StatusOK {
		t.Fatalf("expected unread list 200, got %d", listUnreadResp.StatusCode)
	}
	var unreadPayload NotificationListResponse
	if err := json.NewDecoder(listUnreadResp.Body).Decode(&unreadPayload); err != nil {
		t.Fatalf("decode unread list: %v", err)
	}
	if unreadPayload.UnreadCount != 1 || len(unreadPayload.Items) != 1 {
		t.Fatalf("expected one unread notification, got unread=%d items=%d", unreadPayload.UnreadCount, len(unreadPayload.Items))
	}

	var invite models.DocumentInvite
	if err := db.Where("document_id = ? AND invitee_user_id = ?", docID, inviteeID).First(&invite).Error; err != nil {
		t.Fatalf("load invite: %v", err)
	}
	acceptReq := httptest.NewRequest(http.MethodPost, "/document-invites/"+invite.ID.String()+"/accept", nil)
	acceptResp, err := inviteeApp.Test(acceptReq, -1)
	if err != nil {
		t.Fatalf("accept request failed: %v", err)
	}
	if acceptResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected accept 204, got %d", acceptResp.StatusCode)
	}

	readReq := httptest.NewRequest(http.MethodGet, "/files/"+docID.String()+"?type=document", nil)
	readResp, err := inviteeApp.Test(readReq, -1)
	if err != nil {
		t.Fatalf("read shared file request failed: %v", err)
	}
	if readResp.StatusCode != http.StatusOK {
		t.Fatalf("expected read shared document 200 after accept, got %d", readResp.StatusCode)
	}

	listUnreadAfterReq := httptest.NewRequest(http.MethodGet, "/notifications?unread=1", nil)
	listUnreadAfterResp, err := inviteeApp.Test(listUnreadAfterReq, -1)
	if err != nil {
		t.Fatalf("list unread notifications after accept failed: %v", err)
	}
	if listUnreadAfterResp.StatusCode != http.StatusOK {
		t.Fatalf("expected unread-after list 200, got %d", listUnreadAfterResp.StatusCode)
	}
	var unreadAfterPayload NotificationListResponse
	if err := json.NewDecoder(listUnreadAfterResp.Body).Decode(&unreadAfterPayload); err != nil {
		t.Fatalf("decode unread-after list: %v", err)
	}
	if unreadAfterPayload.UnreadCount != 0 {
		t.Fatalf("expected unread count 0 after accept, got %d", unreadAfterPayload.UnreadCount)
	}

	clearReq := httptest.NewRequest(http.MethodDelete, "/notifications", nil)
	clearResp, err := inviteeApp.Test(clearReq, -1)
	if err != nil {
		t.Fatalf("clear notifications failed: %v", err)
	}
	if clearResp.StatusCode != http.StatusOK {
		t.Fatalf("expected clear 200, got %d", clearResp.StatusCode)
	}

	listAllReq := httptest.NewRequest(http.MethodGet, "/notifications", nil)
	listAllResp, err := inviteeApp.Test(listAllReq, -1)
	if err != nil {
		t.Fatalf("list notifications after clear failed: %v", err)
	}
	if listAllResp.StatusCode != http.StatusOK {
		t.Fatalf("expected list-all 200, got %d", listAllResp.StatusCode)
	}
	var allPayload NotificationListResponse
	if err := json.NewDecoder(listAllResp.Body).Decode(&allPayload); err != nil {
		t.Fatalf("decode list-all: %v", err)
	}
	if allPayload.Total != 0 || len(allPayload.Items) != 0 {
		t.Fatalf("expected no notifications after clear, total=%d items=%d", allPayload.Total, len(allPayload.Items))
	}
}
