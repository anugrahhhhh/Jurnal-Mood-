-- Bukti persetujuan Syarat Penggunaan & Privasi saat mendaftar (UU PDP).
-- Akun lama bernilai NULL (mendaftar sebelum kotak persetujuan ada).

-- +goose Up
ALTER TABLE users ADD COLUMN terms_accepted_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE users DROP COLUMN terms_accepted_at;
