-- +goose Up
CREATE EXTENSION IF NOT EXISTS citext;

-- ---------------------------------------------------------------------------
-- users: identity + authentication only (hot table, kept narrow).
-- ---------------------------------------------------------------------------
CREATE TABLE users (
    id                UUID PRIMARY KEY,
    role              VARCHAR(16)  NOT NULL CHECK (role IN ('user', 'guide', 'admin')),
    status            VARCHAR(16)  NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'deleted')),
    email             CITEXT,
    email_verified_at TIMESTAMPTZ,
    phone             VARCHAR(20),
    phone_verified_at TIMESTAMPTZ,
    password_hash     TEXT,
    token_version     INTEGER      NOT NULL DEFAULT 1,
    last_login_at     TIMESTAMPTZ,
    last_login_ip     VARCHAR(64),
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at        TIMESTAMPTZ,
    CONSTRAINT users_contact_required CHECK (email IS NOT NULL OR phone IS NOT NULL)
);
CREATE UNIQUE INDEX users_email_uq ON users (email) WHERE deleted_at IS NULL AND email IS NOT NULL;
CREATE UNIQUE INDEX users_phone_uq ON users (phone) WHERE deleted_at IS NULL AND phone IS NOT NULL;
CREATE INDEX users_role_status_idx ON users (role, status) WHERE deleted_at IS NULL;
CREATE INDEX users_created_at_idx ON users (created_at DESC);

