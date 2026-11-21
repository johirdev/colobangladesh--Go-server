package service

import (
	"context"
	"errors"
	"log/slog"
	"mime/multipart"
	"strings"
	"time"

	"cholobangladesh-server/internal/modules/user/domain"
	"cholobangladesh-server/internal/modules/user/dto"
	"cholobangladesh-server/pkg/imageutil"
	"cholobangladesh-server/pkg/phone"
	"cholobangladesh-server/pkg/validation"

	"github.com/google/uuid"
)

func (s *Service) GetMe(ctx context.Context, userID uuid.UUID) (*dto.MeResponse, error) {
	u, err := s.Repo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, notFound(err, domain.ErrUserNotFound)
	}
	return s.buildMe(ctx, u)
}

// UpdateProfile applies a partial update. Empty strings clear optional fields.
func (s *Service) UpdateProfile(ctx context.Context, userID uuid.UUID, req dto.UpdateProfileRequest) (*dto.MeResponse, error) {
	u, err := s.Repo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, notFound(err, domain.ErrUserNotFound)
	}

	f := map[string]any{}
	optional := func(col string, v *string, transform func(string) string) {
		if v == nil {
			return
		}
		t := strings.TrimSpace(*v)
		if t == "" {
			f[col] = nil
			return
		}
		if transform != nil {
			t = transform(t)
		}
		f[col] = t
	}

	if req.FullName != nil {
		name := strings.TrimSpace(*req.FullName)
		if len(name) < 2 {
			return nil, validation.Field("full_name", "required", "is required")
		}
		f["full_name"] = name
	}
	optional("bio", req.Bio, nil)
	optional("gender", req.Gender, nil)
	optional("city", req.City, nil)
	optional("country_code", req.CountryCode, strings.ToUpper)
	if req.PreferredLanguage != nil && *req.PreferredLanguage != "" {
		f["preferred_language"] = *req.PreferredLanguage
	}
	if req.WhatsappNumber != nil {
		if *req.WhatsappNumber == "" {
			f["whatsapp_number"] = nil
		} else {
			n, _ := phone.NormalizeE164(*req.WhatsappNumber)
			f["whatsapp_number"] = n
		}
	}
	if req.DateOfBirth != nil {
		if *req.DateOfBirth == "" {
			f["date_of_birth"] = nil
		} else {
			dob, err := time.Parse(dateLayout, *req.DateOfBirth)
			if err != nil || dob.After(now()) || dob.Year() < 1900 {
				return nil, validation.Field("date_of_birth", "datetime", "must be a valid past date (YYYY-MM-DD)")
			}
			minAge := domain.MinUserAge
			if u.Role == domain.RoleGuide {
				minAge = domain.MinGuideAge
			}
			if domain.Age(dob, now()) < minAge {
				if u.Role == domain.RoleGuide {
					return nil, domain.ErrUnderage
				}
				return nil, validation.Field("date_of_birth", "min_age", "you must be at least 13 years old")
			}
			f["date_of_birth"] = dob
		}
	}
	// A verified guide's identity data must match the reviewed NID.
	if u.Role == domain.RoleGuide && u.GuideProfile != nil && u.GuideProfile.VerificationStatus == domain.VerificationApproved {
		if _, ok := f["date_of_birth"]; ok {
			return nil, validation.Field("date_of_birth", "locked", "cannot be changed after identity verification; contact support")
		}
	}

	if len(f) > 0 {
		if err := s.Repo.UpdateProfile(ctx, userID, f); err != nil {
			return nil, err
		}
	}
	return s.GetMe(ctx, userID)
}

