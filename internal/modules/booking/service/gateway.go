package service

import (
	"context"
	"crypto/rand"
	"errors"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"time"

	"cholobangladesh-server/internal/modules/booking/domain"
	"cholobangladesh-server/internal/modules/booking/dto"
	"cholobangladesh-server/internal/platform/payment"
	"cholobangladesh-server/internal/platform/realtime"
	"cholobangladesh-server/internal/shared"
	"cholobangladesh-server/internal/shared/web"
	"cholobangladesh-server/pkg/jsonb"
	"cholobangladesh-server/pkg/listing"
	"cholobangladesh-server/pkg/response"

	"github.com/google/uuid"
)

// TranIDPattern validates our merchant transaction ids ("CBP" + 24 chars).
var TranIDPattern = regexp.MustCompile(`^CBP[A-Z0-9]{24}$`)

func newTranID() string {
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return "CBP" + string(b)
}

// GatewayEnabled reports whether aamarPay is configured.
func (s *Service) GatewayEnabled() bool { return s.gw != nil }

func (s *Service) event(ctx context.Context, r domain.Repository, paymentID uuid.UUID, ev string, payload any, ip string) {
	e := &domain.GatewayEvent{PaymentID: paymentID, Event: ev, Payload: jsonb.From(payload), CreatedAt: time.Now().UTC()}
	if ip != "" {
		e.IPAddress = &ip
	}
	if err := r.AddGatewayEvent(ctx, e); err != nil {
		slog.Warn("gateway event", "event", ev, "err", err)
	}
}

// PayOnline starts an aamarPay checkout for an accepted, unpaid booking (tourist only).
func (s *Service) PayOnline(ctx context.Context, userID, bookingID uuid.UUID, ip string) (*dto.PayOnlineResponse, error) {
	if s.gw == nil {
		return nil, domain.ErrGatewayDisabled
	}
	b, err := s.repo.GetBooking(ctx, bookingID)
	if err != nil || b.UserID != userID {
		return nil, domain.ErrBookingNotFound
	}
	if b.Status != domain.StatusAccepted {
		return nil, domain.ErrPaymentNotDue
	}
	if b.PaymentStatus != domain.PayUnpaid {
		return nil, domain.ErrAlreadyPaid
	}
	payer, err := s.repo.PayerContact(ctx, userID)
	if err != nil {
		return nil, err
	}

	p := &domain.GatewayPayment{ID: uuid.Must(uuid.NewV7()), Gateway: "aamarpay", Mode: s.gw.Mode(), TranID: newTranID(),
		BookingID: b.ID, GigID: b.GigID, PayerID: b.UserID, GuideID: b.GuideID, Amount: b.TotalAmount,
		Currency: b.Currency, Status: domain.GwInitiated, ClientIP: &ip}
	if err := s.repo.CreateGateway(ctx, p); err != nil {
		return nil, err
	}

	title := "Tour booking " + b.Code
	if b.Gig != nil {
		title = b.Gig.Title + " (" + b.Code + ")"
	}
	email := "payments@cholobangladesh.com" // aamarPay requires an email; phone-only tourists use ours
	if payer.Email != nil && *payer.Email != "" {
		email = *payer.Email
	}
	phoneNo := "+8801700000000"
	if b.ContactPhone != nil {
		phoneNo = *b.ContactPhone
	} else if payer.Phone != nil {
		phoneNo = *payer.Phone
	}
	city, country := "Dhaka", "Bangladesh"
	if payer.City != nil && *payer.City != "" {
		city = *payer.City
	}
	if payer.CountryCode != nil && *payer.CountryCode != "" {
		country = *payer.CountryCode
	}
	cb := func(result string) string {
		return s.apiURL + "/api/v1/payments/aamarpay/return?" + url.Values{"tran_id": {p.TranID}, "result": {result}}.Encode()
	}
	checkout := payment.Checkout{TranID: p.TranID, Amount: b.TotalAmount, Currency: b.Currency, Desc: truncate(title, 200),
		CusName: truncate(nonEmpty(payer.FullName, "Traveller"), 100), CusEmail: email, CusPhone: phoneNo, CusCity: city,
		CusCountry: country, SuccessURL: cb("success"), FailURL: cb("fail"), CancelURL: cb("cancel"),
		OptA: b.ID.String(), OptB: b.Code, OptC: b.UserID.String(), OptD: b.GuideID.String()}

	payURL, err := s.gw.Start(ctx, checkout)
	if err != nil {
		reason := truncate(err.Error(), 500)
		_ = s.repo.UpdateGateway(ctx, p.ID, map[string]any{"status": domain.GwFailed, "failure_reason": reason})
		s.event(ctx, s.repo, p.ID, "initiate_failed", map[string]any{"error": err.Error()}, ip)
		if errors.Is(err, payment.ErrRejected) {
			// aamarPay answered but refused (wrong store ID / signature key / URL): show its reason.
			return nil, domain.ErrGatewayRejected.WithDetails(map[string]string{"gateway_message": strings.TrimPrefix(err.Error(), payment.ErrRejected.Error()+": ")})
		}
		return nil, domain.ErrGatewayFailed
	}
	s.event(ctx, s.repo, p.ID, "initiated", map[string]any{"amount": p.Amount, "currency": p.Currency, "mode": p.Mode}, ip)
	return &dto.PayOnlineResponse{PaymentURL: payURL, TranID: p.TranID, Amount: p.Amount, Currency: p.Currency,
		Sandbox: s.gw.Sandbox()}, nil
}

