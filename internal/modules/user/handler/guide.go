package handler

import (
	"net/http"

	"cholobangladesh-server/internal/modules/user/dto"
	"cholobangladesh-server/internal/modules/user/service"
	"cholobangladesh-server/pkg/response"
	"cholobangladesh-server/pkg/validation"

	"github.com/gin-gonic/gin"
)

func fileRequired(field string) error { return validation.Field(field, "required", "file is required") }

// UpsertGuideProfile godoc
//
//	@Summary		Create / replace my guide profile
//	@Description	Full replace (PUT). Either `hourly_rate` or `daily_rate` is required (BDT, whole taka).
//	@Description	`languages`: ISO 639-1 codes (bn, en, hi...). `service_areas`: lowercase slugs (dhaka, sylhet,
//	@Description	coxs-bazar...). `specialties`: see GET /guides/options.
//	@Tags			6. Guide · My guide profile & KYC
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		dto.UpsertGuideProfileRequest	true	"Guide profile"
//	@Success		200		{object}	response.Body{data=dto.MeResponse}
//	@Failure		403		{object}	response.ErrorResponse	"FORBIDDEN (not a guide)"
//	@Failure		422		{object}	response.ErrorResponse	"VALIDATION_ERROR"
//	@Router			/guides/me/profile [put]
func (h *Handler) UpsertGuideProfile(c *gin.Context) {
	var req dto.UpsertGuideProfileRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.UpsertGuideProfile(c.Request.Context(), uid(c), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "guide profile saved", res)
}

// SetAvailability godoc
//
//	@Summary	Toggle availability (hide/show in search)
//	@Tags		6. Guide · My guide profile & KYC
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		dto.AvailabilityRequest	true	"Availability"
//	@Success	200		{object}	response.Body{data=dto.MeResponse}
//	@Router		/guides/me/availability [patch]
func (h *Handler) SetAvailability(c *gin.Context) {
	var req dto.AvailabilityRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.SetAvailability(c.Request.Context(), uid(c), *req.IsAvailable)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "availability updated", res)
}

// SubmitVerification godoc
//
//	@Summary		Submit identity verification (NID + selfie)
//	@Description	multipart/form-data. Allowed only when `onboarding.next_step` is `submit_verification` or
//	@Description	`resubmit_verification` (profile must be complete, see `GUIDE_PROFILE_INCOMPLETE.details`).
//	@Description	Images: JPEG/PNG/WEBP, max MAX_UPLOAD_SIZE_MB each, min 300x300. Documents are stored privately
//	@Description	(never public URLs) and the NID number is encrypted (AES-256-GCM). An admin reviews it;
//	@Description	the guide gets an SMS with the result.
//	@Tags			6. Guide · My guide profile & KYC
//	@Accept			multipart/form-data
//	@Produce		json
//	@Security		BearerAuth
//	@Param			nid_number	formData	string	true	"NID number (10, 13 or 17 digits)"
//	@Param			nid_front	formData	file	true	"Front side of NID"
//	@Param			nid_back	formData	file	true	"Back side of NID"
//	@Param			selfie		formData	file	true	"Selfie holding the NID"
//	@Success		201			{object}	response.Body{data=dto.VerificationStatusResponse}
//	@Failure		409			{object}	response.ErrorResponse	"VERIFICATION_PENDING | ALREADY_VERIFIED"
//	@Failure		422			{object}	response.ErrorResponse	"GUIDE_PROFILE_INCOMPLETE | VALIDATION_ERROR"
//	@Failure		503			{object}	response.ErrorResponse	"UPLOAD_UNAVAILABLE"
//	@Router			/guides/me/verification [post]
func (h *Handler) SubmitVerification(c *gin.Context) {
	in := service.KYCInput{NIDNumber: c.PostForm("nid_number")}
	var err error
	if in.NIDFront, err = c.FormFile("nid_front"); err != nil {
		response.Fail(c, fileRequired("nid_front"))
		return
	}
	if in.NIDBack, err = c.FormFile("nid_back"); err != nil {
		response.Fail(c, fileRequired("nid_back"))
		return
	}
	if in.Selfie, err = c.FormFile("selfie"); err != nil {
		response.Fail(c, fileRequired("selfie"))
		return
	}
	res, err := h.svc.SubmitVerification(c.Request.Context(), uid(c), in, meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusCreated, "verification submitted for review", res)
}

// GetMyVerification godoc
//
//	@Summary	Latest verification status
//	@Tags		6. Guide · My guide profile & KYC
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	response.Body{data=dto.VerificationStatusResponse}
//	@Failure	404	{object}	response.ErrorResponse	"VERIFICATION_NOT_FOUND"
//	@Router		/guides/me/verification [get]
func (h *Handler) GetMyVerification(c *gin.Context) {
	res, err := h.svc.GetMyVerification(c.Request.Context(), uid(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "", res)
}

// SearchGuides godoc
//
//	@Summary		Browse verified guides (public)
//	@Description	Only approved, active and available guides are listed. Contact info is never exposed.
//	@Tags			7. Guides · Public
//	@Produce		json
//	@Param			query	query		dto.GuideSearchQuery	false	"Filters"
//	@Success		200		{object}	response.Body{data=dto.GuidePage}
//	@Failure		422		{object}	response.ErrorResponse	"VALIDATION_ERROR"
//	@Router			/guides [get]
func (h *Handler) SearchGuides(c *gin.Context) {
	var q dto.GuideSearchQuery
	if !bindQuery(c, &q) {
		return
	}
	res, err := h.svc.SearchGuides(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "", res)
}

// GetGuide godoc
//
//	@Summary	Public guide profile
//	@Tags		7. Guides · Public
//	@Produce	json
//	@Param		id	path		string	true	"Guide user ID"	format(uuid)
//	@Success	200	{object}	response.Body{data=dto.PublicGuideResponse}
//	@Failure	404	{object}	response.ErrorResponse	"GUIDE_NOT_FOUND"
//	@Router		/guides/{id} [get]
func (h *Handler) GetGuide(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	res, err := h.svc.GetPublicGuide(c.Request.Context(), id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "", res)
}

// GuideOptions godoc
//
//	@Summary	Allowed values for guide filters/forms
//	@Tags		7. Guides · Public
//	@Produce	json
//	@Success	200	{object}	response.Body{data=dto.GuideOptionsResponse}
//	@Router		/guides/options [get]
func (h *Handler) GuideOptions(c *gin.Context) {
	response.OK(c, http.StatusOK, "", h.svc.GuideOptions())
}
