// Package domain holds the booking module's entities (bookings, online + manual payments,
// trip plans, chat), state rules, errors and the repository port.
package domain

import (
	"context"
	"net/http"
	"slices"
	"time"

	"cholobangladesh-server/pkg/apperror"
	"cholobangladesh-server/pkg/jsonb"
	"cholobangladesh-server/pkg/listing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Booking flow:
//
//	pending ──guide accepts──▶ accepted ──tourist pays (aamarPay)──▶ accepted + paid
//	   │                         │                                       │
//	   ├─guide rejects─▶ rejected │                    tourist confirms (on/after tour_date)
//	   └─cancel─▶ cancelled       └─cancel (unpaid only)─▶ cancelled   or admin (after end_date)
//	                                                                     ▼
//	                                                                 completed ──▶ review
//
// After payment: trip plan (arrival, pickup, hotel, itinerary) + tourist<->guide chat.
const (
	StatusPending   = "pending"
	StatusAccepted  = "accepted"
	StatusRejected  = "rejected"
	StatusCancelled = "cancelled"
	StatusCompleted = "completed"

	PayUnpaid    = "unpaid"
	PaySubmitted = "submitted" // manual mobile-banking payment waiting for an admin
	PayPaid      = "paid"
	PayRefunded  = "refunded"

	PaymentPending  = "pending"
	PaymentVerified = "verified"
	PaymentRejected = "rejected"

	// Online gateway payment statuses.
	GwInitiated = "initiated"
	GwSuccess   = "success"
	GwFailed    = "failed"
	GwCancelled = "cancelled"
	GwDuplicate = "duplicate" // paid after the booking was already paid → admin refunds

	MaxTourDays = 60
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
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	Code           string
	GigID          uuid.UUID `gorm:"type:uuid"`
	GuideID        uuid.UUID `gorm:"type:uuid"`
	UserID         uuid.UUID `gorm:"type:uuid"`
	SpotID         uuid.UUID `gorm:"type:uuid"`
	TourDate       time.Time `gorm:"type:date"`
	EndDate        time.Time `gorm:"type:date"`
	StartTime      *string
	GroupSize      int
	PricingType    string
	UnitPrice      int
	TotalAmount    int
	Currency       string
	Status         string
	PaymentStatus  string
	Note           *string
	ContactPhone   *string
	StatusReason   *string
	AcceptedAt     *time.Time
	PaidAt         *time.Time
	CompletedAt    *time.Time
	CompletedBy    *uuid.UUID `gorm:"type:uuid"`
	CompletionNote *string
	CancelledAt    *time.Time
	CancelledBy    *uuid.UUID `gorm:"type:uuid"`
	CreatedAt      time.Time
	UpdatedAt      time.Time

	HasReview bool `gorm:"->"` // filled by queries
	HasPlan   bool `gorm:"->"` // filled by queries

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

// IsPaid reports a confirmed payment.
func (b *Booking) IsPaid() bool { return b.PaymentStatus == PayPaid }

// Live reports a paid booking whose tour is not finished yet: tourist and guide can chat.
// Once the tour is completed the chat becomes read-only (CholoBangladesh posts a closing note).
func (b *Booking) Live() bool {
	return b.IsPaid() && b.Status == StatusAccepted
}

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
	CountryCode    *string
	City           *string
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
	ID            uuid.UUID
	GuideID       uuid.UUID
	SpotID        uuid.UUID
	Title         string
	Price         int
	Currency      string
	PricingType   string
	MaxGroupSize  int
	DurationHours float64
}

// BusyRange is a date range a guide is already booked for (accepted bookings).
type BusyRange struct {
	BookingID uuid.UUID
	Start     time.Time
	End       time.Time
	Status    string
}

// ------------------------------------------------------------------ manual payments

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

// ------------------------------------------------------------------ online payments

// GatewayPayment is one aamarPay checkout attempt.
type GatewayPayment struct {
	ID               uuid.UUID `gorm:"type:uuid;primaryKey"`
	Gateway          string
	Mode             string
	TranID           string
	BookingID        uuid.UUID `gorm:"type:uuid"`
	GigID            uuid.UUID `gorm:"type:uuid"`
	PayerID          uuid.UUID `gorm:"type:uuid"`
	GuideID          uuid.UUID `gorm:"type:uuid"`
	Amount           int
	Currency         string
	Status           string
	PgTxnID          *string
	BankTxn          *string
	PaymentType      *string
	CardNumber       *string
	PaidAmount       *float64
	StoreAmount      *float64
	ProcessingCharge *float64
	FailureReason    *string
	GatewayResponse  jsonb.JSON `gorm:"type:jsonb"`
	ClientIP         *string
	VerifiedAt       *time.Time
	CreatedAt        time.Time
	UpdatedAt        time.Time

	Booking *Booking   `gorm:"foreignKey:BookingID"`
	Payer   *PersonRef `gorm:"foreignKey:PayerID;references:UserID"`
	Guide   *PersonRef `gorm:"foreignKey:GuideID;references:UserID"`
	Gig     *GigRef    `gorm:"foreignKey:GigID"`
}

func (GatewayPayment) TableName() string { return "gateway_payments" }

type GatewayEvent struct {
	ID        int64     `gorm:"primaryKey"`
	PaymentID uuid.UUID `gorm:"type:uuid"`
	Event     string
	Payload   jsonb.JSON `gorm:"type:jsonb"`
	IPAddress *string
	CreatedAt time.Time
}

func (GatewayEvent) TableName() string { return "gateway_payment_events" }

type GatewayFilter struct {
	Status         string
	BookingID      uuid.UUID
	PayerID        uuid.UUID
	GuideID        uuid.UUID
	listing.Params // q: tran id / pg txn id / booking code
}

// PlanFilter lists trip plans (admin monitor).
type PlanFilter struct {
	Upcoming bool
	listing.Params
}

// ------------------------------------------------------------------ trip plan & chat

type ItineraryItem struct {
	Date   string `json:"date"`
	Time   string `json:"time,omitempty"`
	Title  string `json:"title"`
	Place  string `json:"place,omitempty"`
	MapURL string `json:"map_url,omitempty"`
	Note   string `json:"note,omitempty"`
}

type TripPlan struct {
	BookingID     uuid.UUID `gorm:"type:uuid;primaryKey"`
	ArrivalAt     *time.Time
	ArrivalFrom   *string
	ArrivalMode   *string
	PickupName    *string
	PickupMapURL  *string
	HotelName     *string
	HotelMapURL   *string
	HotelCheckIn  *time.Time `gorm:"type:date"`
	HotelCheckOut *time.Time `gorm:"type:date"`
	Itinerary     jsonb.JSON `gorm:"type:jsonb"`
	Notes         *string
	UpdatedBy     *uuid.UUID `gorm:"type:uuid"`
	CreatedAt     time.Time
	UpdatedAt     time.Time

	Booking *Booking   `gorm:"foreignKey:BookingID"`
	Editor  *PersonRef `gorm:"foreignKey:UpdatedBy;references:UserID"`
}

func (TripPlan) TableName() string { return "booking_trip_plans" }

const (
	MessageUser   = "user"
	MessageSystem = "system" // posted by CholoBangladesh, no sender
)

type Message struct {
	ID        uuid.UUID  `gorm:"type:uuid;primaryKey"`
	BookingID uuid.UUID  `gorm:"type:uuid"`
	SenderID  *uuid.UUID `gorm:"type:uuid"` // nil for system messages
	Kind      string
	Body      string
	ReadAt    *time.Time
	CreatedAt time.Time

	Sender *PersonRef `gorm:"foreignKey:SenderID;references:UserID"`
}

func (Message) TableName() string { return "booking_messages" }

type UnreadRow struct {
	BookingID uuid.UUID
	Count     int64
}

// PayerContact is what aamarPay needs about the customer.
type PayerContact struct {
	FullName    string
	Email       *string
	Phone       *string
	City        *string
	CountryCode *string
}

// ------------------------------------------------------------------ filters & port

type Filter struct {
	UserID        uuid.UUID
	GuideID       uuid.UUID
	Status        string
	PaymentStatus string
	From          *time.Time // bookings ending on/after
	To            *time.Time // bookings starting on/before
	listing.Params
}

type PaymentFilter struct {
	Status    string
	Provider  string
	BookingID uuid.UUID
	listing.Params
}

type Repository interface {
	Tx(ctx context.Context, fn func(r Repository) error) error

	BookableGig(ctx context.Context, gigID uuid.UUID) (*BookableGig, error)
	GigGuide(ctx context.Context, gigID uuid.UUID) (uuid.UUID, error)
	CreateBooking(ctx context.Context, b *Booking) error
	GetBooking(ctx context.Context, id uuid.UUID) (*Booking, error)
	// LockBooking re-reads a booking FOR UPDATE inside a transaction.
	LockBooking(ctx context.Context, id uuid.UUID) (*Booking, error)
	ListBookings(ctx context.Context, f Filter) (listing.Result[Booking], error)
	UpdateBooking(ctx context.Context, id uuid.UUID, fields map[string]any) error
	// GuideBusy lists the guide's accepted bookings overlapping [from, to] (except one booking).
	GuideBusy(ctx context.Context, guideID uuid.UUID, from, to time.Time, except uuid.UUID) ([]BusyRange, error)
	// LockGuide serialises accept decisions for one guide (prevents double-booking races).
	LockGuide(ctx context.Context, guideID uuid.UUID) error
	// Counters keep gig/guide statistics in sync.
	IncGigCounter(ctx context.Context, gigID uuid.UUID, column string) error
	IncGuideTours(ctx context.Context, guideID uuid.UUID) error
	PayerContact(ctx context.Context, userID uuid.UUID) (*PayerContact, error)

	ListMethods(ctx context.Context, activeOnly bool) ([]PaymentMethod, error)
	GetMethod(ctx context.Context, id uuid.UUID) (*PaymentMethod, error)
	CreateMethod(ctx context.Context, m *PaymentMethod) error
	UpdateMethod(ctx context.Context, id uuid.UUID, fields map[string]any) error
	DeleteMethod(ctx context.Context, id uuid.UUID) error

	CreatePayment(ctx context.Context, p *Payment) error
	GetPayment(ctx context.Context, id uuid.UUID) (*Payment, error)
	ListPayments(ctx context.Context, f PaymentFilter) (listing.Result[Payment], error)
	// ReviewPayment moves a pending payment to verified/rejected; false if it was not pending.
	ReviewPayment(ctx context.Context, id uuid.UUID, status string, admin uuid.UUID, note *string) (bool, error)

	CreateGateway(ctx context.Context, p *GatewayPayment) error
	GetGateway(ctx context.Context, tranID string) (*GatewayPayment, error)
	LockGateway(ctx context.Context, tranID string) (*GatewayPayment, error)
	UpdateGateway(ctx context.Context, id uuid.UUID, fields map[string]any) error
	ListGateway(ctx context.Context, f GatewayFilter) (listing.Result[GatewayPayment], error)
	AddGatewayEvent(ctx context.Context, e *GatewayEvent) error
	GatewayEvents(ctx context.Context, paymentID uuid.UUID) ([]GatewayEvent, error)

	GetPlan(ctx context.Context, bookingID uuid.UUID) (*TripPlan, error)
	SavePlan(ctx context.Context, p *TripPlan) error
	ListPlans(ctx context.Context, f PlanFilter) (listing.Result[TripPlan], error)

	CreateMessage(ctx context.Context, m *Message) error
	ListMessages(ctx context.Context, bookingID uuid.UUID, before *time.Time, limit int) ([]Message, error)
	MarkMessagesRead(ctx context.Context, bookingID, reader uuid.UUID) (int64, error)
	UnreadMessages(ctx context.Context, userID uuid.UUID) ([]UnreadRow, error)
}

var (
	ErrBookingNotFound   = apperror.New(http.StatusNotFound, "BOOKING_NOT_FOUND", "booking not found")
	ErrGigNotBookable    = apperror.New(http.StatusUnprocessableEntity, "GIG_NOT_BOOKABLE", "this gig is not available for booking right now")
	ErrOwnGig            = apperror.New(http.StatusUnprocessableEntity, "CANNOT_BOOK_OWN_GIG", "you cannot book your own gig")
	ErrGroupTooLarge     = apperror.New(http.StatusUnprocessableEntity, "GROUP_TOO_LARGE", "group size is larger than this gig allows")
	ErrTourDatePast      = apperror.New(http.StatusUnprocessableEntity, "TOUR_DATE_IN_PAST", "choose today or a future date")
	ErrBadDateRange      = apperror.New(http.StatusUnprocessableEntity, "INVALID_DATE_RANGE", "the end date must be on or after the start date (max 60 days)")
	ErrDatesTaken        = apperror.New(http.StatusConflict, "DATES_ALREADY_BOOKED", "the guide is already booked on some of these dates")
	ErrInvalidTransition = apperror.New(http.StatusConflict, "BOOKING_STATUS_CONFLICT", "this action is not possible in the booking's current status")
	ErrTourNotStarted    = apperror.New(http.StatusConflict, "TOUR_NOT_STARTED", "a tour can be confirmed as completed on or after its start date")
	ErrTourNotEnded      = apperror.New(http.StatusConflict, "TOUR_NOT_ENDED", "admins can complete a tour only after its end date has passed")
	ErrNotPaid           = apperror.New(http.StatusConflict, "BOOKING_NOT_PAID", "this booking has not been paid yet")
	ErrPaidCancel        = apperror.New(http.StatusConflict, "PAID_BOOKING_CANCEL", "a paid booking can only be cancelled by an admin (refund)")
	ErrPaymentNotDue     = apperror.New(http.StatusConflict, "PAYMENT_NOT_DUE", "payment opens after the guide accepts the booking")
	ErrAlreadyPaid       = apperror.New(http.StatusConflict, "ALREADY_PAID", "this booking is already paid or a payment is waiting for verification")
	ErrAmountMismatch    = apperror.New(http.StatusUnprocessableEntity, "PAYMENT_AMOUNT_MISMATCH", "the amount must equal the booking total")
	ErrTrxUsed           = apperror.New(http.StatusConflict, "TRANSACTION_ID_USED", "this transaction ID was already submitted")
	ErrMethodNotFound    = apperror.New(http.StatusNotFound, "PAYMENT_METHOD_NOT_FOUND", "payment method not found or inactive")
	ErrPaymentNotFound   = apperror.New(http.StatusNotFound, "PAYMENT_NOT_FOUND", "payment not found")
	ErrPaymentReviewed   = apperror.New(http.StatusConflict, "PAYMENT_ALREADY_REVIEWED", "this payment has already been reviewed")
	ErrGatewayDisabled   = apperror.New(http.StatusServiceUnavailable, "PAYMENT_GATEWAY_DISABLED", "online payment is not available right now")
	ErrGatewayFailed     = apperror.New(http.StatusBadGateway, "PAYMENT_GATEWAY_ERROR", "the payment gateway did not respond, please try again")
	ErrGatewayRejected   = apperror.New(http.StatusBadGateway, "PAYMENT_GATEWAY_REJECTED", "the payment gateway rejected the request — check STORE_ID / SIGNATURE_KEY / AAMARPAY_URL")
	ErrChatClosed        = apperror.New(http.StatusConflict, "CHAT_NOT_OPEN", "messaging opens after the booking is paid")
	ErrChatEnded         = apperror.New(http.StatusConflict, "CHAT_ENDED", "the tour is completed, this chat is now read-only")
	ErrPlanClosed        = apperror.New(http.StatusConflict, "TRIP_PLAN_NOT_OPEN", "the trip plan opens after the booking is paid")
	ErrReadOnly          = apperror.New(http.StatusForbidden, "READ_ONLY", "admins can view but not post here")
)
