// Package service implements the booking flow:
//
//	request → guide accepts → tourist pays online (aamarPay) → trip plan + chat
//	→ tourist confirms completion (or an admin after the end date) → review.
//
// The older manual mobile-banking payment (transaction ID verified by an admin)
// is kept as a fallback.
package service

import (
	"context"
	"crypto/rand"
	"errors"
	"math"
	"strings"
	"time"

	"cholobangladesh-server/internal/modules/booking/domain"
	"cholobangladesh-server/internal/modules/booking/dto"
	"cholobangladesh-server/internal/platform/payment"
	"cholobangladesh-server/internal/shared"
	"cholobangladesh-server/internal/shared/web"
	"cholobangladesh-server/pkg/apperror"
	"cholobangladesh-server/pkg/listing"
	"cholobangladesh-server/pkg/phone"
	"cholobangladesh-server/pkg/response"

	"github.com/google/uuid"
)

type Options struct {
	Notify       shared.Notifier
	Gateway      *payment.Aamarpay // nil = online payment disabled
	PublicAPIURL string            // payment callbacks
	FrontendURL  string            // where tourists land after paying
}

type Service struct {
	repo   domain.Repository
	notify shared.Notifier
	gw     *payment.Aamarpay
	apiURL string
	webURL string
}

func New(repo domain.Repository, o Options) *Service {
	if o.Notify == nil {
		o.Notify = shared.NopNotifier{}
	}
	return &Service{repo: repo, notify: o.Notify, gw: o.Gateway, apiURL: o.PublicAPIURL, webURL: o.FrontendURL}
}

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

const dateFmt = "2006-01-02"

func day(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC) }

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

func busyDetails(list []domain.BusyRange) map[string]any {
	ranges := make([]dto.BusyRange, 0, len(list))
	for _, r := range list {
		ranges = append(ranges, dto.BusyRange{Start: r.Start.Format(dateFmt), End: r.End.Format(dateFmt)})
	}
	return map[string]any{"booked": ranges}
}

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
	start, _ := time.Parse(dateFmt, req.TourDate)
	if start.Before(today()) {
		return nil, domain.ErrTourDatePast
	}
	days := max(1, int(math.Ceil(gig.DurationHours/24)))
	end := start.AddDate(0, 0, days-1)
	if req.EndDate != nil && *req.EndDate != "" {
		end, _ = time.Parse(dateFmt, *req.EndDate)
	}
	if end.Before(start) || end.After(start.AddDate(0, 0, domain.MaxTourDays)) {
		return nil, domain.ErrBadDateRange
	}
	if busy, err := s.repo.GuideBusy(ctx, gig.GuideID, start, end, uuid.Nil); err != nil {
		return nil, err
	} else if len(busy) > 0 {
		return nil, domain.ErrDatesTaken.WithDetails(busyDetails(busy))
	}

	total := gig.Price
	if gig.PricingType == "per_person" {
		total = gig.Price * req.GroupSize
	}
	b := &domain.Booking{ID: uuid.Must(uuid.NewV7()), Code: newCode(), GigID: gig.ID, GuideID: gig.GuideID, UserID: userID,
		SpotID: gig.SpotID, TourDate: start, EndDate: end, StartTime: req.StartTime, GroupSize: req.GroupSize,
		PricingType: gig.PricingType, UnitPrice: gig.Price, TotalAmount: total, Currency: gig.Currency,
		Status: domain.StatusPending, PaymentStatus: domain.PayUnpaid, Note: trim(req.Note)}
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
	res, err := s.Get(ctx, userID, shared.RoleUser, b.ID)
	if err == nil {
		s.onBookingEvent(ctx, res, evRequested, userID)
	}
	return res, err
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
	f.Status, f.PaymentStatus, f.Params = q.Status, q.PaymentStatus, q.Params
	if t, err := time.Parse(dateFmt, q.From); err == nil {
		f.From = &t
	}
	if t, err := time.Parse(dateFmt, q.To); err == nil {
		f.To = &t
	}
	res, err := s.repo.ListBookings(ctx, f)
	if err != nil {
		return response.Page[dto.BookingResponse]{}, err
	}
	return listing.Map(res, func(b *domain.Booking) dto.BookingResponse { return toBooking(b, viewer, role) }), nil
}

// Get returns a booking to its tourist, its guide or an admin.
func (s *Service) Get(ctx context.Context, viewer uuid.UUID, role string, id uuid.UUID) (*dto.BookingResponse, error) {
	b, err := s.load(ctx, viewer, role, id)
	if err != nil {
		return nil, err
	}
	r := toBooking(b, viewer, role)
	return &r, nil
}

