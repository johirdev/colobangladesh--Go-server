package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"cholobangladesh-server/internal/config"
	"cholobangladesh-server/internal/modules/user/domain"
	"cholobangladesh-server/internal/modules/user/dto"
	"cholobangladesh-server/pkg/response"

	"github.com/google/uuid"
)

const documentURLTTL = 10 * time.Minute

func (s *Service) AdminListUsers(ctx context.Context, q dto.AdminUserQuery) (response.Page[dto.AdminUserResponse], error) {
	page, limit := pageDefaults(q.Page, q.Limit, 20)
	users, total, err := s.Repo.ListUsers(ctx, domain.UserFilter{
		Role: domain.Role(q.Role), Status: domain.Status(q.Status), Query: q.Q, Page: page, Limit: limit,
	})
	if err != nil {
		return response.Page[dto.AdminUserResponse]{}, err
	}
	items := make([]dto.AdminUserResponse, 0, len(users))
	for i := range users {
		items = append(items, toAdminUser(&users[i]))
	}
	return response.NewPage(items, page, limit, total), nil
}

func (s *Service) AdminGetUser(ctx context.Context, id uuid.UUID) (*dto.MeResponse, error) {
	return s.GetMe(ctx, id)
}

// AdminSetUserStatus suspends or re-activates an account. Suspension signs the user out everywhere.
func (s *Service) AdminSetUserStatus(ctx context.Context, adminID, targetID uuid.UUID, req dto.UpdateUserStatusRequest, meta domain.RequestMeta) (*dto.MeResponse, error) {
	u, err := s.Repo.FindUserByID(ctx, targetID)
	if err != nil {
		return nil, notFound(err, domain.ErrUserNotFound)
	}
	if u.Role == domain.RoleAdmin {
		return nil, domain.ErrCannotModifyAdmin
	}
	status := domain.Status(req.Status)
	if err := s.Repo.UpdateUser(ctx, targetID, map[string]any{"status": status}); err != nil {
		return nil, err
	}
	if status == domain.StatusSuspended {
		if err := s.revokeEverything(ctx, targetID); err != nil {
			return nil, err
		}
	}
	s.invalidateAuthState(ctx, targetID)
	s.audit(ctx, &targetID, &adminID, domain.EventStatusChanged, meta,
		map[string]any{"from": u.Status, "to": status, "reason": req.Reason})
	return s.GetMe(ctx, targetID)
}

func (s *Service) AdminListVerifications(ctx context.Context, q dto.VerificationQuery) (response.Page[dto.AdminVerificationResponse], error) {
	page, limit := pageDefaults(q.Page, q.Limit, 20)
	list, total, err := s.Repo.ListVerifications(ctx, domain.VerificationFilter{
		Status: domain.VerificationStatus(q.Status), Page: page, Limit: limit,
	})
	if err != nil {
		return response.Page[dto.AdminVerificationResponse]{}, err
	}
	items := make([]dto.AdminVerificationResponse, 0, len(list))
	for i := range list {
		items = append(items, toAdminVerification(&list[i]))
	}
	return response.NewPage(items, page, limit, total), nil
}

// AdminGetVerification decrypts the NID and creates 10-minute signed URLs for the documents.
func (s *Service) AdminGetVerification(ctx context.Context, adminID, id uuid.UUID, meta domain.RequestMeta) (*dto.AdminVerificationDetailResponse, error) {
	if s.Storage == nil {
		return nil, domain.ErrStorageUnavailable
	}
	v, err := s.Repo.FindVerificationByID(ctx, id)
	if err != nil {
		return nil, notFound(err, domain.ErrVerificationNotFound)
	}
	nid, err := s.Cipher.Decrypt(v.NIDNumberEncrypted)
	if err != nil {
		return nil, err
	}
	sign := func(pid string) string {
		u, err := s.Storage.SignedURL(pid, documentURLTTL)
		if err != nil {
			slog.Warn("sign url failed", "err", err)
		}
		return u
	}
	out := &dto.AdminVerificationDetailResponse{
		AdminVerificationResponse: toAdminVerification(v),
		NIDNumber:                 nid,
		NIDFrontURL:               sign(v.NIDFrontPublicID),
		NIDBackURL:                sign(v.NIDBackPublicID),
		SelfieURL:                 sign(v.SelfiePublicID),
	}
	if v.User != nil {
		if p := v.User.Profile; p != nil {
			out.AvatarURL = p.AvatarURL
			if p.DateOfBirth != nil {
				out.DateOfBirth = ptr(p.DateOfBirth.Format(dateLayout))
			}
		}
		out.GuideProfile = toGuideProfile(v.User.GuideProfile)
	}
	// Viewing identity documents is itself an auditable event.
	s.audit(ctx, &v.UserID, &adminID, "admin.kyc_viewed", meta, map[string]any{"verification_id": id})
	return out, nil
}

