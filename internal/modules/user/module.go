// Package user wires the user module (identity, profiles, guide KYC) and
// registers its routes. Layers: handler -> service -> domain <- repository.
package user

import (
	"context"
	"errors"
	"time"

	"cholobangladesh-server/internal/http/middleware"
	"cholobangladesh-server/internal/modules/user/domain"
	"cholobangladesh-server/internal/modules/user/handler"
	"cholobangladesh-server/internal/modules/user/repository"
	"cholobangladesh-server/internal/modules/user/service"
	"cholobangladesh-server/internal/platform/oauth"
	"cholobangladesh-server/internal/platform/storage"
	"cholobangladesh-server/internal/shared"
	"cholobangladesh-server/pkg/password"
	"cholobangladesh-server/pkg/secure"

	"github.com/gin-gonic/gin"
)

type Module struct {
	Service *service.Service
	h       *handler.Handler
	d       *shared.Deps
}

func New(d *shared.Deps) (*Module, error) {
	cfg := d.Config
	cipher, err := secure.NewCipher(cfg.Security.DataEncryptionKey)
	if err != nil {
		return nil, err
	}

	var files domain.FileStorage
	if d.Storage != nil {
		files = storageAdapter{d.Storage}
	}

	svc := service.New(service.Config{
		AppName:           cfg.App.Name,
		Production:        cfg.App.IsProduction(),
		RefreshTTL:        cfg.JWT.RefreshTTL,
		BlockTime:         cfg.Security.BlockTime,
		FailWindow:        cfg.Security.FailedAttemptWindow,
		MaxFailedAttempts: cfg.Security.MaxFailedAttempts,
		PasswordResetTTL:  cfg.Security.PasswordResetTTL,
		RegistrationTTL:   cfg.Security.RegistrationTTL,
		OTP:               cfg.OTP,
		OTPSecret:         cfg.Security.OTPSecret,
		MaxUploadBytes:    cfg.Security.MaxUploadBytes,
		SMSEnabled:        cfg.SMS.Enabled,
		MailEnabled:       cfg.Mail.Enabled,
	}, service.Deps{
		Repo:     repository.NewPostgres(d.DB),
		Store:    repository.NewRedisStore(d.Redis),
		Tokens:   d.Tokens,
		Hasher:   password.NewHasher(cfg.Security.BcryptCost),
		Cipher:   cipher,
		Mailer:   d.Mailer,
		SMS:      d.SMS,
		Storage:  files,
		Identity: oauthAdapter{d.OAuth},
	})
	return &Module{Service: svc, h: handler.New(svc), d: d}, nil
}

// Start runs bootstrap tasks (admin seed) and background maintenance.
func (m *Module) Start(ctx context.Context) error {
	if err := m.Service.EnsureAdmin(ctx, m.d.Config.Admin); err != nil {
		return err
	}
	m.Service.StartMaintenance(ctx)
	return nil
}

