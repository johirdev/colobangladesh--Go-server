// Package geo is the Bangladesh geography module: divisions (বিভাগ), districts (জেলা)
// and upazilas/thanas (উপজেলা/থানা). Public read endpoints feed dropdowns and the map;
// admins and moderators edit the data under /manage/geo.
package geo

import (
	"context"
	"log/slog"

	"cholobangladesh-server/internal/http/middleware"
	"cholobangladesh-server/internal/modules/geo/handler"
	"cholobangladesh-server/internal/modules/geo/repository"
	"cholobangladesh-server/internal/modules/geo/service"
	"cholobangladesh-server/internal/seed"
	"cholobangladesh-server/internal/shared"

	"github.com/gin-gonic/gin"
)

type Module struct {
	Service *service.Service
	h       *handler.Handler
	d       *shared.Deps
}

func New(d *shared.Deps) *Module {
	svc := service.New(repository.NewPostgres(d.DB))
	return &Module{Service: svc, h: handler.New(svc), d: d}
}

// Start loads the bundled reference data (8 divisions, 64 districts, all upazilas)
// the first time the server runs against an empty database.
func (m *Module) Start(ctx context.Context) error {
	empty, err := seed.GeoIsEmpty(ctx, m.d.DB)
	if err != nil || !empty {
		return err
	}
	c, err := seed.Geo(ctx, m.d.DB)
	if err != nil {
		return err
	}
	slog.Info("geo reference data loaded", "divisions", c.Divisions, "districts", c.Districts, "upazilas", c.Upazilas)
	return nil
}

func (m *Module) Routes(rg *gin.RouterGroup) {
	h := m.h
	g := rg.Group("/geo")
	{
		g.GET("/tree", h.Tree)
		g.GET("/divisions", h.ListDivisions)
		g.GET("/divisions/:id", h.GetDivision)
		g.GET("/districts", h.ListDistricts)
		g.GET("/districts/:id", h.GetDistrict)
		g.GET("/upazilas", h.ListUpazilas)
		g.GET("/upazilas/:id", h.GetUpazila)
	}

	mg := rg.Group("/manage/geo", middleware.NoStore(), m.d.Auth(),
		middleware.RequireRole(shared.RoleAdmin, shared.RoleModerator))
	{
		mg.POST("/divisions", h.CreateDivision)
		mg.PATCH("/divisions/:id", h.UpdateDivision)
		mg.DELETE("/divisions/:id", h.DeleteDivision)
		mg.POST("/districts", h.CreateDistrict)
		mg.PATCH("/districts/:id", h.UpdateDistrict)
		mg.DELETE("/districts/:id", h.DeleteDistrict)
		mg.POST("/upazilas", h.CreateUpazila)
		mg.PATCH("/upazilas/:id", h.UpdateUpazila)
		mg.DELETE("/upazilas/:id", h.DeleteUpazila)
	}
}
