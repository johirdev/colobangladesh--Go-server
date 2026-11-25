package handler

import (
	"net/http"
	"strings"

	"cholobangladesh-server/internal/modules/booking/dto"
	"cholobangladesh-server/internal/shared/web"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ================================================================== slots

// BookedDates godoc
//
//	@Summary		Booked dates of a gig's guide (calendar slots)
//	@Description	Days the guide is already booked (accepted tours of ANY of their gigs), so the date picker can
//	@Description	grey them out. Default range: today → +6 months. No personal data.
//	@Tags			14. Gigs · Public
//	@Produce		json
//	@Param			id		path		string					true	"Gig ID"	format(uuid)
//	@Param			query	query		dto.BookedDatesQuery	false	"Range"
//	@Success		200		{object}	response.Body{data=dto.BookedDatesResponse}
//	@Router			/gigs/{id}/booked-dates [get]
func (h *Handler) BookedDates(c *gin.Context) {
	withID(c, func(id uuid.UUID) {
		var q dto.BookedDatesQuery
		if !web.BindQuery(c, &q) {
			return
		}
		res, err := h.svc.BookedDates(c.Request.Context(), id, q)
		reply(c, http.StatusOK, "", res, err)
	})
}

// ================================================================== online payment

// PayOnline godoc
//
//	@Summary		Pay online with aamarPay (tourist)
//	@Description	Allowed once the guide accepted (`can_pay`). Returns `payment_url`: redirect the browser there
//	@Description	(bKash, Nagad, Rocket, cards…). aamarPay sends the tourist back through
//	@Description	`/payments/aamarpay/return`, the API verifies the transaction server-to-server and redirects to
//	@Description	`FRONTEND_URL/account/bookings/{id}?payment=success|failed|cancelled|pending`.
//	@Tags			16. Payments
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Booking ID"	format(uuid)
//	@Success		201	{object}	response.Body{data=dto.PayOnlineResponse}
//	@Failure		409	{object}	response.ErrorResponse	"PAYMENT_NOT_DUE | ALREADY_PAID"
//	@Failure		502	{object}	response.ErrorResponse	"PAYMENT_GATEWAY_ERROR"
//	@Failure		503	{object}	response.ErrorResponse	"PAYMENT_GATEWAY_DISABLED"
//	@Router			/bookings/{id}/pay [post]
func (h *Handler) PayOnline(c *gin.Context) {
	withID(c, func(id uuid.UUID) {
		res, err := h.svc.PayOnline(c.Request.Context(), web.UserID(c), id, c.ClientIP())
		reply(c, http.StatusCreated, "redirect to the payment page", res, err)
	})
}

// GatewayReturn godoc
//
//	@Summary		aamarPay return URL (browser redirect, not for API clients)
//	@Description	aamarPay posts the result here. Nothing in the request is trusted: the API re-checks the
//	@Description	transaction with aamarPay (amount, currency, store) and then 303-redirects to the website.
//	@Tags			16. Payments
//	@Param			tran_id	query	string	true	"Our transaction ID"
//	@Param			result	query	string	true	"success | fail | cancel"
//	@Success		303
//	@Router			/payments/aamarpay/return [post]
func (h *Handler) GatewayReturn(c *gin.Context) {
	result := c.Query("result")
	if result != "success" && result != "fail" && result != "cancel" {
		result = "unknown"
	}
	_ = c.Request.ParseForm()
	form := map[string]string{}
	for _, k := range []string{"pay_status", "mer_txnid", "pg_txnid", "amount", "currency", "card_type", "pay_time",
		"bank_txn", "status_code", "reason"} {
		if v := c.Request.PostForm.Get(k); v != "" {
			if len(v) > 200 {
				v = v[:200]
			}
			form[k] = v
		}
	}
	tran := strings.TrimSpace(c.Query("tran_id"))
	if tran == "" {
		tran = strings.TrimSpace(c.Request.PostForm.Get("mer_txnid"))
	}
	c.Header("Cache-Control", "no-store")
	c.Redirect(http.StatusSeeOther, h.svc.GatewayReturn(c.Request.Context(), tran, result, form, c.ClientIP()))
}

// CheckTransaction godoc
//
//	@Summary		Re-check an online payment (payer or admin)
//	@Description	Asks aamarPay again (e.g. the browser was closed before returning). Safe to call repeatedly.
//	@Tags			16. Payments
//	@Produce		json
//	@Security		BearerAuth
//	@Param			tran_id	path		string	true	"Transaction ID"
//	@Success		200		{object}	response.Body{data=dto.GatewayPaymentResponse}
//	@Router			/payments/aamarpay/{tran_id}/verify [post]
func (h *Handler) CheckTransaction(c *gin.Context) {
	res, err := h.svc.CheckTransaction(c.Request.Context(), web.UserID(c), web.Role(c), c.Param("tran_id"))
	reply(c, http.StatusOK, "", res, err)
}

// BookingTransactions godoc
//
//	@Summary	Online payment attempts of a booking
//	@Tags		16. Payments
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Booking ID"	format(uuid)
//	@Success	200	{object}	response.Body{data=[]dto.GatewayPaymentResponse}
//	@Router		/bookings/{id}/transactions [get]
func (h *Handler) BookingTransactions(c *gin.Context) {
	withID(c, func(id uuid.UUID) {
		res, err := h.svc.BookingTransactions(c.Request.Context(), web.UserID(c), web.Role(c), id)
		reply(c, http.StatusOK, "", res, err)
	})
}

// ================================================================== trip plan

// GetPlan godoc
//
//	@Summary		Trip plan of a booking
//	@Description	Arrival, pickup point (Google Maps link), hotel and day-by-day itinerary. Tourist, guide and admins.
//	@Description	`exists=false` until someone saves it; `can_edit` once the booking is paid (until completed).
//	@Tags			20. Trip plan & chat
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Booking ID"	format(uuid)
//	@Success		200	{object}	response.Body{data=dto.TripPlanResponse}
//	@Router			/bookings/{id}/plan [get]
func (h *Handler) GetPlan(c *gin.Context) {
	withID(c, func(id uuid.UUID) {
		res, err := h.svc.GetPlan(c.Request.Context(), web.UserID(c), web.Role(c), id)
		reply(c, http.StatusOK, "", res, err)
	})
}

// SavePlan godoc
//
//	@Summary		Save the trip plan (tourist or guide, after payment)
//	@Description	Replaces the whole plan. Map links must be http(s) (e.g. Google Maps share links).
//	@Description	The other side and admins get a live `plan:updated` event.
//	@Tags			20. Trip plan & chat
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string				true	"Booking ID"	format(uuid)
//	@Param			body	body		dto.TripPlanRequest	true	"Plan"
//	@Success		200		{object}	response.Body{data=dto.TripPlanResponse}
//	@Failure		409		{object}	response.ErrorResponse	"TRIP_PLAN_NOT_OPEN"
//	@Router			/bookings/{id}/plan [put]
func (h *Handler) SavePlan(c *gin.Context) {
	withID(c, func(id uuid.UUID) {
		var req dto.TripPlanRequest
		if !web.BindJSON(c, &req) {
			return
		}
		res, err := h.svc.SavePlan(c.Request.Context(), web.UserID(c), web.Role(c), id, req)
		reply(c, http.StatusOK, "trip plan saved", res, err)
	})
}

// ================================================================== chat

// Messages godoc
//
//	@Summary		Chat messages of a booking (oldest → newest)
//	@Description	Opens once the booking is paid. Fetching marks the other side's messages as read.
//	@Description	New messages arrive live as `message:new`. Older pages: `?before=<created_at of the oldest>`.
//	@Tags			20. Trip plan & chat
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string				true	"Booking ID"	format(uuid)
//	@Param			query	query		dto.MessagesQuery	false	"Paging"
//	@Success		200		{object}	response.Body{data=[]dto.MessageResponse}
//	@Failure		409		{object}	response.ErrorResponse	"CHAT_NOT_OPEN"
//	@Router			/bookings/{id}/messages [get]
func (h *Handler) Messages(c *gin.Context) {
	withID(c, func(id uuid.UUID) {
		var q dto.MessagesQuery
		if !web.BindQuery(c, &q) {
			return
		}
		res, err := h.svc.Messages(c.Request.Context(), web.UserID(c), web.Role(c), id, q)
		reply(c, http.StatusOK, "", res, err)
	})
}

// SendMessage godoc
//
//	@Summary	Send a chat message (tourist or guide of a paid booking)
//	@Tags		20. Trip plan & chat
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string				true	"Booking ID"	format(uuid)
//	@Param		body	body		dto.MessageRequest	true	"Message"
//	@Success	201		{object}	response.Body{data=dto.MessageResponse}
//	@Failure	409		{object}	response.ErrorResponse	"CHAT_NOT_OPEN"
//	@Router		/bookings/{id}/messages [post]
func (h *Handler) SendMessage(c *gin.Context) {
	withID(c, func(id uuid.UUID) {
		var req dto.MessageRequest
		if !web.BindJSON(c, &req) {
			return
		}
		res, err := h.svc.SendMessage(c.Request.Context(), web.UserID(c), web.Role(c), id, req)
		reply(c, http.StatusCreated, "", res, err)
	})
}

// UnreadMessages godoc
//
//	@Summary	Unread chat messages (total + per booking)
//	@Tags		20. Trip plan & chat
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	response.Body{data=dto.UnreadMessages}
//	@Router		/messages/unread [get]
func (h *Handler) UnreadMessages(c *gin.Context) {
	res, err := h.svc.UnreadMessages(c.Request.Context(), web.UserID(c))
	reply(c, http.StatusOK, "", res, err)
}

// ================================================================== admin

// AdminComplete godoc
//
//	@Summary		Complete a tour on the tourist's behalf (admin)
//	@Description	Only for paid bookings after the end date, once an admin has confirmed with the guide.
//	@Description	`note` (required) records what was confirmed.
//	@Tags			18. Admin · Marketplace
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string						true	"Booking ID"	format(uuid)
//	@Param			body	body		dto.AdminCompleteRequest	true	"Note"
//	@Success		200		{object}	response.Body{data=dto.BookingResponse}
//	@Failure		409		{object}	response.ErrorResponse	"TOUR_NOT_ENDED | BOOKING_NOT_PAID"
//	@Router			/admin/bookings/{id}/complete [post]
func (h *Handler) AdminComplete(c *gin.Context) {
	withID(c, func(id uuid.UUID) {
		var req dto.AdminCompleteRequest
		if !web.BindJSON(c, &req) {
			return
		}
		res, err := h.svc.AdminComplete(c.Request.Context(), web.UserID(c), id, req.Note)
		reply(c, http.StatusOK, "tour completed", res, err)
	})
}

// AdminCancel godoc
//
//	@Summary		Cancel any pending/accepted booking (admin)
//	@Description	For paid bookings set `refunded=true` after refunding in the aamarPay merchant panel.
//	@Tags			18. Admin · Marketplace
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string					true	"Booking ID"	format(uuid)
//	@Param			body	body		dto.AdminCancelRequest	true	"Reason"
//	@Success		200		{object}	response.Body{data=dto.BookingResponse}
//	@Router			/admin/bookings/{id}/cancel [post]
func (h *Handler) AdminCancel(c *gin.Context) {
	withID(c, func(id uuid.UUID) {
		var req dto.AdminCancelRequest
		if !web.BindJSON(c, &req) {
			return
		}
		res, err := h.svc.AdminCancel(c.Request.Context(), web.UserID(c), id, req)
		reply(c, http.StatusOK, "booking cancelled", res, err)
	})
}

// AdminTransactions godoc
//
//	@Summary		Online payments (aamarPay transactions)
//	@Description	Every checkout attempt with tourist, guide, gig, booking, amount, gateway ids and status.
//	@Tags			18. Admin · Marketplace
//	@Produce		json
//	@Security		BearerAuth
//	@Param			query	query		dto.GatewayQuery	false	"Filters"
//	@Success		200		{object}	response.Body{data=dto.GatewayPaymentPage}
//	@Router			/admin/transactions [get]
func (h *Handler) AdminTransactions(c *gin.Context) {
	var q dto.GatewayQuery
	if !web.BindQuery(c, &q) {
		return
	}
	res, err := h.svc.AdminTransactions(c.Request.Context(), q)
	reply(c, http.StatusOK, "", res, err)
}

// AdminTransaction godoc
//
//	@Summary	Online payment detail with its audit trail (admin)
//	@Tags		18. Admin · Marketplace
//	@Produce	json
//	@Security	BearerAuth
//	@Param		tran_id	path		string	true	"Transaction ID"
//	@Success	200		{object}	response.Body{data=dto.GatewayPaymentResponse}
//	@Router		/admin/transactions/{tran_id} [get]
func (h *Handler) AdminTransaction(c *gin.Context) {
	res, err := h.svc.AdminTransaction(c.Request.Context(), c.Param("tran_id"))
	reply(c, http.StatusOK, "", res, err)
}

// AdminPlans godoc
//
//	@Summary	Trip plans monitor (admin)
//	@Tags		18. Admin · Marketplace
//	@Produce	json
//	@Security	BearerAuth
//	@Param		query	query		dto.TripPlanQuery	false	"Filters"
//	@Success	200		{object}	response.Body{data=dto.TripPlanPage}
//	@Router		/admin/trip-plans [get]
func (h *Handler) AdminPlans(c *gin.Context) {
	var q dto.TripPlanQuery
	if !web.BindQuery(c, &q) {
		return
	}
	res, err := h.svc.AdminPlans(c.Request.Context(), web.UserID(c), q)
	reply(c, http.StatusOK, "", res, err)
}
