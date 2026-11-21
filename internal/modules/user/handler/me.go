package handler

import (
	"net/http"

	"cholobangladesh-server/internal/modules/user/dto"
	"cholobangladesh-server/pkg/response"

	"github.com/gin-gonic/gin"
)

const (
	chEmail = "email"
	chSMS   = "sms"
)

// GetMe godoc
//
//	@Summary		Get my account
//	@Description	Full private profile. Use `onboarding.next_step` to route the user to the right screen
//	@Description	after login (complete_profile | submit_verification | awaiting_review | resubmit_verification | done).
//	@Tags			5. Me · Profile & account
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	response.Body{data=dto.MeResponse}
//	@Failure		401	{object}	response.ErrorResponse	"UNAUTHORIZED | TOKEN_INVALID | SESSION_REVOKED"
//	@Router			/users/me [get]
func (h *Handler) GetMe(c *gin.Context) {
	res, err := h.svc.GetMe(c.Request.Context(), uid(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "", res)
}

// UpdateProfile godoc
//
//	@Summary		Update my personal profile (partial)
//	@Description	PATCH semantics — send only the fields that changed. Empty string clears an optional field.
//	@Description	Tourists must set `country_code` to finish onboarding. Guides must set `date_of_birth`
//	@Description	(18+), `city` and an avatar. A verified guide cannot change `date_of_birth`.
//	@Tags			5. Me · Profile & account
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		dto.UpdateProfileRequest	true	"Fields to update"
//	@Success		200		{object}	response.Body{data=dto.MeResponse}
//	@Failure		422		{object}	response.ErrorResponse	"VALIDATION_ERROR | UNDERAGE"
//	@Router			/users/me/profile [patch]
func (h *Handler) UpdateProfile(c *gin.Context) {
	var req dto.UpdateProfileRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.UpdateProfile(c.Request.Context(), uid(c), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "profile updated", res)
}

// UploadAvatar godoc
//
//	@Summary		Upload / replace profile photo
//	@Description	multipart/form-data with field `avatar`. JPEG, PNG or WEBP, max MAX_UPLOAD_SIZE_MB, min 100x100.
//	@Description	The file type is detected from its content, not its name.
//	@Tags			5. Me · Profile & account
//	@Accept			multipart/form-data
//	@Produce		json
//	@Security		BearerAuth
//	@Param			avatar	formData	file	true	"Image file"
//	@Success		200		{object}	response.Body{data=dto.MeResponse}
//	@Failure		413		{object}	response.ErrorResponse	"PAYLOAD_TOO_LARGE"
//	@Failure		422		{object}	response.ErrorResponse	"INVALID_IMAGE"
//	@Failure		503		{object}	response.ErrorResponse	"UPLOAD_UNAVAILABLE"
//	@Router			/users/me/avatar [put]
func (h *Handler) UploadAvatar(c *gin.Context) {
	fh, err := c.FormFile("avatar")
	if err != nil {
		response.Fail(c, fileRequired("avatar"))
		return
	}
	res, err := h.svc.UploadAvatar(c.Request.Context(), uid(c), fh)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "avatar updated", res)
}

