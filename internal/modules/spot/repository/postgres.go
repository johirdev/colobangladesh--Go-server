// Package repository implements the tourist spot repository with PostgreSQL (GORM).
package repository

import (
	"context"
	"time"

	"cholobangladesh-server/internal/modules/spot/domain"
	"cholobangladesh-server/internal/shared/web"
	"cholobangladesh-server/pkg/listing"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) domain.Repository { return &postgres{db: db} }

func (r *postgres) q(ctx context.Context) *gorm.DB { return r.db.WithContext(ctx) }

// ListSpec: ?sort= values and the columns ?q= searches.
var ListSpec = listing.Spec{
	Sorts: map[string]string{
		"popular": "tourist_spots.is_featured DESC, tourist_spots.gig_count DESC, tourist_spots.rating_count DESC",
		"rating":  "tourist_spots.rating_avg DESC, tourist_spots.rating_count DESC",
		"newest":  "tourist_spots.created_at DESC",
		"title":   "tourist_spots.title ASC",
		"updated": "tourist_spots.updated_at DESC",
	},
	DefaultSort: "popular",
	Tiebreak:    "tourist_spots.id",
	Search:      []string{"tourist_spots.title", "tourist_spots.bn_title", "tourist_spots.location_name"},
}

// missing narrows the staff listing to spots that still need work.
var missing = map[string]string{
	"images":      "NOT EXISTS (SELECT 1 FROM tourist_spot_images i WHERE i.spot_id = tourist_spots.id)",
	"videos":      "NOT EXISTS (SELECT 1 FROM tourist_spot_videos v WHERE v.spot_id = tourist_spots.id)",
	"coordinates": "(tourist_spots.latitude IS NULL OR tourist_spots.longitude IS NULL)",
}

func (r *postgres) List(ctx context.Context, f domain.Filter) (listing.Result[domain.Spot], error) {
	q := listing.Filter(r.q(ctx).Model(&domain.Spot{})).
		Eq("tourist_spots.status", f.Status).
		Eq("tourist_spots.division_id", f.DivisionID).
		// primary district OR any linked upazila in that district (spots spanning districts)
		Where(`(tourist_spots.district_id = ? OR EXISTS (SELECT 1 FROM tourist_spot_upazilas su
			JOIN upazilas u ON u.id = su.upazila_id WHERE su.spot_id = tourist_spots.id AND u.district_id = ?))`, f.DistrictID).
		Where("EXISTS (SELECT 1 FROM tourist_spot_upazilas su WHERE su.spot_id = tourist_spots.id AND su.upazila_id = ?)", f.UpazilaID).
		Eq("tourist_spots.category", f.Category).
		Eq("tourist_spots.is_featured", f.Featured).
		If(missing[f.Missing] != "", missing[f.Missing])

	return listing.Find[domain.Spot](q, f.Params, ListSpec, func(db *gorm.DB) *gorm.DB {
		if f.WithCounts {
			db = db.Select(`tourist_spots.*,
				(SELECT COUNT(*) FROM tourist_spot_images i WHERE i.spot_id = tourist_spots.id) AS image_count,
				(SELECT COUNT(*) FROM tourist_spot_videos v WHERE v.spot_id = tourist_spots.id) AS video_count`)
		}
		return db.Preload("Division").Preload("District")
	})
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

// ================================================================== staff dashboard + bulk

func (r *postgres) Stats(ctx context.Context) (*domain.Stats, error) {
	var c struct {
		Total, Published, Draft, Archived, Featured, MissingImages, MissingCoordinates, MissingVideos, WithGigs, DistrictsCovered, TotalDistricts int64
	}
	err := r.q(ctx).Raw(`SELECT
		COUNT(*) AS total,
		COUNT(*) FILTER (WHERE s.status = ?) AS published,
		COUNT(*) FILTER (WHERE s.status = ?) AS draft,
		COUNT(*) FILTER (WHERE s.status = ?) AS archived,
		COUNT(*) FILTER (WHERE s.is_featured) AS featured,
		COUNT(*) FILTER (WHERE NOT EXISTS (SELECT 1 FROM tourist_spot_images i WHERE i.spot_id = s.id)) AS missing_images,
		COUNT(*) FILTER (WHERE s.latitude IS NULL OR s.longitude IS NULL) AS missing_coordinates,
		COUNT(*) FILTER (WHERE NOT EXISTS (SELECT 1 FROM tourist_spot_videos v WHERE v.spot_id = s.id)) AS missing_videos,
		COUNT(*) FILTER (WHERE s.gig_count > 0) AS with_gigs,
		COUNT(DISTINCT s.district_id) FILTER (WHERE s.status = ?) AS districts_covered,
		(SELECT COUNT(*) FROM districts) AS total_districts
		FROM tourist_spots s WHERE s.deleted_at IS NULL`,
		domain.StatusPublished, domain.StatusDraft, domain.StatusArchived, domain.StatusPublished).Scan(&c).Error
	if err != nil {
		return nil, err
	}
	st := domain.Stats{Total: c.Total, Published: c.Published, Draft: c.Draft, Archived: c.Archived, Featured: c.Featured,
		MissingImages: c.MissingImages, MissingCoordinates: c.MissingCoordinates, MissingVideos: c.MissingVideos,
		WithGigs: c.WithGigs, DistrictsCovered: c.DistrictsCovered, TotalDistricts: c.TotalDistricts}
	err = r.q(ctx).Raw(`SELECT category, COUNT(*) AS count FROM tourist_spots
		WHERE deleted_at IS NULL GROUP BY category ORDER BY count DESC, category`).Scan(&st.ByCategory).Error
	return &st, err
}

func (r *postgres) Briefs(ctx context.Context, ids []uuid.UUID) ([]domain.Brief, error) {
	var out []domain.Brief
	err := r.q(ctx).Model(&domain.Spot{}).Select("id, title, status, gig_count").Where("id IN ?", ids).Scan(&out).Error
	return out, err
}

func (r *postgres) BulkUpdate(ctx context.Context, ids []uuid.UUID, fields map[string]any) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	fields["updated_at"] = time.Now().UTC()
	res := r.q(ctx).Model(&domain.Spot{}).Where("id IN ?", ids).Updates(fields)
	return res.RowsAffected, res.Error
}

func (r *postgres) BulkSoftDelete(ctx context.Context, ids []uuid.UUID) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	// gig_count is re-checked in SQL so a gig created meanwhile still blocks the delete.
	res := r.q(ctx).Where("id IN ? AND gig_count = 0", ids).Delete(&domain.Spot{})
	return res.RowsAffected, res.Error
}
