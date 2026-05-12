package workspace

import (
	"errors"
	"strings"

	"g.co1d.in/Coldin04/Cyime/server/internal/config"
	"g.co1d.in/Coldin04/Cyime/server/internal/content"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

func collaborationDisabledResponse(c *fiber.Ctx) error {
	return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
		Error:   "Not Found",
		Message: ErrDocumentNotFoundOrUnauthorized.Error(),
	})
}

// GetFilesHandler handles GET /api/v1/workspace/files
func GetFilesHandler(c *fiber.Ctx) error {
	// Get user ID from locals (set by Protected middleware)
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	// Parse query parameters
	parentIDStr := c.Query("parent_id")
	var parentID *uuid.UUID
	if parentIDStr != "" && parentIDStr != "null" {
		pid, err := uuid.Parse(parentIDStr)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Invalid parent_id",
				Message: "parent_id must be a valid UUID or 'null'",
			})
		}
		parentID = &pid
	}

	limit := c.QueryInt("limit", 50)
	offset := c.QueryInt("offset", 0)
	sortBy := c.Query("sort_by", "updated_at")
	order := c.Query("order", "desc")
	filterType := c.Query("type", "all")

	// Get files
	response, err := GetFiles(userID, parentID, limit, offset, sortBy, order, filterType)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Internal Server Error",
			Message: err.Error(),
		})
	}

	return c.JSON(response)
}

func SearchHandler(c *fiber.Ctx) error {
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	response, err := SearchWorkspace(userID, c.Query("q"), c.QueryInt("limit", 5))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Internal Server Error",
			Message: err.Error(),
		})
	}

	return c.JSON(response)
}

// GetFileHandler handles GET /api/v1/workspace/files/:id
func GetFileHandler(c *fiber.Ctx) error {
	// Get user ID from locals
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	// Parse file ID from path
	fileIDStr := c.Params("id")
	fileID, err := uuid.Parse(fileIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid File ID",
			Message: "File ID must be a valid UUID",
		})
	}

	// Get file type from query
	fileType := c.Query("type", "")
	if fileType == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "type parameter is required (must be 'folder' or 'document')",
		})
	}

	if fileType != "folder" && fileType != "document" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "type parameter must be either 'folder' or 'document'",
		})
	}

	// Get file details
	file, err := GetFile(userID, fileID, fileType)
	if err != nil {
		switch {
		case errors.Is(err, ErrFileNotFound):
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "Not Found",
				Message: err.Error(),
			})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "Internal Server Error",
				Message: err.Error(),
			})
		}
	}

	return c.JSON(file)
}

// GetPublicDocumentHandler handles GET /api/v1/public/documents/:id
func GetPublicDocumentHandler(c *fiber.Ctx) error {
	documentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid Document ID",
			Message: "Document ID must be a valid UUID",
		})
	}

	var userID *uuid.UUID
	if userIDStr, ok := c.Locals("userId").(string); ok && strings.TrimSpace(userIDStr) != "" {
		if parsed, parseErr := uuid.Parse(userIDStr); parseErr == nil {
			userID = &parsed
		}
	}

	item, err := GetPublicDocument(documentID, userID)
	if err != nil {
		switch {
		case errors.Is(err, ErrPublicDocumentNotFound):
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "Not Found",
				Message: ErrPublicDocumentNotFound.Error(),
			})
		case errors.Is(err, ErrPublicDocumentAuthRequired):
			return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
				Error:   "Unauthorized",
				Message: ErrPublicDocumentAuthRequired.Error(),
			})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "Internal Server Error",
				Message: err.Error(),
			})
		}
	}

	return c.JSON(item)
}

