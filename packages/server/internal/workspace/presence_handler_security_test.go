package workspace

import (
	"testing"

	"g.co1d.in/Coldin04/Cyime/server/internal/acl"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"github.com/google/uuid"
)

func TestPresenceAuthorizationReflectsPermissionRemoval(t *testing.T) {
	db := setupWorkspaceTestDB(t)
	ownerID := uuid.New()
	viewerID := uuid.New()
	docID := seedDocumentForWorkspace(t, db, ownerID, "shared-doc")
	seedVerifiedUser(t, db, viewerID, "viewer@example.com")
	seedWorkspacePermission(t, db, docID, viewerID, ownerID, acl.RoleViewer)

	if err := canReadDocumentForPresence(viewerID, docID); err != nil {
		t.Fatalf("expected viewer to read shared document before removal: %v", err)
	}

	if err := db.Where("document_id = ? AND user_id = ?", docID, viewerID).Delete(&models.DocumentPermission{}).Error; err != nil {
		t.Fatalf("remove viewer permission: %v", err)
	}

	resetPresenceTestState(t)

	if err := canReadDocumentForPresence(viewerID, docID); err == nil {
		t.Fatal("expected presence authorization to deny viewer after permission removal")
	}
}
