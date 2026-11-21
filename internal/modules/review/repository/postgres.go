// Package repository implements the review repository with PostgreSQL (GORM).
package repository

import (
	"context"
	"fmt"
	"time"

	"cholobangladesh-server/internal/modules/review/domain"
	"cholobangladesh-server/internal/shared/web"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) domain.Repository { return &postgres{db: db} }

func (r *postgres) q(ctx context.Context) *gorm.DB { return r.db.WithContext(ctx) }

func (r *postgres) Booking(ctx context.Context, id uuid.UUID) (*domain.BookingInfo, error) {
	var b domain.BookingInfo
	err := r.q(ctx).Raw("SELECT id, user_id, guide_id, gig_id, spot_id, status FROM bookings WHERE id = ?", id).Scan(&b).Error
	if err != nil {
		return nil, err
	}
	if b.ID == uuid.Nil {
		return nil, web.ErrNotFound
	}
	return &b, nil
}

func (r *postgres) Create(ctx context.Context, rv *domain.Review) error {
	return r.q(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Author", "Gig").Create(rv).Error; err != nil { // Images are created with it
			return web.MapDBErr(err)
		}
		return refresh(tx, rv.GigID, rv.GuideID, rv.SpotID)
	})
}

func withRefs(q *gorm.DB) *gorm.DB {
	return q.Preload("Images", func(db *gorm.DB) *gorm.DB { return db.Order("position") }).Preload("Author").
		Preload("Gig", func(db *gorm.DB) *gorm.DB { return db.Unscoped() })
}

func (r *postgres) Get(ctx context.Context, id uuid.UUID) (*domain.Review, error) {
	var rv domain.Review
	err := withRefs(r.q(ctx)).Where("id = ?", id).Take(&rv).Error
	return &rv, web.MapDBErr(err)
}

func scope(q *gorm.DB, f domain.Filter) *gorm.DB {
	if !f.IncludeHide {
		q = q.Where("reviews.is_visible")
	}
	if f.GigID != uuid.Nil {
		q = q.Where("reviews.gig_id = ?", f.GigID)
	}
	if f.GuideID != uuid.Nil {
		q = q.Where("reviews.guide_id = ?", f.GuideID)
	}
	if f.SpotID != uuid.Nil {
		q = q.Where("reviews.spot_id = ?", f.SpotID)
	}
	if f.WithPhotos {
		q = q.Where("EXISTS (SELECT 1 FROM review_images ri WHERE ri.review_id = reviews.id)")
	}
	return q
}

var sorts = map[string]string{
	"newest":  "reviews.created_at DESC",
	"highest": "reviews.rating DESC, reviews.created_at DESC",
	"lowest":  "reviews.rating ASC, reviews.created_at DESC",
}

func (r *postgres) List(ctx context.Context, f domain.Filter) ([]domain.Review, int64, error) {
	q := scope(r.q(ctx).Model(&domain.Review{}), f)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	order, ok := sorts[f.Sort]
	if !ok {
		order = sorts["newest"]
	}
	var out []domain.Review
	err := withRefs(q).Order(order).Offset((f.Page - 1) * f.Limit).Limit(f.Limit).Find(&out).Error
	return out, total, err
}

func (r *postgres) Summary(ctx context.Context, f domain.Filter) (*domain.Summary, error) {
	f.WithPhotos = false
	var rows []struct {
		Rating int
		N      int64
	}
	if err := scope(r.q(ctx).Model(&domain.Review{}), f).Select("rating, count(*) AS n").Group("rating").Scan(&rows).Error; err != nil {
		return nil, err
	}
	s := &domain.Summary{Stars: map[int]int64{1: 0, 2: 0, 3: 0, 4: 0, 5: 0}}
	var sum int64
	for _, row := range rows {
		s.Stars[row.Rating] = row.N
		s.Count += row.N
		sum += int64(row.Rating) * row.N
	}
	if s.Count > 0 {
		s.Average = float64(sum*100/s.Count) / 100
	}
	return s, nil
}

func (r *postgres) Update(ctx context.Context, id uuid.UUID, fields map[string]any) error {
	fields["updated_at"] = time.Now().UTC()
	res := r.q(ctx).Model(&domain.Review{}).Where("id = ?", id).Updates(fields)
	if res.Error == nil && res.RowsAffected == 0 {
		return web.ErrNotFound
	}
	return res.Error
}

func (r *postgres) RefreshRatings(ctx context.Context, gigID, guideID, spotID uuid.UUID) error {
	return r.q(ctx).Transaction(func(tx *gorm.DB) error { return refresh(tx, gigID, guideID, spotID) })
}

func refresh(tx *gorm.DB, gigID, guideID, spotID uuid.UUID) error {
	const agg = `SELECT COALESCE(ROUND(AVG(rating)::numeric, 2), 0), count(*) FROM reviews WHERE is_visible AND %s = ?`
	stmts := []struct {
		sql string
		id  uuid.UUID
	}{
		{`UPDATE guide_gigs SET (rating_avg, rating_count) = (` + fmt.Sprintf(agg, "gig_id") + `) WHERE id = ?`, gigID},
		{`UPDATE guide_profiles SET (rating_avg, rating_count) = (` + fmt.Sprintf(agg, "guide_id") + `) WHERE user_id = ?`, guideID},
		{`UPDATE tourist_spots SET (rating_avg, rating_count) = (` + fmt.Sprintf(agg, "spot_id") + `) WHERE id = ?`, spotID},
	}
	for _, s := range stmts {
		if err := tx.Exec(s.sql, s.id, s.id).Error; err != nil {
			return err
		}
	}
	return nil
}
