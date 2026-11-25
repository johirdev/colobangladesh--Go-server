package repository

import (
	"context"
	"time"

	"cholobangladesh-server/internal/modules/booking/domain"
	"cholobangladesh-server/internal/shared/web"
	"cholobangladesh-server/pkg/listing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func unscoped(db *gorm.DB) *gorm.DB { return db.Unscoped() }

// ------------------------------------------------------------------ schedule

func (r *postgres) GigGuide(ctx context.Context, gigID uuid.UUID) (uuid.UUID, error) {
	var ids []uuid.UUID
	err := r.q(ctx).Table("guide_gigs").Where("id = ? AND deleted_at IS NULL", gigID).Limit(1).Pluck("guide_id", &ids).Error
	if err != nil {
		return uuid.Nil, err
	}
	if len(ids) == 0 {
		return uuid.Nil, web.ErrNotFound
	}
	return ids[0], nil
}

func (r *postgres) GuideBusy(ctx context.Context, guideID uuid.UUID, from, to time.Time, except uuid.UUID) ([]domain.BusyRange, error) {
	var rows []struct {
		ID       uuid.UUID
		TourDate time.Time
		EndDate  time.Time
		Status   string
	}
	err := r.q(ctx).Table("bookings").Select("id, tour_date, end_date, status").
		Where("guide_id = ? AND status = ? AND id <> ? AND tour_date <= ? AND end_date >= ?",
			guideID, domain.StatusAccepted, except, to, from).
		Order("tour_date").Limit(500).Scan(&rows).Error
	out := make([]domain.BusyRange, 0, len(rows))
	for _, x := range rows {
		out = append(out, domain.BusyRange{BookingID: x.ID, Start: x.TourDate, End: x.EndDate, Status: x.Status})
	}
	return out, err
}

func (r *postgres) LockGuide(ctx context.Context, guideID uuid.UUID) error {
	return r.q(ctx).Exec("SELECT 1 FROM users WHERE id = ? FOR UPDATE", guideID).Error
}

func (r *postgres) PayerContact(ctx context.Context, userID uuid.UUID) (*domain.PayerContact, error) {
	var c domain.PayerContact
	err := r.q(ctx).Raw(`SELECT p.full_name, u.email, u.phone, p.city, p.country_code
		FROM users u JOIN user_profiles p ON p.user_id = u.id WHERE u.id = ?`, userID).Scan(&c).Error
	return &c, err
}

// ------------------------------------------------------------------ online payments

func (r *postgres) CreateGateway(ctx context.Context, p *domain.GatewayPayment) error {
	return web.MapDBErr(r.q(ctx).Omit(clause.Associations).Create(p).Error)
}

func gatewayRefs(q *gorm.DB) *gorm.DB {
	return q.Preload("Booking").Preload("Gig", unscoped).Preload("Payer").Preload("Guide")
}

func (r *postgres) GetGateway(ctx context.Context, tranID string) (*domain.GatewayPayment, error) {
	var p domain.GatewayPayment
	err := gatewayRefs(r.q(ctx)).Where("tran_id = ?", tranID).Take(&p).Error
	return &p, web.MapDBErr(err)
}

func (r *postgres) LockGateway(ctx context.Context, tranID string) (*domain.GatewayPayment, error) {
	var p domain.GatewayPayment
	err := r.q(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("tran_id = ?", tranID).Take(&p).Error
	return &p, web.MapDBErr(err)
}

func (r *postgres) UpdateGateway(ctx context.Context, id uuid.UUID, fields map[string]any) error {
	fields["updated_at"] = time.Now().UTC()
	return web.MapDBErr(r.q(ctx).Model(&domain.GatewayPayment{}).Where("id = ?", id).Updates(fields).Error)
}

// GatewaySpec: online (aamarPay) payments; ?q= matches tran id, gateway txn id or booking code.
var GatewaySpec = listing.Spec{
	Sorts: map[string]string{
		"newest":      "gateway_payments.created_at DESC",
		"oldest":      "gateway_payments.created_at ASC",
		"amount_high": "gateway_payments.amount DESC",
		"amount_low":  "gateway_payments.amount ASC",
	},
	DefaultSort: "newest",
	Tiebreak:    "gateway_payments.id",
	Search: []string{
		"gateway_payments.tran_id",
		"gateway_payments.pg_txn_id",
		"EXISTS (SELECT 1 FROM bookings gb WHERE gb.id = gateway_payments.booking_id AND gb.code ILIKE ?)",
	},
}

func (r *postgres) ListGateway(ctx context.Context, f domain.GatewayFilter) (listing.Result[domain.GatewayPayment], error) {
	q := listing.Filter(r.q(ctx).Model(&domain.GatewayPayment{})).
		Eq("gateway_payments.status", f.Status).
		Eq("gateway_payments.booking_id", f.BookingID).
		Eq("gateway_payments.payer_id", f.PayerID).
		Eq("gateway_payments.guide_id", f.GuideID)
	return listing.Find[domain.GatewayPayment](q, f.Params, GatewaySpec, gatewayRefs)
}

func (r *postgres) AddGatewayEvent(ctx context.Context, e *domain.GatewayEvent) error {
	return r.q(ctx).Create(e).Error
}

func (r *postgres) GatewayEvents(ctx context.Context, paymentID uuid.UUID) ([]domain.GatewayEvent, error) {
	var out []domain.GatewayEvent
	err := r.q(ctx).Where("payment_id = ?", paymentID).Order("id").Limit(100).Find(&out).Error
	return out, err
}

// ------------------------------------------------------------------ trip plans

func (r *postgres) GetPlan(ctx context.Context, bookingID uuid.UUID) (*domain.TripPlan, error) {
	var p domain.TripPlan
	err := r.q(ctx).Preload("Editor").Where("booking_id = ?", bookingID).Take(&p).Error
	return &p, web.MapDBErr(err)
}

func (r *postgres) SavePlan(ctx context.Context, p *domain.TripPlan) error {
	return r.q(ctx).Omit(clause.Associations).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "booking_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"arrival_at", "arrival_from", "arrival_mode", "pickup_name",
			"pickup_map_url", "hotel_name", "hotel_map_url", "hotel_check_in", "hotel_check_out", "itinerary", "notes",
			"updated_by", "updated_at"}),
	}).Create(p).Error
}

