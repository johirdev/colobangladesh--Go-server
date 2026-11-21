// Package service implements bookings and the default payment flow
// (mobile-banking "send money" + transaction ID, verified by an admin).
package service

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"time"

	"cholobangladesh-server/internal/modules/booking/domain"
	"cholobangladesh-server/internal/modules/booking/dto"
	"cholobangladesh-server/internal/shared"
	"cholobangladesh-server/internal/shared/web"
	"cholobangladesh-server/pkg/apperror"
	"cholobangladesh-server/pkg/phone"
	"cholobangladesh-server/pkg/response"

	"github.com/google/uuid"
)

type Service struct{ repo domain.Repository }

func New(repo domain.Repository) *Service { return &Service{repo: repo} }

var dhaka = func() *time.Location {
	if l, err := time.LoadLocation("Asia/Dhaka"); err == nil {
		return l
	}
	return time.FixedZone("BDT", 6*3600)
}()

func today() time.Time {
	y, m, d := time.Now().In(dhaka).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func newCode() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/I
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return "CB" + string(b)
}

func bookingNotFound(err error) error { return web.As(err, web.ErrNotFound, domain.ErrBookingNotFound) }

// ================================================================== tourist

func (s *Service) Create(ctx context.Context, userID uuid.UUID, req dto.CreateBookingRequest) (*dto.BookingResponse, error) {
	gig, err := s.repo.BookableGig(ctx, uuid.MustParse(req.GigID))
	if err != nil {
		return nil, web.As(err, web.ErrNotFound, domain.ErrGigNotBookable)
	}
	if gig.GuideID == userID {
		return nil, domain.ErrOwnGig
	}
	if req.GroupSize > gig.MaxGroupSize {
		return nil, domain.ErrGroupTooLarge.WithDetails(map[string]int{"max_group_size": gig.MaxGroupSize})
	}
	date, _ := time.Parse("2006-01-02", req.TourDate)
	if date.Before(today()) {
		return nil, domain.ErrTourDatePast
	}
	total := gig.Price
	if gig.PricingType == "per_person" {
		total = gig.Price * req.GroupSize
	}
	b := &domain.Booking{ID: uuid.Must(uuid.NewV7()), Code: newCode(), GigID: gig.ID, GuideID: gig.GuideID, UserID: userID,
		SpotID: gig.SpotID, TourDate: date, StartTime: req.StartTime, GroupSize: req.GroupSize, PricingType: gig.PricingType,
		UnitPrice: gig.Price, TotalAmount: total, Currency: gig.Currency, Status: domain.StatusPending,
		PaymentStatus: domain.PayUnpaid, Note: trim(req.Note)}
	if req.ContactPhone != nil {
		if p, ok := phone.NormalizeE164(*req.ContactPhone); ok {
			b.ContactPhone = &p
		}
	}
	err = s.repo.Tx(ctx, func(r domain.Repository) error {
		for range 3 { // retry on the (very unlikely) booking code collision
			err := r.CreateBooking(ctx, b)
			if !errors.Is(err, web.ErrDuplicate) {
				if err != nil {
					return err
				}
				return r.IncGigCounter(ctx, gig.ID, "booking_count")
			}
			b.Code = newCode()
		}
		return apperror.ErrInternal
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, userID, shared.RoleUser, b.ID)
}

func (s *Service) ListMine(ctx context.Context, userID uuid.UUID, q dto.BookingQuery) (response.Page[dto.BookingResponse], error) {
	return s.list(ctx, userID, shared.RoleUser, domain.Filter{UserID: userID}, q)
}

func (s *Service) ListForGuide(ctx context.Context, guideID uuid.UUID, q dto.BookingQuery) (response.Page[dto.BookingResponse], error) {
	return s.list(ctx, guideID, shared.RoleGuide, domain.Filter{GuideID: guideID}, q)
}

func (s *Service) AdminList(ctx context.Context, adminID uuid.UUID, q dto.BookingQuery) (response.Page[dto.BookingResponse], error) {
	return s.list(ctx, adminID, shared.RoleAdmin, domain.Filter{}, q)
}

func (s *Service) list(ctx context.Context, viewer uuid.UUID, role string, f domain.Filter, q dto.BookingQuery) (response.Page[dto.BookingResponse], error) {
	page, limit := web.Paging(q.Page, q.Limit, 20, 100)
	f.Status, f.PaymentStatus, f.Page, f.Limit = q.Status, q.PaymentStatus, page, limit
	list, total, err := s.repo.ListBookings(ctx, f)
	if err != nil {
		return response.Page[dto.BookingResponse]{}, err
	}
	items := make([]dto.BookingResponse, 0, len(list))
	for i := range list {
		items = append(items, toBooking(&list[i], viewer, role))
	}
	return response.NewPage(items, page, limit, total), nil
}

// Get returns a booking to its tourist, its guide or an admin.
func (s *Service) Get(ctx context.Context, viewer uuid.UUID, role string, id uuid.UUID) (*dto.BookingResponse, error) {
	b, err := s.repo.GetBooking(ctx, id)
	if err != nil {
		return nil, bookingNotFound(err)
	}
	if role != shared.RoleAdmin && !b.IsParty(viewer) {
		return nil, domain.ErrBookingNotFound
	}
	r := toBooking(b, viewer, role)
	return &r, nil
}

// ================================================================== status changes

// transition applies a status change after checking who may do it.
func (s *Service) transition(ctx context.Context, actor uuid.UUID, role string, id uuid.UUID, to, reason string) (*dto.BookingResponse, error) {
	err := s.repo.Tx(ctx, func(r domain.Repository) error {
		b, err := r.LockBooking(ctx, id)
		if err != nil {
			return bookingNotFound(err)
		}
		isGuide, isTourist := b.GuideID == actor, b.UserID == actor
		switch to {
		case domain.StatusAccepted, domain.StatusRejected, domain.StatusCompleted:
			if !isGuide {
				return domain.ErrBookingNotFound
			}
		case domain.StatusCancelled:
			if !isGuide && !isTourist {
				return domain.ErrBookingNotFound
			}
		}
		if !domain.CanTransition(b.Status, to) {
			return domain.ErrInvalidTransition.WithDetails(map[string]string{"status": b.Status})
		}
		now := time.Now().UTC()
		f := map[string]any{"status": to}
		if r := strings.TrimSpace(reason); r != "" {
			f["status_reason"] = r
		}
		switch to {
		case domain.StatusAccepted:
			f["accepted_at"] = now
		case domain.StatusCancelled:
			f["cancelled_at"], f["cancelled_by"] = now, actor
		case domain.StatusCompleted:
			if b.TourDate.After(today()) {
				return domain.ErrTourNotStarted
			}
			f["completed_at"] = now
			if err := r.IncGigCounter(ctx, b.GigID, "completed_count"); err != nil {
				return err
			}
			if err := r.IncGuideTours(ctx, b.GuideID); err != nil {
				return err
			}
		}
		return r.UpdateBooking(ctx, id, f)
	})
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, actor, role, id)
}

