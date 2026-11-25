package listing

import (
	"reflect"

	"gorm.io/gorm"
)

// Filters is a small builder over a GORM query. Every method SKIPS an empty value
// (0, "", nil pointer, uuid.Nil, empty slice), so a filter struct can be passed straight in:
//
//	listing.Filter(db).Eq("bookings.status", f.Status).Gte("bookings.end_date", f.From)
//
// A non-nil pointer is always applied (so *bool false and *int 0 still filter).
type Filters struct{ db *gorm.DB }

// Filter starts a builder on a base query (usually db.WithContext(ctx).Model(&Row{})).
func Filter(db *gorm.DB) *Filters { return &Filters{db: db} }

// DB returns the query built so far (for Count / Pluck outside a list).
func (f *Filters) DB() *gorm.DB { return f.db }

// Eq: col = v
func (f *Filters) Eq(col string, v any) *Filters { return f.op(col, "=", v) }

// Gte: col >= v
func (f *Filters) Gte(col string, v any) *Filters { return f.op(col, ">=", v) }

// Lte: col <= v
func (f *Filters) Lte(col string, v any) *Filters { return f.op(col, "<=", v) }

// In: col IN (v...) — v is a slice.
func (f *Filters) In(col string, v any) *Filters {
	if val, ok := present(v); ok {
		f.db = f.db.Where(col+" IN ?", val)
	}
	return f
}

// Has: a Postgres text[] column contains the value (col @> ARRAY[v]).
func (f *Filters) Has(col string, v any) *Filters {
	if val, ok := present(v); ok {
		f.db = f.db.Where(col+" @> ARRAY[?]::text[]", val)
	}
	return f
}

// Where adds a raw condition only when v is present; v fills every "?" in cond.
//
//	.Where("x.district_id IN (SELECT id FROM districts WHERE division_id = ?)", f.DivisionID)
func (f *Filters) Where(cond string, v any) *Filters {
	if val, ok := present(v); ok {
		n := 0
		for _, r := range cond {
			if r == '?' {
				n++
			}
		}
		args := make([]any, n)
		for i := range args {
			args[i] = val
		}
		f.db = f.db.Where(cond, args...)
	}
	return f
}

// If adds a raw condition when ok is true.
func (f *Filters) If(ok bool, cond string, args ...any) *Filters {
	if ok {
		f.db = f.db.Where(cond, args...)
	}
	return f
}

// Always adds a raw condition.
func (f *Filters) Always(cond string, args ...any) *Filters {
	f.db = f.db.Where(cond, args...)
	return f
}

// Joins adds a JOIN that filters need (it also applies to the COUNT).
func (f *Filters) Joins(query string, args ...any) *Filters {
	f.db = f.db.Joins(query, args...)
	return f
}

func (f *Filters) op(col, op string, v any) *Filters {
	if val, ok := present(v); ok {
		f.db = f.db.Where(col+" "+op+" ?", val)
	}
	return f
}

// present reports whether v should be applied and returns it dereferenced.
func present(v any) (any, bool) {
	if v == nil {
		return nil, false
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer:
		if rv.IsNil() {
			return nil, false
		}
		return rv.Elem().Interface(), true
	case reflect.Slice, reflect.Map:
		return v, rv.Len() > 0
	}
	if rv.IsZero() {
		return nil, false
	}
	return v, true
}
