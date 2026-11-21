// Package handler is the HTTP layer of the review module (with swagger annotations).
package handler

import (
	"net/http"

	"cholobangladesh-server/internal/modules/review/dto"
	"cholobangladesh-server/internal/modules/review/service"
	"cholobangladesh-server/internal/shared/web"
	"cholobangladesh-server/pkg/response"

	"github.com/gin-gonic/gin"
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

// Create godoc
//
//	@Summary		Review a completed tour (with photos)
//	@Description	multipart/form-data. Only the tourist of a **completed** booking, once. `rating` 1-5, optional
//	@Description	`comment` (max 2000) and up to 5 `images` (JPEG/PNG/WEBP). Updates gig, guide and spot ratings.
//	@Tags			17. Reviews
//	@Accept			multipart/form-data
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string	true	"Booking ID"	format(uuid)
//	@Param			rating	formData	int		true	"1-5 stars"
//	@Param			comment	formData	string	false	"Your experience"
//	@Param			images	formData	file	false	"Photo (repeatable, max 5)"
//	@Success		201		{object}	response.Body{data=dto.ReviewResponse}
//	@Failure		409		{object}	response.ErrorResponse	"TOUR_NOT_COMPLETED | ALREADY_REVIEWED"
//	@Failure		422		{object}	response.ErrorResponse	"VALIDATION_ERROR | TOO_MANY_IMAGES | INVALID_IMAGE"
//	@Router			/bookings/{id}/review [post]
func (h *Handler) Create(c *gin.Context) {
	id, ok := web.UUIDParam(c, "id")
	if !ok {
		return
	}
	in := service.CreateInput{Rating: c.PostForm("rating"), Comment: c.PostForm("comment")}
	if form, err := c.MultipartForm(); err == nil {
		in.Images = form.File["images"]
	}
	res, err := h.svc.Create(c.Request.Context(), web.UserID(c), id, in)
	reply(c, http.StatusCreated, "thank you for your review", res, err)
}

// List godoc
//
//	@Summary		Reviews for a gig, a guide or a tourist spot
//	@Description	Pass one of `gig_id`, `guide_id`, `spot_id`. Includes a star breakdown summary.
//	@Tags			17. Reviews
//	@Produce		json
//	@Param			query	query		dto.ReviewQuery	false	"Filters"
//	@Success		200		{object}	response.Body{data=dto.ReviewList}
//	@Router			/reviews [get]
func (h *Handler) List(c *gin.Context) {
	var q dto.ReviewQuery
	if !web.BindQuery(c, &q) {
		return
	}
	res, err := h.svc.List(c.Request.Context(), q, false)
	reply(c, http.StatusOK, "", res, err)
}

// Reply godoc
//
//	@Summary	Reply to a review of my tour (guide, once)
//	@Tags		17. Reviews
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string				true	"Review ID"	format(uuid)
//	@Param		body	body		dto.ReplyRequest	true	"Reply"
//	@Success	200		{object}	response.Body{data=dto.ReviewResponse}
//	@Failure	409		{object}	response.ErrorResponse	"ALREADY_REPLIED"
//	@Router		/reviews/{id}/reply [post]
func (h *Handler) Reply(c *gin.Context) {
	id, ok := web.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req dto.ReplyRequest
	if !web.BindJSON(c, &req) {
		return
	}
	res, err := h.svc.Reply(c.Request.Context(), web.UserID(c), id, req.Reply)
	reply(c, http.StatusOK, "reply posted", res, err)
}

// AdminList godoc
//
//	@Summary	All reviews incl. hidden
//	@Tags		18. Admin · Marketplace
//	@Produce	json
//	@Security	BearerAuth
//	@Param		query	query		dto.ReviewQuery	false	"Filters"
//	@Success	200		{object}	response.Body{data=dto.ReviewList}
//	@Router		/admin/reviews [get]
func (h *Handler) AdminList(c *gin.Context) {
	var q dto.ReviewQuery
	if !web.BindQuery(c, &q) {
		return
	}
	res, err := h.svc.List(c.Request.Context(), q, true)
	reply(c, http.StatusOK, "", res, err)
}

// SetVisibility godoc
//
//	@Summary	Hide / show a review (ratings are recalculated)
//	@Tags		18. Admin · Marketplace
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string					true	"Review ID"	format(uuid)
//	@Param		body	body		dto.VisibilityRequest	true	"Visibility"
//	@Success	200		{object}	response.Body{data=dto.ReviewResponse}
//	@Router		/admin/reviews/{id}/visibility [patch]
func (h *Handler) SetVisibility(c *gin.Context) {
	id, ok := web.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req dto.VisibilityRequest
	if !web.BindJSON(c, &req) {
		return
	}
	res, err := h.svc.SetVisibility(c.Request.Context(), id, *req.IsVisible)
	reply(c, http.StatusOK, "review updated", res, err)
}
