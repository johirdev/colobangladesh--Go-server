package service

import (
	"strings"

	"cholobangladesh-server/internal/modules/booking/domain"
	"cholobangladesh-server/internal/modules/booking/dto"
	"cholobangladesh-server/internal/shared"

	"github.com/google/uuid"
)

func trim(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return nil
	}
	return &v
}

func person(p *domain.PersonRef, id uuid.UUID) *dto.PersonRef {
	r := &dto.PersonRef{ID: id.String()}
	if p != nil {
		r.FullName, r.AvatarURL, r.CountryCode = p.FullName, p.AvatarURL, p.CountryCode
	}
	return r
}

func toBooking(b *domain.Booking, viewer uuid.UUID, role string) dto.BookingResponse {
	r := dto.BookingResponse{ID: b.ID.String(), Code: b.Code, Status: b.Status, PaymentStatus: b.PaymentStatus,
		TourDate: b.TourDate.Format(dateFmt), EndDate: b.EndDate.Format(dateFmt),
		Days: int(b.EndDate.Sub(b.TourDate).Hours()/24) + 1, StartTime: b.StartTime, GroupSize: b.GroupSize,
		PricingType: b.PricingType, UnitPrice: b.UnitPrice, TotalAmount: b.TotalAmount, Currency: b.Currency, Note: b.Note,
		StatusReason: b.StatusReason, HasReview: b.HasReview, HasPlan: b.HasPlan, CompletionNote: b.CompletionNote,
		AcceptedAt: b.AcceptedAt, PaidAt: b.PaidAt, CompletedAt: b.CompletedAt, CancelledAt: b.CancelledAt,
		CreatedAt: b.CreatedAt, Guide: person(b.Guide, b.GuideID), Tourist: person(b.User, b.UserID)}
	if b.EndDate.IsZero() {
		r.EndDate, r.Days = r.TourDate, 1
	}
	if b.CompletedBy != nil {
		r.CompletedBy = "admin"
		if *b.CompletedBy == b.UserID {
			r.CompletedBy = "tourist"
		}
	}
	if g := b.Gig; g != nil {
		r.Gig = &dto.BookingGig{ID: g.ID.String(), Title: g.Title, DurationHours: g.DurationHours, MeetingPoint: g.MeetingPoint}
	}
	if sp := b.Spot; sp != nil {
		r.Spot = &dto.BookingSpot{ID: sp.ID.String(), Slug: sp.Slug, Title: sp.Title, CoverImageURL: sp.CoverImageURL,
			LocationName: sp.LocationName}
	}

	isTourist, isGuide, isAdmin := b.UserID == viewer, b.GuideID == viewer, role == shared.RoleAdmin
	isParty := isTourist || isGuide
	contactsOpen := b.Status == domain.StatusAccepted || b.Status == domain.StatusCompleted
	if contactsOpen || isAdmin {
		if isTourist || isAdmin {
			r.GuideContact = contact(b.GuideLogin, b.Guide, nil)
		}
		if isGuide || isAdmin {
			r.TouristContact = contact(b.UserLogin, b.User, b.ContactPhone)
		}
	}
	pendingOrUnpaid := b.Status == domain.StatusPending || (b.Status == domain.StatusAccepted && b.PaymentStatus == domain.PayUnpaid)
	paidOpen := b.Status == domain.StatusAccepted && b.IsPaid()

	r.CanAccept = isGuide && b.Status == domain.StatusPending
	r.CanReject = r.CanAccept
	r.CanPay = isTourist && b.Status == domain.StatusAccepted && b.PaymentStatus == domain.PayUnpaid
	r.CanReview = isTourist && b.Status == domain.StatusCompleted && !b.HasReview
	r.CanChat = isParty && b.Live()
	r.CanEditPlan = isParty && paidOpen
	switch {
	case isAdmin:
		r.CanCancel = b.Status == domain.StatusPending || b.Status == domain.StatusAccepted
		r.CanComplete = paidOpen && today().After(b.EndDate)
	default:
		r.CanCancel = isParty && pendingOrUnpaid
		r.CanComplete = isTourist && paidOpen && !b.TourDate.After(today())
	}
	return r
}

func contact(l *domain.LoginRef, p *domain.PersonRef, override *string) *dto.Contact {
	c := &dto.Contact{}
	if l != nil {
		c.Phone, c.Email = l.Phone, l.Email
	}
	if override != nil {
		c.Phone = override
	}
	if p != nil {
		c.WhatsApp = p.WhatsappNumber
	}
	return c
}

func toMethod(m *domain.PaymentMethod) dto.PaymentMethodResponse {
	return dto.PaymentMethodResponse{ID: m.ID.String(), Provider: m.Provider, AccountName: m.AccountName,
		AccountNumber: m.AccountNumber, AccountType: m.AccountType, Instructions: m.Instructions, IsActive: m.IsActive,
		Position: m.Position}
}

func toPayment(p *domain.Payment) dto.PaymentResponse {
	r := dto.PaymentResponse{ID: p.ID.String(), Purpose: p.Purpose, Provider: p.Provider, SenderNumber: p.SenderNumber,
		TransactionID: p.TransactionID, Amount: p.Amount, Currency: p.Currency, Status: p.Status, AdminNote: p.AdminNote,
		ReviewedAt: p.ReviewedAt, CreatedAt: p.CreatedAt, Payer: person(p.Payer, p.PayerID)}
	if p.BookingID != nil {
		id := p.BookingID.String()
		r.BookingID = &id
	}
	if b := p.Booking; b != nil {
		r.BookingCode = b.Code
		r.Guide = person(b.Guide, b.GuideID)
		if b.Gig != nil {
			r.GigTitle = b.Gig.Title
		}
	}
	if p.Method != nil {
		m := toMethod(p.Method)
		r.Method = &m
	}
	return r
}

func toGateway(p *domain.GatewayPayment, withEvents []domain.GatewayEvent) dto.GatewayPaymentResponse {
	r := dto.GatewayPaymentResponse{ID: p.ID.String(), Gateway: p.Gateway, Mode: p.Mode, TranID: p.TranID, Status: p.Status,
		Amount: p.Amount, Currency: p.Currency, BookingID: p.BookingID.String(), GigID: p.GigID.String(),
		Payer: person(p.Payer, p.PayerID), Guide: person(p.Guide, p.GuideID), PgTxnID: p.PgTxnID, BankTxn: p.BankTxn,
		PaymentType: p.PaymentType, CardNumber: p.CardNumber, PaidAmount: p.PaidAmount, StoreAmount: p.StoreAmount,
		ProcessingCharge: p.ProcessingCharge, FailureReason: p.FailureReason, VerifiedAt: p.VerifiedAt,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
	if p.Booking != nil {
		r.BookingCode = p.Booking.Code
	}
	if p.Gig != nil {
		r.GigTitle = p.Gig.Title
	}
	for _, e := range withEvents {
		ev := dto.GatewayEventResponse{Event: e.Event, IP: e.IPAddress, CreatedAt: e.CreatedAt}
		_ = e.Payload.Decode(&ev.Payload)
		r.Events = append(r.Events, ev)
	}
	return r
}
