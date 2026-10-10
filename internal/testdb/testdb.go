// Package testdb menyiapkan database PostgreSQL untuk tes integrasi.
//
// Tes dilewati jika TEST_DATABASE_URL tidak diset. Karena semua paket
// memakai database yang sama, jalankan dengan `go test -p 1 ./...`
// (sudah diatur di run.ps1).
package testdb

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"moodjournal/internal/db"
)

func Open(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL tidak diset; tes database dilewati")
	}
	ctx := context.Background()
	if err := db.Migrate(ctx, url); err != nil {
		t.Fatalf("migrasi: %v", err)
	}
	pool, err := db.Connect(ctx, url)
	if err != nil {
		t.Fatalf("koneksi: %v", err)
	}
	if _, err := pool.Exec(ctx, `TRUNCATE users, invites, sessions, journal_keys, entries, user_settings, push_subscriptions RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("membersihkan tabel: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}
