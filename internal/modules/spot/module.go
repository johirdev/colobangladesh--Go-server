// Package spot is the tourist spot module. Admins and moderators curate spots
// (title, subtitle, location, 1-50 images, YouTube videos, description, Google Maps link,
// 1-5 upazilas/thanas) under /manage/tourist-spots; tourists browse published spots and
// the Bangladesh map under /tourist-spots. Guides attach their gigs to these spots.
package spot

import (
	"context"
	"time"

	"cholobangladesh-server/internal/http/middleware"
	"cholobangladesh-server/internal/modules/spot/domain"
	"cholobangladesh-server/internal/modules/spot/handler"
	"cholobangladesh-server/internal/modules/spot/repository"
	"cholobangladesh-server/internal/modules/spot/service"
	"cholobangladesh-server/internal/platform/autofill"
	"cholobangladesh-server/internal/platform/storage"
	"cholobangladesh-server/internal/shared"

	"github.com/gin-gonic/gin"
)

type Module struct {
	Service *service.Service
	h       *handler.Handler
	d       *shared.Deps
}

func New(d *shared.Deps) *Module {
	cfg := d.Config
	var files domain.FileStorage
	if d.Storage != nil {
		files = storageAdapter{d.Storage}
	}
	af := autofill.New(autofill.Config{
		YouTubeAPIKey:      cfg.Autofill.YouTubeAPIKey,
		GoogleSearchAPIKey: cfg.Autofill.GoogleSearchAPIKey,
		GoogleSearchCX:     cfg.Autofill.GoogleSearchCX,
		AIEnabled:          cfg.Autofill.AIEnabled,
		AIModel:            cfg.Autofill.AIModel,
	})
	svc := service.New(repository.NewPostgres(d.DB), files, af, cfg.Security.MaxUploadBytes)
	return &Module{Service: svc, h: handler.New(svc), d: d}
}

func (m *Module) Routes(rg *gin.RouterGroup) {
	h, d := m.h, m.d
	uploadLimit := middleware.RateLimit(d.Limiter, "upload", d.Config.RateLimit.UploadsPerHour*5, time.Hour, middleware.ByUserOrIP)
	autofillLimit := middleware.RateLimit(d.Limiter, "autofill", 30, time.Hour, middleware.ByUserOrIP)

	p := rg.Group("/tourist-spots")
	{
		p.GET("", h.List)
		p.GET("/options", h.Options)
		p.GET("/map", h.Map)
		p.GET("/:id", h.Get)
	}

	mg := rg.Group("/manage/tourist-spots", middleware.NoStore(), d.Auth(),
		middleware.RequireRole(shared.RoleAdmin, shared.RoleModerator))
	{
		mg.GET("", h.ManageList)
		mg.POST("", h.Create)
		mg.POST("/autofill", autofillLimit, h.Autofill)
		mg.GET("/:id", h.ManageGet)
		mg.PATCH("/:id", h.Update)
		mg.DELETE("/:id", h.Delete)
		mg.POST("/:id/images", uploadLimit, h.UploadImages)
		mg.DELETE("/:id/images/:imageId", h.DeleteImage)
	}
}

// Start has nothing to do yet (default spots are loaded by `manage seed`).
func (m *Module) Start(context.Context) error { return nil }

type storageAdapter struct{ c *storage.Cloudinary }

func (s storageAdapter) UploadPublic(ctx context.Context, data []byte, path, id string) (string, string, error) {
	u, err := s.c.UploadPublic(ctx, data, path, id)
	if err != nil {
		return "", "", err
	}
	return u.URL, u.PublicID, nil
}

func (s storageAdapter) Delete(ctx context.Context, id string) error {
	return s.c.Delete(ctx, id, false)
}
