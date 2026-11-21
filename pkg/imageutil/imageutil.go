// Package imageutil validates uploaded images by their real content, not by
// the client-supplied filename or Content-Type.
package imageutil

import (
	"bytes"
	"image"
	_ "image/jpeg" // register decoders
	_ "image/png"
	"io"
	"mime/multipart"
	"net/http"

	_ "golang.org/x/image/webp"

	"cholobangladesh-server/pkg/apperror"
)

var allowed = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
}

const maxPixels = 40_000_000 // decompression-bomb guard

var ErrInvalidImage = apperror.New(http.StatusUnprocessableEntity, "INVALID_IMAGE",
	"file must be a JPEG, PNG or WEBP image")

type Image struct {
	Data   []byte
	MIME   string
	Ext    string
	Width  int
	Height int
}

// Read loads and validates an uploaded image. minSide rejects tiny images (0 = no check).
func Read(fh *multipart.FileHeader, maxBytes int64, minSide int) (*Image, error) {
	if fh == nil {
		return nil, ErrInvalidImage.WithMessage("file is required")
	}
	if fh.Size > maxBytes {
		return nil, apperror.ErrPayloadLarge.WithMessage("image is larger than the allowed size")
	}
	f, err := fh.Open()
	if err != nil {
		return nil, ErrInvalidImage
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, ErrInvalidImage
	}
	if int64(len(data)) > maxBytes {
		return nil, apperror.ErrPayloadLarge.WithMessage("image is larger than the allowed size")
	}

	mime := http.DetectContentType(data)
	ext, ok := allowed[mime]
	if !ok {
		return nil, ErrInvalidImage
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, ErrInvalidImage.WithMessage("image is corrupted or unsupported")
	}
	if cfg.Width*cfg.Height > maxPixels {
		return nil, ErrInvalidImage.WithMessage("image resolution is too large")
	}
	if minSide > 0 && (cfg.Width < minSide || cfg.Height < minSide) {
		return nil, ErrInvalidImage.WithMessage("image resolution is too small")
	}
	return &Image{Data: data, MIME: mime, Ext: ext, Width: cfg.Width, Height: cfg.Height}, nil
}
