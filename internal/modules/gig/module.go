// Package gig is the guide gig module: verified guides publish tour offers ("gigs")
// for tourist spots with their own price, duration and details. FREE_GIG_LIMIT gigs are
// free (default 5); more need extra slots (admin-granted now, paid purchase later).
package gig

import (
	"context"
	"time"

	"cholobangladesh-server/internal/http/middleware"
	"cholobangladesh-server/internal/modules/gig/domain"
	"cholobangladesh-server/internal/modules/gig/handler"
	"cholobangladesh-server/internal/modules/gig/repository"
	"cholobangladesh-server/internal/modules/gig/service"
	"cholobangladesh-server/internal/platform/storage"
	"cholobangladesh-server/internal/shared"

	"github.com/gin-gonic/gin"
)

type Module struct {
	h *handler.Handler
	d *shared.Deps
}

func New(d *shared.Deps) *Module {
	var files domain.FileStorage
	if d.Storage != nil {
		files = storageAdapter{d.Storage}
	}
	svc := service.New(repository.NewPostgres(d.DB), files, d.Config.Security.MaxUploadBytes, d.Config.Market.FreeGigLimit)
	return &Module{h: handler.New(svc), d: d}
}

// storageAdapter fits Cloudinary to the gig FileStorage port.
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

func (m *Module) Routes(rg *gin.RouterGroup) {
	h, auth := m.h, m.d.Auth()
	uploadLimit := middleware.RateLimit(m.d.Limiter, "upload", m.d.Config.RateLimit.UploadsPerHour*5, time.Hour, middleware.ByUserOrIP)

	mine := rg.Group("/gigs/me", middleware.NoStore(), auth, middleware.RequireRole(shared.RoleGuide))
	{
		mine.GET("", h.MyGigs)
		mine.POST("", h.Create)
		mine.GET("/quota", h.MyQuota)
		mine.GET("/:id", h.MyGig)
		mine.PATCH("/:id", h.Update)
		mine.DELETE("/:id", h.Delete)
		// gallery (photos are cropped in the browser; the first one is the cover)
		mine.POST("/:id/images", uploadLimit, h.UploadImages)
		mine.PUT("/:id/images/:imageId", uploadLimit, h.ReplaceImage)
		mine.PATCH("/:id/images/:imageId", h.UpdateImage)
		mine.DELETE("/:id/images/:imageId", h.DeleteImage)
	}

	rg.GET("/gigs", h.List)
	rg.GET("/gigs/:id", h.Get)

	adm := rg.Group("/admin", middleware.NoStore(), auth, middleware.RequireRole(shared.RoleAdmin))
	{
		adm.GET("/gigs", h.AdminList)
		adm.PATCH("/gigs/:id/status", h.AdminSetStatus)
		adm.PUT("/guides/:id/gig-quota", h.AdminSetQuota)
	}
}
