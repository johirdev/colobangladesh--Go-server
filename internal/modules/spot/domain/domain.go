// Package domain holds the tourist spot module's entities, errors and ports.
package domain

import (
	"context"
	"net/http"
	"time"

	"cholobangladesh-server/pkg/apperror"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"
)

const (
	StatusDraft     = "draft"
	StatusPublished = "published"
	StatusArchived  = "archived"

	MaxImages   = 50 // per spot
	MaxVideos   = 20
	MaxUpazilas = 5 // a spot may span several upazilas/thanas (often 2)
)

// Categories are the allowed values of Spot.Category (filters + forms).
var Categories = []string{"beach", "island", "hill", "forest", "waterfall", "lake", "river", "wetland",
	"heritage", "religious", "park", "city", "other"}

type Spot struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	Slug            string
	Title           string
	BnTitle         *string
	Subtitle        *string
	Category        string
	LocationName    string
	Description     string
	Highlights      pq.StringArray `gorm:"type:text[]"`
	BestTimeToVisit *string
	GoogleMapURL    *string `gorm:"column:google_map_url"`
	Latitude        *float64
	Longitude       *float64
	DivisionID      int
	DistrictID      int
	CoverImageURL   *string `gorm:"column:cover_image_url"`
	SourceURL       *string `gorm:"column:source_url"`
	Status          string
	IsFeatured      bool
	RatingAvg       float64
	RatingCount     int
	GigCount        int
	CreatedBy       *uuid.UUID `gorm:"type:uuid"`
	UpdatedBy       *uuid.UUID `gorm:"type:uuid"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       gorm.DeletedAt

	Division *GeoRef       `gorm:"foreignKey:DivisionID"`
	District *DistrictRef  `gorm:"foreignKey:DistrictID"`
	Upazilas []SpotUpazila `gorm:"foreignKey:SpotID"`
	Images   []Image       `gorm:"foreignKey:SpotID"`
	Videos   []Video       `gorm:"foreignKey:SpotID"`
}

func (Spot) TableName() string { return "tourist_spots" }

// GeoRef is a read-only view of a division.
type GeoRef struct {
	ID     int
	Name   string
	BnName string
	Slug   string
}

func (GeoRef) TableName() string { return "divisions" }

type DistrictRef struct {
	ID         int
	DivisionID int
	Name       string
	BnName     string
	Slug       string
}

func (DistrictRef) TableName() string { return "districts" }

type UpazilaRef struct {
	ID         int
	DistrictID int
	Name       string
	BnName     string
	Slug       string
	Type       string
	Latitude   *float64
	Longitude  *float64

	District *DistrictRef `gorm:"foreignKey:DistrictID"`
}

func (UpazilaRef) TableName() string { return "upazilas" }

type SpotUpazila struct {
	SpotID    uuid.UUID `gorm:"type:uuid;primaryKey"`
	UpazilaID int       `gorm:"primaryKey"`
	Position  int

	Upazila *UpazilaRef `gorm:"foreignKey:UpazilaID"`
}

func (SpotUpazila) TableName() string { return "tourist_spot_upazilas" }

type Image struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	SpotID    uuid.UUID `gorm:"type:uuid"`
	URL       string    `gorm:"column:url"`
	PublicID  *string   // Cloudinary public ID (only for files uploaded through the API)
	Caption   *string
	SourceURL *string `gorm:"column:source_url"`
	Position  int
	CreatedAt time.Time
}

func (Image) TableName() string { return "tourist_spot_images" }

type Video struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	SpotID    uuid.UUID `gorm:"type:uuid"`
	YoutubeID string
	URL       string `gorm:"column:url"`
	Title     *string
	Position  int
	CreatedAt time.Time
}

func (Video) TableName() string { return "tourist_spot_videos" }

// Filter for listings. Public callers always get Status = published.
type Filter struct {
	Status     string
	DivisionID int
	DistrictID int
	UpazilaID  int
	Category   string
	Featured   *bool
	Query      string
	Sort       string // popular | rating | newest | title
	Page       int
	Limit      int
}

// Marker is the lightweight row used by the Bangladesh map.
type Marker struct {
	ID            uuid.UUID
	Slug          string
	Title         string
	BnTitle       *string
	Category      string
	CoverImageURL *string
	RatingAvg     float64
	RatingCount   int
	GigCount      int
	Latitude      *float64
	Longitude     *float64
	DistrictID    int
	DistrictSlug  string
	DistrictName  string
	DivisionID    int
}

// Children replaces the child rows of a spot when the pointer is non-nil.
type Children struct {
	UpazilaIDs *[]int
	Images     *[]Image
	Videos     *[]Video
}

type Repository interface {
	List(ctx context.Context, f Filter) ([]Spot, int64, error)
	Markers(ctx context.Context, divisionID, districtID int) ([]Marker, error)
	Get(ctx context.Context, id uuid.UUID, publishedOnly bool) (*Spot, error)
	GetBySlug(ctx context.Context, slug string, publishedOnly bool) (*Spot, error)
	SlugExists(ctx context.Context, slug string, except uuid.UUID) (bool, error)
	Create(ctx context.Context, s *Spot, c Children) error
	Update(ctx context.Context, id uuid.UUID, fields map[string]any, c Children) error
	SoftDelete(ctx context.Context, id uuid.UUID) error

	Images(ctx context.Context, spotID uuid.UUID) ([]Image, error)
	AddImages(ctx context.Context, spotID uuid.UUID, imgs []Image) error
	DeleteImage(ctx context.Context, spotID, imageID uuid.UUID) (*Image, error)
	RefreshCover(ctx context.Context, spotID uuid.UUID) error

	// ResolveUpazilas returns the upazilas (with district) in the given order.
	ResolveUpazilas(ctx context.Context, ids []int) ([]UpazilaRef, error)
	// NearestUpazilas returns upazilas closest to a coordinate (for autofill suggestions).
	NearestUpazilas(ctx context.Context, lat, lng float64, districtID, limit int) ([]UpazilaRef, error)
}

// FileStorage stores uploaded spot photos (Cloudinary).
type FileStorage interface {
	UploadPublic(ctx context.Context, data []byte, path, publicID string) (url, id string, err error)
	Delete(ctx context.Context, publicID string) error
}

var (
	ErrSpotNotFound       = apperror.New(http.StatusNotFound, "SPOT_NOT_FOUND", "tourist spot not found")
	ErrSlugTaken          = apperror.New(http.StatusConflict, "SLUG_TAKEN", "another tourist spot already uses this slug")
	ErrUpazilaNotFound    = apperror.New(http.StatusUnprocessableEntity, "UPAZILA_NOT_FOUND", "one or more upazila IDs do not exist")
	ErrTooManyImages      = apperror.New(http.StatusUnprocessableEntity, "TOO_MANY_IMAGES", "a tourist spot can have at most 50 images")
	ErrImageNotFound      = apperror.New(http.StatusNotFound, "IMAGE_NOT_FOUND", "image not found")
	ErrInvalidYouTube     = apperror.New(http.StatusUnprocessableEntity, "INVALID_YOUTUBE_URL", "enter a valid YouTube video link")
	ErrStorageUnavailable = apperror.New(http.StatusServiceUnavailable, "UPLOAD_UNAVAILABLE", "file uploads are temporarily unavailable")
	ErrAutofillNotFound   = apperror.New(http.StatusNotFound, "AUTOFILL_NO_RESULT", "no public information was found for this name, fill the form manually")
	ErrAutofillFailed     = apperror.New(http.StatusBadGateway, "AUTOFILL_FAILED", "could not reach the information sources, try again")
	ErrSpotHasGigs        = apperror.New(http.StatusConflict, "SPOT_HAS_ACTIVE_GIGS", "guides still offer tours for this spot; archive it instead of deleting")
)
