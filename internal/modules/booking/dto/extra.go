package dto

import (
	"time"

	"cholobangladesh-server/pkg/listing"
	"cholobangladesh-server/pkg/response"
)

// ================================================================== completion / admin

type CompleteRequest struct {
	Note string `json:"note" binding:"omitempty,max=500" example:"Great tour, everything as planned."`
}

type AdminCompleteRequest struct {
	// Note is required: what the admin confirmed with the guide.
	Note string `json:"note" binding:"required,min=5,max=500" example:"Called the guide: tour finished on 22 Dec, tourist unreachable."`
}

type AdminCancelRequest struct {
	Reason string `json:"reason" binding:"required,min=5,max=500" example:"Guide fell ill, tourist refunded."`
	// Refunded marks the payment as refunded (refund itself is done in the aamarPay merchant panel).
	Refunded bool `json:"refunded" example:"true"`
}

// ================================================================== booked dates (slots)

type BusyRange struct {
	Start string `json:"start" example:"2026-12-20"`
	End   string `json:"end" example:"2026-12-22"`
}

// BookedDatesResponse: dates the gig's guide is already booked (accepted tours). Grey them out in the calendar.
type BookedDatesResponse struct {
	GigID  string      `json:"gig_id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	From   string      `json:"from" example:"2026-12-01"`
	To     string      `json:"to" example:"2027-03-01"`
	Ranges []BusyRange `json:"ranges"`
	Dates  []string    `json:"dates" example:"2026-12-20,2026-12-21,2026-12-22"`
}

type BookedDatesQuery struct {
	From string `form:"from" binding:"omitempty,datetime=2006-01-02" example:"2026-12-01"`
	To   string `form:"to" binding:"omitempty,datetime=2006-01-02" example:"2027-03-01"`
}

// ================================================================== online payment (aamarPay)

type PayOnlineResponse struct {
	// Redirect the browser here (aamarPay hosted payment page).
	PaymentURL string `json:"payment_url" example:"https://sandbox.aamarpay.com/paynow.php?track=AAM123"`
	TranID     string `json:"tran_id" example:"CBP-01JF7Q2W8X3Y4Z5A6B7C8D9E0F"`
	Amount     int    `json:"amount" example:"6000"`
	Currency   string `json:"currency" example:"BDT"`
	Sandbox    bool   `json:"sandbox" example:"true"`
}

type GatewayEventResponse struct {
	Event     string         `json:"event" example:"verified"`
	Payload   map[string]any `json:"payload,omitempty"`
	IP        *string        `json:"ip,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
}

