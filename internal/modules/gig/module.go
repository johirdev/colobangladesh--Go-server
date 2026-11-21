// Package gig is the guide gig module: verified guides publish tour offers ("gigs")
// for tourist spots with their own price, duration and details. FREE_GIG_LIMIT gigs are
// free (default 5); more need extra slots (admin-granted now, paid purchase later).
package gig

import (
	"cholobangladesh-server/internal/http/middleware"
	"cholobangladesh-server/internal/modules/gig/handler"
	"cholobangladesh-server/internal/modules/gig/repository"
	"cholobangladesh-server/internal/modules/gig/service"
	"cholobangladesh-server/internal/shared"

	"github.com/gin-gonic/gin"
)

type Module struct {
	h *handler.Handler
	d *shared.Deps
}

func New(d *shared.Deps) *Module {
	svc := service.New(repository.NewPostgres(d.DB), d.Config.Market.FreeGigLimit)
	return &Module{h: handler.New(svc), d: d}
}

func (m *Module) Routes(rg *gin.RouterGroup) {
	h, auth := m.h, m.d.Auth()

	mine := rg.Group("/gigs/me", middleware.NoStore(), auth, middleware.RequireRole(shared.RoleGuide))
	{
		mine.GET("", h.MyGigs)
		mine.POST("", h.Create)
		mine.GET("/quota", h.MyQuota)
		mine.GET("/:id", h.MyGig)
		mine.PATCH("/:id", h.Update)
		mine.DELETE("/:id", h.Delete)
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
