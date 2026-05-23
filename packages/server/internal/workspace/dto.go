package workspace

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// CreatorInfo represents the creator information in responses.
type CreatorInfo struct {
	ID          uuid.UUID `json:"id"`
	DisplayName *string   `json:"displayName"`
}

// FileItem represents a unified file item (folder or document) in the response.
type FileItem struct {
	ID                     uuid.UUID   `json:"id"`
	Type                   string      `json:"type"` // "folder" | "document"
	DocumentType           *string     `json:"documentType,omitempty"`
	PreferredImageTargetID *string     `json:"preferredImageTargetId,omitempty"`
	MyRole                 *string     `json:"myRole,omitempty"`
	PublicAccess           *string     `json:"publicAccess,omitempty"`
	PublicURL              *string     `json:"publicUrl,omitempty"`
	Name                   string      `json:"name"`
	Description            *string     `json:"description,omitempty"`
	ParentID               *uuid.UUID  `json:"parentId,omitempty"`
	FolderID               *uuid.UUID  `json:"folderId,omitempty"`
	Title                  *string     `json:"title,omitempty"`
	Excerpt                *string     `json:"excerpt,omitempty"`
	ManualExcerpt          *string     `json:"manualExcerpt,omitempty"`
	CreatedAt              time.Time   `json:"createdAt"`
	UpdatedAt              time.Time   `json:"updatedAt"`
	Creator                CreatorInfo `json:"creator"`
}

type FileListResponse struct {
	Items   []FileItem `json:"items"`
	HasMore bool       `json:"hasMore"`
	Total   int64      `json:"total"`
}

type CreateFolderRequest struct {
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	ParentID    *uuid.UUID `json:"parentId"`
}

type CreateFolderResponse struct {
	ID          uuid.UUID   `json:"id"`
	Type        string      `json:"type"`
	Name        string      `json:"name"`
	Description *string     `json:"description,omitempty"`
	ParentID    *uuid.UUID  `json:"parentId,omitempty"`
	CreatedAt   time.Time   `json:"createdAt"`
	UpdatedAt   time.Time   `json:"updatedAt"`
	Creator     CreatorInfo `json:"creator"`
}

type CreateDocumentRequest struct {
	Title                  string          `json:"title"`
	ContentJSON            json.RawMessage `json:"contentJson"`
	FolderID               *uuid.UUID      `json:"folderId"`
	DocumentType           string          `json:"documentType"`
	PreferredImageTargetID string          `json:"preferredImageTargetId"`
}

type CreateDocumentResponse struct {
	ID                     uuid.UUID   `json:"id"`
	Type                   string      `json:"type"`
	DocumentType           string      `json:"documentType"`
	PreferredImageTargetID string      `json:"preferredImageTargetId"`
	Title                  string      `json:"title"`
	Excerpt                string      `json:"excerpt"`
	FolderID               *uuid.UUID  `json:"folderId,omitempty"`
	CreatedAt              time.Time   `json:"createdAt"`
	UpdatedAt              time.Time   `json:"updatedAt"`
	Creator                CreatorInfo `json:"creator"`
}

type DeleteResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type BatchDeleteRequest struct {
	Items []ItemToDelete `json:"items"`
}

type ItemToDelete struct {
	ID   uuid.UUID `json:"id"`
	Type string    `json:"type"` // "folder" | "document"
}

type BatchDeleteResponse struct {
	Success     bool         `json:"success"`
	Message     string       `json:"message"`
	FailedItems []FailedItem `json:"failedItems,omitempty"`
}

type FailedItem struct {
	ID     uuid.UUID `json:"id"`
	Type   string    `json:"type"`
	Reason string    `json:"reason"`
}

type AncestorItem struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type MoveDocumentRequest struct {
	FolderID *uuid.UUID `json:"folderId"`
}

type MoveFolderRequest struct {
	ParentID *uuid.UUID `json:"parentId"`
}

