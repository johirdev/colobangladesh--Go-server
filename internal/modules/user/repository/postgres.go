// Package repository implements the domain ports with PostgreSQL (GORM) and Redis.
package repository

import (
	"context"
	"errors"
	"time"

	"cholobangladesh-server/internal/modules/user/domain"
	"cholobangladesh-server/pkg/listing"

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

// UserSpec: admin user table; ?q= matches email, phone or name.
var UserSpec = listing.Spec{
	Sorts: map[string]string{
		"newest": "users.created_at DESC",
		"oldest": "users.created_at ASC",
		"name":   "p.full_name ASC NULLS LAST",
		"login":  "users.last_login_at DESC NULLS LAST",
	},
	DefaultSort: "newest",
	Tiebreak:    "users.id",
	Search:      []string{"users.email", "users.phone", "p.full_name"},
}

func (r *postgres) ListUsers(ctx context.Context, f domain.UserFilter) (listing.Result[domain.User], error) {
	q := listing.Filter(r.q(ctx).Model(&domain.User{})).
		Joins("LEFT JOIN user_profiles p ON p.user_id = users.id").
		Eq("users.role", f.Role).
		Eq("users.status", f.Status)
	return listing.Find[domain.User](q, f.Params, UserSpec, func(db *gorm.DB) *gorm.DB {
		return db.Preload("Profile").Preload("GuideProfile")
	})
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

// GuideSpec: public guide directory (20 per page, max 50); ?q= matches name or headline.
var GuideSpec = listing.Spec{
	Sorts: map[string]string{
		"rating":     "gp.rating_avg DESC, gp.rating_count DESC",
		"experience": "gp.experience_years DESC",
		"price_low":  "gp.hourly_rate ASC NULLS LAST",
		"price_high": "gp.hourly_rate DESC NULLS LAST",
		"newest":     "gp.verified_at DESC NULLS LAST",
	},
	DefaultSort: "rating",
	Tiebreak:    "users.id",
	Search:      []string{"p.full_name", "gp.headline"},
	MaxLimit:    50,
}

func (r *postgres) SearchGuides(ctx context.Context, f domain.GuideFilter) (listing.Result[domain.User], error) {
	q := listing.Filter(r.q(ctx).Model(&domain.User{})).
		Joins("JOIN guide_profiles gp ON gp.user_id = users.id").
		Joins("JOIN user_profiles p ON p.user_id = users.id").
		Always("users.role = ? AND users.status = ?", domain.RoleGuide, domain.StatusActive).
		Always("gp.verification_status = ? AND gp.is_available = TRUE", domain.VerificationApproved).
		Has("gp.service_areas", f.Area).
		Has("gp.languages", f.Language).
		Has("gp.specialties", f.Specialty).
		Gte("gp.hourly_rate", f.MinRate).
		Lte("gp.hourly_rate", f.MaxRate)
	return listing.Find[domain.User](q, f.Params, GuideSpec, func(db *gorm.DB) *gorm.DB {
		return db.Preload("Profile").Preload("GuideProfile")
	})
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

// VerificationSpec: KYC queue — oldest first by default (fair review order); ?q= matches name or phone.
var VerificationSpec = listing.Spec{
	Sorts: map[string]string{
		"oldest": "guide_verifications.created_at ASC",
		"newest": "guide_verifications.created_at DESC",
	},
	DefaultSort: "oldest",
	Tiebreak:    "guide_verifications.id",
	Search: []string{
		"EXISTS (SELECT 1 FROM user_profiles vp WHERE vp.user_id = guide_verifications.user_id AND vp.full_name ILIKE ?)",
		"EXISTS (SELECT 1 FROM users vu WHERE vu.id = guide_verifications.user_id AND (vu.phone ILIKE ? OR vu.email ILIKE ?))",
	},
}

func (r *postgres) ListVerifications(ctx context.Context, f domain.VerificationFilter) (listing.Result[domain.GuideVerification], error) {
	q := listing.Filter(r.q(ctx).Model(&domain.GuideVerification{})).Eq("guide_verifications.status", f.Status)
	return listing.Find[domain.GuideVerification](q, f.Params, VerificationSpec, func(db *gorm.DB) *gorm.DB {
		return db.Preload("User.Profile")
	})
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
