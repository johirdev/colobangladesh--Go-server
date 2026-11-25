// Package service implements the tourist spot use cases: public browsing, the map,
// staff CRUD (admin + moderator), photo uploads and AI/Google autofill.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime/multipart"
	"slices"
	"strings"

	"cholobangladesh-server/internal/modules/spot/domain"
	"cholobangladesh-server/internal/modules/spot/dto"
	"cholobangladesh-server/internal/platform/autofill"
	"cholobangladesh-server/internal/seed"
	"cholobangladesh-server/internal/shared/web"
	"cholobangladesh-server/pkg/apperror"
	"cholobangladesh-server/pkg/imageutil"
	"cholobangladesh-server/pkg/listing"
	"cholobangladesh-server/pkg/response"
	"cholobangladesh-server/pkg/validation"

	"github.com/google/uuid"
)

type Service struct {
	repo     domain.Repository
	storage  domain.FileStorage // nil when Cloudinary is not configured
	autofill *autofill.Client
	maxBytes int64
}

func New(repo domain.Repository, storage domain.FileStorage, af *autofill.Client, maxUploadBytes int64) *Service {
	return &Service{repo: repo, storage: storage, autofill: af, maxBytes: maxUploadBytes}
}

func newID() uuid.UUID { return uuid.Must(uuid.NewV7()) }

func notFound(err error) error { return web.As(err, web.ErrNotFound, domain.ErrSpotNotFound) }

// ================================================================== public

func (s *Service) List(ctx context.Context, q dto.SpotQuery, status string) (response.Page[dto.SpotCard], error) {
	return s.list(ctx, q, domain.Filter{Status: status})
}

// ManageList is the staff listing: any status, the "needs work" filter and photo/video counts.
func (s *Service) ManageList(ctx context.Context, q dto.ManageSpotQuery) (response.Page[dto.SpotCard], error) {
	return s.list(ctx, q.SpotQuery, domain.Filter{Status: q.Status, Missing: q.Missing, WithCounts: true})
}

func (s *Service) list(ctx context.Context, q dto.SpotQuery, f domain.Filter) (response.Page[dto.SpotCard], error) {
	f.Params, f.DivisionID, f.DistrictID, f.UpazilaID = q.Params, q.DivisionID, q.DistrictID, q.UpazilaID
	f.Category, f.Featured = q.Category, q.Featured
	res, err := s.repo.List(ctx, f)
	if err != nil {
		return response.Page[dto.SpotCard]{}, err
	}
	return listing.Map(res, func(sp *domain.Spot) dto.SpotCard {
		c := toCard(sp)
		if f.WithCounts {
			ic, vc := sp.ImageCount, sp.VideoCount
			c.ImageCount, c.VideoCount = &ic, &vc
		}
		return c
	}), nil
}

func (s *Service) Markers(ctx context.Context, q dto.MarkerQuery) ([]dto.MapMarker, error) {
	rows, err := s.repo.Markers(ctx, q.DivisionID, q.DistrictID)
	if err != nil {
		return nil, err
	}
	out := make([]dto.MapMarker, 0, len(rows))
	for _, m := range rows {
		out = append(out, dto.MapMarker{ID: m.ID.String(), Slug: m.Slug, Title: m.Title, BnTitle: m.BnTitle, Category: m.Category,
			CoverImageURL: m.CoverImageURL, RatingAvg: m.RatingAvg, RatingCount: m.RatingCount, GigCount: m.GigCount,
			Latitude: m.Latitude, Longitude: m.Longitude, DistrictID: m.DistrictID, DistrictSlug: m.DistrictSlug,
			DistrictName: m.DistrictName, DivisionID: m.DivisionID})
	}
	return out, nil
}

// Get accepts a UUID or a slug. Public callers only see published spots.
func (s *Service) Get(ctx context.Context, idOrSlug string, publishedOnly bool) (*dto.SpotDetail, error) {
	var (
		sp  *domain.Spot
		err error
	)
	if id, perr := uuid.Parse(idOrSlug); perr == nil {
		sp, err = s.repo.Get(ctx, id, publishedOnly)
	} else {
		sp, err = s.repo.GetBySlug(ctx, strings.ToLower(idOrSlug), publishedOnly)
	}
	if err != nil {
		return nil, notFound(err)
	}
	d := toDetail(sp)
	return &d, nil
}

