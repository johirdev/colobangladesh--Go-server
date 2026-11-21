package service

import (
	"context"
	"errors"
	"log/slog"
	"mime/multipart"
	"strings"

	"cholobangladesh-server/internal/modules/user/domain"
	"cholobangladesh-server/internal/modules/user/dto"
	"cholobangladesh-server/pkg/apperror"
	"cholobangladesh-server/pkg/imageutil"
	"cholobangladesh-server/pkg/response"
	"cholobangladesh-server/pkg/validation"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

func (s *Service) loadGuide(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	u, err := s.Repo.FindUserByID(ctx, userID)
	if err != nil {
		return nil, notFound(err, domain.ErrUserNotFound)
	}
	if u.Role != domain.RoleGuide || u.GuideProfile == nil {
		return nil, domain.ErrNotGuide
	}
	return u, nil
}

// UpsertGuideProfile replaces the guide's marketplace profile.
func (s *Service) UpsertGuideProfile(ctx context.Context, userID uuid.UUID, req dto.UpsertGuideProfileRequest) (*dto.MeResponse, error) {
	if _, err := s.loadGuide(ctx, userID); err != nil {
		return nil, err
	}
	if req.HourlyRate == nil && req.DailyRate == nil {
		return nil, validation.Field("hourly_rate", "required_without", "set hourly_rate or daily_rate")
	}
	lower := func(in []string) pq.StringArray {
		out := make(pq.StringArray, 0, len(in))
		for _, v := range in {
			out = append(out, strings.ToLower(strings.TrimSpace(v)))
		}
		return out
	}
	var license any
	if req.LicenseNumber != nil && strings.TrimSpace(*req.LicenseNumber) != "" {
		license = strings.TrimSpace(*req.LicenseNumber)
	}
	err := s.Repo.UpdateGuideProfile(ctx, userID, map[string]any{
		"headline":         strings.TrimSpace(req.Headline),
		"about":            strings.TrimSpace(req.About),
		"languages":        lower(req.Languages),
		"service_areas":    lower(req.ServiceAreas),
		"specialties":      lower(req.Specialties),
		"experience_years": req.ExperienceYears,
		"hourly_rate":      req.HourlyRate,
		"daily_rate":       req.DailyRate,
		"max_group_size":   req.MaxGroupSize,
		"license_number":   license,
	})
	if err != nil {
		return nil, err
	}
	return s.GetMe(ctx, userID)
}

func (s *Service) SetAvailability(ctx context.Context, userID uuid.UUID, available bool) (*dto.MeResponse, error) {
	if _, err := s.loadGuide(ctx, userID); err != nil {
		return nil, err
	}
	if err := s.Repo.UpdateGuideProfile(ctx, userID, map[string]any{"is_available": available}); err != nil {
		return nil, err
	}
	return s.GetMe(ctx, userID)
}

type KYCInput struct {
	NIDNumber string
	NIDFront  *multipart.FileHeader
	NIDBack   *multipart.FileHeader
	Selfie    *multipart.FileHeader
}

// SubmitVerification stores the guide's NID (encrypted) and document images
// (private Cloudinary assets) and puts the guide in the admin review queue.
func (s *Service) SubmitVerification(ctx context.Context, userID uuid.UUID, in KYCInput, meta domain.RequestMeta) (*dto.VerificationStatusResponse, error) {
	if s.Storage == nil {
		return nil, domain.ErrStorageUnavailable
	}
	u, err := s.loadGuide(ctx, userID)
	if err != nil {
		return nil, err
	}
	switch u.GuideProfile.VerificationStatus {
	case domain.VerificationPending:
		return nil, domain.ErrVerificationPending
	case domain.VerificationApproved:
		return nil, domain.ErrAlreadyVerified
	}
	if !u.PhoneVerified() {
		return nil, validation.Field("phone", "verified", "a verified Bangladeshi phone number is required")
	}
	if missing := domain.ProfileMissingFields(u); len(missing) > 0 {
		return nil, domain.ErrGuideProfileIncomplete.WithDetails(map[string]any{"missing_fields": missing})
	}

	nid := strings.TrimSpace(in.NIDNumber)
	if l := len(nid); !(l == 10 || l == 13 || l == 17) || strings.Trim(nid, "0123456789") != "" {
		return nil, validation.Field("nid_number", "nid", "must be a 10, 13 or 17 digit NID number")
	}

	// Validate all images before uploading anything.
	files := []struct {
		name string
		fh   *multipart.FileHeader
	}{{"nid_front", in.NIDFront}, {"nid_back", in.NIDBack}, {"selfie", in.Selfie}}
	images := make([]*imageutil.Image, len(files))
	for i, f := range files {
		img, err := imageutil.Read(f.fh, s.cfg.MaxUploadBytes, 300)
		if err != nil {
			if ae, ok := apperror.As(err); ok {
				return nil, validation.Field(f.name, "image", ae.Message)
			}
			return nil, err
		}
		images[i] = img
	}

	encrypted, err := s.Cipher.Encrypt(nid)
	if err != nil {
		return nil, err
	}

	v := &domain.GuideVerification{
		ID: newID(), UserID: userID, NIDNumberEncrypted: encrypted, NIDLast4: nid[len(nid)-4:],
		Status: domain.VerificationPending, CreatedAt: now(), UpdatedAt: now(),
	}
	folder := "kyc/" + userID.String()
	var uploaded []string
	cleanup := func() {
		for _, pid := range uploaded {
			p := pid
			s.async("kyc_cleanup", func(ctx context.Context) error { return s.Storage.Delete(ctx, p, true) })
		}
	}
	ids := make([]string, len(files))
	for i, f := range files {
		up, err := s.Storage.UploadPrivate(ctx, images[i].Data, folder, v.ID.String()+"_"+f.name)
		if err != nil {
			slog.Error("kyc upload failed", "err", err)
			cleanup()
			return nil, domain.ErrStorageUnavailable
		}
		uploaded = append(uploaded, up.PublicID)
		ids[i] = up.PublicID
	}
	v.NIDFrontPublicID, v.NIDBackPublicID, v.SelfiePublicID = ids[0], ids[1], ids[2]

	err = s.Repo.Tx(ctx, func(r domain.Repository) error {
		if err := r.CreateVerification(ctx, v); err != nil {
			return err
		}
		return r.UpdateGuideProfile(ctx, userID, map[string]any{"verification_status": domain.VerificationPending})
	})
	if err != nil {
		cleanup()
		if errors.Is(err, domain.ErrDuplicate) {
			return nil, domain.ErrVerificationPending
		}
		return nil, err
	}
	s.audit(ctx, &userID, &userID, domain.EventKYCSubmitted, meta, map[string]any{"verification_id": v.ID})
	return toVerificationStatus(v), nil
}

func (s *Service) GetMyVerification(ctx context.Context, userID uuid.UUID) (*dto.VerificationStatusResponse, error) {
	v, err := s.Repo.FindLatestVerification(ctx, userID)
	if err != nil {
		return nil, notFound(err, domain.ErrVerificationNotFound)
	}
	return toVerificationStatus(v), nil
}

// ------------------------------------------------------------------ public discovery

func (s *Service) SearchGuides(ctx context.Context, q dto.GuideSearchQuery) (response.Page[dto.PublicGuideResponse], error) {
	page, limit := pageDefaults(q.Page, q.Limit, 20)
	users, total, err := s.Repo.SearchGuides(ctx, domain.GuideFilter{
		Area: strings.ToLower(q.Area), Language: q.Language, Specialty: strings.ToLower(q.Specialty),
		MinRate: q.MinRate, MaxRate: q.MaxRate, Query: q.Q, Sort: q.Sort, Page: page, Limit: limit,
	})
	if err != nil {
		return response.Page[dto.PublicGuideResponse]{}, err
	}
	items := make([]dto.PublicGuideResponse, 0, len(users))
	for i := range users {
		if users[i].Profile != nil && users[i].GuideProfile != nil {
			items = append(items, toPublicGuide(&users[i], false))
		}
	}
	return response.NewPage(items, page, limit, total), nil
}

func (s *Service) GetPublicGuide(ctx context.Context, id uuid.UUID) (*dto.PublicGuideResponse, error) {
	u, err := s.Repo.FindUserByID(ctx, id)
	if err != nil {
		return nil, notFound(err, domain.ErrGuideNotFound)
	}
	if u.Role != domain.RoleGuide || !u.IsActive() || u.GuideProfile == nil || u.Profile == nil ||
		u.GuideProfile.VerificationStatus != domain.VerificationApproved {
		return nil, domain.ErrGuideNotFound
	}
	r := toPublicGuide(u, true)
	return &r, nil
}

func (s *Service) GuideOptions() dto.GuideOptionsResponse {
	return dto.GuideOptionsResponse{
		Specialties: domain.GuideSpecialties,
		SortOptions: []string{"rating", "experience", "price_low", "price_high", "newest"},
		Currency:    "BDT",
	}
}
