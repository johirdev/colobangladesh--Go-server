package middleware

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	"cholobangladesh-server/pkg/apperror"
	"cholobangladesh-server/pkg/ratelimit"
	"cholobangladesh-server/pkg/response"

	"github.com/gin-gonic/gin"
)

type KeyFunc func(c *gin.Context) string

// ByIP keys on the client IP. gin only trusts X-Forwarded-For from TRUSTED_PROXIES,
// so attackers cannot rotate fake IPs through that header.
func ByIP(c *gin.Context) string { return c.ClientIP() }

// ByUserOrIP keys authenticated requests on the user, anonymous ones on IP.
func ByUserOrIP(c *gin.Context) string {
	if uid, ok := c.Get(ctxUserID); ok {
		return "u:" + toString(uid)
	}
	return "ip:" + c.ClientIP()
}

// RateLimit applies a fixed-window limit. If Redis is unavailable it fails open
// (logged) so that a cache outage does not take the whole API down.
func RateLimit(l *ratelimit.Limiter, bucket string, limit int, window time.Duration, key KeyFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 500*time.Millisecond)
		res, err := l.Allow(ctx, bucket+":"+key(c), limit, window)
		cancel()
		if err != nil {
			slog.Warn("rate limiter unavailable", "bucket", bucket, "err", err)
			c.Next()
			return
		}
		c.Header("X-RateLimit-Limit", strconv.Itoa(res.Limit))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))
		if !res.Allowed {
			response.Fail(c, apperror.ErrRateLimited.WithRetryAfter(res.RetryAfter))
			return
		}
		c.Next()
	}
}

func toString(v any) string {
	if s, ok := v.(interface{ String() string }); ok {
		return s.String()
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
