// Package service implements the geo module's use cases.
package service

import (
	"context"
	"errors"
	"strings"
	"sync"

	"cholobangladesh-server/internal/modules/geo/domain"
	"cholobangladesh-server/internal/modules/geo/dto"
	"cholobangladesh-server/internal/shared/web"
	"cholobangladesh-server/pkg/apperror"
	"cholobangladesh-server/pkg/listing"
	"cholobangladesh-server/pkg/response"
	"cholobangladesh-server/pkg/validation"
)

type Service struct {
	repo domain.Repository

	mu   sync.RWMutex
	tree []dto.TreeDivision // cached, reset on every write
}

func New(repo domain.Repository) *Service { return &Service{repo: repo} }

func (s *Service) invalidate() {
	s.mu.Lock()
	s.tree = nil
	s.mu.Unlock()
}

// mapErr converts repository sentinels into API errors.
func mapErr(err error, notFound *apperror.Error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, web.ErrNotFound):
		return notFound
	case errors.Is(err, web.ErrDuplicate):
		return domain.ErrSlugTaken
	case errors.Is(err, web.ErrInUse):
		return domain.ErrInUse
	}
	return err
}

func slugOr(slug, name string) (string, error) {
	if s := strings.TrimSpace(slug); s != "" {
		return s, nil
	}
	if s := web.Slugify(name); s != "" {
		return s, nil
	}
	return "", validation.Field("slug", "required", "is required when the name has no latin letters")
}

// ------------------------------------------------------------------ divisions

func (s *Service) ListDivisions(ctx context.Context) ([]dto.DivisionResponse, error) {
	list, err := s.repo.ListDivisions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]dto.DivisionResponse, 0, len(list))
	for i := range list {
		out = append(out, toDivision(&list[i]))
	}
	return out, nil
}

func (s *Service) GetDivision(ctx context.Context, id int) (*dto.DivisionResponse, error) {
	d, err := s.repo.GetDivision(ctx, id)
	if err != nil {
		return nil, mapErr(err, domain.ErrDivisionNotFound)
	}
	r := toDivision(d)
	return &r, nil
}

func (s *Service) CreateDivision(ctx context.Context, req dto.DivisionRequest) (*dto.DivisionResponse, error) {
	slug, err := slugOr(req.Slug, req.Name)
	if err != nil {
		return nil, err
	}
	d := &domain.Division{Name: strings.TrimSpace(req.Name), BnName: strings.TrimSpace(req.BnName), Slug: slug,
		Latitude: req.Latitude, Longitude: req.Longitude}
	if err := s.repo.CreateDivision(ctx, d); err != nil {
		return nil, mapErr(err, domain.ErrDivisionNotFound)
	}
	s.invalidate()
	return s.GetDivision(ctx, d.ID)
}

func (s *Service) UpdateDivision(ctx context.Context, id int, req dto.DivisionPatch) (*dto.DivisionResponse, error) {
	f := map[string]any{}
	setStr(f, "name", req.Name)
	setStr(f, "bn_name", req.BnName)
	setStr(f, "slug", req.Slug)
	setAny(f, "latitude", req.Latitude)
	setAny(f, "longitude", req.Longitude)
	if err := s.repo.UpdateDivision(ctx, id, f); err != nil {
		return nil, mapErr(err, domain.ErrDivisionNotFound)
	}
	s.invalidate()
	return s.GetDivision(ctx, id)
}

func (s *Service) DeleteDivision(ctx context.Context, id int) error {
	defer s.invalidate()
	return mapErr(s.repo.DeleteDivision(ctx, id), domain.ErrDivisionNotFound)
}

// ------------------------------------------------------------------ districts

func (s *Service) ListDistricts(ctx context.Context, q dto.DistrictQuery) ([]dto.DistrictResponse, error) {
	list, err := s.repo.ListDistricts(ctx, q.DivisionID, q.Q)
	if err != nil {
		return nil, err
	}
	out := make([]dto.DistrictResponse, 0, len(list))
	for i := range list {
		out = append(out, toDistrict(&list[i]))
	}
	return out, nil
}

// GetDistrict accepts a numeric ID or a slug ("coxs-bazar").
func (s *Service) GetDistrict(ctx context.Context, id int, slug string) (*dto.DistrictResponse, error) {
	var (
		d   *domain.District
		err error
	)
	if id > 0 {
		d, err = s.repo.GetDistrict(ctx, id)
	} else {
		d, err = s.repo.GetDistrictBySlug(ctx, slug)
	}
	if err != nil {
		return nil, mapErr(err, domain.ErrDistrictNotFound)
	}
	r := toDistrict(d)
	return &r, nil
}