// PlanSpec: trip plans monitor; ?q= matches the booking code.
var PlanSpec = listing.Spec{
	Sorts: map[string]string{
		"tour_date": "b.tour_date ASC, booking_trip_plans.updated_at DESC",
		"updated":   "booking_trip_plans.updated_at DESC",
	},
	DefaultSort: "tour_date",
	Tiebreak:    "booking_trip_plans.booking_id",
	Search:      []string{"b.code"},
}

func (r *postgres) ListPlans(ctx context.Context, f domain.PlanFilter) (listing.Result[domain.TripPlan], error) {
	q := listing.Filter(r.q(ctx).Model(&domain.TripPlan{})).
		Joins("JOIN bookings b ON b.id = booking_trip_plans.booking_id").
		If(f.Upcoming, "b.end_date >= CURRENT_DATE AND b.status = ?", domain.StatusAccepted)
	return listing.Find[domain.TripPlan](q, f.Params, PlanSpec, func(db *gorm.DB) *gorm.DB {
		return db.Preload("Editor").Preload("Booking").Preload("Booking.Gig", unscoped).Preload("Booking.Spot", unscoped).
			Preload("Booking.Guide").Preload("Booking.User")
	})
}

// ------------------------------------------------------------------ chat

func (r *postgres) CreateMessage(ctx context.Context, m *domain.Message) error {
	return r.q(ctx).Omit(clause.Associations).Create(m).Error
}

func (r *postgres) ListMessages(ctx context.Context, bookingID uuid.UUID, before *time.Time, limit int) ([]domain.Message, error) {
	q := r.q(ctx).Preload("Sender").Where("booking_id = ?", bookingID)
	if before != nil {
		q = q.Where("created_at < ?", *before)
	}
	var out []domain.Message
	err := q.Order("created_at DESC").Limit(limit).Find(&out).Error
	return out, err
}

func (r *postgres) MarkMessagesRead(ctx context.Context, bookingID, reader uuid.UUID) (int64, error) {
	res := r.q(ctx).Model(&domain.Message{}).
		Where("booking_id = ? AND sender_id IS DISTINCT FROM ? AND read_at IS NULL", bookingID, reader).
		Update("read_at", time.Now().UTC())
	return res.RowsAffected, res.Error
}

func (r *postgres) UnreadMessages(ctx context.Context, userID uuid.UUID) ([]domain.UnreadRow, error) {
	var out []domain.UnreadRow
	err := r.q(ctx).Raw(`SELECT m.booking_id, COUNT(*) AS count FROM booking_messages m
		JOIN bookings b ON b.id = m.booking_id AND (b.user_id = ? OR b.guide_id = ?)
		WHERE m.sender_id IS DISTINCT FROM ? AND m.read_at IS NULL GROUP BY m.booking_id`, userID, userID, userID).Scan(&out).Error
	return out, err
}