// Routes registers every endpoint of the module under rg (/api/v1).
//
// Security layers per route group:
//
//	global      : secure headers, body limit, per-IP rate limit (router.go)
//	/auth/*     : no-store, per-IP auth rate limit
//	OTP senders : + captcha (Turnstile) + per-IP OTP-send limit + per-target cooldown/hourly cap (service)
//	login       : + captcha + per-identifier lockout (USER_BLOCK_TIME) + per-IP failure block (service)
//	private     : JWT + session/tokens-version check (instant revocation)
//	uploads     : + per-user upload rate limit
//	admin       : + role check
func (m *Module) Routes(rg *gin.RouterGroup) {
	h, d, rl := m.h, m.d, m.d.Config.RateLimit
	auth := middleware.Auth(d.Tokens, m.Service)
	captcha := middleware.Captcha(d.Captcha)
	authLimit := middleware.RateLimit(d.Limiter, "auth", rl.AuthPerMinute, time.Minute, middleware.ByIP)
	otpLimit := middleware.RateLimit(d.Limiter, "otp-send", rl.OTPSendPer10Min, 10*time.Minute, middleware.ByIP)
	uploadLimit := middleware.RateLimit(d.Limiter, "upload", rl.UploadsPerHour, time.Hour, middleware.ByUserOrIP)
	sensitive := middleware.RateLimit(d.Limiter, "sensitive", 10, 10*time.Minute, middleware.ByUserOrIP)

	a := rg.Group("/auth", middleware.NoStore(), authLimit)
	{
		a.POST("/register/user/send-otp", captcha, otpLimit, h.SendUserRegistrationOTP)
		a.POST("/register/user/verify-otp", h.VerifyUserRegistrationOTP)
		a.POST("/register/user/complete", h.CompleteUserRegistration)

		a.POST("/register/guide/send-otp", captcha, otpLimit, h.SendGuideRegistrationOTP)
		a.POST("/register/guide/verify-otp", h.VerifyGuideRegistrationOTP)
		a.POST("/register/guide/complete", h.CompleteGuideRegistration)

		a.POST("/login", captcha, h.Login)
		a.POST("/login/otp/send", captcha, otpLimit, h.SendLoginOTP)
		a.POST("/login/otp/verify", h.VerifyLoginOTP)
		a.POST("/oauth/google", h.GoogleLogin)
		a.POST("/oauth/apple", h.AppleLogin)
		a.POST("/token/refresh", h.Refresh)

		a.POST("/password/forgot", captcha, otpLimit, h.ForgotPassword)
		a.POST("/password/verify-otp", h.VerifyResetOTP)
		a.POST("/password/reset", h.ResetPassword)

		a.POST("/logout", auth, h.Logout)
		a.POST("/logout-all", auth, h.LogoutAll)
	}

	me := rg.Group("/users/me", middleware.NoStore(), auth)
	{
		me.GET("", h.GetMe)
		me.DELETE("", sensitive, h.DeleteAccount)
		me.PATCH("/profile", h.UpdateProfile)
		me.PUT("/avatar", uploadLimit, h.UploadAvatar)
		me.DELETE("/avatar", h.DeleteAvatar)
		me.POST("/password", sensitive, h.ChangePassword)
		me.POST("/contacts/email/send-otp", otpLimit, h.SendEmailContactOTP)
		me.POST("/contacts/email/verify", h.VerifyEmailContact)
		me.POST("/contacts/phone/send-otp", otpLimit, h.SendPhoneContactOTP)
		me.POST("/contacts/phone/verify", h.VerifyPhoneContact)
		me.GET("/sessions", h.ListSessions)
		me.DELETE("/sessions/:id", h.RevokeSession)
	}

	g := rg.Group("/guides")
	{
		g.GET("", h.SearchGuides)
		g.GET("/options", h.GuideOptions)
		g.GET("/:id", h.GetGuide)

		mine := g.Group("/me", middleware.NoStore(), auth, middleware.RequireRole(string(domain.RoleGuide)))
		mine.PUT("/profile", h.UpsertGuideProfile)
		mine.PATCH("/availability", h.SetAvailability)
		mine.POST("/verification", uploadLimit, h.SubmitVerification)
		mine.GET("/verification", h.GetMyVerification)
	}

	adm := rg.Group("/admin", middleware.NoStore(), auth, middleware.RequireRole(string(domain.RoleAdmin)))
	{
		adm.GET("/users", h.AdminListUsers)
		adm.GET("/users/:id", h.AdminGetUser)
		adm.PATCH("/users/:id/status", h.AdminSetUserStatus)
		adm.PATCH("/users/:id/role", h.AdminSetUserRole)
		adm.GET("/guide-verifications", h.AdminListVerifications)
		adm.GET("/guide-verifications/:id", h.AdminGetVerification)
		adm.POST("/guide-verifications/:id/approve", h.AdminApproveVerification)
		adm.POST("/guide-verifications/:id/reject", h.AdminRejectVerification)
	}
}

// ------------------------------------------------------------------ adapters

type oauthAdapter struct{ v *oauth.Verifier }

func (a oauthAdapter) Verify(ctx context.Context, provider, raw, nonce string) (*domain.ExternalIdentity, error) {
	id, err := a.v.Verify(ctx, provider, raw, nonce)
	if errors.Is(err, oauth.ErrProviderDisabled) {
		return nil, domain.ErrIdentityProviderDisabled
	}
	if err != nil {
		return nil, errors.Join(domain.ErrIdentityTokenInvalid, err)
	}
	return &domain.ExternalIdentity{Provider: id.Provider, Subject: id.Subject, Email: id.Email,
		EmailVerified: id.EmailVerified, Name: id.Name, Picture: id.Picture}, nil
}

type storageAdapter struct{ c *storage.Cloudinary }

func (s storageAdapter) UploadPublic(ctx context.Context, data []byte, path, id string) (*domain.UploadedFile, error) {
	u, err := s.c.UploadPublic(ctx, data, path, id)
	if err != nil {
		return nil, err
	}
	return &domain.UploadedFile{URL: u.URL, PublicID: u.PublicID}, nil
}

func (s storageAdapter) UploadPrivate(ctx context.Context, data []byte, path, id string) (*domain.UploadedFile, error) {
	u, err := s.c.UploadPrivate(ctx, data, path, id)
	if err != nil {
		return nil, err
	}
	return &domain.UploadedFile{URL: u.URL, PublicID: u.PublicID}, nil
}

func (s storageAdapter) Delete(ctx context.Context, id string, private bool) error {
	return s.c.Delete(ctx, id, private)
}

func (s storageAdapter) SignedURL(id string, ttl time.Duration) (string, error) {
	return s.c.SignedURL(id, ttl)
}
