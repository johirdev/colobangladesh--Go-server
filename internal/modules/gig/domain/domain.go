// Package domain holds the gig module's entities, errors and repository port.
// A gig is a verified guide's tour offer for one tourist spot (Fiverr/Upwork-style).
package domain

import (
	"context"
	"net/http"
	"time"

	"cholobangladesh-server/pkg/apperror"
	"cholobangladesh-server/pkg/listing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

const (
	StatusDraft     = "draft"
	StatusActive    = "active"
	StatusPaused    = "paused"
	StatusSuspended = "suspended" // set by admin only

	PricingPerPerson = "per_person"
	PricingPerGroup  = "per_group"
)

type Gig struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey"`
	GuideID            uuid.UUID `gorm:"type:uuid"`
	SpotID             uuid.UUID `gorm:"type:uuid"`
	Title              string
	Description        string
	Price              int
	Currency           string
	PricingType        string
	DurationHours      float64
	MaxGroupSize       int
	Languages          pq.StringArray `gorm:"type:text[]"`
	Includes           pq.StringArray `gorm:"type:text[]"`
	Excludes           pq.StringArray `gorm:"type:text[]"`
	AvailableDays      pq.StringArray `gorm:"type:text[]"`
	MeetingPoint       *string
	CancellationPolicy *string
	CoverImageURL      *string `gorm:"column:cover_image_url"`
	Status             string
	SuspendedReason    *string
	RatingAvg          float64
	RatingCount        int
	BookingCount       int
	CompletedCount     int
	CreatedAt          time.Time
	UpdatedAt          time.Time
	DeletedAt          gorm.DeletedAt

	Spot    *SpotRef    `gorm:"foreignKey:SpotID"`
	Profile *ProfileRef `gorm:"foreignKey:GuideID;references:UserID"`
	Guide   *GuideRef   `gorm:"foreignKey:GuideID;references:UserID"`
	Images  []Image     `gorm:"foreignKey:GigID"`
}

func (Gig) TableName() string { return "guide_gigs" }

// MaxImages per gig. The first image (lowest position) is the cover.
const MaxImages = 12

// Image is one photo of a gig gallery (table guide_gig_images).
type Image struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	GigID     uuid.UUID `gorm:"type:uuid"`
	URL       string    `gorm:"column:url"`
	PublicID  *string   // Cloudinary public ID (nil for old external URLs)
	Caption   *string
	Position  int
	CreatedAt time.Time
}

func (Image) TableName() string { return "guide_gig_images" }

// FileStorage stores uploaded gig photos (Cloudinary).
type FileStorage interface {
	UploadPublic(ctx context.Context, data []byte, path, publicID string) (url, id string, err error)
	Delete(ctx context.Context, publicID string) error
}

// SpotRef is a read-only view of tourist_spots.
type SpotRef struct {
	ID            uuid.UUID `gorm:"type:uuid;primaryKey"`
	Slug          string
	Title         string
	BnTitle       *string
	CoverImageURL *string `gorm:"column:cover_image_url"`
	LocationName  string
	DistrictID    int
	DivisionID    int
	Status        string
	DeletedAt     gorm.DeletedAt

	District *DistrictRef `gorm:"foreignKey:DistrictID"`
}

func (SpotRef) TableName() string { return "tourist_spots" }

type DistrictRef struct {
	ID     int
	Name   string
	BnName string
	Slug   string
}

func (DistrictRef) TableName() string { return "districts" }

// ProfileRef is a read-only view of user_profiles (guide name/avatar).
type ProfileRef struct {
	UserID    uuid.UUID `gorm:"type:uuid;primaryKey"`
	FullName  string
	AvatarURL *string
}

func (ProfileRef) TableName() string { return "user_profiles" }

// GuideRef is a read-only view of guide_profiles.
type GuideRef struct {
	UserID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	Headline           *string
	Languages          pq.StringArray `gorm:"type:text[]"`
	ExperienceYears    int
	VerificationStatus string
	IsAvailable        bool
	RatingAvg          float64
	RatingCount        int
	TotalTours         int
}

func (GuideRef) TableName() string { return "guide_profiles" }

