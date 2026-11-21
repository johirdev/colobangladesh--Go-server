-- +goose Up
-- ---------------------------------------------------------------------------
-- guide_gigs: a verified guide's tour offer for ONE tourist spot (Fiverr-style gig).
-- A guide may have FREE_GIG_LIMIT gigs (default 5) + guide_gig_quotas.extra_slots (paid later).
-- ---------------------------------------------------------------------------
CREATE TABLE guide_gigs (
    id                  UUID PRIMARY KEY,
    guide_id            UUID          NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    spot_id             UUID          NOT NULL REFERENCES tourist_spots (id) ON DELETE RESTRICT,
    title               VARCHAR(150)  NOT NULL,
    description         TEXT          NOT NULL,
    price               INTEGER       NOT NULL CHECK (price >= 0),
    currency            CHAR(3)       NOT NULL DEFAULT 'BDT',
    pricing_type        VARCHAR(16)   NOT NULL DEFAULT 'per_group' CHECK (pricing_type IN ('per_person', 'per_group')),
    duration_hours      NUMERIC(5, 1) NOT NULL CHECK (duration_hours > 0),
    max_group_size      SMALLINT      NOT NULL DEFAULT 10 CHECK (max_group_size BETWEEN 1 AND 100),
    languages           TEXT[]        NOT NULL DEFAULT '{}',
    includes            TEXT[]        NOT NULL DEFAULT '{}',
    excludes            TEXT[]        NOT NULL DEFAULT '{}',
    available_days      TEXT[]        NOT NULL DEFAULT '{}',
    meeting_point       VARCHAR(300),
    cancellation_policy VARCHAR(1000),
    cover_image_url     TEXT,
    status              VARCHAR(16)   NOT NULL DEFAULT 'active' CHECK (status IN ('draft', 'active', 'paused', 'suspended')),
    suspended_reason    VARCHAR(500),
    rating_avg          NUMERIC(3, 2) NOT NULL DEFAULT 0,
    rating_count        INTEGER       NOT NULL DEFAULT 0,
    booking_count       INTEGER       NOT NULL DEFAULT 0,
    completed_count     INTEGER       NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMPTZ
);
CREATE UNIQUE INDEX guide_gigs_guide_spot_uq ON guide_gigs (guide_id, spot_id) WHERE deleted_at IS NULL;
CREATE INDEX guide_gigs_guide_idx ON guide_gigs (guide_id) WHERE deleted_at IS NULL;
CREATE INDEX guide_gigs_spot_listing_idx ON guide_gigs (spot_id, status, rating_avg DESC) WHERE deleted_at IS NULL;

CREATE TABLE guide_gig_quotas (
    guide_id    UUID PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    extra_slots INTEGER     NOT NULL DEFAULT 0 CHECK (extra_slots >= 0),
    note        VARCHAR(500),
    updated_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- bookings: tourist books a gig -> guide accepts -> tour happens -> guide completes
-- -> tourist pays (mobile banking number, verified by admin) -> tourist reviews.
-- Prices are snapshotted so later gig edits never change an existing booking.
-- ---------------------------------------------------------------------------
CREATE TABLE bookings (
    id             UUID PRIMARY KEY,
    code           VARCHAR(16)  NOT NULL,
    gig_id         UUID         NOT NULL REFERENCES guide_gigs (id) ON DELETE RESTRICT,
    guide_id       UUID         NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    user_id        UUID         NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    spot_id        UUID         NOT NULL REFERENCES tourist_spots (id) ON DELETE RESTRICT,
    tour_date      DATE         NOT NULL,
    start_time     VARCHAR(5),
    group_size     SMALLINT     NOT NULL CHECK (group_size BETWEEN 1 AND 100),
    pricing_type   VARCHAR(16)  NOT NULL,
    unit_price     INTEGER      NOT NULL CHECK (unit_price >= 0),
    total_amount   INTEGER      NOT NULL CHECK (total_amount >= 0),
    currency       CHAR(3)      NOT NULL DEFAULT 'BDT',
    status         VARCHAR(16)  NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'accepted', 'rejected', 'cancelled', 'completed')),
    payment_status VARCHAR(16)  NOT NULL DEFAULT 'unpaid'
        CHECK (payment_status IN ('unpaid', 'submitted', 'paid', 'refunded')),
    note           VARCHAR(1000),
    contact_phone  VARCHAR(20),
    status_reason  VARCHAR(500),
    accepted_at    TIMESTAMPTZ,
    completed_at   TIMESTAMPTZ,
    cancelled_at   TIMESTAMPTZ,
    cancelled_by   UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT bookings_code_uq UNIQUE (code)
);
CREATE INDEX bookings_user_idx ON bookings (user_id, created_at DESC);
CREATE INDEX bookings_guide_idx ON bookings (guide_id, status, tour_date);
CREATE INDEX bookings_gig_idx ON bookings (gig_id);