func (s *Service) Options() dto.OptionsResponse {
	return dto.OptionsResponse{Categories: domain.Categories,
		Statuses:  []string{domain.StatusDraft, domain.StatusPublished, domain.StatusArchived},
		MaxImages: domain.MaxImages, MaxVideos: domain.MaxVideos}
}

// ================================================================== manage

func (s *Service) Create(ctx context.Context, actor uuid.UUID, req dto.CreateSpotRequest) (*dto.SpotDetail, error) {
	locs, err := s.resolve(ctx, req.UpazilaIDs)
	if err != nil {
		return nil, err
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = web.Slugify(req.Title)
	}
	if slug == "" {
		return nil, validation.Field("slug", "required", "is required when the title has no latin letters")
	}
	if taken, err := s.repo.SlugExists(ctx, slug, uuid.Nil); err != nil {
		return nil, err
	} else if taken {
		return nil, domain.ErrSlugTaken
	}
	imgs, err := s.images(ctx, uuid.Nil, req.Images)
	if err != nil {
		return nil, err
	}
	vids, err := videos(req.Videos)
	if err != nil {
		return nil, err
	}
	status := req.Status
	if status == "" {
		status = domain.StatusDraft
	}
	sp := &domain.Spot{
		ID: newID(), Slug: slug, Title: strings.TrimSpace(req.Title), BnTitle: trim(req.BnTitle), Subtitle: trim(req.Subtitle),
		Category: req.Category, LocationName: strings.TrimSpace(req.LocationName), Description: strings.TrimSpace(req.Description),
		Highlights: web.CleanList(req.Highlights), BestTimeToVisit: trim(req.BestTimeToVisit), Latitude: req.Latitude,
		Longitude: req.Longitude, DivisionID: locs[0].District.DivisionID, DistrictID: locs[0].DistrictID,
		SourceURL: trim(req.SourceURL), Status: status, IsFeatured: req.IsFeatured, CreatedBy: &actor, UpdatedBy: &actor,
	}
	sp.GoogleMapURL = mapURL(trim(req.GoogleMapURL), sp.Latitude, sp.Longitude, sp.Title, sp.LocationName)
	ids := req.UpazilaIDs
	if err := s.repo.Create(ctx, sp, domain.Children{UpazilaIDs: &ids, Images: &imgs, Videos: &vids}); err != nil {
		if errors.Is(err, web.ErrDuplicate) {
			return nil, domain.ErrSlugTaken
		}
		return nil, err
	}
	return s.Get(ctx, sp.ID.String(), false)
}

