package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"moodjournal/internal/testdb"
)

func TestPasswordHashRoundTrip(t *testing.T) {
	h, err := hashPassword("rahasia-panjang-sekali")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Errorf("format hash tidak terduga: %s", h)
	}
	if ok, _ := verifyPassword("rahasia-panjang-sekali", h); !ok {
		t.Error("password benar ditolak")
	}
	if ok, _ := verifyPassword("rahasia-panjang-sekalI", h); ok {
		t.Error("password salah diterima")
	}
	h2, _ := hashPassword("rahasia-panjang-sekali")
	if h == h2 {
		t.Error("salt harus acak: dua hash identik")
	}
	if _, err := verifyPassword("x", "$argon2id$v=19$m=1,t=0,p=0$AA$AA"); err == nil {
		t.Error("parameter tidak valid harus ditolak")
	}
}

func TestNormalizeInvite(t *testing.T) {
	if normalizeInvite("ab0c-1d2e-f3gh") != normalizeInvite("ABOC1D2EF3GH") {
		t.Error("huruf kecil, pemisah, dan O/0 harus dianggap sama")
	}
	if normalizeInvite("I1L") != "111" {
		t.Error("I dan L harus dipetakan ke 1")
	}
}

func TestNormalizeEmail(t *testing.T) {
	if got, err := NormalizeEmail("  Teman@Contoh.ID "); err != nil || got != "teman@contoh.id" {
		t.Errorf("got %q, %v", got, err)
	}
	for _, bad := range []string{"", "bukan-email", "Nama <a@b.id>", "a@b.id, c@d.id"} {
		if _, err := NormalizeEmail(bad); !errors.Is(err, ErrInvalidEmail) {
			t.Errorf("%q harus ditolak", bad)
		}
	}
}

func TestLimiter(t *testing.T) {
	l := newLimiter(2, time.Minute)
	now := time.Now()
	l.fail("k", now)
	if l.blocked("k", now) {
		t.Error("belum boleh terblokir setelah 1 kegagalan")
	}
	l.fail("k", now)
	if !l.blocked("k", now) {
		t.Error("harus terblokir setelah 2 kegagalan")
	}
	if l.blocked("k", now.Add(2*time.Minute)) {
		t.Error("blokir harus berakhir setelah jendela waktu")
	}
}

// ---- Tes dengan database ----

func newService(t *testing.T, inviteRequired bool) *Service {
	t.Helper()
	s, err := NewService(testdb.Open(t), inviteRequired)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

const goodPassword = "kuda-laut-biru-42"

func TestRegisterWithInvite(t *testing.T) {
	s := newService(t, true)
	ctx := context.Background()

	if _, err := s.Register(ctx, "a@contoh.id", goodPassword, "SALAH-SALA-HSAL"); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("kode palsu: got %v", err)
	}

	code, err := s.CreateInvite(ctx, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Register(ctx, "a@contoh.id", goodPassword, strings.ToLower(code)); err != nil {
		t.Fatalf("kode valid (huruf kecil) harus diterima: %v", err)
	}
	if _, err := s.Register(ctx, "b@contoh.id", goodPassword, code); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("kode sekali pakai dipakai ulang: got %v", err)
	}
}

func TestInviteNotConsumedOnFailedRegistration(t *testing.T) {
	s := newService(t, true)
	ctx := context.Background()
	first, _ := s.CreateInvite(ctx, time.Hour)
	if _, err := s.Register(ctx, "a@contoh.id", goodPassword, first); err != nil {
		t.Fatal(err)
	}

	code, _ := s.CreateInvite(ctx, time.Hour)
	_, err := s.Register(ctx, "A@contoh.id", goodPassword, code) // email sama, beda huruf besar
	if !errors.Is(err, ErrRegistrationFailed) {
		t.Fatalf("email ganda: got %v", err)
	}
	if strings.Contains(err.Error(), "terdaftar") {
		t.Error("pesan error tidak boleh membocorkan bahwa email sudah terdaftar")
	}
	if _, err := s.Register(ctx, "c@contoh.id", goodPassword, code); err != nil {
		t.Errorf("kode harus tetap bisa dipakai setelah pendaftaran gagal: %v", err)
	}
}

func TestExpiredInvite(t *testing.T) {
	s := newService(t, true)
	ctx := context.Background()
	code, _ := s.CreateInvite(ctx, time.Hour)
	s.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	if _, err := s.Register(ctx, "a@contoh.id", goodPassword, code); !errors.Is(err, ErrInviteInvalid) {
		t.Errorf("kode kedaluwarsa: got %v", err)
	}
}

func TestOpenRegistrationAndValidation(t *testing.T) {
	s := newService(t, false)
	ctx := context.Background()
	if _, err := s.Register(ctx, "a@contoh.id", "pendek", ""); !errors.Is(err, ErrWeakPassword) {
		t.Errorf("password pendek: got %v", err)
	}
	if _, err := s.Register(ctx, "a@contoh.id", strings.Repeat("x", 300), ""); !errors.Is(err, ErrPasswordTooLong) {
		t.Errorf("password terlalu panjang: got %v", err)
	}
	if _, err := s.Register(ctx, "a@contoh.id", goodPassword, ""); err != nil {
		t.Errorf("pendaftaran terbuka tanpa kode: %v", err)
	}
}

