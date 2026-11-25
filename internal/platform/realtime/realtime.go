// Package realtime is the Socket.IO server that pushes live updates (notifications,
// booking/payment status, chat messages) to browsers.
//
// Flow:
//
//	browser ──POST /api/v1/realtime/ticket (JWT)──▶ one-time ticket (Redis, 60 s)
//	browser ──io(URL, {auth:{ticket}})──▶ ticket consumed ──▶ joins rooms user:<id>, role:<role>
//	any service ──Publisher.Publish(rooms, event, data)──▶ Redis channel "rt:events"
//	every API instance ──subscribes──▶ io.To(rooms).Emit(event, data)
//
// Going through Redis pub/sub means it works the same with one API instance or many.
// Clients never emit anything that changes state: writes go through the REST API.
package realtime

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

const (
	channel   = "rt:events"
	ticketTTL = 60 * time.Second
	// Path is where the Socket.IO endpoint is mounted (default socket.io-client path).
	Path = "/socket.io/"
)

// Event names sent to browsers.
const (
	EvNotification   = "notification"    // a new inbox item (persisted)
	EvBookingUpdated = "booking:updated" // status / payment status changed
	EvPaymentUpdated = "payment:updated" // online payment changed
	EvMessageNew     = "message:new"     // chat message in a booking
	EvMessagesRead   = "message:read"    // the other side read the chat
	EvPlanUpdated    = "plan:updated"    // trip plan saved
	EvAdminActivity  = "admin:activity"  // marketplace activity for admin dashboards
)

// UserRoom / RoleRoom name the rooms every socket joins.
func UserRoom(id uuid.UUID) string { return "user:" + id.String() }
func RoleRoom(role string) string  { return "role:" + role }

type envelope struct {
	Rooms []string        `json:"rooms"`
	Event string          `json:"event"`
	Data  json.RawMessage `json:"data"`
}

// ------------------------------------------------------------------ publisher

// Publisher sends events to sockets on every API instance.
type Publisher struct{ rdb *redis.Client }

func NewPublisher(rdb *redis.Client) *Publisher { return &Publisher{rdb: rdb} }

// Publish is fire-and-forget: a failed push never fails the business operation
// (clients also refetch on reconnect / window focus).
func (p *Publisher) Publish(ctx context.Context, rooms []string, event string, data any) {
	if p == nil || len(rooms) == 0 {
		return
	}
	raw, err := json.Marshal(data)
	if err != nil {
		slog.Warn("realtime: marshal", "event", event, "err", err)
		return
	}
	msg, _ := json.Marshal(envelope{Rooms: rooms, Event: event, Data: raw})
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer cancel()
	if err := p.rdb.Publish(ctx, channel, msg).Err(); err != nil {
		slog.Warn("realtime: publish", "event", event, "err", err)
	}
}

// ------------------------------------------------------------------ tickets

// IssueTicket returns a single-use connection ticket for an authenticated user.
// Only its SHA-256 is stored, so a Redis dump cannot be replayed.
func IssueTicket(ctx context.Context, rdb *redis.Client, userID uuid.UUID, role string) (string, time.Duration, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", 0, err
	}
	t := base64.RawURLEncoding.EncodeToString(b)
	if err := rdb.Set(ctx, ticketKey(t), userID.String()+"|"+role, ticketTTL).Err(); err != nil {
		return "", 0, err
	}
	return t, ticketTTL, nil
}

func ticketKey(t string) string {
	h := sha256.Sum256([]byte(t))
	return "rt:ticket:" + hex.EncodeToString(h[:])
}

func consumeTicket(ctx context.Context, rdb *redis.Client, t string) (uuid.UUID, string, error) {
	if len(t) < 20 || len(t) > 100 {
		return uuid.Nil, "", errors.New("invalid ticket")
	}
	v, err := rdb.GetDel(ctx, ticketKey(t)).Result()
	if err != nil {
		return uuid.Nil, "", errors.New("invalid ticket")
	}
	idStr, role, ok := strings.Cut(v, "|")
	id, perr := uuid.Parse(idStr)
	if !ok || perr != nil || role == "" {
		return uuid.Nil, "", errors.New("invalid ticket")
	}
	return id, role, nil
}

