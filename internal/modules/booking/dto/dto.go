// Package dto holds the request/response contracts of the booking & payment module.
package dto

import (
	"time"

	"cholobangladesh-server/pkg/response"
)

// ================================================================== requests

type CreateBookingRequest struct {
	GigID        string  `json:"gig_id" binding:"required,uuid" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	TourDate     string  `json:"tour_date" binding:"required,datetime=2006-01-02" example:"2026-12-20"`
	StartTime    *string `json:"start_time" binding:"omitempty,datetime=15:04" example:"08:00"`
	GroupSize    int     `json:"group_size" binding:"required,gte=1,lte=100" example:"4"`
	Note         *string `json:"note" binding:"omitempty,max=1000" example:"Two kids in the group, we are vegetarian."`
	ContactPhone *string `json:"contact_phone" binding:"omitempty,intl_phone" example:"+8801712345678"`
}

type ReasonRequest struct {
	Reason string `json:"reason" binding:"omitempty,max=500" example:"Fully booked on that date"`
}

type BookingQuery struct {
	Status        string `form:"status" binding:"omitempty,oneof=pending accepted rejected cancelled completed" example:"pending"`
	PaymentStatus string `form:"payment_status" binding:"omitempty,oneof=unpaid submitted paid refunded" example:"unpaid"`
	Page          int    `form:"page" binding:"omitempty,gte=1" example:"1"`
	Limit         int    `form:"limit" binding:"omitempty,gte=1,lte=100" example:"20"`
}

// SubmitPaymentRequest: the tourist sends money to one of GET /payment-methods (e.g. bKash
// "Send Money"), then submits the sender number and transaction ID here. An admin verifies it.
type SubmitPaymentRequest struct {
	MethodID      string `json:"method_id" binding:"required,uuid" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	SenderNumber  string `json:"sender_number" binding:"required,bd_phone" example:"01812345678"`
	TransactionID string `json:"transaction_id" binding:"required,min=6,max=50,alphanum" example:"8N7A6B5C4D"`
	Amount        int    `json:"amount" binding:"required,gte=1" example:"6000"`
}

type PaymentMethodRequest struct {
	Provider      string  `json:"provider" binding:"required,oneof=bkash nagad rocket upay bank" example:"bkash"`
	AccountName   string  `json:"account_name" binding:"required,min=2,max=100" example:"CholoBangladesh"`
	AccountNumber string  `json:"account_number" binding:"required,min=5,max=40" example:"01700000000"`
	AccountType   string  `json:"account_type" binding:"required,oneof=personal agent merchant bank" example:"personal"`
	Instructions  *string `json:"instructions" binding:"omitempty,max=1000" example:"Use Send Money and put your booking code in the reference."`
	IsActive      *bool   `json:"is_active" example:"true"`
	Position      int     `json:"position" binding:"gte=0,lte=100" example:"0"`
}

type PaymentMethodPatch struct {
	Provider      *string `json:"provider" binding:"omitempty,oneof=bkash nagad rocket upay bank"`
	AccountName   *string `json:"account_name" binding:"omitempty,min=2,max=100"`
	AccountNumber *string `json:"account_number" binding:"omitempty,min=5,max=40"`
	AccountType   *string `json:"account_type" binding:"omitempty,oneof=personal agent merchant bank"`
	Instructions  *string `json:"instructions" binding:"omitempty,max=1000"`
	IsActive      *bool   `json:"is_active"`
	Position      *int    `json:"position" binding:"omitempty,gte=0,lte=100"`
}

type PaymentQuery struct {
	Status   string `form:"status" binding:"omitempty,oneof=pending verified rejected" example:"pending"`
	Provider string `form:"provider" binding:"omitempty,oneof=bkash nagad rocket upay bank" example:"bkash"`
	Page     int    `form:"page" binding:"omitempty,gte=1" example:"1"`
	Limit    int    `form:"limit" binding:"omitempty,gte=1,lte=100" example:"20"`
}

type ReviewPaymentRequest struct {
	Note string `json:"note" binding:"omitempty,max=500" example:"Matched in bKash statement"`
}

// ================================================================== responses

type PersonRef struct {
	ID        string  `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	FullName  string  `json:"full_name" example:"Rahim Uddin"`
	AvatarURL *string `json:"avatar_url,omitempty"`
}

// Contact is shared between tourist and guide only after the guide accepts.
type Contact struct {
	Phone    *string `json:"phone,omitempty" example:"+8801711000001"`
	WhatsApp *string `json:"whatsapp,omitempty" example:"+8801711000001"`
	Email    *string `json:"email,omitempty" example:"rahim.guide@example.com"`
}

type BookingGig struct {
	ID            string  `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	Title         string  `json:"title" example:"Full-day Sundarbans boat safari"`
	DurationHours float64 `json:"duration_hours" example:"8"`
	MeetingPoint  *string `json:"meeting_point,omitempty" example:"Mongla ferry ghat"`
}

