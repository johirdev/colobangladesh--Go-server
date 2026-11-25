// Package handler is the HTTP layer of the tourist spot module (with swagger annotations).
package handler

import (
	"net/http"

	"cholobangladesh-server/internal/modules/spot/domain"
	"cholobangladesh-server/internal/modules/spot/dto"
	"cholobangladesh-server/internal/modules/spot/service"
	"cholobangladesh-server/internal/shared/web"
	"cholobangladesh-server/pkg/response"
	"cholobangladesh-server/pkg/validation"

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

// ================================================================== public

// List godoc
//
//	@Summary		Browse tourist spots (published)
//	@Description	Grid/search for tourists. Filter by division, district (also matches spots that span a
//	@Description	district through a second upazila), upazila, category, featured or free text.
//	@Tags			10. Tourist spots · Public
//	@Produce		json
//	@Param			query	query		dto.SpotQuery	false	"Filters"
//	@Success		200		{object}	response.Body{data=dto.SpotPage}
//	@Router			/tourist-spots [get]
func (h *Handler) List(c *gin.Context) {
	var q dto.SpotQuery
	if !web.BindQuery(c, &q) {
		return
	}
	res, err := h.svc.List(c.Request.Context(), q, domain.StatusPublished)
	reply(c, http.StatusOK, "", res, err)
}

// Map godoc
//
//	@Summary		Map markers for the Bangladesh map
//	@Description	Every published spot with coordinates, cover image, title and rating — enough for the
//	@Description	64-district hero map and its hover card. Group by `district_id` / `district_slug` to colour districts.
//	@Tags			10. Tourist spots · Public
//	@Produce		json
//	@Param			query	query		dto.MarkerQuery	false	"Filters"
//	@Success		200		{object}	response.Body{data=[]dto.MapMarker}
//	@Router			/tourist-spots/map [get]
func (h *Handler) Map(c *gin.Context) {
	var q dto.MarkerQuery
	if !web.BindQuery(c, &q) {
		return
	}
	res, err := h.svc.Markers(c.Request.Context(), q)
	if err == nil {
		c.Header("Cache-Control", "public, max-age=60")
	}
	reply(c, http.StatusOK, "", res, err)
}

// Get godoc
//
//	@Summary		Tourist spot details
//	@Description	Full page: description, highlights, all images (1-50), YouTube videos (with embed/thumbnail
//	@Description	URLs), upazilas/thanas, Google Maps link. Accepts the UUID or the slug.
//	@Tags			10. Tourist spots · Public
//	@Produce		json
//	@Param			id	path		string	true	"Spot ID or slug (e.g. sajek-valley)"
//	@Success		200	{object}	response.Body{data=dto.SpotDetail}
//	@Failure		404	{object}	response.ErrorResponse	"SPOT_NOT_FOUND"
//	@Router			/tourist-spots/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	res, err := h.svc.Get(c.Request.Context(), c.Param("id"), true)
	reply(c, http.StatusOK, "", res, err)
}

// Options godoc
//
//	@Summary	Allowed categories / statuses / limits
//	@Tags		10. Tourist spots · Public
//	@Produce	json
//	@Success	200	{object}	response.Body{data=dto.OptionsResponse}
//	@Router		/tourist-spots/options [get]
func (h *Handler) Options(c *gin.Context) {
	response.OK(c, http.StatusOK, "", h.svc.Options())
}

// ================================================================== manage

// ManageList godoc
//
//	@Summary	List all tourist spots (drafts, published, archived)
//	@Tags		12. Manage · Tourist spots
//	@Produce	json
//	@Security	BearerAuth
//	@Param		query	query		dto.ManageSpotQuery	false	"Filters"
//	@Success	200		{object}	response.Body{data=dto.SpotPage}
//	@Router		/manage/tourist-spots [get]
func (h *Handler) ManageList(c *gin.Context) {
	var q dto.ManageSpotQuery
	if !web.BindQuery(c, &q) {
		return
	}
	res, err := h.svc.ManageList(c.Request.Context(), q)
	reply(c, http.StatusOK, "", res, err)
}