// GatewayPaymentResponse is one aamarPay transaction (tourist, guide, gig, booking, amounts, gateway ids).
type GatewayPaymentResponse struct {
	ID               string                 `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	Gateway          string                 `json:"gateway" example:"aamarpay"`
	Mode             string                 `json:"mode" example:"sandbox" enums:"sandbox,live"`
	TranID           string                 `json:"tran_id" example:"CBP-01JF7Q2W8X3Y4Z5A6B7C8D9E0F"`
	Status           string                 `json:"status" example:"success" enums:"initiated,success,failed,cancelled,duplicate"`
	Amount           int                    `json:"amount" example:"6000"`
	Currency         string                 `json:"currency" example:"BDT"`
	BookingID        string                 `json:"booking_id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	BookingCode      string                 `json:"booking_code,omitempty" example:"CB7K3D9QXA"`
	GigID            string                 `json:"gig_id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	GigTitle         string                 `json:"gig_title,omitempty" example:"Full-day Sundarbans boat safari"`
	Payer            *PersonRef             `json:"payer,omitempty"`
	Guide            *PersonRef             `json:"guide,omitempty"`
	PgTxnID          *string                `json:"pg_txn_id,omitempty" example:"AAM1702512345678"`
	BankTxn          *string                `json:"bank_txn,omitempty"`
	PaymentType      *string                `json:"payment_type,omitempty" example:"bKash-bKash"`
	CardNumber       *string                `json:"card_number,omitempty"`
	PaidAmount       *float64               `json:"paid_amount,omitempty" example:"6000"`
	StoreAmount      *float64               `json:"store_amount,omitempty" example:"5880"`
	ProcessingCharge *float64               `json:"processing_charge,omitempty" example:"120"`
	FailureReason    *string                `json:"failure_reason,omitempty"`
	VerifiedAt       *time.Time             `json:"verified_at,omitempty"`
	CreatedAt        time.Time              `json:"created_at"`
	UpdatedAt        time.Time              `json:"updated_at"`
	Events           []GatewayEventResponse `json:"events,omitempty"` // admin detail only
}

type GatewayPaymentPage struct {
	Items []GatewayPaymentResponse `json:"items"`
	Meta  response.PageMeta        `json:"meta"`
}

// GatewayQuery — ?page ?limit ?q (tran id, gateway txn id, booking code) ?sort=newest|oldest|amount_high|amount_low.
type GatewayQuery struct {
	listing.Params
	Status string `form:"status" binding:"omitempty,oneof=initiated success failed cancelled duplicate" example:"success"`
}

// ================================================================== trip plan

type ItineraryItem struct {
	Date   string `json:"date" binding:"required,datetime=2006-01-02" example:"2026-12-20"`
	Time   string `json:"time" binding:"omitempty,datetime=15:04" example:"09:00"`
	Title  string `json:"title" binding:"required,min=2,max=150" example:"Boat leaves Mongla for Harbaria"`
	Place  string `json:"place" binding:"omitempty,max=200" example:"Mongla ferry ghat"`
	MapURL string `json:"map_url" binding:"omitempty,max=1000,url" example:"https://maps.app.goo.gl/xyz"`
	Note   string `json:"note" binding:"omitempty,max=500" example:"Bring sunscreen"`
}

// TripPlanRequest replaces the whole plan (send every field you want to keep).
type TripPlanRequest struct {
	// When the tourist arrives (RFC3339).
	ArrivalAt     *time.Time      `json:"arrival_at" example:"2026-12-19T18:30:00+06:00"`
	ArrivalFrom   *string         `json:"arrival_from" binding:"omitempty,max=200" example:"Dhaka (Sayedabad bus terminal)"`
	ArrivalMode   *string         `json:"arrival_mode" binding:"omitempty,oneof=bus train air car launch other" example:"bus"`
	PickupName    *string         `json:"pickup_name" binding:"omitempty,max=200" example:"Khulna Sonadanga bus stand"`
	PickupMapURL  *string         `json:"pickup_map_url" binding:"omitempty,max=1000,url" example:"https://maps.app.goo.gl/abc"`
	HotelName     *string         `json:"hotel_name" binding:"omitempty,max=200" example:"Hotel Castle Salam, Khulna"`
	HotelMapURL   *string         `json:"hotel_map_url" binding:"omitempty,max=1000,url" example:"https://maps.app.goo.gl/def"`
	HotelCheckIn  *string         `json:"hotel_check_in" binding:"omitempty,datetime=2006-01-02" example:"2026-12-19"`
	HotelCheckOut *string         `json:"hotel_check_out" binding:"omitempty,datetime=2006-01-02" example:"2026-12-22"`
	Itinerary     []ItineraryItem `json:"itinerary" binding:"omitempty,max=60,dive"`
	Notes         *string         `json:"notes" binding:"omitempty,max=2000" example:"Guide will wait with a CholoBangladesh sign."`
}

type TripPlanResponse struct {
	BookingID     string          `json:"booking_id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	ArrivalAt     *time.Time      `json:"arrival_at,omitempty"`
	ArrivalFrom   *string         `json:"arrival_from,omitempty"`
	ArrivalMode   *string         `json:"arrival_mode,omitempty"`
	PickupName    *string         `json:"pickup_name,omitempty"`
	PickupMapURL  *string         `json:"pickup_map_url,omitempty"`
	HotelName     *string         `json:"hotel_name,omitempty"`
	HotelMapURL   *string         `json:"hotel_map_url,omitempty"`
	HotelCheckIn  *string         `json:"hotel_check_in,omitempty"`
	HotelCheckOut *string         `json:"hotel_check_out,omitempty"`
	Itinerary     []ItineraryItem `json:"itinerary"`
	Notes         *string         `json:"notes,omitempty"`
	UpdatedBy     *PersonRef      `json:"updated_by,omitempty"`
	UpdatedAt     *time.Time      `json:"updated_at,omitempty"`
	Exists        bool            `json:"exists" example:"true"`
	CanEdit       bool            `json:"can_edit" example:"true"`
}

// TripPlanAdminItem is a plan with its booking, for the admin monitor.
type TripPlanAdminItem struct {
	Plan    TripPlanResponse `json:"plan"`
	Booking BookingResponse  `json:"booking"`
}

type TripPlanPage struct {
	Items []TripPlanAdminItem `json:"items"`
	Meta  response.PageMeta   `json:"meta"`
}

// TripPlanQuery — ?page ?limit ?q (booking code) ?sort=tour_date|updated.
type TripPlanQuery struct {
	listing.Params
	Upcoming bool `form:"upcoming" example:"true"`
}

// ================================================================== chat

type MessageRequest struct {
	Body string `json:"body" binding:"required,min=1,max=2000" example:"Hi! We land at 6:30 pm, see you at the bus stand."`
}

// MessageResponse — kind "system" is a note from CholoBangladesh (sender.id = "system",
// sender.role = "system"), e.g. the thank-you message posted when the tour is completed.
type MessageResponse struct {
	ID        string     `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	BookingID string     `json:"booking_id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	Kind      string     `json:"kind" example:"user" enums:"user,system"`
	Sender    *PersonRef `json:"sender"`
	Mine      bool       `json:"mine" example:"true"`
	Body      string     `json:"body" example:"See you tomorrow!"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type MessagesQuery struct {
	// Before: RFC3339 created_at of the oldest message you have (older page).
	Before string `form:"before" binding:"omitempty,max=40" example:"2026-12-19T10:00:00Z"`
	Limit  int    `form:"limit" binding:"omitempty,gte=1,lte=100" example:"50"`
}

type UnreadMessages struct {
	Total    int64            `json:"total" example:"4"`
	Bookings map[string]int64 `json:"bookings"`
}
