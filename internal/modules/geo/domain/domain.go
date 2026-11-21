// Package domain holds the geo module's entities (division -> district -> upazila/thana),
// errors and the repository port.
package domain

import (
	"context"
	"net/http"
	"time"

	"cholobangladesh-server/pkg/apperror"
)

type Division struct {
	ID        int `gorm:"primaryKey"`
	Name      string
	BnName    string
	Slug      string
	Latitude  *float64
	Longitude *float64
	CreatedAt time.Time
	UpdatedAt time.Time

	DistrictCount int64 `gorm:"->"` // read-only, filled by list queries
}

type District struct {
	ID         int `gorm:"primaryKey"`
	DivisionID int
	Name       string
	BnName     string
	Slug       string
	Latitude   *float64
	Longitude  *float64
	Website    *string
	CreatedAt  time.Time
	UpdatedAt  time.Time

	Division     *Division `gorm:"foreignKey:DivisionID"`
	UpazilaCount int64     `gorm:"->"`
	SpotCount    int64     `gorm:"->"` // published tourist spots
}

// Upazila is an upazila or a metropolitan thana (Type = "metro_thana").
type Upazila struct {
	ID         int `gorm:"primaryKey"`
	DistrictID int
	Name       string
	BnName     string
	Slug       string
	Type       string
	Latitude   *float64
	Longitude  *float64
	CreatedAt  time.Time
	UpdatedAt  time.Time

	District *District `gorm:"foreignKey:DistrictID"`
}

const (
	TypeUpazila    = "upazila"
	TypeMetroThana = "metro_thana"
)

type UpazilaFilter struct {
	DistrictID int
	DivisionID int
	Type       string
	Query      string
	Page       int
	Limit      int
}

type Repository interface {
	ListDivisions(ctx context.Context) ([]Division, error)
	GetDivision(ctx context.Context, id int) (*Division, error)
	CreateDivision(ctx context.Context, d *Division) error
	UpdateDivision(ctx context.Context, id int, fields map[string]any) error
	DeleteDivision(ctx context.Context, id int) error

	ListDistricts(ctx context.Context, divisionID int, query string) ([]District, error)
	GetDistrict(ctx context.Context, id int) (*District, error)
	GetDistrictBySlug(ctx context.Context, slug string) (*District, error)
	CreateDistrict(ctx context.Context, d *District) error
	UpdateDistrict(ctx context.Context, id int, fields map[string]any) error
	DeleteDistrict(ctx context.Context, id int) error

	ListUpazilas(ctx context.Context, f UpazilaFilter) ([]Upazila, int64, error)
	GetUpazila(ctx context.Context, id int) (*Upazila, error)
	CreateUpazila(ctx context.Context, u *Upazila) error
	UpdateUpazila(ctx context.Context, id int, fields map[string]any) error
	DeleteUpazila(ctx context.Context, id int) error
}

var (
	ErrDivisionNotFound = apperror.New(http.StatusNotFound, "DIVISION_NOT_FOUND", "division not found")
	ErrDistrictNotFound = apperror.New(http.StatusNotFound, "DISTRICT_NOT_FOUND", "district not found")
	ErrUpazilaNotFound  = apperror.New(http.StatusNotFound, "UPAZILA_NOT_FOUND", "upazila/thana not found")
	ErrSlugTaken        = apperror.New(http.StatusConflict, "SLUG_TAKEN", "another item already uses this slug")
	ErrInUse            = apperror.New(http.StatusConflict, "GEO_IN_USE", "this item is still used by districts, upazilas or tourist spots; move or delete them first")
)
