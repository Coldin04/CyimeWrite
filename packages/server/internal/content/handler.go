package content

import (
	"encoding/json"
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

// ErrorResponse represents a standard error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// UpdateContentRequest represents the request body for updating content.
type UpdateContentRequest struct {
	ContentJSON json.RawMessage `json:"contentJson"`
}

// GetContentHandler handles GET /api/v1/edit/documents/:id/content.
func GetContentHandler(c *fiber.Ctx) error {
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

	documentIDStr := c.Params("id")
	documentID, err := uuid.Parse(documentIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid Document ID",
			Message: "Document ID must be a valid UUID",
		})
	}

	result, err := GetContent(userID, documentID)
	if err != nil {
		switch {
		case errors.Is(err, ErrDocumentNotFoundOrUnauthorized), errors.Is(err, ErrDocumentContentNotFound):
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

	return c.JSON(result)
}

// UpdateContentHandler handles PUT /api/v1/edit/documents/:id/content.
func UpdateContentHandler(c *fiber.Ctx) error {
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

	documentIDStr := c.Params("id")
	documentID, err := uuid.Parse(documentIDStr)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid Document ID",
			Message: "Document ID must be a valid UUID",
		})
	}

	var req UpdateContentRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: "Invalid request body",
		})
	}

	result, err := UpdateContent(userID, documentID, req.ContentJSON)
	if err != nil {
		switch {
		case errors.Is(err, ErrDocumentNotFoundOrUnauthorized):
			return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
				Error:   "Not Found",
				Message: err.Error(),
			})
		case errors.Is(err, ErrInvalidContentJSON), errors.Is(err, ErrContentJSONTooLarge), errors.Is(err, ErrInvalidContentAssetReferences):
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Bad Request",
				Message: err.Error(),
			})
		case errors.Is(err, ErrWorkspaceStorageQuotaExceeded):
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

	return c.JSON(result)
}
