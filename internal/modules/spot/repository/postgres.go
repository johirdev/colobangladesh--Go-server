// Package repository implements the tourist spot repository with PostgreSQL (GORM).
package repository

import (
	"context"
	"time"

	"cholobangladesh-server/internal/modules/spot/domain"
	"cholobangladesh-server/internal/shared/web"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) domain.Repository { return &postgres{db: db} }

func (r *postgres) q(ctx context.Context) *gorm.DB { return r.db.WithContext(ctx) }

var sorts = map[string]string{
	"popular": "tourist_spots.is_featured DESC, tourist_spots.gig_count DESC, tourist_spots.rating_count DESC",
	"rating":  "tourist_spots.rating_avg DESC, tourist_spots.rating_count DESC",
	"newest":  "tourist_spots.created_at DESC",
	"title":   "tourist_spots.title ASC",
}

func (r *postgres) List(ctx context.Context, f domain.Filter) ([]domain.Spot, int64, error) {
	q := r.q(ctx).Model(&domain.Spot{})
	if f.Status != "" {
		q = q.Where("tourist_spots.status = ?", f.Status)
	}
	if f.DivisionID > 0 {
		q = q.Where("tourist_spots.division_id = ?", f.DivisionID)
	}
	if f.DistrictID > 0 {
		// primary district OR any linked upazila in that district (spots spanning districts)
		q = q.Where(`(tourist_spots.district_id = ? OR EXISTS (SELECT 1 FROM tourist_spot_upazilas su
			JOIN upazilas u ON u.id = su.upazila_id WHERE su.spot_id = tourist_spots.id AND u.district_id = ?))`,
			f.DistrictID, f.DistrictID)
	}
	if f.UpazilaID > 0 {
		q = q.Where("EXISTS (SELECT 1 FROM tourist_spot_upazilas su WHERE su.spot_id = tourist_spots.id AND su.upazila_id = ?)", f.UpazilaID)
	}
	if f.Category != "" {
		q = q.Where("tourist_spots.category = ?", f.Category)
	}
	if f.Featured != nil {
		q = q.Where("tourist_spots.is_featured = ?", *f.Featured)
	}
	if f.Query != "" {
		p := web.Like(f.Query)
		q = q.Where("(tourist_spots.title ILIKE ? OR tourist_spots.bn_title ILIKE ? OR tourist_spots.location_name ILIKE ?)", p, p, p)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	order, ok := sorts[f.Sort]
	if !ok {
		order = sorts["popular"]
	}
	var out []domain.Spot
	err := q.Preload("Division").Preload("District").
		Order(order).Order("tourist_spots.id").
		Offset((f.Page - 1) * f.Limit).Limit(f.Limit).Find(&out).Error
	return out, total, err
}

func (r *postgres) Markers(ctx context.Context, divisionID, districtID int) ([]domain.Marker, error) {
	q := r.q(ctx).Table("tourist_spots s").
		Select(`s.id, s.slug, s.title, s.bn_title, s.category, s.cover_image_url, s.rating_avg, s.rating_count,
			s.gig_count, s.latitude, s.longitude, s.district_id, d.slug AS district_slug, d.name AS district_name, s.division_id`).
		Joins("JOIN districts d ON d.id = s.district_id").
		Where("s.status = ? AND s.deleted_at IS NULL", domain.StatusPublished)
	if divisionID > 0 {
		q = q.Where("s.division_id = ?", divisionID)
	}
	if districtID > 0 {
		q = q.Where("s.district_id = ?", districtID)
	}
	var out []domain.Marker
	err := q.Order("s.is_featured DESC, s.rating_avg DESC").Scan(&out).Error
	return out, err
}

func (r *postgres) detail(ctx context.Context) *gorm.DB {
	return r.q(ctx).
		Preload("Division").Preload("District").
		Preload("Upazilas", func(db *gorm.DB) *gorm.DB { return db.Order("position") }).
		Preload("Upazilas.Upazila.District").
		Preload("Images", func(db *gorm.DB) *gorm.DB { return db.Order("position, created_at") }).
		Preload("Videos", func(db *gorm.DB) *gorm.DB { return db.Order("position, created_at") })
}

func (r *postgres) get(ctx context.Context, publishedOnly bool, where string, arg any) (*domain.Spot, error) {
	q := r.detail(ctx).Where(where, arg)
	if publishedOnly {
		q = q.Where("status = ?", domain.StatusPublished)
	}
	var s domain.Spot
	if err := q.Take(&s).Error; err != nil {
		return nil, web.MapDBErr(err)
	}
	return &s, nil
}

func (r *postgres) Get(ctx context.Context, id uuid.UUID, publishedOnly bool) (*domain.Spot, error) {
	return r.get(ctx, publishedOnly, "id = ?", id)
}

func (r *postgres) GetBySlug(ctx context.Context, slug string, publishedOnly bool) (*domain.Spot, error) {
	return r.get(ctx, publishedOnly, "slug = ?", slug)
}

func (r *postgres) SlugExists(ctx context.Context, slug string, except uuid.UUID) (bool, error) {
	var n int64
	err := r.q(ctx).Model(&domain.Spot{}).Where("slug = ? AND id <> ?", slug, except).Limit(1).Count(&n).Error
	return n > 0, err
}

func (r *postgres) Create(ctx context.Context, s *domain.Spot, c domain.Children) error {
	return r.q(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Division", "District", "Upazilas", "Images", "Videos").Create(s).Error; err != nil {
			return web.MapDBErr(err)
		}
		return replaceChildren(tx, s.ID, c)
	})
}