-- ---------------------------------------------------------------------------
-- payment_methods: receiving accounts shown to payers (bKash / Nagad / Rocket ... numbers).
-- payments: manual "send money + transaction ID" submissions, verified by an admin.
-- purpose = 'booking' now; 'gig_slot' is reserved for buying extra gig slots later.
-- Online gateways (SSLCommerz, bKash PGW) can be added later as new providers.
-- ---------------------------------------------------------------------------
CREATE TABLE payment_methods (
    id             UUID PRIMARY KEY,
    provider       VARCHAR(20)  NOT NULL CHECK (provider IN ('bkash', 'nagad', 'rocket', 'upay', 'bank')),
    account_name   VARCHAR(100) NOT NULL,
    account_number VARCHAR(40)  NOT NULL,
    account_type   VARCHAR(16)  NOT NULL DEFAULT 'personal' CHECK (account_type IN ('personal', 'agent', 'merchant', 'bank')),
    instructions   VARCHAR(1000),
    is_active      BOOLEAN      NOT NULL DEFAULT TRUE,
    position       SMALLINT     NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT now()
);

CREATE TABLE payments (
    id             UUID PRIMARY KEY,
    purpose        VARCHAR(16)  NOT NULL DEFAULT 'booking' CHECK (purpose IN ('booking', 'gig_slot')),
    booking_id     UUID REFERENCES bookings (id) ON DELETE RESTRICT,
    payer_id       UUID         NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    method_id      UUID REFERENCES payment_methods (id) ON DELETE SET NULL,
    provider       VARCHAR(20)  NOT NULL,
    sender_number  VARCHAR(20)  NOT NULL,
    transaction_id VARCHAR(50)  NOT NULL,
    amount         INTEGER      NOT NULL CHECK (amount > 0),
    currency       CHAR(3)      NOT NULL DEFAULT 'BDT',
    status         VARCHAR(16)  NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'verified', 'rejected')),
    admin_note     VARCHAR(500),
    reviewed_by    UUID REFERENCES users (id) ON DELETE SET NULL,
    reviewed_at    TIMESTAMPTZ,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ  NOT NULL DEFAULT now()
);
-- A transaction ID can be claimed only once per provider (stops re-using one payment twice).
CREATE UNIQUE INDEX payments_provider_trx_uq ON payments (provider, upper(transaction_id)) WHERE status <> 'rejected';
CREATE UNIQUE INDEX payments_one_pending_per_booking_uq ON payments (booking_id) WHERE status = 'pending';
CREATE INDEX payments_status_idx ON payments (status, created_at);
CREATE INDEX payments_booking_idx ON payments (booking_id);

-- ---------------------------------------------------------------------------
-- reviews: one per completed booking, with optional photos. Aggregated into
-- guide_gigs, guide_profiles and tourist_spots rating columns.
-- ---------------------------------------------------------------------------
CREATE TABLE reviews (
    id          UUID PRIMARY KEY,
    booking_id  UUID         NOT NULL REFERENCES bookings (id) ON DELETE CASCADE,
    gig_id      UUID         NOT NULL REFERENCES guide_gigs (id) ON DELETE CASCADE,
    guide_id    UUID         NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    spot_id     UUID         NOT NULL REFERENCES tourist_spots (id) ON DELETE CASCADE,
    user_id     UUID         NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    rating      SMALLINT     NOT NULL CHECK (rating BETWEEN 1 AND 5),
    comment     VARCHAR(2000) NOT NULL DEFAULT '',
    guide_reply VARCHAR(1000),
    replied_at  TIMESTAMPTZ,
    is_visible  BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT reviews_booking_uq UNIQUE (booking_id)
);
CREATE INDEX reviews_gig_idx ON reviews (gig_id, created_at DESC) WHERE is_visible;
CREATE INDEX reviews_guide_idx ON reviews (guide_id, created_at DESC) WHERE is_visible;
CREATE INDEX reviews_spot_idx ON reviews (spot_id, created_at DESC) WHERE is_visible;

CREATE TABLE review_images (
    id         UUID PRIMARY KEY,
    review_id  UUID         NOT NULL REFERENCES reviews (id) ON DELETE CASCADE,
    url        TEXT         NOT NULL,
    public_id  VARCHAR(255),
    position   SMALLINT     NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX review_images_review_idx ON review_images (review_id, position);

-- +goose Down
DROP TABLE IF EXISTS review_images;
DROP TABLE IF EXISTS reviews;
DROP TABLE IF EXISTS payments;
DROP TABLE IF EXISTS payment_methods;
DROP TABLE IF EXISTS bookings;
DROP TABLE IF EXISTS guide_gig_quotas;
DROP TABLE IF EXISTS guide_gigs;
