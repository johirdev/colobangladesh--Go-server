// Package repository implements the booking repository with PostgreSQL (GORM).
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

type postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) domain.Repository { return &postgres{db: db} }

func (r *postgres) q(ctx context.Context) *gorm.DB { return r.db.WithContext(ctx) }

func (r *postgres) Tx(ctx context.Context, fn func(domain.Repository) error) error {
	return r.q(ctx).Transaction(func(tx *gorm.DB) error { return fn(&postgres{db: tx}) })
}

func (r *postgres) BookableGig(ctx context.Context, gigID uuid.UUID) (*domain.BookableGig, error) {
	var g domain.BookableGig
	err := r.q(ctx).Raw(`SELECT g.id, g.guide_id, g.spot_id, g.title, g.price, g.currency, g.pricing_type,
			g.max_group_size, g.duration_hours
		FROM guide_gigs g
		JOIN users u ON u.id = g.guide_id AND u.status = 'active' AND u.deleted_at IS NULL
		JOIN guide_profiles gp ON gp.user_id = g.guide_id AND gp.verification_status = 'approved' AND gp.is_available
		JOIN tourist_spots s ON s.id = g.spot_id AND s.status = 'published' AND s.deleted_at IS NULL
		WHERE g.id = ? AND g.status = 'active' AND g.deleted_at IS NULL`, gigID).Scan(&g).Error
	if err != nil {
		return nil, err
	}
	if g.ID == uuid.Nil {
		return nil, web.ErrNotFound
	}
	return &g, nil
}

func (r *postgres) CreateBooking(ctx context.Context, b *domain.Booking) error {
	return web.MapDBErr(r.q(ctx).Omit(clause.Associations).Create(b).Error)
}

const hasReview = "bookings.*, EXISTS (SELECT 1 FROM reviews rv WHERE rv.booking_id = bookings.id) AS has_review, " +
	"EXISTS (SELECT 1 FROM booking_trip_plans tp WHERE tp.booking_id = bookings.id) AS has_plan"

func withRefs(q *gorm.DB) *gorm.DB {
	unscoped := func(db *gorm.DB) *gorm.DB { return db.Unscoped() }
	return q.Preload("Gig", unscoped).Preload("Spot", unscoped).Preload("Guide").Preload("User").
		Preload("GuideLogin").Preload("UserLogin")
}

func (r *postgres) GetBooking(ctx context.Context, id uuid.UUID) (*domain.Booking, error) {
	var b domain.Booking
	err := withRefs(r.q(ctx).Model(&domain.Booking{}).Select(hasReview)).Where("bookings.id = ?", id).Take(&b).Error
	return &b, web.MapDBErr(err)
}

func (r *postgres) LockBooking(ctx context.Context, id uuid.UUID) (*domain.Booking, error) {
	var b domain.Booking
	err := r.q(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).Take(&b).Error
	return &b, web.MapDBErr(err)
}

// BookingSpec: ?sort= values and what ?q= searches (code, tourist / guide name, tour title).
var BookingSpec = listing.Spec{
	Sorts: map[string]string{
		"newest":         "bookings.created_at DESC",
		"oldest":         "bookings.created_at ASC",
		"tour_date":      "bookings.tour_date ASC, bookings.start_time ASC NULLS LAST, bookings.created_at ASC",
		"tour_date_desc": "bookings.tour_date DESC, bookings.start_time DESC NULLS LAST",
		"amount_high":    "bookings.total_amount DESC",
		"amount_low":     "bookings.total_amount ASC",
	},
	DefaultSort: "newest",
	Tiebreak:    "bookings.id",
	Search: []string{
		"bookings.code",
		"EXISTS (SELECT 1 FROM user_profiles sp WHERE sp.user_id IN (bookings.user_id, bookings.guide_id) AND sp.full_name ILIKE ?)",
		"EXISTS (SELECT 1 FROM guide_gigs sg WHERE sg.id = bookings.gig_id AND sg.title ILIKE ?)",
	},
}

func (r *postgres) ListBookings(ctx context.Context, f domain.Filter) (listing.Result[domain.Booking], error) {
	q := listing.Filter(r.q(ctx).Model(&domain.Booking{})).
		Eq("bookings.user_id", f.UserID).
		Eq("bookings.guide_id", f.GuideID).
		Eq("bookings.status", f.Status).
		Eq("bookings.payment_status", f.PaymentStatus).
		Gte("bookings.end_date", f.From).
		Lte("bookings.tour_date", f.To)
	return listing.Find[domain.Booking](q, f.Params, BookingSpec, func(db *gorm.DB) *gorm.DB {
		return withRefs(db.Select(hasReview))
	})
}