// load fetches a booking the viewer may see.
func (s *Service) load(ctx context.Context, viewer uuid.UUID, role string, id uuid.UUID) (*domain.Booking, error) {
	b, err := s.repo.GetBooking(ctx, id)
	if err != nil {
		return nil, bookingNotFound(err)
	}
	if role != shared.RoleAdmin && !b.IsParty(viewer) {
		return nil, domain.ErrBookingNotFound
	}
	return b, nil
}

// BookedDates lists the days a gig's guide is already booked (accepted tours) — public, no personal data.
func (s *Service) BookedDates(ctx context.Context, gigID uuid.UUID, q dto.BookedDatesQuery) (*dto.BookedDatesResponse, error) {
	guideID, err := s.repo.GigGuide(ctx, gigID)
	if err != nil {
		return nil, web.As(err, web.ErrNotFound, apperror.ErrNotFound)
	}
	from, to := today(), today().AddDate(0, 6, 0)
	if t, err := time.Parse(dateFmt, q.From); err == nil {
		from = t
	}
	if t, err := time.Parse(dateFmt, q.To); err == nil {
		to = t
	}
	if to.Before(from) || to.Sub(from) > 400*24*time.Hour {
		to = from.AddDate(0, 6, 0)
	}
	busy, err := s.repo.GuideBusy(ctx, guideID, from, to, uuid.Nil)
	if err != nil {
		return nil, err
	}
	out := &dto.BookedDatesResponse{GigID: gigID.String(), From: from.Format(dateFmt), To: to.Format(dateFmt),
		Ranges: []dto.BusyRange{}, Dates: []string{}}
	seen := map[string]bool{}
	for _, r := range busy {
		out.Ranges = append(out.Ranges, dto.BusyRange{Start: r.Start.Format(dateFmt), End: r.End.Format(dateFmt)})
		for d := day(r.Start); !d.After(r.End); d = d.AddDate(0, 0, 1) {
			if k := d.Format(dateFmt); !seen[k] && !d.Before(from) && !d.After(to) {
				seen[k] = true
				out.Dates = append(out.Dates, k)
			}
		}
	}
	return out, nil
}

// ================================================================== status changes

type change struct {
	actor  uuid.UUID
	role   string // role of the actor
	to     string
	reason string
	admin  bool // admin override (complete after end date / cancel paid booking)
	refund bool
}