// ------------------------------------------------------------------ server

type identity struct {
	UserID uuid.UUID
	Role   string
}

// Server is the Socket.IO endpoint.
type Server struct {
	io      *socket.Server
	rdb     *redis.Client
	handler http.Handler
}

// NewServer builds the Socket.IO server. origins = CORS_ORIGINS (browsers allowed to connect).
func NewServer(rdb *redis.Client, origins []string) *Server {
	opts := socket.DefaultServerOptions()
	opts.SetPath(strings.TrimSuffix(Path, "/"))
	opts.SetServeClient(false)
	opts.SetPingInterval(25 * time.Second)
	opts.SetPingTimeout(20 * time.Second)
	opts.SetMaxHttpBufferSize(16 << 10) // clients only send tiny control frames
	opts.SetConnectTimeout(10 * time.Second)
	if len(origins) == 0 {
		origins = []string{"http://localhost:3000"}
	}
	opts.SetCors(&types.Cors{Origin: origins, Credentials: true})

	io := socket.NewServer(nil, opts)
	s := &Server{io: io, rdb: rdb}

	io.Use(func(c *socket.Socket, next func(*socket.ExtendedError)) {
		ticket, _ := c.Handshake().Auth["ticket"].(string)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		uid, role, err := consumeTicket(ctx, rdb, ticket)
		if err != nil {
			next(socket.NewExtendedError("unauthorized", map[string]any{"code": "REALTIME_TICKET_INVALID"}))
			return
		}
		c.SetData(&identity{UserID: uid, Role: role})
		next(nil)
	})

	_ = io.On("connection", func(args ...any) {
		c, ok := args[0].(*socket.Socket)
		if !ok {
			return
		}
		id, ok := c.Data().(*identity)
		if !ok {
			c.Disconnect(true)
			return
		}
		c.Join(socket.Room(UserRoom(id.UserID)), socket.Room(RoleRoom(id.Role)))
		_ = c.Emit("ready", map[string]any{"user_id": id.UserID.String(), "role": id.Role})
	})

	s.handler = io.ServeHandler(opts)
	return s
}

// Handler serves /socket.io/. Long-lived connections must not inherit the API's
// read/write timeouts, so they are cleared for this endpoint only.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		_ = rc.SetReadDeadline(time.Time{})
		_ = rc.SetWriteDeadline(time.Time{})
		s.handler.ServeHTTP(w, r)
	})
}

// Run relays published events to local sockets until ctx is cancelled.
func (s *Server) Run(ctx context.Context) {
	go func() {
		for ctx.Err() == nil {
			sub := s.rdb.Subscribe(ctx, channel)
			ch := sub.Channel()
			for msg := range ch {
				s.dispatch(msg.Payload)
			}
			_ = sub.Close()
			if ctx.Err() == nil {
				slog.Warn("realtime: redis subscription closed, reconnecting")
				time.Sleep(time.Second)
			}
		}
	}()
	slog.Info("realtime (socket.io) ready", "path", Path)
}

func (s *Server) dispatch(payload string) {
	var e envelope
	if err := json.Unmarshal([]byte(payload), &e); err != nil || e.Event == "" || len(e.Rooms) == 0 {
		return
	}
	var data any
	if err := json.Unmarshal(e.Data, &data); err != nil {
		return
	}
	rooms := make([]socket.Room, 0, len(e.Rooms))
	for _, r := range e.Rooms {
		rooms = append(rooms, socket.Room(r))
	}
	_ = s.io.To(rooms...).Emit(e.Event, data)
}

// Close disconnects every socket.
func (s *Server) Close() { s.io.Close(nil) }
