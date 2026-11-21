// Package storage uploads images to Cloudinary. KYC documents are uploaded
// with delivery type "authenticated" so they are never publicly reachable;
// admins get short-lived signed download URLs.
package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cloudinary/cloudinary-go/v2"
	"github.com/cloudinary/cloudinary-go/v2/api"
	"github.com/cloudinary/cloudinary-go/v2/api/uploader"
)

type Uploaded struct {
	URL      string
	PublicID string
}

type Cloudinary struct {
	cld    *cloudinary.Cloudinary
	folder string
}

func NewCloudinary(cloudinaryURL, folder string) (*Cloudinary, error) {
	if cloudinaryURL == "" {
		return nil, errors.New("CLOUDINARY_URL is empty")
	}
	cld, err := cloudinary.NewFromURL(cloudinaryURL)
	if err != nil {
		return nil, err
	}
	cld.Config.URL.Secure = true
	return &Cloudinary{cld: cld, folder: folder}, nil
}

func (c *Cloudinary) upload(ctx context.Context, data []byte, path, publicID string, private bool) (*Uploaded, error) {
	t := true
	f := false
	params := uploader.UploadParams{
		PublicID:       publicID,
		Folder:         c.folder + "/" + path,
		Overwrite:      &t,
		Invalidate:     &t,
		UniqueFilename: &f,
		AllowedFormats: api.CldAPIArray{"jpg", "jpeg", "png", "webp"},
	}
	if private {
		params.Type = api.Authenticated
	}
	res, err := c.cld.Upload.Upload(ctx, bytes.NewReader(data), params)
	if err != nil {
		return nil, fmt.Errorf("cloudinary upload: %w", err)
	}
	if res.Error.Message != "" {
		return nil, fmt.Errorf("cloudinary upload: %s", res.Error.Message)
	}
	return &Uploaded{URL: res.SecureURL, PublicID: res.PublicID}, nil
}

// UploadPublic is for avatars and other images shown to everyone.
func (c *Cloudinary) UploadPublic(ctx context.Context, data []byte, path, publicID string) (*Uploaded, error) {
	return c.upload(ctx, data, path, publicID, false)
}

// UploadPrivate is for identity documents.
func (c *Cloudinary) UploadPrivate(ctx context.Context, data []byte, path, publicID string) (*Uploaded, error) {
	return c.upload(ctx, data, path, publicID, true)
}

func (c *Cloudinary) Delete(ctx context.Context, publicID string, private bool) error {
	t := true
	params := uploader.DestroyParams{PublicID: publicID, Invalidate: &t}
	if private {
		params.Type = string(api.Authenticated)
	}
	_, err := c.cld.Upload.Destroy(ctx, params)
	return err
}

// SignedURL returns a download URL for a private asset that expires after ttl.
func (c *Cloudinary) SignedURL(publicID string, ttl time.Duration) (string, error) {
	exp := time.Now().Add(ttl)
	return c.cld.Upload.PrivateDownloadURL(uploader.PrivateDownloadURLParams{
		PublicID:     publicID,
		Format:       "jpg",
		DeliveryType: string(api.Authenticated),
		ExpiresAt:    &exp,
	})
}
