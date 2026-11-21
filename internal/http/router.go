// Package http builds the gin engine: global security middleware, health
// probes, swagger and module routes.
package http

import (
	"context"
	"net/http"
	"time"

	"cholobangladesh-server/internal/http/middleware"
	"cholobangladesh-server/internal/shared"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Module is implemented by every feature module.
type Module interface {
	Routes(rg *gin.RouterGroup)
}

func NewRouter(d *shared.Deps, modules ...Module) (*gin.Engine, error) {
	cfg := d.Config
	if cfg.App.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.HandleMethodNotAllowed = true
	r.MaxMultipartMemory = 8 << 20
	// Only these proxies may set X-Forwarded-For; otherwise ClientIP() = socket address.
	if err := r.SetTrustedProxies(cfg.App.TrustedProxies); err != nil {
		return nil, err
	}

	r.Use(
		middleware.RequestID(),
		middleware.Logger(),
		middleware.Recovery(),
		middleware.SecureHeaders(cfg.App.IsProduction()),
		cors.New(cors.Config{
			AllowOrigins:     cfg.App.CORSOrigins,
			AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Captcha-Token", "X-Request-ID"},
			ExposeHeaders:    []string{"X-Request-ID", "Retry-After", "X-RateLimit-Limit", "X-RateLimit-Remaining"},
			AllowCredentials: true,
			MaxAge:           12 * time.Hour,
		}),
		middleware.BodyLimit(cfg.Security.MaxBodyBytes, cfg.Security.MaxUploadBytes*3+(1<<20)),
	)
	r.NoRoute(middleware.NotFound())
	r.NoMethod(middleware.MethodNotAllowed())

	// Liveness: process is up. Readiness: dependencies reachable.
	r.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/ready", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		sqlDB, err := d.DB.DB()
		if err == nil {
			err = sqlDB.PingContext(ctx)
		}
		if err == nil {
			err = d.Redis.Ping(ctx).Err()
		}
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready"})
	})

	if cfg.App.SwaggerEnabled {
		r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler,
			ginSwagger.DocExpansion("none"), ginSwagger.PersistAuthorization(true)))
	}

	// Version discovery: GET / and GET /api list all versions; GET /api/v1 shows v1 info.
	name, env, sw := cfg.App.Name, cfg.App.Env, cfg.App.SwaggerEnabled
	r.GET("/", apiIndex(name, env, "v1", sw))
	r.GET("/api", apiIndex(name, env, "v1", sw))

	globalLimit := middleware.RateLimit(d.Limiter, "global", cfg.RateLimit.GlobalPerMinute, time.Minute, middleware.ByIP)

	// ---- v1 ----
	v1 := r.Group(versions[0].BasePath, versionHeaders(versions[0]), globalLimit)
	v1.GET("", apiIndex(name, env, "v1", sw))
	for _, m := range modules {
		m.Routes(v1)
	}

	// ---- v2 (future) ----
	// v2 := r.Group("/api/v2", versionHeaders(versions[1]), globalLimit)
	// v2.GET("", apiIndex(name, env, "v2", sw))
	// for _, m := range modulesV2 { m.Routes(v2) }

	return r, nil
}
