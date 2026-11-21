-- +goose Up
-- ---------------------------------------------------------------------------
-- Bangladesh administrative geography: division (বিভাগ) -> district (জেলা) -> upazila/thana (উপজেলা/থানা).
-- Integer IDs (stable, small, friendly in URLs). Reference data is seeded at startup.
-- Future modules (food, culture, hotels, transport) reference district_id / upazila_id.
-- ---------------------------------------------------------------------------
CREATE TABLE divisions (
    id         SERIAL PRIMARY KEY,
    name       VARCHAR(80)  NOT NULL,
    bn_name    VARCHAR(80)  NOT NULL,
    slug       VARCHAR(80)  NOT NULL,
    latitude   NUMERIC(9, 6),
    longitude  NUMERIC(9, 6),
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT divisions_slug_uq UNIQUE (slug)
);

CREATE TABLE districts (
    id          SERIAL PRIMARY KEY,
    division_id INTEGER      NOT NULL REFERENCES divisions (id) ON DELETE RESTRICT,
    name        VARCHAR(80)  NOT NULL,
    bn_name     VARCHAR(80)  NOT NULL,
    slug        VARCHAR(80)  NOT NULL,
    latitude    NUMERIC(9, 6),
    longitude   NUMERIC(9, 6),
    website     VARCHAR(255),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT districts_slug_uq UNIQUE (slug)
);
CREATE INDEX districts_division_idx ON districts (division_id);

CREATE TABLE upazilas (
    id          SERIAL PRIMARY KEY,
    district_id INTEGER      NOT NULL REFERENCES districts (id) ON DELETE RESTRICT,
    name        VARCHAR(100) NOT NULL,
    bn_name     VARCHAR(100) NOT NULL,
    slug        VARCHAR(100) NOT NULL,
    type        VARCHAR(16)  NOT NULL DEFAULT 'upazila' CHECK (type IN ('upazila', 'metro_thana')),
    latitude    NUMERIC(9, 6),
    longitude   NUMERIC(9, 6),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT upazilas_district_slug_uq UNIQUE (district_id, slug)
);
CREATE INDEX upazilas_district_idx ON upazilas (district_id);

-- +goose Down
DROP TABLE IF EXISTS upazilas;
DROP TABLE IF EXISTS districts;
DROP TABLE IF EXISTS divisions;