func (s *Service) Accept(ctx context.Context, guideID, id uuid.UUID) (*dto.BookingResponse, error) {
	return s.transition(ctx, guideID, shared.RoleGuide, id, domain.StatusAccepted, "")
}

func (s *Service) Reject(ctx context.Context, guideID, id uuid.UUID, reason string) (*dto.BookingResponse, error) {
	return s.transition(ctx, guideID, shared.RoleGuide, id, domain.StatusRejected, reason)
}

func (s *Service) Complete(ctx context.Context, guideID, id uuid.UUID) (*dto.BookingResponse, error) {
	return s.transition(ctx, guideID, shared.RoleGuide, id, domain.StatusCompleted, "")
}

func (s *Service) Cancel(ctx context.Context, actor uuid.UUID, role string, id uuid.UUID, reason string) (*dto.BookingResponse, error) {
	return s.transition(ctx, actor, role, id, domain.StatusCancelled, reason)
}

// ================================================================== payments

func (s *Service) Methods(ctx context.Context, activeOnly bool) ([]dto.PaymentMethodResponse, error) {
	list, err := s.repo.ListMethods(ctx, activeOnly)
	if err != nil {
		return nil, err
	}
	out := make([]dto.PaymentMethodResponse, 0, len(list))
	for i := range list {
		out = append(out, toMethod(&list[i]))
	}
	return out, nil
}

