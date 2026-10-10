package web

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

const testPassword = "kuda-laut-biru-42"

// signUp mendaftar lewat HTTP (pendaftaran terbuka) dan mengembalikan token CSRF.
func (f *flow) signUp(email string) string {
	f.t.Helper()
	_, _, token := f.get("/daftar")
	resp := f.post("/daftar", url.Values{
		"csrf": {token}, "email": {email}, "password": {testPassword}, "password_confirm": {testPassword}, "setuju": {"on"},
	}, f.srv.URL)
	if resp.StatusCode != http.StatusSeeOther {
		f.t.Fatalf("daftar: status %d", resp.StatusCode)
	}
	_, _, token = f.get("/beranda")
	if token == "" {
		f.t.Fatal("token CSRF tidak ditemukan di beranda")
	}
	return token
}

func (f *flow) api(method, path, csrf string, body any) (int, map[string]any) {
	f.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, f.srv.URL+path, r)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", f.srv.URL)
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	resp, err := f.client.Do(req)
	if err != nil {
		f.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

func b64(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func validMaterial() map[string]any {
	return map[string]any{
		"version": 1, "kdf": "PBKDF2-SHA256", "iterations": 600000,
		"salt": b64(16), "wrappedKey": b64(48), "wrapIv": b64(12),
		"verifier": b64(40), "verifierIv": b64(12),
	}
}

func TestJournalKeyAPI(t *testing.T) {
	f := newFlow(t, false)

	if code, _ := f.api("GET", "/api/jurnal/kunci", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("tanpa login: status %d, mau 401", code)
	}

	csrf := f.signUp("a@contoh.id")

	if code, _, _ := f.get("/jurnal/buka"); code != http.StatusSeeOther {
		t.Errorf("/jurnal/buka sebelum kunci dibuat harus redirect ke /jurnal/buat, status %d", code)
	}
	if code, _ := f.api("GET", "/api/jurnal/kunci", csrf, nil); code != http.StatusNotFound {
		t.Errorf("GET sebelum dibuat: status %d", code)
	}
	if code, _ := f.api("POST", "/api/jurnal/kunci", "", validMaterial()); code != http.StatusForbidden {
		t.Errorf("POST tanpa CSRF: status %d, mau 403", code)
	}

	m := validMaterial()
	if code, out := f.api("POST", "/api/jurnal/kunci", csrf, m); code != http.StatusCreated {
		t.Fatalf("membuat kunci: status %d %v", code, out)
	}
	if code, _ := f.api("POST", "/api/jurnal/kunci", csrf, validMaterial()); code != http.StatusConflict {
		t.Errorf("kunci yang ada tidak boleh ditimpa: status %d", code)
	}

	code, got := f.api("GET", "/api/jurnal/kunci", csrf, nil)
	if code != http.StatusOK || got["wrappedKey"] != m["wrappedKey"] || got["verifier"] != m["verifier"] {
		t.Fatalf("GET kunci: status %d %v", code, got)
	}

	if code, _, _ := f.get("/jurnal/buat"); code != http.StatusSeeOther {
		t.Errorf("/jurnal/buat setelah kunci ada harus redirect, status %d", code)
	}
	if code, body, _ := f.get("/jurnal/buka"); code != http.StatusOK || !strings.Contains(body, `data-form="journal-unlock"`) {
		t.Errorf("/jurnal/buka: status %d", code)
	}

	newWrap := map[string]any{
		"kdf": "PBKDF2-SHA256", "iterations": 600000,
		"salt": b64(16), "wrappedKey": b64(48), "wrapIv": b64(12),
	}
	wrong := map[string]any{"loginPassword": "salah-salah-salah"}
	for k, v := range newWrap {
		wrong[k] = v
	}
	if code, _ := f.api("PUT", "/api/jurnal/kunci", csrf, wrong); code != http.StatusForbidden {
		t.Errorf("ganti passphrase dengan password login salah: status %d, mau 403", code)
	}

	right := map[string]any{"loginPassword": testPassword}
	for k, v := range newWrap {
		right[k] = v
	}
	if code, out := f.api("PUT", "/api/jurnal/kunci", csrf, right); code != http.StatusNoContent {
		t.Fatalf("ganti passphrase: status %d %v", code, out)
	}
	_, got = f.api("GET", "/api/jurnal/kunci", csrf, nil)
	if got["wrappedKey"] != newWrap["wrappedKey"] || got["salt"] != newWrap["salt"] {
		t.Error("bungkus baru tidak tersimpan")
	}
	if got["verifier"] != m["verifier"] {
		t.Error("nilai uji tidak boleh berubah saat ganti passphrase")
	}
}

func TestJournalKeyAPIRejectsPlaintextAndBadInput(t *testing.T) {
	f := newFlow(t, false)
	csrf := f.signUp("a@contoh.id")

	withPassphrase := validMaterial()
	withPassphrase["passphrase"] = "kopi jendela hujan sepeda"
	if code, _ := f.api("POST", "/api/jurnal/kunci", csrf, withPassphrase); code != http.StatusBadRequest {
		t.Errorf("field passphrase harus ditolak: status %d", code)
	}
	withRawKey := validMaterial()
	withRawKey["dek"] = b64(32)
	if code, _ := f.api("POST", "/api/jurnal/kunci", csrf, withRawKey); code != http.StatusBadRequest {
		t.Errorf("field kunci mentah harus ditolak: status %d", code)
	}

	weak := validMaterial()
	weak["iterations"] = 10000
	if code, _ := f.api("POST", "/api/jurnal/kunci", csrf, weak); code != http.StatusBadRequest {
		t.Errorf("iterasi rendah harus ditolak: status %d", code)
	}
	short := validMaterial()
	short["salt"] = b64(4)
	if code, _ := f.api("POST", "/api/jurnal/kunci", csrf, short); code != http.StatusBadRequest {
		t.Errorf("salt pendek harus ditolak: status %d", code)
	}

	// Setelah semua penolakan, kunci belum pernah tersimpan.
	if code, _ := f.api("GET", "/api/jurnal/kunci", csrf, nil); code != http.StatusNotFound {
		t.Errorf("tidak boleh ada kunci tersimpan, status %d", code)
	}
}

func TestJournalPagesHaveNoNamedSecretInputs(t *testing.T) {
	f := newFlow(t, false)
	csrf := f.signUp("a@contoh.id")

	check := func(path string) {
		t.Helper()
		code, body, _ := f.get(path)
		start := strings.Index(body, "data-form=")
		if code != http.StatusOK || start < 0 {
			t.Fatalf("%s: status %d, form jurnal tidak ditemukan", path, code)
		}
		form := body[start:]
		form = form[:strings.Index(form, "</form>")]
		if strings.Contains(form, " name=") {
			t.Errorf("%s: input di form jurnal tidak boleh punya atribut name (agar tidak pernah terkirim)", path)
		}
	}

	check("/jurnal/buat")
	if code, out := f.api("POST", "/api/jurnal/kunci", csrf, validMaterial()); code != http.StatusCreated {
		t.Fatalf("membuat kunci: %d %v", code, out)
	}
	check("/jurnal/buka")
	check("/jurnal/ganti")
}