// DeleteAvatar godoc
//
//	@Summary	Remove profile photo
//	@Tags		5. Me · Profile & account
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	response.Body{data=dto.MeResponse}
//	@Router		/users/me/avatar [delete]
func (h *Handler) DeleteAvatar(c *gin.Context) {
	res, err := h.svc.DeleteAvatar(c.Request.Context(), uid(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "avatar removed", res)
}

// ChangePassword godoc
//
//	@Summary		Change (or set first) password
//	@Description	`current_password` is required when `has_password` is true. Google/Apple-only accounts can
//	@Description	set a first password without it. All other devices are signed out.
//	@Tags			5. Me · Profile & account
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		dto.ChangePasswordRequest	true	"Passwords"
//	@Success		200		{object}	response.Body
//	@Failure		401		{object}	response.ErrorResponse	"WRONG_PASSWORD"
//	@Failure		422		{object}	response.ErrorResponse	"VALIDATION_ERROR | PASSWORD_UNCHANGED"
//	@Failure		429		{object}	response.ErrorResponse	"ACCOUNT_TEMPORARILY_LOCKED"
//	@Router			/users/me/password [post]
func (h *Handler) ChangePassword(c *gin.Context) {
	var req dto.ChangePasswordRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.ChangePassword(c.Request.Context(), uid(c), sid(c), req, meta(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "password updated", nil)
}

// SendEmailContactOTP godoc
//
//	@Summary		Add / change email · Step 1 — send OTP
//	@Description	Lets guides (who signed up with phone) add an email, or anyone change their email.
//	@Tags			5. Me · Profile & account
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		dto.EmailOTPRequest	true	"New email"
//	@Success		200		{object}	response.Body{data=dto.OTPSentResponse}
//	@Failure		409		{object}	response.ErrorResponse	"EMAIL_ALREADY_REGISTERED | CONTACT_ALREADY_VERIFIED"
//	@Router			/users/me/contacts/email/send-otp [post]
func (h *Handler) SendEmailContactOTP(c *gin.Context) {
	var req dto.EmailOTPRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.SendContactOTP(c.Request.Context(), uid(c), chEmail, req.Email, meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "verification code sent", res)
}

// VerifyEmailContact godoc
//
//	@Summary	Add / change email · Step 2 — verify OTP
//	@Tags		5. Me · Profile & account
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		dto.EmailOTPVerifyRequest	true	"Email + OTP"
//	@Success	200		{object}	response.Body{data=dto.MeResponse}
//	@Failure	400		{object}	response.ErrorResponse	"OTP_INVALID | OTP_EXPIRED"
//	@Router		/users/me/contacts/email/verify [post]
func (h *Handler) VerifyEmailContact(c *gin.Context) {
	var req dto.EmailOTPVerifyRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.VerifyContactOTP(c.Request.Context(), uid(c), chEmail, req.Email, req.OTP, meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "email verified", res)
}

// SendPhoneContactOTP godoc
//
//	@Summary		Add / change BD phone · Step 1 — send SMS OTP
//	@Description	Optional for tourists (international numbers are not SMS-verified — use `whatsapp_number`
//	@Description	in the profile instead). Guides can change their phone number here.
//	@Tags			5. Me · Profile & account
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		dto.PhoneOTPRequest	true	"New BD phone"
//	@Success		200		{object}	response.Body{data=dto.OTPSentResponse}
//	@Failure		409		{object}	response.ErrorResponse	"PHONE_ALREADY_REGISTERED | CONTACT_ALREADY_VERIFIED"
//	@Router			/users/me/contacts/phone/send-otp [post]
func (h *Handler) SendPhoneContactOTP(c *gin.Context) {
	var req dto.PhoneOTPRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.SendContactOTP(c.Request.Context(), uid(c), chSMS, req.Phone, meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "verification code sent", res)
}

// VerifyPhoneContact godoc
//
//	@Summary	Add / change BD phone · Step 2 — verify OTP
//	@Tags		5. Me · Profile & account
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		dto.PhoneOTPVerifyRequest	true	"Phone + OTP"
//	@Success	200		{object}	response.Body{data=dto.MeResponse}
//	@Failure	400		{object}	response.ErrorResponse	"OTP_INVALID | OTP_EXPIRED"
//	@Router		/users/me/contacts/phone/verify [post]
func (h *Handler) VerifyPhoneContact(c *gin.Context) {
	var req dto.PhoneOTPVerifyRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.VerifyContactOTP(c.Request.Context(), uid(c), chSMS, req.Phone, req.OTP, meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "phone verified", res)
}

// ListSessions godoc
//
//	@Summary	List my logged-in devices
//	@Tags		5. Me · Profile & account
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	response.Body{data=[]dto.SessionResponse}
//	@Router		/users/me/sessions [get]
func (h *Handler) ListSessions(c *gin.Context) {
	res, err := h.svc.ListSessions(c.Request.Context(), uid(c), sid(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "", res)
}

// RevokeSession godoc
//
//	@Summary	Sign out a specific device
//	@Tags		5. Me · Profile & account
//	@Produce	json
//	@Security	BearerAuth
//	@Param		id	path		string	true	"Session ID"	format(uuid)
//	@Success	200	{object}	response.Body
//	@Failure	404	{object}	response.ErrorResponse	"SESSION_NOT_FOUND"
//	@Router		/users/me/sessions/{id} [delete]
func (h *Handler) RevokeSession(c *gin.Context) {
	id, ok := uuidParam(c, "id")
	if !ok {
		return
	}
	if err := h.svc.RevokeSession(c.Request.Context(), uid(c), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "session revoked", nil)
}

// DeleteAccount godoc
//
//	@Summary		Delete my account
//	@Description	Soft-deletes the account and signs out everywhere. `confirm` must be "DELETE";
//	@Description	`password` is required when the account has one.
//	@Tags			5. Me · Profile & account
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		dto.DeleteAccountRequest	true	"Confirmation"
//	@Success		200		{object}	response.Body
//	@Failure		401		{object}	response.ErrorResponse	"WRONG_PASSWORD"
//	@Failure		422		{object}	response.ErrorResponse	"PASSWORD_REQUIRED | VALIDATION_ERROR"
//	@Router			/users/me [delete]
func (h *Handler) DeleteAccount(c *gin.Context) {
	var req dto.DeleteAccountRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.DeleteAccount(c.Request.Context(), uid(c), req, meta(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "account deleted", nil)
}
