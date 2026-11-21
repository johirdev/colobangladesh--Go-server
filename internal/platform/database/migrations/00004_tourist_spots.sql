-- +goose Up
-- ---------------------------------------------------------------------------
-- tourist_spots: curated by admins/moderators. A spot can span several upazilas
-- (e.g. Sundarbans = Mongla + Shyamnagar), see tourist_spot_upazilas.
-- district_id / division_id are denormalised from the FIRST (primary) upazila for fast filtering.
-- ---------------------------------------------------------------------------
CREATE TABLE tourist_spots (
    id                 UUID PRIMARY KEY,
    slug               VARCHAR(120)  NOT NULL,
    title              VARCHAR(150)  NOT NULL,
    bn_title           VARCHAR(150),
    subtitle           VARCHAR(300),
    category           VARCHAR(30)   NOT NULL,
    location_name      VARCHAR(200)  NOT NULL,
    description        TEXT          NOT NULL DEFAULT '',
    highlights         TEXT[]        NOT NULL DEFAULT '{}',
    best_time_to_visit VARCHAR(120),
    google_map_url     VARCHAR(500),
    latitude           NUMERIC(9, 6),
    longitude          NUMERIC(9, 6),
    division_id        INTEGER       NOT NULL REFERENCES divisions (id) ON DELETE RESTRICT,
    district_id        INTEGER       NOT NULL REFERENCES districts (id) ON DELETE RESTRICT,
    cover_image_url    TEXT,
    source_url         VARCHAR(500),
    status             VARCHAR(16)   NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'archived')),
    is_featured        BOOLEAN       NOT NULL DEFAULT FALSE,
    rating_avg         NUMERIC(3, 2) NOT NULL DEFAULT 0,
    rating_count       INTEGER       NOT NULL DEFAULT 0,
    gig_count          INTEGER       NOT NULL DEFAULT 0,
    created_by         UUID REFERENCES users (id) ON DELETE SET NULL,
    updated_by         UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at         TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ   NOT NULL DEFAULT now(),
    deleted_at         TIMESTAMPTZ
);
CREATE UNIQUE INDEX tourist_spots_slug_uq ON tourist_spots (slug) WHERE deleted_at IS NULL;
CREATE INDEX tourist_spots_listing_idx ON tourist_spots (status, is_featured DESC, rating_avg DESC) WHERE deleted_at IS NULL;
CREATE INDEX tourist_spots_district_idx ON tourist_spots (district_id) WHERE deleted_at IS NULL;
CREATE INDEX tourist_spots_division_idx ON tourist_spots (division_id) WHERE deleted_at IS NULL;
CREATE INDEX tourist_spots_category_idx ON tourist_spots (category) WHERE deleted_at IS NULL;

CREATE TABLE tourist_spot_upazilas (
    spot_id    UUID     NOT NULL REFERENCES tourist_spots (id) ON DELETE CASCADE,
    upazila_id INTEGER  NOT NULL REFERENCES upazilas (id) ON DELETE RESTRICT,
    position   SMALLINT NOT NULL DEFAULT 0,
    PRIMARY KEY (spot_id, upazila_id)
);
CREATE INDEX tourist_spot_upazilas_upazila_idx ON tourist_spot_upazilas (upazila_id);

CREATE TABLE tourist_spot_images (
    id         UUID PRIMARY KEY,
    spot_id    UUID         NOT NULL REFERENCES tourist_spots (id) ON DELETE CASCADE,
    url        TEXT         NOT NULL,
    public_id  VARCHAR(255), -- set when uploaded to our Cloudinary (deleted with the image)
    caption    VARCHAR(300),
    source_url VARCHAR(500), -- attribution (e.g. Wikimedia Commons file page)
    position   SMALLINT     NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX tourist_spot_images_spot_idx ON tourist_spot_images (spot_id, position);

CREATE TABLE tourist_spot_videos (
    id         UUID PRIMARY KEY,
    spot_id    UUID         NOT NULL REFERENCES tourist_spots (id) ON DELETE CASCADE,
    youtube_id VARCHAR(20)  NOT NULL,
    url        VARCHAR(300) NOT NULL,
    title      VARCHAR(300),
    position   SMALLINT     NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX tourist_spot_videos_spot_idx ON tourist_spot_videos (spot_id, position);

-- +goose Down
DROP TABLE IF EXISTS tourist_spot_videos;
DROP TABLE IF EXISTS tourist_spot_images;
DROP TABLE IF EXISTS tourist_spot_upazilas;
DROP TABLE IF EXISTS tourist_spots;