// GetPublicDocumentContentHandler handles GET /api/v1/public/documents/:id/content
func GetPublicDocumentContentHandler(c *fiber.Ctx) error {
	documentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid Document ID",
			Message: "Document ID must be a valid UUID",
		})
	}

	var userID *uuid.UUID
	if userIDStr, ok := c.Locals("userId").(string); ok && strings.TrimSpace(userIDStr) != "" {
		if parsed, parseErr := uuid.Parse(userIDStr); parseErr == nil {
			userID = &parsed
		}
	}

	content, err := GetPublicDocumentContent(documentID, userID)
	if err != nil {
		switch {
		case errors.Is(err, ErrPublicDocumentNotFound):
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "Not Found",
				Message: ErrPublicDocumentNotFound.Error(),
			})
		case errors.Is(err, ErrPublicDocumentAuthRequired):
			return c.Status(fiber.StatusUnauthorized).JSON(ErrorResponse{
				Error:   "Unauthorized",
				Message: ErrPublicDocumentAuthRequired.Error(),
			})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "Internal Server Error",
				Message: err.Error(),
			})
		}
	}

	return c.JSON(content)
}

// CreateFolderHandler handles POST /api/v1/workspace/folders
func CreateFolderHandler(c *fiber.Ctx) error {
	// Get user ID from locals
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	// Parse request body
	var req CreateFolderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid request body",
		})
	}

	// Create folder
	folder, err := CreateFolder(userID, req.Name, req.Description, req.ParentID)
	if err != nil {
		if errors.Is(err, ErrFolderNameRequired) ||
			errors.Is(err, ErrFolderNameTooLong) ||
			errors.Is(err, ErrReservedFolderName) ||
			errors.Is(err, ErrFolderDescriptionTooLong) ||
			errors.Is(err, ErrDuplicateFolderName) {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Validation Error",
				Message: err.Error(),
			})
		}
		if errors.Is(err, ErrWorkspaceStorageQuotaExceeded) {
			return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
				Error:   "Quota Exceeded",
				Message: err.Error(),
			})
		}
		if errors.Is(err, ErrParentFolderNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "Not Found",
				Message: err.Error(),
			})
		}
		// Handle unknown errors with user-friendly message
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Server Error",
			Message: "创建文件夹失败，请稍后重试",
		})
	}

	// Get creator info
	creatorInfo, err := GetCreatorInfo(userID)
	if err != nil {
		creatorInfo = &CreatorInfo{
			ID:          userID,
			DisplayName: nil,
		}
	}

	// Build response
	response := CreateFolderResponse{
		ID:          folder.ID,
		Type:        "folder",
		Name:        folder.Name,
		Description: folder.Description,
		ParentID:    folder.ParentID,
		CreatedAt:   folder.CreatedAt,
		UpdatedAt:   folder.UpdatedAt,
		Creator:     *creatorInfo,
	}

	return c.Status(fiber.StatusCreated).JSON(response)
}

// CreateDocumentHandler handles POST /api/v1/workspace/documents
func CreateDocumentHandler(c *fiber.Ctx) error {
	// Get user ID from locals
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	// Parse request body
	var req CreateDocumentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid request body",
		})
	}

	// Create document
	document, err := CreateDocument(
		userID,
		req.Title,
		string(req.ContentJSON),
		req.FolderID,
		req.DocumentType,
		req.PreferredImageTargetID,
	)
	if err != nil {
		if errors.Is(err, ErrDocumentTitleRequired) ||
			errors.Is(err, ErrDocumentTitleTooLong) ||
			errors.Is(err, ErrDuplicateDocumentTitle) ||
			errors.Is(err, ErrUnsupportedDocumentType) ||
			errors.Is(err, ErrUnsupportedImageTarget) ||
			errors.Is(err, content.ErrContentJSONTooLarge) {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Validation Error",
				Message: err.Error(),
			})
		}
		if errors.Is(err, ErrDocumentQuotaExceeded) || errors.Is(err, ErrWorkspaceStorageQuotaExceeded) {
			return c.Status(fiber.StatusForbidden).JSON(ErrorResponse{
				Error:   "Quota Exceeded",
				Message: err.Error(),
			})
		}
		if errors.Is(err, ErrFolderNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "Not Found",
				Message: err.Error(),
			})
		}
		// Handle unknown errors with user-friendly message
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Server Error",
			Message: "创建文档失败，请稍后重试",
		})
	}

	// Get creator info
	creatorInfo, err := GetCreatorInfo(userID)
	if err != nil {
		creatorInfo = &CreatorInfo{
			ID:          userID,
			DisplayName: nil,
		}
	}

	// Build response
	response := CreateDocumentResponse{
		ID:                     document.ID,
		Type:                   "document",
		DocumentType:           document.DocumentType,
		PreferredImageTargetID: resolveDocumentPreferredImageTargetID(document.PreferredImageTargetID),
		Title:                  document.Title,
		Excerpt:                resolveDocumentListExcerpt(document.Excerpt, document.ManualExcerpt),
		FolderID:               document.FolderID,
		CreatedAt:              document.CreatedAt,
		UpdatedAt:              document.UpdatedAt,
		Creator:                *creatorInfo,
	}

	return c.Status(fiber.StatusCreated).JSON(response)
}

