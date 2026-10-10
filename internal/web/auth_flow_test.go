package web

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"moodjournal/internal/auth"
	"moodjournal/internal/config"
	"moodjournal/internal/entries"
	"moodjournal/internal/journal"
	"moodjournal/internal/reminders"
	"moodjournal/internal/testdb"
	assets "moodjournal/web"
)

type flow struct {
	t      *testing.T
	srv    *httptest.Server
	client *http.Client
	auth   *auth.Service
	pool   *pgxpool.Pool
}

func newFlow(t *testing.T, inviteRequired bool) *flow {
	t.Helper()
	pool := testdb.Open(t)
	authSvc, err := auth.NewService(pool, inviteRequired)
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(config.Config{Env: "development", InviteRequired: inviteRequired},
		Deps{DB: pool, Auth: authSvc, Journal: journal.NewStore(pool), Entries: entries.NewStore(pool), Reminders: reminders.NewStore(pool), RemindersEnabled: true, VAPIDPublicKey: testVAPIDPublicKey, Crisis: testCrisis(true), Assets: assets.FS})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.Routes())
	t.Cleanup(srv.Close)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse // periksa redirect secara manual
		},
	}
	return &flow{t: t, srv: srv, client: client, auth: authSvc, pool: pool}
}

// newFlowSharedDB membuat klien kedua (cookie terpisah) ke server yang sama,
// untuk menguji bahwa data satu pengguna tidak terlihat oleh pengguna lain.
func newFlowSharedDB(t *testing.T, f *flow) *flow {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Jar:           jar,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &flow{t: t, srv: f.srv, client: client, auth: f.auth, pool: f.pool}
}

var csrfRe = regexp.MustCompile(`name="csrf" value="([^"]+)"`)

// get mengembalikan status, body, dan token CSRF (jika ada di halaman).
func (f *flow) get(path string) (int, string, string) {
	f.t.Helper()
	resp, err := f.client.Get(f.srv.URL + path)
	if err != nil {
		f.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	token := ""
	if m := csrfRe.FindSubmatch(b); m != nil {
		token = string(m[1])
	}
	return resp.StatusCode, string(b), token
}

func (f *flow) post(path string, form url.Values, origin string) *http.Response {
	f.t.Helper()
	req, _ := http.NewRequest(http.MethodPost, f.srv.URL+path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp
}

func TestRegisterLoginLogoutFlow(t *testing.T) {
	f := newFlow(t, true)
	invite, err := f.auth.CreateInvite(context.Background(), time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if code, _, _ := f.get("/beranda"); code != http.StatusSeeOther {
		t.Fatalf("tamu membuka /beranda: status %d, mau redirect", code)
	}

	_, body, token := f.get("/daftar")
	if token == "" || !strings.Contains(body, `name="invite"`) {
		t.Fatal("form daftar harus punya token CSRF dan kolom kode undangan")
	}

	resp := f.post("/daftar", url.Values{
		"csrf": {token}, "invite": {invite}, "email": {"teman@contoh.id"},
		"password": {"kuda-laut-biru-42"}, "password_confirm": {"kuda-laut-biru-42"}, "setuju": {"on"},
	}, f.srv.URL)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/jurnal/buat" {
		t.Fatalf("daftar: status %d lokasi %q (mau langsung ke langkah passphrase)", resp.StatusCode, resp.Header.Get("Location"))
	}

	code, body, token := f.get("/beranda")
	if code != http.StatusOK || !strings.Contains(body, "teman@contoh.id") {
		t.Fatalf("beranda setelah daftar: status %d", code)
	}
	if !strings.Contains(body, `action="/keluar"`) {
		t.Error("tombol keluar harus tampil untuk pengguna yang masuk")
	}

	if code, _, _ := f.get("/masuk"); code != http.StatusSeeOther {
		t.Errorf("pengguna yang sudah masuk membuka /masuk: status %d, mau redirect", code)
	}

	resp = f.post("/keluar", url.Values{"csrf": {token}}, f.srv.URL)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("keluar: status %d", resp.StatusCode)
	}
	if code, _, _ := f.get("/beranda"); code != http.StatusSeeOther {
		t.Error("setelah keluar, /beranda harus redirect ke halaman masuk")
	}

	_, _, token = f.get("/masuk")
	resp = f.post("/masuk", url.Values{"csrf": {token}, "email": {"teman@contoh.id"}, "password": {"salah-salah-salah"}}, f.srv.URL)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("password salah: status %d", resp.StatusCode)
	}
	resp = f.post("/masuk", url.Values{"csrf": {token}, "email": {"teman@contoh.id"}, "password": {"kuda-laut-biru-42"}}, f.srv.URL)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/beranda" {
		t.Fatalf("masuk: status %d", resp.StatusCode)
	}
	if code, _, _ := f.get("/beranda"); code != http.StatusOK {
		t.Error("setelah masuk, /beranda harus bisa dibuka")
	}
}

func TestCSRFProtection(t *testing.T) {
	f := newFlow(t, false)
	_, _, token := f.get("/masuk")
	creds := url.Values{"email": {"a@contoh.id"}, "password": {"kuda-laut-biru-42"}}

	if resp := f.post("/masuk", creds, f.srv.URL); resp.StatusCode != http.StatusForbidden {
		t.Errorf("tanpa token CSRF: status %d, mau 403", resp.StatusCode)
	}

	withBadToken := url.Values{"csrf": {"token-palsu-token-palsu-token-palsu-xx"}}
	for k, v := range creds {
		withBadToken[k] = v
	}
	if resp := f.post("/masuk", withBadToken, f.srv.URL); resp.StatusCode != http.StatusForbidden {
		t.Errorf("token CSRF salah: status %d, mau 403", resp.StatusCode)
	}

	withToken := url.Values{"csrf": {token}}
	for k, v := range creds {
		withToken[k] = v
	}
	if resp := f.post("/masuk", withToken, "https://situs-jahat.example"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("origin lain: status %d, mau 403", resp.StatusCode)
	}
	// "Origin: null" (mis. dari iframe sandbox) ditolak. Agar browser tidak
	// mengirim null untuk form situs sendiri, Referrer-Policy harus same-origin.
	if resp := f.post("/masuk", withToken, "null"); resp.StatusCode != http.StatusForbidden {
		t.Errorf("origin null: status %d, mau 403", resp.StatusCode)
	}
}

func TestRegisterFormErrorsKeepEmailNotPassword(t *testing.T) {
	f := newFlow(t, true)
	_, _, token := f.get("/daftar")
	form := url.Values{
		"csrf": {token}, "invite": {"SALAH-SALA-HSAL"}, "email": {"teman@contoh.id"},
		"password": {"rahasia-sekali-123"}, "password_confirm": {"rahasia-sekali-123"}, "setuju": {"on"},
	}
	req, _ := http.NewRequest(http.MethodPost, f.srv.URL+"/daftar", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := f.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	body := string(b)

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status %d, mau 422", resp.StatusCode)
	}
	if !strings.Contains(body, auth.ErrInviteInvalid.Error()) {
		t.Error("pesan kode undangan tidak valid harus tampil")
	}
	if !strings.Contains(body, `value="teman@contoh.id"`) {
		t.Error("email harus diisi ulang")
	}
	if strings.Contains(body, "rahasia-sekali-123") {
		t.Error("password tidak boleh dikembalikan ke halaman")
	}
}