// SubmitPayment records a manual mobile-banking payment for a completed tour.
func (s *Service) SubmitPayment(ctx context.Context, userID, bookingID uuid.UUID, req dto.SubmitPaymentRequest) (*dto.PaymentResponse, error) {
	m, err := s.repo.GetMethod(ctx, uuid.MustParse(req.MethodID))
	if err != nil || !m.IsActive {
		return nil, domain.ErrMethodNotFound
	}
	sender, _ := phone.NormalizeBD(req.SenderNumber)
	p := &domain.Payment{ID: uuid.Must(uuid.NewV7()), Purpose: "booking", BookingID: &bookingID, PayerID: userID,
		MethodID: &m.ID, Provider: m.Provider, SenderNumber: sender, TransactionID: strings.ToUpper(req.TransactionID),
		Amount: req.Amount, Currency: "BDT", Status: domain.PaymentPending}

	err = s.repo.Tx(ctx, func(r domain.Repository) error {
		b, err := r.LockBooking(ctx, bookingID)
		if err != nil || b.UserID != userID {
			return domain.ErrBookingNotFound
		}
		if b.Status != domain.StatusCompleted {
			return domain.ErrPaymentNotDue
		}
		if b.PaymentStatus != domain.PayUnpaid {
			return domain.ErrAlreadyPaid
		}
		if req.Amount != b.TotalAmount {
			return domain.ErrAmountMismatch.WithDetails(map[string]int{"total_amount": b.TotalAmount})
		}
		p.Currency = b.Currency
		if err := r.CreatePayment(ctx, p); err != nil {
			if errors.Is(err, web.ErrDuplicate) {
				return domain.ErrTrxUsed
			}
			return err
		}
		return r.UpdateBooking(ctx, bookingID, map[string]any{"payment_status": domain.PaySubmitted})
	})
	if err != nil {
		return nil, err
	}
	return s.payment(ctx, p.ID)
}

func (s *Service) BookingPayments(ctx context.Context, viewer uuid.UUID, role string, bookingID uuid.UUID) ([]dto.PaymentResponse, error) {
	if _, err := s.Get(ctx, viewer, role, bookingID); err != nil {
		return nil, err
	}
	list, _, err := s.repo.ListPayments(ctx, domain.PaymentFilter{BookingID: bookingID, Page: 1, Limit: 50})
	if err != nil {
		return nil, err
	}
	out := make([]dto.PaymentResponse, 0, len(list))
	for i := range list {
		out = append(out, toPayment(&list[i]))
	}
	return out, nil
}

func (s *Service) AdminPayments(ctx context.Context, q dto.PaymentQuery) (response.Page[dto.PaymentResponse], error) {
	page, limit := web.Paging(q.Page, q.Limit, 20, 100)
	list, total, err := s.repo.ListPayments(ctx, domain.PaymentFilter{Status: q.Status, Provider: q.Provider, Page: page, Limit: limit})
	if err != nil {
		return response.Page[dto.PaymentResponse]{}, err
	}
	items := make([]dto.PaymentResponse, 0, len(list))
	for i := range list {
		items = append(items, toPayment(&list[i]))
	}
	return response.NewPage(items, page, limit, total), nil
}