func (s *Service) Update(ctx context.Context, actor, id uuid.UUID, req dto.UpdateSpotRequest) (*dto.SpotDetail, error) {
	cur, err := s.repo.Get(ctx, id, false)
	if err != nil {
		return nil, notFound(err)
	}
	f := map[string]any{"updated_by": actor}
	var c domain.Children

	if req.Title != nil {
		f["title"] = strings.TrimSpace(*req.Title)
	}
	if req.Slug != nil {
		if taken, err := s.repo.SlugExists(ctx, *req.Slug, id); err != nil {
			return nil, err
		} else if taken {
			return nil, domain.ErrSlugTaken
		}
		f["slug"] = *req.Slug
	}
	for key, v := range map[string]*string{"bn_title": req.BnTitle, "subtitle": req.Subtitle, "best_time_to_visit": req.BestTimeToVisit,
		"source_url": req.SourceURL, "google_map_url": req.GoogleMapURL} {
		if v != nil {
			f[key] = trim(v) // "" clears the field
		}
	}
	if req.Category != nil {
		f["category"] = *req.Category
	}
	if req.LocationName != nil {
		f["location_name"] = strings.TrimSpace(*req.LocationName)
	}
	if req.Description != nil {
		f["description"] = strings.TrimSpace(*req.Description)
	}
	if req.Highlights != nil {
		f["highlights"] = web.CleanList(*req.Highlights)
	}
	if req.Latitude != nil {
		f["latitude"] = *req.Latitude
	}
	if req.Longitude != nil {
		f["longitude"] = *req.Longitude
	}
	if req.Status != nil {
		f["status"] = *req.Status
	}
	if req.IsFeatured != nil {
		f["is_featured"] = *req.IsFeatured
	}
	if req.UpazilaIDs != nil {
		locs, err := s.resolve(ctx, *req.UpazilaIDs)
		if err != nil {
			return nil, err
		}
		f["district_id"], f["division_id"] = locs[0].DistrictID, locs[0].District.DivisionID
		ids := *req.UpazilaIDs
		c.UpazilaIDs = &ids
	}
	// Coordinates changed but no explicit map link -> regenerate the Google Maps link.
	if (req.Latitude != nil || req.Longitude != nil) && req.GoogleMapURL == nil {
		lat, lng := pick(req.Latitude, cur.Latitude), pick(req.Longitude, cur.Longitude)
		f["google_map_url"] = mapURL(nil, lat, lng, cur.Title, cur.LocationName)
	}

	var orphaned []string // uploaded files removed from the gallery
	if req.Images != nil {
		imgs, err := s.images(ctx, id, *req.Images)
		if err != nil {
			return nil, err
		}
		keep := map[uuid.UUID]bool{}
		for _, im := range imgs {
			keep[im.ID] = true
		}
		for _, old := range cur.Images {
			if !keep[old.ID] && old.PublicID != nil {
				orphaned = append(orphaned, *old.PublicID)
			}
		}
		c.Images = &imgs
	}
	if req.Videos != nil {
		vids, err := videos(*req.Videos)
		if err != nil {
			return nil, err
		}
		c.Videos = &vids
	}
	if err := s.repo.Update(ctx, id, f, c); err != nil {
		if errors.Is(err, web.ErrDuplicate) {
			return nil, domain.ErrSlugTaken
		}
		return nil, notFound(err)
	}
	s.deleteFiles(orphaned)
	return s.Get(ctx, id.String(), false)
}

// Delete soft-deletes the spot (gigs keep pointing at it; it disappears from listings).
func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	sp, err := s.repo.Get(ctx, id, false)
	if err != nil {
		return notFound(err)
	}
	if sp.GigCount > 0 {
		return domain.ErrSpotHasGigs
	}
	return notFound(s.repo.SoftDelete(ctx, id))
}

// Stats powers the content studio header and the status tabs.
func (s *Service) Stats(ctx context.Context) (*dto.SpotStats, error) {
	st, err := s.repo.Stats(ctx)
	if err != nil {
		return nil, err
	}
	out := &dto.SpotStats{Total: st.Total, Published: st.Published, Draft: st.Draft, Archived: st.Archived,
		Featured: st.Featured, MissingImages: st.MissingImages, MissingCoordinates: st.MissingCoordinates,
		MissingVideos: st.MissingVideos, WithGigs: st.WithGigs, DistrictsCovered: st.DistrictsCovered,
		TotalDistricts: st.TotalDistricts, ByCategory: make([]dto.CategoryCount, 0, len(st.ByCategory))}
	for _, c := range st.ByCategory {
		out.ByCategory = append(out.ByCategory, dto.CategoryCount{Category: c.Category, Count: c.Count})
	}
	return out, nil
}

// bulkField is the column a bulk action sets, and its value.
type bulkField struct {
	col string
	val any
}

var bulkFields = map[string]bulkField{
	"publish":   {"status", domain.StatusPublished},
	"unpublish": {"status", domain.StatusDraft},
	"archive":   {"status", domain.StatusArchived},
	"feature":   {"is_featured", true},
	"unfeature": {"is_featured", false},
}

