// Package web holds small HTTP helpers shared by every feature module
// (binding, path params, paging, DB error mapping) so modules stay consistent.
package web

import (
	"errors"
	"strconv"
	"strings"

	"cholobangladesh-server/internal/http/middleware"
	"cholobangladesh-server/pkg/apperror"
	"cholobangladesh-server/pkg/listing"
	"cholobangladesh-server/pkg/response"
	"cholobangladesh-server/pkg/validation"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ErrNotFound / ErrDuplicate are returned by module repositories.
var (
	ErrNotFound  = errors.New("not found")
	ErrDuplicate = errors.New("duplicate")
	ErrInUse     = errors.New("in use") // foreign key violation (row is referenced)
)

// MapDBErr converts GORM errors into the sentinel errors above.
func MapDBErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return ErrNotFound
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return ErrDuplicate
	case errors.Is(err, gorm.ErrForeignKeyViolated):
		return ErrInUse
	}
	return err
}

// As replaces a repository sentinel with an API error, leaving other errors untouched.
func As(err error, sentinel error, as *apperror.Error) error {
	if errors.Is(err, sentinel) {
		return as
	}
	return err
}

func BindJSON(c *gin.Context, dst any) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		response.Fail(c, validation.FromBindError(err))
		return false
	}
	return true
}

func BindQuery(c *gin.Context, dst any) bool {
	if err := c.ShouldBindQuery(dst); err != nil {
		response.Fail(c, validation.FromBindError(err))
		return false
	}
	return true
}

func UUIDParam(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		response.Fail(c, apperror.ErrNotFound)
		return uuid.Nil, false
	}
	return id, true
}

func IntParam(c *gin.Context, name string) (int, bool) {
	n, err := strconv.Atoi(c.Param(name))
	if err != nil || n <= 0 {
		response.Fail(c, apperror.ErrNotFound)
		return 0, false
	}
	return n, true
}

func UserID(c *gin.Context) uuid.UUID { return middleware.UserID(c) }

func Role(c *gin.Context) string { return middleware.Role(c) }

// Like builds an escaped ILIKE pattern: %term% (see listing.Like).
func Like(s string) string { return listing.Like(s) }

// Slugify turns "Cox's Bazar Sea Beach" into "coxs-bazar-sea-beach".
func Slugify(s string) string {
	s = strings.ToLower(strings.ReplaceAll(s, "'", ""))
	var b strings.Builder
	dash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// CleanList trims items, drops empties and duplicates.
func CleanList(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		k := strings.ToLower(s)
		if s == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, s)
	}
	return out
}
