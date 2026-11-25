// Package jsonb is a raw JSON column type for PostgreSQL JSONB with GORM.
package jsonb

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

// JSON holds raw JSON bytes; nil/empty is stored as SQL NULL.
type JSON json.RawMessage

// From marshals v (nil stays NULL).
func From(v any) JSON {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return JSON(b)
}

func (j JSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	return string(j), nil
}

func (j *JSON) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*j = nil
	case []byte:
		*j = append((*j)[:0], v...)
	case string:
		*j = JSON(v)
	default:
		return errors.New("jsonb: unsupported type")
	}
	return nil
}

func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}

func (j *JSON) UnmarshalJSON(b []byte) error {
	*j = append((*j)[:0], b...)
	return nil
}

// Decode unmarshals into dst (no-op when empty).
func (j JSON) Decode(dst any) error {
	if len(j) == 0 {
		return nil
	}
	return json.Unmarshal(j, dst)
}
