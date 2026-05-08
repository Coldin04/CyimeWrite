package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"g.co1d.in/Coldin04/Cyime/server/internal/acl"
	"g.co1d.in/Coldin04/Cyime/server/internal/database"
	"g.co1d.in/Coldin04/Cyime/server/internal/models"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	documentInviteStatusSent       = "sent"
	documentInviteStatusAccepted   = "accepted"
	documentInviteStatusDeclined   = "declined"
	documentInviteStatusCanceled   = "canceled"
	notificationTypeDocumentInvite = "document_invite"
)

var ErrInviteRateLimited = errors.New("invite rate limited")

type InviteRateLimitError struct {
	RemainingSeconds int
}

func (e *InviteRateLimitError) Error() string {
	return fmt.Sprintf("邀请过于频繁，请在 %d 秒后重试", e.RemainingSeconds)
}

func (e *InviteRateLimitError) Unwrap() error {
	return ErrInviteRateLimited
}

type notificationInviteData struct {
	InviteID           uuid.UUID `json:"inviteId"`
	DocumentID         uuid.UUID `json:"documentId"`
	DocumentTitle      string    `json:"documentTitle"`
	InviterUserID      uuid.UUID `json:"inviterUserId"`
	InviterDisplayName *string   `json:"inviterDisplayName,omitempty"`
	Role               string    `json:"role"`
}

func InviteDocumentByEmail(actorUserID, documentID uuid.UUID, email, role string) (*ShareDocumentResponse, error) {
	normalizedRole := normalizePermissionRole(role)
	if normalizedRole == "" {
		return nil, ErrInvalidShareRole
	}

	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	if normalizedEmail == "" {
		return nil, ErrInviteEmailRequired
	}

	err := database.DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureSharingEnabledForUser(tx, actorUserID); err != nil {
			return err
		}

		document, actorRole, err := loadShareManagedDocument(tx, actorUserID, documentID)
		if err != nil {
			return err
		}
		if actorRole == acl.RoleCollaborator && normalizedRole == acl.RoleCollaborator {
			return ErrCollaboratorGrantRestricted
		}

		var invitee models.User
		if err := tx.Where("email = ?", normalizedEmail).First(&invitee).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTargetUserNotFound
			}
			return err
		}
		if invitee.ID == actorUserID {
			return ErrCannotShareSelf
		}
		if err := ensureSharingEnabledForUser(tx, invitee.ID); err != nil {
			return ErrTargetUserEmailUnverified
		}

		now := time.Now()
		invite, err := upsertDocumentInvite(tx, documentID, actorUserID, invitee.ID, normalizedRole, now)
		if err != nil {
			return err
		}

		var inviter models.User
		if err := tx.Select("id", "display_name").Where("id = ?", actorUserID).First(&inviter).Error; err != nil {
			return err
		}

		payload, err := json.Marshal(notificationInviteData{
			InviteID:           invite.ID,
			DocumentID:         document.ID,
			DocumentTitle:      document.Title,
			InviterUserID:      actorUserID,
			InviterDisplayName: inviter.DisplayName,
			Role:               normalizedRole,
		})
		if err != nil {
			return err
		}

		return tx.Create(&models.Notification{
			ID:       uuid.New(),
			UserID:   invitee.ID,
			Type:     notificationTypeDocumentInvite,
			GroupKey: "doc:" + document.ID.String(),
			DataJSON: string(payload),
		}).Error
	})
	if err != nil {
		return nil, err
	}

	return ListDocumentMembers(actorUserID, documentID)
}

