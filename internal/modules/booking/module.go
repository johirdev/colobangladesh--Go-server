// Package booking is the booking & payment module:
//
//	tourist requests a gig → guide accepts → tourist pays online (aamarPay)
//	→ trip plan + tourist/guide chat → tourist confirms completion (or admin after end date) → review.
//
// The manual mobile-banking payment (transaction ID verified by an admin) remains as a fallback.
package booking

import (
	"context"
	"log/slog"
	"time"

	"cholobangladesh-server/internal/http/middleware"
	"cholobangladesh-server/internal/modules/booking/handler"
	"cholobangladesh-server/internal/modules/booking/repository"
	"cholobangladesh-server/internal/modules/booking/service"
	"cholobangladesh-server/internal/platform/payment"
	"cholobangladesh-server/internal/shared"

	"github.com/gin-gonic/gin"
)

type Module struct {
	svc *service.Service
	h   *handler.Handler
	d   *shared.Deps
}

func New(d *shared.Deps, gw *payment.Aamarpay) *Module {
	svc := service.New(repository.NewPostgres(d.DB), service.Options{Notify: d.Notify, Gateway: gw,
		PublicAPIURL: d.Config.App.PublicAPIURL, FrontendURL: d.Config.App.FrontendURL})
	if gw == nil {
		slog.Warn("aamarPay not configured (AAMARPAY_URL / STORE_ID / SIGNATURE_KEY): online payment disabled")
	} else {
		slog.Info("aamarPay online payment enabled", "mode", gw.Mode())
	}
	return &Module{svc: svc, h: handler.New(svc), d: d}
}

// Start runs the payment reconciler (re-checks checkouts whose callback never arrived).
func (m *Module) Start(ctx context.Context) { m.svc.StartReconciler(ctx) }

func (m *Module) Routes(rg *gin.RouterGroup) {
	h, d := m.h, m.d
	auth := d.Auth()
	tourist := middleware.RequireRole(shared.RoleUser)
	guide := middleware.RequireRole(shared.RoleGuide)
	createLimit := middleware.RateLimit(d.Limiter, "booking", 30, time.Hour, middleware.ByUserOrIP)
	payLimit := middleware.RateLimit(d.Limiter, "pay", 20, time.Hour, middleware.ByUserOrIP)
	msgLimit := middleware.RateLimit(d.Limiter, "chat", 60, time.Minute, middleware.ByUserOrIP)
	planLimit := middleware.RateLimit(d.Limiter, "plan", 60, time.Hour, middleware.ByUserOrIP)
	returnLimit := middleware.RateLimit(d.Limiter, "pay-return", 30, time.Minute, middleware.ByIP)

	rg.GET("/payment-methods", h.Methods)
	rg.GET("/gigs/:id/booked-dates", h.BookedDates)

	// aamarPay redirects the browser here (POST form; some flows use GET). Not authenticated by design:
	// the result is re-verified server-to-server.
	rg.POST("/payments/aamarpay/return", returnLimit, h.GatewayReturn)
	rg.GET("/payments/aamarpay/return", returnLimit, h.GatewayReturn)
	rg.POST("/payments/aamarpay/:tran_id/verify", middleware.NoStore(), auth, payLimit, h.CheckTransaction)
	rg.GET("/messages/unread", middleware.NoStore(), auth, h.UnreadMessages)

	b := rg.Group("/bookings", middleware.NoStore(), auth)
	{
		b.POST("", tourist, createLimit, h.Create)
		b.GET("/me", tourist, h.ListMine)
		b.GET("/guide", guide, h.ListForGuide)
		b.GET("/:id", h.Get)
		b.POST("/:id/accept", guide, h.Accept)
		b.POST("/:id/reject", guide, h.Reject)
		b.POST("/:id/complete", tourist, h.Complete)
		b.POST("/:id/cancel", h.Cancel)
		b.POST("/:id/pay", tourist, payLimit, h.PayOnline)
		b.GET("/:id/transactions", h.BookingTransactions)
		b.GET("/:id/payments", h.BookingPayments)
		b.POST("/:id/payments", tourist, createLimit, h.SubmitPayment)
		b.GET("/:id/plan", h.GetPlan)
		b.PUT("/:id/plan", planLimit, h.SavePlan)
		b.GET("/:id/messages", h.Messages)
		b.POST("/:id/messages", msgLimit, h.SendMessage)
	}

	adm := rg.Group("/admin", middleware.NoStore(), auth, middleware.RequireRole(shared.RoleAdmin))
	{
		adm.GET("/bookings", h.AdminBookings)
		adm.POST("/bookings/:id/complete", h.AdminComplete)
		adm.POST("/bookings/:id/cancel", h.AdminCancel)
		adm.GET("/transactions", h.AdminTransactions)
		adm.GET("/transactions/:tran_id", h.AdminTransaction)
		adm.GET("/trip-plans", h.AdminPlans)
		adm.GET("/payments", h.AdminPayments)
		adm.POST("/payments/:id/verify", h.VerifyPayment)
		adm.POST("/payments/:id/reject", h.RejectPayment)
		adm.GET("/payment-methods", h.AdminMethods)
		adm.POST("/payment-methods", h.CreateMethod)
		adm.PATCH("/payment-methods/:id", h.UpdateMethod)
		adm.DELETE("/payment-methods/:id", h.DeleteMethod)
	}
}
