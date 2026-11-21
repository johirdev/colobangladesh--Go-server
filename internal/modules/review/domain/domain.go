// Package domain holds the review module's entities, errors and repository port.
package domain

import (
	"context"
	"net/http"
	"time"

	"cholobangladesh-server/pkg/apperror"

	"github.com/google/uuid"
)

const MaxImages = 5

type Review struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	BookingID  uuid.UUID `gorm:"type:uuid"`
	GigID      uuid.UUID `gorm:"type:uuid"`
	GuideID    uuid.UUID `gorm:"type:uuid"`
	SpotID     uuid.UUID `gorm:"type:uuid"`
	UserID     uuid.UUID `gorm:"type:uuid"`
	Rating     int
	Comment    string
	GuideReply *string
	RepliedAt  *time.Time
	IsVisible  bool
	CreatedAt  time.Time
	UpdatedAt  time.Time

	Images []Image    `gorm:"foreignKey:ReviewID"`
	Author *AuthorRef `gorm:"foreignKey:UserID;references:UserID"`
	Gig    *GigRef    `gorm:"foreignKey:GigID"`
}

func (Review) TableName() string { return "reviews" }

type Image struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	ReviewID  uuid.UUID `gorm:"type:uuid"`
	URL       string    `gorm:"column:url"`
	PublicID  *string
	Position  int
	CreatedAt time.Time
}

func (Image) TableName() string { return "review_images" }

type AuthorRef struct {
	UserID      uuid.UUID `gorm:"type:uuid;primaryKey"`
	FullName    string
	AvatarURL   *string
	CountryCode *string
}

func (AuthorRef) TableName() string { return "user_profiles" }

type GigRef struct {
	ID    uuid.UUID `gorm:"type:uuid;primaryKey"`
	Title string
}

func (GigRef) TableName() string { return "guide_gigs" }

// BookingInfo is what reviewing needs to know about a booking.
type BookingInfo struct {
	ID      uuid.UUID
	UserID  uuid.UUID
	GuideID uuid.UUID
	GigID   uuid.UUID
	SpotID  uuid.UUID
	Status  string
}

type Filter struct {
	GigID       uuid.UUID
	GuideID     uuid.UUID
	SpotID      uuid.UUID
	WithPhotos  bool
	IncludeHide bool // admin only
	Sort        string
	Page        int
	Limit       int
}

// Summary is the rating breakdown shown above the review list.
type Summary struct {
	Average float64
	Count   int64
	Stars   map[int]int64 // 1..5 -> count
}

type Repository interface {
	Booking(ctx context.Context, id uuid.UUID) (*BookingInfo, error)
	Create(ctx context.Context, r *Review) error // also inserts images and refreshes ratings
	Get(ctx context.Context, id uuid.UUID) (*Review, error)
	List(ctx context.Context, f Filter) ([]Review, int64, error)
	Summary(ctx context.Context, f Filter) (*Summary, error)
	Update(ctx context.Context, id uuid.UUID, fields map[string]any) error
	// RefreshRatings recomputes rating_avg/rating_count for the gig, guide and spot.
	RefreshRatings(ctx context.Context, gigID, guideID, spotID uuid.UUID) error
}

type FileStorage interface {
	UploadPublic(ctx context.Context, data []byte, path, publicID string) (url, id string, err error)
	Delete(ctx context.Context, publicID string) error
}

var (
	ErrReviewNotFound     = apperror.New(http.StatusNotFound, "REVIEW_NOT_FOUND", "review not found")
	ErrBookingNotFound    = apperror.New(http.StatusNotFound, "BOOKING_NOT_FOUND", "booking not found")
	ErrNotCompleted       = apperror.New(http.StatusConflict, "TOUR_NOT_COMPLETED", "you can review after the guide marks the tour completed")
	ErrAlreadyReviewed    = apperror.New(http.StatusConflict, "ALREADY_REVIEWED", "you have already reviewed this booking")
	ErrTooManyImages      = apperror.New(http.StatusUnprocessableEntity, "TOO_MANY_IMAGES", "a review can have at most 5 photos")
	ErrAlreadyReplied     = apperror.New(http.StatusConflict, "ALREADY_REPLIED", "you have already replied to this review")
	ErrStorageUnavailable = apperror.New(http.StatusServiceUnavailable, "UPLOAD_UNAVAILABLE", "photo uploads are temporarily unavailable")
)
