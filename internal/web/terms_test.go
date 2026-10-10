package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"moodjournal/internal/auth"
	"moodjournal/internal/config"
	"moodjournal/internal/entries"
	"moodjournal/internal/journal"
	"moodjournal/internal/reminders"
	assets "moodjournal/web"
)

func TestSignupRequiresConsentAndRecordsIt(t *testing.T) {
	f := newFlow(t, false)
	_, body, token := f.get("/daftar")
	if !strings.Contains(body, `name="setuju"`) || !strings.Contains(body, "minimal <strong>18 tahun</strong>") {
		t.Fatal("halaman daftar harus memuat kotak persetujuan dengan usia minimum")
	}
	form := url.Values{"csrf": {token}, "email": {"a@contoh.id"}, "password": {testPassword}, "password_confirm": {testPassword}}
	if resp := f.post("/daftar", form, f.srv.URL); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("daftar tanpa persetujuan: status %d, mau 422", resp.StatusCode)
	}
	var n int
	f.pool.QueryRow(context.Background(), `SELECT count(*) FROM users`).Scan(&n)
	if n != 0 {
		t.Fatal("akun tidak boleh dibuat tanpa persetujuan")
	}

	f.signUp("a@contoh.id")
	var accepted bool
	f.pool.QueryRow(context.Background(), `SELECT terms_accepted_at IS NOT NULL FROM users WHERE email = 'a@contoh.id'`).Scan(&accepted)
	if !accepted {
		t.Error("waktu persetujuan harus dicatat")
	}
}

func TestTermsAndOperatorInfo(t *testing.T) {
	// Tanpa identitas pengelola: halaman memperingatkan.
	h := newTestServer(t, "development", fakeDB{}, testCrisis(true))
	for _, p := range []string{"/syarat", "/privasi"} {
		rec := get(h, p)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "belum diisi") {
			t.Errorf("%s tanpa identitas pengelola harus memperingatkan (status %d)", p, rec.Code)
		}
	}
	if body := get(h, "/").Body.String(); !strings.Contains(body, `href="/syarat"`) {
		t.Error("tautan Syarat harus ada di footer")
	}

	// Dengan identitas pengelola dan usia minimum khusus.
	authSvc, _ := auth.NewService(nil, false)
	s, err := New(config.Config{Env: "development", OperatorName: "Komunitas Tenang", OperatorContact: "halo@tenang.id", MinAge: 21},
		Deps{DB: fakeDB{}, Auth: authSvc, Journal: journal.NewStore(nil), Entries: entries.NewStore(nil),
			Reminders: reminders.NewStore(nil), Crisis: testCrisis(true), Assets: assets.FS})
	if err != nil {
		t.Fatal(err)
	}
	h = s.Routes()
	for _, p := range []string{"/syarat", "/privasi", "/masuk"} {
		body := get(h, p).Body.String()
		if !strings.Contains(body, "Komunitas Tenang") || !strings.Contains(body, "halo@tenang.id") {
			t.Errorf("%s harus menampilkan identitas pengelola", p)
		}
		if strings.Contains(body, "belum diisi di konfigurasi") {
			t.Errorf("%s tidak boleh memperingatkan bila identitas sudah diisi", p)
		}
	}
	if body := get(h, "/syarat").Body.String(); !strings.Contains(body, "21 tahun") {
		t.Error("usia minimum dari konfigurasi harus tampil")
	}
}
