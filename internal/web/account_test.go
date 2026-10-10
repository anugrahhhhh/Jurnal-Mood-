package web

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestPrivacyPagePublicAndHonest(t *testing.T) {
	h := newTestServer(t, "development", fakeDB{}, testCrisis(true))
	rec := get(h, "/privasi")
	if rec.Code != http.StatusOK {
		t.Fatalf("/privasi tanpa login: status %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Skor mood", "jam tidur", "Tanggal", "Ukuran", "Alamat email", // yang terlihat pengelola
		"Teks catatan", "Aktivitas", // yang terenkripsi
		"Kode enkripsi dikirim oleh server ini", // batasan JavaScript
		"tidak bisa dipulihkan",
		"tanpa alamat IP",
		`href="/bantuan"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("halaman privasi tidak memuat %q", want)
		}
	}
	for _, path := range []string{"/", "/bantuan", "/masuk", "/daftar"} {
		if !strings.Contains(get(h, path).Body.String(), `href="/privasi"`) {
			t.Errorf("%s: tautan privasi tidak ada", path)
		}
	}
}

func TestAccessLogHasNoIPOrQuery(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	h := newTestServer(t, "development", fakeDB{}, testCrisis(true))
	req := httptest.NewRequest(http.MethodGet, "/bantuan?rahasia=1", nil)
	req.RemoteAddr = "203.0.113.7:5555"
	req.Header.Set("User-Agent", "AgenUji/1.0")
	h.ServeHTTP(httptest.NewRecorder(), req)

	logged := buf.String()
	if !strings.Contains(logged, "path=/bantuan") {
		t.Fatalf("log akses tidak tercatat: %s", logged)
	}
	for _, leak := range []string{"203.0.113.7", "rahasia", "AgenUji"} {
		if strings.Contains(logged, leak) {
			t.Errorf("log akses memuat %q: %s", leak, logged)
		}
	}
}

func countRows(t *testing.T, f *flow, table string, userID int64) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(),
		"SELECT count(*) FROM "+table+" WHERE user_id = $1", userID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func userID(t *testing.T, f *flow, email string) int64 {
	t.Helper()
	var id int64
	if err := f.pool.QueryRow(context.Background(), "SELECT id FROM users WHERE email = $1", email).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestDeleteAccount(t *testing.T) {
	f := newFlow(t, false)
	csrf := f.signUp("a@contoh.id")
	f.api("POST", "/api/jurnal/kunci", csrf, validMaterial())
	f.api("PUT", "/api/entri/"+today(), csrf, validEntry())
	f.api("PUT", "/api/aktivitas", csrf, map[string]any{"ct": b64(60), "iv": b64(12)})
	a := userID(t, f, "a@contoh.id")

	other := newFlowSharedDB(t, f)
	csrf2 := other.signUp("b@contoh.id")
	other.api("PUT", "/api/entri/"+today(), csrf2, validEntry())
	b := userID(t, f, "b@contoh.id")

	if code, body, _ := f.get("/pengaturan"); code != http.StatusOK || !strings.Contains(body, `action="/akun/hapus"`) {
		t.Fatalf("/pengaturan: status %d", code)
	}

	if resp := f.post("/akun/hapus", url.Values{"password": {testPassword}, "confirm": {"HAPUS"}}, f.srv.URL); resp.StatusCode != http.StatusForbidden {
		t.Errorf("tanpa CSRF: status %d, mau 403", resp.StatusCode)
	}
	if resp := f.post("/akun/hapus", url.Values{"csrf": {csrf}, "password": {testPassword}, "confirm": {"hapus"}}, f.srv.URL); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("konfirmasi salah: status %d, mau 422", resp.StatusCode)
	}
	if resp := f.post("/akun/hapus", url.Values{"csrf": {csrf}, "password": {"salah-salah-salah"}, "confirm": {"HAPUS"}}, f.srv.URL); resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("password salah: status %d, mau 422", resp.StatusCode)
	}
	if countRows(t, f, "entries", a) != 1 {
		t.Fatal("akun tidak boleh terhapus sebelum konfirmasi yang benar")
	}

	resp := f.post("/akun/hapus", url.Values{"csrf": {csrf}, "password": {testPassword}, "confirm": {"HAPUS"}}, f.srv.URL)
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(resp.Header.Get("Location"), "/akun-dihapus?u=") {
		t.Fatalf("hapus akun: status %d lokasi %q", resp.StatusCode, resp.Header.Get("Location"))
	}

	for _, table := range []string{"entries", "journal_keys", "user_settings", "sessions"} {
		if n := countRows(t, f, table, a); n != 0 {
			t.Errorf("%s: masih ada %d baris milik akun yang dihapus", table, n)
		}
	}
	var users int
	f.pool.QueryRow(context.Background(), "SELECT count(*) FROM users WHERE id = $1", a).Scan(&users)
	if users != 0 {
		t.Error("baris users masih ada")
	}
	if code, _, _ := f.get("/beranda"); code != http.StatusSeeOther {
		t.Error("setelah akun dihapus, sesi harus tidak berlaku")
	}
	if _, err := f.auth.Login(context.Background(), "a@contoh.id", testPassword, "1.2.3.4"); err == nil {
		t.Error("login dengan akun yang dihapus harus gagal")
	}

	if countRows(t, f, "entries", b) != 1 {
		t.Error("data pengguna lain tidak boleh ikut terhapus")
	}

	code, body, _ := f.get(resp.Header.Get("Location"))
	if code != http.StatusOK || !strings.Contains(body, `data-clear-user="`) {
		t.Errorf("halaman akun dihapus: status %d", code)
	}
}

var inlineStyleOrScript = regexp.MustCompile(`\sstyle="|<script(?:\s[^>]*)?>[^<]|\son[a-z]+="`)

// Semua halaman harus lolos CSP ketat: tanpa atribut style, skrip inline,
// atau handler on*="..." (CSP memblokirnya diam-diam di browser).
func TestNoInlineStyleOrScript(t *testing.T) {
	f := newFlow(t, false)
	pages := []string{"/", "/masuk", "/daftar", "/bantuan", "/privasi", "/tidak-ada"}
	check := func(path string) {
		t.Helper()
		_, body, _ := f.get(path)
		if m := inlineStyleOrScript.FindString(body); m != "" {
			t.Errorf("%s: memuat %q yang diblokir CSP", path, m)
		}
	}
	for _, p := range pages {
		check(p)
	}
	csrf := f.signUp("a@contoh.id")
	check("/jurnal/buat")
	f.api("POST", "/api/jurnal/kunci", csrf, validMaterial())
	for _, p := range []string{"/beranda", "/hari-ini", "/riwayat", "/pengaturan", "/jurnal/buka", "/jurnal/ganti", "/bantuan", "/privasi"} {
		check(p)
	}
}

func TestNavMarksActivePage(t *testing.T) {
	f := newFlow(t, false)
	csrf := f.signUp("a@contoh.id")
	f.api("POST", "/api/jurnal/kunci", csrf, validMaterial())
	for path, href := range map[string]string{"/beranda": "/beranda", "/riwayat": "/riwayat", "/pengaturan": "/pengaturan", "/hari-ini": "/hari-ini"} {
		_, body, _ := f.get(path)
		if !strings.Contains(body, `href="`+href+`" aria-current="page"`) && !strings.Contains(body, `href="`+href+`" class="tab" aria-current="page"`) &&
			!strings.Contains(body, `href="`+href+`" class="tab tab-fab" aria-label="Isi catatan hari ini" aria-current="page"`) {
			t.Errorf("%s: menu %s tidak ditandai aktif", path, href)
		}
		if !strings.Contains(body, `class="sidebar"`) || !strings.Contains(body, `class="tabbar"`) {
			t.Errorf("%s: sidebar dan tab bar harus ada (CSS memilih yang tampil per ukuran layar)", path)
		}
	}
	_, body, _ := f.get("/beranda")
	if !strings.Contains(body, `data-page="dashboard"`) {
		t.Error("beranda harus berisi dashboard setelah kunci jurnal dibuat")
	}
}
