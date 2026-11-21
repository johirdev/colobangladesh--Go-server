// Package handler is the HTTP layer of the geo module (with swagger annotations).
package handler

import (
	"net/http"
	"strconv"

	"cholobangladesh-server/internal/modules/geo/dto"
	"cholobangladesh-server/internal/modules/geo/service"
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

// ================================================================== public

// ListDivisions godoc
//
//	@Summary	List the 8 divisions (বিভাগ)
//	@Tags		9. Geo · Public
//	@Produce	json
//	@Success	200	{object}	response.Body{data=[]dto.DivisionResponse}
//	@Router		/geo/divisions [get]
func (h *Handler) ListDivisions(c *gin.Context) {
	res, err := h.svc.ListDivisions(c.Request.Context())
	reply(c, http.StatusOK, "", res, err)
}

// GetDivision godoc
//
//	@Summary	Get a division
//	@Tags		9. Geo · Public
//	@Produce	json
//	@Param		id	path		int	true	"Division ID"
//	@Success	200	{object}	response.Body{data=dto.DivisionResponse}
//	@Failure	404	{object}	response.ErrorResponse	"DIVISION_NOT_FOUND"
//	@Router		/geo/divisions/{id} [get]
func (h *Handler) GetDivision(c *gin.Context) {
	id, ok := web.IntParam(c, "id")
	if !ok {
		return
	}
	res, err := h.svc.GetDivision(c.Request.Context(), id)
	reply(c, http.StatusOK, "", res, err)
}

// ListDistricts godoc
//
//	@Summary		List districts (জেলা) — all 64 or one division's
//	@Description	Each district has `latitude`/`longitude` (for map markers) and `spot_count` (published tourist spots).
//	@Tags			9. Geo · Public
//	@Produce		json
//	@Param			query	query		dto.DistrictQuery	false	"Filters"
//	@Success		200		{object}	response.Body{data=[]dto.DistrictResponse}
//	@Router			/geo/districts [get]
func (h *Handler) ListDistricts(c *gin.Context) {
	var q dto.DistrictQuery
	if !web.BindQuery(c, &q) {
		return
	}
	res, err := h.svc.ListDistricts(c.Request.Context(), q)
	reply(c, http.StatusOK, "", res, err)
}

// GetDistrict godoc
//
//	@Summary	Get a district by ID or slug
//	@Tags		9. Geo · Public
//	@Produce	json
//	@Param		id	path		string	true	"District ID (e.g. 9) or slug (e.g. coxs-bazar)"
//	@Success	200	{object}	response.Body{data=dto.DistrictResponse}
//	@Failure	404	{object}	response.ErrorResponse	"DISTRICT_NOT_FOUND"
//	@Router		/geo/districts/{id} [get]
func (h *Handler) GetDistrict(c *gin.Context) {
	raw := c.Param("id")
	id, _ := strconv.Atoi(raw)
	res, err := h.svc.GetDistrict(c.Request.Context(), id, raw)
	reply(c, http.StatusOK, "", res, err)
}

// ListUpazilas godoc
//
//	@Summary		List upazilas / thanas (উপজেলা/থানা)
//	@Description	Filter by `district_id` for a cascading dropdown. Includes Dhaka & Chattogram metropolitan
//	@Description	thanas (`type = metro_thana`). Default limit 100, max 600.
//	@Tags			9. Geo · Public
//	@Produce		json
//	@Param			query	query		dto.UpazilaQuery	false	"Filters"
//	@Success		200		{object}	response.Body{data=dto.UpazilaPage}
//	@Router			/geo/upazilas [get]
func (h *Handler) ListUpazilas(c *gin.Context) {
	var q dto.UpazilaQuery
	if !web.BindQuery(c, &q) {
		return
	}
	res, err := h.svc.ListUpazilas(c.Request.Context(), q)
	reply(c, http.StatusOK, "", res, err)
}

// GetUpazila godoc
//
//	@Summary	Get an upazila / thana
//	@Tags		9. Geo · Public
//	@Produce	json
//	@Param		id	path		int	true	"Upazila ID"
//	@Success	200	{object}	response.Body{data=dto.UpazilaResponse}
//	@Failure	404	{object}	response.ErrorResponse	"UPAZILA_NOT_FOUND"
//	@Router		/geo/upazilas/{id} [get]
func (h *Handler) GetUpazila(c *gin.Context) {
	id, ok := web.IntParam(c, "id")
	if !ok {
		return
	}
	res, err := h.svc.GetUpazila(c.Request.Context(), id)
	reply(c, http.StatusOK, "", res, err)
}

// Tree godoc
//
//	@Summary		Full hierarchy: division → district → upazila
//	@Description	One call for cascading dropdowns (≈ 60 KB, cached). Use the list endpoints for large screens.
//	@Tags			9. Geo · Public
//	@Produce		json
//	@Success		200	{object}	response.Body{data=[]dto.TreeDivision}
//	@Router			/geo/tree [get]
func (h *Handler) Tree(c *gin.Context) {
	res, err := h.svc.Tree(c.Request.Context())
	if err == nil {
		c.Header("Cache-Control", "public, max-age=300")
	}
	reply(c, http.StatusOK, "", res, err)
}

// ================================================================== manage

// CreateDivision godoc
//
//	@Summary		Create a division
//	@Description	`slug` is generated from `name` when omitted.
//	@Tags			11. Manage · Geo
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		dto.DivisionRequest	true	"Division"
//	@Success		201		{object}	response.Body{data=dto.DivisionResponse}
//	@Failure		409		{object}	response.ErrorResponse	"SLUG_TAKEN"
//	@Router			/manage/geo/divisions [post]
func (h *Handler) CreateDivision(c *gin.Context) {
	var req dto.DivisionRequest
	if !web.BindJSON(c, &req) {
		return
	}
	res, err := h.svc.CreateDivision(c.Request.Context(), req)
	reply(c, http.StatusCreated, "division created", res, err)
}

// UpdateDivision godoc
//
//	@Summary	Update a division (partial)
//	@Tags		11. Manage · Geo
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		int					true	"Division ID"
//	@Param		body	body		dto.DivisionPatch	true	"Fields to change"
//	@Success	200		{object}	response.Body{data=dto.DivisionResponse}
//	@Failure	404		{object}	response.ErrorResponse	"DIVISION_NOT_FOUND"
//	@Router		/manage/geo/divisions/{id} [patch]
func (h *Handler) UpdateDivision(c *gin.Context) {
	id, ok := web.IntParam(c, "id")
	if !ok {
		return
	}
	var req dto.DivisionPatch
	if !web.BindJSON(c, &req) {
		return
	}
	res, err := h.svc.UpdateDivision(c.Request.Context(), id, req)
	reply(c, http.StatusOK, "division updated", res, err)
}

// DeleteDivision godoc
//
//	@Summary	Delete a division (only when it has no districts)
//	@Tags		11. Manage · Geo
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		int	true	"Division ID"
//	@Success	200	{object}	response.Body
//	@Failure	409	{object}	response.ErrorResponse	"GEO_IN_USE"
//	@Router		/manage/geo/divisions/{id} [delete]
func (h *Handler) DeleteDivision(c *gin.Context) {
	id, ok := web.IntParam(c, "id")
	if !ok {
		return
	}
	reply(c, http.StatusOK, "division deleted", nil, h.svc.DeleteDivision(c.Request.Context(), id))
}

// CreateDistrict godoc
//
//	@Summary	Create a district
//	@Tags		11. Manage · Geo
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		dto.DistrictRequest	true	"District"
//	@Success	201		{object}	response.Body{data=dto.DistrictResponse}
//	@Failure	404		{object}	response.ErrorResponse	"DIVISION_NOT_FOUND"
//	@Failure	409		{object}	response.ErrorResponse	"SLUG_TAKEN"
//	@Router		/manage/geo/districts [post]
func (h *Handler) CreateDistrict(c *gin.Context) {
	var req dto.DistrictRequest
	if !web.BindJSON(c, &req) {
		return
	}
	res, err := h.svc.CreateDistrict(c.Request.Context(), req)
	reply(c, http.StatusCreated, "district created", res, err)
}

// UpdateDistrict godoc
//
//	@Summary	Update a district (partial)
//	@Tags		11. Manage · Geo
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		int					true	"District ID"
//	@Param		body	body		dto.DistrictPatch	true	"Fields to change"
//	@Success	200		{object}	response.Body{data=dto.DistrictResponse}
//	@Failure	404		{object}	response.ErrorResponse	"DISTRICT_NOT_FOUND"
//	@Router		/manage/geo/districts/{id} [patch]
func (h *Handler) UpdateDistrict(c *gin.Context) {
	id, ok := web.IntParam(c, "id")
	if !ok {
		return
	}
	var req dto.DistrictPatch
	if !web.BindJSON(c, &req) {
		return
	}
	res, err := h.svc.UpdateDistrict(c.Request.Context(), id, req)
	reply(c, http.StatusOK, "district updated", res, err)
}

// DeleteDistrict godoc
//
//	@Summary	Delete a district (only when nothing references it)
//	@Tags		11. Manage · Geo
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		int	true	"District ID"
//	@Success	200	{object}	response.Body
//	@Failure	409	{object}	response.ErrorResponse	"GEO_IN_USE"
//	@Router		/manage/geo/districts/{id} [delete]
func (h *Handler) DeleteDistrict(c *gin.Context) {
	id, ok := web.IntParam(c, "id")
	if !ok {
		return
	}
	reply(c, http.StatusOK, "district deleted", nil, h.svc.DeleteDistrict(c.Request.Context(), id))
}

// CreateUpazila godoc
//
//	@Summary	Create an upazila / thana
//	@Tags		11. Manage · Geo
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		dto.UpazilaRequest	true	"Upazila"
//	@Success	201		{object}	response.Body{data=dto.UpazilaResponse}
//	@Failure	404		{object}	response.ErrorResponse	"DISTRICT_NOT_FOUND"
//	@Failure	409		{object}	response.ErrorResponse	"SLUG_TAKEN"
//	@Router		/manage/geo/upazilas [post]
func (h *Handler) CreateUpazila(c *gin.Context) {
	var req dto.UpazilaRequest
	if !web.BindJSON(c, &req) {
		return
	}
	res, err := h.svc.CreateUpazila(c.Request.Context(), req)
	reply(c, http.StatusCreated, "upazila created", res, err)
}

// UpdateUpazila godoc
//
//	@Summary	Update an upazila / thana (partial)
//	@Tags		11. Manage · Geo
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		int					true	"Upazila ID"
//	@Param		body	body		dto.UpazilaPatch	true	"Fields to change"
//	@Success	200		{object}	response.Body{data=dto.UpazilaResponse}
//	@Failure	404		{object}	response.ErrorResponse	"UPAZILA_NOT_FOUND"
//	@Router		/manage/geo/upazilas/{id} [patch]
func (h *Handler) UpdateUpazila(c *gin.Context) {
	id, ok := web.IntParam(c, "id")
	if !ok {
		return
	}
	var req dto.UpazilaPatch
	if !web.BindJSON(c, &req) {
		return
	}
	res, err := h.svc.UpdateUpazila(c.Request.Context(), id, req)
	reply(c, http.StatusOK, "upazila updated", res, err)
}

// DeleteUpazila godoc
//
//	@Summary	Delete an upazila / thana (only when no tourist spot uses it)
//	@Tags		11. Manage · Geo
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		int	true	"Upazila ID"
//	@Success	200	{object}	response.Body
//	@Failure	409	{object}	response.ErrorResponse	"GEO_IN_USE"
//	@Router		/manage/geo/upazilas/{id} [delete]
func (h *Handler) DeleteUpazila(c *gin.Context) {
	id, ok := web.IntParam(c, "id")
	if !ok {
		return
	}
	reply(c, http.StatusOK, "upazila deleted", nil, h.svc.DeleteUpazila(c.Request.Context(), id))
}
