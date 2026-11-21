package middleware

import (
	"context"
	"log/slog"
	"slices"
	"strings"

	"cholobangladesh-server/internal/platform/captcha"
	"cholobangladesh-server/pkg/apperror"
	"cholobangladesh-server/pkg/response"
	"cholobangladesh-server/pkg/token"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	ctxUserID    = "auth.user_id"
	ctxRole      = "auth.role"
	ctxSessionID = "auth.session_id"
)

// SessionValidator checks server-side state the JWT alone cannot express:
// revoked sessions, bumped token versions (logout-all / password reset) and
// suspended accounts. Implemented by the user service (Redis-cached).
type SessionValidator interface {
	ValidateAccess(ctx context.Context, t *token.Parsed) error
}

// Auth requires a valid "Authorization: Bearer <access_token>" header.
func Auth(tm *token.Manager, sv SessionValidator) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, found := strings.CutPrefix(c.GetHeader("Authorization"), "Bearer ")
		if !found || raw == "" || len(raw) > 4096 {
			response.Fail(c, apperror.ErrUnauthorized)
			return
		}
		parsed, err := tm.Parse(raw)
		if err != nil {
			response.Fail(c, apperror.ErrTokenInvalid)
			return
		}
		if err := sv.ValidateAccess(c.Request.Context(), parsed); err != nil {
			response.Fail(c, err)
			return
		}
		c.Set(ctxUserID, parsed.UserID)
		c.Set(ctxRole, parsed.Role)
		c.Set(ctxSessionID, parsed.SessionID)
		c.Next()
	}
}

// RequireRole("admin") or RequireRole("guide", "admin"). Must run after Auth.
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !slices.Contains(roles, c.GetString(ctxRole)) {
			response.Fail(c, apperror.ErrForbidden)
			return
		}
		c.Next()
	}
}

// Captcha requires a valid Cloudflare Turnstile token in the X-Captcha-Token header
// (no-op when CAPTCHA_ENABLED=false).
func Captcha(t *captcha.Turnstile) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !t.Enabled() {
			c.Next()
			return
		}
		ok, err := t.Verify(c.Request.Context(), c.GetHeader("X-Captcha-Token"), c.ClientIP())
		if err != nil {
			slog.Warn("captcha verify error", "err", err)
		}
		if !ok {
			response.Fail(c, apperror.ErrCaptcha)
			return
		}
		c.Next()
	}
}

func UserID(c *gin.Context) uuid.UUID {
	v, _ := c.Get(ctxUserID)
	id, _ := v.(uuid.UUID)
	return id
}

func SessionID(c *gin.Context) uuid.UUID {
	v, _ := c.Get(ctxSessionID)
	id, _ := v.(uuid.UUID)
	return id
}

func Role(c *gin.Context) string { return c.GetString(ctxRole) }
