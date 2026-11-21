// Package domain holds the booking module's entities (bookings, payment methods,
// manual mobile-banking payments), state rules, errors and the repository port.
package domain

import (
	"context"
	"net/http"
	"slices"
	"time"

	"cholobangladesh-server/pkg/apperror"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Booking status flow:
//
//	pending ──accept──▶ accepted ──complete──▶ completed ──(pay + admin verifies)──▶ payment_status = paid
//	   │                   │
//	   ├─reject──▶ rejected└─cancel──▶ cancelled
//	   └─cancel──▶ cancelled
const (
	StatusPending   = "pending"
	StatusAccepted  = "accepted"
	StatusRejected  = "rejected"
	StatusCancelled = "cancelled"
	StatusCompleted = "completed"

	PayUnpaid    = "unpaid"
	PaySubmitted = "submitted"
	PayPaid      = "paid"
	PayRefunded  = "refunded"

	PaymentPending  = "pending"
	PaymentVerified = "verified"
	PaymentRejected = "rejected"
)

// CanTransition reports whether a booking may move from -> to.
func CanTransition(from, to string) bool {
	allowed := map[string][]string{
		StatusPending:  {StatusAccepted, StatusRejected, StatusCancelled},
		StatusAccepted: {StatusCompleted, StatusCancelled},
	}
	return slices.Contains(allowed[from], to)
}

type Booking struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey"`
	Code          string
	GigID         uuid.UUID `gorm:"type:uuid"`
	GuideID       uuid.UUID `gorm:"type:uuid"`
	UserID        uuid.UUID `gorm:"type:uuid"`
	SpotID        uuid.UUID `gorm:"type:uuid"`
	TourDate      time.Time `gorm:"type:date"`
	StartTime     *string
	GroupSize     int
	PricingType   string
	UnitPrice     int
	TotalAmount   int
	Currency      string
	Status        string
	PaymentStatus string
	Note          *string
	ContactPhone  *string
	StatusReason  *string
	AcceptedAt    *time.Time
	CompletedAt   *time.Time
	CancelledAt   *time.Time
	CancelledBy   *uuid.UUID `gorm:"type:uuid"`
	CreatedAt     time.Time
	UpdatedAt     time.Time

	HasReview bool `gorm:"->"` // filled by queries

	Gig        *GigRef    `gorm:"foreignKey:GigID"`
	Spot       *SpotRef   `gorm:"foreignKey:SpotID"`
	Guide      *PersonRef `gorm:"foreignKey:GuideID;references:UserID"`
	User       *PersonRef `gorm:"foreignKey:UserID;references:UserID"`
	GuideLogin *LoginRef  `gorm:"foreignKey:GuideID"`
	UserLogin  *LoginRef  `gorm:"foreignKey:UserID"`
}

func (Booking) TableName() string { return "bookings" }

// IsParty reports whether uid is the tourist or the guide of the booking.
func (b *Booking) IsParty(uid uuid.UUID) bool { return b.UserID == uid || b.GuideID == uid }

type GigRef struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey"`
	Title         string
	DurationHours float64
	MeetingPoint  *string
	DeletedAt     gorm.DeletedAt
}

func (GigRef) TableName() string { return "guide_gigs" }

type SpotRef struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey"`
	Slug          string
	Title         string
	CoverImageURL *string `gorm:"column:cover_image_url"`
	LocationName  string
	DeletedAt     gorm.DeletedAt
}

func (SpotRef) TableName() string { return "tourist_spots" }

type PersonRef struct {
	UserID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	FullName       string
	AvatarURL      *string
	WhatsappNumber *string
}

func (PersonRef) TableName() string { return "user_profiles" }

// LoginRef gives contact details, shared only after the guide accepts.
type LoginRef struct {
	ID    uuid.UUID `gorm:"type:uuid;primaryKey"`
	Email *string
	Phone *string
}

func (LoginRef) TableName() string { return "users" }

// BookableGig is what booking needs to know about a gig.
type BookableGig struct {
	ID           uuid.UUID
	GuideID      uuid.UUID
	SpotID       uuid.UUID
	Price        int
	Currency     string
	PricingType  string
	MaxGroupSize int
}

type PaymentMethod struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey"`
	Provider      string
	AccountName   string
	AccountNumber string
	AccountType   string
	Instructions  *string
	IsActive      bool
	Position      int
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func (PaymentMethod) TableName() string { return "payment_methods" }

