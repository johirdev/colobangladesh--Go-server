// Package handler is the HTTP layer of the booking & payment module (with swagger annotations).
package handler

import (
	"net/http"

	"cholobangladesh-server/internal/modules/booking/dto"
	"cholobangladesh-server/internal/modules/booking/service"
	"cholobangladesh-server/internal/shared/web"
	"cholobangladesh-server/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct{ svc *service.Service }

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

func reply(c *gin.Context, status int, msg string, data any, err error) {
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, status, msg, data)
}

// withID parses :id and runs fn.
func withID(c *gin.Context, fn func(id uuid.UUID)) {
	if id, ok := web.UUIDParam(c, "id"); ok {
		fn(id)
	}
}

// ================================================================== tourist

// Create godoc
//
//	@Summary		Book a gig
//	@Description	Tourist accounts only. The price is fixed at booking time (`per_person` × group size or `per_group`).
//	@Description	Payment is NOT taken now: it opens after the guide marks the tour completed (`can_pay`).
//	@Tags			15. Bookings
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		dto.CreateBookingRequest	true	"Booking"
//	@Success		201		{object}	response.Body{data=dto.BookingResponse}
//	@Failure		422		{object}	response.ErrorResponse	"GIG_NOT_BOOKABLE | GROUP_TOO_LARGE | TOUR_DATE_IN_PAST | CANNOT_BOOK_OWN_GIG"
//	@Router			/bookings [post]
func (h *Handler) Create(c *gin.Context) {
	var req dto.CreateBookingRequest
	if !web.BindJSON(c, &req) {
		return
	}
	res, err := h.svc.Create(c.Request.Context(), web.UserID(c), req)
	reply(c, http.StatusCreated, "booking requested, waiting for the guide", res, err)
}

// ListMine godoc
//
//	@Summary	My bookings (tourist)
//	@Tags		15. Bookings
//	@Produce	json
//	@Security	BearerAuth
//	@Param		query	query		dto.BookingQuery	false	"Filters"
//	@Success	200		{object}	response.Body{data=dto.BookingPage}
//	@Router		/bookings/me [get]
func (h *Handler) ListMine(c *gin.Context) {
	var q dto.BookingQuery
	if !web.BindQuery(c, &q) {
		return
	}
	res, err := h.svc.ListMine(c.Request.Context(), web.UserID(c), q)
	reply(c, http.StatusOK, "", res, err)
}

// ListForGuide godoc
//
//	@Summary	Booking requests for my gigs (guide)
//	@Tags		15. Bookings
//	@Produce	json
//	@Security	BearerAuth
//	@Param		query	query		dto.BookingQuery	false	"Filters"
//	@Success	200		{object}	response.Body{data=dto.BookingPage}
//	@Router		/bookings/guide [get]
func (h *Handler) ListForGuide(c *gin.Context) {
	var q dto.BookingQuery
	if !web.BindQuery(c, &q) {
		return
	}
	res, err := h.svc.ListForGuide(c.Request.Context(), web.UserID(c), q)
	reply(c, http.StatusOK, "", res, err)
}

// Get godoc
//
//	@Summary		Booking details
//	@Description	Visible to the tourist, the guide and admins. Contact details of the other side appear once the
//	@Description	guide accepts. `can_*` flags tell the UI which buttons to show.
//	@Tags			15. Bookings
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Booking ID"	format(uuid)
//	@Success		200	{object}	response.Body{data=dto.BookingResponse}
//	@Failure		404	{object}	response.ErrorResponse	"BOOKING_NOT_FOUND"
//	@Router			/bookings/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	withID(c, func(id uuid.UUID) {
		res, err := h.svc.Get(c.Request.Context(), web.UserID(c), web.Role(c), id)
		reply(c, http.StatusOK, "", res, err)
	})
}

