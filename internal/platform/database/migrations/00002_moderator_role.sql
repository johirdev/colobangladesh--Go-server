-- +goose Up
-- Moderators manage content (geo data, tourist spots) but cannot manage users or money.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('user', 'guide', 'admin', 'moderator'));

-- +goose Down
UPDATE users SET role = 'user' WHERE role = 'moderator';
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_role_check;
ALTER TABLE users ADD CONSTRAINT users_role_check CHECK (role IN ('user', 'guide', 'admin'));
