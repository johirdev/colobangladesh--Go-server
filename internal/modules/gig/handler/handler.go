// Package handler is the HTTP layer of the gig module (with swagger annotations).
package handler

import (
	"net/http"

	"cholobangladesh-server/internal/modules/gig/dto"
	"cholobangladesh-server/internal/modules/gig/service"
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

// ================================================================== guide

// MyGigs godoc
//
//	@Summary		My gigs + slot quota
//	@Description	`quota.remaining = 0` → show "get more slots" (FREE_GIG_LIMIT free gigs, default 5; extra slots are paid, coming later).
//	@Tags			13. Guide · My gigs
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	response.Body{data=dto.MyGigsResponse}
//	@Router			/gigs/me [get]
func (h *Handler) MyGigs(c *gin.Context) {
	res, err := h.svc.MyGigs(c.Request.Context(), web.UserID(c))
	reply(c, http.StatusOK, "", res, err)
}

// MyQuota godoc
//
//	@Summary	My gig slot quota
//	@Tags		13. Guide · My gigs
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	response.Body{data=dto.QuotaResponse}
//	@Router		/gigs/me/quota [get]
func (h *Handler) MyQuota(c *gin.Context) {
	res, err := h.svc.Quota(c.Request.Context(), web.UserID(c))
	reply(c, http.StatusOK, "", res, err)
}

// MyGig godoc
//
//	@Summary	One of my gigs (any status)
//	@Tags		13. Guide · My gigs
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Gig ID"	format(uuid)
//	@Success	200	{object}	response.Body{data=dto.GigResponse}
//	@Failure	404	{object}	response.ErrorResponse	"GIG_NOT_FOUND"
//	@Router		/gigs/me/{id} [get]
func (h *Handler) MyGig(c *gin.Context) {
	id, ok := web.UUIDParam(c, "id")
	if !ok {
		return
	}
	res, err := h.svc.MyGig(c.Request.Context(), web.UserID(c), id)
	reply(c, http.StatusOK, "", res, err)
}

// Create godoc
//
//	@Summary		Create a gig for a tourist spot
//	@Description	Verified guides only (KYC approved). One gig per tourist spot. Price is whole BDT,
//	@Description	`per_person` (× group size) or `per_group`. Returns 402 GIG_LIMIT_REACHED (with the quota in
//	@Description	`error.details`) when all slots are used.
//	@Tags			13. Guide · My gigs
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		dto.CreateGigRequest	true	"Gig"
//	@Success		201		{object}	response.Body{data=dto.GigResponse}
//	@Failure		402		{object}	response.ErrorResponse	"GIG_LIMIT_REACHED"
//	@Failure		403		{object}	response.ErrorResponse	"GUIDE_NOT_VERIFIED"
//	@Failure		409		{object}	response.ErrorResponse	"GIG_ALREADY_EXISTS"
//	@Failure		422		{object}	response.ErrorResponse	"SPOT_NOT_FOUND | VALIDATION_ERROR"
//	@Router			/gigs/me [post]
func (h *Handler) Create(c *gin.Context) {
	var req dto.CreateGigRequest
	if !web.BindJSON(c, &req) {
		return
	}
	res, err := h.svc.Create(c.Request.Context(), web.UserID(c), req)
	reply(c, http.StatusCreated, "gig created", res, err)
}

// Update godoc
//
//	@Summary		Update my gig (partial) — price, hours, group size, languages, status...
//	@Description	Pause with `{"status":"paused"}`, resume with `{"status":"active"}`. Suspended gigs cannot be edited.
//	@Tags			13. Guide · My gigs
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string					true	"Gig ID"	format(uuid)
//	@Param			body	body		dto.UpdateGigRequest	true	"Fields to change"
//	@Success		200		{object}	response.Body{data=dto.GigResponse}
//	@Failure		403		{object}	response.ErrorResponse	"GIG_SUSPENDED"
//	@Failure		404		{object}	response.ErrorResponse	"GIG_NOT_FOUND"
//	@Router			/gigs/me/{id} [patch]
func (h *Handler) Update(c *gin.Context) {
	id, ok := web.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req dto.UpdateGigRequest
	if !web.BindJSON(c, &req) {
		return
	}
	res, err := h.svc.Update(c.Request.Context(), web.UserID(c), id, req)
	reply(c, http.StatusOK, "gig updated", res, err)
}

// Delete godoc
//
//	@Summary	Delete my gig (frees a slot)
//	@Tags		13. Guide · My gigs
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Gig ID"	format(uuid)
//	@Success	200	{object}	response.Body
//	@Failure	404	{object}	response.ErrorResponse	"GIG_NOT_FOUND"
//	@Router		/gigs/me/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, ok := web.UUIDParam(c, "id")
	if !ok {
		return
	}
	reply(c, http.StatusOK, "gig deleted", nil, h.svc.Delete(c.Request.Context(), web.UserID(c), id))
}