func (s *Service) UploadAvatar(ctx context.Context, userID uuid.UUID, fh *multipart.FileHeader) (*dto.MeResponse, error) {
	if s.Storage == nil {
		return nil, domain.ErrStorageUnavailable
	}
	img, err := imageutil.Read(fh, s.cfg.MaxUploadBytes, 100)
	if err != nil {
		return nil, err
	}
	up, err := s.Storage.UploadPublic(ctx, img.Data, "avatars", userID.String())
	if err != nil {
		slog.Error("avatar upload failed", "err", err)
		return nil, domain.ErrStorageUnavailable
	}
	if err := s.Repo.UpdateProfile(ctx, userID, map[string]any{"avatar_url": up.URL, "avatar_public_id": up.PublicID}); err != nil {
		return nil, err
	}
	return s.GetMe(ctx, userID)
}

func (s *Service) DeleteAvatar(ctx context.Context, userID uuid.UUID) (*dto.MeResponse, error) {
	u, err := s.Repo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, notFound(err, domain.ErrUserNotFound)
	}
	if u.Profile != nil && u.Profile.AvatarPublicID != nil && s.Storage != nil {
		if err := s.Storage.Delete(ctx, *u.Profile.AvatarPublicID, false); err != nil {
			slog.Warn("avatar delete failed", "err", err)
		}
	}
	if err := s.Repo.UpdateProfile(ctx, userID, map[string]any{"avatar_url": nil, "avatar_public_id": nil}); err != nil {
		return nil, err
	}
	return s.GetMe(ctx, userID)
}

// ChangePassword keeps the current device signed in and signs out all others.
func (s *Service) ChangePassword(ctx context.Context, userID, sid uuid.UUID, req dto.ChangePasswordRequest, meta domain.RequestMeta) error {
	u, err := s.Repo.FindUserByID(ctx, userID)
	if err != nil {
		return notFound(err, domain.ErrUserNotFound)
	}
	lockKey := "pwchange:" + userID.String()
	if ttl, _ := s.Store.TTL(ctx, "login:block:"+lockKey); ttl > 0 {
		return domain.ErrAccountLocked.WithRetryAfter(ttl)
	}
	if u.HasPassword() {
		if req.CurrentPassword == "" {
			return validation.Field("current_password", "required", "is required")
		}
		if !s.Hasher.Verify(*u.PasswordHash, req.CurrentPassword) {
			if err := s.loginFailed(ctx, lockKey, u, meta); errors.Is(err, domain.ErrAccountLocked) {
				return err
			}
			return domain.ErrWrongPassword
		}
		if s.Hasher.Verify(*u.PasswordHash, req.NewPassword) {
			return domain.ErrSamePassword
		}
	}
	h, err := s.Hasher.Hash(req.NewPassword)
	if err != nil {
		return err
	}
	if err := s.Repo.UpdateUser(ctx, userID, map[string]any{"password_hash": h}); err != nil {
		return err
	}
	if err := s.revokeOtherSessions(ctx, userID, sid); err != nil {
		return err
	}
	s.audit(ctx, &userID, &userID, domain.EventPasswordChanged, meta, nil)
	s.notifySecurity(u, "Your password was changed", "Your password was just changed and other devices were signed out. "+
		"If this wasn't you, reset your password immediately.")
	return nil
}

// ------------------------------------------------------------------ contacts

// SendContactOTP lets a user add/verify an email (anyone) or a BD phone (anyone; required for guides).
func (s *Service) SendContactOTP(ctx context.Context, userID uuid.UUID, channel, raw string, meta domain.RequestMeta) (*dto.OTPSentResponse, error) {
	u, err := s.Repo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, notFound(err, domain.ErrUserNotFound)
	}
	value, err := s.normalizeContact(channel, raw)
	if err != nil {
		return nil, err
	}
	if err := s.ensureContactFree(ctx, u, channel, value); err != nil {
		return nil, err
	}
	return s.issueOTP(ctx, otpRequest{purpose: purposeContact, channel: channel, key: userID.String() + ":" + value,
		destination: value, ip: meta.IP, deliver: true})
}

