package handler

import (
	"net/http"

	"cholobangladesh-server/internal/modules/user/dto"
	"cholobangladesh-server/internal/platform/oauth"
	"cholobangladesh-server/pkg/response"

	"github.com/gin-gonic/gin"
)

// ============================================================ tourist sign-up

// SendUserRegistrationOTP godoc
//
//	@Summary		Tourist sign-up · Step 1 — send email OTP
//	@Description	Sends a 6-digit code to the email. Fails with `EMAIL_ALREADY_REGISTERED` (409) if the email
//	@Description	already has an account (send the user to login). Re-sending is allowed after
//	@Description	`resend_after_seconds`; otherwise `OTP_COOLDOWN` (429, see `retry_after_seconds`).
//	@Description	Requires `X-Captcha-Token` header when captcha is enabled.
//	@Tags			1. Auth · Tourist sign-up
//	@Accept			json
//	@Produce		json
//	@Param			X-Captcha-Token	header		string				false	"Cloudflare Turnstile token (when CAPTCHA_ENABLED)"
//	@Param			body			body		dto.EmailOTPRequest	true	"Email"
//	@Success		200				{object}	response.Body{data=dto.OTPSentResponse}
//	@Failure		409				{object}	response.ErrorResponse	"EMAIL_ALREADY_REGISTERED"
//	@Failure		422				{object}	response.ErrorResponse	"VALIDATION_ERROR"
//	@Failure		429				{object}	response.ErrorResponse	"OTP_COOLDOWN | OTP_LIMIT_REACHED | RATE_LIMITED"
//	@Failure		503				{object}	response.ErrorResponse	"OTP_DELIVERY_FAILED"
//	@Router			/auth/register/user/send-otp [post]
func (h *Handler) SendUserRegistrationOTP(c *gin.Context) {
	var req dto.EmailOTPRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.SendUserRegistrationOTP(c.Request.Context(), req.Email, meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "verification code sent", res)
}

// VerifyUserRegistrationOTP godoc
//
//	@Summary		Tourist sign-up · Step 2 — verify email OTP
//	@Description	Returns a single-use `token` (registration_token) valid for USER_REGISTRATION_TOKEN_EXPIRE_IN.
//	@Description	Keep it in memory and send it with step 3. Wrong codes return `OTP_INVALID` with
//	@Description	`details.remaining_attempts`; after OTP_MAX_ATTEMPTS the email is blocked for USER_BLOCK_TIME
//	@Description	(`ACCOUNT_TEMPORARILY_LOCKED`, 429).
//	@Tags			1. Auth · Tourist sign-up
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.EmailOTPVerifyRequest	true	"Email + OTP"
//	@Success		200		{object}	response.Body{data=dto.VerificationTicketResponse}
//	@Failure		400		{object}	response.ErrorResponse	"OTP_INVALID | OTP_EXPIRED"
//	@Failure		429		{object}	response.ErrorResponse	"ACCOUNT_TEMPORARILY_LOCKED"
//	@Router			/auth/register/user/verify-otp [post]
func (h *Handler) VerifyUserRegistrationOTP(c *gin.Context) {
	var req dto.EmailOTPVerifyRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.VerifyUserRegistrationOTP(c.Request.Context(), req.Email, req.OTP)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "email verified", res)
}

// CompleteUserRegistration godoc
//
//	@Summary		Tourist sign-up · Step 3 — create account
//	@Description	Creates the tourist account and logs in (returns tokens). `password` is optional: without
//	@Description	it the tourist signs in with email OTP or Google/Apple. Then follow `user.onboarding.next_step`.
//	@Tags			1. Auth · Tourist sign-up
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.CompleteUserRegistrationRequest	true	"Account details"
//	@Success		201		{object}	response.Body{data=dto.AuthResponse}
//	@Failure		400		{object}	response.ErrorResponse	"VERIFICATION_TOKEN_INVALID"
//	@Failure		409		{object}	response.ErrorResponse	"EMAIL_ALREADY_REGISTERED"
//	@Failure		422		{object}	response.ErrorResponse	"VALIDATION_ERROR"
//	@Router			/auth/register/user/complete [post]
func (h *Handler) CompleteUserRegistration(c *gin.Context) {
	var req dto.CompleteUserRegistrationRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.CompleteUserRegistration(c.Request.Context(), req, meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusCreated, "account created", res)
}

