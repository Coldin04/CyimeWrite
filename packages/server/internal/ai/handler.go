package ai

import (
	"errors"
	"strings"

	"g.co1d.in/Coldin04/Cyime/server/internal/content"
	"g.co1d.in/Coldin04/Cyime/server/internal/workspace"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

type UpdateMarkdownRequest struct {
	Format  string `json:"format"`
	Content string `json:"content"`
}

type PatchMarkdownRequest struct {
	Format     string           `json:"format"`
	Operations []PatchOperation `json:"operations"`
}

type CreateMarkdownDocumentRequest struct {
	Title                  string     `json:"title"`
	Format                 string     `json:"format"`
	Content                string     `json:"content"`
	FolderID               *uuid.UUID `json:"folderId"`
	PreferredImageTargetID string     `json:"preferredImageTargetId"`
}

type CreateFolderRequest struct {
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	ParentID    *uuid.UUID `json:"parentId"`
}

type RenameFileRequest struct {
	Type string `json:"type"`
	Name string `json:"name"`
}

type MoveFileRequest struct {
	Type                string     `json:"type"`
	DestinationFolderID *uuid.UUID `json:"destinationFolderId"`
}

type CopyFileRequest struct {
	Type                string     `json:"type"`
	DestinationFolderID *uuid.UUID `json:"destinationFolderId"`
	Name                string     `json:"name"`
}

type DeleteFileResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type FileOperationResponse struct {
	Success bool                `json:"success"`
	Item    *workspace.FileItem `json:"item,omitempty"`
}

func GetDocumentMarkdownHandler(c *fiber.Ctx) error {
	userID, documentID, ok := parseRequestIDs(c)
	if !ok {
		return nil
	}
	if _, err := normalizeFormat(c.Query("format", "markdown")); err != nil {
		return badRequest(c, err)
	}

	result, err := GetMarkdownContent(userID, documentID)
	if err != nil {
		return contentError(c, err)
	}
	return c.JSON(result)
}

func UpdateDocumentMarkdownHandler(c *fiber.Ctx) error {
	userID, documentID, ok := parseRequestIDs(c)
	if !ok {
		return nil
	}

	var req UpdateMarkdownRequest
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c, err)
	}
	if _, err := normalizeFormat(req.Format); err != nil {
		return badRequest(c, err)
	}

	result, err := UpdateMarkdownContent(userID, documentID, req.Content)
	if err != nil {
		return contentError(c, err)
	}
	return c.JSON(result)
}

func PatchDocumentMarkdownHandler(c *fiber.Ctx) error {
	userID, documentID, ok := parseRequestIDs(c)
	if !ok {
		return nil
	}

	var req PatchMarkdownRequest
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c, err)
	}
	if _, err := normalizeFormat(req.Format); err != nil {
		return badRequest(c, err)
	}
	if len(req.Operations) == 0 {
		return badRequest(c, errors.New("at least one patch operation is required"))
	}

	result, err := PatchMarkdownContent(userID, documentID, req.Operations)
	if err != nil {
		return contentError(c, err)
	}
	return c.JSON(result)
}

func CreateMarkdownDocumentHandler(c *fiber.Ctx) error {
	userID, err := userIDFromContext(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{Error: "Unauthorized", Message: "Invalid user context"})
	}

	var req CreateMarkdownDocumentRequest
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c, err)
	}
	if _, err := normalizeFormat(req.Format); err != nil {
		return badRequest(c, err)
	}

	result, err := CreateMarkdownDocument(userID, CreateMarkdownDocumentInput{
		Title:                  req.Title,
		Content:                req.Content,
		FolderID:               req.FolderID,
		PreferredImageTargetID: req.PreferredImageTargetID,
	})
	if err != nil {
		return workspaceError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(result)
}

