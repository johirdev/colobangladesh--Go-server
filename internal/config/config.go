// Package config loads and validates all runtime configuration from the
// environment (and an optional .env file in development).
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	App        AppConfig
	DB         DBConfig
	Redis      RedisConfig
	JWT        JWTConfig
	Security   SecurityConfig
	OTP        OTPConfig
	RateLimit  RateLimitConfig
	OAuth      OAuthConfig
	Captcha    CaptchaConfig
	Cloudinary CloudinaryConfig
	SMS        SMSConfig
	Mail       MailConfig
	Admin      AdminConfig
	Autofill   AutofillConfig
	Market     MarketConfig
	Payment    PaymentConfig
}

type AppConfig struct {
	Name           string
	Env            string
	Port           string
	TrustedProxies []string
	CORSOrigins    []string
	SwaggerEnabled bool
	MigrateOnStart bool
	// PublicAPIURL is how browsers / payment gateways reach this API (callbacks), e.g. https://api.example.com
	PublicAPIURL string
	// FrontendURL is where users are sent back after an online payment, e.g. https://example.com
	FrontendURL string
}

func (a AppConfig) IsProduction() bool { return a.Env == "production" }

type DBConfig struct {
	URL             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type JWTConfig struct {
	Secret     string
	Issuer     string
	AccessTTL  time.Duration
	RefreshTTL time.Duration
}

type SecurityConfig struct {
	BlockTime           time.Duration // USER_BLOCK_TIME
	MaxFailedAttempts   int           // USER_MAX_FAILED_ATTEMPTS
	FailedAttemptWindow time.Duration
	PasswordResetTTL    time.Duration
	RegistrationTTL     time.Duration
	OTPSecret           []byte
	DataEncryptionKey   []byte // 32 bytes, AES-256-GCM (NID numbers)
	MaxUploadBytes      int64
	MaxBodyBytes        int64
	BcryptCost          int
}

type OTPConfig struct {
	Length            int
	TTL               time.Duration
	ResendCooldown    time.Duration
	MaxAttempts       int
	MaxPerHour        int
	SMSMaxPerIPPerDay int
	DevCode           string // DEV_OTP_CODE: fixed OTP in development (never in production)
	DevExpose         bool   // DEV_EXPOSE_OTP: return the OTP as "dev_otp" in API responses
}

type RateLimitConfig struct {
	GlobalPerMinute int
	AuthPerMinute   int
	OTPSendPer10Min int
	UploadsPerHour  int
}

type OAuthConfig struct {
	GoogleClientIDs []string
	AppleClientIDs  []string
}

type CaptchaConfig struct {
	Enabled   bool
	SecretKey string
}

type CloudinaryConfig struct {
	URL    string
	Folder string
}

type SMSConfig struct {
	Enabled  bool
	APIURL   string
	APIKey   string
	SenderID string
}

type MailConfig struct {
	Enabled   bool
	Host      string
	Port      int
	Username  string
	Password  string
	FromEmail string
	FromName  string
}

type AdminConfig struct {
	Email    string
	Password string
	FullName string
}

// AutofillConfig: optional sources for the tourist spot autofill (Wikipedia needs no key).
type AutofillConfig struct {
	YouTubeAPIKey      string
	GoogleSearchAPIKey string
	GoogleSearchCX     string
	AIEnabled          bool // uses ANTHROPIC_API_KEY (read by the Anthropic SDK)
	AIModel            string
}

// MarketConfig: guide marketplace rules.
type MarketConfig struct {
	FreeGigLimit int // gigs a guide may publish without buying extra slots
}

// PaymentConfig: aamarPay online payments.
//
//	development: AAMARPAY_URL=https://sandbox.aamarpay.com/jsonpost.php
//	production : AAMARPAY_URL=https://secure.aamarpay.com/jsonpost.php
type PaymentConfig struct {
	AamarpayURL  string
	StoreID      string
	SignatureKey string
}

// Enabled reports whether online payments are configured.
func (p PaymentConfig) Enabled() bool {
	return p.AamarpayURL != "" && p.StoreID != "" && p.SignatureKey != ""
}

// Sandbox reports whether the sandbox gateway is configured.
func (p PaymentConfig) Sandbox() bool { return strings.Contains(p.AamarpayURL, "sandbox") }

// Load reads configuration. It returns an error describing every invalid value.
func Load() (*Config, error) {
	_ = godotenv.Load()

	var errs []error
	p := &parser{errs: &errs}

	cfg := &Config{
		App: AppConfig{
			Name:           p.str("APP_NAME", "CholoBangladesh"),
			Env:            p.str("APP_ENV", "development"),
			Port:           p.str("PORT", "8080"),
			TrustedProxies: p.list("TRUSTED_PROXIES"),
			CORSOrigins:    p.list("CORS_ORIGINS"),
			SwaggerEnabled: p.boolean("SWAGGER_ENABLED", true),
			MigrateOnStart: p.boolean("MIGRATE_ON_START", true),
			PublicAPIURL:   strings.TrimRight(p.str("PUBLIC_API_URL", "http://localhost:8080"), "/"),
			FrontendURL:    strings.TrimRight(p.str("FRONTEND_URL", "http://localhost:3000"), "/"),
		},
		DB: DBConfig{
			URL:             p.str("DATABASE_URL", ""),
			MaxOpenConns:    p.integer("DB_MAX_OPEN_CONNS", 25),
			MaxIdleConns:    p.integer("DB_MAX_IDLE_CONNS", 10),
			ConnMaxLifetime: p.duration("DB_CONN_MAX_LIFETIME", "30m"),
		},
		Redis: RedisConfig{
			Addr:     p.str("REDIS_ADDR", "localhost:6379"),
			Password: p.str("REDIS_PASSWORD", ""),
			DB:       p.integer("REDIS_DB", 0),
		},
		JWT: JWTConfig{
			Secret:     p.str("JWT_SECRET", ""),
			Issuer:     p.str("JWT_ISSUER", "cholobangladesh"),
			AccessTTL:  p.duration("JWT_ACCESS_EXPIRES_IN", "15m"),
			RefreshTTL: p.duration("JWT_REFRESH_EXPIRES_IN", "30d"),
		},
		Security: SecurityConfig{
			BlockTime:           p.duration("USER_BLOCK_TIME", "3m"),
			MaxFailedAttempts:   p.integer("USER_MAX_FAILED_ATTEMPTS", 5),
			FailedAttemptWindow: p.duration("USER_FAILED_ATTEMPT_WINDOW", "15m"),
			PasswordResetTTL:    p.duration("USER_PASSWORD_RESET_TOKEN_EXPIRE_IN", "15m"),
			RegistrationTTL:     p.duration("USER_REGISTRATION_TOKEN_EXPIRE_IN", "30m"),
			OTPSecret:           []byte(p.str("OTP_HMAC_SECRET", "")),
			DataEncryptionKey:   p.base64Key("DATA_ENCRYPTION_KEY"),
			MaxUploadBytes:      int64(p.integer("MAX_UPLOAD_SIZE_MB", 5)) << 20,
			MaxBodyBytes:        int64(p.integer("MAX_BODY_SIZE_KB", 1024)) << 10,
			BcryptCost:          p.integer("BCRYPT_COST", 12),
		},
		OTP: OTPConfig{
			Length:            p.integer("OTP_LENGTH", 6),
			TTL:               p.duration("OTP_EXPIRES_IN", "5m"),
			ResendCooldown:    p.duration("OTP_RESEND_COOLDOWN", "60s"),
			MaxAttempts:       p.integer("OTP_MAX_ATTEMPTS", 5),
			MaxPerHour:        p.integer("OTP_MAX_PER_HOUR", 5),
			SMSMaxPerIPPerDay: p.integer("SMS_MAX_PER_IP_PER_DAY", 10),
			DevCode:           p.str("DEV_OTP_CODE", ""),
			DevExpose:         p.boolean("DEV_EXPOSE_OTP", false),
		},
		RateLimit: RateLimitConfig{
			GlobalPerMinute: p.integer("RATE_LIMIT_GLOBAL_PER_MIN", 120),
			AuthPerMinute:   p.integer("RATE_LIMIT_AUTH_PER_MIN", 20),
			OTPSendPer10Min: p.integer("RATE_LIMIT_OTP_SEND_PER_10MIN", 5),
			UploadsPerHour:  p.integer("RATE_LIMIT_UPLOADS_PER_HOUR", 20),
		},
		OAuth: OAuthConfig{
			GoogleClientIDs: p.list("GOOGLE_CLIENT_IDS"),
			AppleClientIDs:  p.list("APPLE_CLIENT_IDS"),
		},
		Captcha: CaptchaConfig{
			Enabled:   p.boolean("CAPTCHA_ENABLED", false),
			SecretKey: p.str("TURNSTILE_SECRET_KEY", ""),
		},
		Cloudinary: CloudinaryConfig{
			URL:    p.str("CLOUDINARY_URL", ""),
			Folder: p.str("CLOUDINARY_FOLDER", "cholobangladesh"),
		},
		SMS: SMSConfig{
			Enabled:  p.boolean("SMS_ENABLED", false),
			APIURL:   p.str("BULKSMSBD_API_URL", "http://bulksmsbd.net/api/smsapi"),
			APIKey:   p.str("BULKSMSBD_API_KEY", ""),
			SenderID: p.str("BULKSMSBD_SENDER_ID", ""),
		},
		Mail: MailConfig{
			Enabled:   p.boolean("EMAIL_ENABLED", false),
			Host:      p.str("SMTP_HOST", "smtp.gmail.com"),
			Port:      p.integer("SMTP_PORT", 587),
			Username:  p.str("SMTP_USERNAME", ""),
			Password:  p.str("SMTP_PASSWORD", ""),
			FromEmail: p.str("SMTP_FROM_EMAIL", ""),
			FromName:  p.str("SMTP_FROM_NAME", "CholoBangladesh"),
		},
		Admin: AdminConfig{
			Email:    p.str("ADMIN_EMAIL", ""),
			Password: p.str("ADMIN_PASSWORD", ""),
			FullName: p.str("ADMIN_FULL_NAME", "Platform Admin"),
		},
		Autofill: AutofillConfig{
			YouTubeAPIKey:      p.str("YOUTUBE_API_KEY", ""),
			GoogleSearchAPIKey: p.str("GOOGLE_SEARCH_API_KEY", ""),
			GoogleSearchCX:     p.str("GOOGLE_SEARCH_CX", ""),
			AIEnabled:          p.boolean("AI_AUTOFILL_ENABLED", false),
			AIModel:            p.str("AI_AUTOFILL_MODEL", "claude-opus-5-5"),
		},
		Market: MarketConfig{
			FreeGigLimit: p.integer("FREE_GIG_LIMIT", 5),
		},
		Payment: PaymentConfig{
			AamarpayURL:  p.str("AAMARPAY_URL", ""),
			StoreID:      p.str("STORE_ID", p.str("AAMARPAY_STORE_ID", "")),
			SignatureKey: p.str("SIGNATURE_KEY", p.str("AAMARPAY_SIGNATURE_KEY", "")),
		},
	}

	cfg.validate(&errs)
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return cfg, nil
}

func (c *Config) validate(errs *[]error) {
	add := func(format string, a ...any) { *errs = append(*errs, fmt.Errorf(format, a...)) }

	if c.DB.URL == "" {
		add("DATABASE_URL is required")
	}
	if len(c.JWT.Secret) < 32 {
		add("JWT_SECRET must be at least 32 characters")
	}
	if c.App.IsProduction() && strings.Contains(c.JWT.Secret, "change-this") {
		add("JWT_SECRET still uses the example value")
	}
	if len(c.Security.OTPSecret) < 32 {
		add("OTP_HMAC_SECRET must be at least 32 characters")
	}
	if len(c.Security.DataEncryptionKey) != 32 {
		add("DATA_ENCRYPTION_KEY must be base64 of exactly 32 bytes (openssl rand -base64 32)")
	}
	if c.JWT.AccessTTL <= 0 || c.JWT.AccessTTL > 24*time.Hour {
		add("JWT_ACCESS_EXPIRES_IN must be between 1s and 24h (use refresh tokens for long sessions)")
	}
	if c.OTP.Length < 4 || c.OTP.Length > 10 {
		add("OTP_LENGTH must be between 4 and 10")
	}
	if c.Security.MaxFailedAttempts < 1 || c.OTP.MaxAttempts < 1 {
		add("USER_MAX_FAILED_ATTEMPTS and OTP_MAX_ATTEMPTS must be >= 1")
	}
	if c.Security.BcryptCost < 10 || c.Security.BcryptCost > 15 {
		add("BCRYPT_COST must be between 10 and 15")
	}
	if d := c.OTP.DevCode; d != "" && (len(d) != c.OTP.Length || strings.Trim(d, "0123456789") != "") {
		add("DEV_OTP_CODE must be exactly OTP_LENGTH digits")
	}
	if c.Autofill.AIEnabled && os.Getenv("ANTHROPIC_API_KEY") == "" {
		add("ANTHROPIC_API_KEY is required when AI_AUTOFILL_ENABLED=true")
	}
	if c.Market.FreeGigLimit < 0 || c.Market.FreeGigLimit > 100 {
		add("FREE_GIG_LIMIT must be between 0 and 100")
	}
	if c.Captcha.Enabled && c.Captcha.SecretKey == "" {
		add("TURNSTILE_SECRET_KEY is required when CAPTCHA_ENABLED=true")
	}
	if c.SMS.Enabled && (c.SMS.APIKey == "" || c.SMS.SenderID == "") {
		add("BULKSMSBD_API_KEY and BULKSMSBD_SENDER_ID are required when SMS_ENABLED=true")
	}
	if c.Mail.Enabled && (c.Mail.Username == "" || c.Mail.Password == "" || c.Mail.FromEmail == "") {
		add("SMTP_USERNAME, SMTP_PASSWORD and SMTP_FROM_EMAIL are required when EMAIL_ENABLED=true")
	}
	if u := c.Payment.AamarpayURL; u != "" && !strings.HasPrefix(u, "https://") {
		add("AAMARPAY_URL must be an https:// URL")
	}
	if c.App.IsProduction() {
		if c.Payment.Enabled() && c.Payment.Sandbox() {
			add("AAMARPAY_URL points to the sandbox in production (use https://secure.aamarpay.com/jsonpost.php)")
		}
		if !strings.HasPrefix(c.App.PublicAPIURL, "https://") || !strings.HasPrefix(c.App.FrontendURL, "https://") {
			add("PUBLIC_API_URL and FRONTEND_URL must be https:// in production")
		}
		if !c.SMS.Enabled || !c.Mail.Enabled {
			add("SMS_ENABLED and EMAIL_ENABLED must be true in production (OTPs are never logged there)")
		}
		if c.OTP.DevCode != "" || c.OTP.DevExpose {
			add("DEV_OTP_CODE and DEV_EXPOSE_OTP must be empty/false in production")
		}
		if len(c.App.CORSOrigins) == 0 {
			add("CORS_ORIGINS is required in production")
		}
	}
}

// ---------------------------------------------------------------------------

type parser struct{ errs *[]error }

func (p *parser) str(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return def
}

func (p *parser) integer(key string, def int) int {
	raw := p.str(key, "")
	if raw == "" {
		return def
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		*p.errs = append(*p.errs, fmt.Errorf("%s: %q is not an integer", key, raw))
		return def
	}
	return n
}

func (p *parser) boolean(key string, def bool) bool {
	raw := p.str(key, "")
	if raw == "" {
		return def
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		*p.errs = append(*p.errs, fmt.Errorf("%s: %q is not a boolean", key, raw))
		return def
	}
	return b
}

func (p *parser) duration(key, def string) time.Duration {
	raw := p.str(key, def)
	d, err := ParseDuration(raw)
	if err != nil {
		*p.errs = append(*p.errs, fmt.Errorf("%s: %w", key, err))
	}
	return d
}

func (p *parser) list(key string) []string {
	raw := p.str(key, "")
	if raw == "" {
		return nil
	}
	var out []string
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func (p *parser) base64Key(key string) []byte {
	raw := p.str(key, "")
	if raw == "" {
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		*p.errs = append(*p.errs, fmt.Errorf("%s: invalid base64", key))
	}
	return b
}

// ParseDuration extends time.ParseDuration with a "d" (day) suffix, e.g. "30d".
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q", s)
	}
	return d, nil
}
