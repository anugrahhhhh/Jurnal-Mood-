// Package db mengelola koneksi PostgreSQL dan migrasi skema.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // driver database/sql "pgx" untuk goose
	"github.com/pressly/goose/v3"

	"moodjournal/internal/migrations"
)

func Connect(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("konfigurasi database: %w", err)
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("tidak bisa terhubung ke database: %w", err)
	}
	return pool, nil
}

// Migrate menjalankan semua migrasi yang belum diterapkan.
func Migrate(ctx context.Context, url string) error {
	sqlDB, err := sql.Open("pgx", url)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, migrations.FS)
	if err != nil {
		return fmt.Errorf("menyiapkan migrasi: %w", err)
	}
	if _, err := p.Up(ctx); err != nil {
		return fmt.Errorf("menjalankan migrasi: %w", err)
	}
	return nil
}
