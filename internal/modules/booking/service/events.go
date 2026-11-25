package service

import (
	"context"
	"fmt"
	"time"

	"cholobangladesh-server/internal/modules/booking/domain"
	"cholobangladesh-server/internal/modules/booking/dto"
	"cholobangladesh-server/internal/platform/realtime"
	"cholobangladesh-server/internal/shared"

	"github.com/google/uuid"
)

// Booking events (besides the status names) that trigger notifications.
const (
	evRequested       = "requested"
	evPaid            = "paid"
	evPaymentFailed   = "payment_failed"
	evPaymentRejected = "payment_rejected"
	evPlanUpdated     = "plan_updated"
)

func touristLink(id string) string { return "/account/bookings/" + id }
func guideLink(id string) string   { return "/guide/bookings/" + id }
func adminLink(id string) string   { return "/admin/bookings/" + id }

func ids(b *dto.BookingResponse) (tourist, guide uuid.UUID) {
	if b.Tourist != nil {
		tourist, _ = uuid.Parse(b.Tourist.ID)
	}
	if b.Guide != nil {
		guide, _ = uuid.Parse(b.Guide.ID)
	}
	return
}

func name(p *dto.PersonRef) string {
	if p == nil || p.FullName == "" {
		return "Someone"
	}
	return p.FullName
}

func gigTitle(b *dto.BookingResponse) string {
	if b.Gig != nil {
		return b.Gig.Title
	}
	return "your tour"
}

func money(amount int, cur string) string {
	if cur == "BDT" {
		return fmt.Sprintf("৳%d", amount)
	}
	return fmt.Sprintf("%s %d", cur, amount)
}

// onBookingEvent pushes live updates to both parties + admins and stores inbox items.
func (s *Service) onBookingEvent(ctx context.Context, b *dto.BookingResponse, kind string, actor uuid.UUID) {
	tourist, guide := ids(b)
	data := map[string]any{"booking_id": b.ID, "code": b.Code, "status": b.Status, "payment_status": b.PaymentStatus,
		"kind": kind}

	s.notify.Push(ctx, []string{realtime.UserRoom(tourist), realtime.UserRoom(guide), realtime.RoleRoom(shared.RoleAdmin)},
		realtime.EvBookingUpdated, data)
	s.notify.Push(ctx, []string{realtime.RoleRoom(shared.RoleAdmin)}, realtime.EvAdminActivity, map[string]any{
		"kind": kind, "booking_id": b.ID, "code": b.Code, "amount": b.TotalAmount, "currency": b.Currency,
		"at": time.Now().UTC()})

	toTourist := func(typ, title, body string) {
		s.notify.Notify(ctx, []uuid.UUID{tourist}, shared.Notification{Type: typ, Title: title, Body: body,
			Link: touristLink(b.ID), Data: data})
	}
	toGuide := func(typ, title, body string) {
		s.notify.Notify(ctx, []uuid.UUID{guide}, shared.Notification{Type: typ, Title: title, Body: body,
			Link: guideLink(b.ID), Data: data})
	}
	toAdmins := func(typ, title, body string) {
		s.notify.NotifyRole(ctx, shared.RoleAdmin, shared.Notification{Type: typ, Title: title, Body: body,
			Link: adminLink(b.ID), Data: data})
	}
	when := b.TourDate
	if b.EndDate != b.TourDate {
		when += " → " + b.EndDate
	}

	switch kind {
	case evRequested:
		toGuide("booking.requested", "New booking request",
			fmt.Sprintf("%s wants to book “%s” on %s (%d people).", name(b.Tourist), gigTitle(b), when, b.GroupSize))
	case domain.StatusAccepted:
		toTourist("booking.accepted", "Booking accepted — pay to confirm",
			fmt.Sprintf("%s accepted “%s” on %s. Pay %s to confirm your tour.", name(b.Guide), gigTitle(b), when,
				money(b.TotalAmount, b.Currency)))
	case domain.StatusRejected:
		toTourist("booking.rejected", "Booking declined",
			fmt.Sprintf("%s could not take “%s” on %s.", name(b.Guide), gigTitle(b), when))
	case domain.StatusCancelled:
		body := fmt.Sprintf("Booking %s (“%s”, %s) was cancelled.", b.Code, gigTitle(b), when)
		if actor != tourist {
			toTourist("booking.cancelled", "Booking cancelled", body)
		}
		if actor != guide {
			toGuide("booking.cancelled", "Booking cancelled", body)
		}
		if b.PaymentStatus == domain.PayPaid || b.PaymentStatus == domain.PayRefunded {
			toAdmins("booking.cancelled", "Paid booking cancelled", body)
		}
	case domain.StatusCompleted:
		if b.CompletedBy == "admin" {
			toTourist("booking.completed", "Tour marked as completed",
				fmt.Sprintf("Our team confirmed “%s” is complete. Tell others how it went!", gigTitle(b)))
			toGuide("booking.completed", "Tour marked as completed",
				fmt.Sprintf("Booking %s was confirmed complete by an admin.", b.Code))
		} else {
			toGuide("booking.completed", "Tourist confirmed the tour",
				fmt.Sprintf("%s confirmed “%s” is complete.", name(b.Tourist), gigTitle(b)))
		}
	case evPaid:
		toTourist("payment.success", "Payment successful",
			fmt.Sprintf("%s received for “%s”. Your guide will share the trip plan.", money(b.TotalAmount, b.Currency), gigTitle(b)))
		toGuide("booking.paid", "Booking paid — share the trip plan",
			fmt.Sprintf("%s paid for “%s” on %s. Add pickup, hotel and itinerary.", name(b.Tourist), gigTitle(b), when))
		toAdmins("payment.success", "Payment received",
			fmt.Sprintf("%s · booking %s · %s", money(b.TotalAmount, b.Currency), b.Code, gigTitle(b)))
	case evPaymentFailed:
		toTourist("payment.failed", "Payment not completed",
			fmt.Sprintf("Your payment for booking %s did not go through. You can try again.", b.Code))
	case evPaymentRejected:
		toTourist("payment.rejected", "Payment could not be verified",
			fmt.Sprintf("We could not verify your transaction for booking %s. Please pay again.", b.Code))
	}
}
