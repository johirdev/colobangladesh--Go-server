// Package service implements reviews: one review (1-5 stars, comment, up to 5 photos)
// per completed booking, a guide reply, admin moderation and rating aggregation.
package service

import (
	"context"
	"errors"
	"log/slog"
	"mime/multipart"
	"strconv"
	"strings"
	"time"

	"cholobangladesh-server/internal/modules/review/domain"
	"cholobangladesh-server/internal/modules/review/dto"
	"cholobangladesh-server/internal/shared/web"
	"cholobangladesh-server/pkg/imageutil"
	"cholobangladesh-server/pkg/response"
	"cholobangladesh-server/pkg/validation"

	"github.com/google/uuid"
)

type Service struct {
	repo     domain.Repository
	storage  domain.FileStorage
	maxBytes int64
}

func New(repo domain.Repository, storage domain.FileStorage, maxBytes int64) *Service {
	return &Service{repo: repo, storage: storage, maxBytes: maxBytes}
}

type CreateInput struct {
	Rating  string
	Comment string
	Images  []*multipart.FileHeader
}

// Create adds the tourist's review for a completed booking.
func (s *Service) Create(ctx context.Context, userID, bookingID uuid.UUID, in CreateInput) (*dto.ReviewResponse, error) {
	rating, err := strconv.Atoi(strings.TrimSpace(in.Rating))
	if err != nil || rating < 1 || rating > 5 {
		return nil, validation.Field("rating", "range", "must be a number from 1 to 5")
	}
	comment := strings.TrimSpace(in.Comment)
	if len([]rune(comment)) > 2000 {
		return nil, validation.Field("comment", "max", "must be at most 2000 characters")
	}
	if len(in.Images) > domain.MaxImages {
		return nil, domain.ErrTooManyImages
	}
	b, err := s.repo.Booking(ctx, bookingID)
	if err != nil || b.UserID != userID {
		return nil, domain.ErrBookingNotFound
	}
	if b.Status != "completed" {
		return nil, domain.ErrNotCompleted
	}

	rv := &domain.Review{ID: uuid.Must(uuid.NewV7()), BookingID: b.ID, GigID: b.GigID, GuideID: b.GuideID, SpotID: b.SpotID,
		UserID: userID, Rating: rating, Comment: comment, IsVisible: true}

	var uploaded []string
	if len(in.Images) > 0 {
		if s.storage == nil {
			return nil, domain.ErrStorageUnavailable
		}
		for i, fh := range in.Images {
			img, err := imageutil.Read(fh, s.maxBytes, 200)
			if err != nil {
				s.cleanup(uploaded)
				return nil, err
			}
			id := uuid.Must(uuid.NewV7())
			url, pid, err := s.storage.UploadPublic(ctx, img.Data, "reviews/"+rv.ID.String(), id.String())
			if err != nil {
				s.cleanup(uploaded)
				slog.Error("review image upload failed", "err", err)
				return nil, domain.ErrStorageUnavailable
			}
			uploaded = append(uploaded, pid)
			rv.Images = append(rv.Images, domain.Image{ID: id, URL: url, PublicID: &pid, Position: i})
		}
	}
	if err := s.repo.Create(ctx, rv); err != nil {
		s.cleanup(uploaded)
		if errors.Is(err, web.ErrDuplicate) {
			return nil, domain.ErrAlreadyReviewed
		}
		return nil, err
	}
	return s.get(ctx, rv.ID)
}

