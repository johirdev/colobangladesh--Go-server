// Command server runs the CholoBangladesh API.
//
//	@title		CholoBangladesh API
//	@version	1.0
//	@description.markdown
//	@contact.name				CholoBangladesh Backend
//	@BasePath					/api/v1
//	@schemes					http https
//
//	@securityDefinitions.apikey	BearerAuth
//	@in							header
//	@name						Authorization
//	@description				Type: **Bearer &lt;access_token&gt;** (from login / register / refresh)
//
//	@tag.name					0. Meta
//	@tag.description			API version & build info.
//	@tag.name					1. Auth · Tourist sign-up
//	@tag.description			Foreign/local tourists: email OTP (3 steps) or Google/Apple. No SMS cost.
//	@tag.name					2. Auth · Guide sign-up
//	@tag.description			Guides: Bangladeshi mobile OTP (3 steps), then profile + NID/selfie KYC.
//	@tag.name					3. Auth · Login & session
//	@tag.description			Password login with lockout, passwordless OTP login, Google/Apple, refresh rotation, logout.
//	@tag.name					4. Auth · Password reset
//	@tag.description			OTP → reset_token → new password (signs out every device).
//	@tag.name					5. Me · Profile & account
//	@tag.description			Logged-in user's profile, avatar, password, contacts, devices, deletion.
//	@tag.name					6. Guide · My guide profile & KYC
//	@tag.description			Guide-only: marketplace profile, availability, identity verification.
//	@tag.name					7. Guides · Public
//	@tag.description			Tourist-facing guide discovery (verified guides only).
//	@tag.name					8. Admin
//	@tag.description			User moderation and guide KYC review (role = admin).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // timezone data embedded for distroless images

	_ "cholobangladesh-server/docs" // generated swagger spec (swag init)
	"cholobangladesh-server/internal/config"
	apphttp "cholobangladesh-server/internal/http"
	"cholobangladesh-server/internal/modules/user"
	"cholobangladesh-server/internal/platform/cache"
	"cholobangladesh-server/internal/platform/captcha"
	"cholobangladesh-server/internal/platform/database"
	"cholobangladesh-server/internal/platform/mailer"
	"cholobangladesh-server/internal/platform/oauth"
	"cholobangladesh-server/internal/platform/sms"
	"cholobangladesh-server/internal/platform/storage"
	"cholobangladesh-server/internal/shared"
	"cholobangladesh-server/pkg/buildinfo"
	"cholobangladesh-server/pkg/ratelimit"
	"cholobangladesh-server/pkg/token"
	"cholobangladesh-server/pkg/validation"
)

func main() {
	// `server healthcheck` is used by the Docker HEALTHCHECK (distroless has no curl).
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck())
	}
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("config:\n%w", err)
	}
	setupLogger(cfg.App.IsProduction())
	validation.Register()

	db, err := database.Connect(cfg.DB, cfg.App.IsProduction())
	if err != nil {
		return err
	}
	if sqlDB, err := db.DB(); err == nil {
		defer sqlDB.Close()
	}
	if cfg.App.MigrateOnStart {
		if err := database.Migrate(db); err != nil {
			return err
		}
	}

	rdb, err := cache.Connect(cfg.Redis)
	if err != nil {
		return err
	}
	defer rdb.Close()

	var cld *storage.Cloudinary
	if cfg.Cloudinary.URL != "" {
		if cld, err = storage.NewCloudinary(cfg.Cloudinary.URL, cfg.Cloudinary.Folder); err != nil {
			return fmt.Errorf("cloudinary: %w", err)
		}
	} else {
		slog.Warn("CLOUDINARY_URL not set: avatar and KYC uploads are disabled")
	}

	deps := &shared.Deps{
		Config:  cfg,
		DB:      db,
		Redis:   rdb,
		Tokens:  token.NewManager(cfg.JWT.Secret, cfg.JWT.Issuer, cfg.JWT.AccessTTL),
		Limiter: ratelimit.New(rdb),
		Captcha: captcha.NewTurnstile(cfg.Captcha.Enabled, cfg.Captcha.SecretKey),
		Mailer:  mailer.New(cfg.Mail),
		SMS:     sms.New(cfg.SMS),
		Storage: cld,
		OAuth:   oauth.NewVerifier(cfg.OAuth.GoogleClientIDs, cfg.OAuth.AppleClientIDs),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	userModule, err := user.New(deps)
	if err != nil {
		return err
	}
	if err := userModule.Start(ctx); err != nil {
		return err
	}

	handler, err := apphttp.NewRouter(deps, userModule)
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              ":" + cfg.App.Port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("server running", "version", buildinfo.Version, "commit", buildinfo.Commit, "port", cfg.App.Port, "env", cfg.App.Env, "swagger", cfg.App.SwaggerEnabled)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	slog.Info("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("forced shutdown: %w", err)
	}
	slog.Info("server stopped")
	return nil
}

func setupLogger(production bool) {
	var h slog.Handler
	if production {
		h = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		h = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	}
	slog.SetDefault(slog.New(h))
}

func healthcheck() int {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	c := &http.Client{Timeout: 3 * time.Second}
	res, err := c.Get("http://127.0.0.1:" + port + "/health")
	if err != nil || res.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
