package dto

import "time"

// OTPSentResponse is returned whenever a code is sent.
type OTPSentResponse struct {
	Channel     string `json:"channel" example:"email" enums:"email,sms"`
	Destination string `json:"destination" example:"tr*****@example.com"` // masked
	ExpiresIn   int    `json:"expires_in_seconds" example:"300"`
	ResendAfter int    `json:"resend_after_seconds" example:"60"`
	// DevOTP is present ONLY in development when DEV_EXPOSE_OTP=true. Never in production.
	DevOTP string `json:"dev_otp,omitempty" example:"123456"`
}

// VerificationTicketResponse proves a contact was verified; pass it to the next step.
type VerificationTicketResponse struct {
	Token     string `json:"token" example:"q4Zr8...registration-token"`
	ExpiresIn int    `json:"expires_in_seconds" example:"1800"`
}

// AuthResponse is returned by every successful login / registration / refresh.
type AuthResponse struct {
	AccessToken      string      `json:"access_token" example:"eyJhbGciOiJIUzI1NiIs..."`
	RefreshToken     string      `json:"refresh_token" example:"mJ0p...refresh-token"`
	TokenType        string      `json:"token_type" example:"Bearer"`
	ExpiresIn        int         `json:"expires_in_seconds" example:"900"`
	RefreshExpiresIn int         `json:"refresh_expires_in_seconds" example:"2592000"`
	IsNewUser        bool        `json:"is_new_user" example:"false"`
	User             *MeResponse `json:"user"`
}

// Onboarding tells the frontend which screen to show next.
//
// next_step values:
//   - complete_profile       -> show profile form (see missing_fields)
//   - submit_verification    -> guide: upload NID + selfie
//   - awaiting_review        -> guide: KYC submitted, waiting for admin
//   - resubmit_verification  -> guide: KYC rejected (see guide_verification.rejection_reason)
//   - done                   -> fully onboarded
type Onboarding struct {
	Completed     bool     `json:"completed" example:"false"`
	NextStep      string   `json:"next_step" example:"complete_profile" enums:"complete_profile,submit_verification,awaiting_review,resubmit_verification,done"`
	MissingFields []string `json:"missing_fields" example:"country_code"`
}

type ProfileResponse struct {
	FullName          string  `json:"full_name" example:"Emma Watson"`
	AvatarURL         *string `json:"avatar_url" example:"https://res.cloudinary.com/demo/image/upload/v1/avatar.jpg"`
	Bio               *string `json:"bio" example:"Backpacker who loves rivers."`
	Gender            *string `json:"gender" example:"female"`
	DateOfBirth       *string `json:"date_of_birth" example:"1995-04-21"`
	CountryCode       *string `json:"country_code" example:"GB"`
	City              *string `json:"city" example:"London"`
	PreferredLanguage string  `json:"preferred_language" example:"en"`
	WhatsappNumber    *string `json:"whatsapp_number" example:"+447911123456"`
}

type GuideProfileResponse struct {
	Headline           *string    `json:"headline" example:"Licensed Sundarbans & Old Dhaka guide"`
	About              *string    `json:"about" example:"I have been guiding travellers..."`
	Languages          []string   `json:"languages" example:"bn,en"`
	ServiceAreas       []string   `json:"service_areas" example:"dhaka,sundarbans"`
	Specialties        []string   `json:"specialties" example:"nature_wildlife,river_cruise"`
	ExperienceYears    int        `json:"experience_years" example:"8"`
	HourlyRate         *int       `json:"hourly_rate" example:"800"`
	DailyRate          *int       `json:"daily_rate" example:"5000"`
	Currency           string     `json:"currency" example:"BDT"`
	MaxGroupSize       int        `json:"max_group_size" example:"8"`
	LicenseNumber      *string    `json:"license_number,omitempty" example:"BTB-2021-0457"`
	IsAvailable        bool       `json:"is_available" example:"true"`
	VerificationStatus string     `json:"verification_status" example:"approved" enums:"unverified,pending,approved,rejected"`
	VerifiedAt         *time.Time `json:"verified_at"`
	RatingAvg          float64    `json:"rating_avg" example:"4.8"`
	RatingCount        int        `json:"rating_count" example:"37"`
	TotalTours         int        `json:"total_tours" example:"52"`
}

