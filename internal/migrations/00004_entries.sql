-- Tahap 3: entri harian (SPEC 3.1, 5).
-- mood dan sleep_minutes disimpan biasa (bisa dibaca admin, disebut di halaman privasi).
-- Aktivitas + catatan hanya ada di payload_ct, terenkripsi di browser.

-- +goose Up
CREATE TABLE entries (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id        BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    entry_date     DATE NOT NULL,
    mood           SMALLINT NOT NULL CHECK (mood BETWEEN 1 AND 5),
    sleep_minutes  INTEGER NOT NULL CHECK (sleep_minutes BETWEEN 0 AND 1440 AND sleep_minutes % 30 = 0),
    format_version SMALLINT NOT NULL,
    payload_ct     BYTEA NOT NULL,
    payload_iv     BYTEA NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, entry_date)
);

-- Daftar chip aktivitas buatan pengguna, terenkripsi.
CREATE TABLE user_settings (
    user_id       BIGINT PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    activities_ct BYTEA NOT NULL,
    activities_iv BYTEA NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE user_settings;
DROP TABLE entries;