func ListSharedDocumentsHandler(c *fiber.Ctx) error {
	if !config.GetCollaborationEnabled() {
		return collaborationDisabledResponse(c)
	}

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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	result, err := ListSharedDocuments(userID, c.QueryInt("limit", 20), c.QueryInt("offset", 0))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Internal Server Error",
			Message: err.Error(),
		})
	}
	return c.JSON(result)
}

func SharedDocumentSummaryHandler(c *fiber.Ctx) error {
	if !config.GetCollaborationEnabled() {
		return collaborationDisabledResponse(c)
	}

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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	result, err := GetSharedDocumentSummary(userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Internal Server Error",
			Message: err.Error(),
		})
	}
	return c.JSON(result)
}

func ListOutgoingSharedDocumentsHandler(c *fiber.Ctx) error {
	if !config.GetCollaborationEnabled() {
		return collaborationDisabledResponse(c)
	}

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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	result, err := ListOutgoingSharedDocuments(userID, c.QueryInt("limit", 20), c.QueryInt("offset", 0))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Internal Server Error",
			Message: err.Error(),
		})
	}
	return c.JSON(result)
}

func ShareDocumentHandler(c *fiber.Ctx) error {
	if !config.GetCollaborationEnabled() {
		return collaborationDisabledResponse(c)
	}

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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}
	documentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid document id",
		})
	}

	var req ShareDocumentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid request body",
		})
	}

	result, err := ShareDocument(userID, documentID, req.UserID, req.Role)
	if err != nil {
		if errors.Is(err, ErrDocumentNotFoundOrUnauthorized) || errors.Is(err, ErrTargetUserNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Not Found", Message: err.Error()})
		}
		if errors.Is(err, ErrInvalidShareRole) ||
			errors.Is(err, ErrCannotShareSelf) ||
			errors.Is(err, ErrCollaboratorGrantRestricted) ||
			errors.Is(err, ErrTargetUserEmailUnverified) ||
			errors.Is(err, ErrSharingDisabled) ||
			errors.Is(err, ErrSharingEmailUnverified) {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Bad Request", Message: err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
	}
	return c.JSON(result)
}

func InviteDocumentByEmailHandler(c *fiber.Ctx) error {
	if !config.GetCollaborationEnabled() {
		return collaborationDisabledResponse(c)
	}

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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	documentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid document id",
		})
	}

	var req InviteDocumentByEmailRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid request body",
		})
	}

	result, err := InviteDocumentByEmail(userID, documentID, req.Email, req.Role)
	if err != nil {
		if errors.Is(err, ErrDocumentNotFoundOrUnauthorized) ||
			errors.Is(err, ErrInviteNotFound) ||
			errors.Is(err, ErrTargetUserNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Not Found", Message: err.Error()})
		}
		if errors.Is(err, ErrInvalidShareRole) ||
			errors.Is(err, ErrInviteEmailRequired) ||
			errors.Is(err, ErrCannotShareSelf) ||
			errors.Is(err, ErrCollaboratorGrantRestricted) ||
			errors.Is(err, ErrTargetUserEmailUnverified) ||
			errors.Is(err, ErrSharingDisabled) ||
			errors.Is(err, ErrSharingEmailUnverified) {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Bad Request", Message: err.Error()})
		}
		var rateLimitErr *InviteRateLimitError
		if errors.As(err, &rateLimitErr) {
			return c.Status(fiber.StatusTooManyRequests).JSON(ErrorResponse{Error: "Too Many Requests", Message: err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
	}
	return c.JSON(result)
}

func ListDocumentMembersHandler(c *fiber.Ctx) error {
	if !config.GetCollaborationEnabled() {
		return collaborationDisabledResponse(c)
	}

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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}
	documentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid document id",
		})
	}

	result, err := ListDocumentMembers(userID, documentID)
	if err != nil {
		if errors.Is(err, ErrDocumentNotFoundOrUnauthorized) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Not Found", Message: err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
	}
	return c.JSON(result)
}

