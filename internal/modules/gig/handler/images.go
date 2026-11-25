package handler

import (
	"net/http"

	"cholobangladesh-server/internal/modules/gig/dto"
	"cholobangladesh-server/internal/shared/web"
	"cholobangladesh-server/pkg/response"
	"cholobangladesh-server/pkg/validation"

	"github.com/gin-gonic/gin"
)

// ================================================================== gallery

// UploadImages godoc
//
//	@Summary		Upload photos to my gig
//	@Description	multipart/form-data: repeat `images` for several files (JPEG/PNG/WEBP, max MAX_UPLOAD_SIZE_MB
//	@Description	each, min 400x400 — crop them in the browser first). Optional `captions` in the same order.
//	@Description	Max 12 photos per gig; the first photo is the cover.
//	@Tags			13. Guide · My gigs
//	@Accept			multipart/form-data
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id			path		string	true	"Gig ID"	format(uuid)
//	@Param			images		formData	file	true	"Photo (repeatable)"
//	@Param			captions	formData	string	false	"Caption for each photo (repeatable)"
//	@Success		201			{object}	response.Body{data=dto.GigResponse}
//	@Failure		422			{object}	response.ErrorResponse	"INVALID_IMAGE | TOO_MANY_IMAGES"
//	@Failure		503			{object}	response.ErrorResponse	"UPLOAD_UNAVAILABLE"
//	@Router			/gigs/me/{id}/images [post]
func (h *Handler) UploadImages(c *gin.Context) {
	id, ok := web.UUIDParam(c, "id")
	if !ok {
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		response.Fail(c, validation.Field("images", "required", "send the photos as multipart/form-data"))
		return
	}
	res, err := h.svc.UploadImages(c.Request.Context(), web.UserID(c), id, form.File["images"], form.Value["captions"])
	reply(c, http.StatusCreated, "photos uploaded", res, err)
}

// ReplaceImage godoc
//
//	@Summary		Replace one photo (e.g. after re-cropping)
//	@Description	multipart/form-data with one `image`. Position and caption stay the same.
//	@Tags			13. Guide · My gigs
//	@Accept			multipart/form-data
//	@Produce		json
//	@Security		BearerAuth
//	@Param			id		path		string	true	"Gig ID"	format(uuid)
//	@Param			imageId	path		string	true	"Photo ID"	format(uuid)
//	@Param			image	formData	file	true	"New photo"
//	@Success		200		{object}	response.Body{data=dto.GigResponse}
//	@Failure		404		{object}	response.ErrorResponse	"IMAGE_NOT_FOUND"
//	@Router			/gigs/me/{id}/images/{imageId} [put]
func (h *Handler) ReplaceImage(c *gin.Context) {
	id, ok := web.UUIDParam(c, "id")
	if !ok {
		return
	}
	imageID, ok := web.UUIDParam(c, "imageId")
	if !ok {
		return
	}
	fh, err := c.FormFile("image")
	if err != nil {
		response.Fail(c, validation.Field("image", "required", "attach the new photo"))
		return
	}
	res, err := h.svc.ReplaceImage(c.Request.Context(), web.UserID(c), id, imageID, fh)
	reply(c, http.StatusOK, "photo replaced", res, err)
}

// UpdateImage godoc
//
//	@Summary	Edit a photo caption / make it the cover
//	@Tags		13. Guide · My gigs
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string					true	"Gig ID"	format(uuid)
//	@Param		imageId	path		string					true	"Photo ID"	format(uuid)
//	@Param		body	body		dto.UpdateImageRequest	true	"Changes"
//	@Success	200		{object}	response.Body{data=dto.GigResponse}
//	@Failure	404		{object}	response.ErrorResponse	"IMAGE_NOT_FOUND"
//	@Router		/gigs/me/{id}/images/{imageId} [patch]
func (h *Handler) UpdateImage(c *gin.Context) {
	id, ok := web.UUIDParam(c, "id")
	if !ok {
		return
	}
	imageID, ok := web.UUIDParam(c, "imageId")
	if !ok {
		return
	}
	var req dto.UpdateImageRequest
	if !web.BindJSON(c, &req) {
		return
	}
	res, err := h.svc.UpdateImage(c.Request.Context(), web.UserID(c), id, imageID, req)
	reply(c, http.StatusOK, "photo updated", res, err)
}

// DeleteImage godoc
//
//	@Summary	Delete one photo of my gig
//	@Tags		13. Guide · My gigs
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id		path		string	true	"Gig ID"	format(uuid)
//	@Param		imageId	path		string	true	"Photo ID"	format(uuid)
//	@Success	200		{object}	response.Body{data=dto.GigResponse}
//	@Failure	404		{object}	response.ErrorResponse	"IMAGE_NOT_FOUND"
//	@Router		/gigs/me/{id}/images/{imageId} [delete]
func (h *Handler) DeleteImage(c *gin.Context) {
	id, ok := web.UUIDParam(c, "id")
	if !ok {
		return
	}
	imageID, ok := web.UUIDParam(c, "imageId")
	if !ok {
		return
	}
	res, err := h.svc.DeleteImage(c.Request.Context(), web.UserID(c), id, imageID)
	reply(c, http.StatusOK, "photo deleted", res, err)
}
