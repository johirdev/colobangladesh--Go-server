package repository

import (
	"context"

	"cholobangladesh-server/internal/modules/gig/domain"
	"cholobangladesh-server/internal/shared/web"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// orderedImages preloads a gig's gallery in display order (cover first).
func orderedImages(db *gorm.DB) *gorm.DB { return db.Order("position, created_at") }

// syncCover sets guide_gigs.cover_image_url to the first image (NULL → the spot cover is shown).
func syncCover(tx *gorm.DB, gigID uuid.UUID) error {
	return tx.Exec(`UPDATE guide_gigs SET cover_image_url =
		(SELECT url FROM guide_gig_images WHERE gig_id = ? ORDER BY position, created_at LIMIT 1), updated_at = now()
		WHERE id = ?`, gigID, gigID).Error
}

func (r *postgres) Images(ctx context.Context, gigID uuid.UUID) ([]domain.Image, error) {
	var out []domain.Image
	err := orderedImages(r.q(ctx).Where("gig_id = ?", gigID)).Find(&out).Error
	return out, err
}

func (r *postgres) AddImages(ctx context.Context, gigID uuid.UUID, imgs []domain.Image) error {
	return r.q(ctx).Transaction(func(tx *gorm.DB) error {
		var maxPos *int
		if err := tx.Model(&domain.Image{}).Where("gig_id = ?", gigID).Select("MAX(position)").Scan(&maxPos).Error; err != nil {
			return err
		}
		next := 0
		if maxPos != nil {
			next = *maxPos + 1
		}
		for i := range imgs {
			imgs[i].GigID, imgs[i].Position = gigID, next+i
		}
		if err := tx.Create(&imgs).Error; err != nil {
			return err
		}
		return syncCover(tx, gigID)
	})
}

func (r *postgres) UpdateImage(ctx context.Context, gigID, imageID uuid.UUID, fields map[string]any) error {
	return r.q(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&domain.Image{}).Where("id = ? AND gig_id = ?", imageID, gigID).Updates(fields)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return web.ErrNotFound
		}
		return syncCover(tx, gigID)
	})
}

// MakeCover moves the image to the front; the others keep their relative order.
func (r *postgres) MakeCover(ctx context.Context, gigID, imageID uuid.UUID) error {
	return r.q(ctx).Transaction(func(tx *gorm.DB) error {
		var ids []uuid.UUID
		if err := orderedImages(tx.Model(&domain.Image{}).Where("gig_id = ?", gigID)).Pluck("id", &ids).Error; err != nil {
			return err
		}
		order := []uuid.UUID{imageID}
		found := false
		for _, id := range ids {
			if id == imageID {
				found = true
				continue
			}
			order = append(order, id)
		}
		if !found {
			return web.ErrNotFound
		}
		for pos, id := range order {
			if err := tx.Model(&domain.Image{}).Where("id = ?", id).Update("position", pos).Error; err != nil {
				return err
			}
		}
		return syncCover(tx, gigID)
	})
}

func (r *postgres) DeleteImage(ctx context.Context, gigID, imageID uuid.UUID) (*domain.Image, error) {
	var img domain.Image
	err := r.q(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND gig_id = ?", imageID, gigID).Take(&img).Error; err != nil {
			return web.MapDBErr(err)
		}
		if err := tx.Delete(&img).Error; err != nil {
			return err
		}
		return syncCover(tx, gigID)
	})
	if err != nil {
		return nil, err
	}
	return &img, nil
}
