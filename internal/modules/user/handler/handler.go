// Package handler is the HTTP delivery layer of the user module: it binds and
// validates input, calls the service and writes the standard JSON envelope.
// Swagger annotations live here.
package handler

import (
	"cholobangladesh-server/internal/http/middleware"
	"cholobangladesh-server/internal/modules/user/domain"
	"cholobangladesh-server/internal/modules/user/service"
	"cholobangladesh-server/pkg/apperror"
	"cholobangladesh-server/pkg/response"
	"cholobangladesh-server/pkg/validation"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	svc *service.Service
}

func New(svc *service.Service) *Handler { return &Handler{svc: svc} }

func meta(c *gin.Context) domain.RequestMeta {
	return domain.RequestMeta{IP: c.ClientIP(), UserAgent: c.Request.UserAgent()}
}

func bindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		response.Fail(c, validation.FromBindError(err))
		return false
	}
	return true
}

func bindQuery(c *gin.Context, dst any) bool {
	if err := c.ShouldBindQuery(dst); err != nil {
		response.Fail(c, validation.FromBindError(err))
		return false
	}
	return true
}

func uuidParam(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		response.Fail(c, apperror.ErrNotFound)
		return uuid.Nil, false
	}
	return id, true
}

func uid(c *gin.Context) uuid.UUID { return middleware.UserID(c) }

func sid(c *gin.Context) uuid.UUID { return middleware.SessionID(c) }
