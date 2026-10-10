-- Tahap 6: pengingat harian lewat Web Push (opsional, per perangkat).
-- Terlihat pengelola: jam pengingat, zona waktu, dan alamat layanan push
-- (menunjukkan jenis browser). Disebut di halaman privasi.

-- +goose Up
CREATE TABLE push_subscriptions (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id       BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    endpoint      TEXT NOT NULL UNIQUE,
    p256dh        TEXT NOT NULL,
    auth          TEXT NOT NULL,
    remind_minute SMALLINT NOT NULL CHECK (remind_minute BETWEEN 0 AND 1439), -- menit sejak tengah malam, waktu lokal
    timezone      TEXT NOT NULL,                                             -- nama IANA, mis. Asia/Jakarta
    last_sent_on  DATE,                                                      -- tanggal lokal terakhir dikirim
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX push_subscriptions_user_id_idx ON push_subscriptions (user_id);

-- +goose Down
DROP TABLE push_subscriptions;
