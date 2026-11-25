// Package repository implements the geo repository port with PostgreSQL (GORM).
package repository

import (
	"context"
	"time"

	"cholobangladesh-server/internal/modules/geo/domain"
	"cholobangladesh-server/internal/shared/web"
	"cholobangladesh-server/pkg/listing"

	"gorm.io/gorm"
)

type postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) domain.Repository { return &postgres{db: db} }

func (r *postgres) q(ctx context.Context) *gorm.DB { return r.db.WithContext(ctx) }

func touch(fields map[string]any) map[string]any {
	fields["updated_at"] = time.Now().UTC()
	return fields
}

// ------------------------------------------------------------------ divisions

func (r *postgres) ListDivisions(ctx context.Context) ([]domain.Division, error) {
	var out []domain.Division
	err := r.q(ctx).Model(&domain.Division{}).
		Select("divisions.*, (SELECT count(*) FROM districts d WHERE d.division_id = divisions.id) AS district_count").
		Order("divisions.name").Find(&out).Error
	return out, err
}

func (r *postgres) GetDivision(ctx context.Context, id int) (*domain.Division, error) {
	var d domain.Division
	err := r.q(ctx).Model(&domain.Division{}).
		Select("divisions.*, (SELECT count(*) FROM districts d WHERE d.division_id = divisions.id) AS district_count").
		Where("divisions.id = ?", id).Take(&d).Error
	return &d, web.MapDBErr(err)
}

func (r *postgres) CreateDivision(ctx context.Context, d *domain.Division) error {
	return web.MapDBErr(r.q(ctx).Omit("DistrictCount").Create(d).Error)
}

func (r *postgres) UpdateDivision(ctx context.Context, id int, fields map[string]any) error {
	return r.update(ctx, &domain.Division{}, id, fields)
}

func (r *postgres) DeleteDivision(ctx context.Context, id int) error {
	return r.delete(ctx, &domain.Division{}, id)
}

// ------------------------------------------------------------------ districts

const districtSelect = `districts.*,
	(SELECT count(*) FROM upazilas u WHERE u.district_id = districts.id) AS upazila_count,
	(SELECT count(*) FROM tourist_spots s WHERE s.district_id = districts.id AND s.status = 'published' AND s.deleted_at IS NULL) AS spot_count`

func (r *postgres) ListDistricts(ctx context.Context, divisionID int, query string) ([]domain.District, error) {
	q := r.q(ctx).Model(&domain.District{}).Select(districtSelect).Preload("Division")
	if divisionID > 0 {
		q = q.Where("districts.division_id = ?", divisionID)
	}
	if query != "" {
		p := web.Like(query)
		q = q.Where("(districts.name ILIKE ? OR districts.bn_name ILIKE ?)", p, p)
	}
	var out []domain.District
	err := q.Order("districts.name").Find(&out).Error
	return out, err
}

func (r *postgres) getDistrict(ctx context.Context, where string, arg any) (*domain.District, error) {
	var d domain.District
	err := r.q(ctx).Model(&domain.District{}).Select(districtSelect).Preload("Division").
		Where(where, arg).Take(&d).Error
	return &d, web.MapDBErr(err)
}

func (r *postgres) GetDistrict(ctx context.Context, id int) (*domain.District, error) {
	return r.getDistrict(ctx, "districts.id = ?", id)
}

func (r *postgres) GetDistrictBySlug(ctx context.Context, slug string) (*domain.District, error) {
	return r.getDistrict(ctx, "districts.slug = ?", slug)
}

func (r *postgres) CreateDistrict(ctx context.Context, d *domain.District) error {
	return web.MapDBErr(r.q(ctx).Omit("Division", "UpazilaCount", "SpotCount").Create(d).Error)
}

func (r *postgres) UpdateDistrict(ctx context.Context, id int, fields map[string]any) error {
	return r.update(ctx, &domain.District{}, id, fields)
}

func (r *postgres) DeleteDistrict(ctx context.Context, id int) error {
	return r.delete(ctx, &domain.District{}, id)
}

// ------------------------------------------------------------------ upazilas

// UpazilaSpec: ?sort= values and the columns ?q= searches (100 per page, max 600).
var UpazilaSpec = listing.Spec{
	Sorts: map[string]string{
		"name":     "upazilas.name ASC",
		"district": "upazilas.district_id ASC, upazilas.name ASC",
		"newest":   "upazilas.id DESC",
	},
	DefaultSort:  "name",
	Tiebreak:     "upazilas.id",
	Search:       []string{"upazilas.name", "upazilas.bn_name", "upazilas.slug"},
	DefaultLimit: 100,
	MaxLimit:     600,
}

func (r *postgres) ListUpazilas(ctx context.Context, f domain.UpazilaFilter) (listing.Result[domain.Upazila], error) {
	q := listing.Filter(r.q(ctx).Model(&domain.Upazila{})).
		Eq("upazilas.district_id", f.DistrictID).
		Where("upazilas.district_id IN (SELECT id FROM districts WHERE division_id = ?)", f.DivisionID).
		Eq("upazilas.type", f.Type)
	return listing.Find[domain.Upazila](q, f.Params, UpazilaSpec, func(db *gorm.DB) *gorm.DB { return db.Preload("District") })
}

func (r *postgres) AllUpazilas(ctx context.Context) ([]domain.Upazila, error) {
	var out []domain.Upazila
	err := r.q(ctx).Order("name").Find(&out).Error
	return out, err
}

func (r *postgres) GetUpazila(ctx context.Context, id int) (*domain.Upazila, error) {
	var u domain.Upazila
	err := r.q(ctx).Preload("District.Division").Where("id = ?", id).Take(&u).Error
	return &u, web.MapDBErr(err)
}

func (r *postgres) CreateUpazila(ctx context.Context, u *domain.Upazila) error {
	return web.MapDBErr(r.q(ctx).Omit("District").Create(u).Error)
}

func (r *postgres) UpdateUpazila(ctx context.Context, id int, fields map[string]any) error {
	return r.update(ctx, &domain.Upazila{}, id, fields)
}

func (r *postgres) DeleteUpazila(ctx context.Context, id int) error {
	return r.delete(ctx, &domain.Upazila{}, id)
}

// ------------------------------------------------------------------ helpers

func (r *postgres) update(ctx context.Context, model any, id int, fields map[string]any) error {
	res := r.q(ctx).Model(model).Where("id = ?", id).Updates(touch(fields))
	if res.Error != nil {
		return web.MapDBErr(res.Error)
	}
	if res.RowsAffected == 0 {
		return web.ErrNotFound
	}
	return nil
}

func (r *postgres) delete(ctx context.Context, model any, id int) error {
	res := r.q(ctx).Where("id = ?", id).Delete(model)
	if res.Error != nil {
		return web.MapDBErr(res.Error)
	}
	if res.RowsAffected == 0 {
		return web.ErrNotFound
	}
	return nil
}
