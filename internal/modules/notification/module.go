// Package notification is the in-app inbox + realtime push module.
// It implements shared.Notifier for every other module and serves:
//
//	POST /realtime/ticket                 one-time Socket.IO connection ticket
//	GET  /notifications                   my inbox (paged, ?unread=true, ?q=, ?sort=newest|oldest)
//	GET  /notifications/unread-count      badge counter
//	POST /notifications/:id/read          mark one read
//	POST /notifications/read-all          mark all read
//
// Layout (same as every module): domain/ (entity + ports) · dto/ (API shapes) · repository/ (SQL)
// · service/ (use cases) · handler/ (HTTP + swagger) · module.go (wiring + routes).
package notification

import (
	"time"

	"cholobangladesh-server/internal/http/middleware"
	"cholobangladesh-server/internal/modules/notification/handler"
	"cholobangladesh-server/internal/modules/notification/repository"
	"cholobangladesh-server/internal/modules/notification/service"
	"cholobangladesh-server/internal/platform/realtime"
	"cholobangladesh-server/internal/shared"

	"github.com/gin-gonic/gin"
)

type Module struct {
	Service *service.Service
	h       *handler.Handler
	d       *shared.Deps
}

func New(d *shared.Deps, pub *realtime.Publisher) *Module {
	svc := service.New(repository.NewPostgres(d.DB), pub)
	return &Module{Service: svc, h: handler.New(svc, d.Redis), d: d}
}

func (m *Module) Routes(rg *gin.RouterGroup) {
	h, auth := m.h, m.d.Auth()
	ticketLimit := middleware.RateLimit(m.d.Limiter, "rt-ticket", 30, time.Minute, middleware.ByUserOrIP)

	rg.POST("/realtime/ticket", middleware.NoStore(), auth, ticketLimit, h.Ticket)

	n := rg.Group("/notifications", middleware.NoStore(), auth)
	{
		n.GET("", h.List)
		n.GET("/unread-count", h.Unread)
		n.POST("/read-all", h.ReadAll)
		n.POST("/:id/read", h.Read)
	}
}
