package workspace

import (
	"strings"
	"sync"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/acl"
	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const (
	presenceTTL                        = 40 * time.Second
	presenceSessionIDHeader            = "X-Presence-Session-Id"
	presenceMaxRequestBodyBytes        = 256
	presenceMaxSessionsPerUserDocument = 4
)

type documentPresenceHeartbeatRequest struct {
	SessionID string `json:"sessionId"`
}

type documentPresenceResponse struct {
	DocumentID      uuid.UUID `json:"documentId"`
	ConnectedCount  int       `json:"connectedCount"`
	UniqueUserCount int       `json:"uniqueUserCount"`
}

type presenceEntry struct {
	userID   uuid.UUID
	lastSeen time.Time
}

var (
	presenceMu        sync.Mutex
	presenceStore     = map[uuid.UUID]map[string]presenceEntry{}
	presenceAuthMu    sync.Mutex
	presenceAuthTTL   = 40 * time.Second
	presenceAuthCache = map[uuid.UUID]map[uuid.UUID]time.Time{}
)

func parsePresenceSessionID(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fiber.NewError(fiber.StatusBadRequest, "Presence session ID is required")
	}
	if len(trimmed) > 36 {
		return "", fiber.NewError(fiber.StatusBadRequest, "Presence session ID must be a UUID")
	}

	sessionID, err := uuid.Parse(trimmed)
	if err != nil {
		return "", fiber.NewError(fiber.StatusBadRequest, "Presence session ID must be a UUID")
	}
	return sessionID.String(), nil
}

func presenceSessionKey(userID uuid.UUID, sessionID string) string {
	return userID.String() + ":" + sessionID
}

func countUserPresenceSessionsLocked(sessions map[string]presenceEntry, userID uuid.UUID) int {
	count := 0
	for _, entry := range sessions {
		if entry.userID == userID {
			count++
		}
	}
	return count
}

func cleanupDocumentPresenceLocked(documentID uuid.UUID, now time.Time) {
	sessions := presenceStore[documentID]
	for sessionID, entry := range sessions {
		if now.Sub(entry.lastSeen) > presenceTTL {
			delete(sessions, sessionID)
		}
	}
	if len(sessions) == 0 {
		delete(presenceStore, documentID)
	}
}

func countPresenceLocked(documentID uuid.UUID) (int, int) {
	sessions := presenceStore[documentID]
	if len(sessions) == 0 {
		return 0, 0
	}

	uniqueUsers := map[uuid.UUID]struct{}{}
	for _, entry := range sessions {
		uniqueUsers[entry.userID] = struct{}{}
	}
	return len(sessions), len(uniqueUsers)
}

func updatePresence(documentID uuid.UUID, userID uuid.UUID, sessionID string) (int, int, error) {
	validatedSessionID, err := parsePresenceSessionID(sessionID)
	if err != nil {
		return 0, 0, err
	}
	now := time.Now()

	presenceMu.Lock()
	defer presenceMu.Unlock()

	cleanupDocumentPresenceLocked(documentID, now)

	sessions, exists := presenceStore[documentID]
	if !exists {
		sessions = map[string]presenceEntry{}
		presenceStore[documentID] = sessions
	}
	sessionKey := presenceSessionKey(userID, validatedSessionID)
	if _, exists := sessions[sessionKey]; !exists && countUserPresenceSessionsLocked(sessions, userID) >= presenceMaxSessionsPerUserDocument {
		connectedCount, uniqueUserCount := countPresenceLocked(documentID)
		return connectedCount, uniqueUserCount, fiber.NewError(fiber.StatusTooManyRequests, "Too many active presence sessions")
	}

	sessions[sessionKey] = presenceEntry{
		userID:   userID,
		lastSeen: now,
	}

	connectedCount, uniqueUserCount := countPresenceLocked(documentID)
	return connectedCount, uniqueUserCount, nil
}

func readPresence(documentID uuid.UUID) (int, int) {
	now := time.Now()

	presenceMu.Lock()
	defer presenceMu.Unlock()

	cleanupDocumentPresenceLocked(documentID, now)
	return countPresenceLocked(documentID)
}

func cleanupPresenceAuthLocked(now time.Time) {
	for documentID, users := range presenceAuthCache {
		for userID, expiry := range users {
			if now.After(expiry) {
				delete(users, userID)
			}
		}
		if len(users) == 0 {
			delete(presenceAuthCache, documentID)
		}
	}
}