func (r *postgres) Update(ctx context.Context, id uuid.UUID, fields map[string]any, c domain.Children) error {
	return r.q(ctx).Transaction(func(tx *gorm.DB) error {
		fields["updated_at"] = time.Now().UTC()
		res := tx.Model(&domain.Spot{}).Where("id = ?", id).Updates(fields)
		if res.Error != nil {
			return web.MapDBErr(res.Error)
		}
		if res.RowsAffected == 0 {
			return web.ErrNotFound
		}
		return replaceChildren(tx, id, c)
	})
}

func replaceChildren(tx *gorm.DB, id uuid.UUID, c domain.Children) error {
	if c.UpazilaIDs != nil {
		if err := tx.Where("spot_id = ?", id).Delete(&domain.SpotUpazila{}).Error; err != nil {
			return err
		}
		for i, uid := range *c.UpazilaIDs {
			if err := tx.Omit("Upazila").Create(&domain.SpotUpazila{SpotID: id, UpazilaID: uid, Position: i}).Error; err != nil {
				return web.MapDBErr(err)
			}
		}
	}
	if c.Images != nil {
		if err := tx.Where("spot_id = ?", id).Delete(&domain.Image{}).Error; err != nil {
			return err
		}
		if imgs := *c.Images; len(imgs) > 0 {
			for i := range imgs {
				imgs[i].SpotID, imgs[i].Position = id, i
			}
			if err := tx.Create(&imgs).Error; err != nil {
				return err
			}
		}
		if err := refreshCover(tx, id); err != nil {
			return err
		}
	}
	if c.Videos != nil {
		if err := tx.Where("spot_id = ?", id).Delete(&domain.Video{}).Error; err != nil {
			return err
		}
		if vids := *c.Videos; len(vids) > 0 {
			for i := range vids {
				vids[i].SpotID, vids[i].Position = id, i
			}
			if err := tx.Create(&vids).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// refreshCover sets cover_image_url to the first image (or NULL).
func refreshCover(tx *gorm.DB, id uuid.UUID) error {
	return tx.Exec(`UPDATE tourist_spots SET cover_image_url =
		(SELECT url FROM tourist_spot_images WHERE spot_id = ? ORDER BY position, created_at LIMIT 1) WHERE id = ?`, id, id).Error
}

func (r *postgres) RefreshCover(ctx context.Context, id uuid.UUID) error {
	return refreshCover(r.q(ctx), id)
}

func (r *postgres) SoftDelete(ctx context.Context, id uuid.UUID) error {
	res := r.q(ctx).Where("id = ?", id).Delete(&domain.Spot{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return web.ErrNotFound
	}
	return nil
}

func (r *postgres) Images(ctx context.Context, spotID uuid.UUID) ([]domain.Image, error) {
	var out []domain.Image
	err := r.q(ctx).Where("spot_id = ?", spotID).Order("position, created_at").Find(&out).Error
	return out, err
}

func (r *postgres) AddImages(ctx context.Context, spotID uuid.UUID, imgs []domain.Image) error {
	return r.q(ctx).Transaction(func(tx *gorm.DB) error {
		var maxPos *int
		if err := tx.Model(&domain.Image{}).Where("spot_id = ?", spotID).Select("MAX(position)").Scan(&maxPos).Error; err != nil {
			return err
		}
		next := 0
		if maxPos != nil {
			next = *maxPos + 1
		}
		for i := range imgs {
			imgs[i].SpotID, imgs[i].Position = spotID, next+i
		}
		if err := tx.Create(&imgs).Error; err != nil {
			return err
		}
		return refreshCover(tx, spotID)
	})
}

func (r *postgres) DeleteImage(ctx context.Context, spotID, imageID uuid.UUID) (*domain.Image, error) {
	var img domain.Image
	err := r.q(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND spot_id = ?", imageID, spotID).Take(&img).Error; err != nil {
			return web.MapDBErr(err)
		}
		if err := tx.Delete(&img).Error; err != nil {
			return err
		}
		return refreshCover(tx, spotID)
	})
	if err != nil {
		return nil, err
	}
	return &img, nil
}

func (r *postgres) ResolveUpazilas(ctx context.Context, ids []int) ([]domain.UpazilaRef, error) {
	var rows []domain.UpazilaRef
	if err := r.q(ctx).Preload("District").Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	byID := make(map[int]domain.UpazilaRef, len(rows))
	for _, u := range rows {
		byID[u.ID] = u
	}
	out := make([]domain.UpazilaRef, 0, len(ids))
	for _, id := range ids {
		u, ok := byID[id]
		if !ok {
			return nil, web.ErrNotFound
		}
		out = append(out, u)
	}
	return out, nil
}

func (r *postgres) NearestUpazilas(ctx context.Context, lat, lng float64, districtID, limit int) ([]domain.UpazilaRef, error) {
	q := r.q(ctx).Preload("District").Where("latitude IS NOT NULL AND longitude IS NOT NULL")
	if districtID > 0 {
		q = q.Where("district_id = ?", districtID)
	}
	var out []domain.UpazilaRef
	// Equirectangular approximation is plenty for ranking within Bangladesh.
	err := q.Clauses(clause.OrderBy{Expression: clause.Expr{
		SQL:                "power(latitude - ?, 2) + power((longitude - ?) * cos(radians(?)), 2)",
		Vars:               []any{lat, lng, lat},
		WithoutParentheses: true,
	}}).
		Limit(limit).Find(&out).Error
	return out, err
}
