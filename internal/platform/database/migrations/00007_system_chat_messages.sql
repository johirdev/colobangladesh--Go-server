-- +goose Up
-- ---------------------------------------------------------------------------
-- System messages in the booking chat: CholoBangladesh posts a thank-you note when a
-- tour is completed (and the chat becomes read-only). System messages have no sender.
-- ---------------------------------------------------------------------------
ALTER TABLE booking_messages ADD COLUMN kind VARCHAR(20) NOT NULL DEFAULT 'user';
ALTER TABLE booking_messages ALTER COLUMN sender_id DROP NOT NULL;
ALTER TABLE booking_messages ADD CONSTRAINT booking_messages_kind_chk
    CHECK ((kind = 'user' AND sender_id IS NOT NULL) OR (kind = 'system' AND sender_id IS NULL));
ALTER TABLE booking_messages ALTER COLUMN body TYPE VARCHAR(4000);

-- +goose Down
DELETE FROM booking_messages WHERE kind = 'system';
ALTER TABLE booking_messages DROP CONSTRAINT IF EXISTS booking_messages_kind_chk;
ALTER TABLE booking_messages ALTER COLUMN sender_id SET NOT NULL;
ALTER TABLE booking_messages DROP COLUMN IF EXISTS kind;
ALTER TABLE booking_messages ALTER COLUMN body TYPE VARCHAR(2000);