func ListNotifications(userID uuid.UUID, notificationType string, unreadOnly bool, limit, offset int) (*NotificationListResponse, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	query := database.DB.Model(&models.Notification{}).Where("user_id = ?", userID)
	if t := strings.TrimSpace(notificationType); t != "" {
		query = query.Where("type = ?", t)
	}
	if unreadOnly {
		query = query.Where("read_at IS NULL")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	var unreadCount int64
	if err := database.DB.Model(&models.Notification{}).
		Where("user_id = ? AND read_at IS NULL", userID).
		Count(&unreadCount).Error; err != nil {
		return nil, err
	}

	var items []models.Notification
	if err := query.Order("created_at desc").Limit(limit).Offset(offset).Find(&items).Error; err != nil {
		return nil, err
	}

	responseItems := make([]NotificationItem, 0, len(items))
	for _, item := range items {
		responseItems = append(responseItems, NotificationItem{
			ID:        item.ID,
			UserID:    item.UserID,
			Type:      item.Type,
			GroupKey:  item.GroupKey,
			Data:      json.RawMessage(item.DataJSON),
			ReadAt:    item.ReadAt,
			CreatedAt: item.CreatedAt,
			UpdatedAt: item.UpdatedAt,
		})
	}

	return &NotificationListResponse{
		Items:       responseItems,
		HasMore:     int64(offset+len(responseItems)) < total,
		Total:       total,
		UnreadCount: unreadCount,
	}, nil
}

func MarkNotificationRead(userID, notificationID uuid.UUID) error {
	now := time.Now()
	result := database.DB.Model(&models.Notification{}).
		Where("id = ? AND user_id = ? AND read_at IS NULL", notificationID, userID).
		Updates(map[string]any{
			"read_at":    now,
			"updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotificationNotFound
	}
	return nil
}

func ClearNotifications(userID uuid.UUID) (int64, error) {
	result := database.DB.Where("user_id = ?", userID).Delete(&models.Notification{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

func AcceptDocumentInvite(userID, inviteID uuid.UUID) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := ensureSharingEnabledForUser(tx, userID); err != nil {
			return err
		}

		var invite models.DocumentInvite
		if err := tx.Where("id = ? AND invitee_user_id = ?", inviteID, userID).First(&invite).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInviteNotFound
			}
			return err
		}

		if invite.Status != documentInviteStatusSent {
			return ErrInviteInvalidStatus
		}
		if !acl.RoleAllowsAction(invite.Role, acl.ActionRead) {
			return ErrInviteInvalidRole
		}
		if err := ensureDocumentInviteStillAuthorized(tx, invite); err != nil {
			return err
		}
		if err := upsertDocumentPermission(tx, invite.DocumentID, userID, invite.InviterUserID, invite.Role); err != nil {
			return err
		}

		now := time.Now()
		if err := tx.Model(&models.DocumentInvite{}).
			Where("id = ?", invite.ID).
			Updates(map[string]any{
				"status":     documentInviteStatusAccepted,
				"updated_at": now,
			}).Error; err != nil {
			return err
		}

		return tx.Model(&models.Notification{}).
			Where("user_id = ? AND type = ? AND group_key = ?", userID, notificationTypeDocumentInvite, "doc:"+invite.DocumentID.String()).
			Updates(map[string]any{
				"read_at":    now,
				"updated_at": now,
			}).Error
	})
}

func DeclineDocumentInvite(userID, inviteID uuid.UUID) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		var invite models.DocumentInvite
		if err := tx.Where("id = ? AND invitee_user_id = ?", inviteID, userID).First(&invite).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInviteNotFound
			}
			return err
		}

		now := time.Now()
		if err := tx.Model(&models.DocumentInvite{}).
			Where("id = ?", invite.ID).
			Updates(map[string]any{
				"status":     documentInviteStatusDeclined,
				"updated_at": now,
			}).Error; err != nil {
			return err
		}

		if err := tx.Where("document_id = ? AND user_id = ?", invite.DocumentID, userID).Delete(&models.DocumentPermission{}).Error; err != nil {
			return err
		}

		return tx.Model(&models.Notification{}).
			Where("user_id = ? AND type = ? AND group_key = ?", userID, notificationTypeDocumentInvite, "doc:"+invite.DocumentID.String()).
			Updates(map[string]any{
				"read_at":    now,
				"updated_at": now,
			}).Error
	})
}

