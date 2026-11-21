package domain

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned by repositories when no row/key matches.
var ErrNotFound = errors.New("not found")

// ErrDuplicate is returned on unique-constraint violations.
var ErrDuplicate = errors.New("duplicate")

// ---------------------------------------------------------------- persistence

type UserFilter struct {
	Role   Role
	Status Status
	Query  string // email / phone / name contains
	Page   int
	Limit  int
}

type GuideFilter struct {
	Area      string
	Language  string
	Specialty string
	MinRate   *int
	MaxRate   *int
	Query     string
	Sort      string // rating | experience | price_low | price_high | newest
	Page      int
	Limit     int
}

type VerificationFilter struct {
	Status VerificationStatus
	Page   int
	Limit  int
}

// Repository is the persistence port. Tx runs fn atomically.
type Repository interface {
	Tx(ctx context.Context, fn func(r Repository) error) error

	// users
	CreateUser(ctx context.Context, u *User) error // also inserts Profile / GuideProfile when set
	FindUserByID(ctx context.Context, id uuid.UUID) (*User, error)
	FindUserByEmail(ctx context.Context, email string) (*User, error)
	FindUserByPhone(ctx context.Context, phone string) (*User, error)
	EmailExists(ctx context.Context, email string) (bool, error)
	PhoneExists(ctx context.Context, phone string) (bool, error)
	UpdateUser(ctx context.Context, id uuid.UUID, fields map[string]any) error
	BumpTokenVersion(ctx context.Context, id uuid.UUID) (int, error)
	SoftDeleteUser(ctx context.Context, id uuid.UUID) error
	ListUsers(ctx context.Context, f UserFilter) ([]User, int64, error)
	AuthState(ctx context.Context, id uuid.UUID) (tokenVersion int, status Status, err error)

	// profiles
	UpdateProfile(ctx context.Context, userID uuid.UUID, fields map[string]any) error
	UpdateGuideProfile(ctx context.Context, userID uuid.UUID, fields map[string]any) error
	SearchGuides(ctx context.Context, f GuideFilter) ([]User, int64, error)

	// oauth identities
	FindIdentity(ctx context.Context, provider, subject string) (*AuthIdentity, error)
	CreateIdentity(ctx context.Context, i *AuthIdentity) error
	TouchIdentity(ctx context.Context, id uuid.UUID) error
	ListIdentityProviders(ctx context.Context, userID uuid.UUID) ([]string, error)
	DeleteIdentities(ctx context.Context, userID uuid.UUID) error

	// sessions
	CreateSession(ctx context.Context, s *Session) error
	FindSessionByID(ctx context.Context, id uuid.UUID) (*Session, error)
	FindSessionByTokenHash(ctx context.Context, hash string) (*Session, error)
	FindSessionByPreviousHash(ctx context.Context, hash string) (*Session, error)
	RotateSession(ctx context.Context, id uuid.UUID, oldHash, newHash string, expiresAt time.Time, meta RequestMeta) (bool, error)
	RevokeSession(ctx context.Context, userID, id uuid.UUID) (bool, error)
	RevokeAllSessions(ctx context.Context, userID uuid.UUID, except *uuid.UUID) ([]uuid.UUID, error)
	ListActiveSessions(ctx context.Context, userID uuid.UUID) ([]Session, error)
	PurgeExpiredSessions(ctx context.Context, before time.Time) (int64, error)

	// guide KYC
	CreateVerification(ctx context.Context, v *GuideVerification) error
	FindVerificationByID(ctx context.Context, id uuid.UUID) (*GuideVerification, error)
	FindLatestVerification(ctx context.Context, userID uuid.UUID) (*GuideVerification, error)
	ListVerifications(ctx context.Context, f VerificationFilter) ([]GuideVerification, int64, error)
	ReviewVerification(ctx context.Context, id uuid.UUID, status VerificationStatus, reviewer uuid.UUID, reason *string) (bool, error)

	// audit
	Audit(ctx context.Context, l *AuditLog) error
}

// ---------------------------------------------------------------- ephemeral state

// OTPRecord is what is kept in the store for an issued code (never the code itself).
type OTPRecord struct {
	Hash     string
	Attempts int
}

// Store holds short-lived security state (OTPs, lockouts, tickets, caches).
type Store interface {
	SaveOTP(ctx context.Context, key, hash string, ttl time.Duration) error
	GetOTP(ctx context.Context, key string) (*OTPRecord, error) // ErrNotFound when absent
	IncrOTPAttempts(ctx context.Context, key string) (int, error)

	// Incr increments a counter, setting the window TTL on first hit.
	Incr(ctx context.Context, key string, window time.Duration) (int64, error)
	// SetNX sets a flag only if absent; returns false if it already existed.
	SetNX(ctx context.Context, key string, ttl time.Duration) (bool, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Get(ctx context.Context, key string) (string, error)        // ErrNotFound when absent
	TTL(ctx context.Context, key string) (time.Duration, error) // 0 when absent
	Del(ctx context.Context, keys ...string) (int64, error)
}

// ---------------------------------------------------------------- external services

type Mailer interface {
	Send(ctx context.Context, to, subject, text, html string) error
}

type SMSSender interface {
	Send(ctx context.Context, e164, text string) error
}

type UploadedFile struct {
	URL      string
	PublicID string
}

// FileStorage stores images. Private files are not publicly reachable.
type FileStorage interface {
	UploadPublic(ctx context.Context, data []byte, path, publicID string) (*UploadedFile, error)
	UploadPrivate(ctx context.Context, data []byte, path, publicID string) (*UploadedFile, error)
	Delete(ctx context.Context, publicID string, private bool) error
	SignedURL(publicID string, ttl time.Duration) (string, error)
}

type ExternalIdentity struct {
	Provider      string
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	Picture       string
}

type IdentityVerifier interface {
	Verify(ctx context.Context, provider, rawToken, nonce string) (*ExternalIdentity, error)
}

var (
	ErrIdentityProviderDisabled = errors.New("identity provider disabled")
	ErrIdentityTokenInvalid     = errors.New("identity token invalid")
)