func CreateFolderHandler(c *fiber.Ctx) error {
	userID, err := userIDFromContext(c)
	if err != nil {
		return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{Error: "Unauthorized", Message: "Invalid user context"})
	}

	var req CreateFolderRequest
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c, err)
	}

	folder, err := workspace.CreateFolder(userID, req.Name, req.Description, req.ParentID)
	if err != nil {
		return workspaceError(c, err)
	}

	return c.Status(fiber.StatusCreated).JSON(workspace.CreateFolderResponse{
		ID:          folder.ID,
		Type:        "folder",
		Name:        folder.Name,
		Description: folder.Description,
		ParentID:    folder.ParentID,
		CreatedAt:   folder.CreatedAt,
		UpdatedAt:   folder.UpdatedAt,
		Creator: workspace.CreatorInfo{
			ID: userID,
		},
	})
}

func RenameFileHandler(c *fiber.Ctx) error {
	userID, fileID, ok := parseFileRequestID(c)
	if !ok {
		return nil
	}

	var req RenameFileRequest
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c, err)
	}
	fileType, err := normalizeFileType(req.Type)
	if err != nil {
		return badRequest(c, err)
	}

	switch fileType {
	case "document":
		err = workspace.UpdateDocumentTitle(userID, fileID, req.Name)
	case "folder":
		err = workspace.UpdateFolderName(userID, fileID, req.Name)
	}
	if err != nil {
		return workspaceError(c, err)
	}

	item, err := workspace.GetFile(userID, fileID, fileType)
	if err != nil {
		return workspaceError(c, err)
	}
	return c.JSON(FileOperationResponse{Success: true, Item: item})
}

func MoveFileHandler(c *fiber.Ctx) error {
	userID, fileID, ok := parseFileRequestID(c)
	if !ok {
		return nil
	}

	var req MoveFileRequest
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c, err)
	}
	fileType, err := normalizeFileType(req.Type)
	if err != nil {
		return badRequest(c, err)
	}

	switch fileType {
	case "document":
		_, err = workspace.MoveDocument(userID, fileID, req.DestinationFolderID)
	case "folder":
		_, err = workspace.MoveFolder(userID, fileID, req.DestinationFolderID)
	}
	if err != nil {
		return workspaceError(c, err)
	}

	item, err := workspace.GetFile(userID, fileID, fileType)
	if err != nil {
		return workspaceError(c, err)
	}
	return c.JSON(FileOperationResponse{Success: true, Item: item})
}

func CopyFileHandler(c *fiber.Ctx) error {
	userID, fileID, ok := parseFileRequestID(c)
	if !ok {
		return nil
	}

	var req CopyFileRequest
	if err := c.BodyParser(&req); err != nil {
		return badRequest(c, err)
	}
	fileType, err := normalizeFileType(req.Type)
	if err != nil {
		return badRequest(c, err)
	}

	item, err := workspace.CopyFile(userID, fileID, fileType, req.DestinationFolderID, req.Name)
	if err != nil {
		return workspaceError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(FileOperationResponse{Success: true, Item: item})
}

func DeleteFileHandler(c *fiber.Ctx) error {
	userID, fileID, ok := parseFileRequestID(c)
	if !ok {
		return nil
	}

	fileType, err := normalizeFileType(c.Query("type"))
	if err != nil {
		return badRequest(c, err)
	}

	if err := workspace.DeleteFile(userID, fileID, fileType); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Not Found", Message: err.Error()})
	}

	return c.JSON(DeleteFileResponse{Success: true, Message: "File moved to trash"})
}

func parseRequestIDs(c *fiber.Ctx) (uuid.UUID, uuid.UUID, bool) {
	userID, err := userIDFromContext(c)
	if err != nil {
		_ = c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{Error: "Unauthorized", Message: "Invalid user context"})
		return uuid.Nil, uuid.Nil, false
	}
	documentID, err := parseDocumentID(c.Params("id"))
	if err != nil {
		_ = badRequest(c, err)
		return uuid.Nil, uuid.Nil, false
	}
	return userID, documentID, true
}

func parseFileRequestID(c *fiber.Ctx) (uuid.UUID, uuid.UUID, bool) {
	userID, err := userIDFromContext(c)
	if err != nil {
		_ = c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{Error: "Unauthorized", Message: "Invalid user context"})
		return uuid.Nil, uuid.Nil, false
	}
	fileID, err := uuid.Parse(strings.TrimSpace(c.Params("id")))
	if err != nil {
		_ = badRequest(c, errors.New("invalid file id"))
		return uuid.Nil, uuid.Nil, false
	}
	return userID, fileID, true
}