func (r *postgres) UpdateBooking(ctx context.Context, id uuid.UUID, fields map[string]any) error {
	fields["updated_at"] = time.Now().UTC()
	return r.q(ctx).Model(&domain.Booking{}).Where("id = ?", id).Updates(fields).Error
}

func (r *postgres) IncGigCounter(ctx context.Context, gigID uuid.UUID, column string) error {
	if column != "booking_count" && column != "completed_count" {
		return nil
	}
	return r.q(ctx).Exec("UPDATE guide_gigs SET "+column+" = "+column+" + 1 WHERE id = ?", gigID).Error
}

func (r *postgres) IncGuideTours(ctx context.Context, guideID uuid.UUID) error {
	return r.q(ctx).Exec("UPDATE guide_profiles SET total_tours = total_tours + 1 WHERE user_id = ?", guideID).Error
}

// ------------------------------------------------------------------ payment methods

func (r *postgres) ListMethods(ctx context.Context, activeOnly bool) ([]domain.PaymentMethod, error) {
	q := r.q(ctx)
	if activeOnly {
		q = q.Where("is_active")
	}
	var out []domain.PaymentMethod
	err := q.Order("position, created_at").Find(&out).Error
	return out, err
}

func (r *postgres) GetMethod(ctx context.Context, id uuid.UUID) (*domain.PaymentMethod, error) {
	var m domain.PaymentMethod
	err := r.q(ctx).Where("id = ?", id).Take(&m).Error
	return &m, web.MapDBErr(err)
}

func (r *postgres) CreateMethod(ctx context.Context, m *domain.PaymentMethod) error {
	return r.q(ctx).Create(m).Error
}

func (r *postgres) UpdateMethod(ctx context.Context, id uuid.UUID, fields map[string]any) error {
	fields["updated_at"] = time.Now().UTC()
	res := r.q(ctx).Model(&domain.PaymentMethod{}).Where("id = ?", id).Updates(fields)
	if res.Error == nil && res.RowsAffected == 0 {
		return web.ErrNotFound
	}
	return res.Error
}

func (r *postgres) DeleteMethod(ctx context.Context, id uuid.UUID) error {
	res := r.q(ctx).Where("id = ?", id).Delete(&domain.PaymentMethod{})
	if res.Error == nil && res.RowsAffected == 0 {
		return web.ErrNotFound
	}
	return res.Error
}

// ------------------------------------------------------------------ payments

func (r *postgres) CreatePayment(ctx context.Context, p *domain.Payment) error {
	return web.MapDBErr(r.q(ctx).Omit(clause.Associations).Create(p).Error)
}

func paymentRefs(q *gorm.DB) *gorm.DB {
	return q.Preload("Booking").Preload("Booking.Gig", func(db *gorm.DB) *gorm.DB { return db.Unscoped() }).
		Preload("Booking.Guide").Preload("Payer").Preload("Method")
}

func (r *postgres) GetPayment(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	var p domain.Payment
	err := paymentRefs(r.q(ctx)).Where("id = ?", id).Take(&p).Error
	return &p, web.MapDBErr(err)
}

// PaymentSpec: manual (bKash/Nagad...) payments; ?q= matches TrxID, sender number or booking code.
var PaymentSpec = listing.Spec{
	Sorts: map[string]string{
		"newest":      "payments.created_at DESC",
		"oldest":      "payments.created_at ASC",
		"amount_high": "payments.amount DESC",
		"amount_low":  "payments.amount ASC",
	},
	DefaultSort: "newest",
	Tiebreak:    "payments.id",
	Search: []string{
		"payments.transaction_id",
		"payments.sender_number",
		"EXISTS (SELECT 1 FROM bookings pb WHERE pb.id = payments.booking_id AND pb.code ILIKE ?)",
	},
}

func (r *postgres) ListPayments(ctx context.Context, f domain.PaymentFilter) (listing.Result[domain.Payment], error) {
	q := listing.Filter(r.q(ctx).Model(&domain.Payment{})).
		Eq("payments.status", f.Status).
		Eq("payments.provider", f.Provider).
		Eq("payments.booking_id", f.BookingID)
	return listing.Find[domain.Payment](q, f.Params, PaymentSpec, paymentRefs)
}

func (r *postgres) ReviewPayment(ctx context.Context, id uuid.UUID, status string, admin uuid.UUID, note *string) (bool, error) {
	now := time.Now().UTC()
	res := r.q(ctx).Model(&domain.Payment{}).Where("id = ? AND status = ?", id, domain.PaymentPending).
		Updates(map[string]any{"status": status, "reviewed_by": admin, "reviewed_at": now, "admin_note": note, "updated_at": now})
	return res.RowsAffected == 1, res.Error
}
