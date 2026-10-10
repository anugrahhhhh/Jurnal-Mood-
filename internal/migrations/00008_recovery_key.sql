-- Kunci pemulihan passphrase: DEK yang sama dibungkus oleh kunci acak 120 bit
-- yang hanya dimiliki pengguna (dicatat/dicetak saat dibuat). Server hanya
-- menyimpan bungkusnya, sama seperti bungkus dari passphrase.

-- +goose Up
ALTER TABLE journal_keys
    ADD COLUMN recovery_iterations  INTEGER,
    ADD COLUMN recovery_salt        BYTEA,
    ADD COLUMN recovery_wrapped_dek BYTEA,
    ADD COLUMN recovery_wrap_iv     BYTEA,
    ADD COLUMN recovery_updated_at  TIMESTAMPTZ;

-- +goose Down
ALTER TABLE journal_keys
    DROP COLUMN recovery_iterations,
    DROP COLUMN recovery_salt,
    DROP COLUMN recovery_wrapped_dek,
    DROP COLUMN recovery_wrap_iv,
    DROP COLUMN recovery_updated_at;