func (s *Service) CreateDistrict(ctx context.Context, req dto.DistrictRequest) (*dto.DistrictResponse, error) {
	if _, err := s.repo.GetDivision(ctx, req.DivisionID); err != nil {
		return nil, mapErr(err, domain.ErrDivisionNotFound)
	}
	slug, err := slugOr(req.Slug, req.Name)
	if err != nil {
		return nil, err
	}
	d := &domain.District{DivisionID: req.DivisionID, Name: strings.TrimSpace(req.Name), BnName: strings.TrimSpace(req.BnName),
		Slug: slug, Latitude: req.Latitude, Longitude: req.Longitude, Website: req.Website}
	if err := s.repo.CreateDistrict(ctx, d); err != nil {
		return nil, mapErr(err, domain.ErrDistrictNotFound)
	}
	s.invalidate()
	return s.GetDistrict(ctx, d.ID, "")
}

func (s *Service) UpdateDistrict(ctx context.Context, id int, req dto.DistrictPatch) (*dto.DistrictResponse, error) {
	f := map[string]any{}
	if req.DivisionID != nil {
		if _, err := s.repo.GetDivision(ctx, *req.DivisionID); err != nil {
			return nil, mapErr(err, domain.ErrDivisionNotFound)
		}
		f["division_id"] = *req.DivisionID
	}
	setStr(f, "name", req.Name)
	setStr(f, "bn_name", req.BnName)
	setStr(f, "slug", req.Slug)
	setStr(f, "website", req.Website)
	setAny(f, "latitude", req.Latitude)
	setAny(f, "longitude", req.Longitude)
	if err := s.repo.UpdateDistrict(ctx, id, f); err != nil {
		return nil, mapErr(err, domain.ErrDistrictNotFound)
	}
	s.invalidate()
	return s.GetDistrict(ctx, id, "")
}

func (s *Service) DeleteDistrict(ctx context.Context, id int) error {
	defer s.invalidate()
	return mapErr(s.repo.DeleteDistrict(ctx, id), domain.ErrDistrictNotFound)
}

// ------------------------------------------------------------------ upazilas

func (s *Service) ListUpazilas(ctx context.Context, q dto.UpazilaQuery) (response.Page[dto.UpazilaResponse], error) {
	res, err := s.repo.ListUpazilas(ctx, domain.UpazilaFilter{DistrictID: q.DistrictID, DivisionID: q.DivisionID,
		Type: q.Type, Params: q.Params})
	if err != nil {
		return response.Page[dto.UpazilaResponse]{}, err
	}
	return listing.Map(res, toUpazila), nil
}

func (s *Service) GetUpazila(ctx context.Context, id int) (*dto.UpazilaResponse, error) {
	u, err := s.repo.GetUpazila(ctx, id)
	if err != nil {
		return nil, mapErr(err, domain.ErrUpazilaNotFound)
	}
	r := toUpazila(u)
	return &r, nil
}

func (s *Service) CreateUpazila(ctx context.Context, req dto.UpazilaRequest) (*dto.UpazilaResponse, error) {
	if _, err := s.repo.GetDistrict(ctx, req.DistrictID); err != nil {
		return nil, mapErr(err, domain.ErrDistrictNotFound)
	}
	slug, err := slugOr(req.Slug, req.Name)
	if err != nil {
		return nil, err
	}
	typ := req.Type
	if typ == "" {
		typ = domain.TypeUpazila
	}
	u := &domain.Upazila{DistrictID: req.DistrictID, Name: strings.TrimSpace(req.Name), BnName: strings.TrimSpace(req.BnName),
		Slug: slug, Type: typ, Latitude: req.Latitude, Longitude: req.Longitude}
	if err := s.repo.CreateUpazila(ctx, u); err != nil {
		return nil, mapErr(err, domain.ErrUpazilaNotFound)
	}
	s.invalidate()
	return s.GetUpazila(ctx, u.ID)
}