// ============================================================ guide sign-up

// SendGuideRegistrationOTP godoc
//
//	@Summary		Guide sign-up · Step 1 — send SMS OTP (Bangladeshi numbers only)
//	@Description	Accepts `01XXXXXXXXX`, `8801XXXXXXXXX` or `+8801XXXXXXXXX`. Only BD mobile operators are
//	@Description	allowed (prevents international SMS fraud). Also limited to SMS_MAX_PER_IP_PER_DAY per IP.
//	@Tags			2. Auth · Guide sign-up
//	@Accept			json
//	@Produce		json
//	@Param			X-Captcha-Token	header		string				false	"Cloudflare Turnstile token (when CAPTCHA_ENABLED)"
//	@Param			body			body		dto.PhoneOTPRequest	true	"BD mobile number"
//	@Success		200				{object}	response.Body{data=dto.OTPSentResponse}
//	@Failure		409				{object}	response.ErrorResponse	"PHONE_ALREADY_REGISTERED"
//	@Failure		422				{object}	response.ErrorResponse	"VALIDATION_ERROR"
//	@Failure		429				{object}	response.ErrorResponse	"OTP_COOLDOWN | OTP_LIMIT_REACHED"
//	@Failure		503				{object}	response.ErrorResponse	"OTP_DELIVERY_FAILED"
//	@Router			/auth/register/guide/send-otp [post]
func (h *Handler) SendGuideRegistrationOTP(c *gin.Context) {
	var req dto.PhoneOTPRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.SendGuideRegistrationOTP(c.Request.Context(), req.Phone, meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "verification code sent", res)
}

// VerifyGuideRegistrationOTP godoc
//
//	@Summary	Guide sign-up · Step 2 — verify SMS OTP
//	@Tags		2. Auth · Guide sign-up
//	@Accept		json
//	@Produce	json
//	@Param		body	body		dto.PhoneOTPVerifyRequest	true	"Phone + OTP"
//	@Success	200		{object}	response.Body{data=dto.VerificationTicketResponse}
//	@Failure	400		{object}	response.ErrorResponse	"OTP_INVALID | OTP_EXPIRED"
//	@Failure	429		{object}	response.ErrorResponse	"ACCOUNT_TEMPORARILY_LOCKED"
//	@Router		/auth/register/guide/verify-otp [post]
func (h *Handler) VerifyGuideRegistrationOTP(c *gin.Context) {
	var req dto.PhoneOTPVerifyRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.VerifyGuideRegistrationOTP(c.Request.Context(), req.Phone, req.OTP)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "phone verified", res)
}

// CompleteGuideRegistration godoc
//
//	@Summary		Guide sign-up · Step 3 — create guide account
//	@Description	Creates the guide account (verification_status = `unverified`) and logs in.
//	@Description	Next steps (see `user.onboarding`): PATCH /users/me/profile + PUT /users/me/avatar →
//	@Description	PUT /guides/me/profile → POST /guides/me/verification → admin approval → profile goes live.
//	@Tags			2. Auth · Guide sign-up
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.CompleteGuideRegistrationRequest	true	"Account details"
//	@Success		201		{object}	response.Body{data=dto.AuthResponse}
//	@Failure		400		{object}	response.ErrorResponse	"VERIFICATION_TOKEN_INVALID"
//	@Failure		409		{object}	response.ErrorResponse	"PHONE_ALREADY_REGISTERED"
//	@Failure		422		{object}	response.ErrorResponse	"VALIDATION_ERROR"
//	@Router			/auth/register/guide/complete [post]
func (h *Handler) CompleteGuideRegistration(c *gin.Context) {
	var req dto.CompleteGuideRegistrationRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.CompleteGuideRegistration(c.Request.Context(), req, meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusCreated, "guide account created", res)
}

// ============================================================ login