// transition applies a status change after checking who may do it.
func (s *Service) transition(ctx context.Context, id uuid.UUID, c change) (*dto.BookingResponse, error) {
	err := s.repo.Tx(ctx, func(r domain.Repository) error {
		b, err := r.LockBooking(ctx, id)
		if err != nil {
			return bookingNotFound(err)
		}
		isGuide, isTourist := b.GuideID == c.actor, b.UserID == c.actor
		if !c.admin {
			switch c.to {
			case domain.StatusAccepted, domain.StatusRejected:
				if !isGuide {
					return domain.ErrBookingNotFound
				}
			case domain.StatusCompleted:
				if !isTourist {
					return domain.ErrBookingNotFound
				}
			case domain.StatusCancelled:
				if !isGuide && !isTourist {
					return domain.ErrBookingNotFound
				}
			}
		}
		if !domain.CanTransition(b.Status, c.to) {
			return domain.ErrInvalidTransition.WithDetails(map[string]string{"status": b.Status})
		}
		now := time.Now().UTC()
		f := map[string]any{"status": c.to}
		if rs := strings.TrimSpace(c.reason); rs != "" && c.to != domain.StatusCompleted {
			f["status_reason"] = rs
		}
		switch c.to {
		case domain.StatusAccepted:
			if b.TourDate.Before(today()) {
				return domain.ErrTourDatePast
			}
			// One decision at a time per guide, then make sure the days are still free.
			if err := r.LockGuide(ctx, b.GuideID); err != nil {
				return err
			}
			busy, err := r.GuideBusy(ctx, b.GuideID, b.TourDate, b.EndDate, b.ID)
			if err != nil {
				return err
			}
			if len(busy) > 0 {
				return domain.ErrDatesTaken.WithDetails(busyDetails(busy))
			}
			f["accepted_at"] = now
		case domain.StatusCancelled:
			if !c.admin && b.PaymentStatus != domain.PayUnpaid {
				return domain.ErrPaidCancel
			}
			f["cancelled_at"], f["cancelled_by"] = now, c.actor
			if c.admin && c.refund && b.PaymentStatus == domain.PayPaid {
				f["payment_status"] = domain.PayRefunded
			}
		case domain.StatusCompleted:
			if !b.IsPaid() {
				return domain.ErrNotPaid
			}
			if c.admin {
				if !today().After(b.EndDate) {
					return domain.ErrTourNotEnded.WithDetails(map[string]string{"end_date": b.EndDate.Format(dateFmt)})
				}
			} else if b.TourDate.After(today()) {
				return domain.ErrTourNotStarted
			}
			f["completed_at"], f["completed_by"] = now, c.actor
			if rs := strings.TrimSpace(c.reason); rs != "" {
				f["completion_note"] = rs
			}
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
	res, err := s.Get(ctx, c.actor, c.role, id)
	if err == nil {
		if c.to == domain.StatusCompleted {
			s.postCompletionMessage(ctx, res)
		}
		s.onBookingEvent(ctx, res, c.to, c.actor)
	}
	return res, err
}

func (s *Service) Accept(ctx context.Context, guideID, id uuid.UUID) (*dto.BookingResponse, error) {
	return s.transition(ctx, id, change{actor: guideID, role: shared.RoleGuide, to: domain.StatusAccepted})
}

func (s *Service) Reject(ctx context.Context, guideID, id uuid.UUID, reason string) (*dto.BookingResponse, error) {
	return s.transition(ctx, id, change{actor: guideID, role: shared.RoleGuide, to: domain.StatusRejected, reason: reason})
}

// Complete: the tourist confirms the tour happened.
func (s *Service) Complete(ctx context.Context, touristID, id uuid.UUID, note string) (*dto.BookingResponse, error) {
	return s.transition(ctx, id, change{actor: touristID, role: shared.RoleUser, to: domain.StatusCompleted, reason: note})
}

func (s *Service) Cancel(ctx context.Context, actor uuid.UUID, role string, id uuid.UUID, reason string) (*dto.BookingResponse, error) {
	return s.transition(ctx, id, change{actor: actor, role: role, to: domain.StatusCancelled, reason: reason})
}

// AdminComplete: after the end date, once an admin has confirmed with the guide.
func (s *Service) AdminComplete(ctx context.Context, adminID, id uuid.UUID, note string) (*dto.BookingResponse, error) {
	return s.transition(ctx, id, change{actor: adminID, role: shared.RoleAdmin, to: domain.StatusCompleted, reason: note, admin: true})
}

func (s *Service) AdminCancel(ctx context.Context, adminID, id uuid.UUID, req dto.AdminCancelRequest) (*dto.BookingResponse, error) {
	return s.transition(ctx, id, change{actor: adminID, role: shared.RoleAdmin, to: domain.StatusCancelled,
		reason: req.Reason, admin: true, refund: req.Refunded})
}

// ================================================================== manual payments (fallback)

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

// SubmitPayment records a manual mobile-banking payment for an accepted booking.
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
		if b.Status != domain.StatusAccepted {
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
	s.notify.NotifyRole(ctx, shared.RoleAdmin, shared.Notification{Type: "payment.submitted", Title: "Manual payment to verify",
		Body: "A tourist submitted a mobile-banking transaction ID.", Link: "/admin/payments",
		Data: map[string]any{"booking_id": bookingID.String()}})
	return s.payment(ctx, p.ID)
}

func (s *Service) BookingPayments(ctx context.Context, viewer uuid.UUID, role string, bookingID uuid.UUID) ([]dto.PaymentResponse, error) {
	if _, err := s.load(ctx, viewer, role, bookingID); err != nil {
		return nil, err
	}
	res, err := s.repo.ListPayments(ctx, domain.PaymentFilter{BookingID: bookingID, Params: listing.Params{Limit: 50}})
	if err != nil {
		return nil, err
	}
	return listing.Map(res, toPayment).Items, nil
}

func (s *Service) AdminPayments(ctx context.Context, q dto.PaymentQuery) (response.Page[dto.PaymentResponse], error) {
	res, err := s.repo.ListPayments(ctx, domain.PaymentFilter{Status: q.Status, Provider: q.Provider, Params: q.Params})
	if err != nil {
		return response.Page[dto.PaymentResponse]{}, err
	}
	return listing.Map(res, toPayment), nil
}

// ReviewPayment verifies (booking becomes paid) or rejects (booking back to unpaid) a manual payment.
func (s *Service) ReviewPayment(ctx context.Context, adminID, id uuid.UUID, verify bool, note string) (*dto.PaymentResponse, error) {
	status, bookingPay := domain.PaymentRejected, domain.PayUnpaid
	if verify {
		status, bookingPay = domain.PaymentVerified, domain.PayPaid
	}
	var bookingID uuid.UUID
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
			bookingID = *p.BookingID
			f := map[string]any{"payment_status": bookingPay}
			if verify {
				f["paid_at"] = time.Now().UTC()
			}
			return r.UpdateBooking(ctx, *p.BookingID, f)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if bookingID != uuid.Nil {
		if b, err := s.Get(ctx, adminID, shared.RoleAdmin, bookingID); err == nil {
			ev := evPaymentRejected
			if verify {
				ev = evPaid
			}
			s.onBookingEvent(ctx, b, ev, adminID)
		}
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