// Stats godoc
//
//	@Summary		Content studio counters
//	@Description	Spot counts by status, featured, "needs work" (no photos / no videos / no map coordinates),
//	@Description	spots with guide gigs, districts covered by published spots and spots per category.
//	@Tags			12. Manage · Tourist spots
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	response.Body{data=dto.SpotStats}
//	@Router			/manage/tourist-spots/stats [get]
func (h *Handler) Stats(c *gin.Context) {
	res, err := h.svc.Stats(c.Request.Context())
	reply(c, http.StatusOK, "", res, err)
}

// Bulk godoc
//
//	@Summary		Bulk action on tourist spots
//	@Description	Apply one action to 1-100 spots: publish, unpublish (back to draft), archive, feature, unfeature
//	@Description	or delete. Spots that cannot take the action are listed in data.skipped (SPOT_NOT_FOUND,
//	@Description	SPOT_HAS_ACTIVE_GIGS for delete) — the rest are still changed.
//	@Tags			12. Manage · Tourist spots
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		dto.BulkSpotRequest	true	"IDs + action"
//	@Success		200		{object}	response.Body{data=dto.BulkSpotResponse}
//	@Failure		422		{object}	response.ErrorResponse	"VALIDATION_ERROR"
//	@Router			/manage/tourist-spots/bulk [post]
func (h *Handler) Bulk(c *gin.Context) {
	var req dto.BulkSpotRequest
	if !web.BindJSON(c, &req) {
		return
	}
	res, err := h.svc.Bulk(c.Request.Context(), web.UserID(c), req)
	reply(c, http.StatusOK, "bulk action applied", res, err)
}

// ManageGet godoc
//
//	@Summary	Get any tourist spot (any status)
//	@Tags		12. Manage · Tourist spots
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Spot ID or slug"
//	@Success	200	{object}	response.Body{data=dto.SpotDetail}
//	@Failure	404	{object}	response.ErrorResponse	"SPOT_NOT_FOUND"
//	@Router		/manage/tourist-spots/{id} [get]
func (h *Handler) ManageGet(c *gin.Context) {
	res, err := h.svc.Get(c.Request.Context(), c.Param("id"), false)
	reply(c, http.StatusOK, "", res, err)
}

// Create godoc
//
//	@Summary		Create a tourist spot
//	@Description	Admins and moderators. Images can be URLs here (e.g. from autofill) and/or uploaded later via
//	@Description	POST /manage/tourist-spots/{id}/images. The first image becomes the cover. `status` defaults to `draft`.
//	@Description	`google_map_url` is generated from the coordinates when omitted.
//	@Tags			12. Manage · Tourist spots
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		dto.CreateSpotRequest	true	"Tourist spot"
//	@Success		201		{object}	response.Body{data=dto.SpotDetail}
//	@Failure		409		{object}	response.ErrorResponse	"SLUG_TAKEN"
//	@Failure		422		{object}	response.ErrorResponse	"VALIDATION_ERROR | UPAZILA_NOT_FOUND | INVALID_YOUTUBE_URL"
//	@Router			/manage/tourist-spots [post]
func (h *Handler) Create(c *gin.Context) {
	var req dto.CreateSpotRequest
	if !web.BindJSON(c, &req) {
		return
	}
	res, err := h.svc.Create(c.Request.Context(), web.UserID(c), req)
	reply(c, http.StatusCreated, "tourist spot created", res, err)
}

// Update godoc
//
//	@Summary		Update a tourist spot (partial)
//	@Description	Only sent fields change. `upazila_ids`, `images`, `videos` replace the whole list when sent.
//	@Description	Publish / unpublish with `{"status":"published"}` or `{"status":"draft"}`.
//	@Tags			12. Manage · Tourist spots
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string					true	"Spot ID"	format(uuid)
//	@Param			body	body		dto.UpdateSpotRequest	true	"Fields to change"
//	@Success		200		{object}	response.Body{data=dto.SpotDetail}
//	@Failure		404		{object}	response.ErrorResponse	"SPOT_NOT_FOUND"
//	@Failure		409		{object}	response.ErrorResponse	"SLUG_TAKEN"
//	@Router			/manage/tourist-spots/{id} [patch]
func (h *Handler) Update(c *gin.Context) {
	id, ok := web.UUIDParam(c, "id")
	if !ok {
		return
	}
	var req dto.UpdateSpotRequest
	if !web.BindJSON(c, &req) {
		return
	}
	res, err := h.svc.Update(c.Request.Context(), web.UserID(c), id, req)
	reply(c, http.StatusOK, "tourist spot updated", res, err)
}