// Accept godoc
//
//	@Summary	Accept a booking request (guide)
//	@Tags		15. Bookings
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Booking ID"	format(uuid)
//	@Success	200	{object}	response.Body{data=dto.BookingResponse}
//	@Failure	409	{object}	response.ErrorResponse	"BOOKING_STATUS_CONFLICT"
//	@Router		/bookings/{id}/accept [post]
func (h *Handler) Accept(c *gin.Context) {
	withID(c, func(id uuid.UUID) {
		res, err := h.svc.Accept(c.Request.Context(), web.UserID(c), id)
		reply(c, http.StatusOK, "booking accepted", res, err)
	})
}

// Reject godoc
//
//	@Summary	Reject a booking request (guide)
//	@Tags		15. Bookings
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string				true	"Booking ID"	format(uuid)
//	@Param		body	body		dto.ReasonRequest	false	"Reason"
//	@Success	200		{object}	response.Body{data=dto.BookingResponse}
//	@Failure	409		{object}	response.ErrorResponse	"BOOKING_STATUS_CONFLICT"
//	@Router		/bookings/{id}/reject [post]
func (h *Handler) Reject(c *gin.Context) {
	withID(c, func(id uuid.UUID) {
		var req dto.ReasonRequest
		_ = c.ShouldBindJSON(&req)
		res, err := h.svc.Reject(c.Request.Context(), web.UserID(c), id, req.Reason)
		reply(c, http.StatusOK, "booking rejected", res, err)
	})
}

// Complete godoc
//
//	@Summary		Confirm the tour is completed (tourist only)
//	@Description	Only the tourist can confirm, once the booking is **paid** and on/after the tour start date.
//	@Description	If the tourist never confirms, an admin completes it after the end date
//	@Description	(POST /admin/bookings/{id}/complete) after talking to the guide. Opens the review.
//	@Tags			15. Bookings
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string				true	"Booking ID"	format(uuid)
//	@Param			body	body		dto.CompleteRequest	false	"Optional note"
//	@Success		200		{object}	response.Body{data=dto.BookingResponse}
//	@Failure		409		{object}	response.ErrorResponse	"BOOKING_STATUS_CONFLICT | TOUR_NOT_STARTED | BOOKING_NOT_PAID"
//	@Router			/bookings/{id}/complete [post]
func (h *Handler) Complete(c *gin.Context) {
	withID(c, func(id uuid.UUID) {
		var req dto.CompleteRequest
		_ = c.ShouldBindJSON(&req)
		res, err := h.svc.Complete(c.Request.Context(), web.UserID(c), id, req.Note)
		reply(c, http.StatusOK, "tour completed", res, err)
	})
}

// Cancel godoc
//
//	@Summary		Cancel a booking (tourist or guide, while pending or accepted-but-unpaid)
//	@Description	Paid bookings can only be cancelled by an admin (POST /admin/bookings/{id}/cancel, with refund).
//	@Tags		15. Bookings
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string				true	"Booking ID"	format(uuid)
//	@Param		body	body		dto.ReasonRequest	false	"Reason"
//	@Success	200		{object}	response.Body{data=dto.BookingResponse}
//	@Failure	409		{object}	response.ErrorResponse	"BOOKING_STATUS_CONFLICT | PAID_BOOKING_CANCEL"
//	@Router		/bookings/{id}/cancel [post]
func (h *Handler) Cancel(c *gin.Context) {
	withID(c, func(id uuid.UUID) {
		var req dto.ReasonRequest
		_ = c.ShouldBindJSON(&req)
		res, err := h.svc.Cancel(c.Request.Context(), web.UserID(c), web.Role(c), id, req.Reason)
		reply(c, http.StatusOK, "booking cancelled", res, err)
	})
}

// ================================================================== payments