// Login godoc
//
//	@Summary		Login with email/phone + password
//	@Description	`identifier` = email (tourists) or BD mobile (guides). Wrong password returns
//	@Description	`INVALID_CREDENTIALS` with `details.remaining_attempts`. After USER_MAX_FAILED_ATTEMPTS
//	@Description	failures within USER_FAILED_ATTEMPT_WINDOW the account is locked for USER_BLOCK_TIME
//	@Description	(`ACCOUNT_TEMPORARILY_LOCKED`, 429 + `retry_after_seconds`). Response time is identical
//	@Description	whether or not the account exists.
//	@Tags			3. Auth · Login & session
//	@Accept			json
//	@Produce		json
//	@Param			X-Captcha-Token	header		string				false	"Cloudflare Turnstile token (when CAPTCHA_ENABLED)"
//	@Param			body			body		dto.LoginRequest	true	"Credentials"
//	@Success		200				{object}	response.Body{data=dto.AuthResponse}
//	@Failure		401				{object}	response.ErrorResponse	"INVALID_CREDENTIALS"
//	@Failure		403				{object}	response.ErrorResponse	"ACCOUNT_SUSPENDED"
//	@Failure		429				{object}	response.ErrorResponse	"ACCOUNT_TEMPORARILY_LOCKED | RATE_LIMITED"
//	@Router			/auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var req dto.LoginRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.Login(c.Request.Context(), req, meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "logged in", res)
}

// SendLoginOTP godoc
//
//	@Summary		Passwordless login · Step 1 — send OTP
//	@Description	Sends a code to a verified email (tourist) or BD phone (guide). Always returns the same
//	@Description	response whether or not the account exists (prevents account enumeration).
//	@Tags			3. Auth · Login & session
//	@Accept			json
//	@Produce		json
//	@Param			X-Captcha-Token	header		string					false	"Cloudflare Turnstile token (when CAPTCHA_ENABLED)"
//	@Param			body			body		dto.IdentifierRequest	true	"Email or BD phone"
//	@Success		200				{object}	response.Body{data=dto.OTPSentResponse}
//	@Failure		429				{object}	response.ErrorResponse	"OTP_COOLDOWN | OTP_LIMIT_REACHED"
//	@Router			/auth/login/otp/send [post]
func (h *Handler) SendLoginOTP(c *gin.Context) {
	var req dto.IdentifierRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.SendLoginOTP(c.Request.Context(), req.Identifier, meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "if an account exists, a verification code has been sent", res)
}

// VerifyLoginOTP godoc
//
//	@Summary	Passwordless login · Step 2 — verify OTP and log in
//	@Tags		3. Auth · Login & session
//	@Accept		json
//	@Produce	json
//	@Param		body	body		dto.IdentifierOTPRequest	true	"Identifier + OTP"
//	@Success	200		{object}	response.Body{data=dto.AuthResponse}
//	@Failure	400		{object}	response.ErrorResponse	"OTP_INVALID | OTP_EXPIRED"
//	@Failure	403		{object}	response.ErrorResponse	"ACCOUNT_SUSPENDED"
//	@Failure	429		{object}	response.ErrorResponse	"ACCOUNT_TEMPORARILY_LOCKED"
//	@Router		/auth/login/otp/verify [post]
func (h *Handler) VerifyLoginOTP(c *gin.Context) {
	var req dto.IdentifierOTPRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.VerifyLoginOTP(c.Request.Context(), req, meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "logged in", res)
}

// GoogleLogin godoc
//
//	@Summary		Sign in with Google (tourists)
//	@Description	Send the `id_token` (credential) from Google Identity Services (web) or Google Sign-In (mobile).
//	@Description	Creates a tourist account on first use (`is_new_user: true`) or links to an existing account
//	@Description	with the same verified email. The token audience must be one of GOOGLE_CLIENT_IDS.
//	@Tags			3. Auth · Login & session
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.OAuthRequest	true	"Google ID token"
//	@Success		200		{object}	response.Body{data=dto.AuthResponse}
//	@Failure		401		{object}	response.ErrorResponse	"OAUTH_TOKEN_INVALID"
//	@Failure		422		{object}	response.ErrorResponse	"OAUTH_EMAIL_UNVERIFIED"
//	@Failure		503		{object}	response.ErrorResponse	"OAUTH_PROVIDER_DISABLED"
//	@Router			/auth/oauth/google [post]
func (h *Handler) GoogleLogin(c *gin.Context) { h.oauth(c, oauth.ProviderGoogle) }

// AppleLogin godoc
//
//	@Summary		Sign in with Apple (tourists)
//	@Description	Send the `id_token` (identityToken) from Sign in with Apple. Apple gives the user's name to the
//	@Description	client only on the first sign-in — forward it in `full_name`. Supports private relay emails.
//	@Tags			3. Auth · Login & session
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.OAuthRequest	true	"Apple identity token"
//	@Success		200		{object}	response.Body{data=dto.AuthResponse}
//	@Failure		401		{object}	response.ErrorResponse	"OAUTH_TOKEN_INVALID"
//	@Failure		503		{object}	response.ErrorResponse	"OAUTH_PROVIDER_DISABLED"
//	@Router			/auth/oauth/apple [post]
func (h *Handler) AppleLogin(c *gin.Context) { h.oauth(c, oauth.ProviderApple) }

