package shared

import (
	"context"

	"github.com/google/uuid"
)

// Notification is one in-app inbox item (also pushed live over Socket.IO).
type Notification struct {
	Type  string         // e.g. booking.requested, payment.success
	Title string         // short headline
	Body  string         // one or two sentences
	Link  string         // in-app path the item opens, e.g. /guide/bookings/<id>
	Data  map[string]any // small machine-readable payload (ids, statuses)
}

// Notifier is implemented by the notification module. Every method is
// best-effort: failures are logged, never returned to the business flow.
type Notifier interface {
	// Notify stores an inbox item for each user and pushes it live.
	Notify(ctx context.Context, userIDs []uuid.UUID, n Notification)
	// NotifyRole does the same for every active user with the role (e.g. all admins).
	NotifyRole(ctx context.Context, role string, n Notification)
	// Push sends a live-only event (no inbox item) to Socket.IO rooms.
	Push(ctx context.Context, rooms []string, event string, data any)
}

// NopNotifier is used until the notification module is wired (and in tools/tests).
type NopNotifier struct{}

func (NopNotifier) Notify(context.Context, []uuid.UUID, Notification) {}
func (NopNotifier) NotifyRole(context.Context, string, Notification)  {}
func (NopNotifier) Push(context.Context, []string, string, any)       {}
