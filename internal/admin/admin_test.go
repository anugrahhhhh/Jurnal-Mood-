package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	"moodjournal/internal/auth"
	"moodjournal/internal/testdb"
)

func TestStatsSignupsDelete(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	svc, _ := auth.NewService(pool, false)
	a, err := svc.Register(ctx, "a@contoh.id", "kuda-laut-biru-42", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Register(ctx, "b@contoh.id", "kuda-laut-biru-42", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO entries (user_id, entry_date, mood, sleep_minutes, format_version, payload_ct, payload_iv)
		VALUES ($1, CURRENT_DATE, 4, 420, 1, decode(repeat('ab', 20), 'hex'), decode(repeat('cd', 12), 'hex'))`, a); err != nil {
		t.Fatal(err)
	}
	now := time.Now()

	s, err := GetStats(ctx, pool, now)
	if err != nil {
		t.Fatal(err)
	}
	if s.Users != 2 || s.ActiveLast7Days != 1 || s.EntriesLast7Days != 1 || s.SignupsLast7Days != 2 {
		t.Errorf("statistik: %+v", s)
	}

	list, err := RecentSignups(ctx, pool, now.AddDate(0, 0, -7))
	if err != nil || len(list) != 2 {
		t.Fatalf("pendaftaran terbaru: %v %v", list, err)
	}
	if list[0].Email != "b@contoh.id" || list[1].Entries != 1 {
		t.Errorf("urutan/isi tidak sesuai: %+v", list)
	}

	if _, err := DeleteUserByEmail(ctx, pool, "tidak@ada.id", true); !errors.Is(err, auth.ErrUserNotFound) {
		t.Errorf("email tidak ada: %v", err)
	}
	sum, err := DeleteUserByEmail(ctx, pool, "A@contoh.id", false)
	if err != nil || sum.Deleted || sum.Entries != 1 {
		t.Fatalf("pratinjau tidak boleh menghapus: %+v %v", sum, err)
	}
	if s, _ := GetStats(ctx, pool, now); s.Users != 2 {
		t.Fatal("pratinjau menghapus akun")
	}
	sum, err = DeleteUserByEmail(ctx, pool, "a@contoh.id", true)
	if err != nil || !sum.Deleted {
		t.Fatalf("hapus: %+v %v", sum, err)
	}
	s, _ = GetStats(ctx, pool, now)
	if s.Users != 1 || s.EntriesLast7Days != 0 {
		t.Errorf("akun dan entrinya harus hilang: %+v", s)
	}
}
