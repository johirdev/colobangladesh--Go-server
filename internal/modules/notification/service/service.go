// Package service implements the notification use cases: store + push (shared.Notifier for every
// other module), the inbox, read marks and the daily cleanup.
package service

import (
	"context"
	"log/slog"
	"time"

	"cholobangladesh-server/internal/modules/notification/domain"
	"cholobangladesh-server/internal/modules/notification/dto"
	"cholobangladesh-server/internal/platform/realtime"
	"cholobangladesh-server/internal/shared"
	"cholobangladesh-server/pkg/jsonb"
	"cholobangladesh-server/pkg/listing"
	"cholobangladesh-server/pkg/response"

	"github.com/google/uuid"
)

// Service persists notifications and pushes them through the realtime publisher.
type Service struct {
	repo domain.Repository
	pub  *realtime.Publisher
}

var _ shared.Notifier = (*Service)(nil)

func New(repo domain.Repository, pub *realtime.Publisher) *Service {
	return &Service{repo: repo, pub: pub}
}

func toItem(n *domain.Notification) dto.Item {
	it := dto.Item{ID: n.ID.String(), Type: n.Type, Title: n.Title, Body: n.Body, Link: n.Link, Read: n.ReadAt != nil,
		CreatedAt: n.CreatedAt}
	if len(n.Data) > 0 {
		_ = n.Data.Decode(&it.Data)
	}
	return it
}

// ------------------------------------------------------------------ shared.Notifier

// Notify stores one notification per user and pushes it live.
func (s *Service) Notify(ctx context.Context, userIDs []uuid.UUID, n shared.Notification) {
	if len(userIDs) == 0 {
		return
	}
	ctx = context.WithoutCancel(ctx)
	var data jsonb.JSON
	if n.Data != nil {
		data = jsonb.From(n.Data)
	}
	var link *string
	if n.Link != "" {
		link = &n.Link
	}
	now := time.Now().UTC()
	seen := map[uuid.UUID]bool{}
	rows := make([]domain.Notification, 0, len(userIDs))
	for _, uid := range userIDs {
		if uid == uuid.Nil || seen[uid] {
			continue
		}
		seen[uid] = true
		rows = append(rows, domain.Notification{ID: uuid.Must(uuid.NewV7()), UserID: uid, Type: n.Type, Title: n.Title,
			Body: n.Body, Link: link, Data: data, CreatedAt: now})
	}
	if len(rows) == 0 {
		return
	}
	if err := s.repo.Insert(ctx, rows); err != nil {
		slog.Warn("notification: store", "type", n.Type, "err", err)
	}
	for i := range rows {
		s.pub.Publish(ctx, []string{realtime.UserRoom(rows[i].UserID)}, realtime.EvNotification, toItem(&rows[i]))
	}
}

// NotifyRole notifies every active account of a role (used for the few admin accounts).
func (s *Service) NotifyRole(ctx context.Context, role string, n shared.Notification) {
	ids, err := s.repo.ActiveUserIDsByRole(context.WithoutCancel(ctx), role, 200)
	if err != nil {
		slog.Warn("notification: role lookup", "role", role, "err", err)
		return
	}
	s.Notify(ctx, ids, n)
}

// Push sends a realtime event without storing it.
func (s *Service) Push(ctx context.Context, rooms []string, event string, data any) {
	s.pub.Publish(ctx, rooms, event, data)
}

// ------------------------------------------------------------------ inbox

func (s *Service) List(ctx context.Context, uid uuid.UUID, q dto.ListQuery) (response.Page[dto.Item], error) {
	res, err := s.repo.List(ctx, domain.Filter{UserID: uid, Unread: q.Unread, Type: q.Type, Params: q.Params})
	if err != nil {
		return response.Page[dto.Item]{}, err
	}
	return listing.Map(res, toItem), nil
}

func (s *Service) UnreadCount(ctx context.Context, uid uuid.UUID) (int64, error) {
	return s.repo.UnreadCount(ctx, uid)
}

func (s *Service) MarkRead(ctx context.Context, uid, id uuid.UUID) error {
	return s.repo.MarkRead(ctx, uid, id)
}

func (s *Service) MarkAllRead(ctx context.Context, uid uuid.UUID) error {
	return s.repo.MarkAllRead(ctx, uid)
}

// StartCleanup deletes read notifications older than 90 days, once a day.
func (s *Service) StartCleanup(ctx context.Context) {
	go func() {
		t := time.NewTicker(24 * time.Hour)
		defer t.Stop()
		for {
			if err := s.repo.DeleteReadBefore(ctx, time.Now().UTC().AddDate(0, 0, -90)); err != nil && ctx.Err() == nil {
				slog.Warn("notification: cleanup", "err", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}
