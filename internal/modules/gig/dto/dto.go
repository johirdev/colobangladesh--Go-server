// Package dto holds the request/response contracts of the gig module.
package dto

import (
	"time"

	"cholobangladesh-server/pkg/response"
)

// ================================================================== requests

// CreateGigRequest — one gig per tourist spot per guide. `price` is whole taka (BDT).
type CreateGigRequest struct {
	SpotID             string   `json:"spot_id" binding:"required,uuid" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	Title              string   `json:"title" binding:"required,min=10,max=150" example:"Full-day Sundarbans boat safari with a licensed guide"`
	Description        string   `json:"description" binding:"required,min=50,max=10000" example:"We start at Mongla port at 7am and cruise through the tidal creeks..."`
	Price              int      `json:"price" binding:"required,gte=100,lte=1000000" example:"6000"`
	PricingType        string   `json:"pricing_type" binding:"required,oneof=per_person per_group" example:"per_group"`
	DurationHours      float64  `json:"duration_hours" binding:"required,gt=0,lte=720" example:"8"`
	MaxGroupSize       int      `json:"max_group_size" binding:"required,gte=1,lte=100" example:"10"`
	Languages          []string `json:"languages" binding:"required,min=1,max=10,unique,dive,lang" example:"bn,en"`
	Includes           []string `json:"includes" binding:"omitempty,max=20,dive,min=2,max=150" example:"Boat ride,Lunch,Forest permit"`
	Excludes           []string `json:"excludes" binding:"omitempty,max=20,dive,min=2,max=150" example:"Hotel,Transport to Mongla"`
	AvailableDays      []string `json:"available_days" binding:"omitempty,max=7,unique,dive,oneof=sat sun mon tue wed thu fri" example:"fri,sat"`
	MeetingPoint       *string  `json:"meeting_point" binding:"omitempty,max=300" example:"Mongla ferry ghat"`
	CancellationPolicy *string  `json:"cancellation_policy" binding:"omitempty,max=1000" example:"Free cancellation up to 48 hours before the tour."`
	CoverImageURL      *string  `json:"cover_image_url" binding:"omitempty,url,max=1000" example:""`
	Status             string   `json:"status" binding:"omitempty,oneof=draft active paused" example:"active"`
}

// UpdateGigRequest is a partial update. The tourist spot cannot be changed (create a new gig).
type UpdateGigRequest struct {
	Title              *string   `json:"title" binding:"omitempty,min=10,max=150"`
	Description        *string   `json:"description" binding:"omitempty,min=50,max=10000"`
	Price              *int      `json:"price" binding:"omitempty,gte=100,lte=1000000" example:"6500"`
	PricingType        *string   `json:"pricing_type" binding:"omitempty,oneof=per_person per_group" example:"per_person"`
	DurationHours      *float64  `json:"duration_hours" binding:"omitempty,gt=0,lte=720" example:"6"`
	MaxGroupSize       *int      `json:"max_group_size" binding:"omitempty,gte=1,lte=100" example:"8"`
	Languages          *[]string `json:"languages" binding:"omitempty,min=1,max=10,unique,dive,lang" example:"bn,en"`
	Includes           *[]string `json:"includes" binding:"omitempty,max=20,dive,min=2,max=150"`
	Excludes           *[]string `json:"excludes" binding:"omitempty,max=20,dive,min=2,max=150"`
	AvailableDays      *[]string `json:"available_days" binding:"omitempty,max=7,unique,dive,oneof=sat sun mon tue wed thu fri"`
	MeetingPoint       *string   `json:"meeting_point" binding:"omitempty,max=300"`
	CancellationPolicy *string   `json:"cancellation_policy" binding:"omitempty,max=1000"`
	CoverImageURL      *string   `json:"cover_image_url" binding:"omitempty,max=1000"`
	Status             *string   `json:"status" binding:"omitempty,oneof=draft active paused" example:"paused"`
}

type GigQuery struct {
	SpotID     string `form:"spot_id" binding:"omitempty,uuid" example:""`
	GuideID    string `form:"guide_id" binding:"omitempty,uuid" example:""`
	DistrictID int    `form:"district_id" binding:"omitempty,gte=1" example:"28"`
	DivisionID int    `form:"division_id" binding:"omitempty,gte=1" example:"3"`
	Language   string `form:"language" binding:"omitempty,lang" example:"en"`
	MinPrice   *int   `form:"min_price" binding:"omitempty,gte=0" example:"500"`
	MaxPrice   *int   `form:"max_price" binding:"omitempty,gte=0" example:"10000"`
	Q          string `form:"q" binding:"omitempty,max=100" example:"boat"`
	Sort       string `form:"sort" binding:"omitempty,oneof=rating price_low price_high newest popular" example:"rating"`
	Page       int    `form:"page" binding:"omitempty,gte=1" example:"1"`
	Limit      int    `form:"limit" binding:"omitempty,gte=1,lte=100" example:"20"`
}

type AdminGigQuery struct {
	GigQuery
	Status string `form:"status" binding:"omitempty,oneof=draft active paused suspended" example:"active"`
}

type AdminGigStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=active suspended" example:"suspended"`
	Reason string `json:"reason" binding:"required_if=Status suspended,max=500" example:"Misleading description"`
}

