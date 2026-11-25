-- +goose Up
-- ---------------------------------------------------------------------------
-- New booking flow:
--   tourist requests ─▶ guide accepts ─▶ tourist pays online (aamarPay) ─▶ tour
--   ─▶ tourist confirms completion (or an admin after end_date, after talking to the guide).
-- A booking now covers a date range (tour_date .. end_date) so a guide's busy days can be
-- shown as booked slots on the gig page and double-booking is blocked.
-- ---------------------------------------------------------------------------
ALTER TABLE bookings ADD COLUMN end_date DATE;
UPDATE bookings SET end_date = tour_date WHERE end_date IS NULL;
ALTER TABLE bookings ALTER COLUMN end_date SET NOT NULL;
ALTER TABLE bookings ADD CONSTRAINT bookings_date_range_chk CHECK (end_date >= tour_date AND end_date <= tour_date + 60);
ALTER TABLE bookings ADD COLUMN paid_at TIMESTAMPTZ;
ALTER TABLE bookings ADD COLUMN completed_by UUID REFERENCES users (id) ON DELETE SET NULL;
ALTER TABLE bookings ADD COLUMN completion_note VARCHAR(500);
CREATE INDEX bookings_guide_range_idx ON bookings (guide_id, tour_date, end_date) WHERE status IN ('pending', 'accepted');

-- ---------------------------------------------------------------------------
-- gateway_payments: one row per online checkout attempt (aamarPay).
-- Nothing here is trusted from the browser: a row only becomes `success` after the
-- server re-checks the transaction with aamarPay's Search Transaction API and the
-- amount / currency / store match. The signature key is never stored.
-- ---------------------------------------------------------------------------
CREATE TABLE gateway_payments (
    id                UUID PRIMARY KEY,
    gateway           VARCHAR(20)  NOT NULL DEFAULT 'aamarpay' CHECK (gateway IN ('aamarpay')),
    mode              VARCHAR(10)  NOT NULL CHECK (mode IN ('sandbox', 'live')),
    tran_id           VARCHAR(40)  NOT NULL,             -- our merchant transaction id (mer_txnid)
    booking_id        UUID         NOT NULL REFERENCES bookings (id) ON DELETE RESTRICT,
    gig_id            UUID         NOT NULL REFERENCES guide_gigs (id) ON DELETE RESTRICT,
    payer_id          UUID         NOT NULL REFERENCES users (id) ON DELETE RESTRICT,   -- tourist
    guide_id          UUID         NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    amount            INTEGER      NOT NULL CHECK (amount > 0),
    currency          CHAR(3)      NOT NULL DEFAULT 'BDT',
    status            VARCHAR(16)  NOT NULL DEFAULT 'initiated'
        CHECK (status IN ('initiated', 'success', 'failed', 'cancelled', 'duplicate')),
    pg_txn_id         VARCHAR(100),                      -- aamarPay transaction id
    bank_txn          VARCHAR(100),
    payment_type      VARCHAR(50),                       -- bKash, Nagad, VISA, ...
    card_number       VARCHAR(40),                       -- masked by aamarPay
    paid_amount       NUMERIC(12, 2),
    store_amount      NUMERIC(12, 2),
    processing_charge NUMERIC(12, 2),
    failure_reason    VARCHAR(500),
    gateway_response  JSONB,                             -- last verified response (no secrets)
    client_ip         VARCHAR(64),
    verified_at       TIMESTAMPTZ,
    created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT gateway_payments_tran_uq UNIQUE (tran_id)
);
-- A booking can be paid successfully only once.
CREATE UNIQUE INDEX gateway_payments_one_success_uq ON gateway_payments (booking_id) WHERE status = 'success';
CREATE UNIQUE INDEX gateway_payments_pg_txn_uq ON gateway_payments (pg_txn_id) WHERE pg_txn_id IS NOT NULL;
CREATE INDEX gateway_payments_booking_idx ON gateway_payments (booking_id, created_at DESC);
CREATE INDEX gateway_payments_status_idx ON gateway_payments (status, created_at DESC);
CREATE INDEX gateway_payments_payer_idx ON gateway_payments (payer_id, created_at DESC);
CREATE INDEX gateway_payments_guide_idx ON gateway_payments (guide_id, created_at DESC);

