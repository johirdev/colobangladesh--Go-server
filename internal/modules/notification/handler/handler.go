// Package handler is the HTTP layer of the notification module (with swagger annotations).
package handler

import (
	"net/http"

	"cholobangladesh-server/internal/modules/notification/dto"
	"cholobangladesh-server/internal/modules/notification/service"
	"cholobangladesh-server/internal/platform/realtime"
	"cholobangladesh-server/internal/shared/web"
	"cholobangladesh-server/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

type Handler struct {
	svc   *service.Service
	redis *redis.Client
}

func New(svc *service.Service, rdb *redis.Client) *Handler { return &Handler{svc: svc, redis: rdb} }

// Ticket godoc
//
//	@Summary		Socket.IO connection ticket
//	@Description	Returns a single-use ticket (valid 60 s). Connect with socket.io-client v4:
//	@Description	`io(API_ORIGIN, { path: "/socket.io/", auth: { ticket } })`. The socket joins the rooms
//	@Description	`user:<id>` and `role:<role>` and receives: `notification`, `booking:updated`, `payment:updated`,
//	@Description	`message:new`, `message:read`, `plan:updated`, `admin:activity` (admins). Get a new ticket for every (re)connect.
//	@Tags			19. Realtime & notifications
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	response.Body{data=dto.Ticket}
//	@Router			/realtime/ticket [post]
func (h *Handler) Ticket(c *gin.Context) {
	t, ttl, err := realtime.IssueTicket(c.Request.Context(), h.redis, web.UserID(c), web.Role(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "", dto.Ticket{Ticket: t, ExpiresIn: int(ttl.Seconds()), Path: realtime.Path})
}

// List godoc
//
//	@Summary	My notifications
//	@Tags		19. Realtime & notifications
//	@Produce	json
//	@Security	BearerAuth
//	@Param		query	query		dto.ListQuery	false	"Filters"
//	@Success	200		{object}	response.Body{data=dto.ItemPage}
//	@Router		/notifications [get]
func (h *Handler) List(c *gin.Context) {
	var q dto.ListQuery
	if !web.BindQuery(c, &q) {
		return
	}
	res, err := h.svc.List(c.Request.Context(), web.UserID(c), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "", res)
}

// Unread godoc
//
//	@Summary	Unread notification count (badge)
//	@Tags		19. Realtime & notifications
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	response.Body{data=dto.UnreadCount}
//	@Router		/notifications/unread-count [get]
func (h *Handler) Unread(c *gin.Context) {
	n, err := h.svc.UnreadCount(c.Request.Context(), web.UserID(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "", dto.UnreadCount{Count: n})
}

// Read godoc
//
//	@Summary	Mark one notification read
//	@Tags		19. Realtime & notifications
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Notification ID"	format(uuid)
//	@Success	200	{object}	response.Body
//	@Router		/notifications/{id}/read [post]
func (h *Handler) Read(c *gin.Context) {
	id, ok := web.UUIDParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.MarkRead(c.Request.Context(), web.UserID(c), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "marked read", nil)
}

// ReadAll godoc
//
//	@Summary	Mark all my notifications read
//	@Tags		19. Realtime & notifications
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	response.Body
//	@Router		/notifications/read-all [post]
func (h *Handler) ReadAll(c *gin.Context) {
	if err := h.svc.MarkAllRead(c.Request.Context(), web.UserID(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "all read", nil)
}