// GatewayReturn handles aamarPay's browser redirect (success / fail / cancel). The posted
// fields are only logged; the outcome comes from a server-to-server verification.
// It returns the frontend URL to redirect the tourist to.
func (s *Service) GatewayReturn(ctx context.Context, tranID, result string, form map[string]string, ip string) string {
	fallback := s.webURL + "/account/bookings?payment=unknown"
	if !TranIDPattern.MatchString(tranID) {
		return fallback
	}
	p, err := s.repo.GetGateway(ctx, tranID)
	if err != nil {
		return fallback
	}
	s.event(ctx, s.repo, p.ID, "callback_"+result, form, ip)
	if fp, _ := s.finalize(ctx, tranID, result); fp != nil {
		p = fp
	}
	status := "pending"
	{
		switch p.Status {
		case domain.GwSuccess, domain.GwDuplicate:
			status = "success"
		case domain.GwFailed:
			status = "failed"
		case domain.GwCancelled:
			status = "cancelled"
		}
	}
	return s.webURL + touristLink(p.BookingID.String()) + "?" + url.Values{"payment": {status}, "tran": {tranID}}.Encode()
}

// finalize verifies a transaction with aamarPay and applies the result exactly once.
// source: success | fail | cancel (browser callback), manual (user/admin "check again"), reconcile.
func (s *Service) finalize(ctx context.Context, tranID, source string) (*domain.GatewayPayment, error) {
	cur, err := s.repo.GetGateway(ctx, tranID)
	if err != nil {
		return nil, web.As(err, web.ErrNotFound, domain.ErrPaymentNotFound)
	}
	if cur.Status == domain.GwSuccess || cur.Status == domain.GwDuplicate || s.gw == nil {
		return cur, nil
	}

	vctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	v, verr := s.gw.Verify(vctx, tranID)
	cancel()
	if verr != nil {
		s.event(ctx, s.repo, cur.ID, "verify_error", map[string]any{"error": verr.Error(), "source": source}, "")
		return cur, nil
	}

	var became string
	err = s.repo.Tx(ctx, func(r domain.Repository) error {
		p, err := r.LockGateway(ctx, tranID)
		if err != nil {
			return err
		}
		if p.Status == domain.GwSuccess || p.Status == domain.GwDuplicate {
			return nil
		}
		b, err := r.LockBooking(ctx, p.BookingID)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		f := map[string]any{"gateway_response": jsonb.From(v.Raw), "verified_at": now}
		setStr := func(col, val string) {
			if val != "" {
				f[col] = val
			}
		}
		setStr("pg_txn_id", v.PgTxnID)
		setStr("bank_txn", v.BankTrx)
		setStr("payment_type", v.PaymentType)
		setStr("card_number", v.CardNumber)
		if v.Amount > 0 {
			f["paid_amount"] = v.Amount
		}
		if v.StoreAmount > 0 {
			f["store_amount"] = v.StoreAmount
		}
		if v.ProcessingCharge > 0 {
			f["processing_charge"] = v.ProcessingCharge
		}

		switch {
		case v.Successful():
			mismatch := ""
			switch {
			case v.MerTxnID != "" && v.MerTxnID != p.TranID:
				mismatch = "transaction id mismatch"
			case !v.AmountMatches(p.Amount):
				mismatch = "amount mismatch"
			case v.Currency != "" && !strings.EqualFold(v.Currency, p.Currency):
				mismatch = "currency mismatch"
			case !s.gw.StoreMatches(v):
				mismatch = "store mismatch"
			}
			if mismatch != "" {
				f["status"], f["failure_reason"] = domain.GwFailed, "verification failed: "+mismatch
				became = domain.GwFailed
				slog.Error("aamarpay verification mismatch", "tran_id", tranID, "reason", mismatch)
				break
			}
			if b.PaymentStatus == domain.PayPaid || b.Status == domain.StatusCancelled || b.Status == domain.StatusRejected {
				// Paid twice (two tabs) or after the booking closed: keep the money record, admin refunds.
				f["status"], f["failure_reason"] = domain.GwDuplicate, "booking was already paid or closed — refund needed"
				became = domain.GwDuplicate
				break
			}
			f["status"] = domain.GwSuccess
			became = domain.GwSuccess
			if err := r.UpdateBooking(ctx, b.ID, map[string]any{"payment_status": domain.PayPaid, "paid_at": now}); err != nil {
				return err
			}
		case v.Failed() || source == "fail":
			f["status"], f["failure_reason"] = domain.GwFailed, "payment "+strings.ToLower(v.PayStatus)
			became = domain.GwFailed
		case source == "cancel":
			f["status"], f["failure_reason"] = domain.GwCancelled, "cancelled by the payer"
			became = domain.GwCancelled
		}
		if err := r.UpdateGateway(ctx, p.ID, f); err != nil {
			if errors.Is(err, web.ErrDuplicate) { // pg_txn_id already used by another row
				delete(f, "pg_txn_id")
				f["status"], f["failure_reason"] = domain.GwFailed, "gateway transaction id already used"
				became = domain.GwFailed
				return r.UpdateGateway(ctx, p.ID, f)
			}
			return err
		}
		s.event(ctx, r, p.ID, "verified", map[string]any{"source": source, "pay_status": v.PayStatus,
			"status_code": v.StatusCode, "amount": v.Amount, "result": became}, "")
		return nil
	})
	if err != nil {
		slog.Error("aamarpay finalize", "tran_id", tranID, "err", err)
		return cur, err
	}

	p, err := s.repo.GetGateway(ctx, tranID)
	if err != nil {
		return cur, nil
	}
	if became != "" {
		s.afterGateway(ctx, p, became)
	}
	return p, nil
}