// ================================================================== public

// List godoc
//
//	@Summary		Browse bookable gigs
//	@Description	Only active gigs of verified, available guides on published spots. Use `spot_id` on a tourist
//	@Description	spot page ("guides for this place"), `guide_id` on a guide profile.
//	@Tags			14. Gigs · Public
//	@Produce		json
//	@Param			query	query		dto.GigQuery	false	"Filters"
//	@Success		200		{object}	response.Body{data=dto.GigPage}
//	@Router			/gigs [get]
func (h *Handler) List(c *gin.Context) {
	var q dto.GigQuery
	if !web.BindQuery(c, &q) {
		return
	}
	res, err := h.svc.List(c.Request.Context(), q)
	reply(c, http.StatusOK, "", res, err)
}

// Get godoc
//
//	@Summary	Gig details (with guide and spot)
//	@Tags		14. Gigs · Public
//	@Produce	json
//	@Param		id	path		string	true	"Gig ID"	format(uuid)
//	@Success	200	{object}	response.Body{data=dto.GigResponse}
//	@Failure	404	{object}	response.ErrorResponse	"GIG_NOT_FOUND"
//	@Router		/gigs/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	id, ok := web.UUIDParam(c, "id")
	if !ok {
		return
	}
	res, err := h.svc.Get(c.Request.Context(), id)
	reply(c, http.StatusOK, "", res, err)
}

// ================================================================== admin

// AdminList godoc
//
//	@Summary	List all gigs (any status)
//	@Tags		18. Admin · Marketplace
//	@Produce	json
//	@Security	BearerAuth
//	@Param		query	query		dto.AdminGigQuery	false	"Filters"
//	@Success	200		{object}	response.Body{data=dto.GigPage}
//	@Router		/admin/gigs [get]
func (h *Handler) AdminList(c *gin.Context) {
	var q dto.AdminGigQuery
	if !web.BindQuery(c, &q) {
		return
	}
	res, err := h.svc.AdminList(c.Request.Context(), q)
	reply(c, http.StatusOK, "", res, err)
}

// AdminSetStatus godoc
//
//	@Summary	Suspend / re-activate a gig
//	@Tags		18. Admin · Marketplace
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string						true	"Gig ID"	format(uuid)
//	@Param		body	body		dto.AdminGigStatusRequest	true	"Status"
//	@Success	200		{object}	response.Body{data=dto.GigResponse}
//	@Router		/admin/gigs/{id}/status [patch]
func (h *Handler) AdminSetStatus(c *gin.Context) {
	id, ok := web.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req dto.AdminGigStatusRequest
	if !web.BindJSON(c, &req) {
		return
	}
	res, err := h.svc.AdminSetStatus(c.Request.Context(), id, req)
	reply(c, http.StatusOK, "gig status updated", res, err)
}

// AdminSetQuota godoc
//
//	@Summary		Grant extra gig slots to a guide
//	@Description	Total gigs allowed = FREE_GIG_LIMIT + extra_slots. Until online slot purchase exists, admins
//	@Description	grant slots after the guide pays (keep the payment reference in `note`).
//	@Tags			18. Admin · Marketplace
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string					true	"Guide user ID"	format(uuid)
//	@Param			body	body		dto.SetQuotaRequest		true	"Extra slots"
//	@Success		200		{object}	response.Body{data=dto.QuotaResponse}
//	@Router			/admin/guides/{id}/gig-quota [put]
func (h *Handler) AdminSetQuota(c *gin.Context) {
	id, ok := web.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req dto.SetQuotaRequest
	if !web.BindJSON(c, &req) {
		return
	}
	res, err := h.svc.AdminSetQuota(c.Request.Context(), web.UserID(c), id, req)
	reply(c, http.StatusOK, "gig quota updated", res, err)
}
