package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	AdminRoleAdmin = "admin"

	DocumentQuotaModeInherit   = "inherit"
	DocumentQuotaModeCustom    = "custom"
	DocumentQuotaModeUnlimited = "unlimited"
)

// BeforeCreate will set a UUID rather than relying on the database to generate it.
func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return
}

// User represents the core user model
type User struct {
	ID                uuid.UUID `gorm:"type:uuid;primary_key"`
	Email             *string   `gorm:"unique"`
	EmailVerified     bool      `gorm:"not null;default:false"`
	EmailVerifiedAt   *time.Time
	DisplayName       *string
	AvatarURL         *string
	AvatarObjectKey   *string
	AdminRole         *string `gorm:"type:varchar(50);index"`
	AdminGrantedAt    *time.Time
	AdminGrantedBy    *uuid.UUID `gorm:"type:uuid"`
	DocumentQuotaMode string     `gorm:"type:varchar(20);not null;default:'inherit'"`
	DocumentQuota     *int
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// BeforeCreate will set a UUID rather than relying on the database to generate it.
func (uib *UserImageBedConfig) BeforeCreate(tx *gorm.DB) (err error) {
	if uib.ID == uuid.Nil {
		uib.ID = uuid.New()
	}
	return
}

// UserImageBedConfig stores one user-defined public image-bed target.
type UserImageBedConfig struct {
	ID           uuid.UUID `gorm:"type:uuid;primary_key"`
	UserID       uuid.UUID `gorm:"not null;index"`
	User         User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE;"`
	Name         string    `gorm:"type:varchar(120);not null"`
	ProviderType string    `gorm:"type:varchar(50);not null;index"`
	BaseURL      *string   `gorm:"type:varchar(255)"`
	APIToken     *string   `gorm:"type:text"`
	ConfigJSON   *string   `gorm:"type:text"`
	IsEnabled    bool      `gorm:"not null;default:true"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

// BeforeCreate will set a UUID rather than relying on the database to generate it.
func (a *AuthProvider) BeforeCreate(tx *gorm.DB) (err error) {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return
}

// AuthProvider stores configuration for an OIDC or OAuth2 provider
type AuthProvider struct {
	ID                    uuid.UUID `gorm:"type:uuid;primary_key"`
	Name                  string    `gorm:"type:varchar(100);not null;unique"`
	DisplayName           *string   `gorm:"type:varchar(100)"`
	ProtocolType          string    `gorm:"type:varchar(20);not null;default:'oidc'"`
	IssuerURL             *string   `gorm:"type:varchar(255)"`
	AuthURL               *string   `gorm:"type:varchar(255)"` // For OAuth2
	TokenURL              *string   `gorm:"type:varchar(255)"` // For OAuth2
	UserInfoURL           *string   `gorm:"type:varchar(255)"`
	ClientID              string    `gorm:"type:varchar(255);not null"`
	ClientSecretEncrypted string    `gorm:"type:text;not null"`
	IconURL               *string   `gorm:"type:varchar(255)"`
	Scopes                string    `gorm:"type:varchar(255);not null"`
	IsActive              bool      `gorm:"not null;default:true"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// BeforeCreate will set a UUID rather than relying on the database to generate it.
func (uip *UserIdentityProvider) BeforeCreate(tx *gorm.DB) (err error) {
	if uip.ID == uuid.Nil {
		uip.ID = uuid.New()
	}
	return
}

// UserIdentityProvider links a user to an OIDC identity
type UserIdentityProvider struct {
	ID             uuid.UUID `gorm:"type:uuid;primary_key"`
	UserID         uuid.UUID `gorm:"not null"`
	User           User      `gorm:"foreignKey:UserID"`
	ProviderName   string    `gorm:"type:varchar(100);not null"`
	ProviderUserID string    `gorm:"type:varchar(255);not null"`
	CreatedAt      time.Time

	// Unique constraints
	// _      struct{} `gorm:"uniqueIndex:idx_user_provider,columns:user_id,provider_name"`
	// _      struct{} `gorm:"uniqueIndex:idx_provider_user,columns:provider_name,provider_user_id"`
}

// BeforeCreate will set a UUID rather than relying on the database to generate it.
func (us *UserSession) BeforeCreate(tx *gorm.DB) (err error) {
	if us.ID == uuid.Nil {
		us.ID = uuid.New()
	}
	return
}

// BeforeCreate will set a UUID rather than relying on the database to generate it.
func (t *ApiToken) BeforeCreate(tx *gorm.DB) (err error) {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return
}

// BeforeCreate will set a UUID rather than relying on the database to generate it.
func (c *SkillOAuthCode) BeforeCreate(tx *gorm.DB) (err error) {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return
}

// BeforeCreate will set a UUID rather than relying on the database to generate it.
func (r *SkillOAuthRequest) BeforeCreate(tx *gorm.DB) (err error) {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return
}

// ApiToken stores a long-lived user-created API credential. Only a hash of the
// raw token is persisted; the raw token is shown once at creation time.
type ApiToken struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key"`
	UserID      uuid.UUID `gorm:"not null;index"`
	User        User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE;"`
	Name        string    `gorm:"type:varchar(120);not null"`
	TokenPrefix string    `gorm:"type:varchar(32);not null;index"`
	TokenHash   string    `gorm:"type:varchar(64);not null;uniqueIndex"`
	Scopes      string    `gorm:"type:text;not null"`
	LastUsedAt  *time.Time
	LastUsedIP  string `gorm:"type:varchar(64);not null;default:''"`
	ExpiresAt   *time.Time
	RevokedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// SkillOAuthCode stores a short-lived browser OAuth authorization code for
// issuing an API token to a skill client. Only a hash of the raw code is stored.
type SkillOAuthCode struct {
	ID                  uuid.UUID `gorm:"type:uuid;primary_key"`
	UserID              uuid.UUID `gorm:"not null;index"`
	User                User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE;"`
	ClientID            string    `gorm:"type:varchar(255);not null;default:''"`
	RedirectURI         string    `gorm:"type:text;not null"`
	CodeHash            string    `gorm:"type:varchar(64);not null;uniqueIndex"`
	CodeChallenge       string    `gorm:"type:text;not null;default:''"`
	CodeChallengeMethod string    `gorm:"type:varchar(16);not null;default:''"`
	Scopes              string    `gorm:"type:text;not null"`
	ExpiresAt           time.Time `gorm:"not null;index"`
	UsedAt              *time.Time
	CreatedAt           time.Time
}

// SkillOAuthRequest stores a short-lived authorization request that must be
// explicitly approved by the logged-in user before an authorization code exists.
type SkillOAuthRequest struct {
	ID                  uuid.UUID `gorm:"type:uuid;primary_key"`
	UserID              uuid.UUID `gorm:"not null;index"`
	User                User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE;"`
	ClientID            string    `gorm:"type:varchar(255);not null;default:''"`
	RedirectURI         string    `gorm:"type:text;not null"`
	State               string    `gorm:"type:text;not null;default:''"`
	CodeChallenge       string    `gorm:"type:text;not null;default:''"`
	CodeChallengeMethod string    `gorm:"type:varchar(16);not null;default:''"`
	Scopes              string    `gorm:"type:text;not null"`
	ExpiresAt           time.Time `gorm:"not null;index"`
	ConsumedAt          *time.Time
	ApprovedAt          *time.Time
	DeniedAt            *time.Time
	CreatedAt           time.Time
}

// UserSession stores a logical login session for one user.
type UserSession struct {
	ID          uuid.UUID `gorm:"type:uuid;primary_key"`
	UserID      uuid.UUID `gorm:"not null;index"`
	User        User      `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE;"`
	UserAgent   string    `gorm:"type:text;not null;default:''"`
	DeviceLabel string    `gorm:"type:varchar(255);not null;default:''"`
	LastSeenAt  time.Time `gorm:"not null;index"`
	RevokedAt   *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// BeforeCreate will set a UUID rather than relying on the database to generate it.
func (urt *UserRefreshToken) BeforeCreate(tx *gorm.DB) (err error) {
	if urt.ID == uuid.Nil {
		urt.ID = uuid.New()
	}
	return
}

// UserRefreshToken stores a user's long-lived refresh token.
type UserRefreshToken struct {
	ID        uuid.UUID   `gorm:"type:uuid;primary_key"`
	UserID    uuid.UUID   `gorm:"not null;index"`
	User      User        `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE;"`
	SessionID uuid.UUID   `gorm:"type:uuid;not null;index"`
	Session   UserSession `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE;"`
	TokenHash string      `gorm:"type:varchar(255);not null;uniqueIndex"`
	ExpiresAt time.Time   `gorm:"not null"`
	CreatedAt time.Time
}

// BeforeCreate will set a UUID rather than relying on the database to generate it.
func (f *Folder) BeforeCreate(tx *gorm.DB) (err error) {
	if f.ID == uuid.Nil {
		f.ID = uuid.New()
	}
	return
}

// Folder represents a folder in the workspace
type Folder struct {
	ID          uuid.UUID  `gorm:"type:uuid;primary_key"`
	OwnerUserID uuid.UUID  `gorm:"not null;index:idx_owner_parent"`
	ParentID    *uuid.UUID `gorm:"index:idx_owner_parent"`
	Name        string     `gorm:"type:varchar(255);not null"`
	Description *string    `gorm:"type:text"`
	CreatedBy   uuid.UUID  `gorm:"not null"`
	UpdatedBy   uuid.UUID  `gorm:"not null"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// BeforeCreate will set a UUID rather than relying on the database to generate it.
func (d *Document) BeforeCreate(tx *gorm.DB) (err error) {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return
}

// Document represents an editable workspace document (metadata only).
type Document struct {
	ID                     uuid.UUID  `gorm:"type:uuid;primary_key"`
	OwnerUserID            uuid.UUID  `gorm:"not null;index:idx_owner_folder"`
	FolderID               *uuid.UUID `gorm:"index:idx_owner_folder"`
	Title                  string     `gorm:"type:varchar(255);not null"`
	Excerpt                string     `gorm:"type:text"`
	ManualExcerpt          string     `gorm:"type:text;not null;default:''"`
	PublicAccess           string     `gorm:"type:varchar(20);not null;default:'private';index"`
	DocumentType           string     `gorm:"type:varchar(50);not null;default:'rich_text'"`
	PreferredImageTargetID string     `gorm:"type:varchar(100)"`
	EditorType             string     `gorm:"type:varchar(50);not null;default:'tiptap'"`
	CreatedBy              uuid.UUID  `gorm:"not null"`
	UpdatedBy              uuid.UUID  `gorm:"not null"`
	CreatedAt              time.Time
	UpdatedAt              time.Time
	DeletedAt              gorm.DeletedAt `gorm:"index"`
}

// BeforeCreate will set a UUID rather than relying on the database to generate it.
func (dbd *DocumentBody) BeforeCreate(tx *gorm.DB) (err error) {
	if dbd.ID == uuid.Nil {
		dbd.ID = uuid.New()
	}
	return
}

// DocumentBody stores current canonical editor content for a document.
//
// YjsVersion is an optimistic-concurrency token bumped on every successful
// PUT /api/v1/realtime/documents/:id/state. Writers must echo the version
// they last observed; mismatches are rejected as 409 Conflict so a stale
// or malicious client cannot blindly overwrite a fresher CRDT state.
type DocumentBody struct {
	ID             uuid.UUID      `gorm:"type:uuid;primary_key"`
	DocumentID     uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex"`
	ContentJSON    string         `gorm:"type:text;not null"`
	PlainText      string         `gorm:"type:text;not null;default:''"`
	ContentVersion int64          `gorm:"not null;default:1"`
	YjsState       string         `gorm:"type:text"`
	YjsStateVector string         `gorm:"type:text"`
	YjsVersion     int64          `gorm:"not null;default:1"`
	UpdatedBy      uuid.UUID      `gorm:"not null"`
	CreatedAt      time.Time      `gorm:"autoCreateTime"`
	UpdatedAt      time.Time      `gorm:"autoUpdateTime"`
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

// BeforeCreate will set a UUID rather than relying on the database to generate it.
func (p *DocumentPermission) BeforeCreate(tx *gorm.DB) (err error) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return
}

// DocumentPermission stores per-user access to a document.
type DocumentPermission struct {
	ID         uuid.UUID      `gorm:"type:uuid;primary_key"`
	DocumentID uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_document_user_permission"`
	UserID     uuid.UUID      `gorm:"type:uuid;not null;index;uniqueIndex:idx_document_user_permission"`
	Role       string         `gorm:"type:varchar(20);not null;default:'viewer';index"`
	CreatedBy  uuid.UUID      `gorm:"type:uuid;not null"`
	CreatedAt  time.Time      `gorm:"autoCreateTime"`
	UpdatedAt  time.Time      `gorm:"autoUpdateTime"`
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}

// BeforeCreate will set a UUID rather than relying on the database to generate it.
func (p *DocumentImageTargetPreference) BeforeCreate(tx *gorm.DB) (err error) {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return
}

// DocumentImageTargetPreference stores per-user image upload target for one document.
type DocumentImageTargetPreference struct {
	ID         uuid.UUID      `gorm:"type:uuid;primary_key"`
	DocumentID uuid.UUID      `gorm:"type:uuid;not null;index;uniqueIndex:idx_document_user_image_target"`
	UserID     uuid.UUID      `gorm:"type:uuid;not null;index;uniqueIndex:idx_document_user_image_target"`
	TargetID   string         `gorm:"type:varchar(100);not null;default:'managed-r2'"`
	CreatedAt  time.Time      `gorm:"autoCreateTime"`
	UpdatedAt  time.Time      `gorm:"autoUpdateTime"`
	DeletedAt  gorm.DeletedAt `gorm:"index"`
}

// BeforeCreate will set a UUID rather than relying on the database to generate it.
func (i *DocumentInvite) BeforeCreate(tx *gorm.DB) (err error) {
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}
	return
}

// DocumentInvite stores share invitation records for rate limiting and inbox notifications.
type DocumentInvite struct {
	ID            uuid.UUID      `gorm:"type:uuid;primary_key"`
	DocumentID    uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_document_invite_scope"`
	InviterUserID uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_document_invite_scope"`
	InviteeUserID uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_document_invite_scope"`
	Role          string         `gorm:"type:varchar(20);not null;default:'viewer'"`
	Status        string         `gorm:"type:varchar(20);not null;default:'sent';index"`
	ResendCount   int            `gorm:"not null;default:0"`
	LastSentAt    time.Time      `gorm:"not null"`
	CreatedAt     time.Time      `gorm:"autoCreateTime"`
	UpdatedAt     time.Time      `gorm:"autoUpdateTime"`
	DeletedAt     gorm.DeletedAt `gorm:"index"`
}

// BeforeCreate will set a UUID rather than relying on the database to generate it.
func (n *Notification) BeforeCreate(tx *gorm.DB) (err error) {
	if n.ID == uuid.Nil {
		n.ID = uuid.New()
	}
	return
}

// Notification stores user-visible activity entries for workspace inbox.
type Notification struct {
	ID        uuid.UUID `gorm:"type:uuid;primary_key"`
	UserID    uuid.UUID `gorm:"type:uuid;not null;index"`
	Type      string    `gorm:"type:varchar(50);not null;index"`
	GroupKey  string    `gorm:"type:varchar(120);not null;index"`
	DataJSON  string    `gorm:"type:text;not null"`
	ReadAt    *time.Time
	CreatedAt time.Time      `gorm:"autoCreateTime"`
	UpdatedAt time.Time      `gorm:"autoUpdateTime"`
	DeletedAt gorm.DeletedAt `gorm:"index"`
}

// BeforeCreate will set a UUID rather than relying on the database to generate it.
func (a *Asset) BeforeCreate(tx *gorm.DB) (err error) {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return
}

// Asset represents a stored binary resource such as an image, video, or file.
type Asset struct {
	ID             uuid.UUID  `gorm:"type:uuid;primary_key"`
	OwnerUserID    uuid.UUID  `gorm:"not null;index:idx_owner_document;uniqueIndex:idx_owner_blob_asset"`
	DocumentID     *uuid.UUID `gorm:"type:uuid;index:idx_owner_document"`
	BlobID         uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:idx_owner_blob_asset"`
	Kind           string     `gorm:"type:varchar(20);not null;default:'image'"`
	Filename       string     `gorm:"type:varchar(255);not null"`
	URL            string     `gorm:"type:text;not null"`
	Visibility     string     `gorm:"type:varchar(20);not null;default:'private'"`
	AltText        *string    `gorm:"type:text"`
	Width          *int       `gorm:"type:int"`
	Height         *int       `gorm:"type:int"`
	Status         string     `gorm:"type:varchar(20);not null;default:'ready'"`
	ReferenceCount int        `gorm:"not null;default:0;index"`
	CreatedBy      uuid.UUID  `gorm:"not null"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      gorm.DeletedAt `gorm:"index"`
}

// BeforeCreate will set a UUID rather than relying on the database to generate it.
func (b *BlobObject) BeforeCreate(tx *gorm.DB) (err error) {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return
}

// BlobObject represents one deduplicated physical object in object storage.
type BlobObject struct {
	ID                 uuid.UUID `gorm:"type:uuid;primary_key"`
	OwnerUserID        uuid.UUID `gorm:"type:uuid;not null;default:'00000000-0000-0000-0000-000000000000';index:idx_blob_owner;uniqueIndex:idx_blob_owner_hash_size"`
	SHA256             string    `gorm:"type:varchar(64);not null;uniqueIndex:idx_blob_owner_hash_size"`
	Size               int64     `gorm:"not null;uniqueIndex:idx_blob_owner_hash_size"`
	MimeType           string    `gorm:"type:varchar(100);not null"`
	StorageProvider    string    `gorm:"type:varchar(50);not null;default:'r2'"`
	Bucket             string    `gorm:"type:varchar(255)"`
	ObjectKey          string    `gorm:"type:varchar(255);not null;unique"`
	ThumbnailObjectKey string    `gorm:"type:varchar(255)"`
	ThumbnailMimeType  string    `gorm:"type:varchar(100)"`
	ThumbnailSize      int64
	ThumbnailWidth     *int
	ThumbnailHeight    *int
	ThumbnailStatus    string `gorm:"type:varchar(20);not null;default:'pending'"`
	URL                string `gorm:"type:text;not null"`
	Status             string `gorm:"type:varchar(20);not null;default:'ready'"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          gorm.DeletedAt `gorm:"index"`
}

// BeforeCreate will set a UUID rather than relying on the database to generate it.
func (r *DocumentAssetRef) BeforeCreate(tx *gorm.DB) (err error) {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return
}

// DocumentAssetRef is the source of truth for whether a document references an asset.
type DocumentAssetRef struct {
	ID          uuid.UUID      `gorm:"type:uuid;primary_key"`
	DocumentID  uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_document_asset_ref"`
	AssetID     uuid.UUID      `gorm:"type:uuid;not null;uniqueIndex:idx_document_asset_ref;index"`
	OwnerUserID uuid.UUID      `gorm:"type:uuid;not null;index:idx_owner_document_asset_ref"`
	RefType     string         `gorm:"type:varchar(50);not null;default:'editor_content';uniqueIndex:idx_document_asset_ref"`
	CreatedAt   time.Time      `gorm:"autoCreateTime"`
	UpdatedAt   time.Time      `gorm:"autoUpdateTime"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

// BeforeCreate will set a UUID rather than relying on the database to generate it.
func (j *AssetGCJob) BeforeCreate(tx *gorm.DB) (err error) {
	if j.ID == uuid.Nil {
		j.ID = uuid.New()
	}
	return
}

// AssetGCJob stores delayed cleanup work for unused assets.
type AssetGCJob struct {
	ID           uuid.UUID      `gorm:"type:uuid;primary_key"`
	AssetID      uuid.UUID      `gorm:"type:uuid;not null;index"`
	JobType      string         `gorm:"type:varchar(20);not null;index"`
	Status       string         `gorm:"type:varchar(20);not null;default:'pending';index"`
	RunAfter     time.Time      `gorm:"not null;index"`
	AttemptCount int            `gorm:"not null;default:0"`
	LastError    *string        `gorm:"type:text"`
	CreatedAt    time.Time      `gorm:"autoCreateTime"`
	UpdatedAt    time.Time      `gorm:"autoUpdateTime"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

// BeforeCreate will set a UUID rather than relying on the database to generate it.
func (j *BlobGCJob) BeforeCreate(tx *gorm.DB) (err error) {
	if j.ID == uuid.Nil {
		j.ID = uuid.New()
	}
	return
}

// BlobGCJob stores delayed cleanup work for unused physical objects.
type BlobGCJob struct {
	ID           uuid.UUID      `gorm:"type:uuid;primary_key"`
	BlobID       uuid.UUID      `gorm:"type:uuid;not null;index"`
	JobType      string         `gorm:"type:varchar(20);not null;index"`
	Status       string         `gorm:"type:varchar(20);not null;default:'pending';index"`
	RunAfter     time.Time      `gorm:"not null;index"`
	AttemptCount int            `gorm:"not null;default:0"`
	LastError    *string        `gorm:"type:text"`
	CreatedAt    time.Time      `gorm:"autoCreateTime"`
	UpdatedAt    time.Time      `gorm:"autoUpdateTime"`
	DeletedAt    gorm.DeletedAt `gorm:"index"`
}

// Set GORM table names to use snake_case
func (User) TableName() string {
	return "users"
}

func (AuthProvider) TableName() string {
	return "auth_providers"
}

func (UserIdentityProvider) TableName() string {
	return "user_identity_providers"
}

func (UserSession) TableName() string {
	return "user_sessions"
}

func (UserRefreshToken) TableName() string {
	return "user_refresh_tokens"
}

func (ApiToken) TableName() string {
	return "api_tokens"
}

func (Folder) TableName() string {
	return "folders"
}

func (Document) TableName() string {
	return "documents"
}

func (DocumentBody) TableName() string {
	return "document_bodies"
}

func (DocumentPermission) TableName() string {
	return "document_permissions"
}

func (DocumentInvite) TableName() string {
	return "document_invites"
}

func (Notification) TableName() string {
	return "notifications"
}

func (BlobObject) TableName() string {
	return "blob_objects"
}

func (Asset) TableName() string {
	return "assets"
}

func (DocumentAssetRef) TableName() string {
	return "document_asset_refs"
}

func (AssetGCJob) TableName() string {
	return "asset_gc_jobs"
}

func (BlobGCJob) TableName() string {
	return "blob_gc_jobs"
}