func RemoveDocumentMemberHandler(c *fiber.Ctx) error {
	if !config.GetCollaborationEnabled() {
		return collaborationDisabledResponse(c)
	}

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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}
	documentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid document id",
		})
	}
	targetUserID, err := uuid.Parse(c.Params("userId"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid target user id",
		})
	}

	result, err := RemoveDocumentMember(userID, documentID, targetUserID)
	if err != nil {
		if errors.Is(err, ErrDocumentNotFoundOrUnauthorized) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Not Found", Message: err.Error()})
		}
		if errors.Is(err, ErrCannotRemoveSelf) ||
			errors.Is(err, ErrCannotRemoveOwner) ||
			errors.Is(err, ErrCollaboratorRemoveRestricted) ||
			errors.Is(err, ErrMemberNotFound) ||
			errors.Is(err, ErrSharingDisabled) ||
			errors.Is(err, ErrSharingEmailUnverified) {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Bad Request", Message: err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
	}
	return c.JSON(result)
}

func ListNotificationsHandler(c *fiber.Ctx) error {
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	unreadOnly := c.Query("unread", "0") == "1"
	result, err := ListNotifications(userID, c.Query("type", ""), unreadOnly, c.QueryInt("limit", 20), c.QueryInt("offset", 0))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Internal Server Error",
			Message: err.Error(),
		})
	}
	return c.JSON(result)
}

func MarkNotificationReadHandler(c *fiber.Ctx) error {
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}
	notificationID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid notification id",
		})
	}

	if err := MarkNotificationRead(userID, notificationID); err != nil {
		if errors.Is(err, ErrNotificationNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Not Found", Message: err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func ClearNotificationsHandler(c *fiber.Ctx) error {
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	clearedCount, err := ClearNotifications(userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Internal Server Error",
			Message: err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success":      true,
		"clearedCount": clearedCount,
	})
}

func AcceptDocumentInviteHandler(c *fiber.Ctx) error {
	if !config.GetCollaborationEnabled() {
		return collaborationDisabledResponse(c)
	}

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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}
	inviteID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid invite id",
		})
	}

	if err := AcceptDocumentInvite(userID, inviteID); err != nil {
		if errors.Is(err, ErrInviteNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Not Found", Message: err.Error()})
		}
		if errors.Is(err, ErrInviteInvalidStatus) ||
			errors.Is(err, ErrInviteInvalidRole) ||
			errors.Is(err, ErrSharingDisabled) ||
			errors.Is(err, ErrSharingEmailUnverified) {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Bad Request", Message: err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func DeclineDocumentInviteHandler(c *fiber.Ctx) error {
	if !config.GetCollaborationEnabled() {
		return collaborationDisabledResponse(c)
	}

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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}
	inviteID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid invite id",
		})
	}

	if err := DeclineDocumentInvite(userID, inviteID); err != nil {
		if errors.Is(err, ErrInviteNotFound) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Not Found", Message: err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func LeaveSharedDocumentHandler(c *fiber.Ctx) error {
	if !config.GetCollaborationEnabled() {
		return collaborationDisabledResponse(c)
	}

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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}
	documentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid document id",
		})
	}

	if err := LeaveSharedDocument(userID, documentID); err != nil {
		if errors.Is(err, ErrDocumentNotFoundOrUnauthorized) {
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{Error: "Not Found", Message: err.Error()})
		}
		if errors.Is(err, ErrOwnerCannotLeaveShared) {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{Error: "Bad Request", Message: err.Error()})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{Error: "Internal Server Error", Message: err.Error()})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// BatchDeleteHandler handles POST /api/v1/workspace/files/batch-delete
