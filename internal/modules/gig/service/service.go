// Package service implements the gig use cases: verified guides create up to
// FREE_GIG_LIMIT (+ extra slots) gigs, tourists browse bookable gigs, admins moderate.
package service

import (
	"context"
	"errors"
	"strings"

	"cholobangladesh-server/internal/modules/gig/domain"
	"cholobangladesh-server/internal/modules/gig/dto"
	"cholobangladesh-server/internal/shared/web"
	"cholobangladesh-server/pkg/listing"
	"cholobangladesh-server/pkg/response"

	"github.com/google/uuid"
)

type Service struct {
	repo      domain.Repository
	files     domain.FileStorage // nil when Cloudinary is not configured
	maxBytes  int64
	freeLimit int
}

func New(repo domain.Repository, files domain.FileStorage, maxUploadBytes int64, freeLimit int) *Service {
	return &Service{repo: repo, files: files, maxBytes: maxUploadBytes, freeLimit: freeLimit}
}

func gigNotFound(err error) error { return web.As(err, web.ErrNotFound, domain.ErrGigNotFound) }

// ================================================================== guide

func (s *Service) requireVerified(ctx context.Context, guideID uuid.UUID) error {
	e, err := s.repo.Eligibility(ctx, guideID)
	if err != nil {
		return err
	}
	if e.Role != "guide" || e.Status != "active" || e.VerificationStatus != "approved" {
		return domain.ErrGuideNotVerifed
	}
	return nil
}

func (s *Service) Quota(ctx context.Context, guideID uuid.UUID) (*dto.QuotaResponse, error) {
	q, err := s.repo.Quota(ctx, guideID)
	if err != nil {
		return nil, err
	}
	used, err := s.repo.CountGigs(ctx, guideID)
	if err != nil {
		return nil, err
	}
	total := s.freeLimit + q.ExtraSlots
	rem := max(total-int(used), 0)
	return &dto.QuotaResponse{FreeLimit: s.freeLimit, ExtraSlots: q.ExtraSlots, TotalLimit: total, Used: int(used),
		Remaining: rem, CanCreate: rem > 0, UpgradeRequired: rem == 0}, nil
}

func (s *Service) MyGigs(ctx context.Context, guideID uuid.UUID) (*dto.MyGigsResponse, error) {
	q, err := s.Quota(ctx, guideID)
	if err != nil {
		return nil, err
	}
	list, err := s.repo.ListByGuide(ctx, guideID)
	if err != nil {
		return nil, err
	}
	out := &dto.MyGigsResponse{Quota: *q, Items: make([]dto.GigResponse, 0, len(list))}
	for i := range list {
		out.Items = append(out.Items, toGig(&list[i]))
	}
	return out, nil
}

func (s *Service) MyGig(ctx context.Context, guideID, id uuid.UUID) (*dto.GigResponse, error) {
	g, err := s.owned(ctx, guideID, id)
	if err != nil {
		return nil, err
	}
	r := toGig(g)
	return &r, nil
}

func (s *Service) Create(ctx context.Context, guideID uuid.UUID, req dto.CreateGigRequest) (*dto.GigResponse, error) {
	if err := s.requireVerified(ctx, guideID); err != nil {
		return nil, err
	}
	spotID := uuid.MustParse(req.SpotID)
	spot, err := s.repo.Spot(ctx, spotID)
	if err != nil || spot.Status != "published" {
		return nil, domain.ErrSpotNotFound
	}
	q, err := s.Quota(ctx, guideID)
	if err != nil {
		return nil, err
	}
	status := req.Status
	if status == "" {
		status = domain.StatusActive
	}
	g := &domain.Gig{
		ID: uuid.Must(uuid.NewV7()), GuideID: guideID, SpotID: spotID, Title: strings.TrimSpace(req.Title),
		Description: strings.TrimSpace(req.Description), Price: req.Price, Currency: "BDT", PricingType: req.PricingType,
		DurationHours: req.DurationHours, MaxGroupSize: req.MaxGroupSize, Languages: lower(req.Languages),
		Includes: web.CleanList(req.Includes), Excludes: web.CleanList(req.Excludes), AvailableDays: req.AvailableDays,
		MeetingPoint: trim(req.MeetingPoint), CancellationPolicy: trim(req.CancellationPolicy),
		Status: status,
	}
	if g.AvailableDays == nil {
		g.AvailableDays = []string{}
	}
	ok, err := s.repo.CreateWithinLimit(ctx, g, q.TotalLimit)
	if errors.Is(err, web.ErrDuplicate) {
		return nil, domain.ErrDuplicateGig
	}
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.ErrGigLimitReached.WithDetails(q)
	}
	_ = s.repo.RefreshSpotGigCount(ctx, spotID)
	return s.MyGig(ctx, guideID, g.ID)
}

