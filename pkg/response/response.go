// Package response writes the single JSON envelope used by every endpoint.
package response

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strconv"

	"cholobangladesh-server/pkg/apperror"

	"github.com/gin-gonic/gin"
)

// Body is the envelope for every response.
//
//	success: true  -> read `data`
//	success: false -> read `error.code` (stable, switch on it) and `message` (human readable)
type Body struct {
	Success   bool       `json:"success" example:"true"`
	Message   string     `json:"message,omitempty" example:"ok"`
	Data      any        `json:"data,omitempty"`
	Error     *ErrorBody `json:"error,omitempty"`
	RequestID string     `json:"request_id,omitempty" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
}

type ErrorBody struct {
	Code       string `json:"code" example:"VALIDATION_ERROR"`
	Details    any    `json:"details,omitempty"`
	RetryAfter int    `json:"retry_after_seconds,omitempty" example:"60"`
}

// ErrorResponse is used only for swagger documentation.
type ErrorResponse struct {
	Success   bool      `json:"success" example:"false"`
	Message   string    `json:"message" example:"some fields are invalid"`
	Error     ErrorBody `json:"error"`
	RequestID string    `json:"request_id" example:"0193a0c4-9f7e-7c2a-a1d3-5f2b9c8e4d10"`
}

type PageMeta struct {
	Page       int   `json:"page" example:"1"`
	Limit      int   `json:"limit" example:"20"`
	Total      int64 `json:"total" example:"57"`
	TotalPages int   `json:"total_pages" example:"3"`
}

type Page[T any] struct {
	Items []T      `json:"items"`
	Meta  PageMeta `json:"meta"`
}

func NewPage[T any](items []T, page, limit int, total int64) Page[T] {
	if items == nil {
		items = []T{}
	}
	pages := 0
	if limit > 0 {
		pages = int(math.Ceil(float64(total) / float64(limit)))
	}
	return Page[T]{Items: items, Meta: PageMeta{Page: page, Limit: limit, Total: total, TotalPages: pages}}
}

const RequestIDKey = "request_id"

func OK(c *gin.Context, status int, message string, data any) {
	c.JSON(status, Body{Success: true, Message: message, Data: data, RequestID: c.GetString(RequestIDKey)})
}

// Fail converts any error into the envelope. Unknown errors become 500 and are logged;
// their internals are never sent to the client.
func Fail(c *gin.Context, err error) {
	ae, ok := apperror.As(err)
	if !ok {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			ae = apperror.ErrPayloadLarge
		} else if errors.Is(err, context.Canceled) {
			c.Abort()
			return
		} else {
			ae = apperror.ErrInternal
		}
	}
	if ae.Status >= 500 {
		slog.ErrorContext(c.Request.Context(), "request failed",
			"err", err, "path", c.FullPath(), "request_id", c.GetString(RequestIDKey))
	}

	body := Body{
		Success:   false,
		Message:   ae.Message,
		Error:     &ErrorBody{Code: ae.Code, Details: ae.Details},
		RequestID: c.GetString(RequestIDKey),
	}
	if ae.RetryAfter > 0 {
		secs := int(math.Ceil(ae.RetryAfter.Seconds()))
		body.Error.RetryAfter = secs
		c.Header("Retry-After", strconv.Itoa(secs))
	}
	c.AbortWithStatusJSON(ae.Status, body)
}
