// Package review is the review module: tourists rate and review a tour (with photos)
// after it is completed; guides reply; admins hide abusive reviews.
package review

import (
	"context"
	"time"

	"cholobangladesh-server/internal/http/middleware"
	"cholobangladesh-server/internal/modules/review/domain"
	"cholobangladesh-server/internal/modules/review/handler"
	"cholobangladesh-server/internal/modules/review/repository"
	"cholobangladesh-server/internal/modules/review/service"
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
	svc := service.New(repository.NewPostgres(d.DB), files, d.Config.Security.MaxUploadBytes)
	return &Module{h: handler.New(svc), d: d}
}

func (m *Module) Routes(rg *gin.RouterGroup) {
	h, d := m.h, m.d
	auth := d.Auth()
	uploadLimit := middleware.RateLimit(d.Limiter, "upload", d.Config.RateLimit.UploadsPerHour, time.Hour, middleware.ByUserOrIP)

	rg.GET("/reviews", h.List)
	rg.POST("/bookings/:id/review", middleware.NoStore(), auth, middleware.RequireRole(shared.RoleUser), uploadLimit, h.Create)
	rg.POST("/reviews/:id/reply", middleware.NoStore(), auth, middleware.RequireRole(shared.RoleGuide), h.Reply)

	adm := rg.Group("/admin", middleware.NoStore(), auth, middleware.RequireRole(shared.RoleAdmin))
	adm.GET("/reviews", h.AdminList)
	adm.PATCH("/reviews/:id/visibility", h.SetVisibility)
}

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