func (s *Service) UpdateUpazila(ctx context.Context, id int, req dto.UpazilaPatch) (*dto.UpazilaResponse, error) {
	f := map[string]any{}
	if req.DistrictID != nil {
		if _, err := s.repo.GetDistrict(ctx, *req.DistrictID); err != nil {
			return nil, mapErr(err, domain.ErrDistrictNotFound)
		}
		f["district_id"] = *req.DistrictID
	}
	setStr(f, "name", req.Name)
	setStr(f, "bn_name", req.BnName)
	setStr(f, "slug", req.Slug)
	setStr(f, "type", req.Type)
	setAny(f, "latitude", req.Latitude)
	setAny(f, "longitude", req.Longitude)
	if err := s.repo.UpdateUpazila(ctx, id, f); err != nil {
		return nil, mapErr(err, domain.ErrUpazilaNotFound)
	}
	s.invalidate()
	return s.GetUpazila(ctx, id)
}

func (s *Service) DeleteUpazila(ctx context.Context, id int) error {
	defer s.invalidate()
	return mapErr(s.repo.DeleteUpazila(ctx, id), domain.ErrUpazilaNotFound)
}

// ------------------------------------------------------------------ tree

// Tree returns division -> district -> upazila in one response (cached in memory).
func (s *Service) Tree(ctx context.Context) ([]dto.TreeDivision, error) {
	s.mu.RLock()
	t := s.tree
	s.mu.RUnlock()
	if t != nil {
		return t, nil
	}

	divs, err := s.repo.ListDivisions(ctx)
	if err != nil {
		return nil, err
	}
	dists, err := s.repo.ListDistricts(ctx, 0, "")
	if err != nil {
		return nil, err
	}
	ups, err := s.repo.AllUpazilas(ctx)
	if err != nil {
		return nil, err
	}
	byDistrict := map[int][]dto.TreeUpazila{}
	for _, u := range ups {
		byDistrict[u.DistrictID] = append(byDistrict[u.DistrictID],
			dto.TreeUpazila{ID: u.ID, Name: u.Name, BnName: u.BnName, Slug: u.Slug, Type: u.Type})
	}
	byDivision := map[int][]dto.TreeDistrict{}
	for _, d := range dists {
		up := byDistrict[d.ID]
		if up == nil {
			up = []dto.TreeUpazila{}
		}
		byDivision[d.DivisionID] = append(byDivision[d.DivisionID],
			dto.TreeDistrict{ID: d.ID, Name: d.Name, BnName: d.BnName, Slug: d.Slug, Upazilas: up})
	}
	t = make([]dto.TreeDivision, 0, len(divs))
	for _, d := range divs {
		ds := byDivision[d.ID]
		if ds == nil {
			ds = []dto.TreeDistrict{}
		}
		t = append(t, dto.TreeDivision{ID: d.ID, Name: d.Name, BnName: d.BnName, Slug: d.Slug, Districts: ds})
	}
	s.mu.Lock()
	s.tree = t
	s.mu.Unlock()
	return t, nil
}

// ------------------------------------------------------------------ mapping

func setStr(f map[string]any, key string, v *string) {
	if v != nil {
		f[key] = strings.TrimSpace(*v)
	}
}

func setAny[T any](f map[string]any, key string, v *T) {
	if v != nil {
		f[key] = *v
	}
}

func toDivision(d *domain.Division) dto.DivisionResponse {
	return dto.DivisionResponse{ID: d.ID, Name: d.Name, BnName: d.BnName, Slug: d.Slug,
		Latitude: d.Latitude, Longitude: d.Longitude, DistrictCount: d.DistrictCount}
}

func toDistrict(d *domain.District) dto.DistrictResponse {
	r := dto.DistrictResponse{ID: d.ID, DivisionID: d.DivisionID, Name: d.Name, BnName: d.BnName, Slug: d.Slug,
		Latitude: d.Latitude, Longitude: d.Longitude, Website: d.Website, UpazilaCount: d.UpazilaCount, SpotCount: d.SpotCount}
	if d.Division != nil {
		r.Division = &dto.DivisionRef{ID: d.Division.ID, Name: d.Division.Name, BnName: d.Division.BnName, Slug: d.Division.Slug}
	}
	return r
}

func toUpazila(u *domain.Upazila) dto.UpazilaResponse {
	r := dto.UpazilaResponse{ID: u.ID, DistrictID: u.DistrictID, Name: u.Name, BnName: u.BnName, Slug: u.Slug,
		Type: u.Type, Latitude: u.Latitude, Longitude: u.Longitude}
	if d := u.District; d != nil {
		r.District = &dto.DistrictRef{ID: d.ID, DivisionID: d.DivisionID, Name: d.Name, BnName: d.BnName, Slug: d.Slug}
	}
	return r
}