func (h *Handler) oauth(c *gin.Context, provider string) {
	var req dto.OAuthRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.OAuthLogin(c.Request.Context(), provider, req, meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "logged in", res)
}

// Refresh godoc
//
//	@Summary		Refresh access token (rotates refresh token)
//	@Description	Call when an API returns 401 `TOKEN_INVALID`, or shortly before `expires_in_seconds`.
//	@Description	The old refresh token becomes invalid — always store the new one. Re-using an old refresh
//	@Description	token (theft) revokes the session. If two tabs refresh at once, the loser gets
//	@Description	`REFRESH_TOKEN_ROTATED` (409): re-read the newest token from storage and retry.
//	@Tags			3. Auth · Login & session
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.RefreshRequest	true	"Refresh token"
//	@Success		200		{object}	response.Body{data=dto.AuthResponse}
//	@Failure		401		{object}	response.ErrorResponse	"REFRESH_TOKEN_INVALID → redirect to login"
//	@Failure		409		{object}	response.ErrorResponse	"REFRESH_TOKEN_ROTATED"
//	@Router			/auth/token/refresh [post]
func (h *Handler) Refresh(c *gin.Context) {
	var req dto.RefreshRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.Refresh(c.Request.Context(), req.RefreshToken, meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "token refreshed", res)
}