func TestLoginAndSessions(t *testing.T) {
	s := newService(t, false)
	ctx := context.Background()
	id, err := s.Register(ctx, "a@contoh.id", goodPassword, "")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.Login(ctx, "a@contoh.id", "salah-salah-salah", "1.2.3.4"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("password salah: got %v", err)
	}
	if _, err := s.Login(ctx, "tidak@ada.id", goodPassword, "1.2.3.4"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("email tidak ada harus memberi error yang sama: got %v", err)
	}
	got, err := s.Login(ctx, " A@Contoh.id ", goodPassword, "1.2.3.4")
	if err != nil || got != id {
		t.Fatalf("login benar: id=%d err=%v", got, err)
	}

	token, _, err := s.CreateSession(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	u, err := s.UserBySession(ctx, token)
	if err != nil || u.ID != id || u.Email != "a@contoh.id" {
		t.Fatalf("UserBySession: %+v %v", u, err)
	}

	var stored []byte
	s.db.QueryRow(ctx, `SELECT token_hash FROM sessions`).Scan(&stored)
	if strings.Contains(string(stored), token) {
		t.Error("token sesi tidak boleh disimpan apa adanya")
	}

	s.now = func() time.Time { return time.Now().Add(SessionTTL + time.Hour) }
	if _, err := s.UserBySession(ctx, token); !errors.Is(err, ErrNoSession) {
		t.Error("sesi kedaluwarsa harus ditolak")
	}
	s.now = time.Now

	if err := s.DeleteSession(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserBySession(ctx, token); !errors.Is(err, ErrNoSession) {
		t.Error("sesi yang dihapus harus ditolak")
	}
}

func TestLoginRateLimit(t *testing.T) {
	s := newService(t, false)
	ctx := context.Background()
	if _, err := s.Register(ctx, "a@contoh.id", goodPassword, ""); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		s.Login(ctx, "a@contoh.id", "salah-salah-salah", "1.2.3.4")
	}
	if _, err := s.Login(ctx, "a@contoh.id", goodPassword, "5.6.7.8"); !errors.Is(err, ErrTooManyAttempts) {
		t.Errorf("setelah 5 kegagalan, login harus dibatasi walau password benar: got %v", err)
	}
}

func TestPurgeExpired(t *testing.T) {
	s := newService(t, true)
	ctx := context.Background()
	code, _ := s.CreateInvite(ctx, time.Hour)
	used, _ := s.CreateInvite(ctx, time.Hour)
	id, err := s.Register(ctx, "a@contoh.id", goodPassword, used)
	if err != nil {
		t.Fatal(err)
	}
	token, _, _ := s.CreateSession(ctx, id)

	s.now = func() time.Time { return time.Now().Add(SessionTTL + 2*time.Hour) }
	if err := s.PurgeExpired(ctx); err != nil {
		t.Fatal(err)
	}
	s.now = time.Now

	var sessions, invites int
	s.db.QueryRow(ctx, `SELECT count(*) FROM sessions`).Scan(&sessions)
	s.db.QueryRow(ctx, `SELECT count(*) FROM invites`).Scan(&invites)
	if sessions != 0 {
		t.Errorf("sesi kedaluwarsa masih ada: %d", sessions)
	}
	if invites != 1 {
		t.Errorf("hanya undangan terpakai yang disimpan, ada %d", invites)
	}
	if _, err := s.UserBySession(ctx, token); !errors.Is(err, ErrNoSession) {
		t.Error("sesi yang dibersihkan harus ditolak")
	}
	if _, err := s.Register(ctx, "b@contoh.id", goodPassword, code); !errors.Is(err, ErrInviteInvalid) {
		t.Error("undangan kedaluwarsa yang dibersihkan tidak boleh berlaku")
	}
}

func TestAllowSignupLimit(t *testing.T) {
	s, err := NewService(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := s.AllowSignup("1.2.3.4"); err != nil {
			t.Fatalf("pendaftaran ke-%d ditolak: %v", i+1, err)
		}
	}
	if err := s.AllowSignup("1.2.3.4"); !errors.Is(err, ErrTooManySignups) {
		t.Errorf("pendaftaran ke-6 dalam satu jam harus ditolak: %v", err)
	}
	if err := s.AllowSignup("5.6.7.8"); err != nil {
		t.Errorf("IP lain tidak boleh ikut terblokir: %v", err)
	}
	s.now = func() time.Time { return time.Now().Add(61 * time.Minute) }
	if err := s.AllowSignup("1.2.3.4"); err != nil {
		t.Errorf("setelah satu jam harus boleh lagi: %v", err)
	}
}