// SetQuotaRequest grants extra gig slots on top of FREE_GIG_LIMIT
// (until the paid slot purchase flow exists, admins grant them after payment).
type SetQuotaRequest struct {
	ExtraSlots int    `json:"extra_slots" binding:"gte=0,lte=500" example:"3"`
	Note       string `json:"note" binding:"omitempty,max=500" example:"Paid 1500 BDT via bKash TX 8N7A6B5C"`
}

// ================================================================== responses

type SpotRef struct {
	ID            string  `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	Slug          string  `json:"slug" example:"sundarbans"`
	Title         string  `json:"title" example:"Sundarbans"`
	BnTitle       *string `json:"bn_title,omitempty" example:"সুন্দরবন"`
	CoverImageURL *string `json:"cover_image_url,omitempty"`
	LocationName  string  `json:"location_name" example:"Mongla (Bagerhat) & Shyamnagar (Satkhira)"`
	DistrictID    int     `json:"district_id" example:"28"`
	DistrictName  string  `json:"district_name" example:"Bagerhat"`
	Published     bool    `json:"published" example:"true"`
}

type GuideRef struct {
	ID              string   `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	FullName        string   `json:"full_name" example:"Rahim Uddin"`
	AvatarURL       *string  `json:"avatar_url,omitempty"`
	Headline        *string  `json:"headline,omitempty" example:"Licensed Sundarbans mangrove & wildlife guide"`
	Languages       []string `json:"languages" example:"bn,en"`
	ExperienceYears int      `json:"experience_years" example:"10"`
	Verified        bool     `json:"verified" example:"true"`
	RatingAvg       float64  `json:"rating_avg" example:"4.9"`
	RatingCount     int      `json:"rating_count" example:"128"`
	TotalTours      int      `json:"total_tours" example:"310"`
}

type GigResponse struct {
	ID                 string    `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	Title              string    `json:"title" example:"Full-day Sundarbans boat safari"`
	Description        string    `json:"description"`
	Price              int       `json:"price" example:"6000"`
	Currency           string    `json:"currency" example:"BDT"`
	PricingType        string    `json:"pricing_type" example:"per_group" enums:"per_person,per_group"`
	DurationHours      float64   `json:"duration_hours" example:"8"`
	MaxGroupSize       int       `json:"max_group_size" example:"10"`
	Languages          []string  `json:"languages" example:"bn,en"`
	Includes           []string  `json:"includes"`
	Excludes           []string  `json:"excludes"`
	AvailableDays      []string  `json:"available_days" example:"fri,sat"`
	MeetingPoint       *string   `json:"meeting_point,omitempty"`
	CancellationPolicy *string   `json:"cancellation_policy,omitempty"`
	CoverImageURL      *string   `json:"cover_image_url,omitempty"` // gig cover, falls back to the spot cover
	Status             string    `json:"status" example:"active" enums:"draft,active,paused,suspended"`
	SuspendedReason    *string   `json:"suspended_reason,omitempty"`
	RatingAvg          float64   `json:"rating_avg" example:"4.8"`
	RatingCount        int       `json:"rating_count" example:"12"`
	BookingCount       int       `json:"booking_count" example:"20"`
	CompletedCount     int       `json:"completed_count" example:"15"`
	Spot               *SpotRef  `json:"spot,omitempty"`
	Guide              *GuideRef `json:"guide,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// GigPage documents response.Page[GigResponse] for swagger.
type GigPage struct {
	Items []GigResponse     `json:"items"`
	Meta  response.PageMeta `json:"meta"`
}

// QuotaResponse tells the guide dashboard how many gigs are left.
type QuotaResponse struct {
	FreeLimit       int  `json:"free_limit" example:"5"`
	ExtraSlots      int  `json:"extra_slots" example:"0"`
	TotalLimit      int  `json:"total_limit" example:"5"`
	Used            int  `json:"used" example:"5"`
	Remaining       int  `json:"remaining" example:"0"`
	CanCreate       bool `json:"can_create" example:"false"`
	UpgradeRequired bool `json:"upgrade_required" example:"true"` // show "buy more slots" (payment comes later)
}

type MyGigsResponse struct {
	Quota QuotaResponse `json:"quota"`
	Items []GigResponse `json:"items"`
}