func ensureDocumentInviteStillAuthorized(tx *gorm.DB, invite models.DocumentInvite) error {
	_, inviterRole, err := acl.CanManageDocumentMembers(tx, invite.InviterUserID, invite.DocumentID)
	if err != nil {
		if errors.Is(err, acl.ErrDocumentNotFoundOrForbidden) {
			return ErrInviteInvalidStatus
		}
		return err
	}
	if inviterRole == acl.RoleCollaborator && invite.Role == acl.RoleCollaborator {
		return ErrInviteInvalidRole
	}
	return nil
}

func upsertDocumentPermission(tx *gorm.DB, documentID, targetUserID, createdBy uuid.UUID, role string) error {
	var permission models.DocumentPermission
	err := tx.Unscoped().Where("document_id = ? AND user_id = ?", documentID, targetUserID).First(&permission).Error
	switch {
	case err == nil:
		now := time.Now()
		if err := tx.Unscoped().Model(&models.DocumentPermission{}).
			Where("id = ?", permission.ID).
			Update("deleted_at", nil).Error; err != nil {
			return err
		}
		return tx.Unscoped().Model(&models.DocumentPermission{}).
			Where("id = ?", permission.ID).
			Updates(map[string]any{
				"role":       role,
				"updated_at": now,
			}).Error
	case errors.Is(err, gorm.ErrRecordNotFound):
		return tx.Create(&models.DocumentPermission{
			ID:         uuid.New(),
			DocumentID: documentID,
			UserID:     targetUserID,
			Role:       role,
			CreatedBy:  createdBy,
		}).Error
	default:
		return err
	}
}

func upsertDocumentInvite(tx *gorm.DB, documentID, inviterUserID, inviteeUserID uuid.UUID, role string, now time.Time) (*models.DocumentInvite, error) {
	base, multiplier, maxSeconds := getInviteCooldownConfig()

	var invite models.DocumentInvite
	err := tx.Unscoped().
		Where("document_id = ? AND inviter_user_id = ? AND invitee_user_id = ?", documentID, inviterUserID, inviteeUserID).
		First(&invite).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		invite = models.DocumentInvite{
			ID:            uuid.New(),
			DocumentID:    documentID,
			InviterUserID: inviterUserID,
			InviteeUserID: inviteeUserID,
			Role:          role,
			Status:        documentInviteStatusSent,
			ResendCount:   0,
			LastSentAt:    now,
		}
		if err := tx.Create(&invite).Error; err != nil {
			return nil, err
		}
		return &invite, nil
	case err != nil:
		return nil, err
	default:
		cooldownSeconds := base
		for i := 0; i < invite.ResendCount; i++ {
			cooldownSeconds *= multiplier
			if cooldownSeconds >= maxSeconds {
				cooldownSeconds = maxSeconds
				break
			}
		}
		nextAllowedAt := invite.LastSentAt.Add(time.Duration(cooldownSeconds) * time.Second)
		if now.Before(nextAllowedAt) {
			remainingSeconds := int(nextAllowedAt.Sub(now).Seconds())
			if remainingSeconds < 1 {
				remainingSeconds = 1
			}
			return nil, &InviteRateLimitError{RemainingSeconds: remainingSeconds}
		}
		nextResendCount := invite.ResendCount + 1
		if err := tx.Unscoped().Model(&models.DocumentInvite{}).
			Where("id = ?", invite.ID).
			Updates(map[string]any{
				"role":         role,
				"status":       documentInviteStatusSent,
				"resend_count": nextResendCount,
				"last_sent_at": now,
				"deleted_at":   nil,
				"updated_at":   now,
			}).Error; err != nil {
			return nil, err
		}
		invite.Role = role
		invite.Status = documentInviteStatusSent
		invite.ResendCount = nextResendCount
		invite.LastSentAt = now
		return &invite, nil
	}
}
