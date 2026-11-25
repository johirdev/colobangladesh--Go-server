// Package dto holds the request/response contracts of the user module.
// These are exactly what the frontend sends and receives.
package dto

import "cholobangladesh-server/pkg/listing"

// ============================================================== registration

// EmailOTPRequest starts tourist sign-up (step 1).
type EmailOTPRequest struct {
	Email string `json:"email" binding:"required,email,max=150" example:"traveler@example.com"`
}

// EmailOTPVerifyRequest verifies the e-mailed code (step 2).
type EmailOTPVerifyRequest struct {
	Email string `json:"email" binding:"required,email,max=150" example:"traveler@example.com"`
	OTP   string `json:"otp" binding:"required,min=4,max=10,numeric_code" example:"482913"`
}

// PhoneOTPRequest starts guide sign-up (step 1). Bangladeshi mobile numbers only.
type PhoneOTPRequest struct {
	Phone string `json:"phone" binding:"required,bd_phone" example:"01712345678"`
}

// PhoneOTPVerifyRequest verifies the SMS code (step 2).
type PhoneOTPVerifyRequest struct {
	Phone string `json:"phone" binding:"required,bd_phone" example:"01712345678"`
	OTP   string `json:"otp" binding:"required,min=4,max=10,numeric_code" example:"482913"`
}

// CompleteUserRegistrationRequest creates the tourist account (step 3).
// Password is optional: without one the tourist signs in with e-mail OTP or Google/Apple.
type CompleteUserRegistrationRequest struct {
	RegistrationToken string  `json:"registration_token" binding:"required,min=20,max=200" example:"q4Zr8...registration-token"`
	FullName          string  `json:"full_name" binding:"required,min=2,max=100,person_name" example:"Emma Watson"`
	Password          *string `json:"password" binding:"omitempty,strong_password" example:"Travel2026!"`
	CountryCode       *string `json:"country_code" binding:"omitempty,country" example:"GB"`
	PreferredLanguage *string `json:"preferred_language" binding:"omitempty,lang" example:"en"`
}

// CompleteLocalUserRegistrationRequest creates a Bangladeshi tourist account (phone sign-up, step 3).
// Country is set to BD automatically. Password is optional (OTP login by SMS works without it).
type CompleteLocalUserRegistrationRequest struct {
	RegistrationToken string  `json:"registration_token" binding:"required,min=20,max=200" example:"q4Zr8...registration-token"`
	FullName          string  `json:"full_name" binding:"required,min=2,max=100,person_name" example:"Tanvir Ahmed"`
	Password          *string `json:"password" binding:"omitempty,strong_password" example:"Travel2026!"`
	City              *string `json:"city" binding:"omitempty,max=100" example:"Chattogram"`
	PreferredLanguage *string `json:"preferred_language" binding:"omitempty,lang" example:"bn"`
}

// CompleteGuideRegistrationRequest creates the guide account (step 3). Password is required.
type CompleteGuideRegistrationRequest struct {
	RegistrationToken string `json:"registration_token" binding:"required,min=20,max=200" example:"q4Zr8...registration-token"`
	FullName          string `json:"full_name" binding:"required,min=2,max=100,person_name" example:"Rahim Uddin"`
	Password          string `json:"password" binding:"required,strong_password" example:"Guide2026!"`
}

// ============================================================== login

// LoginRequest — identifier is an e-mail address or a Bangladeshi mobile number.
type LoginRequest struct {
	Identifier string `json:"identifier" binding:"required,min=3,max=150" example:"traveler@example.com"`
	Password   string `json:"password" binding:"required,min=1,max=72" example:"Travel2026!"`
}

// IdentifierRequest — e-mail or BD mobile number.
type IdentifierRequest struct {
	Identifier string `json:"identifier" binding:"required,min=3,max=150" example:"01712345678"`
}

type IdentifierOTPRequest struct {
	Identifier string `json:"identifier" binding:"required,min=3,max=150" example:"01712345678"`
	OTP        string `json:"otp" binding:"required,min=4,max=10,numeric_code" example:"482913"`
}

// OAuthRequest — id_token from Google Identity Services / Sign in with Apple (web, Android or iOS).
type OAuthRequest struct {
	IDToken string `json:"id_token" binding:"required,min=20,max=8192" example:"eyJhbGciOiJSUzI1NiIs..."`
	// Optional raw nonce you passed to the provider (replay protection).
	Nonce string `json:"nonce" binding:"omitempty,max=256" example:"n-0S6_WzA2Mj"`
	// Apple only sends the user's name to the client on the very first sign-in; forward it here.
	FullName string `json:"full_name" binding:"omitempty,max=100" example:"Emma Watson"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required,min=20,max=200" example:"mJ0p...refresh-token"`
}

// ============================================================== password

type ResetPasswordRequest struct {
	ResetToken  string `json:"reset_token" binding:"required,min=20,max=200" example:"Yb7...reset-token"`
	NewPassword string `json:"new_password" binding:"required,strong_password" example:"NewPass2026!"`
}