// Logout godoc
//
//	@Summary		Logout this device
//	@Description	Revokes the current session; the access token stops working immediately.
//	@Tags			3. Auth · Login & session
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	response.Body
//	@Failure		401	{object}	response.ErrorResponse
//	@Router			/auth/logout [post]
func (h *Handler) Logout(c *gin.Context) {
	if err := h.svc.Logout(c.Request.Context(), uid(c), sid(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "logged out", nil)
}

// LogoutAll godoc
//
//	@Summary	Logout from all devices
//	@Tags		3. Auth · Login & session
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	response.Body
//	@Failure	401	{object}	response.ErrorResponse
//	@Router		/auth/logout-all [post]
func (h *Handler) LogoutAll(c *gin.Context) {
	if err := h.svc.LogoutAll(c.Request.Context(), uid(c), meta(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "logged out from all devices", nil)
}

// ============================================================ password reset

// ForgotPassword godoc
//
//	@Summary		Forgot password · Step 1 — send reset OTP
//	@Description	Sends a code to the verified email/phone. Same response whether or not the account exists.
//	@Tags			4. Auth · Password reset
//	@Accept			json
//	@Produce		json
//	@Param			X-Captcha-Token	header		string					false	"Cloudflare Turnstile token (when CAPTCHA_ENABLED)"
//	@Param			body			body		dto.IdentifierRequest	true	"Email or BD phone"
//	@Success		200				{object}	response.Body{data=dto.OTPSentResponse}
//	@Failure		429				{object}	response.ErrorResponse	"OTP_COOLDOWN | OTP_LIMIT_REACHED"
//	@Router			/auth/password/forgot [post]
func (h *Handler) ForgotPassword(c *gin.Context) {
	var req dto.IdentifierRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.ForgotPassword(c.Request.Context(), req.Identifier, meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "if an account exists, a verification code has been sent", res)
}

// VerifyResetOTP godoc
//
//	@Summary		Forgot password · Step 2 — verify OTP, get reset_token
//	@Description	`token` is valid for USER_PASSWORD_RESET_TOKEN_EXPIRE_IN and can be used once.
//	@Tags			4. Auth · Password reset
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.IdentifierOTPRequest	true	"Identifier + OTP"
//	@Success		200		{object}	response.Body{data=dto.VerificationTicketResponse}
//	@Failure		400		{object}	response.ErrorResponse	"OTP_INVALID | OTP_EXPIRED"
//	@Failure		429		{object}	response.ErrorResponse	"ACCOUNT_TEMPORARILY_LOCKED"
//	@Router			/auth/password/verify-otp [post]
func (h *Handler) VerifyResetOTP(c *gin.Context) {
	var req dto.IdentifierOTPRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.VerifyResetOTP(c.Request.Context(), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "code verified", res)
}

// ResetPassword godoc
//
//	@Summary		Forgot password · Step 3 — set new password
//	@Description	Sets the new password, signs out every device and clears any login lock. Then log in again.
//	@Tags			4. Auth · Password reset
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.ResetPasswordRequest	true	"Reset token + new password"
//	@Success		200		{object}	response.Body
//	@Failure		400		{object}	response.ErrorResponse	"VERIFICATION_TOKEN_INVALID"
//	@Failure		422		{object}	response.ErrorResponse	"VALIDATION_ERROR"
//	@Router			/auth/password/reset [post]
func (h *Handler) ResetPassword(c *gin.Context) {
	var req dto.ResetPasswordRequest
	if !bindJSON(c, &req) {
		return
	}
	if err := h.svc.ResetPassword(c.Request.Context(), req, meta(c)); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "password has been reset, please log in", nil)
}

// ============================================================ Bangladeshi tourist sign-up (phone)

// SendLocalUserRegistrationOTP godoc
//
//	@Summary		Bangladeshi tourist sign-up · Step 1 — send SMS OTP
//	@Description	The sign-up screen first asks "Foreigner or Bangladeshi?". Foreigners use the email flow
//	@Description	(/auth/register/user/*); Bangladeshi tourists sign up with their BD mobile number here.
//	@Tags			1. Auth · Tourist sign-up
//	@Accept			json
//	@Produce		json
//	@Param			X-Captcha-Token	header		string				false	"Cloudflare Turnstile token (when CAPTCHA_ENABLED)"
//	@Param			body			body		dto.PhoneOTPRequest	true	"BD mobile number"
//	@Success		200				{object}	response.Body{data=dto.OTPSentResponse}
//	@Failure		409				{object}	response.ErrorResponse	"PHONE_ALREADY_REGISTERED"
//	@Failure		429				{object}	response.ErrorResponse	"OTP_COOLDOWN | OTP_LIMIT_REACHED"
//	@Router			/auth/register/user/phone/send-otp [post]
func (h *Handler) SendLocalUserRegistrationOTP(c *gin.Context) {
	var req dto.PhoneOTPRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.SendLocalUserRegistrationOTP(c.Request.Context(), req.Phone, meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "verification code sent", res)
}

// VerifyLocalUserRegistrationOTP godoc
//
//	@Summary	Bangladeshi tourist sign-up · Step 2 — verify SMS OTP
//	@Tags		1. Auth · Tourist sign-up
//	@Accept		json
//	@Produce	json
//	@Param		body	body		dto.PhoneOTPVerifyRequest	true	"Phone + OTP"
//	@Success	200		{object}	response.Body{data=dto.VerificationTicketResponse}
//	@Failure	400		{object}	response.ErrorResponse	"OTP_INVALID | OTP_EXPIRED"
//	@Router		/auth/register/user/phone/verify-otp [post]
func (h *Handler) VerifyLocalUserRegistrationOTP(c *gin.Context) {
	var req dto.PhoneOTPVerifyRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.VerifyLocalUserRegistrationOTP(c.Request.Context(), req.Phone, req.OTP)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusOK, "phone verified", res)
}

// CompleteLocalUserRegistration godoc
//
//	@Summary		Bangladeshi tourist sign-up · Step 3 — create account
//	@Description	Creates a tourist account with country = BD and logs in (returns tokens).
//	@Tags			1. Auth · Tourist sign-up
//	@Accept			json
//	@Produce		json
//	@Param			body	body		dto.CompleteLocalUserRegistrationRequest	true	"Account details"
//	@Success		201		{object}	response.Body{data=dto.AuthResponse}
//	@Failure		400		{object}	response.ErrorResponse	"VERIFICATION_TOKEN_INVALID"
//	@Failure		409		{object}	response.ErrorResponse	"PHONE_ALREADY_REGISTERED"
//	@Router			/auth/register/user/phone/complete [post]
func (h *Handler) CompleteLocalUserRegistration(c *gin.Context) {
	var req dto.CompleteLocalUserRegistrationRequest
	if !bindJSON(c, &req) {
		return
	}
	res, err := h.svc.CompleteLocalUserRegistration(c.Request.Context(), req, meta(c))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, http.StatusCreated, "account created", res)
}