func (s *Service) afterGateway(ctx context.Context, p *domain.GatewayPayment, became string) {
	payload := map[string]any{"tran_id": p.TranID, "booking_id": p.BookingID.String(), "status": p.Status,
		"amount": p.Amount}
	s.notify.Push(ctx, []string{realtime.UserRoom(p.PayerID), realtime.UserRoom(p.GuideID),
		realtime.RoleRoom(shared.RoleAdmin)}, realtime.EvPaymentUpdated, payload)

	b, err := s.Get(ctx, p.PayerID, shared.RoleUser, p.BookingID)
	if err != nil {
		return
	}
	switch became {
	case domain.GwSuccess:
		s.onBookingEvent(ctx, b, evPaid, p.PayerID)
	case domain.GwFailed, domain.GwCancelled:
		s.onBookingEvent(ctx, b, evPaymentFailed, p.PayerID)
	case domain.GwDuplicate:
		s.notify.NotifyRole(ctx, shared.RoleAdmin, shared.Notification{Type: "payment.duplicate",
			Title: "Duplicate payment — refund needed", Body: "Transaction " + p.TranID + " for booking " + b.Code +
				" was paid after the booking was already paid or closed.", Link: "/admin/transactions/" + p.TranID,
			Data: payload})
	}
}

// CheckTransaction re-verifies a transaction (payer or admin), e.g. when the callback was missed.
func (s *Service) CheckTransaction(ctx context.Context, viewer uuid.UUID, role, tranID string) (*dto.GatewayPaymentResponse, error) {
	if !TranIDPattern.MatchString(tranID) {
		return nil, domain.ErrPaymentNotFound
	}
	p, err := s.repo.GetGateway(ctx, tranID)
	if err != nil || (role != shared.RoleAdmin && p.PayerID != viewer) {
		return nil, domain.ErrPaymentNotFound
	}
	if p, err = s.finalize(ctx, tranID, "manual"); err != nil && p == nil {
		return nil, err
	}
	r := toGateway(p, nil)
	return &r, nil
}