// ChangePasswordRequest — current_password is required if the account already has a password
// (OAuth-only accounts can set their first password without it).
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"omitempty,max=72" example:"OldPass2026!"`
	NewPassword     string `json:"new_password" binding:"required,strong_password" example:"NewPass2026!"`
}

// ============================================================== profile

// UpdateProfileRequest — PATCH semantics: only fields present are changed.
// Send an empty string to clear an optional text field.
type UpdateProfileRequest struct {
	FullName          *string `json:"full_name" binding:"omitempty,min=2,max=100,person_name" example:"Emma Watson"`
	Bio               *string `json:"bio" binding:"omitempty,max=500" example:"Backpacker who loves rivers and street food."`
	Gender            *string `json:"gender" binding:"omitempty,oneof=male female other prefer_not_to_say" example:"female"`
	DateOfBirth       *string `json:"date_of_birth" binding:"omitempty,datetime=2006-01-02" example:"1995-04-21"`
	CountryCode       *string `json:"country_code" binding:"omitempty,country" example:"GB"`
	City              *string `json:"city" binding:"omitempty,max=100" example:"London"`
	PreferredLanguage *string `json:"preferred_language" binding:"omitempty,lang" example:"en"`
	WhatsappNumber    *string `json:"whatsapp_number" binding:"omitempty,intl_phone" example:"+447911123456"`
}

type DeleteAccountRequest struct {
	Confirm  string `json:"confirm" binding:"required,eq=DELETE" example:"DELETE"`
	Password string `json:"password" binding:"omitempty,max=72" example:"Travel2026!"`
}

// ============================================================== guide

// UpsertGuideProfileRequest — full replace of the guide's marketplace profile.
type UpsertGuideProfileRequest struct {
	Headline        string   `json:"headline" binding:"required,min=10,max=120" example:"Licensed Sundarbans & Old Dhaka guide"`
	About           string   `json:"about" binding:"required,min=50,max=2000" example:"I have been guiding travellers through the Sundarbans mangrove forest for 8 years..."`
	Languages       []string `json:"languages" binding:"required,min=1,max=10,unique,dive,lang" example:"bn,en,hi"`
	ServiceAreas    []string `json:"service_areas" binding:"required,min=1,max=20,unique,dive,min=2,max=60,slug" example:"dhaka,sundarbans,khulna"`
	Specialties     []string `json:"specialties" binding:"omitempty,max=15,unique,dive,oneof=history_culture food nature_wildlife adventure_trekking photography religious_heritage city_tour river_cruise tea_garden beach tribal_culture shopping" example:"nature_wildlife,river_cruise"`
	ExperienceYears int      `json:"experience_years" binding:"gte=0,lte=60" example:"8"`
	HourlyRate      *int     `json:"hourly_rate" binding:"omitempty,gte=0,lte=100000" example:"800"`
	DailyRate       *int     `json:"daily_rate" binding:"omitempty,gte=0,lte=500000" example:"5000"`
	MaxGroupSize    int      `json:"max_group_size" binding:"required,gte=1,lte=100" example:"8"`
	LicenseNumber   *string  `json:"license_number" binding:"omitempty,max=50" example:"BTB-2021-0457"`
}

type AvailabilityRequest struct {
	IsAvailable *bool `json:"is_available" binding:"required" example:"true"`
}

// GuideSearchQuery — public guide discovery filters.
// GuideSearchQuery — ?page ?limit (max 50) ?q (name, headline)
// ?sort=rating|experience|price_low|price_high|newest (default rating).
type GuideSearchQuery struct {
	listing.Params
	Area      string `form:"area" binding:"omitempty,max=60,slug" example:"sylhet"`
	Language  string `form:"language" binding:"omitempty,lang" example:"en"`
	Specialty string `form:"specialty" binding:"omitempty,max=40,slug" example:"tea_garden"`
	MinRate   *int   `form:"min_rate" binding:"omitempty,gte=0" example:"300"`
	MaxRate   *int   `form:"max_rate" binding:"omitempty,gte=0" example:"2000"`
}

// ============================================================== admin

// AdminUserQuery — ?page ?limit ?q (email, phone, name) ?sort=newest|oldest|name|login.
type AdminUserQuery struct {
	listing.Params
	Role   string `form:"role" binding:"omitempty,oneof=user guide admin moderator" example:"guide"`
	Status string `form:"status" binding:"omitempty,oneof=active suspended deleted" example:"active"`
}

type UpdateUserStatusRequest struct {
	Status string `json:"status" binding:"required,oneof=active suspended" example:"suspended"`
	Reason string `json:"reason" binding:"omitempty,max=500" example:"Multiple fraud reports"`
}

// VerificationQuery — ?page ?limit ?q (guide name, phone, email) ?sort=oldest|newest (default oldest).
type VerificationQuery struct {
	listing.Params
	Status string `form:"status" binding:"omitempty,oneof=pending approved rejected" example:"pending"`
}

type RejectVerificationRequest struct {
	Reason string `json:"reason" binding:"required,min=5,max=500" example:"NID photo is blurry, please upload a clearer image"`
}

// UpdateUserRoleRequest promotes a tourist account to moderator (or back).
// Guide and admin roles cannot be assigned here.
type UpdateUserRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=user moderator" example:"moderator"`
}
