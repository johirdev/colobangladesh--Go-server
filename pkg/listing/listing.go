// Package listing is the ONE place that knows how to page, sort, search and filter a list.
//
// Every list endpoint works the same way:
//
//	GET /things?page=2&limit=20&sort=newest&q=sajek&<module filters>
//
// A module only describes WHAT is allowed (a Spec) and WHICH filters apply; listing does the rest:
//
//	// repository
//	var spotSpec = listing.Spec{
//		Sorts:       map[string]string{"newest": "tourist_spots.created_at DESC", "title": "tourist_spots.title"},
//		DefaultSort: "newest",
//		Tiebreak:    "tourist_spots.id",
//		Search:      []string{"tourist_spots.title", "tourist_spots.location_name"},
//	}
//
//	func (r *postgres) List(ctx context.Context, f domain.Filter) (listing.Result[domain.Spot], error) {
//		q := listing.Filter(r.db.WithContext(ctx).Model(&domain.Spot{})).
//			Eq("tourist_spots.status", f.Status).
//			Eq("tourist_spots.district_id", f.DistrictID)
//		return listing.Find[domain.Spot](q, f.Params, spotSpec, func(db *gorm.DB) *gorm.DB {
//			return db.Preload("District")
//		})
//	}
//
//	// service
//	res, err := s.repo.List(ctx, f)
//	return listing.Map(res, toCard), err // → response.Page[dto.SpotCard]
//
// Query cost: one SELECT for the page. The COUNT query is skipped whenever the page itself
// proves the total (first page not full, or the last page), so small lists cost one query.
package listing

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"cholobangladesh-server/pkg/response"
	"cholobangladesh-server/pkg/validation"

	"gorm.io/gorm"
)

const (
	DefaultLimit = 20
	MaxLimit     = 100
)

// Params are the common list query parameters. Embed it in every list query DTO and domain filter.
type Params struct {
	Page  int    `form:"page" json:"-" binding:"omitempty,gte=1" example:"1"`
	Limit int    `form:"limit" json:"-" binding:"omitempty,gte=1,lte=1000" example:"20"` // capped by the list Spec (usually 100)
	Sort  string `form:"sort" json:"-" binding:"omitempty,max=40" example:"newest"`
	Q     string `form:"q" json:"-" binding:"omitempty,max=100" example:""` // free-text search
}

// Spec describes what a list allows. Declare one per list, next to the query that uses it.
type Spec struct {
	// Sorts maps the ?sort= value to an ORDER BY clause.
	Sorts       map[string]string
	DefaultSort string
	// Tiebreak is a unique column appended to every ORDER BY so pages never overlap.
	Tiebreak string
	// Search: columns matched (case-insensitive, contains) by ?q=, OR-ed together. An entry with a
	// "?" is used as raw SQL, e.g. "EXISTS (SELECT 1 FROM bookings b WHERE b.id = x.booking_id AND b.code ILIKE ?)".
	Search []string
	// DefaultLimit / MaxLimit override the package defaults (20 / 100).
	DefaultLimit int
	MaxLimit     int
}

// Result is one page read from the database (domain rows). Turn it into the API shape with Map.
type Result[T any] struct {
	Items []T
	Total int64
	Page  int
	Limit int
	Sort  string
}

// Normalize fills defaults, caps the limit and checks the sort against the spec.
func (s Spec) Normalize(p Params) (Params, error) {
	def, max := s.DefaultLimit, s.MaxLimit
	if def <= 0 {
		def = DefaultLimit
	}
	if max <= 0 {
		max = MaxLimit
	}
	if p.Page < 1 {
		p.Page = 1
	}
	if p.Limit < 1 {
		p.Limit = def
	}
	if p.Limit > max {
		p.Limit = max
	}
	p.Q = strings.TrimSpace(p.Q)
	p.Sort = strings.TrimSpace(p.Sort)
	if p.Sort == "" {
		p.Sort = s.DefaultSort
	}
	if len(s.Sorts) > 0 {
		if _, ok := s.Sorts[p.Sort]; !ok {
			return p, validation.Field("sort", "oneof", "must be one of: "+strings.Join(s.SortKeys(), ", "))
		}
	}
	return p, nil
}

// SortKeys lists the allowed ?sort= values (alphabetical).
func (s Spec) SortKeys() []string { return slices.Sorted(maps.Keys(s.Sorts)) }

// Offset of the first row of the page.
func (p Params) Offset() int { return (p.Page - 1) * p.Limit }

// Find runs the list: search (?q=), sort, page — and the count only when it is needed.
// q carries the module filters (build it with Filter); prepare adds Select/Preload/Joins that
// must not take part in the COUNT (may be nil).
func Find[T any](q *Filters, p Params, spec Spec, prepare func(*gorm.DB) *gorm.DB) (Result[T], error) {
	p, err := spec.Normalize(p)
	if err != nil {
		return Result[T]{}, err
	}
	base := search(q.db, p.Q, spec.Search).Session(&gorm.Session{})

	page := base
	if prepare != nil {
		page = prepare(page)
	}
	if order := spec.Sorts[p.Sort]; order != "" {
		page = page.Order(order)
	}
	if spec.Tiebreak != "" {
		page = page.Order(spec.Tiebreak)
	}
	var items []T
	if err := page.Offset(p.Offset()).Limit(p.Limit).Find(&items).Error; err != nil {
		return Result[T]{}, err
	}

	total, known := int64(0), false
	switch n := len(items); {
	case n > 0 && n < p.Limit: // partial page = last page
		total, known = int64(p.Offset()+n), true
	case n == 0 && p.Page == 1: // nothing at all
		known = true
	}
	if !known {
		if err := base.Count(&total).Error; err != nil {
			return Result[T]{}, err
		}
	}
	if items == nil {
		items = []T{}
	}
	return Result[T]{Items: items, Total: total, Page: p.Page, Limit: p.Limit, Sort: p.Sort}, nil
}

// Map converts a Result of domain rows into the API page.
func Map[T, U any](r Result[T], fn func(*T) U) response.Page[U] {
	out := make([]U, 0, len(r.Items))
	for i := range r.Items {
		out = append(out, fn(&r.Items[i]))
	}
	pg := response.NewPage(out, r.Page, r.Limit, r.Total)
	pg.Meta.Sort = r.Sort
	return pg
}

// Empty is a zero-row page (e.g. when a filter can never match).
func Empty[T any](p Params, spec Spec) Result[T] {
	p, _ = spec.Normalize(p)
	return Result[T]{Items: []T{}, Page: p.Page, Limit: p.Limit, Sort: p.Sort}
}

// Like builds an escaped ILIKE pattern: %term%.
func Like(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + r.Replace(strings.TrimSpace(s)) + "%"
}

func search(db *gorm.DB, term string, cols []string) *gorm.DB {
	if term == "" || len(cols) == 0 {
		return db
	}
	like := Like(term)
	parts := make([]string, 0, len(cols))
	args := make([]any, 0, len(cols))
	for _, c := range cols {
		if strings.Contains(c, "?") {
			parts = append(parts, c)
			for range strings.Count(c, "?") {
				args = append(args, like)
			}
			continue
		}
		parts = append(parts, fmt.Sprintf("%s ILIKE ?", c))
		args = append(args, like)
	}
	return db.Where("("+strings.Join(parts, " OR ")+")", args...)
}