func (s *Service) Update(ctx context.Context, guideID, id uuid.UUID, req dto.UpdateGigRequest) (*dto.GigResponse, error) {
	g, err := s.owned(ctx, guideID, id)
	if err != nil {
		return nil, err
	}
	if g.Status == domain.StatusSuspended {
		return nil, domain.ErrGigSuspended
	}
	f := map[string]any{}
	if req.Title != nil {
		f["title"] = strings.TrimSpace(*req.Title)
	}
	if req.Description != nil {
		f["description"] = strings.TrimSpace(*req.Description)
	}
	if req.Price != nil {
		f["price"] = *req.Price
	}
	if req.PricingType != nil {
		f["pricing_type"] = *req.PricingType
	}
	if req.DurationHours != nil {
		f["duration_hours"] = *req.DurationHours
	}
	if req.MaxGroupSize != nil {
		f["max_group_size"] = *req.MaxGroupSize
	}
	if req.Languages != nil {
		f["languages"] = lower(*req.Languages)
	}
	if req.Includes != nil {
		f["includes"] = web.CleanList(*req.Includes)
	}
	if req.Excludes != nil {
		f["excludes"] = web.CleanList(*req.Excludes)
	}
	if req.AvailableDays != nil {
		f["available_days"] = *req.AvailableDays
	}
	for key, v := range map[string]*string{"meeting_point": req.MeetingPoint, "cancellation_policy": req.CancellationPolicy} {
		if v != nil {
			f[key] = trim(v)
		}
	}
	if req.Status != nil {
		if *req.Status == domain.StatusActive {
			if err := s.requireVerified(ctx, guideID); err != nil {
				return nil, err
			}
		}
		f["status"] = *req.Status
	}
	if len(f) > 0 {
		if err := s.repo.Update(ctx, id, f); err != nil {
			return nil, gigNotFound(err)
		}
	}
	_ = s.repo.RefreshSpotGigCount(ctx, g.SpotID)
	return s.MyGig(ctx, guideID, id)
}

// Delete frees the slot. Bookings keep a reference to the (soft-deleted) gig.
func (s *Service) Delete(ctx context.Context, guideID, id uuid.UUID) error {
	g, err := s.owned(ctx, guideID, id)
	if err != nil {
		return err
	}
	if err := s.repo.SoftDelete(ctx, id); err != nil {
		return gigNotFound(err)
	}
	_ = s.repo.RefreshSpotGigCount(ctx, g.SpotID)
	return nil
}

func (s *Service) owned(ctx context.Context, guideID, id uuid.UUID) (*domain.Gig, error) {
	g, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, gigNotFound(err)
	}
	if g.GuideID != guideID {
		return nil, domain.ErrGigNotFound // never reveal other guides' gigs
	}
	return g, nil
}

// ================================================================== public

func (s *Service) List(ctx context.Context, q dto.GigQuery) (response.Page[dto.GigResponse], error) {
	res, err := s.repo.ListPublic(ctx, filter(q, ""))
	if err != nil {
		return response.Page[dto.GigResponse]{}, err
	}
	return listing.Map(res, toGig), nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (*dto.GigResponse, error) {
	g, err := s.repo.GetPublic(ctx, id)
	if err != nil {
		return nil, gigNotFound(err)
	}
	r := toGig(g)
	return &r, nil
}

// ================================================================== admin

func (s *Service) AdminList(ctx context.Context, q dto.AdminGigQuery) (response.Page[dto.GigResponse], error) {
	res, err := s.repo.ListAll(ctx, filter(q.GigQuery, q.Status))
	if err != nil {
		return response.Page[dto.GigResponse]{}, err
	}
	return listing.Map(res, toGig), nil
}

func (s *Service) AdminSetStatus(ctx context.Context, id uuid.UUID, req dto.AdminGigStatusRequest) (*dto.GigResponse, error) {
	g, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, gigNotFound(err)
	}
	f := map[string]any{"status": req.Status, "suspended_reason": nil}
	if req.Status == domain.StatusSuspended {
		f["suspended_reason"] = strings.TrimSpace(req.Reason)
	}
	if err := s.repo.Update(ctx, id, f); err != nil {
		return nil, gigNotFound(err)
	}
	_ = s.repo.RefreshSpotGigCount(ctx, g.SpotID)
	g, err = s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	r := toGig(g)
	return &r, nil
}

