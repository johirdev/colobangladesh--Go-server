// Package booking is the booking & payment module: tourists book gigs, guides
// accept/complete them, and after the tour the tourist pays through the default
// option — a mobile-banking number (bKash/Nagad/Rocket) + transaction ID that an
// admin verifies. Online gateways can be added later as new payment providers.
package booking

import (
	"time"

	"cholobangladesh-server/internal/http/middleware"
	"cholobangladesh-server/internal/modules/booking/handler"
	"cholobangladesh-server/internal/modules/booking/repository"
	"cholobangladesh-server/internal/modules/booking/service"
	"cholobangladesh-server/internal/shared"

	"github.com/gin-gonic/gin"
)

type Module struct {
	h *handler.Handler
	d *shared.Deps
}

func New(d *shared.Deps) *Module {
	return &Module{h: handler.New(service.New(repository.NewPostgres(d.DB))), d: d}
}

func (m *Module) Routes(rg *gin.RouterGroup) {
	h, d := m.h, m.d
	auth := d.Auth()
	tourist := middleware.RequireRole(shared.RoleUser)
	guide := middleware.RequireRole(shared.RoleGuide)
	createLimit := middleware.RateLimit(d.Limiter, "booking", 30, time.Hour, middleware.ByUserOrIP)

	rg.GET("/payment-methods", h.Methods)

	b := rg.Group("/bookings", middleware.NoStore(), auth)
	{
		b.POST("", tourist, createLimit, h.Create)
		b.GET("/me", tourist, h.ListMine)
		b.GET("/guide", guide, h.ListForGuide)
		b.GET("/:id", h.Get)
		b.POST("/:id/accept", guide, h.Accept)
		b.POST("/:id/reject", guide, h.Reject)
		b.POST("/:id/complete", guide, h.Complete)
		b.POST("/:id/cancel", h.Cancel)
		b.GET("/:id/payments", h.BookingPayments)
		b.POST("/:id/payments", tourist, createLimit, h.SubmitPayment)
	}

	adm := rg.Group("/admin", middleware.NoStore(), auth, middleware.RequireRole(shared.RoleAdmin))
	{
		adm.GET("/bookings", h.AdminBookings)
		adm.GET("/payments", h.AdminPayments)
		adm.POST("/payments/:id/verify", h.VerifyPayment)
		adm.POST("/payments/:id/reject", h.RejectPayment)
		adm.GET("/payment-methods", h.AdminMethods)
		adm.POST("/payment-methods", h.CreateMethod)
		adm.PATCH("/payment-methods/:id", h.UpdateMethod)
		adm.DELETE("/payment-methods/:id", h.DeleteMethod)
	}
}
