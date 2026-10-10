-- Reset password oleh pengelola: pengguna dengan password sementara wajib
-- membuat password baru saat masuk berikutnya. Passphrase jurnal tidak terpengaruh.

-- +goose Up
ALTER TABLE users ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE users DROP COLUMN must_change_password;
