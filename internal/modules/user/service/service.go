// Package service implements the user module's use cases (registration, login,
// sessions, profiles, guide KYC, admin review). It depends only on domain ports.
package service

import (
	"context"
	"errors"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"cholobangladesh-server/internal/config"
	"cholobangladesh-server/internal/modules/user/domain"
	"cholobangladesh-server/pkg/apperror"
	"cholobangladesh-server/pkg/password"
	"cholobangladesh-server/pkg/phone"
	"cholobangladesh-server/pkg/secure"
	"cholobangladesh-server/pkg/token"

	"github.com/google/uuid"
)

type Config struct {
	AppName           string
	Production        bool
	RefreshTTL        time.Duration
	BlockTime         time.Duration
	FailWindow        time.Duration
	MaxFailedAttempts int
	PasswordResetTTL  time.Duration
	RegistrationTTL   time.Duration
	OTP               config.OTPConfig
	OTPSecret         []byte
	MaxUploadBytes    int64
	SMSEnabled        bool // false = dev mode: OTPs are logged instead of sent
	MailEnabled       bool
}

type Deps struct {
	Repo     domain.Repository
	Store    domain.Store
	Tokens   *token.Manager
	Hasher   *password.Hasher
	Cipher   *secure.Cipher
	Mailer   domain.Mailer
	SMS      domain.SMSSender
	Storage  domain.FileStorage // nil when Cloudinary is not configured
	Identity domain.IdentityVerifier
}

type Service struct {
	cfg Config
	Deps
}

func New(cfg Config, d Deps) *Service { return &Service{cfg: cfg, Deps: d} }

// ------------------------------------------------------------------ helpers

func newID() uuid.UUID { return uuid.Must(uuid.NewV7()) } // time-ordered, index friendly

func now() time.Time { return time.Now().UTC() }

func ptr[T any](v T) *T { return &v }

func normalizeEmail(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s || len(s) > 150 {
		return "", false
	}
	return s, true
}

type identifierKind int

const (
	kindEmail identifierKind = iota
	kindPhone
)

// parseIdentifier accepts an email or a Bangladeshi mobile number.
func parseIdentifier(raw string) (identifierKind, string, error) {
	if strings.Contains(raw, "@") {
		if e, ok := normalizeEmail(raw); ok {
			return kindEmail, e, nil
		}
		return 0, "", domain.ErrInvalidIdentifier
	}
	if p, ok := phone.NormalizeBD(raw); ok {
		return kindPhone, p, nil
	}
	return 0, "", domain.ErrInvalidIdentifier
}

func (s *Service) findByIdentifier(ctx context.Context, kind identifierKind, ident string) (*domain.User, error) {
	if kind == kindEmail {
		return s.Repo.FindUserByEmail(ctx, ident)
	}
	return s.Repo.FindUserByPhone(ctx, ident)
}

// contactVerified reports whether ident is a verified contact of u (OTP may only go there).
func contactVerified(u *domain.User, kind identifierKind) bool {
	if kind == kindEmail {
		return u.EmailVerified()
	}
	return u.PhoneVerified()
}

func accountUsable(u *domain.User) error {
	switch u.Status {
	case domain.StatusActive:
		return nil
	case domain.StatusSuspended:
		return domain.ErrAccountSuspended
	default:
		return domain.ErrAccountDeleted
	}
}

func (s *Service) audit(ctx context.Context, userID *uuid.UUID, actor *uuid.UUID, event string, meta domain.RequestMeta, data map[string]any) {
	l := &domain.AuditLog{UserID: userID, ActorID: actor, Event: event, IPAddress: meta.IP,
		UserAgent: truncate(meta.UserAgent, 255)}
	if data != nil {
		l.Metadata = domain.JSONMap(data)
	}
	if err := s.Repo.Audit(context.WithoutCancel(ctx), l); err != nil {
		slog.Warn("audit log failed", "event", event, "err", err)
	}
}

// async runs side effects (notifications) outside the request path.
func (s *Service) async(name string, fn func(ctx context.Context) error) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("async task panic", "task", name, "panic", r)
			}
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := fn(ctx); err != nil {
			slog.Warn("async task failed", "task", name, "err", err)
		}
	}()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func notFound(err error, as *apperror.Error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return as
	}
	return err
}

func pageDefaults(page, limit, def int) (int, int) {
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = def
	}
	return page, limit
}
