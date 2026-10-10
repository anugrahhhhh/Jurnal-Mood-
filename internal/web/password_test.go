package web

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestForcedPasswordChange(t *testing.T) {
	f := newFlow(t, false)
	csrf := f.signUp("a@contoh.id")
	f.api("POST", "/api/jurnal/kunci", csrf, validMaterial())

	temp, err := f.auth.SetTemporaryPassword(context.Background(), "a@contoh.id")
	if err != nil {
		t.Fatal(err)
	}
	if code, _, _ := f.get("/beranda"); code != http.StatusSeeOther {
		t.Fatalf("sesi lama harus berakhir setelah reset, status %d", code)
	}

	_, _, token := f.get("/masuk")
	resp := f.post("/masuk", url.Values{"csrf": {token}, "email": {"a@contoh.id"}, "password": {temp}}, f.srv.URL)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("masuk dengan password sementara: %d", resp.StatusCode)
	}
	for _, p := range []string{"/beranda", "/riwayat", "/pengaturan", "/hari-ini"} {
		r, err := f.client.Get(f.srv.URL + p)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != http.StatusSeeOther || r.Header.Get("Location") != "/akun/password" {
			t.Errorf("%s harus diarahkan ke /akun/password, dapat %d %q", p, r.StatusCode, r.Header.Get("Location"))
		}
	}
	code, body, csrf := f.get("/akun/password")
	if code != http.StatusOK || !strings.Contains(body, "password sementara") {
		t.Fatalf("/akun/password: %d", code)
	}
	if c, _ := f.api("GET", "/api/entri?dari=2026-01-01&sampai=2026-01-02", csrf, nil); c != http.StatusForbidden {
		t.Errorf("API harus ditolak selama wajib ganti password: %d", c)
	}
	if code, _, _ := f.get("/bantuan"); code != http.StatusOK {
		t.Errorf("halaman bantuan krisis harus tetap bisa dibuka: %d", code)
	}

	bad := f.post("/akun/password", url.Values{"csrf": {csrf}, "current_password": {"salah-salah-salah"},
		"new_password": {"password-baru-123"}, "new_password_confirm": {"password-baru-123"}}, f.srv.URL)
	if bad.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("password sementara salah: %d", bad.StatusCode)
	}
	mismatch := f.post("/akun/password", url.Values{"csrf": {csrf}, "current_password": {temp},
		"new_password": {"password-baru-123"}, "new_password_confirm": {"password-lain-123"}}, f.srv.URL)
	if mismatch.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("konfirmasi tidak sama: %d", mismatch.StatusCode)
	}
	ok := f.post("/akun/password", url.Values{"csrf": {csrf}, "current_password": {temp},
		"new_password": {"password-baru-123"}, "new_password_confirm": {"password-baru-123"}}, f.srv.URL)
	if ok.StatusCode != http.StatusSeeOther || ok.Header.Get("Location") != "/pengaturan?password=diganti" {
		t.Fatalf("ganti password: %d %q", ok.StatusCode, ok.Header.Get("Location"))
	}
	code, body, _ = f.get("/pengaturan?password=diganti")
	if code != http.StatusOK || !strings.Contains(body, "berhasil diganti") {
		t.Errorf("pengaturan setelah ganti password: %d", code)
	}
	if code, _, _ := f.get("/beranda"); code != http.StatusOK {
		t.Errorf("setelah ganti password, beranda harus bisa dibuka: %d", code)
	}
}

func TestVoluntaryPasswordChangeKeepsJournal(t *testing.T) {
	f := newFlow(t, false)
	csrf := f.signUp("a@contoh.id")
	m := validMaterial()
	f.api("POST", "/api/jurnal/kunci", csrf, m)

	code, body, csrf := f.get("/akun/password")
	if code != http.StatusOK || !strings.Contains(body, "Password saat ini") {
		t.Fatalf("/akun/password: %d", code)
	}
	ok := f.post("/akun/password", url.Values{"csrf": {csrf}, "current_password": {testPassword},
		"new_password": {"password-baru-123"}, "new_password_confirm": {"password-baru-123"}}, f.srv.URL)
	if ok.StatusCode != http.StatusSeeOther {
		t.Fatalf("ganti password: %d", ok.StatusCode)
	}
	_, got := f.api("GET", "/api/jurnal/kunci", csrf, nil)
	if got["wrappedKey"] != m["wrappedKey"] {
		t.Error("ganti password login tidak boleh mengubah kunci jurnal")
	}
}