func BatchDeleteHandler(c *fiber.Ctx) error {
	// Get user ID from locals
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	// Parse request body
	var req BatchDeleteRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid request body",
		})
	}

	if len(req.Items) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "至少需要删除一个项目",
		})
	}

	// Batch delete files
	response, err := BatchDeleteFiles(userID, req.Items)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Internal Server Error",
			Message: err.Error(),
		})
	}

	if !response.Success {
		return c.Status(fiber.StatusMultiStatus).JSON(response)
	}

	return c.JSON(response)
}

// DeleteFileHandler handles DELETE /api/v1/workspace/files/:id
func DeleteFileHandler(c *fiber.Ctx) error {
	// Get user ID from locals
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	// Parse file ID
	fileIDStr := c.Params("id")
	fileID, err := uuid.Parse(fileIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid File ID",
			Message: "File ID must be a valid UUID",
		})
	}

	// Get file type from query parameter
	fileType := c.Query("type")
	if fileType != "folder" && fileType != "document" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "type parameter must be either 'folder' or 'document'",
		})
	}

	// Delete file
	if err := DeleteFile(userID, fileID, fileType); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "Not Found",
			Message: err.Error(),
		})
	}

	return c.JSON(DeleteResponse{
		Success: true,
		Message: "文件已移动到回收站",
	})
}

// GetFolderAncestorsHandler handles GET /api/v1/workspace/folders/:id/ancestors
func GetFolderAncestorsHandler(c *fiber.Ctx) error {
	// Get user ID from locals
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	// Parse folder ID from path
	folderIDStr := c.Params("id")
	folderID, err := uuid.Parse(folderIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid Folder ID",
			Message: "Folder ID must be a valid UUID",
		})
	}

	// Get ancestors from service
	ancestors, err := GetFolderAncestors(userID, folderID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Internal Server Error",
			Message: err.Error(),
		})
	}

	return c.JSON(ancestors)
}

// GetTrashHandler handles GET /api/v1/workspace/trash
func GetTrashHandler(c *fiber.Ctx) error {
	// Get user ID from locals (set by Protected middleware)
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	// Parse query parameters
	limit := c.QueryInt("limit", 50)
	offset := c.QueryInt("offset", 0)
	sortBy := c.Query("sort_by", "deleted_at")
	order := c.Query("order", "desc")

	// Get trashed files
	response, err := GetTrashedFiles(userID, limit, offset, sortBy, order)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Internal Server Error",
			Message: err.Error(),
		})
	}

	return c.JSON(response)
}

// RestoreTrashRequest defines the shape of the request for restoring items
type RestoreTrashRequest struct {
	Items []ItemToRestore `json:"items"`
}

// RestoreTrashHandler handles POST /api/v1/workspace/trash/restore
func RestoreTrashHandler(c *fiber.Ctx) error {
	// Get user ID from locals
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	// Parse request body
	var req RestoreTrashRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid request body",
		})
	}

	// Restore items via service
	response, err := RestoreTrashedItems(userID, req.Items)
	if err != nil {
		// Specific error handling can be added here if needed (e.g., for conflicts)
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Internal Server Error",
			Message: err.Error(),
		})
	}

	return c.JSON(response)
}

// PermanentDeleteRequest defines the shape of the request for permanently deleting items
type PermanentDeleteRequest struct {
	Items []ItemToRestore `json:"items"`
}

// PermanentDeleteHandler handles DELETE /api/v1/workspace/trash
func PermanentDeleteHandler(c *fiber.Ctx) error {
	// Get user ID from locals
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	// Parse request body
	var req PermanentDeleteRequest
	// It's a DELETE request, but we might have a body. If not, req will be empty.
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Bad Request",
				Message: "Invalid request body",
			})
		}
	}

	// Permanently delete items via service
	response, err := PermanentDeleteItems(userID, req.Items)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
			Error:   "Internal Server Error",
			Message: err.Error(),
		})
	}

	return c.JSON(response)
}