func userIDFromContext(c *fiber.Ctx) (uuid.UUID, error) {
	userIDStr, ok := c.Locals("userId").(string)
	if !ok {
		return uuid.Nil, fiber.ErrUnauthorized
	}
	return uuid.Parse(userIDStr)
}

func badRequest(c *fiber.Ctx, err error) error {
	return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Bad Request", Message: err.Error()})
}

func normalizeFileType(fileType string) (string, error) {
	switch strings.TrimSpace(fileType) {
	case "document":
		return "document", nil
	case "folder":
		return "folder", nil
	default:
		return "", errors.New("type must be document or folder")
	}
}

func contentError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, content.ErrDocumentNotFoundOrUnauthorized), errors.Is(err, content.ErrDocumentContentNotFound):
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Not Found", Message: err.Error()})
	case errors.Is(err, ErrVersionConflict):
		return c.Status(fiber.StatusConflict).JSON(ErrorResponse{Error: "Conflict", Message: err.Error()})
	case errors.Is(err, ErrMarkdownConverterUnavailable):
		return c.Status(fiber.StatusBadGateway).JSON(ErrorResponse{Error: "Markdown Converter Unavailable", Message: err.Error()})
	case errors.Is(err, ErrMarkdownConversionFailed):
		return badRequest(c, err)
	case errors.Is(err, ErrUnsupportedFormat), errors.Is(err, content.ErrInvalidContentJSON), errors.Is(err, content.ErrContentJSONTooLarge), errors.Is(err, content.ErrInvalidContentAssetReferences), errors.Is(err, content.ErrWorkspaceStorageQuotaExceeded):
		return badRequest(c, err)
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
	}
}

func workspaceError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, workspace.ErrDocumentTitleRequired),
		errors.Is(err, workspace.ErrDocumentTitleTooLong),
		errors.Is(err, workspace.ErrDuplicateDocumentTitle),
		errors.Is(err, workspace.ErrFolderNameRequired),
		errors.Is(err, workspace.ErrFolderNameTooLong),
		errors.Is(err, workspace.ErrFolderDescriptionTooLong),
		errors.Is(err, workspace.ErrDuplicateFolderName),
		errors.Is(err, workspace.ErrReservedFolderName),
		errors.Is(err, workspace.ErrFolderMoveCycle),
		errors.Is(err, workspace.ErrUnsupportedDocumentType),
		errors.Is(err, workspace.ErrUnsupportedImageTarget),
		errors.Is(err, content.ErrContentJSONTooLarge),
		errors.Is(err, ErrMarkdownConversionFailed):
		return badRequest(c, err)
	case errors.Is(err, ErrMarkdownConverterUnavailable):
		return c.Status(fiber.StatusBadGateway).JSON(ErrorResponse{Error: "Markdown Converter Unavailable", Message: err.Error()})
	case errors.Is(err, workspace.ErrFolderNotFound),
		errors.Is(err, workspace.ErrParentFolderNotFound),
		errors.Is(err, workspace.ErrDocumentNotFoundOrUnauthorized),
		errors.Is(err, workspace.ErrDocumentNotFoundOrDeleted),
		errors.Is(err, workspace.ErrFolderNotFoundOrDeleted),
		errors.Is(err, workspace.ErrTargetFolderNotFoundOrDeleted),
		errors.Is(err, workspace.ErrTargetParentNotFoundOrDeleted),
		errors.Is(err, content.ErrDocumentContentNotFound),
		errors.Is(err, workspace.ErrFileNotFound):
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Not Found", Message: err.Error()})
	case errors.Is(err, workspace.ErrDocumentQuotaExceeded), errors.Is(err, workspace.ErrWorkspaceStorageQuotaExceeded):
		return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{Error: "Quota Exceeded", Message: err.Error()})
	default:
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
	}
}