type BookingSpot struct {
	ID            string  `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	Slug          string  `json:"slug" example:"sundarbans"`
	Title         string  `json:"title" example:"Sundarbans"`
	CoverImageURL *string `json:"cover_image_url,omitempty"`
	LocationName  string  `json:"location_name" example:"Mongla, Bagerhat"`
}

type BookingResponse struct {
	ID             string       `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	Code           string       `json:"code" example:"CB7K3D9QXA"`
	Status         string       `json:"status" example:"pending" enums:"pending,accepted,rejected,cancelled,completed"`
	PaymentStatus  string       `json:"payment_status" example:"unpaid" enums:"unpaid,submitted,paid,refunded"`
	TourDate       string       `json:"tour_date" example:"2026-12-20"`
	StartTime      *string      `json:"start_time,omitempty" example:"08:00"`
	GroupSize      int          `json:"group_size" example:"4"`
	PricingType    string       `json:"pricing_type" example:"per_group"`
	UnitPrice      int          `json:"unit_price" example:"6000"`
	TotalAmount    int          `json:"total_amount" example:"6000"`
	Currency       string       `json:"currency" example:"BDT"`
	Note           *string      `json:"note,omitempty"`
	StatusReason   *string      `json:"status_reason,omitempty"`
	Gig            *BookingGig  `json:"gig,omitempty"`
	Spot           *BookingSpot `json:"spot,omitempty"`
	Guide          *PersonRef   `json:"guide,omitempty"`
	Tourist        *PersonRef   `json:"tourist,omitempty"`
	GuideContact   *Contact     `json:"guide_contact,omitempty"`   // visible to the tourist once accepted
	TouristContact *Contact     `json:"tourist_contact,omitempty"` // visible to the guide once accepted
	// Actions the current user may take — drive the buttons from these.
	CanAccept   bool       `json:"can_accept" example:"false"`
	CanReject   bool       `json:"can_reject" example:"false"`
	CanCancel   bool       `json:"can_cancel" example:"true"`
	CanComplete bool       `json:"can_complete" example:"false"`
	CanPay      bool       `json:"can_pay" example:"false"`
	CanReview   bool       `json:"can_review" example:"false"`
	HasReview   bool       `json:"has_review" example:"false"`
	AcceptedAt  *time.Time `json:"accepted_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CancelledAt *time.Time `json:"cancelled_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// BookingPage documents response.Page[BookingResponse] for swagger.
type BookingPage struct {
	Items []BookingResponse `json:"items"`
	Meta  response.PageMeta `json:"meta"`
}

type PaymentMethodResponse struct {
	ID            string  `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	Provider      string  `json:"provider" example:"bkash" enums:"bkash,nagad,rocket,upay,bank"`
	AccountName   string  `json:"account_name" example:"CholoBangladesh"`
	AccountNumber string  `json:"account_number" example:"01700000000"`
	AccountType   string  `json:"account_type" example:"personal"`
	Instructions  *string `json:"instructions,omitempty"`
	IsActive      bool    `json:"is_active" example:"true"`
	Position      int     `json:"position" example:"0"`
}

type PaymentResponse struct {
	ID            string                 `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	Purpose       string                 `json:"purpose" example:"booking"`
	BookingID     *string                `json:"booking_id,omitempty"`
	BookingCode   string                 `json:"booking_code,omitempty" example:"CB7K3D9QXA"`
	GigTitle      string                 `json:"gig_title,omitempty"`
	Guide         *PersonRef             `json:"guide,omitempty"`
	Payer         *PersonRef             `json:"payer,omitempty"`
	Method        *PaymentMethodResponse `json:"method,omitempty"`
	Provider      string                 `json:"provider" example:"bkash"`
	SenderNumber  string                 `json:"sender_number" example:"+8801812345678"`
	TransactionID string                 `json:"transaction_id" example:"8N7A6B5C4D"`
	Amount        int                    `json:"amount" example:"6000"`
	Currency      string                 `json:"currency" example:"BDT"`
	Status        string                 `json:"status" example:"pending" enums:"pending,verified,rejected"`
	AdminNote     *string                `json:"admin_note,omitempty"`
	ReviewedAt    *time.Time             `json:"reviewed_at,omitempty"`
	CreatedAt     time.Time              `json:"created_at"`
}

// PaymentPage documents response.Page[PaymentResponse] for swagger.
type PaymentPage struct {
	Items []PaymentResponse `json:"items"`
	Meta  response.PageMeta `json:"meta"`
}