// Bulk applies one action to many spots. Spots that cannot take it are skipped; they never fail the batch.
func (s *Service) Bulk(ctx context.Context, actor uuid.UUID, req dto.BulkSpotRequest) (*dto.BulkSpotResponse, error) {
	ids := make([]uuid.UUID, 0, len(req.IDs))
	for _, raw := range req.IDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			return nil, validation.Field("ids", "uuid", "must be tourist spot IDs")
		}
		ids = append(ids, id)
	}
	briefs, err := s.repo.Briefs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[uuid.UUID]domain.Brief, len(briefs))
	for _, b := range briefs {
		byID[b.ID] = b
	}

	res := &dto.BulkSpotResponse{Action: req.Action, Requested: len(ids), Skipped: []dto.BulkSkipped{}}
	skip := func(id uuid.UUID, title string, e *apperror.Error) {
		res.Skipped = append(res.Skipped, dto.BulkSkipped{ID: id.String(), Title: title, Code: e.Code, Message: e.Message})
	}
	bf, isUpdate := bulkFields[req.Action]
	var todo []uuid.UUID
	for _, id := range ids {
		b, ok := byID[id]
		switch {
		case !ok:
			skip(id, "", domain.ErrSpotNotFound)
		case !isUpdate && b.GigCount > 0:
			skip(id, b.Title, domain.ErrSpotHasGigs)
		case isUpdate && bf.col == "status" && b.Status == bf.val:
			res.Unchanged++
		default:
			todo = append(todo, id)
		}
	}

	if isUpdate {
		res.Affected, err = s.repo.BulkUpdate(ctx, todo, map[string]any{bf.col: bf.val, "updated_by": actor})
	} else {
		res.Affected, err = s.repo.BulkSoftDelete(ctx, todo)
	}
	if err != nil {
		return nil, err
	}
	return res, nil
}

// UploadImages stores photos in Cloudinary and appends them to the gallery.
func (s *Service) UploadImages(ctx context.Context, id uuid.UUID, files []*multipart.FileHeader, captions []string) (*dto.SpotDetail, error) {
	if s.storage == nil {
		return nil, domain.ErrStorageUnavailable
	}
	if len(files) == 0 {
		return nil, validation.Field("images", "required", "attach at least one image")
	}
	existing, err := s.repo.Images(ctx, id)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.Get(ctx, id, false); err != nil {
		return nil, notFound(err)
	}
	if len(existing)+len(files) > domain.MaxImages {
		return nil, domain.ErrTooManyImages
	}
	imgs := make([]domain.Image, 0, len(files))
	var uploaded []string
	for i, fh := range files {
		img, err := imageutil.Read(fh, s.maxBytes, 400)
		if err != nil {
			s.deleteFiles(uploaded)
			return nil, err
		}
		imgID := newID()
		url, pid, err := s.storage.UploadPublic(ctx, img.Data, "spots/"+id.String(), imgID.String())
		if err != nil {
			s.deleteFiles(uploaded)
			slog.Error("spot image upload failed", "err", err)
			return nil, domain.ErrStorageUnavailable
		}
		uploaded = append(uploaded, pid)
		im := domain.Image{ID: imgID, URL: url, PublicID: &pid}
		if i < len(captions) {
			im.Caption = trim(&captions[i])
		}
		imgs = append(imgs, im)
	}
	if err := s.repo.AddImages(ctx, id, imgs); err != nil {
		s.deleteFiles(uploaded)
		return nil, err
	}
	return s.Get(ctx, id.String(), false)
}

func (s *Service) DeleteImage(ctx context.Context, spotID, imageID uuid.UUID) error {
	img, err := s.repo.DeleteImage(ctx, spotID, imageID)
	if err != nil {
		return web.As(err, web.ErrNotFound, domain.ErrImageNotFound)
	}
	if img.PublicID != nil {
		s.deleteFiles([]string{*img.PublicID})
	}
	return nil
}

