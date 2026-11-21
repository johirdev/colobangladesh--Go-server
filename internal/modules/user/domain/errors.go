package domain

import (
	"net/http"

	"cholobangladesh-server/pkg/apperror"
)

// Stable error codes for the frontend. Never change a code once shipped.
var (
	ErrUserNotFound           = apperror.New(http.StatusNotFound, "USER_NOT_FOUND", "user not found")
	ErrGuideNotFound          = apperror.New(http.StatusNotFound, "GUIDE_NOT_FOUND", "guide not found")
	ErrEmailTaken             = apperror.New(http.StatusConflict, "EMAIL_ALREADY_REGISTERED", "this email is already registered, please log in")
	ErrPhoneTaken             = apperror.New(http.StatusConflict, "PHONE_ALREADY_REGISTERED", "this phone number is already registered, please log in")
	ErrInvalidCredentials     = apperror.New(http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid email/phone or password")
	ErrInvalidIdentifier      = apperror.New(http.StatusUnprocessableEntity, "INVALID_IDENTIFIER", "enter a valid email address or Bangladeshi mobile number")
	ErrAccountLocked          = apperror.New(http.StatusTooManyRequests, "ACCOUNT_TEMPORARILY_LOCKED", "too many failed attempts, try again later")
	ErrAccountSuspended       = apperror.New(http.StatusForbidden, "ACCOUNT_SUSPENDED", "your account has been suspended, contact support")
	ErrAccountDeleted         = apperror.New(http.StatusForbidden, "ACCOUNT_DELETED", "this account has been deleted")
	ErrOTPInvalid             = apperror.New(http.StatusBadRequest, "OTP_INVALID", "the verification code is incorrect")
	ErrOTPExpired             = apperror.New(http.StatusBadRequest, "OTP_EXPIRED", "the verification code has expired, request a new one")
	ErrOTPCooldown            = apperror.New(http.StatusTooManyRequests, "OTP_COOLDOWN", "please wait before requesting another code")
	ErrOTPLimit               = apperror.New(http.StatusTooManyRequests, "OTP_LIMIT_REACHED", "too many codes requested, try again later")
	ErrOTPDelivery            = apperror.New(http.StatusServiceUnavailable, "OTP_DELIVERY_FAILED", "could not send the verification code, try again shortly")
	ErrTicketInvalid          = apperror.New(http.StatusBadRequest, "VERIFICATION_TOKEN_INVALID", "verification session expired, please verify again")
	ErrRefreshInvalid         = apperror.New(http.StatusUnauthorized, "REFRESH_TOKEN_INVALID", "session expired, please log in again")
	ErrRefreshRotated         = apperror.New(http.StatusConflict, "REFRESH_TOKEN_ROTATED", "token was just refreshed by another request, retry with the newest token")
	ErrSessionRevoked         = apperror.New(http.StatusUnauthorized, "SESSION_REVOKED", "session has been signed out, please log in again")
	ErrSessionNotFound        = apperror.New(http.StatusNotFound, "SESSION_NOT_FOUND", "session not found")
	ErrWrongPassword          = apperror.New(http.StatusUnauthorized, "WRONG_PASSWORD", "current password is incorrect")
	ErrPasswordRequired       = apperror.New(http.StatusUnprocessableEntity, "PASSWORD_REQUIRED", "password is required")
	ErrSamePassword           = apperror.New(http.StatusUnprocessableEntity, "PASSWORD_UNCHANGED", "new password must be different from the current one")
	ErrOAuthDisabled          = apperror.New(http.StatusServiceUnavailable, "OAUTH_PROVIDER_DISABLED", "this sign-in method is not enabled")
	ErrOAuthInvalid           = apperror.New(http.StatusUnauthorized, "OAUTH_TOKEN_INVALID", "could not verify the sign-in token")
	ErrOAuthEmailUnverified   = apperror.New(http.StatusUnprocessableEntity, "OAUTH_EMAIL_UNVERIFIED", "your provider account email is not verified")
	ErrNotGuide               = apperror.New(http.StatusForbidden, "NOT_A_GUIDE", "only guide accounts can do this")
	ErrGuideProfileIncomplete = apperror.New(http.StatusUnprocessableEntity, "GUIDE_PROFILE_INCOMPLETE", "complete your guide profile before submitting verification")
	ErrVerificationPending    = apperror.New(http.StatusConflict, "VERIFICATION_PENDING", "your verification is already under review")
	ErrAlreadyVerified        = apperror.New(http.StatusConflict, "ALREADY_VERIFIED", "your identity is already verified")
	ErrVerificationNotFound   = apperror.New(http.StatusNotFound, "VERIFICATION_NOT_FOUND", "verification request not found")
	ErrVerificationReviewed   = apperror.New(http.StatusConflict, "VERIFICATION_ALREADY_REVIEWED", "this request has already been reviewed")
	ErrStorageUnavailable     = apperror.New(http.StatusServiceUnavailable, "UPLOAD_UNAVAILABLE", "file uploads are temporarily unavailable")
	ErrCannotModifyAdmin      = apperror.New(http.StatusForbidden, "CANNOT_MODIFY_ADMIN", "admin accounts cannot be changed here")
	ErrRoleNotChangeable      = apperror.New(http.StatusConflict, "ROLE_NOT_CHANGEABLE", "only tourist and moderator accounts can switch roles")
	ErrDeleteConfirmation     = apperror.New(http.StatusUnprocessableEntity, "DELETE_CONFIRMATION_REQUIRED", `type "DELETE" to confirm`)
	ErrUnderage               = apperror.New(http.StatusUnprocessableEntity, "UNDERAGE", "you must be at least 18 years old to be a guide")
	ErrContactAlreadyYours    = apperror.New(http.StatusConflict, "CONTACT_ALREADY_VERIFIED", "this contact is already verified on your account")
)