// Eligibility is what decides whether a user may sell gigs.
type Eligibility struct {
	Role               string
	Status             string
	VerificationStatus string
}

type Quota struct {
	GuideID    uuid.UUID `gorm:"type:uuid;primaryKey"`
	ExtraSlots int
	Note       *string
	UpdatedBy  *uuid.UUID `gorm:"type:uuid"`
	UpdatedAt  time.Time
}

func (Quota) TableName() string { return "guide_gig_quotas" }

type Filter struct {
	GuideID    uuid.UUID
	SpotID     uuid.UUID
	DistrictID int
	DivisionID int
	Language   string
	MinPrice   *int
	MaxPrice   *int
	Status     string // admin listing only
	listing.Params
}

type Repository interface {
	Eligibility(ctx context.Context, guideID uuid.UUID) (*Eligibility, error)
	Spot(ctx context.Context, spotID uuid.UUID) (*SpotRef, error)
	CountGigs(ctx context.Context, guideID uuid.UUID) (int64, error)
	Quota(ctx context.Context, guideID uuid.UUID) (*Quota, error) // zero quota when absent
	SetQuota(ctx context.Context, q *Quota) error

	// CreateWithinLimit inserts the gig only if the guide has fewer than `limit` gigs
	// (serialised per guide with an advisory lock). Returns false when the limit is reached.
	CreateWithinLimit(ctx context.Context, g *Gig, limit int) (bool, error)
	Update(ctx context.Context, id uuid.UUID, fields map[string]any) error
	SoftDelete(ctx context.Context, id uuid.UUID) error
	Get(ctx context.Context, id uuid.UUID) (*Gig, error)
	// GetPublic returns the gig only when it is bookable (active, guide verified & active, spot published).
	GetPublic(ctx context.Context, id uuid.UUID) (*Gig, error)
	ListByGuide(ctx context.Context, guideID uuid.UUID) ([]Gig, error)
	ListPublic(ctx context.Context, f Filter) (listing.Result[Gig], error)
	ListAll(ctx context.Context, f Filter) (listing.Result[Gig], error)
	// RefreshSpotGigCount recomputes tourist_spots.gig_count (active gigs only).
	RefreshSpotGigCount(ctx context.Context, spotID uuid.UUID) error

	// Gallery — every write also re-syncs guide_gigs.cover_image_url to the first image.
	Images(ctx context.Context, gigID uuid.UUID) ([]Image, error)
	AddImages(ctx context.Context, gigID uuid.UUID, imgs []Image) error
	UpdateImage(ctx context.Context, gigID, imageID uuid.UUID, fields map[string]any) error
	MakeCover(ctx context.Context, gigID, imageID uuid.UUID) error
	DeleteImage(ctx context.Context, gigID, imageID uuid.UUID) (*Image, error)
}

var (
	ErrGigNotFound     = apperror.New(http.StatusNotFound, "GIG_NOT_FOUND", "gig not found")
	ErrGuideNotVerifed = apperror.New(http.StatusForbidden, "GUIDE_NOT_VERIFIED", "only verified guides can create gigs; complete identity verification first")
	ErrSpotNotFound    = apperror.New(http.StatusUnprocessableEntity, "SPOT_NOT_FOUND", "tourist spot not found or not published")
	ErrGigLimitReached = apperror.New(http.StatusPaymentRequired, "GIG_LIMIT_REACHED", "you have used all your gig slots; delete a gig or get extra slots")
	ErrDuplicateGig    = apperror.New(http.StatusConflict, "GIG_ALREADY_EXISTS", "you already have a gig for this tourist spot; edit it instead")
	ErrImageNotFound   = apperror.New(http.StatusNotFound, "IMAGE_NOT_FOUND", "photo not found")
	ErrTooManyImages   = apperror.New(http.StatusUnprocessableEntity, "TOO_MANY_IMAGES", "a gig can have at most 12 photos")
	ErrUploadsOff      = apperror.New(http.StatusServiceUnavailable, "UPLOAD_UNAVAILABLE", "photo uploads are temporarily unavailable")
	ErrGigSuspended    = apperror.New(http.StatusForbidden, "GIG_SUSPENDED", "this gig was suspended by an admin; contact support")
)