// Autofill searches the web sources for a name and returns an unsaved draft.
func (s *Service) Autofill(ctx context.Context, req dto.AutofillRequest) (*dto.AutofillResponse, error) {
	res, err := s.autofill.Lookup(ctx, req.Name)
	switch {
	case errors.Is(err, autofill.ErrNotFound):
		return nil, domain.ErrAutofillNotFound
	case err != nil:
		return nil, domain.ErrAutofillFailed.Wrap(err)
	}

	draft := dto.CreateSpotRequest{Title: res.Title, Slug: web.Slugify(res.Title), Category: res.Category,
		LocationName: res.Location, Description: res.Description, Highlights: res.Highlights,
		Latitude: res.Latitude, Longitude: res.Longitude, Status: domain.StatusDraft, UpazilaIDs: []int{}}
	if !slices.Contains(domain.Categories, draft.Category) {
		draft.Category = guessCategory(res.Title + " " + res.Subtitle + " " + res.Description)
	}
	draft.BnTitle, draft.Subtitle, draft.BestTimeToVisit, draft.SourceURL =
		opt(res.BnTitle), opt(res.Subtitle), opt(res.BestTime), opt(res.SourceURL)
	for _, im := range res.Images {
		draft.Images = append(draft.Images, dto.ImageInput{URL: im.URL, Caption: opt(im.Caption), SourceURL: opt(im.SourceURL)})
	}
	for _, v := range res.Videos {
		draft.Videos = append(draft.Videos, dto.VideoInput{URL: v.URL, Title: opt(v.Title)})
	}

	out := &dto.AutofillResponse{Sources: res.Sources, Notes: res.Notes, Suggested: []dto.UpazilaRef{}}
	if res.Latitude != nil && res.Longitude != nil {
		draft.GoogleMapURL = mapURL(nil, res.Latitude, res.Longitude, "", "")
		near, err := s.repo.NearestUpazilas(ctx, *res.Latitude, *res.Longitude, req.DistrictID, 3)
		if err == nil && len(near) > 0 {
			for i := range near {
				out.Suggested = append(out.Suggested, toUpazila(&near[i]))
			}
			draft.UpazilaIDs = []int{near[0].ID}
			if draft.LocationName == "" && near[0].District != nil {
				draft.LocationName = near[0].Name + ", " + near[0].District.Name
			}
		}
	} else {
		draft.GoogleMapURL = mapURL(nil, nil, nil, res.Title, "Bangladesh")
	}
	if out.Sources == nil {
		out.Sources = []string{}
	}
	out.Draft = draft
	return out, nil
}

// ================================================================== helpers

func (s *Service) resolve(ctx context.Context, ids []int) ([]domain.UpazilaRef, error) {
	locs, err := s.repo.ResolveUpazilas(ctx, ids)
	if err != nil {
		return nil, web.As(err, web.ErrNotFound, domain.ErrUpazilaNotFound)
	}
	if len(locs) == 0 || locs[0].District == nil {
		return nil, domain.ErrUpazilaNotFound
	}
	return locs, nil
}

// images converts inputs to rows. Existing image IDs keep their uploaded file (public_id).
func (s *Service) images(ctx context.Context, spotID uuid.UUID, in []dto.ImageInput) ([]domain.Image, error) {
	if len(in) > domain.MaxImages {
		return nil, domain.ErrTooManyImages
	}
	known := map[uuid.UUID]domain.Image{}
	if spotID != uuid.Nil {
		cur, err := s.repo.Images(ctx, spotID)
		if err != nil {
			return nil, err
		}
		for _, im := range cur {
			known[im.ID] = im
		}
	}
	out := make([]domain.Image, 0, len(in))
	seen := map[uuid.UUID]bool{}
	for _, im := range in {
		row := domain.Image{ID: newID(), URL: strings.TrimSpace(im.URL), Caption: trim(im.Caption), SourceURL: trim(im.SourceURL)}
		if im.ID != "" {
			if id, err := uuid.Parse(im.ID); err == nil && !seen[id] {
				if old, ok := known[id]; ok {
					row.ID, row.PublicID = id, old.PublicID
				}
			}
		}
		seen[row.ID] = true
		out = append(out, row)
	}
	return out, nil
}

func videos(in []dto.VideoInput) ([]domain.Video, error) {
	out := make([]domain.Video, 0, len(in))
	for i, v := range in {
		id := seed.YouTubeID(v.URL)
		if id == "" {
			return nil, domain.ErrInvalidYouTube.WithDetails([]validation.FieldError{{Field: fmt.Sprintf("videos[%d].url", i),
				Rule: "youtube", Message: "must be a YouTube video link"}})
		}
		out = append(out, domain.Video{ID: newID(), YoutubeID: id, URL: "https://www.youtube.com/watch?v=" + id, Title: trim(v.Title)})
	}
	return out, nil
}

func (s *Service) deleteFiles(ids []string) {
	if s.storage == nil || len(ids) == 0 {
		return
	}
	go func() {
		for _, id := range ids {
			if err := s.storage.Delete(context.Background(), id); err != nil {
				slog.Warn("delete spot image failed", "public_id", id, "err", err)
			}
		}
	}()
}