type MoveResponse struct {
	Success   bool      `json:"success"`
	Message   string    `json:"message"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type CopyFileRequest struct {
	Type                string     `json:"type"`
	DestinationFolderID *uuid.UUID `json:"destinationFolderId"`
	Name                string     `json:"name"`
}

type CopyResponse struct {
	Success bool      `json:"success"`
	Message string    `json:"message"`
	Item    *FileItem `json:"item,omitempty"`
}

type UpdateDocumentImageTargetRequest struct {
	PreferredImageTargetID string `json:"preferredImageTargetId"`
}

type UpdateDocumentExcerptRequest struct {
	Excerpt string `json:"excerpt"`
}

type UpdateDocumentPublicAccessRequest struct {
	PublicAccess string `json:"publicAccess"`
}

type DocumentPublicContentResponse struct {
	ID             uuid.UUID       `json:"id"`
	DocumentID     uuid.UUID       `json:"documentId"`
	ContentJSON    json.RawMessage `json:"contentJson"`
	PlainText      string          `json:"plainText"`
	ContentVersion int64           `json:"contentVersion"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

type SearchDocumentItem struct {
	ID                     uuid.UUID  `json:"id"`
	Title                  string     `json:"title"`
	Excerpt                string     `json:"excerpt"`
	DocumentType           string     `json:"documentType"`
	PreferredImageTargetID string     `json:"preferredImageTargetId"`
	MyRole                 string     `json:"myRole"`
	PublicAccess           string     `json:"publicAccess"`
	PublicURL              string     `json:"publicUrl"`
	FolderID               *uuid.UUID `json:"folderId,omitempty"`
	UpdatedAt              time.Time  `json:"updatedAt"`
}

type SearchFolderItem struct {
	ID        uuid.UUID  `json:"id"`
	Name      string     `json:"name"`
	ParentID  *uuid.UUID `json:"parentId,omitempty"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

type SearchMediaItem struct {
	ID            uuid.UUID  `json:"id"`
	Filename      string     `json:"filename"`
	Kind          string     `json:"kind"`
	MimeType      string     `json:"mimeType"`
	DocumentID    *uuid.UUID `json:"documentId,omitempty"`
	DocumentTitle *string    `json:"documentTitle,omitempty"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

type SearchResponse struct {
	Query     string               `json:"query"`
	Documents []SearchDocumentItem `json:"documents"`
	Folders   []SearchFolderItem   `json:"folders"`
	Media     []SearchMediaItem    `json:"media"`
	Total     int                  `json:"total"`
}

type BatchMoveRequest struct {
	Items               []ItemToMove `json:"items"`
	DestinationFolderID *uuid.UUID   `json:"destinationFolderId"`
}

type ItemToMove struct {
	ID   uuid.UUID `json:"id"`
	Type string    `json:"type"` // "folder" or "document"
}

type BatchMoveResponse struct {
	Success     bool         `json:"success"`
	Message     string       `json:"message"`
	MovedCount  int          `json:"movedCount"`
	FailedItems []FailedItem `json:"failedItems,omitempty"`
}

type ShareDocumentRequest struct {
	UserID uuid.UUID `json:"userId"`
	Role   string    `json:"role"`
}

type InviteDocumentByEmailRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

type ShareDocumentMember struct {
	UserID      uuid.UUID `json:"userId"`
	Role        string    `json:"role"`
	DisplayName *string   `json:"displayName,omitempty"`
	Email       *string   `json:"email,omitempty"`
}

type ShareDocumentResponse struct {
	DocumentID uuid.UUID             `json:"documentId"`
	Members    []ShareDocumentMember `json:"members"`
}

type SharedDocumentItem struct {
	DocumentID             uuid.UUID  `json:"documentId"`
	Title                  string     `json:"title"`
	Excerpt                string     `json:"excerpt"`
	DocumentType           string     `json:"documentType"`
	PreferredImageTargetID string     `json:"preferredImageTargetId"`
	FolderID               *uuid.UUID `json:"folderId,omitempty"`
	OwnerUserID            uuid.UUID  `json:"ownerUserId"`
	OwnerDisplayName       *string    `json:"ownerDisplayName,omitempty"`
	MyRole                 string     `json:"myRole"`
	CreatedAt              time.Time  `json:"createdAt"`
	UpdatedAt              time.Time  `json:"updatedAt"`
}

type SharedDocumentListResponse struct {
	Items   []SharedDocumentItem `json:"items"`
	HasMore bool                 `json:"hasMore"`
	Total   int64                `json:"total"`
}

type SharedDocumentSummaryResponse struct {
	HasSharedDocuments bool `json:"hasSharedDocuments"`
}

type OutgoingSharedDocumentItem struct {
	DocumentID             uuid.UUID  `json:"documentId"`
	Title                  string     `json:"title"`
	Excerpt                string     `json:"excerpt"`
	DocumentType           string     `json:"documentType"`
	PreferredImageTargetID string     `json:"preferredImageTargetId"`
	FolderID               *uuid.UUID `json:"folderId,omitempty"`
	MyRole                 string     `json:"myRole"`
	PublicAccess           string     `json:"publicAccess"`
	PublicURL              string     `json:"publicUrl"`
	SharedMemberCount      int64      `json:"sharedMemberCount"`
	CreatedAt              time.Time  `json:"createdAt"`
	UpdatedAt              time.Time  `json:"updatedAt"`
}

type OutgoingSharedDocumentListResponse struct {
	Items   []OutgoingSharedDocumentItem `json:"items"`
	HasMore bool                         `json:"hasMore"`
	Total   int64                        `json:"total"`
}

type NotificationItem struct {
	ID        uuid.UUID       `json:"id"`
	UserID    uuid.UUID       `json:"userId"`
	Type      string          `json:"type"`
	GroupKey  string          `json:"groupKey"`
	Data      json.RawMessage `json:"data"`
	ReadAt    *time.Time      `json:"readAt,omitempty"`
	CreatedAt time.Time       `json:"createdAt"`
	UpdatedAt time.Time       `json:"updatedAt"`
}

type NotificationListResponse struct {
	Items       []NotificationItem `json:"items"`
	HasMore     bool               `json:"hasMore"`
	Total       int64              `json:"total"`
	UnreadCount int64              `json:"unreadCount"`
}

// DocumentACLResponse represents the ACL information for a document
type DocumentACLResponse struct {
	MyRole           string `json:"myRole"` // "viewer", "editor", "collaborator", "owner"
	CanRead          bool   `json:"canRead"`
	CanEdit          bool   `json:"canEdit"`
	CanManageMembers bool   `json:"canManageMembers"`
}

// GetYjsStateResponse represents the Yjs state response
type GetYjsStateResponse struct {
	YjsState       string `json:"yjsState"`
	YjsStateVector string `json:"yjsStateVector"`
	YjsVersion     int64  `json:"yjsVersion"`
}

// UpdateYjsStateRequest represents the request to update Yjs state.
//
// ExpectedYjsVersion is the version the caller last observed. The handler
// only writes when it matches the current row, providing optimistic
// concurrency control. A value <= 0 is treated as "I have no version yet"
// and only succeeds when no row exists for the document (initial create).
type UpdateYjsStateRequest struct {
	YjsState           string          `json:"yjsState"`
	YjsStateVector     string          `json:"yjsStateVector"`
	ExpectedYjsVersion int64           `json:"expectedYjsVersion"`
	ContentJSON        json.RawMessage `json:"contentJson,omitempty"`
}

// YjsStateConflictResponse is returned when ExpectedYjsVersion mismatches the
// stored row, so the client can re-fetch the latest state and retry.
type YjsStateConflictResponse struct {
	Error          string `json:"error"`
	Message        string `json:"message"`
	CurrentVersion int64  `json:"currentYjsVersion"`
}