func (s *Service) AdminSetQuota(ctx context.Context, adminID, guideID uuid.UUID, req dto.SetQuotaRequest) (*dto.QuotaResponse, error) {
	e, err := s.repo.Eligibility(ctx, guideID)
	if err != nil || e.Role != "guide" {
		return nil, domain.ErrGuideNotVerifed.WithMessage("guide not found")
	}
	if err := s.repo.SetQuota(ctx, &domain.Quota{GuideID: guideID, ExtraSlots: req.ExtraSlots,
		Note: trim(&req.Note), UpdatedBy: &adminID}); err != nil {
		return nil, err
	}
	return s.Quota(ctx, guideID)
}

// ================================================================== helpers

func filter(q dto.GigQuery, status string) domain.Filter {
	f := domain.Filter{DistrictID: q.DistrictID, DivisionID: q.DivisionID, Language: q.Language, MinPrice: q.MinPrice,
		MaxPrice: q.MaxPrice, Status: status, Params: q.Params}
	if id, err := uuid.Parse(q.SpotID); err == nil {
		f.SpotID = id
	}
	if id, err := uuid.Parse(q.GuideID); err == nil {
		f.GuideID = id
	}
	return f
}

func lower(in []string) []string {
	out := web.CleanList(in)
	for i := range out {
		out[i] = strings.ToLower(out[i])
	}
	return out
}

func trim(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return nil
	}
	return &v
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func toGig(g *domain.Gig) dto.GigResponse {
	r := dto.GigResponse{ID: g.ID.String(), Title: g.Title, Description: g.Description, Price: g.Price, Currency: g.Currency,
		PricingType: g.PricingType, DurationHours: g.DurationHours, MaxGroupSize: g.MaxGroupSize,
		Languages: nonNil(g.Languages), Includes: nonNil(g.Includes), Excludes: nonNil(g.Excludes),
		AvailableDays: nonNil(g.AvailableDays), MeetingPoint: g.MeetingPoint, CancellationPolicy: g.CancellationPolicy,
		CoverImageURL: g.CoverImageURL, Status: g.Status, SuspendedReason: g.SuspendedReason, RatingAvg: g.RatingAvg,
		RatingCount: g.RatingCount, BookingCount: g.BookingCount, CompletedCount: g.CompletedCount,
		CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt, Images: toImages(g.Images)}
	if sp := g.Spot; sp != nil {
		r.Spot = &dto.SpotRef{ID: sp.ID.String(), Slug: sp.Slug, Title: sp.Title, BnTitle: sp.BnTitle,
			CoverImageURL: sp.CoverImageURL, LocationName: sp.LocationName, DistrictID: sp.DistrictID,
			Published: sp.Status == "published" && !sp.DeletedAt.Valid}
		if sp.District != nil {
			r.Spot.DistrictName = sp.District.Name
		}
		if r.CoverImageURL == nil {
			r.CoverImageURL = sp.CoverImageURL
		}
	}
	gr := &dto.GuideRef{ID: g.GuideID.String(), Languages: []string{}}
	if p := g.Profile; p != nil {
		gr.FullName, gr.AvatarURL = p.FullName, p.AvatarURL
	}
	if gp := g.Guide; gp != nil {
		gr.Headline, gr.Languages, gr.ExperienceYears = gp.Headline, nonNil(gp.Languages), gp.ExperienceYears
		gr.Verified, gr.RatingAvg, gr.RatingCount, gr.TotalTours =
			gp.VerificationStatus == "approved", gp.RatingAvg, gp.RatingCount, gp.TotalTours
	}
	r.Guide = gr
	return r
}