// UpdateDocumentTitleHandler handles PUT /api/v1/workspace/documents/:id/title
func UpdateDocumentTitleHandler(c *fiber.Ctx) error {
	// Get user ID from locals
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	// Parse document ID from path
	documentIDStr := c.Params("id")
	documentID, err := uuid.Parse(documentIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid Document ID",
			Message: "Document ID must be a valid UUID",
		})
	}

	// Parse request body
	var req struct {
		Title string `json:"title"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid request body",
		})
	}

	if req.Title == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Title cannot be empty",
		})
	}

	// Update title
	err = UpdateDocumentTitle(userID, documentID, req.Title)
	if err != nil {
		switch {
		case errors.Is(err, ErrDocumentNotFoundOrUnauthorized):
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "Not Found",
				Message: err.Error(),
			})
		case errors.Is(err, ErrDocumentTitleRequired), errors.Is(err, ErrDocumentTitleTooLong), errors.Is(err, ErrDuplicateDocumentTitle):
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

	return c.JSON(fiber.Map{
		"success": true,
	})
}

// UpdateDocumentExcerptHandler handles PUT /api/v1/workspace/documents/:id/excerpt
func UpdateDocumentExcerptHandler(c *fiber.Ctx) error {
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	documentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid Document ID",
			Message: "Document ID must be a valid UUID",
		})
	}

	var req UpdateDocumentExcerptRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid request body",
		})
	}

	manualExcerpt, excerpt, err := UpdateDocumentManualExcerpt(userID, documentID, req.Excerpt)
	if err != nil {
		switch {
		case errors.Is(err, ErrDocumentNotFoundOrUnauthorized):
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "Not Found",
				Message: err.Error(),
			})
		case errors.Is(err, ErrDocumentExcerptTooLong):
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

	return c.JSON(fiber.Map{
		"success":       true,
		"manualExcerpt": manualExcerpt,
		"excerpt":       excerpt,
	})
}

// UpdateDocumentImageTargetHandler handles PUT /api/v1/workspace/documents/:id/image-target
func UpdateDocumentImageTargetHandler(c *fiber.Ctx) error {
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	documentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid Document ID",
			Message: "Document ID must be a valid UUID",
		})
	}

	var req UpdateDocumentImageTargetRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid request body",
		})
	}

	err = UpdateDocumentImageTarget(userID, documentID, req.PreferredImageTargetID)
	if err != nil {
		switch {
		case errors.Is(err, ErrUnsupportedImageTarget):
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Validation Error",
				Message: err.Error(),
			})
		case errors.Is(err, ErrDocumentNotFoundOrUnauthorized):
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "Not Found",
				Message: err.Error(),
			})
		case errors.Is(err, ErrImageTargetNotFound):
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Validation Error",
				Message: err.Error(),
			})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "Server Error",
				Message: "更新图片上传目标失败，请稍后重试",
			})
		}
	}

	return c.JSON(fiber.Map{
		"success":                true,
		"preferredImageTargetId": normalizePreferredImageTargetID(req.PreferredImageTargetID),
	})
}

// UpdateDocumentPublicAccessHandler handles PUT /api/v1/workspace/documents/:id/public-access
func UpdateDocumentPublicAccessHandler(c *fiber.Ctx) error {
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	documentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid Document ID",
			Message: "Document ID must be a valid UUID",
		})
	}

	var req UpdateDocumentPublicAccessRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid request body",
		})
	}

	if err := UpdateDocumentPublicAccess(userID, documentID, req.PublicAccess); err != nil {
		switch {
		case errors.Is(err, ErrDocumentNotFoundOrUnauthorized):
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "Not Found",
				Message: err.Error(),
			})
		case errors.Is(err, ErrPublicAccessInvalid):
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

	return c.JSON(fiber.Map{
		"success":      true,
		"publicAccess": req.PublicAccess,
		"publicUrl":    buildDocumentPublicURL(documentID),
	})
}

// UpdateFolderNameHandler handles PUT /api/v1/workspace/folders/:id/name
func UpdateFolderNameHandler(c *fiber.Ctx) error {
	// Get user ID from locals
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	// Parse folder ID from path
	folderIDStr := c.Params("id")
	folderID, err := uuid.Parse(folderIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid Folder ID",
			Message: "Folder ID must be a valid UUID",
		})
	}

	// Parse request body
	var req struct {
		Name string `json:"name"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid request body",
		})
	}

	if req.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Folder name cannot be empty",
		})
	}

	// Update name
	err = UpdateFolderName(userID, folderID, req.Name)
	if err != nil {
		switch {
		case errors.Is(err, ErrFolderNotFound):
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "Not Found",
				Message: err.Error(),
			})
		case errors.Is(err, ErrFolderNameRequired), errors.Is(err, ErrFolderNameTooLong), errors.Is(err, ErrDuplicateFolderName), errors.Is(err, ErrReservedFolderName):
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

	return c.JSON(fiber.Map{
		"success": true,
	})
}