// MeResponse is the full private view of the logged-in account.
type MeResponse struct {
	ID              string                      `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	Role            string                      `json:"role" example:"user" enums:"user,guide,admin"`
	Status          string                      `json:"status" example:"active" enums:"active,suspended,deleted"`
	Email           *string                     `json:"email" example:"traveler@example.com"`
	EmailVerified   bool                        `json:"email_verified" example:"true"`
	Phone           *string                     `json:"phone" example:"+8801712345678"`
	PhoneVerified   bool                        `json:"phone_verified" example:"false"`
	HasPassword     bool                        `json:"has_password" example:"true"`
	LinkedProviders []string                    `json:"linked_providers" example:"google"`
	Profile         ProfileResponse             `json:"profile"`
	GuideProfile    *GuideProfileResponse       `json:"guide_profile,omitempty"`
	Verification    *VerificationStatusResponse `json:"guide_verification,omitempty"`
	Onboarding      Onboarding                  `json:"onboarding"`
	LastLoginAt     *time.Time                  `json:"last_login_at"`
	CreatedAt       time.Time                   `json:"created_at"`
}

// PublicGuideResponse is what tourists see. Contact details are never exposed —
// tourists reach guides through bookings.
type PublicGuideResponse struct {
	ID              string     `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
	FullName        string     `json:"full_name" example:"Rahim Uddin"`
	AvatarURL       *string    `json:"avatar_url" example:"https://res.cloudinary.com/demo/image/upload/v1/avatar.jpg"`
	City            *string    `json:"city" example:"Khulna"`
	Headline        *string    `json:"headline" example:"Licensed Sundarbans & Old Dhaka guide"`
	About           *string    `json:"about,omitempty" example:"I have been guiding travellers..."`
	Languages       []string   `json:"languages" example:"bn,en"`
	ServiceAreas    []string   `json:"service_areas" example:"dhaka,sundarbans"`
	Specialties     []string   `json:"specialties" example:"nature_wildlife"`
	ExperienceYears int        `json:"experience_years" example:"8"`
	HourlyRate      *int       `json:"hourly_rate" example:"800"`
	DailyRate       *int       `json:"daily_rate" example:"5000"`
	Currency        string     `json:"currency" example:"BDT"`
	MaxGroupSize    int        `json:"max_group_size" example:"8"`
	RatingAvg       float64    `json:"rating_avg" example:"4.8"`
	RatingCount     int        `json:"rating_count" example:"37"`
	TotalTours      int        `json:"total_tours" example:"52"`
	IsVerified      bool       `json:"is_verified" example:"true"`
	IsLicensed      bool       `json:"is_licensed" example:"true"`
	VerifiedAt      *time.Time `json:"verified_at"`
	MemberSince     time.Time  `json:"member_since"`
}

type VerificationStatusResponse struct {
	ID              string     `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d11"`
	Status          string     `json:"status" example:"rejected" enums:"pending,approved,rejected"`
	NIDLast4        string     `json:"nid_last4" example:"4821"`
	RejectionReason *string    `json:"rejection_reason" example:"NID photo is blurry"`
	SubmittedAt     time.Time  `json:"submitted_at"`
	ReviewedAt      *time.Time `json:"reviewed_at"`
}

type SessionResponse struct {
	ID         string    `json:"id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d12"`
	UserAgent  string    `json:"user_agent" example:"Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X)"`
	IPAddress  string    `json:"ip_address" example:"103.4.145.2"`
	Current    bool      `json:"current" example:"true"`
	CreatedAt  time.Time `json:"created_at"`
	LastUsedAt time.Time `json:"last_used_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

type GuideOptionsResponse struct {
	Specialties []string `json:"specialties" example:"history_culture,food,nature_wildlife"`
	SortOptions []string `json:"sort_options" example:"rating,experience,price_low,price_high,newest"`
	Currency    string   `json:"currency" example:"BDT"`
}

// ------------------------------------------------------------------ admin

type AdminUserResponse struct {
	ID                 string     `json:"id"`
	Role               string     `json:"role" example:"guide"`
	Status             string     `json:"status" example:"active"`
	FullName           string     `json:"full_name" example:"Rahim Uddin"`
	Email              *string    `json:"email"`
	EmailVerified      bool       `json:"email_verified"`
	Phone              *string    `json:"phone" example:"+8801712345678"`
	PhoneVerified      bool       `json:"phone_verified"`
	VerificationStatus *string    `json:"verification_status,omitempty" example:"pending"`
	LastLoginAt        *time.Time `json:"last_login_at"`
	CreatedAt          time.Time  `json:"created_at"`
}

type AdminVerificationResponse struct {
	ID              string     `json:"id"`
	UserID          string     `json:"user_id"`
	FullName        string     `json:"full_name" example:"Rahim Uddin"`
	Phone           *string    `json:"phone" example:"+8801712345678"`
	Status          string     `json:"status" example:"pending"`
	NIDLast4        string     `json:"nid_last4" example:"4821"`
	RejectionReason *string    `json:"rejection_reason"`
	SubmittedAt     time.Time  `json:"submitted_at"`
	ReviewedAt      *time.Time `json:"reviewed_at"`
	ReviewedBy      *string    `json:"reviewed_by"`
}

// AdminVerificationDetailResponse includes the decrypted NID and signed document URLs
// (valid for 10 minutes; do not cache or share them).
type AdminVerificationDetailResponse struct {
	AdminVerificationResponse
	NIDNumber    string                `json:"nid_number" example:"19901234567894821"`
	NIDFrontURL  string                `json:"nid_front_url"`
	NIDBackURL   string                `json:"nid_back_url"`
	SelfieURL    string                `json:"selfie_url"`
	AvatarURL    *string               `json:"avatar_url"`
	DateOfBirth  *string               `json:"date_of_birth" example:"1990-01-15"`
	GuideProfile *GuideProfileResponse `json:"guide_profile"`
}

// ------------------------------------------------------------------ paginated lists (swagger shapes)

type PageMeta struct {
	Page       int   `json:"page" example:"1"`
	Limit      int   `json:"limit" example:"20"`
	Total      int64 `json:"total" example:"57"`
	TotalPages int   `json:"total_pages" example:"3"`
}

type GuidePage struct {
	Items []PublicGuideResponse `json:"items"`
	Meta  PageMeta              `json:"meta"`
}

type AdminUserPage struct {
	Items []AdminUserResponse `json:"items"`
	Meta  PageMeta            `json:"meta"`
}

type AdminVerificationPage struct {
	Items []AdminVerificationResponse `json:"items"`
	Meta  PageMeta                    `json:"meta"`
}
