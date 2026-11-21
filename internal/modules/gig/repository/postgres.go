// Package repository implements the gig repository with PostgreSQL (GORM).
package repository

import (
	"context"
	"errors"
	"time"

	"cholobangladesh-server/internal/modules/gig/domain"
	"cholobangladesh-server/internal/shared/web"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) domain.Repository { return &postgres{db: db} }

func (r *postgres) q(ctx context.Context) *gorm.DB { return r.db.WithContext(ctx) }

func (r *postgres) Eligibility(ctx context.Context, guideID uuid.UUID) (*domain.Eligibility, error) {
	var e domain.Eligibility
	err := r.q(ctx).Raw(`SELECT u.role, u.status, COALESCE(gp.verification_status, '') AS verification_status
		FROM users u LEFT JOIN guide_profiles gp ON gp.user_id = u.id
		WHERE u.id = ? AND u.deleted_at IS NULL`, guideID).Scan(&e).Error
	if err != nil {
		return nil, err
	}
	if e.Role == "" {
		return nil, web.ErrNotFound
	}
	return &e, nil
}

func (r *postgres) Spot(ctx context.Context, spotID uuid.UUID) (*domain.SpotRef, error) {
	var s domain.SpotRef
	err := r.q(ctx).Where("id = ?", spotID).Take(&s).Error
	return &s, web.MapDBErr(err)
}

func (r *postgres) CountGigs(ctx context.Context, guideID uuid.UUID) (int64, error) {
	var n int64
	err := r.q(ctx).Model(&domain.Gig{}).Where("guide_id = ?", guideID).Count(&n).Error
	return n, err
}

func (r *postgres) Quota(ctx context.Context, guideID uuid.UUID) (*domain.Quota, error) {
	var q domain.Quota
	err := r.q(ctx).Where("guide_id = ?", guideID).Take(&q).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &domain.Quota{GuideID: guideID}, nil
	}
	return &q, err
}

func (r *postgres) SetQuota(ctx context.Context, q *domain.Quota) error {
	q.UpdatedAt = time.Now().UTC()
	return r.q(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "guide_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"extra_slots", "note", "updated_by", "updated_at"}),
	}).Create(q).Error
}

func (r *postgres) CreateWithinLimit(ctx context.Context, g *domain.Gig, limit int) (bool, error) {
	ok := false
	err := r.q(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", "gig-quota:"+g.GuideID.String()).Error; err != nil {
			return err
		}
		var n int64
		if err := tx.Model(&domain.Gig{}).Where("guide_id = ?", g.GuideID).Count(&n).Error; err != nil {
			return err
		}
		if int(n) >= limit {
			return nil
		}
		if err := tx.Omit("Spot", "Profile", "Guide").Create(g).Error; err != nil {
			return web.MapDBErr(err)
		}
		ok = true
		return nil
	})
	return ok, err
}

func (r *postgres) Update(ctx context.Context, id uuid.UUID, fields map[string]any) error {
	fields["updated_at"] = time.Now().UTC()
	res := r.q(ctx).Model(&domain.Gig{}).Where("id = ?", id).Updates(fields)
	if res.Error != nil {
		return web.MapDBErr(res.Error)
	}
	if res.RowsAffected == 0 {
		return web.ErrNotFound
	}
	return nil
}