// BookingTransactions lists online payment attempts of one booking (tourist, guide, admin).
func (s *Service) BookingTransactions(ctx context.Context, viewer uuid.UUID, role string, bookingID uuid.UUID) ([]dto.GatewayPaymentResponse, error) {
	if _, err := s.load(ctx, viewer, role, bookingID); err != nil {
		return nil, err
	}
	res, err := s.repo.ListGateway(ctx, domain.GatewayFilter{BookingID: bookingID, Params: listing.Params{Limit: 50}})
	if err != nil {
		return nil, err
	}
	return listing.Map(res, gatewayItem).Items, nil
}

func (s *Service) AdminTransactions(ctx context.Context, q dto.GatewayQuery) (response.Page[dto.GatewayPaymentResponse], error) {
	res, err := s.repo.ListGateway(ctx, domain.GatewayFilter{Status: q.Status, Params: q.Params})
	if err != nil {
		return response.Page[dto.GatewayPaymentResponse]{}, err
	}
	return listing.Map(res, gatewayItem), nil
}

// gatewayItem is the list shape of an online payment (no audit trail).
func gatewayItem(p *domain.GatewayPayment) dto.GatewayPaymentResponse { return toGateway(p, nil) }

func (s *Service) AdminTransaction(ctx context.Context, tranID string) (*dto.GatewayPaymentResponse, error) {
	if !TranIDPattern.MatchString(tranID) {
		return nil, domain.ErrPaymentNotFound
	}
	p, err := s.repo.GetGateway(ctx, tranID)
	if err != nil {
		return nil, web.As(err, web.ErrNotFound, domain.ErrPaymentNotFound)
	}
	events, err := s.repo.GatewayEvents(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	r := toGateway(p, events)
	return &r, nil
}

// StartReconciler re-checks checkouts whose browser callback never arrived (closed tab,
// network drop) every 10 minutes; attempts older than a day are closed as failed.
func (s *Service) StartReconciler(ctx context.Context) {
	if s.gw == nil {
		return
	}
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			res, err := s.repo.ListGateway(ctx, domain.GatewayFilter{Status: domain.GwInitiated,
				Params: listing.Params{Limit: 50, Sort: "oldest"}})
			if err != nil {
				continue
			}
			list := res.Items
			for i := range list {
				p := list[i]
				age := time.Since(p.CreatedAt)
				if age < 10*time.Minute {
					continue
				}
				res, _ := s.finalize(ctx, p.TranID, "reconcile")
				if res != nil && res.Status == domain.GwInitiated && age > 24*time.Hour {
					_ = s.repo.UpdateGateway(ctx, p.ID, map[string]any{"status": domain.GwFailed,
						"failure_reason": "expired: no payment within 24 hours"})
					s.event(ctx, s.repo, p.ID, "expired", nil, "")
				}
			}
		}
	}()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

func nonEmpty(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