func (s *Service) List(ctx context.Context, q dto.ReviewQuery, includeHidden bool) (*dto.ReviewList, error) {
	page, limit := web.Paging(q.Page, q.Limit, 10, 50)
	f := domain.Filter{WithPhotos: q.WithPhotos, IncludeHide: includeHidden, Sort: q.Sort, Page: page, Limit: limit}
	f.GigID, _ = uuid.Parse(q.GigID)
	f.GuideID, _ = uuid.Parse(q.GuideID)
	f.SpotID, _ = uuid.Parse(q.SpotID)

	list, total, err := s.repo.List(ctx, f)
	if err != nil {
		return nil, err
	}
	sum, err := s.repo.Summary(ctx, f)
	if err != nil {
		return nil, err
	}
	items := make([]dto.ReviewResponse, 0, len(list))
	for i := range list {
		items = append(items, toReview(&list[i]))
	}
	stars := map[string]int64{}
	for k, v := range sum.Stars {
		stars[strconv.Itoa(k)] = v
	}
	return &dto.ReviewList{Summary: dto.Summary{Average: sum.Average, Count: sum.Count, Stars: stars},
		Reviews: dto.ReviewPage{Items: items, Meta: response.NewPage(items, page, limit, total).Meta}}, nil
}

// Reply lets the reviewed guide answer once.
func (s *Service) Reply(ctx context.Context, guideID, id uuid.UUID, reply string) (*dto.ReviewResponse, error) {
	rv, err := s.repo.Get(ctx, id)
	if err != nil || rv.GuideID != guideID {
		return nil, domain.ErrReviewNotFound
	}
	if rv.GuideReply != nil {
		return nil, domain.ErrAlreadyReplied
	}
	if err := s.repo.Update(ctx, id, map[string]any{"guide_reply": strings.TrimSpace(reply), "replied_at": time.Now().UTC()}); err != nil {
		return nil, err
	}
	return s.get(ctx, id)
}

// SetVisibility hides/shows a review (admin) and recomputes ratings.
func (s *Service) SetVisibility(ctx context.Context, id uuid.UUID, visible bool) (*dto.ReviewResponse, error) {
	rv, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, web.As(err, web.ErrNotFound, domain.ErrReviewNotFound)
	}
	if err := s.repo.Update(ctx, id, map[string]any{"is_visible": visible}); err != nil {
		return nil, err
	}
	if err := s.repo.RefreshRatings(ctx, rv.GigID, rv.GuideID, rv.SpotID); err != nil {
		return nil, err
	}
	return s.get(ctx, id)
}

func (s *Service) get(ctx context.Context, id uuid.UUID) (*dto.ReviewResponse, error) {
	rv, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, web.As(err, web.ErrNotFound, domain.ErrReviewNotFound)
	}
	r := toReview(rv)
	return &r, nil
}

func (s *Service) cleanup(ids []string) {
	if s.storage == nil {
		return
	}
	for _, id := range ids {
		if err := s.storage.Delete(context.Background(), id); err != nil {
			slog.Warn("delete review image failed", "public_id", id, "err", err)
		}
	}
}

func toReview(rv *domain.Review) dto.ReviewResponse {
	r := dto.ReviewResponse{ID: rv.ID.String(), BookingID: rv.BookingID.String(), GigID: rv.GigID.String(),
		GuideID: rv.GuideID.String(), SpotID: rv.SpotID.String(), Rating: rv.Rating, Comment: rv.Comment,
		Images: make([]string, 0, len(rv.Images)), GuideReply: rv.GuideReply, RepliedAt: rv.RepliedAt,
		IsVisible: rv.IsVisible, CreatedAt: rv.CreatedAt, Author: dto.Author{ID: rv.UserID.String()}}
	for _, im := range rv.Images {
		r.Images = append(r.Images, im.URL)
	}
	if g := rv.Gig; g != nil {
		r.GigTitle = g.Title
	}
	if a := rv.Author; a != nil {
		r.Author.FullName, r.Author.AvatarURL, r.Author.CountryCode = shortName(a.FullName), a.AvatarURL, a.CountryCode
		if a.CountryCode != nil && len(*a.CountryCode) == 2 {
			u := "https://flagcdn.com/w40/" + strings.ToLower(*a.CountryCode) + ".png"
			r.Author.FlagURL = &u
		}
	}
	return r
}

// shortName shows "Emma W." publicly instead of the full surname.
func shortName(full string) string {
	parts := strings.Fields(full)
	if len(parts) < 2 {
		return full
	}
	last := []rune(parts[len(parts)-1])
	return strings.Join(parts[:len(parts)-1], " ") + " " + string(last[0]) + "."
}