func (s *Service) VerifyContactOTP(ctx context.Context, userID uuid.UUID, channel, raw, code string, meta domain.RequestMeta) (*dto.MeResponse, error) {
	u, err := s.Repo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, notFound(err, domain.ErrUserNotFound)
	}
	value, err := s.normalizeContact(channel, raw)
	if err != nil {
		return nil, err
	}
	if err := s.verifyOTP(ctx, purposeContact, userID.String()+":"+value, value, code); err != nil {
		return nil, err
	}
	if err := s.ensureContactFree(ctx, u, channel, value); err != nil {
		return nil, err
	}
	fields := map[string]any{"email": value, "email_verified_at": now()}
	taken := domain.ErrEmailTaken
	if channel == channelSMS {
		fields = map[string]any{"phone": value, "phone_verified_at": now()}
		taken = domain.ErrPhoneTaken
	}
	if err := s.Repo.UpdateUser(ctx, userID, fields); err != nil {
		if errors.Is(err, domain.ErrDuplicate) {
			return nil, taken
		}
		return nil, err
	}
	s.audit(ctx, &userID, &userID, domain.EventContactVerified, meta, map[string]any{"channel": channel})
	return s.GetMe(ctx, userID)
}

func (s *Service) normalizeContact(channel, raw string) (string, error) {
	if channel == channelEmail {
		if e, ok := normalizeEmail(raw); ok {
			return e, nil
		}
		return "", validation.Field("email", "email", "must be a valid email address")
	}
	if p, ok := phone.NormalizeBD(raw); ok {
		return p, nil
	}
	return "", validation.Field("phone", "bd_phone", "must be a valid Bangladeshi mobile number")
}

func (s *Service) ensureContactFree(ctx context.Context, u *domain.User, channel, value string) error {
	if channel == channelEmail {
		if u.Email != nil && *u.Email == value && u.EmailVerified() {
			return domain.ErrContactAlreadyYours
		}
		if exists, err := s.Repo.EmailExists(ctx, value); err != nil {
			return err
		} else if exists {
			return domain.ErrEmailTaken
		}
		return nil
	}
	if u.Phone != nil && *u.Phone == value && u.PhoneVerified() {
		return domain.ErrContactAlreadyYours
	}
	if exists, err := s.Repo.PhoneExists(ctx, value); err != nil {
		return err
	} else if exists {
		return domain.ErrPhoneTaken
	}
	return nil
}

// ------------------------------------------------------------------ account deletion

// DeleteAccount soft-deletes the account (required by Apple App Store / GDPR-style requests).
// Email/phone become free for a new registration.
func (s *Service) DeleteAccount(ctx context.Context, userID uuid.UUID, req dto.DeleteAccountRequest, meta domain.RequestMeta) error {
	u, err := s.Repo.FindUserByID(ctx, userID)
	if err != nil {
		return notFound(err, domain.ErrUserNotFound)
	}
	if u.Role == domain.RoleAdmin {
		return domain.ErrCannotModifyAdmin
	}
	if u.HasPassword() {
		if req.Password == "" {
			return domain.ErrPasswordRequired
		}
		if !s.Hasher.Verify(*u.PasswordHash, req.Password) {
			return domain.ErrWrongPassword
		}
	}
	err = s.Repo.Tx(ctx, func(r domain.Repository) error {
		if _, err := r.RevokeAllSessions(ctx, userID, nil); err != nil {
			return err
		}
		if err := r.DeleteIdentities(ctx, userID); err != nil {
			return err
		}
		if u.GuideProfile != nil {
			if err := r.UpdateGuideProfile(ctx, userID, map[string]any{"is_available": false}); err != nil {
				return err
			}
		}
		return r.SoftDeleteUser(ctx, userID)
	})
	if err != nil {
		return err
	}
	s.invalidateAuthState(ctx, userID)
	if u.Profile != nil && u.Profile.AvatarPublicID != nil && s.Storage != nil {
		pid := *u.Profile.AvatarPublicID
		s.async("delete_avatar", func(ctx context.Context) error { return s.Storage.Delete(ctx, pid, false) })
	}
	s.audit(ctx, &userID, &userID, domain.EventAccountDeleted, meta, nil)
	return nil
}