func (r *postgres) SoftDelete(ctx context.Context, id uuid.UUID) error {
	res := r.q(ctx).Where("id = ?", id).Delete(&domain.Gig{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return web.ErrNotFound
	}
	return nil
}

func (r *postgres) withRefs(q *gorm.DB) *gorm.DB {
	return q.Preload("Spot", func(db *gorm.DB) *gorm.DB { return db.Unscoped() }).Preload("Spot.District").
		Preload("Profile").Preload("Guide")
}

func (r *postgres) Get(ctx context.Context, id uuid.UUID) (*domain.Gig, error) {
	var g domain.Gig
	err := r.withRefs(r.q(ctx)).Where("guide_gigs.id = ?", id).Take(&g).Error
	return &g, web.MapDBErr(err)
}

// bookable restricts a query to gigs tourists may see and book.
func bookable(q *gorm.DB) *gorm.DB {
	return q.Joins("JOIN users gu ON gu.id = guide_gigs.guide_id AND gu.status = 'active' AND gu.deleted_at IS NULL").
		Joins("JOIN guide_profiles gp ON gp.user_id = guide_gigs.guide_id AND gp.verification_status = 'approved' AND gp.is_available").
		Joins("JOIN tourist_spots ts ON ts.id = guide_gigs.spot_id AND ts.status = 'published' AND ts.deleted_at IS NULL").
		Where("guide_gigs.status = ?", domain.StatusActive)
}

func (r *postgres) GetPublic(ctx context.Context, id uuid.UUID) (*domain.Gig, error) {
	var g domain.Gig
	err := r.withRefs(bookable(r.q(ctx).Model(&domain.Gig{}))).Where("guide_gigs.id = ?", id).Take(&g).Error
	return &g, web.MapDBErr(err)
}

func (r *postgres) ListByGuide(ctx context.Context, guideID uuid.UUID) ([]domain.Gig, error) {
	var out []domain.Gig
	err := r.withRefs(r.q(ctx)).Where("guide_id = ?", guideID).Order("created_at DESC").Find(&out).Error
	return out, err
}

var sorts = map[string]string{
	"rating":     "guide_gigs.rating_avg DESC, guide_gigs.rating_count DESC",
	"price_low":  "guide_gigs.price ASC",
	"price_high": "guide_gigs.price DESC",
	"newest":     "guide_gigs.created_at DESC",
	"popular":    "guide_gigs.completed_count DESC, guide_gigs.rating_avg DESC",
}

func filter(q *gorm.DB, f domain.Filter) *gorm.DB {
	if f.GuideID != uuid.Nil {
		q = q.Where("guide_gigs.guide_id = ?", f.GuideID)
	}
	if f.SpotID != uuid.Nil {
		q = q.Where("guide_gigs.spot_id = ?", f.SpotID)
	}
	if f.DistrictID > 0 {
		q = q.Where("guide_gigs.spot_id IN (SELECT id FROM tourist_spots WHERE district_id = ?)", f.DistrictID)
	}
	if f.DivisionID > 0 {
		q = q.Where("guide_gigs.spot_id IN (SELECT id FROM tourist_spots WHERE division_id = ?)", f.DivisionID)
	}
	if f.Language != "" {
		q = q.Where("guide_gigs.languages @> ARRAY[?]::text[]", f.Language)
	}
	if f.MinPrice != nil {
		q = q.Where("guide_gigs.price >= ?", *f.MinPrice)
	}
	if f.MaxPrice != nil {
		q = q.Where("guide_gigs.price <= ?", *f.MaxPrice)
	}
	if f.Status != "" {
		q = q.Where("guide_gigs.status = ?", f.Status)
	}
	if f.Query != "" {
		q = q.Where("guide_gigs.title ILIKE ?", web.Like(f.Query))
	}
	return q
}

func (r *postgres) list(q *gorm.DB, f domain.Filter) ([]domain.Gig, int64, error) {
	q = filter(q, f)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	order, ok := sorts[f.Sort]
	if !ok {
		order = sorts["rating"]
	}
	var out []domain.Gig
	err := r.withRefs(q).Order(order).Order("guide_gigs.id").Offset((f.Page - 1) * f.Limit).Limit(f.Limit).Find(&out).Error
	return out, total, err
}

func (r *postgres) ListPublic(ctx context.Context, f domain.Filter) ([]domain.Gig, int64, error) {
	return r.list(bookable(r.q(ctx).Model(&domain.Gig{})), f)
}

func (r *postgres) ListAll(ctx context.Context, f domain.Filter) ([]domain.Gig, int64, error) {
	return r.list(r.q(ctx).Model(&domain.Gig{}), f)
}

func (r *postgres) RefreshSpotGigCount(ctx context.Context, spotID uuid.UUID) error {
	return r.q(ctx).Exec(`UPDATE tourist_spots SET gig_count =
		(SELECT count(*) FROM guide_gigs WHERE spot_id = ? AND status = 'active' AND deleted_at IS NULL) WHERE id = ?`,
		spotID, spotID).Error
}