// ReviewPayment verifies (booking becomes paid) or rejects (booking back to unpaid) a payment.
func (s *Service) ReviewPayment(ctx context.Context, adminID, id uuid.UUID, verify bool, note string) (*dto.PaymentResponse, error) {
	status, bookingPay := domain.PaymentRejected, domain.PayUnpaid
	if verify {
		status, bookingPay = domain.PaymentVerified, domain.PayPaid
	}
	err := s.repo.Tx(ctx, func(r domain.Repository) error {
		p, err := r.GetPayment(ctx, id)
		if err != nil {
			return web.As(err, web.ErrNotFound, domain.ErrPaymentNotFound)
		}
		ok, err := r.ReviewPayment(ctx, id, status, adminID, trim(&note))
		if err != nil {
			return err
		}
		if !ok {
			return domain.ErrPaymentReviewed
		}
		if p.BookingID != nil {
			return r.UpdateBooking(ctx, *p.BookingID, map[string]any{"payment_status": bookingPay})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.payment(ctx, id)
}

func (s *Service) payment(ctx context.Context, id uuid.UUID) (*dto.PaymentResponse, error) {
	p, err := s.repo.GetPayment(ctx, id)
	if err != nil {
		return nil, web.As(err, web.ErrNotFound, domain.ErrPaymentNotFound)
	}
	r := toPayment(p)
	return &r, nil
}

// ------------------------------------------------------------------ payment methods (admin)

func (s *Service) CreateMethod(ctx context.Context, req dto.PaymentMethodRequest) (*dto.PaymentMethodResponse, error) {
	m := &domain.PaymentMethod{ID: uuid.Must(uuid.NewV7()), Provider: req.Provider, AccountName: strings.TrimSpace(req.AccountName),
		AccountNumber: strings.TrimSpace(req.AccountNumber), AccountType: req.AccountType, Instructions: trim(req.Instructions),
		IsActive: req.IsActive == nil || *req.IsActive, Position: req.Position}
	if err := s.repo.CreateMethod(ctx, m); err != nil {
		return nil, err
	}
	r := toMethod(m)
	return &r, nil
}

func (s *Service) UpdateMethod(ctx context.Context, id uuid.UUID, req dto.PaymentMethodPatch) (*dto.PaymentMethodResponse, error) {
	f := map[string]any{}
	for k, v := range map[string]*string{"provider": req.Provider, "account_name": req.AccountName,
		"account_number": req.AccountNumber, "account_type": req.AccountType} {
		if v != nil {
			f[k] = strings.TrimSpace(*v)
		}
	}
	if req.Instructions != nil {
		f["instructions"] = trim(req.Instructions)
	}
	if req.IsActive != nil {
		f["is_active"] = *req.IsActive
	}
	if req.Position != nil {
		f["position"] = *req.Position
	}
	if err := s.repo.UpdateMethod(ctx, id, f); err != nil {
		return nil, web.As(err, web.ErrNotFound, domain.ErrMethodNotFound)
	}
	m, err := s.repo.GetMethod(ctx, id)
	if err != nil {
		return nil, web.As(err, web.ErrNotFound, domain.ErrMethodNotFound)
	}
	r := toMethod(m)
	return &r, nil
}

func (s *Service) DeleteMethod(ctx context.Context, id uuid.UUID) error {
	return web.As(s.repo.DeleteMethod(ctx, id), web.ErrNotFound, domain.ErrMethodNotFound)
}

// ================================================================== mapping

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
		r.FullName, r.AvatarURL = p.FullName, p.AvatarURL
	}
	return r
}

func toBooking(b *domain.Booking, viewer uuid.UUID, role string) dto.BookingResponse {
	r := dto.BookingResponse{ID: b.ID.String(), Code: b.Code, Status: b.Status, PaymentStatus: b.PaymentStatus,
		TourDate: b.TourDate.Format("2006-01-02"), StartTime: b.StartTime, GroupSize: b.GroupSize, PricingType: b.PricingType,
		UnitPrice: b.UnitPrice, TotalAmount: b.TotalAmount, Currency: b.Currency, Note: b.Note, StatusReason: b.StatusReason,
		HasReview: b.HasReview, AcceptedAt: b.AcceptedAt, CompletedAt: b.CompletedAt, CancelledAt: b.CancelledAt,
		CreatedAt: b.CreatedAt, Guide: person(b.Guide, b.GuideID), Tourist: person(b.User, b.UserID)}
	if g := b.Gig; g != nil {
		r.Gig = &dto.BookingGig{ID: g.ID.String(), Title: g.Title, DurationHours: g.DurationHours, MeetingPoint: g.MeetingPoint}
	}
	if sp := b.Spot; sp != nil {
		r.Spot = &dto.BookingSpot{ID: sp.ID.String(), Slug: sp.Slug, Title: sp.Title, CoverImageURL: sp.CoverImageURL,
			LocationName: sp.LocationName}
	}

	isTourist, isGuide, isAdmin := b.UserID == viewer, b.GuideID == viewer, role == shared.RoleAdmin
	contactsOpen := b.Status == domain.StatusAccepted || b.Status == domain.StatusCompleted
	if contactsOpen || isAdmin {
		if isTourist || isAdmin {
			r.GuideContact = contact(b.GuideLogin, b.Guide, nil)
		}
		if isGuide || isAdmin {
			r.TouristContact = contact(b.UserLogin, b.User, b.ContactPhone)
		}
	}
	r.CanAccept = isGuide && b.Status == domain.StatusPending
	r.CanReject = r.CanAccept
	r.CanCancel = (isGuide || isTourist) && (b.Status == domain.StatusPending || b.Status == domain.StatusAccepted)
	r.CanComplete = isGuide && b.Status == domain.StatusAccepted && !b.TourDate.After(today())
	r.CanPay = isTourist && b.Status == domain.StatusCompleted && b.PaymentStatus == domain.PayUnpaid
	r.CanReview = isTourist && b.Status == domain.StatusCompleted && !b.HasReview
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
