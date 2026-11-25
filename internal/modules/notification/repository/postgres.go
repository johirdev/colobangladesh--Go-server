// Package repository implements the notification repository with PostgreSQL (GORM).
package repository

import (
	"context"
	"time"

	"cholobangladesh-server/internal/modules/notification/domain"
	"cholobangladesh-server/pkg/listing"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) domain.Repository { return &postgres{db: db} }

func (r *postgres) q(ctx context.Context) *gorm.DB { return r.db.WithContext(ctx) }

// ListSpec: inbox (20 per page, max 50); ?q= matches title or text.
var ListSpec = listing.Spec{
	Sorts: map[string]string{
		"newest": "notifications.created_at DESC",
		"oldest": "notifications.created_at ASC",
	},
	DefaultSort: "newest",
	Tiebreak:    "notifications.id",
	Search:      []string{"notifications.title", "notifications.body"},
	MaxLimit:    50,
}

func (r *postgres) Insert(ctx context.Context, rows []domain.Notification) error {
	return r.q(ctx).Create(&rows).Error
}

func (r *postgres) List(ctx context.Context, f domain.Filter) (listing.Result[domain.Notification], error) {
	q := listing.Filter(r.q(ctx).Model(&domain.Notification{})).
		Always("notifications.user_id = ?", f.UserID).
		If(f.Unread, "notifications.read_at IS NULL").
		Eq("notifications.type", f.Type)
	return listing.Find[domain.Notification](q, f.Params, ListSpec, nil)
}

func (r *postgres) UnreadCount(ctx context.Context, userID uuid.UUID) (int64, error) {
	var n int64
	err := r.q(ctx).Model(&domain.Notification{}).Where("user_id = ? AND read_at IS NULL", userID).Count(&n).Error
	return n, err
}

func (r *postgres) MarkRead(ctx context.Context, userID, id uuid.UUID) error {
	return r.q(ctx).Model(&domain.Notification{}).Where("id = ? AND user_id = ? AND read_at IS NULL", id, userID).
		Update("read_at", time.Now().UTC()).Error
}

func (r *postgres) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	return r.q(ctx).Model(&domain.Notification{}).Where("user_id = ? AND read_at IS NULL", userID).
		Update("read_at", time.Now().UTC()).Error
}

func (r *postgres) ActiveUserIDsByRole(ctx context.Context, role string, max int) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := r.q(ctx).Table("users").Where("role = ? AND status = 'active' AND deleted_at IS NULL", role).
		Limit(max).Pluck("id", &ids).Error
	return ids, err
}

func (r *postgres) DeleteReadBefore(ctx context.Context, cut time.Time) error {
	return r.q(ctx).Where("read_at IS NOT NULL AND created_at < ?", cut).Delete(&domain.Notification{}).Error
}
