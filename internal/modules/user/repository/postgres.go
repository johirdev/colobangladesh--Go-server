// Package repository implements the domain ports with PostgreSQL (GORM) and Redis.
package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"cholobangladesh-server/internal/modules/user/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type postgres struct{ db *gorm.DB }

func NewPostgres(db *gorm.DB) domain.Repository { return &postgres{db: db} }

func (r *postgres) q(ctx context.Context) *gorm.DB { return r.db.WithContext(ctx) }

func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return domain.ErrNotFound
	case errors.Is(err, gorm.ErrDuplicatedKey):
		return domain.ErrDuplicate
	}
	return err
}

func (r *postgres) Tx(ctx context.Context, fn func(domain.Repository) error) error {
	return r.q(ctx).Transaction(func(tx *gorm.DB) error { return fn(&postgres{db: tx}) })
}

// ------------------------------------------------------------------ users

func (r *postgres) CreateUser(ctx context.Context, u *domain.User) error {
	return mapErr(r.q(ctx).Create(u).Error)
}

func (r *postgres) findUser(ctx context.Context, where string, arg any) (*domain.User, error) {
	var u domain.User
	err := r.q(ctx).Preload("Profile").Preload("GuideProfile").Where(where, arg).Take(&u).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return &u, nil
}

func (r *postgres) FindUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return r.findUser(ctx, "id = ?", id)
}

func (r *postgres) FindUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.findUser(ctx, "email = ?", email)
}

func (r *postgres) FindUserByPhone(ctx context.Context, phone string) (*domain.User, error) {
	return r.findUser(ctx, "phone = ?", phone)
}

func (r *postgres) exists(ctx context.Context, where string, arg any) (bool, error) {
	var n int64
	err := r.q(ctx).Model(&domain.User{}).Where(where, arg).Limit(1).Count(&n).Error
	return n > 0, err
}

func (r *postgres) EmailExists(ctx context.Context, email string) (bool, error) {
	return r.exists(ctx, "email = ?", email)
}

func (r *postgres) PhoneExists(ctx context.Context, phone string) (bool, error) {
	return r.exists(ctx, "phone = ?", phone)
}

func (r *postgres) UpdateUser(ctx context.Context, id uuid.UUID, fields map[string]any) error {
	fields["updated_at"] = time.Now().UTC()
	res := r.q(ctx).Model(&domain.User{}).Where("id = ?", id).Updates(fields)
	if res.Error != nil {
		return mapErr(res.Error)
	}
	if res.RowsAffected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *postgres) BumpTokenVersion(ctx context.Context, id uuid.UUID) (int, error) {
	var v int
	err := r.q(ctx).Raw(`UPDATE users SET token_version = token_version + 1, updated_at = now()
		WHERE id = ? RETURNING token_version`, id).Scan(&v).Error
	return v, err
}

func (r *postgres) SoftDeleteUser(ctx context.Context, id uuid.UUID) error {
	return r.q(ctx).Exec(`UPDATE users SET status = 'deleted', deleted_at = now(), updated_at = now(),
		token_version = token_version + 1 WHERE id = ? AND deleted_at IS NULL`, id).Error
}

func (r *postgres) AuthState(ctx context.Context, id uuid.UUID) (int, domain.Status, error) {
	var row struct {
		TokenVersion int
		Status       domain.Status
	}
	err := r.q(ctx).Model(&domain.User{}).Select("token_version", "status").Where("id = ?", id).Take(&row).Error
	return row.TokenVersion, row.Status, mapErr(err)
}

func likePattern(s string) string {
	s = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(strings.TrimSpace(s))
	return "%" + s + "%"
}

