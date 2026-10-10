-- Tahap 2: material kunci jurnal per pengguna (SPEC 3.2).
-- Server hanya menyimpan kunci data (DEK) yang sudah dibungkus oleh kunci
-- turunan passphrase. Passphrase dan DEK asli tidak pernah sampai ke server.

-- +goose Up
CREATE TABLE journal_keys (
    user_id        BIGINT PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    format_version SMALLINT NOT NULL,
    kdf            TEXT NOT NULL,          -- 'PBKDF2-SHA256'
    kdf_iterations INTEGER NOT NULL,
    salt           BYTEA NOT NULL,         -- 16 byte
    wrapped_dek    BYTEA NOT NULL,         -- AES-GCM(KEK, DEK) = 32 + 16 byte tag
    wrap_iv        BYTEA NOT NULL,         -- 12 byte
    verifier_ct    BYTEA NOT NULL,         -- AES-GCM(DEK, konstanta), untuk cek passphrase
    verifier_iv    BYTEA NOT NULL,         -- 12 byte
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE journal_keys;