type Payment struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey"`
	Purpose       string     // booking | gig_slot (later)
	BookingID     *uuid.UUID `gorm:"type:uuid"`
	PayerID       uuid.UUID  `gorm:"type:uuid"`
	MethodID      *uuid.UUID `gorm:"type:uuid"`
	Provider      string
	SenderNumber  string
	TransactionID string
	Amount        int
	Currency      string
	Status        string
	AdminNote     *string
	ReviewedBy    *uuid.UUID `gorm:"type:uuid"`
	ReviewedAt    *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time

	Booking *Booking   `gorm:"foreignKey:BookingID"`
	Payer   *PersonRef `gorm:"foreignKey:PayerID;references:UserID"`
	Method  *PaymentMethod
}

func (Payment) TableName() string { return "payments" }

type Filter struct {
	UserID        uuid.UUID
	GuideID       uuid.UUID
	Status        string
	PaymentStatus string
	Page          int
	Limit         int
}

type PaymentFilter struct {
	Status    string
	Provider  string
	BookingID uuid.UUID
	Page      int
	Limit     int
}

type Repository interface {
	Tx(ctx context.Context, fn func(r Repository) error) error

	BookableGig(ctx context.Context, gigID uuid.UUID) (*BookableGig, error)
	CreateBooking(ctx context.Context, b *Booking) error
	GetBooking(ctx context.Context, id uuid.UUID) (*Booking, error)
	// LockBooking re-reads a booking FOR UPDATE inside a transaction.
	LockBooking(ctx context.Context, id uuid.UUID) (*Booking, error)
	ListBookings(ctx context.Context, f Filter) ([]Booking, int64, error)
	UpdateBooking(ctx context.Context, id uuid.UUID, fields map[string]any) error
	// Counters keep gig/guide statistics in sync.
	IncGigCounter(ctx context.Context, gigID uuid.UUID, column string) error
	IncGuideTours(ctx context.Context, guideID uuid.UUID) error

	ListMethods(ctx context.Context, activeOnly bool) ([]PaymentMethod, error)
	GetMethod(ctx context.Context, id uuid.UUID) (*PaymentMethod, error)
	CreateMethod(ctx context.Context, m *PaymentMethod) error
	UpdateMethod(ctx context.Context, id uuid.UUID, fields map[string]any) error
	DeleteMethod(ctx context.Context, id uuid.UUID) error

	CreatePayment(ctx context.Context, p *Payment) error
	GetPayment(ctx context.Context, id uuid.UUID) (*Payment, error)
	ListPayments(ctx context.Context, f PaymentFilter) ([]Payment, int64, error)
	// ReviewPayment moves a pending payment to verified/rejected; false if it was not pending.
	ReviewPayment(ctx context.Context, id uuid.UUID, status string, admin uuid.UUID, note *string) (bool, error)
}

var (
	ErrBookingNotFound   = apperror.New(http.StatusNotFound, "BOOKING_NOT_FOUND", "booking not found")
	ErrGigNotBookable    = apperror.New(http.StatusUnprocessableEntity, "GIG_NOT_BOOKABLE", "this gig is not available for booking right now")
	ErrOwnGig            = apperror.New(http.StatusUnprocessableEntity, "CANNOT_BOOK_OWN_GIG", "you cannot book your own gig")
	ErrGroupTooLarge     = apperror.New(http.StatusUnprocessableEntity, "GROUP_TOO_LARGE", "group size is larger than this gig allows")
	ErrTourDatePast      = apperror.New(http.StatusUnprocessableEntity, "TOUR_DATE_IN_PAST", "choose today or a future date")
	ErrInvalidTransition = apperror.New(http.StatusConflict, "BOOKING_STATUS_CONFLICT", "this action is not possible in the booking's current status")
	ErrTourNotStarted    = apperror.New(http.StatusConflict, "TOUR_NOT_STARTED", "a tour can be completed on or after the tour date")
	ErrPaymentNotDue     = apperror.New(http.StatusConflict, "PAYMENT_NOT_DUE", "payment opens after the guide marks the tour as completed")
	ErrAlreadyPaid       = apperror.New(http.StatusConflict, "ALREADY_PAID", "this booking is already paid or a payment is waiting for verification")
	ErrAmountMismatch    = apperror.New(http.StatusUnprocessableEntity, "PAYMENT_AMOUNT_MISMATCH", "the amount must equal the booking total")
	ErrTrxUsed           = apperror.New(http.StatusConflict, "TRANSACTION_ID_USED", "this transaction ID was already submitted")
	ErrMethodNotFound    = apperror.New(http.StatusNotFound, "PAYMENT_METHOD_NOT_FOUND", "payment method not found or inactive")
	ErrPaymentNotFound   = apperror.New(http.StatusNotFound, "PAYMENT_NOT_FOUND", "payment not found")
	ErrPaymentReviewed   = apperror.New(http.StatusConflict, "PAYMENT_ALREADY_REVIEWED", "this payment has already been reviewed")
)