// MoveDocumentHandler handles PUT /api/v1/workspace/documents/:id/move
func MoveDocumentHandler(c *fiber.Ctx) error {
	// Get user ID from locals
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	// Parse document ID from path
	documentIDStr := c.Params("id")
	documentID, err := uuid.Parse(documentIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid Document ID",
			Message: "Document ID must be a valid UUID",
		})
	}

	// Parse request body
	var req MoveDocumentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid request body",
		})
	}

	// Move the document
	updatedAt, err := MoveDocument(userID, documentID, req.FolderID)
	if err != nil {
		switch {
		case errors.Is(err, ErrDocumentNotFoundOrDeleted), errors.Is(err, ErrTargetFolderNotFoundOrDeleted):
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "Not Found",
				Message: err.Error(),
			})
		default:
			return c.Status(fiber.StatusInternalServerError).JSON(ErrorResponse{
				Error:   "Internal Server Error",
				Message: err.Error(),
			})
		}
	}

	return c.JSON(MoveResponse{
		Success:   true,
		Message:   "文档移动成功",
		UpdatedAt: *updatedAt,
	})
}

// MoveFolderHandler handles PUT /api/v1/workspace/folders/:id/move
func MoveFolderHandler(c *fiber.Ctx) error {
	// Get user ID from locals
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	// Parse folder ID from path
	folderIDStr := c.Params("id")
	folderID, err := uuid.Parse(folderIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid Folder ID",
			Message: "Folder ID must be a valid UUID",
		})
	}

	// Parse request body
	var req MoveFolderRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid request body",
		})
	}

	// Move the folder
	updatedAt, err := MoveFolder(userID, folderID, req.ParentID)
	if err != nil {
		switch {
		case errors.Is(err, ErrFolderNotFoundOrDeleted), errors.Is(err, ErrTargetParentNotFoundOrDeleted):
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "Not Found",
				Message: err.Error(),
			})
		case errors.Is(err, ErrFolderMoveCycle):
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

	return c.JSON(MoveResponse{
		Success:   true,
		Message:   "文件夹移动成功",
		UpdatedAt: *updatedAt,
	})
}

// BatchMoveHandler handles POST /api/v1/workspace/files/batch-move
func BatchMoveHandler(c *fiber.Ctx) error {
	// Get user ID from locals
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
			Error:   "Invalid User ID",
			Message: "User ID format is invalid",
		})
	}

	// Parse request body
	var req BatchMoveRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid request body: " + err.Error(),
		})
	}

	if len(req.Items) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "至少需要移动一个项目",
		})
	}

	// Call the service function
	response, err := BatchMoveFiles(userID, req.Items, req.DestinationFolderID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{ // Most likely a validation error
			Error:   "Bad Request",
			Message: err.Error(),
		})
	}

	// Use 207 Multi-Status if some items failed
	if !response.Success {
		return c.Status(fiber.StatusMultiStatus).JSON(response)
	}

	return c.Status(fiber.StatusOK).JSON(response)
}