func canReadDocumentForPresence(userID, documentID uuid.UUID) error {
	now := time.Now()

	presenceAuthMu.Lock()
	documentCache := presenceAuthCache[documentID]
	if expiry, ok := documentCache[userID]; ok && now.Before(expiry) {
		presenceAuthMu.Unlock()
		return nil
	}

	if documentCache == nil {
		documentCache = map[uuid.UUID]time.Time{}
		presenceAuthCache[documentID] = documentCache
	}
	cleanupPresenceAuthLocked(now)
	presenceAuthMu.Unlock()

	_, err := acl.CanReadDocument(database.DB, userID, documentID)
	if err != nil {
		return err
	}

	presenceAuthMu.Lock()
	if _, exists := presenceAuthCache[documentID]; !exists {
		presenceAuthCache[documentID] = map[uuid.UUID]time.Time{}
	}
	presenceAuthCache[documentID][userID] = now.Add(presenceAuthTTL)
	presenceAuthMu.Unlock()

	return nil
}

func parseUserAndDocumentID(c *fiber.Ctx) (uuid.UUID, uuid.UUID, error) {
	userIDStr, ok := c.Locals("userId").(string)
	if !ok {
		return uuid.Nil, uuid.Nil, fiber.NewError(fiber.StatusUnauthorized, "Invalid user context")
	}

	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		return uuid.Nil, uuid.Nil, fiber.NewError(fiber.StatusBadRequest, "User ID format is invalid")
	}

	documentID, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return uuid.Nil, uuid.Nil, fiber.NewError(fiber.StatusBadRequest, "Document ID must be a valid UUID")
	}

	return userID, documentID, nil
}

// HeartbeatDocumentPresenceHandler handles PUT /api/v1/workspace/documents/:id/presence
func HeartbeatDocumentPresenceHandler(c *fiber.Ctx) error {
	userID, documentID, err := parseUserAndDocumentID(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: err.Error(),
		})
	}

	if err := canReadDocumentForPresence(userID, documentID); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "Not Found",
			Message: ErrDocumentNotFoundOrUnauthorized.Error(),
		})
	}

	if len(c.Body()) > presenceMaxRequestBodyBytes {
		return c.Status(fiber.StatusRequestEntityTooLarge).JSON(ErrorResponse{
			Error:   "Request Entity Too Large",
			Message: "Presence heartbeat request body is too large",
		})
	}

	var req documentPresenceHeartbeatRequest
	if len(c.Body()) > 0 {
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(ErrorResponse{
				Error:   "Bad Request",
				Message: "Invalid presence heartbeat request body",
			})
		}
	}

	sessionIDRaw := req.SessionID
	if strings.TrimSpace(sessionIDRaw) == "" {
		sessionIDRaw = c.Get(presenceSessionIDHeader)
	}
	sessionID, err := parsePresenceSessionID(sessionIDRaw)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: err.Error(),
		})
	}

	connectedCount, uniqueUserCount, err := updatePresence(documentID, userID, sessionID)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(ErrorResponse{
			Error:   "Too Many Requests",
			Message: err.Error(),
		})
	}
	return c.JSON(documentPresenceResponse{
		DocumentID:      documentID,
		ConnectedCount:  connectedCount,
		UniqueUserCount: uniqueUserCount,
	})
}

// GetDocumentPresenceHandler handles GET /api/v1/workspace/documents/:id/presence
func GetDocumentPresenceHandler(c *fiber.Ctx) error {
	userID, documentID, err := parseUserAndDocumentID(c)
	if err != nil {
		return c.Status(err.(*fiber.Error).Code).JSON(ErrorResponse{
			Error:   "Bad Request",
			Message: err.Error(),
		})
	}

	if err := canReadDocumentForPresence(userID, documentID); err != nil {
		return c.Status(fiber.StatusNotFound).JSON(ErrorResponse{
			Error:   "Not Found",
			Message: ErrDocumentNotFoundOrUnauthorized.Error(),
		})
	}

	connectedCount, uniqueUserCount := readPresence(documentID)
	return c.JSON(documentPresenceResponse{
		DocumentID:      documentID,
		ConnectedCount:  connectedCount,
		UniqueUserCount: uniqueUserCount,
	})
}
