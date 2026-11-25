// Package domain holds the notification module's entity, list filter and repository port.
package domain

import (
	"context"
	"time"

	"cholobangladesh-server/pkg/jsonb"
	"cholobangladesh-server/pkg/listing"

	"github.com/google/uuid"
)

// Notification row (table notifications).
type Notification struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID    uuid.UUID `gorm:"type:uuid"`
	Type      string
	Title     string
	Body      string
	Link      *string
	Data      jsonb.JSON `gorm:"type:jsonb"`
	ReadAt    *time.Time
	CreatedAt time.Time
}

func (Notification) TableName() string { return "notifications" }

// Filter lists one user's inbox.
type Filter struct {
	UserID uuid.UUID
	Unread bool
	Type   string // e.g. "booking.accepted"
	listing.Params
}

type Repository interface {
	Insert(ctx context.Context, rows []Notification) error
	List(ctx context.Context, f Filter) (listing.Result[Notification], error)
	UnreadCount(ctx context.Context, userID uuid.UUID) (int64, error)
	MarkRead(ctx context.Context, userID, id uuid.UUID) error
	MarkAllRead(ctx context.Context, userID uuid.UUID) error
	ActiveUserIDsByRole(ctx context.Context, role string, max int) ([]uuid.UUID, error)
	DeleteReadBefore(ctx context.Context, cut time.Time) error
}