// Delete godoc
//
//	@Summary		Delete a tourist spot
//	@Description	Refused while guides have gigs for it (archive it with status=archived instead).
//	@Tags			12. Manage · Tourist spots
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id	path		string	true	"Spot ID"	format(uuid)
//	@Success		200	{object}	response.Body
//	@Failure		409	{object}	response.ErrorResponse	"SPOT_HAS_ACTIVE_GIGS"
//	@Router			/manage/tourist-spots/{id} [delete]
func (h *Handler) Delete(c *gin.Context) {
	id, ok := web.UUIDParam(c, "id")
	if !ok {
		return
	}
	reply(c, http.StatusOK, "tourist spot deleted", nil, h.svc.Delete(c.Request.Context(), id))
}

// UploadImages godoc
//
//	@Summary		Upload photos to a tourist spot
//	@Description	multipart/form-data, repeat the `images` field for several files (JPEG/PNG/WEBP, max
//	@Description	MAX_UPLOAD_SIZE_MB each, min 400x400). Optional `captions` in the same order. Max 50 images per spot.
//	@Tags			12. Manage · Tourist spots
//	@Accept			multipart/form-data
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id			path		string	true	"Spot ID"	format(uuid)
//	@Param			images		formData	file	true	"Image file (repeatable)"
//	@Param			captions	formData	string	false	"Caption for each image (repeatable)"
//	@Success		201			{object}	response.Body{data=dto.SpotDetail}
//	@Failure		422			{object}	response.ErrorResponse	"INVALID_IMAGE | TOO_MANY_IMAGES"
//	@Failure		503			{object}	response.ErrorResponse	"UPLOAD_UNAVAILABLE"
//	@Router			/manage/tourist-spots/{id}/images [post]
func (h *Handler) UploadImages(c *gin.Context) {
	id, ok := web.UUIDParam(c, "id")
	if !ok {
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		response.Fail(c, validation.Field("images", "required", "send the files as multipart/form-data"))
		return
	}
	res, err := h.svc.UploadImages(c.Request.Context(), id, form.File["images"], form.Value["captions"])
	reply(c, http.StatusCreated, "images uploaded", res, err)
}

// DeleteImage godoc
//
//	@Summary	Remove one image from a tourist spot
//	@Tags		12. Manage · Tourist spots
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string	true	"Spot ID"	format(uuid)
//	@Param		imageId	path		string	true	"Image ID"	format(uuid)
//	@Success	200		{object}	response.Body
//	@Failure	404		{object}	response.ErrorResponse	"IMAGE_NOT_FOUND"
//	@Router		/manage/tourist-spots/{id}/images/{imageId} [delete]
func (h *Handler) DeleteImage(c *gin.Context) {
	id, ok := web.UUIDParam(c, "id")
	if !ok {
		return
	}
	imageID, ok := web.UUIDParam(c, "imageId")
	if !ok {
		return
	}
	reply(c, http.StatusOK, "image removed", nil, h.svc.DeleteImage(c.Request.Context(), id, imageID))
}

// Autofill godoc
//
//	@Summary		Auto-fill a tourist spot from its name (Wikipedia / Google / YouTube / AI)
//	@Description	Type a name (e.g. "Ratargul Swamp Forest") and get a ready draft: description, highlights,
//	@Description	coordinates, Google Maps link, photos, YouTube videos, suggested upazilas (nearest to the
//	@Description	coordinates) and category. Nothing is saved — review it, then POST `data.draft` to /manage/tourist-spots.
//	@Description	Wikipedia is always used; YouTube, Google image search and AI writing need their API keys
//	@Description	(see `data.notes`).
//	@Tags			12. Manage · Tourist spots
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		dto.AutofillRequest	true	"Spot name"
//	@Success		200		{object}	response.Body{data=dto.AutofillResponse}
//	@Failure		404		{object}	response.ErrorResponse	"AUTOFILL_NO_RESULT"
//	@Failure		502		{object}	response.ErrorResponse	"AUTOFILL_FAILED"
//	@Router			/manage/tourist-spots/autofill [post]
func (h *Handler) Autofill(c *gin.Context) {
	var req dto.AutofillRequest
	if !web.BindJSON(c, &req) {
		return
	}
	res, err := h.svc.Autofill(c.Request.Context(), req)
	reply(c, http.StatusOK, "", res, err)
}