func (s *Service) AdminApproveVerification(ctx context.Context, adminID, id uuid.UUID, meta domain.RequestMeta) error {
	return s.review(ctx, adminID, id, domain.VerificationApproved, nil, meta)
}

func (s *Service) AdminRejectVerification(ctx context.Context, adminID, id uuid.UUID, reason string, meta domain.RequestMeta) error {
	r := strings.TrimSpace(reason)
	return s.review(ctx, adminID, id, domain.VerificationRejected, &r, meta)
}

func (s *Service) review(ctx context.Context, adminID, id uuid.UUID, status domain.VerificationStatus, reason *string, meta domain.RequestMeta) error {
	v, err := s.Repo.FindVerificationByID(ctx, id)
	if err != nil {
		return notFound(err, domain.ErrVerificationNotFound)
	}
	err = s.Repo.Tx(ctx, func(r domain.Repository) error {
		ok, err := r.ReviewVerification(ctx, id, status, adminID, reason)
		if err != nil {
			return err
		}
		if !ok {
			return domain.ErrVerificationReviewed
		}
		fields := map[string]any{"verification_status": status}
		if status == domain.VerificationApproved {
			fields["verified_at"] = now()
		}
		return r.UpdateGuideProfile(ctx, v.UserID, fields)
	})
	if err != nil {
		return err
	}

	event, msg := domain.EventKYCApproved, "Congratulations! Your guide profile is verified and now visible to travellers."
	if status == domain.VerificationRejected {
		event = domain.EventKYCRejected
		msg = "Your guide verification was not approved: " + *reason + ". Please fix it and resubmit in the app."
	}
	s.audit(ctx, &v.UserID, &adminID, event, meta, map[string]any{"verification_id": id, "reason": reason})
	if v.User != nil && v.User.PhoneVerified() {
		to := *v.User.Phone
		s.async("kyc_result_sms", func(ctx context.Context) error {
			return s.SMS.Send(ctx, to, s.cfg.AppName+": "+msg)
		})
	}
	return nil
}

// EnsureAdmin creates the bootstrap admin from ADMIN_EMAIL / ADMIN_PASSWORD if missing.
func (s *Service) EnsureAdmin(ctx context.Context, cfg config.AdminConfig) error {
	if cfg.Email == "" || cfg.Password == "" {
		return nil
	}
	email, ok := normalizeEmail(cfg.Email)
	if !ok {
		return errors.New("ADMIN_EMAIL is invalid")
	}
	if exists, err := s.Repo.EmailExists(ctx, email); err != nil || exists {
		return err
	}
	h, err := s.Hasher.Hash(cfg.Password)
	if err != nil {
		return err
	}
	t := now()
	u := &domain.User{ID: newID(), Role: domain.RoleAdmin, Status: domain.StatusActive,
		Email: &email, EmailVerifiedAt: &t, PasswordHash: &h, TokenVersion: 1}
	u.Profile = &domain.UserProfile{UserID: u.ID, FullName: cfg.FullName, PreferredLanguage: "en"}
	if err := s.Repo.CreateUser(ctx, u); err != nil {
		return err
	}
	slog.Info("bootstrap admin created", "email", email)
	return nil
}
