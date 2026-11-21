// Package domain contains the user module's entities, business rules,
// errors and the ports (interfaces) the service layer depends on.
// It has no knowledge of HTTP, Redis or a specific SQL driver.
package domain

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

type Role string

const (
	RoleUser  Role = "user"  // tourist / traveller
	RoleGuide Role = "guide" // local guide (BD phone verified + KYC)
	RoleAdmin Role = "admin"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusSuspended Status = "suspended"
	StatusDeleted   Status = "deleted"
)

type VerificationStatus string

const (
	VerificationUnverified VerificationStatus = "unverified"
	VerificationPending    VerificationStatus = "pending"
	VerificationApproved   VerificationStatus = "approved"
	VerificationRejected   VerificationStatus = "rejected"
)

// User is the identity/authentication record. Personal data lives in UserProfile,
// guide marketplace data in GuideProfile.
type User struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	Role            Role
	Status          Status
	Email           *string
	EmailVerifiedAt *time.Time
	Phone           *string
	PhoneVerifiedAt *time.Time
	PasswordHash    *string
	TokenVersion    int
	LastLoginAt     *time.Time
	LastLoginIP     *string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt

	Profile      *UserProfile  `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
	GuideProfile *GuideProfile `gorm:"foreignKey:UserID;constraint:OnDelete:CASCADE"`
}

func (u *User) HasPassword() bool   { return u.PasswordHash != nil && *u.PasswordHash != "" }
func (u *User) EmailVerified() bool { return u.Email != nil && u.EmailVerifiedAt != nil }
func (u *User) PhoneVerified() bool { return u.Phone != nil && u.PhoneVerifiedAt != nil }
func (u *User) IsActive() bool      { return u.Status == StatusActive }

type UserProfile struct {
	UserID            uuid.UUID `gorm:"type:uuid;primaryKey"`
	FullName          string
	AvatarURL         *string
	AvatarPublicID    *string
	Bio               *string
	Gender            *string
	DateOfBirth       *time.Time `gorm:"type:date"`
	CountryCode       *string
	City              *string
	PreferredLanguage string
	WhatsappNumber    *string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type GuideProfile struct {
	UserID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	Headline           *string
	About              *string
	Languages          pq.StringArray `gorm:"type:text[]"`
	ServiceAreas       pq.StringArray `gorm:"type:text[]"`
	Specialties        pq.StringArray `gorm:"type:text[]"`
	ExperienceYears    int
	HourlyRate         *int
	DailyRate          *int
	Currency           string
	MaxGroupSize       int
	LicenseNumber      *string
	IsAvailable        bool
	VerificationStatus VerificationStatus
	VerifiedAt         *time.Time
	RatingAvg          float64
	RatingCount        int
	TotalTours         int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type GuideVerification struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID             uuid.UUID `gorm:"type:uuid"`
	NIDNumberEncrypted string    `gorm:"column:nid_number_encrypted"`
	NIDLast4           string    `gorm:"column:nid_last4"`
	NIDFrontPublicID   string    `gorm:"column:nid_front_public_id"`
	NIDBackPublicID    string    `gorm:"column:nid_back_public_id"`
	SelfiePublicID     string
	Status             VerificationStatus
	RejectionReason    *string
	ReviewedBy         *uuid.UUID `gorm:"type:uuid"`
	ReviewedAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time

	User *User `gorm:"foreignKey:UserID"`
}

type AuthIdentity struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID          uuid.UUID `gorm:"type:uuid"`
	Provider        string
	ProviderSubject string
	Email           *string
	CreatedAt       time.Time
	LastUsedAt      time.Time
}

type Session struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID            uuid.UUID `gorm:"type:uuid"`
	RefreshTokenHash  string
	PreviousTokenHash *string
	RotatedAt         *time.Time
	UserAgent         string
	IPAddress         string
	CreatedAt         time.Time
	LastUsedAt        time.Time
	ExpiresAt         time.Time
	RevokedAt         *time.Time
}

func (Session) TableName() string { return "user_sessions" }

type AuditLog struct {
	ID        int64      `gorm:"primaryKey"`
	UserID    *uuid.UUID `gorm:"type:uuid"`
	ActorID   *uuid.UUID `gorm:"type:uuid"`
	Event     string
	IPAddress string
	UserAgent string
	Metadata  JSONMap `gorm:"type:jsonb"`
	CreatedAt time.Time
}

// RequestMeta describes the client making a request (for sessions & audit logs).
type RequestMeta struct {
	IP        string
	UserAgent string
}

// Audit event names.
const (
	EventRegistered      = "user.registered"
	EventLoginSuccess    = "auth.login_success"
	EventLoginFailed     = "auth.login_failed"
	EventAccountLocked   = "auth.account_locked"
	EventOAuthLinked     = "auth.oauth_linked"
	EventRefreshReuse    = "auth.refresh_token_reuse"
	EventLogoutAll       = "auth.logout_all"
	EventPasswordChanged = "user.password_changed"
	EventPasswordReset   = "user.password_reset"
	EventContactVerified = "user.contact_verified"
	EventAccountDeleted  = "user.deleted"
	EventStatusChanged   = "admin.user_status_changed"
	EventKYCSubmitted    = "guide.kyc_submitted"
	EventKYCApproved     = "guide.kyc_approved"
	EventKYCRejected     = "guide.kyc_rejected"
)

// JSONMap is stored as JSONB.
type JSONMap map[string]any

func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	b, err := json.Marshal(m)
	return string(b), err
}
