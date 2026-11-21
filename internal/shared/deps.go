// Package shared holds infrastructure dependencies passed to every module.
package shared

import (
	"cholobangladesh-server/internal/config"
	"cholobangladesh-server/internal/platform/captcha"
	"cholobangladesh-server/internal/platform/mailer"
	"cholobangladesh-server/internal/platform/oauth"
	"cholobangladesh-server/internal/platform/sms"
	"cholobangladesh-server/internal/platform/storage"
	"cholobangladesh-server/pkg/ratelimit"
	"cholobangladesh-server/pkg/token"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type Deps struct {
	Config  *config.Config
	DB      *gorm.DB
	Redis   *redis.Client
	Tokens  *token.Manager
	Limiter *ratelimit.Limiter
	Captcha *captcha.Turnstile
	Mailer  *mailer.SMTP
	SMS     *sms.BulkSMSBD
	Storage *storage.Cloudinary // nil when CLOUDINARY_URL is not set
	OAuth   *oauth.Verifier
}
