package entries

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"moodjournal/internal/auth"
	"moodjournal/internal/testdb"
)

func randBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

var fixedNow = time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)

func day(s string) time.Time {
	d, err := ParseDate(s)
	if err != nil {
		panic(err)
	}
	return d
}

func entry(date string, mood, sleep int) Entry {
	return Entry{Date: day(date), Mood: mood, SleepMinutes: sleep, Version: FormatVersion,
		PayloadCT: randBytes(64), PayloadIV: randBytes(12)}
}

func TestValidate(t *testing.T) {
	cases := map[string]func(*Entry){
		"mood 0":      func(e *Entry) { e.Mood = 0 },
		"mood 6":      func(e *Entry) { e.Mood = 6 },
		"tidur 45":    func(e *Entry) { e.SleepMinutes = 45 },
		"tidur > 24j": func(e *Entry) { e.SleepMinutes = 1470 },
		"tidur minus": func(e *Entry) { e.SleepMinutes = -30 },
		"versi":       func(e *Entry) { e.Version = 2 },
		"iv":          func(e *Entry) { e.PayloadIV = randBytes(8) },
		"ct kosong":   func(e *Entry) { e.PayloadCT = nil },
		"ct raksasa":  func(e *Entry) { e.PayloadCT = randBytes(maxCTLen + 1) },
	}
	if err := entry("2026-10-10", 3, 450).validate(); err != nil {
		t.Fatalf("entri valid ditolak: %v", err)
	}
	for name, mutate := range cases {
		e := entry("2026-10-10", 3, 450)
		mutate(&e)
		if e.validate() == nil {
			t.Errorf("%s: diterima padahal tidak valid", name)
		}
	}
}

func TestEditableDate(t *testing.T) {
	s := &Store{now: func() time.Time { return fixedNow }}
	for _, ok := range []string{"2026-10-08", "2026-10-09", "2026-10-10", "2026-10-11"} {
		if err := s.EditableDate(day(ok)); err != nil {
			t.Errorf("%s harus boleh (toleransi zona waktu): %v", ok, err)
		}
	}
	for _, bad := range []string{"2026-10-07", "2026-10-12", "2025-10-10"} {
		if s.EditableDate(day(bad)) == nil {
			t.Errorf("%s tidak boleh diisi", bad)
		}
	}
}

func setup(t *testing.T) (*Store, int64, int64) {
	t.Helper()
	pool := testdb.Open(t)
	svc, _ := auth.NewService(pool, false)
	ctx := context.Background()
	a, err := svc.Register(ctx, "a@contoh.id", "kuda-laut-biru-42", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.Register(ctx, "b@contoh.id", "kuda-laut-biru-42", "")
	if err != nil {
		t.Fatal(err)
	}
	s := NewStore(pool)
	s.now = func() time.Time { return fixedNow }
	return s, a, b
}

func TestSaveRangeAndUpdate(t *testing.T) {
	s, a, b := setup(t)
	ctx := context.Background()

	first := entry("2026-10-09", 2, 420)
	if err := s.Save(ctx, a, first); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(ctx, a, entry("2026-10-10", 4, 480)); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(ctx, b, entry("2026-10-10", 1, 300)); err != nil {
		t.Fatal(err)
	}

	got, err := s.Range(ctx, a, day("2026-10-01"), day("2026-10-10"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].DateString() != "2026-10-09" || got[1].Mood != 4 {
		t.Fatalf("Range pengguna a: %+v", got)
	}
	if !bytes.Equal(got[0].PayloadCT, first.PayloadCT) {
		t.Error("ciphertext harus disimpan apa adanya")
	}

	// Edit: satu entri per hari, ditimpa.
	edited := entry("2026-10-09", 3, 450)
	if err := s.Save(ctx, a, edited); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Range(ctx, a, day("2026-10-09"), day("2026-10-09"))
	if len(got) != 1 || got[0].Mood != 3 || !bytes.Equal(got[0].PayloadIV, edited.PayloadIV) {
		t.Errorf("edit tidak tersimpan: %+v", got)
	}

	// Pengguna b hanya melihat entrinya sendiri.
	got, _ = s.Range(ctx, b, day("2026-10-01"), day("2026-10-10"))
	if len(got) != 1 || got[0].Mood != 1 {
		t.Errorf("Range pengguna b: %+v", got)
	}

	var verr ValidationError
	if err := s.Save(ctx, a, entry("2026-09-01", 3, 450)); !errors.As(err, &verr) {
		t.Errorf("tanggal lama harus ditolak: %v", err)
	}
	if _, err := s.Range(ctx, a, day("2024-01-01"), day("2026-10-10")); !errors.As(err, &verr) {
		t.Errorf("rentang terlalu panjang harus ditolak: %v", err)
	}
	if _, err := s.Range(ctx, a, day("2026-10-10"), day("2026-10-01")); !errors.As(err, &verr) {
		t.Errorf("rentang terbalik harus ditolak: %v", err)
	}
}

func TestLastSleep(t *testing.T) {
	s, a, _ := setup(t)
	ctx := context.Background()
	if _, ok, err := s.LastSleep(ctx, a, day("2026-10-10")); ok || err != nil {
		t.Errorf("tanpa entri: ok=%v err=%v", ok, err)
	}
	s.Save(ctx, a, entry("2026-10-08", 3, 390))
	s.Save(ctx, a, entry("2026-10-09", 3, 420))
	m, ok, err := s.LastSleep(ctx, a, day("2026-10-10"))
	if err != nil || !ok || m != 420 {
		t.Errorf("LastSleep = %d %v %v, mau 420", m, ok, err)
	}
	m, _, _ = s.LastSleep(ctx, a, day("2026-10-09"))
	if m != 390 {
		t.Errorf("LastSleep sebelum 09 = %d, mau 390", m)
	}
}

func TestActivities(t *testing.T) {
	s, a, b := setup(t)
	ctx := context.Background()
	if _, err := s.Activities(ctx, a); !errors.Is(err, ErrNotFound) {
		t.Errorf("sebelum disimpan: %v", err)
	}
	blob := Blob{CT: randBytes(40), IV: randBytes(12)}
	if err := s.SaveActivities(ctx, a, blob); err != nil {
		t.Fatal(err)
	}
	blob2 := Blob{CT: randBytes(50), IV: randBytes(12)}
	if err := s.SaveActivities(ctx, a, blob2); err != nil {
		t.Fatal(err)
	}
	got, err := s.Activities(ctx, a)
	if err != nil || !bytes.Equal(got.CT, blob2.CT) {
		t.Errorf("Activities: %v", err)
	}
	if _, err := s.Activities(ctx, b); !errors.Is(err, ErrNotFound) {
		t.Error("pengguna lain tidak boleh melihat daftar aktivitas")
	}
	var verr ValidationError
	if err := s.SaveActivities(ctx, a, Blob{CT: randBytes(40), IV: randBytes(4)}); !errors.As(err, &verr) {
		t.Errorf("IV salah harus ditolak: %v", err)
	}
}
