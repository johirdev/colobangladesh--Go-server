// Package middleware contains the HTTP security layers applied in front of every module.
package middleware

import (
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"cholobangladesh-server/pkg/apperror"
	"cholobangladesh-server/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var requestIDRe = regexp.MustCompile(`^[A-Za-z0-9\-_.]{8,64}$`)

// RequestID propagates X-Request-ID (or creates one) for log correlation.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if !requestIDRe.MatchString(id) {
			id = uuid.NewString()
		}
		c.Set(response.RequestIDKey, id)
		c.Header("X-Request-ID", id)
		c.Next()
	}
}

// Logger writes one structured line per request. Bodies are never logged.
func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		status := c.Writer.Status()
		attrs := []any{
			"method", c.Request.Method,
			"route", c.FullPath(),
			"status", status,
			"latency_ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
			"request_id", c.GetString(response.RequestIDKey),
		}
		if uid, ok := c.Get(ctxUserID); ok {
			attrs = append(attrs, "user_id", uid)
		}
		switch {
		case status >= 500:
			slog.Error("http", attrs...)
		case status >= 400:
			slog.Warn("http", attrs...)
		default:
			slog.Info("http", attrs...)
		}
	}
}

// Recovery turns panics into a 500 envelope without leaking the stack to the client.
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, rec any) {
		slog.Error("panic recovered", "panic", rec, "route", c.FullPath(),
			"request_id", c.GetString(response.RequestIDKey))
		response.Fail(c, apperror.ErrInternal)
	})
}

// SecureHeaders sets defensive HTTP headers. Swagger UI is excluded from the strict CSP.
func SecureHeaders(production bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "geolocation=(), camera=(), microphone=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if !strings.HasPrefix(c.Request.URL.Path, "/swagger") {
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		}
		if production {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
		}
		c.Next()
	}
}

// NoStore prevents caching of responses that contain tokens or personal data.
func NoStore() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Header("Pragma", "no-cache")
		c.Next()
	}
}

// BodyLimit caps request bodies: JSON requests get jsonMax, multipart uploads get uploadMax.
func BodyLimit(jsonMax, uploadMax int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := jsonMax
		if strings.HasPrefix(c.ContentType(), "multipart/form-data") {
			limit = uploadMax
		}
		if c.Request.ContentLength > limit {
			response.Fail(c, apperror.ErrPayloadLarge)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}

// NotFound renders unknown routes in the standard envelope.
func NotFound() gin.HandlerFunc {
	return func(c *gin.Context) {
		response.Fail(c, apperror.ErrNotFound.WithMessage("route not found"))
	}
}

func MethodNotAllowed() gin.HandlerFunc {
	return func(c *gin.Context) {
		response.Fail(c, apperror.New(http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed"))
	}
}