func mapURL(explicit *string, lat, lng *float64, title, location string) *string {
	if explicit != nil && *explicit != "" {
		return explicit
	}
	var u string
	if lat != nil && lng != nil {
		u = fmt.Sprintf("https://www.google.com/maps/search/?api=1&query=%.6f,%.6f", *lat, *lng)
	} else if title != "" {
		u = "https://www.google.com/maps/search/?api=1&query=" + strings.ReplaceAll(strings.TrimSpace(title+" "+location), " ", "+")
	} else {
		return nil
	}
	return &u
}

var categoryWords = []struct{ word, cat string }{
	{"beach", "beach"}, {"island", "island"}, {"waterfall", "waterfall"}, {"fall", "waterfall"}, {"lake", "lake"},
	{"haor", "wetland"}, {"wetland", "wetland"}, {"swamp", "forest"}, {"forest", "forest"}, {"national park", "park"},
	{"hill", "hill"}, {"valley", "hill"}, {"mosque", "religious"}, {"temple", "religious"}, {"church", "religious"},
	{"vihara", "heritage"}, {"fort", "heritage"}, {"palace", "heritage"}, {"museum", "heritage"}, {"archaeolog", "heritage"},
	{"river", "river"}, {"park", "park"},
}

func guessCategory(text string) string {
	t := strings.ToLower(text)
	for _, w := range categoryWords {
		if strings.Contains(t, w.word) {
			return w.cat
		}
	}
	return "other"
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

func opt(s string) *string { return trim(&s) }

func pick(a, b *float64) *float64 {
	if a != nil {
		return a
	}
	return b
}

// ------------------------------------------------------------------ mapping

func toCard(s *domain.Spot) dto.SpotCard {
	c := dto.SpotCard{ID: s.ID.String(), Slug: s.Slug, Title: s.Title, BnTitle: s.BnTitle, Subtitle: s.Subtitle,
		Category: s.Category, LocationName: s.LocationName, CoverImageURL: s.CoverImageURL, Latitude: s.Latitude,
		Longitude: s.Longitude, RatingAvg: s.RatingAvg, RatingCount: s.RatingCount, GigCount: s.GigCount,
		IsFeatured: s.IsFeatured, Status: s.Status, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
	if d := s.Division; d != nil {
		c.Division = &dto.GeoRef{ID: d.ID, Name: d.Name, BnName: d.BnName, Slug: d.Slug}
	}
	if d := s.District; d != nil {
		c.District = &dto.GeoRef{ID: d.ID, Name: d.Name, BnName: d.BnName, Slug: d.Slug}
	}
	return c
}

func toUpazila(u *domain.UpazilaRef) dto.UpazilaRef {
	r := dto.UpazilaRef{ID: u.ID, Name: u.Name, BnName: u.BnName, Slug: u.Slug, Type: u.Type}
	if d := u.District; d != nil {
		r.District = &dto.GeoRef{ID: d.ID, Name: d.Name, BnName: d.BnName, Slug: d.Slug}
	}
	return r
}

func toDetail(s *domain.Spot) dto.SpotDetail {
	d := dto.SpotDetail{SpotCard: toCard(s), Description: s.Description, Highlights: s.Highlights,
		BestTimeToVisit: s.BestTimeToVisit, GoogleMapURL: s.GoogleMapURL, SourceURL: s.SourceURL,
		Upazilas: []dto.UpazilaRef{}, Images: []dto.ImageResponse{}, Videos: []dto.VideoResponse{}}
	if d.Highlights == nil {
		d.Highlights = []string{}
	}
	for _, su := range s.Upazilas {
		if su.Upazila != nil {
			d.Upazilas = append(d.Upazilas, toUpazila(su.Upazila))
		}
	}
	for _, im := range s.Images {
		d.Images = append(d.Images, dto.ImageResponse{ID: im.ID.String(), URL: im.URL, Caption: im.Caption,
			SourceURL: im.SourceURL, Uploaded: im.PublicID != nil})
	}
	for _, v := range s.Videos {
		d.Videos = append(d.Videos, dto.VideoResponse{ID: v.ID.String(), YoutubeID: v.YoutubeID, URL: v.URL,
			EmbedURL: "https://www.youtube.com/embed/" + v.YoutubeID, ThumbnailURL: "https://i.ytimg.com/vi/" + v.YoutubeID + "/hqdefault.jpg",
			Title: v.Title})
	}
	return d
}