// Methods godoc
//
//	@Summary		Manual fallback: where to send money (bKash / Nagad / Rocket numbers)
//	@Description	The main option is online payment (POST /bookings/{id}/pay, aamarPay). Fallback: send the booking total to one of these numbers ("Send Money"/"Payment"),
//	@Description	then submit the transaction ID with POST /bookings/{id}/payments.
//	@Tags			16. Payments
//	@Produce		json
//	@Success		200	{object}	response.Body{data=[]dto.PaymentMethodResponse}
//	@Router			/payment-methods [get]
func (h *Handler) Methods(c *gin.Context) {
	res, err := h.svc.Methods(c.Request.Context(), true)
	reply(c, http.StatusOK, "", res, err)
}

// SubmitPayment godoc
//
//	@Summary		Manual fallback: submit a mobile-banking payment for an accepted booking
//	@Description	Only after the guide accepts (`can_pay`). `amount` must equal `total_amount`. The payment stays
//	@Description	`pending` until an admin verifies it; then the booking's `payment_status` becomes `paid`.
//	@Tags			16. Payments
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string						true	"Booking ID"	format(uuid)
//	@Param			body	body		dto.SubmitPaymentRequest	true	"Payment"
//	@Success		201		{object}	response.Body{data=dto.PaymentResponse}
//	@Failure		409		{object}	response.ErrorResponse	"PAYMENT_NOT_DUE | ALREADY_PAID | TRANSACTION_ID_USED"
//	@Failure		422		{object}	response.ErrorResponse	"PAYMENT_AMOUNT_MISMATCH"
//	@Router			/bookings/{id}/payments [post]
func (h *Handler) SubmitPayment(c *gin.Context) {
	withID(c, func(id uuid.UUID) {
		var req dto.SubmitPaymentRequest
		if !web.BindJSON(c, &req) {
			return
		}
		res, err := h.svc.SubmitPayment(c.Request.Context(), web.UserID(c), id, req)
		reply(c, http.StatusCreated, "payment submitted, waiting for verification", res, err)
	})
}

// BookingPayments godoc
//
//	@Summary	Payments submitted for a booking
//	@Tags		16. Payments
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Booking ID"	format(uuid)
//	@Success	200	{object}	response.Body{data=[]dto.PaymentResponse}
//	@Router		/bookings/{id}/payments [get]
func (h *Handler) BookingPayments(c *gin.Context) {
	withID(c, func(id uuid.UUID) {
		res, err := h.svc.BookingPayments(c.Request.Context(), web.UserID(c), web.Role(c), id)
		reply(c, http.StatusOK, "", res, err)
	})
}

// ================================================================== admin

// AdminBookings godoc
//
//	@Summary	All bookings
//	@Tags		18. Admin · Marketplace
//	@Produce	json
//	@Security	BearerAuth
//	@Param		query	query		dto.BookingQuery	false	"Filters"
//	@Success	200		{object}	response.Body{data=dto.BookingPage}
//	@Router		/admin/bookings [get]
func (h *Handler) AdminBookings(c *gin.Context) {
	var q dto.BookingQuery
	if !web.BindQuery(c, &q) {
		return
	}
	res, err := h.svc.AdminList(c.Request.Context(), web.UserID(c), q)
	reply(c, http.StatusOK, "", res, err)
}

// AdminPayments godoc
//
//	@Summary	Payments to verify
//	@Tags		18. Admin · Marketplace
//	@Produce	json
//	@Security	BearerAuth
//	@Param		query	query		dto.PaymentQuery	false	"Filters (status=pending for the queue)"
//	@Success	200		{object}	response.Body{data=dto.PaymentPage}
//	@Router		/admin/payments [get]
func (h *Handler) AdminPayments(c *gin.Context) {
	var q dto.PaymentQuery
	if !web.BindQuery(c, &q) {
		return
	}
	res, err := h.svc.AdminPayments(c.Request.Context(), q)
	reply(c, http.StatusOK, "", res, err)
}

