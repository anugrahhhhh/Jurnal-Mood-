// Package admin berisi perintah pengelola yang dijalankan dari terminal server
// (bukan halaman web), sehingga tidak ada permukaan serangan tambahan.
//
// Statistik sengaja agregat: tidak menampilkan mood, tidur, atau isi apa pun
// per pengguna. Daftar pendaftaran hanya untuk mendeteksi akun spam.
package admin

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"moodjournal/internal/auth"
)

type Stats struct {
	Users              int
	UsersWithJournal   int
	ActiveLast7Days    int // pengguna yang mengisi minimal satu entri dalam 7 hari terakhir
	ActiveLast30Days   int
	EntriesLast7Days   int
	SignupsLast7Days   int
	ReminderDevices    int
	MustChangePassword int
}

func GetStats(ctx context.Context, db *pgxpool.Pool, now time.Time) (Stats, error) {
	var s Stats
	d7 := now.AddDate(0, 0, -7)
	d30 := now.AddDate(0, 0, -30)
	err := db.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM users),
			(SELECT count(*) FROM journal_keys),
			(SELECT count(DISTINCT user_id) FROM entries WHERE updated_at >= $1),
			(SELECT count(DISTINCT user_id) FROM entries WHERE updated_at >= $2),
			(SELECT count(*) FROM entries WHERE updated_at >= $1),
			(SELECT count(*) FROM users WHERE created_at >= $1),
			(SELECT count(*) FROM push_subscriptions),
			(SELECT count(*) FROM users WHERE must_change_password)`, d7, d30).
		Scan(&s.Users, &s.UsersWithJournal, &s.ActiveLast7Days, &s.ActiveLast30Days,
			&s.EntriesLast7Days, &s.SignupsLast7Days, &s.ReminderDevices, &s.MustChangePassword)
	return s, err
}

type Signup struct {
	CreatedAt  time.Time
	Email      string
	HasJournal bool
	Entries    int
}

// RecentSignups: akun yang dibuat sejak since, terbaru dulu (maks 200).
// Akun tanpa kunci jurnal dan tanpa entri yang banyak sekaligus adalah tanda spam.
func RecentSignups(ctx context.Context, db *pgxpool.Pool, since time.Time) ([]Signup, error) {
	rows, err := db.Query(ctx, `
		SELECT u.created_at, u.email,
			EXISTS (SELECT 1 FROM journal_keys k WHERE k.user_id = u.id),
			(SELECT count(*) FROM entries e WHERE e.user_id = u.id)
		FROM users u WHERE u.created_at >= $1
		ORDER BY u.created_at DESC LIMIT 200`, since)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Signup, error) {
		var s Signup
		err := r.Scan(&s.CreatedAt, &s.Email, &s.HasJournal, &s.Entries)
		return s, err
	})
}

// Summary menjelaskan apa yang akan atau sudah dihapus.
type Summary struct {
	Email     string
	CreatedAt time.Time
	Entries   int
	Devices   int
	Deleted   bool
}

// DeleteUserByEmail menghapus akun beserta semua datanya (ON DELETE CASCADE).
// Tanpa confirm, hanya mengembalikan ringkasan (pratinjau) tanpa menghapus.
func DeleteUserByEmail(ctx context.Context, db *pgxpool.Pool, email string, confirm bool) (Summary, error) {
	email, err := auth.NormalizeEmail(email)
	if err != nil {
		return Summary{}, err
	}
	var sum Summary
	var id int64
	err = db.QueryRow(ctx, `
		SELECT u.id, u.email, u.created_at,
			(SELECT count(*) FROM entries e WHERE e.user_id = u.id),
			(SELECT count(*) FROM push_subscriptions p WHERE p.user_id = u.id)
		FROM users u WHERE u.email = $1`, email).Scan(&id, &sum.Email, &sum.CreatedAt, &sum.Entries, &sum.Devices)
	if errors.Is(err, pgx.ErrNoRows) {
		return Summary{}, auth.ErrUserNotFound
	}
	if err != nil || !confirm {
		return sum, err
	}
	if _, err := db.Exec(ctx, `DELETE FROM users WHERE id = $1`, id); err != nil {
		return sum, err
	}
	sum.Deleted = true
	return sum, nil
}