-- Append-only audit trail of every initiate / callback / verification.
CREATE TABLE gateway_payment_events (
    id         BIGSERIAL PRIMARY KEY,
    payment_id UUID        NOT NULL REFERENCES gateway_payments (id) ON DELETE CASCADE,
    event      VARCHAR(40) NOT NULL,
    payload    JSONB,
    ip_address VARCHAR(64),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX gateway_payment_events_payment_idx ON gateway_payment_events (payment_id, id);

-- Payments can now be submitted after the guide accepts (before the tour).
-- (The manual mobile-banking flow stays available as a fallback.)

-- ---------------------------------------------------------------------------
-- booking_trip_plans: after payment the guide (and the tourist) write down the plan —
-- arrival, pickup point (Google Maps link), hotel, day-by-day itinerary. Admins monitor it.
-- ---------------------------------------------------------------------------
CREATE TABLE booking_trip_plans (
    booking_id      UUID PRIMARY KEY REFERENCES bookings (id) ON DELETE CASCADE,
    arrival_at      TIMESTAMPTZ,
    arrival_from    VARCHAR(200),
    arrival_mode    VARCHAR(30),
    pickup_name     VARCHAR(200),
    pickup_map_url  VARCHAR(1000),
    hotel_name      VARCHAR(200),
    hotel_map_url   VARCHAR(1000),
    hotel_check_in  DATE,
    hotel_check_out DATE,
    itinerary       JSONB        NOT NULL DEFAULT '[]',
    notes           VARCHAR(2000),
    updated_by      UUID REFERENCES users (id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX booking_trip_plans_updated_idx ON booking_trip_plans (updated_at DESC);

-- ---------------------------------------------------------------------------
-- booking_messages: tourist <-> guide chat, opened once the booking is paid.
-- ---------------------------------------------------------------------------
CREATE TABLE booking_messages (
    id         UUID PRIMARY KEY,
    booking_id UUID          NOT NULL REFERENCES bookings (id) ON DELETE CASCADE,
    sender_id  UUID          NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    body       VARCHAR(2000) NOT NULL,
    read_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ   NOT NULL DEFAULT now()
);
CREATE INDEX booking_messages_booking_idx ON booking_messages (booking_id, created_at DESC);
CREATE INDEX booking_messages_unread_idx ON booking_messages (booking_id, sender_id) WHERE read_at IS NULL;

-- ---------------------------------------------------------------------------
-- notifications: in-app inbox, pushed live over Socket.IO.
-- ---------------------------------------------------------------------------
CREATE TABLE notifications (
    id         UUID PRIMARY KEY,
    user_id    UUID         NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    type       VARCHAR(40)  NOT NULL,
    title      VARCHAR(150) NOT NULL,
    body       VARCHAR(500) NOT NULL DEFAULT '',
    link       VARCHAR(300),
    data       JSONB,
    read_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX notifications_user_idx ON notifications (user_id, created_at DESC);
CREATE INDEX notifications_unread_idx ON notifications (user_id) WHERE read_at IS NULL;

-- +goose Down
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS booking_messages;
DROP TABLE IF EXISTS booking_trip_plans;
DROP TABLE IF EXISTS gateway_payment_events;
DROP TABLE IF EXISTS gateway_payments;
DROP INDEX IF EXISTS bookings_guide_range_idx;
ALTER TABLE bookings DROP COLUMN IF EXISTS completion_note;
ALTER TABLE bookings DROP COLUMN IF EXISTS completed_by;
ALTER TABLE bookings DROP COLUMN IF EXISTS paid_at;
ALTER TABLE bookings DROP CONSTRAINT IF EXISTS bookings_date_range_chk;
ALTER TABLE bookings DROP COLUMN IF EXISTS end_date;
