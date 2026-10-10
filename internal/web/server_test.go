package web

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"moodjournal/internal/auth"
	"moodjournal/internal/config"
	"moodjournal/internal/entries"
	"moodjournal/internal/journal"
	"moodjournal/internal/reminders"
	assets "moodjournal/web"
)

type fakeDB struct{ err error }

func (f fakeDB) Ping(context.Context) error { return f.err }

func testCrisis(verified bool) *config.Crisis {
	return &config.Crisis{
		Region:   "Indonesia",
		Verified: verified,
		Emergency: []config.CrisisContact{
			{Name: "Nomor darurat nasional", Phone: "112"},
		},
		Counseling: []config.CrisisContact{
			{Name: "Layanan konseling", Phone: "119 ext 8", Dial: "119"},
			{Name: "Puskesmas terdekat", Note: "Tanpa nomor"},
		},
	}
}

// newTestServer membuat server tanpa database sungguhan. Cukup untuk halaman
// yang tidak memerlukan sesi; alur akun diuji di auth_flow_test.go.
func newTestServer(t *testing.T, env string, db Pinger, crisis *config.Crisis) http.Handler {
	t.Helper()
	authSvc, err := auth.NewService(nil, true)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(config.Config{Env: env}, Deps{
		DB: db, Auth: authSvc, Journal: journal.NewStore(nil), Entries: entries.NewStore(nil), Reminders: reminders.NewStore(nil), Crisis: crisis, Assets: assets.FS,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s.Routes()
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func TestSecurityHeaders(t *testing.T) {
	h := newTestServer(t, "development", fakeDB{}, testCrisis(true))
	rec := get(h, "/bantuan")

	csp := rec.Header().Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'self'", "script-src 'self'", "object-src 'none'", "frame-ancestors 'none'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP tidak memuat %q: %s", want, csp)
		}
	}
	if strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
		t.Errorf("CSP tidak boleh mengizinkan inline/eval: %s", csp)
	}
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "same-origin",
	} {
		if got := rec.Header().Get(header); got != want {
			t.Errorf("%s = %q, mau %q", header, got, want)
		}
	}
	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Error("HSTS tidak boleh aktif di development")
	}
}

func TestHSTSInProduction(t *testing.T) {
	h := newTestServer(t, "production", fakeDB{}, testCrisis(true))
	if get(h, "/").Header().Get("Strict-Transport-Security") == "" {
		t.Error("HSTS harus aktif di production")
	}
}

func TestCrisisLinkOnEveryPage(t *testing.T) {
	h := newTestServer(t, "development", fakeDB{}, testCrisis(true))
	for _, path := range []string{"/", "/bantuan", "/masuk", "/daftar", "/tidak-ada"} {
		body := get(h, path).Body.String()
		if !strings.Contains(body, `href="/bantuan"`) {
			t.Errorf("%s: tautan bantuan krisis tidak ada", path)
		}
		if !strings.Contains(body, "bukan layanan darurat") {
			t.Errorf("%s: disclaimer tidak ada", path)
		}
	}
}

func TestHelpPage(t *testing.T) {
	h := newTestServer(t, "development", fakeDB{}, testCrisis(true))
	rec := get(h, "/bantuan")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Nomor darurat nasional",
		`href="tel:112"`,
		`href="tel:119"`, // memakai Dial, bukan teks "119 ext 8"
		"119 ext 8",
		"Tanpa nomor",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("halaman bantuan tidak memuat %q", want)
		}
	}
	if strings.Contains(body, "belum diverifikasi") {
		t.Error("peringatan verifikasi muncul padahal Verified = true")
	}
}

func TestHelpPageWarnsWhenUnverified(t *testing.T) {
	h := newTestServer(t, "development", fakeDB{}, testCrisis(false))
	if !strings.Contains(get(h, "/bantuan").Body.String(), "belum diverifikasi") {
		t.Error("peringatan verifikasi harus muncul saat Verified = false")
	}
}

func TestHealth(t *testing.T) {
	ok := newTestServer(t, "development", fakeDB{}, testCrisis(true))
	if rec := get(ok, "/healthz"); rec.Code != http.StatusOK {
		t.Errorf("healthz saat DB sehat: status %d", rec.Code)
	}
	down := newTestServer(t, "development", fakeDB{err: errors.New("down")}, testCrisis(true))
	if rec := get(down, "/healthz"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("healthz saat DB mati: status %d", rec.Code)
	}
}

func TestNotFound(t *testing.T) {
	h := newTestServer(t, "development", fakeDB{}, testCrisis(true))
	if rec := get(h, "/tidak-ada"); rec.Code != http.StatusNotFound {
		t.Errorf("status %d, mau 404", rec.Code)
	}
}

func TestStaticCSS(t *testing.T) {
	h := newTestServer(t, "development", fakeDB{}, testCrisis(true))
	rec := get(h, "/static/css/app.css")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestTelURL(t *testing.T) {
	cases := map[string]string{
		"112":              "tel:112",
		"119 ext 8":        "tel:1198", // itulah sebabnya Dial dipakai untuk nomor berekstensi
		"+62 811-000":      "tel:+62811000",
		"javascript:alert": "",
		"":                 "",
	}
	for in, want := range cases {
		if got := string(telURL(in)); got != want {
			t.Errorf("telURL(%q) = %q, mau %q", in, got, want)
		}
	}
}

func TestClientIP(t *testing.T) {
	req := func(remote string, xff ...string) *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = remote
		for _, v := range xff {
			r.Header.Add("X-Forwarded-For", v)
		}
		return r
	}
	direct := &Server{cfg: config.Config{}}
	if got := direct.clientIP(req("203.0.113.7:5555", "1.1.1.1")); got != "203.0.113.7" {
		t.Errorf("tanpa TRUST_PROXY, X-Forwarded-For harus diabaikan: %s", got)
	}

	proxied := &Server{cfg: config.Config{TrustProxy: true}}
	cases := []struct {
		name string
		r    *http.Request
		want string
	}{
		{"satu hop", req("172.18.0.3:4000", "198.51.100.9"), "198.51.100.9"},
		{"entri palsu dari klien diabaikan", req("172.18.0.3:4000", "6.6.6.6, 198.51.100.9"), "198.51.100.9"},
		{"beberapa header: pakai yang terakhir", req("172.18.0.3:4000", "6.6.6.6", "198.51.100.9"), "198.51.100.9"},
		{"header rusak: alamat koneksi", req("172.18.0.3:4000", "bukan-ip"), "172.18.0.3"},
		{"tanpa header: alamat koneksi", req("172.18.0.3:4000"), "172.18.0.3"},
	}
	for _, c := range cases {
		if got := proxied.clientIP(c.r); got != c.want {
			t.Errorf("%s: %s, mau %s", c.name, got, c.want)
		}
	}
}