// VerifyPayment godoc
//
//	@Summary	Verify a payment (booking becomes paid)
//	@Tags		18. Admin · Marketplace
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string						true	"Payment ID"	format(uuid)
//	@Param		body	body		dto.ReviewPaymentRequest	false	"Note"
//	@Success	200		{object}	response.Body{data=dto.PaymentResponse}
//	@Failure	409		{object}	response.ErrorResponse	"PAYMENT_ALREADY_REVIEWED"
//	@Router		/admin/payments/{id}/verify [post]
func (h *Handler) VerifyPayment(c *gin.Context) { h.review(c, true) }

// RejectPayment godoc
//
//	@Summary	Reject a payment (booking goes back to unpaid)
//	@Tags		18. Admin · Marketplace
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string						true	"Payment ID"	format(uuid)
//	@Param		body	body		dto.ReviewPaymentRequest	false	"Reason shown to the tourist"
//	@Success	200		{object}	response.Body{data=dto.PaymentResponse}
//	@Failure	409		{object}	response.ErrorResponse	"PAYMENT_ALREADY_REVIEWED"
//	@Router		/admin/payments/{id}/reject [post]
func (h *Handler) RejectPayment(c *gin.Context) { h.review(c, false) }

func (h *Handler) review(c *gin.Context, verify bool) {
	withID(c, func(id uuid.UUID) {
		var req dto.ReviewPaymentRequest
		_ = c.ShouldBindJSON(&req)
		res, err := h.svc.ReviewPayment(c.Request.Context(), web.UserID(c), id, verify, req.Note)
		reply(c, http.StatusOK, "payment reviewed", res, err)
	})
}

// AdminMethods godoc
//
//	@Summary	All payment methods (incl. inactive)
//	@Tags		18. Admin · Marketplace
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	response.Body{data=[]dto.PaymentMethodResponse}
//	@Router		/admin/payment-methods [get]
func (h *Handler) AdminMethods(c *gin.Context) {
	res, err := h.svc.Methods(c.Request.Context(), false)
	reply(c, http.StatusOK, "", res, err)
}

// CreateMethod godoc
//
//	@Summary	Add a receiving number (bKash/Nagad/Rocket...)
//	@Tags		18. Admin · Marketplace
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		dto.PaymentMethodRequest	true	"Payment method"
//	@Success	201		{object}	response.Body{data=dto.PaymentMethodResponse}
//	@Router		/admin/payment-methods [post]
func (h *Handler) CreateMethod(c *gin.Context) {
	var req dto.PaymentMethodRequest
	if !web.BindJSON(c, &req) {
		return
	}
	res, err := h.svc.CreateMethod(c.Request.Context(), req)
	reply(c, http.StatusCreated, "payment method added", res, err)
}

// UpdateMethod godoc
//
//	@Summary	Update / (de)activate a payment method
//	@Tags		18. Admin · Marketplace
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string					true	"Method ID"	format(uuid)
//	@Param		body	body		dto.PaymentMethodPatch	true	"Fields to change"
//	@Success	200		{object}	response.Body{data=dto.PaymentMethodResponse}
//	@Router		/admin/payment-methods/{id} [patch]
func (h *Handler) UpdateMethod(c *gin.Context) {
	withID(c, func(id uuid.UUID) {
		var req dto.PaymentMethodPatch
		if !web.BindJSON(c, &req) {
			return
		}
		res, err := h.svc.UpdateMethod(c.Request.Context(), id, req)
		reply(c, http.StatusOK, "payment method updated", res, err)
	})
}

// DeleteMethod godoc
//
//	@Summary	Delete a payment method
//	@Tags		18. Admin · Marketplace
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Method ID"	format(uuid)
//	@Success	200	{object}	response.Body
//	@Router		/admin/payment-methods/{id} [delete]
func (h *Handler) DeleteMethod(c *gin.Context) {
	withID(c, func(id uuid.UUID) {
		reply(c, http.StatusOK, "payment method deleted", nil, h.svc.DeleteMethod(c.Request.Context(), id))
	})
}
