-- Tahap 1: akun, kode undangan, dan sesi login.

-- +goose Up
CREATE TABLE users (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email         TEXT NOT NULL UNIQUE,      -- disimpan sudah dinormalisasi (huruf kecil)
    password_hash TEXT NOT NULL,             -- Argon2id, format PHC
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Kode undangan disimpan sebagai hash SHA-256; kode aslinya hanya dilihat admin sekali.
CREATE TABLE invites (
    code_hash  BYTEA PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ,
    used_by    BIGINT REFERENCES users (id) ON DELETE SET NULL
);

-- Token sesi disimpan sebagai hash SHA-256; token aslinya hanya ada di cookie.
CREATE TABLE sessions (
    token_hash BYTEA PRIMARY KEY,
    user_id    BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX sessions_user_id_idx ON sessions (user_id);

-- +goose Down
DROP TABLE sessions;
DROP TABLE invites;
DROP TABLE users;
