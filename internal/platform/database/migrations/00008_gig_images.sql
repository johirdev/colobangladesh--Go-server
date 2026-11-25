-- +goose Up
-- ---------------------------------------------------------------------------
-- Gig photo gallery: guides upload (cropped) photos of their tour; the first one is the
-- gig cover (guide_gigs.cover_image_url is kept in sync). Replaces the old "cover image URL" field.
-- ---------------------------------------------------------------------------
CREATE TABLE guide_gig_images (
    id         UUID PRIMARY KEY,
    gig_id     UUID          NOT NULL REFERENCES guide_gigs (id) ON DELETE CASCADE,
    url        VARCHAR(1000) NOT NULL,
    public_id  VARCHAR(300),            -- Cloudinary id (NULL for old external URLs)
    caption    VARCHAR(300),
    position   INT           NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX guide_gig_images_gig_idx ON guide_gig_images (gig_id, position, created_at);

-- Keep existing covers: an old cover URL becomes the first gallery photo.
INSERT INTO guide_gig_images (id, gig_id, url, position, created_at)
SELECT gen_random_uuid(), id, cover_image_url, 0, now()
FROM guide_gigs WHERE cover_image_url IS NOT NULL AND cover_image_url <> '';

-- +goose Down
DROP TABLE IF EXISTS guide_gig_images;
