package service

import (
	"context"
	"log/slog"
	"mime/multipart"
	"strings"

	"cholobangladesh-server/internal/modules/gig/domain"
	"cholobangladesh-server/internal/modules/gig/dto"
	"cholobangladesh-server/internal/shared/web"
	"cholobangladesh-server/pkg/imageutil"
	"cholobangladesh-server/pkg/validation"

	"github.com/google/uuid"
)

// Gallery use cases. Photos are cropped in the browser, stored in Cloudinary under
// gigs/<gig id>/<image id>; the first photo is the gig cover.

const minImageSide = 400 // px

func imageNotFound(err error) error { return web.As(err, web.ErrNotFound, domain.ErrImageNotFound) }

func (s *Service) upload(ctx context.Context, gigID uuid.UUID, fh *multipart.FileHeader) (*domain.Image, error) {
	img, err := imageutil.Read(fh, s.maxBytes, minImageSide)
	if err != nil {
		return nil, err
	}
	id := uuid.Must(uuid.NewV7())
	url, pid, err := s.files.UploadPublic(ctx, img.Data, "gigs/"+gigID.String(), id.String())
	if err != nil {
		slog.Error("gig image upload failed", "gig", gigID, "err", err)
		return nil, domain.ErrUploadsOff
	}
	return &domain.Image{ID: id, URL: url, PublicID: &pid}, nil
}

// UploadImages appends photos (multipart `images`, optional `captions` in the same order).
func (s *Service) UploadImages(ctx context.Context, guideID, gigID uuid.UUID, files []*multipart.FileHeader, captions []string) (*dto.GigResponse, error) {
	if s.files == nil {
		return nil, domain.ErrUploadsOff
	}
	if len(files) == 0 {
		return nil, validation.Field("images", "required", "attach at least one photo")
	}
	g, err := s.owned(ctx, guideID, gigID)
	if err != nil {
		return nil, err
	}
	if len(g.Images)+len(files) > domain.MaxImages {
		return nil, domain.ErrTooManyImages
	}
	imgs := make([]domain.Image, 0, len(files))
	var uploaded []string
	for i, fh := range files {
		im, err := s.upload(ctx, gigID, fh)
		if err != nil {
			s.deleteFiles(uploaded)
			return nil, err
		}
		uploaded = append(uploaded, *im.PublicID)
		if i < len(captions) {
			im.Caption = trim(&captions[i])
		}
		imgs = append(imgs, *im)
	}
	if err := s.repo.AddImages(ctx, gigID, imgs); err != nil {
		s.deleteFiles(uploaded)
		return nil, err
	}
	return s.MyGig(ctx, guideID, gigID)
}

// ReplaceImage swaps the file of one photo (e.g. after re-cropping); position and caption stay.
func (s *Service) ReplaceImage(ctx context.Context, guideID, gigID, imageID uuid.UUID, fh *multipart.FileHeader) (*dto.GigResponse, error) {
	if s.files == nil {
		return nil, domain.ErrUploadsOff
	}
	g, err := s.owned(ctx, guideID, gigID)
	if err != nil {
		return nil, err
	}
	old := findImage(g.Images, imageID)
	if old == nil {
		return nil, domain.ErrImageNotFound
	}
	im, err := s.upload(ctx, gigID, fh)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateImage(ctx, gigID, imageID, map[string]any{"url": im.URL, "public_id": *im.PublicID}); err != nil {
		s.deleteFiles([]string{*im.PublicID})
		return nil, imageNotFound(err)
	}
	if old.PublicID != nil {
		s.deleteFiles([]string{*old.PublicID})
	}
	return s.MyGig(ctx, guideID, gigID)
}

// UpdateImage edits the caption and/or makes the photo the cover.
func (s *Service) UpdateImage(ctx context.Context, guideID, gigID, imageID uuid.UUID, req dto.UpdateImageRequest) (*dto.GigResponse, error) {
	if _, err := s.owned(ctx, guideID, gigID); err != nil {
		return nil, err
	}
	if req.Caption != nil {
		if err := s.repo.UpdateImage(ctx, gigID, imageID, map[string]any{"caption": trim(req.Caption)}); err != nil {
			return nil, imageNotFound(err)
		}
	}
	if req.Cover {
		if err := s.repo.MakeCover(ctx, gigID, imageID); err != nil {
			return nil, imageNotFound(err)
		}
	}
	return s.MyGig(ctx, guideID, gigID)
}

func (s *Service) DeleteImage(ctx context.Context, guideID, gigID, imageID uuid.UUID) (*dto.GigResponse, error) {
	if _, err := s.owned(ctx, guideID, gigID); err != nil {
		return nil, err
	}
	img, err := s.repo.DeleteImage(ctx, gigID, imageID)
	if err != nil {
		return nil, imageNotFound(err)
	}
	if img.PublicID != nil {
		s.deleteFiles([]string{*img.PublicID})
	}
	return s.MyGig(ctx, guideID, gigID)
}

func findImage(list []domain.Image, id uuid.UUID) *domain.Image {
	for i := range list {
		if list[i].ID == id {
			return &list[i]
		}
	}
	return nil
}

// deleteFiles removes stored files in the background (best effort; never blocks the request).
func (s *Service) deleteFiles(ids []string) {
	if s.files == nil || len(ids) == 0 {
		return
	}
	go func() {
		for _, id := range ids {
			if err := s.files.Delete(context.Background(), id); err != nil {
				slog.Warn("gig image delete failed", "public_id", id, "err", err)
			}
		}
	}()
}

func toImages(list []domain.Image) []dto.ImageResponse {
	out := make([]dto.ImageResponse, 0, len(list))
	for i, im := range list {
		out = append(out, dto.ImageResponse{ID: im.ID.String(), URL: im.URL, Caption: im.Caption,
			Uploaded: im.PublicID != nil && strings.TrimSpace(*im.PublicID) != "", Cover: i == 0})
	}
	return out
}
