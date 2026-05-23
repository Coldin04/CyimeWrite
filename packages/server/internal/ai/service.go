package ai

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/acl"
	"g.co1d.in/Coldin04/Cyime/server/internal/content"
	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"g.co1d.in/Coldin04/Cyime/server/internal/workspace"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrUnsupportedFormat = errors.New("unsupported content format")
	ErrVersionConflict   = errors.New("document content version conflict")
)

type MarkdownContent struct {
	Format    string    `json:"format"`
	Content   string    `json:"content"`
	Version   int64     `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type MarkdownUpdateResult struct {
	Success   bool      `json:"success"`
	Version   int64     `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type CreateMarkdownDocumentInput struct {
	Title                  string
	Content                string
	FolderID               *uuid.UUID
	PreferredImageTargetID string
}

type CreateMarkdownDocumentResult struct {
	ID        uuid.UUID  `json:"id"`
	Type      string     `json:"type"`
	Title     string     `json:"title"`
	FolderID  *uuid.UUID `json:"folderId,omitempty"`
	Version   int64      `json:"version"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

func GetMarkdownContent(userID uuid.UUID, documentID uuid.UUID) (*MarkdownContent, error) {
	result, err := content.GetContent(userID, documentID)
	if err != nil {
		return nil, err
	}
	markdown, err := contentJSONToMarkdown(result.ContentJSON)
	if err != nil {
		return nil, err
	}
	return &MarkdownContent{
		Format:    "markdown",
		Content:   markdown,
		Version:   result.ContentVersion,
		UpdatedAt: result.UpdatedAt,
	}, nil
}

func UpdateMarkdownContent(userID uuid.UUID, documentID uuid.UUID, markdown string, baseVersion *int64) (*MarkdownUpdateResult, error) {
	contentJSON, err := markdownToContentJSON(markdown)
	if err != nil {
		return nil, err
	}

	var result *content.UpdateContentResult
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		document, err := acl.CanEditDocument(tx, userID, documentID)
		if err != nil {
			return content.ErrDocumentNotFoundOrUnauthorized
		}

		if baseVersion != nil {
			var body models.DocumentBody
			if err := tx.Where("document_id = ?", documentID).First(&body).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return content.ErrDocumentContentNotFound
				}
				return err
			}
			if body.ContentVersion != *baseVersion {
				return ErrVersionConflict
			}
		}

		result, err = content.PersistCanonicalContent(tx, document, userID, contentJSON, nil)
		return err
	})
	if err != nil {
		return nil, err
	}
	return &MarkdownUpdateResult{
		Success:   result.Success,
		Version:   result.ContentVersion,
		UpdatedAt: result.UpdatedAt,
	}, nil
}

func PatchMarkdownContent(userID uuid.UUID, documentID uuid.UUID, operations []PatchOperation, baseVersion *int64) (*MarkdownUpdateResult, error) {
	current, err := GetMarkdownContent(userID, documentID)
	if err != nil {
		return nil, err
	}
	if baseVersion != nil && current.Version != *baseVersion {
		return nil, ErrVersionConflict
	}

	patched, err := applyMarkdownPatch(current.Content, operations)
	if err != nil {
		return nil, err
	}
	return UpdateMarkdownContent(userID, documentID, patched, &current.Version)
}

func CreateMarkdownDocument(userID uuid.UUID, input CreateMarkdownDocumentInput) (*CreateMarkdownDocumentResult, error) {
	contentJSON, err := markdownToContentJSON(input.Content)
	if err != nil {
		return nil, err
	}
	document, err := workspace.CreateDocument(
		userID,
		input.Title,
		string(contentJSON),
		input.FolderID,
		"rich_text",
		input.PreferredImageTargetID,
	)
	if err != nil {
		return nil, err
	}

	var body models.DocumentBody
	if err := database.DB.Select("content_version").Where("document_id = ?", document.ID).First(&body).Error; err != nil {
		return nil, err
	}

	return &CreateMarkdownDocumentResult{
		ID:        document.ID,
		Type:      "document",
		Title:     document.Title,
		FolderID:  document.FolderID,
		Version:   body.ContentVersion,
		CreatedAt: document.CreatedAt,
		UpdatedAt: document.UpdatedAt,
	}, nil
}

func normalizeFormat(format string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(format))
	if normalized == "" {
		normalized = "markdown"
	}
	if normalized != "markdown" {
		return "", ErrUnsupportedFormat
	}
	return normalized, nil
}

func parseDocumentID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid document id")
	}
	return id, nil
}
