// Package apperror defines a typed application error that carries an HTTP
// status and a stable machine-readable code the frontend can switch on.
package apperror

import (
	"errors"
	"net/http"
	"time"
)

type Error struct {
	Status     int
	Code       string
	Message    string
	Details    any
	RetryAfter time.Duration
	cause      error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return e.Code + ": " + e.Message + ": " + e.cause.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *Error) Unwrap() error { return e.cause }

// Is lets errors.Is match on code, so copies made by With* still match the sentinel.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code
}

func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

func (e *Error) clone() *Error { c := *e; return &c }

func (e *Error) WithDetails(d any) *Error { c := e.clone(); c.Details = d; return c }

func (e *Error) WithMessage(m string) *Error { c := e.clone(); c.Message = m; return c }

func (e *Error) WithRetryAfter(d time.Duration) *Error { c := e.clone(); c.RetryAfter = d; return c }

func (e *Error) Wrap(err error) *Error { c := e.clone(); c.cause = err; return c }

// As extracts an *Error from any error chain.
func As(err error) (*Error, bool) {
	var e *Error
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}

// Generic errors shared across modules.
var (
	ErrValidation   = New(http.StatusUnprocessableEntity, "VALIDATION_ERROR", "some fields are invalid")
	ErrBadRequest   = New(http.StatusBadRequest, "BAD_REQUEST", "malformed request")
	ErrUnauthorized = New(http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
	ErrTokenInvalid = New(http.StatusUnauthorized, "TOKEN_INVALID", "invalid or expired token")
	ErrForbidden    = New(http.StatusForbidden, "FORBIDDEN", "you do not have permission to perform this action")
	ErrNotFound     = New(http.StatusNotFound, "NOT_FOUND", "resource not found")
	ErrRateLimited  = New(http.StatusTooManyRequests, "RATE_LIMITED", "too many requests, please slow down")
	ErrCaptcha      = New(http.StatusForbidden, "CAPTCHA_FAILED", "captcha verification failed")
	ErrPayloadLarge = New(http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body is too large")
	ErrUnavailable  = New(http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "service temporarily unavailable")
	ErrInternal     = New(http.StatusInternalServerError, "INTERNAL_ERROR", "something went wrong")
)