func (r *postgres) ListUsers(ctx context.Context, f domain.UserFilter) ([]domain.User, int64, error) {
	q := r.q(ctx).Model(&domain.User{}).Joins("LEFT JOIN user_profiles p ON p.user_id = users.id")
	if f.Role != "" {
		q = q.Where("users.role = ?", f.Role)
	}
	if f.Status != "" {
		q = q.Where("users.status = ?", f.Status)
	}
	if f.Query != "" {
		p := likePattern(f.Query)
		q = q.Where("(users.email ILIKE ? OR users.phone ILIKE ? OR p.full_name ILIKE ?)", p, p, p)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var users []domain.User
	err := q.Preload("Profile").Preload("GuideProfile").Order("users.created_at DESC").
		Offset((f.Page - 1) * f.Limit).Limit(f.Limit).Find(&users).Error
	return users, total, err
}

// ------------------------------------------------------------------ profiles

func (r *postgres) UpdateProfile(ctx context.Context, userID uuid.UUID, fields map[string]any) error {
	fields["updated_at"] = time.Now().UTC()
	return mapErr(r.q(ctx).Model(&domain.UserProfile{}).Where("user_id = ?", userID).Updates(fields).Error)
}

func (r *postgres) UpdateGuideProfile(ctx context.Context, userID uuid.UUID, fields map[string]any) error {
	fields["updated_at"] = time.Now().UTC()
	return mapErr(r.q(ctx).Model(&domain.GuideProfile{}).Where("user_id = ?", userID).Updates(fields).Error)
}

var guideSorts = map[string]string{
	"rating":     "gp.rating_avg DESC, gp.rating_count DESC",
	"experience": "gp.experience_years DESC",
	"price_low":  "gp.hourly_rate ASC NULLS LAST",
	"price_high": "gp.hourly_rate DESC NULLS LAST",
	"newest":     "gp.verified_at DESC NULLS LAST",
}

func (r *postgres) SearchGuides(ctx context.Context, f domain.GuideFilter) ([]domain.User, int64, error) {
	q := r.q(ctx).Model(&domain.User{}).
		Joins("JOIN guide_profiles gp ON gp.user_id = users.id").
		Joins("JOIN user_profiles p ON p.user_id = users.id").
		Where("users.role = ? AND users.status = ?", domain.RoleGuide, domain.StatusActive).
		Where("gp.verification_status = ? AND gp.is_available = TRUE", domain.VerificationApproved)

	if f.Area != "" {
		q = q.Where("gp.service_areas @> ARRAY[?]::text[]", f.Area)
	}
	if f.Language != "" {
		q = q.Where("gp.languages @> ARRAY[?]::text[]", f.Language)
	}
	if f.Specialty != "" {
		q = q.Where("gp.specialties @> ARRAY[?]::text[]", f.Specialty)
	}
	if f.MinRate != nil {
		q = q.Where("gp.hourly_rate >= ?", *f.MinRate)
	}
	if f.MaxRate != nil {
		q = q.Where("gp.hourly_rate <= ?", *f.MaxRate)
	}
	if f.Query != "" {
		p := likePattern(f.Query)
		q = q.Where("(p.full_name ILIKE ? OR gp.headline ILIKE ?)", p, p)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	order, ok := guideSorts[f.Sort]
	if !ok {
		order = guideSorts["rating"]
	}
	var users []domain.User
	err := q.Preload("Profile").Preload("GuideProfile").Order(order).Order("users.id").
		Offset((f.Page - 1) * f.Limit).Limit(f.Limit).Find(&users).Error
	return users, total, err
}

// ------------------------------------------------------------------ identities

func (r *postgres) FindIdentity(ctx context.Context, provider, subject string) (*domain.AuthIdentity, error) {
	var i domain.AuthIdentity
	err := r.q(ctx).Where("provider = ? AND provider_subject = ?", provider, subject).Take(&i).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return &i, nil
}

func (r *postgres) CreateIdentity(ctx context.Context, i *domain.AuthIdentity) error {
	return mapErr(r.q(ctx).Create(i).Error)
}

func (r *postgres) TouchIdentity(ctx context.Context, id uuid.UUID) error {
	return r.q(ctx).Model(&domain.AuthIdentity{}).Where("id = ?", id).Update("last_used_at", time.Now().UTC()).Error
}

func (r *postgres) ListIdentityProviders(ctx context.Context, userID uuid.UUID) ([]string, error) {
	var out []string
	err := r.q(ctx).Model(&domain.AuthIdentity{}).Where("user_id = ?", userID).Order("provider").Pluck("provider", &out).Error
	return out, err
}

func (r *postgres) DeleteIdentities(ctx context.Context, userID uuid.UUID) error {
	return r.q(ctx).Where("user_id = ?", userID).Delete(&domain.AuthIdentity{}).Error
}

// ------------------------------------------------------------------ sessions

func (r *postgres) CreateSession(ctx context.Context, s *domain.Session) error {
	return mapErr(r.q(ctx).Create(s).Error)
}

func (r *postgres) findSession(ctx context.Context, where string, arg any) (*domain.Session, error) {
	var s domain.Session
	if err := r.q(ctx).Where(where, arg).Take(&s).Error; err != nil {
		return nil, mapErr(err)
	}
	return &s, nil
}

func (r *postgres) FindSessionByID(ctx context.Context, id uuid.UUID) (*domain.Session, error) {
	return r.findSession(ctx, "id = ?", id)
}

func (r *postgres) FindSessionByTokenHash(ctx context.Context, hash string) (*domain.Session, error) {
	return r.findSession(ctx, "refresh_token_hash = ?", hash)
}

func (r *postgres) FindSessionByPreviousHash(ctx context.Context, hash string) (*domain.Session, error) {
	return r.findSession(ctx, "previous_token_hash = ?", hash)
}

func (r *postgres) RotateSession(ctx context.Context, id uuid.UUID, oldHash, newHash string, expiresAt time.Time, meta domain.RequestMeta) (bool, error) {
	now := time.Now().UTC()
	res := r.q(ctx).Model(&domain.Session{}).
		Where("id = ? AND refresh_token_hash = ? AND revoked_at IS NULL", id, oldHash).
		Updates(map[string]any{
			"refresh_token_hash":  newHash,
			"previous_token_hash": oldHash,
			"rotated_at":          now,
			"last_used_at":        now,
			"expires_at":          expiresAt,
			"ip_address":          meta.IP,
			"user_agent":          meta.UserAgent,
		})
	return res.RowsAffected == 1, res.Error
}

func (r *postgres) RevokeSession(ctx context.Context, userID, id uuid.UUID) (bool, error) {
	res := r.q(ctx).Model(&domain.Session{}).
		Where("id = ? AND user_id = ? AND revoked_at IS NULL", id, userID).
		Update("revoked_at", time.Now().UTC())
	return res.RowsAffected == 1, res.Error
}

func (r *postgres) RevokeAllSessions(ctx context.Context, userID uuid.UUID, except *uuid.UUID) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	var err error
	if except != nil {
		err = r.q(ctx).Raw(`UPDATE user_sessions SET revoked_at = now()
			WHERE user_id = ? AND revoked_at IS NULL AND id <> ? RETURNING id`, userID, *except).Scan(&ids).Error
	} else {
		err = r.q(ctx).Raw(`UPDATE user_sessions SET revoked_at = now()
			WHERE user_id = ? AND revoked_at IS NULL RETURNING id`, userID).Scan(&ids).Error
	}
	return ids, err
}

func (r *postgres) ListActiveSessions(ctx context.Context, userID uuid.UUID) ([]domain.Session, error) {
	var out []domain.Session
	err := r.q(ctx).Where("user_id = ? AND revoked_at IS NULL AND expires_at > now()", userID).
		Order("last_used_at DESC").Find(&out).Error
	return out, err
}

func (r *postgres) PurgeExpiredSessions(ctx context.Context, before time.Time) (int64, error) {
	res := r.q(ctx).Where("expires_at < ? OR revoked_at < ?", before, before).Delete(&domain.Session{})
	return res.RowsAffected, res.Error
}

// ------------------------------------------------------------------ verifications

func (r *postgres) CreateVerification(ctx context.Context, v *domain.GuideVerification) error {
	return mapErr(r.q(ctx).Omit("User").Create(v).Error)
}

func (r *postgres) FindVerificationByID(ctx context.Context, id uuid.UUID) (*domain.GuideVerification, error) {
	var v domain.GuideVerification
	err := r.q(ctx).Preload("User.Profile").Preload("User.GuideProfile").Where("id = ?", id).Take(&v).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return &v, nil
}

func (r *postgres) FindLatestVerification(ctx context.Context, userID uuid.UUID) (*domain.GuideVerification, error) {
	var v domain.GuideVerification
	err := r.q(ctx).Where("user_id = ?", userID).Order("created_at DESC").Take(&v).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return &v, nil
}

func (r *postgres) ListVerifications(ctx context.Context, f domain.VerificationFilter) ([]domain.GuideVerification, int64, error) {
	q := r.q(ctx).Model(&domain.GuideVerification{})
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []domain.GuideVerification
	err := q.Preload("User.Profile").Order("created_at ASC"). // oldest first = fair review queue
									Offset((f.Page - 1) * f.Limit).Limit(f.Limit).Find(&out).Error
	return out, total, err
}

func (r *postgres) ReviewVerification(ctx context.Context, id uuid.UUID, status domain.VerificationStatus, reviewer uuid.UUID, reason *string) (bool, error) {
	now := time.Now().UTC()
	res := r.q(ctx).Model(&domain.GuideVerification{}).
		Where("id = ? AND status = ?", id, domain.VerificationPending).
		Updates(map[string]any{
			"status":           status,
			"reviewed_by":      reviewer,
			"reviewed_at":      now,
			"rejection_reason": reason,
			"updated_at":       now,
		})
	return res.RowsAffected == 1, res.Error
}

// ------------------------------------------------------------------ audit

func (r *postgres) Audit(ctx context.Context, l *domain.AuditLog) error {
	return r.q(ctx).Create(l).Error
}