-- ---------------------------------------------------------------------------
-- user_profiles: personal info shared by every role (1:1 with users).
-- ---------------------------------------------------------------------------
CREATE TABLE user_profiles (
    user_id            UUID PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    full_name          VARCHAR(100) NOT NULL,
    avatar_url         TEXT,
    avatar_public_id   VARCHAR(255),
    bio                VARCHAR(500),
    gender             VARCHAR(20) CHECK (gender IN ('male', 'female', 'other', 'prefer_not_to_say')),
    date_of_birth      DATE,
    country_code       CHAR(2),
    city               VARCHAR(100),
    preferred_language VARCHAR(10)  NOT NULL DEFAULT 'en',
    whatsapp_number    VARCHAR(20),
    created_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ  NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- guide_profiles: marketplace data only guides have (1:1 with users).
-- ---------------------------------------------------------------------------
CREATE TABLE guide_profiles (
    user_id             UUID PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    headline            VARCHAR(120),
    about               TEXT,
    languages           TEXT[]       NOT NULL DEFAULT '{}',
    service_areas       TEXT[]       NOT NULL DEFAULT '{}',
    specialties         TEXT[]       NOT NULL DEFAULT '{}',
    experience_years    SMALLINT     NOT NULL DEFAULT 0 CHECK (experience_years BETWEEN 0 AND 60),
    hourly_rate         INTEGER CHECK (hourly_rate >= 0),
    daily_rate          INTEGER CHECK (daily_rate >= 0),
    currency            CHAR(3)      NOT NULL DEFAULT 'BDT',
    max_group_size      SMALLINT     NOT NULL DEFAULT 10 CHECK (max_group_size BETWEEN 1 AND 100),
    license_number      VARCHAR(50),
    is_available        BOOLEAN      NOT NULL DEFAULT TRUE,
    verification_status VARCHAR(16)  NOT NULL DEFAULT 'unverified'
        CHECK (verification_status IN ('unverified', 'pending', 'approved', 'rejected')),
    verified_at         TIMESTAMPTZ,
    rating_avg          NUMERIC(3, 2) NOT NULL DEFAULT 0,
    rating_count        INTEGER      NOT NULL DEFAULT 0,
    total_tours         INTEGER      NOT NULL DEFAULT 0,
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX guide_profiles_listing_idx ON guide_profiles (verification_status, is_available, rating_avg DESC);
CREATE INDEX guide_profiles_languages_gin ON guide_profiles USING GIN (languages);
CREATE INDEX guide_profiles_areas_gin ON guide_profiles USING GIN (service_areas);
CREATE INDEX guide_profiles_specialties_gin ON guide_profiles USING GIN (specialties);

-- ---------------------------------------------------------------------------
-- guide_verifications: KYC submissions history (NID + selfie), admin-reviewed.
-- ---------------------------------------------------------------------------
CREATE TABLE guide_verifications (
    id                   UUID PRIMARY KEY,
    user_id              UUID         NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    nid_number_encrypted TEXT         NOT NULL,
    nid_last4            VARCHAR(4)   NOT NULL,
    nid_front_public_id  VARCHAR(255) NOT NULL,
    nid_back_public_id   VARCHAR(255) NOT NULL,
    selfie_public_id     VARCHAR(255) NOT NULL,
    status               VARCHAR(16)  NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected')),
    rejection_reason     VARCHAR(500),
    reviewed_by          UUID REFERENCES users (id) ON DELETE SET NULL,
    reviewed_at          TIMESTAMPTZ,
    created_at           TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX guide_verifications_user_idx ON guide_verifications (user_id, created_at DESC);
CREATE INDEX guide_verifications_status_idx ON guide_verifications (status, created_at);
CREATE UNIQUE INDEX guide_verifications_one_pending_uq ON guide_verifications (user_id) WHERE status = 'pending';

-- ---------------------------------------------------------------------------
-- auth_identities: Google / Apple accounts linked to a user.
-- ---------------------------------------------------------------------------
CREATE TABLE auth_identities (
    id               UUID PRIMARY KEY,
    user_id          UUID         NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider         VARCHAR(20)  NOT NULL CHECK (provider IN ('google', 'apple')),
    provider_subject VARCHAR(255) NOT NULL,
    email            CITEXT,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
    last_used_at     TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT auth_identities_provider_subject_uq UNIQUE (provider, provider_subject)
);
CREATE INDEX auth_identities_user_idx ON auth_identities (user_id);

-- ---------------------------------------------------------------------------
-- user_sessions: one row per logged-in device; refresh tokens stored as SHA-256.
-- ---------------------------------------------------------------------------
CREATE TABLE user_sessions (
    id                  UUID PRIMARY KEY,
    user_id             UUID         NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    refresh_token_hash  CHAR(64)     NOT NULL,
    previous_token_hash CHAR(64),
    rotated_at          TIMESTAMPTZ,
    user_agent          VARCHAR(255),
    ip_address          VARCHAR(64),
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    last_used_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    expires_at          TIMESTAMPTZ  NOT NULL,
    revoked_at          TIMESTAMPTZ,
    CONSTRAINT user_sessions_refresh_hash_uq UNIQUE (refresh_token_hash)
);
CREATE INDEX user_sessions_user_active_idx ON user_sessions (user_id) WHERE revoked_at IS NULL;
CREATE INDEX user_sessions_prev_hash_idx ON user_sessions (previous_token_hash) WHERE previous_token_hash IS NOT NULL;
CREATE INDEX user_sessions_expires_idx ON user_sessions (expires_at);

-- ---------------------------------------------------------------------------
-- audit_logs: security-relevant events (login, lockout, KYC review, ...).
-- ---------------------------------------------------------------------------
CREATE TABLE audit_logs (
    id         BIGSERIAL PRIMARY KEY,
    user_id    UUID,
    actor_id   UUID,
    event      VARCHAR(50) NOT NULL,
    ip_address VARCHAR(64),
    user_agent VARCHAR(255),
    metadata   JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX audit_logs_user_idx ON audit_logs (user_id, created_at DESC);
CREATE INDEX audit_logs_event_idx ON audit_logs (event, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS user_sessions;
DROP TABLE IF EXISTS auth_identities;
DROP TABLE IF EXISTS guide_verifications;
DROP TABLE IF EXISTS guide_profiles;
DROP TABLE IF EXISTS user_profiles;
DROP TABLE IF EXISTS users;
