package web

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func validRecoveryJSON() map[string]any {
	return map[string]any{"iterations": 100000, "salt": b64(16), "wrappedKey": b64(48), "wrapIv": b64(12)}
}

func TestRecoveryAPI(t *testing.T) {
	f := newFlow(t, false)
	csrf := f.signUp("a@contoh.id")

	m := validMaterial()
	m["recovery"] = validRecoveryJSON()
	if code, out := f.api("POST", "/api/jurnal/kunci", csrf, m); code != http.StatusCreated {
		t.Fatalf("membuat kunci dengan pemulihan: %d %v", code, out)
	}
	_, got := f.api("GET", "/api/jurnal/kunci", csrf, nil)
	rec, _ := got["recovery"].(map[string]any)
	if rec == nil || rec["wrappedKey"] != m["recovery"].(map[string]any)["wrappedKey"] {
		t.Fatalf("GET harus mengembalikan bungkus pemulihan: %v", got)
	}

	newRec := validRecoveryJSON()
	wrong := map[string]any{"loginPassword": "salah-salah-salah"}
	for k, v := range newRec {
		wrong[k] = v
	}
	if code, _ := f.api("PUT", "/api/jurnal/pemulihan", csrf, wrong); code != http.StatusForbidden {
		t.Errorf("tanpa password login yang benar: %d, mau 403", code)
	}
	right := map[string]any{"loginPassword": testPassword}
	for k, v := range newRec {
		right[k] = v
	}
	if code, _ := f.api("PUT", "/api/jurnal/pemulihan", "", right); code != http.StatusForbidden {
		t.Errorf("tanpa CSRF: %d, mau 403", code)
	}
	if code, out := f.api("PUT", "/api/jurnal/pemulihan", csrf, right); code != http.StatusNoContent {
		t.Fatalf("mengganti kunci pemulihan: %d %v", code, out)
	}
	_, got = f.api("GET", "/api/jurnal/kunci", csrf, nil)
	if got["recovery"].(map[string]any)["salt"] != newRec["salt"] {
		t.Error("kunci pemulihan baru tidak tersimpan")
	}
	if got["wrappedKey"] != m["wrappedKey"] {
		t.Error("bungkus passphrase tidak boleh berubah")
	}

	bad := map[string]any{"loginPassword": testPassword, "iterations": 10, "salt": b64(16), "wrappedKey": b64(48), "wrapIv": b64(12)}
	if code, _ := f.api("PUT", "/api/jurnal/pemulihan", csrf, bad); code != http.StatusBadRequest {
		t.Errorf("iterasi rendah: %d, mau 400", code)
	}
	leak := map[string]any{"loginPassword": testPassword, "code": "AAAA-BBBB", "iterations": 100000, "salt": b64(16), "wrappedKey": b64(48), "wrapIv": b64(12)}
	if code, _ := f.api("PUT", "/api/jurnal/pemulihan", csrf, leak); code != http.StatusBadRequest {
		t.Errorf("kunci pemulihan mentah tidak boleh diterima server: %d, mau 400", code)
	}

	for _, p := range []string{"/jurnal/pulihkan", "/jurnal/pemulihan"} {
		code, body, _ := f.get(p)
		if code != http.StatusOK || !strings.Contains(body, "data-page=") {
			t.Errorf("%s: %d", p, code)
		}
		start := strings.Index(body, "data-form=")
		form := body[start:]
		form = form[:strings.Index(form, "</form>")]
		if strings.Contains(form, " name=") {
			t.Errorf("%s: input rahasia tidak boleh punya atribut name", p)
		}
	}
}

func TestEntriesRangeAPI(t *testing.T) {
	f := newFlow(t, false)
	csrf := f.signUp("a@contoh.id")
	_, out := f.api("GET", "/api/entri/rentang", csrf, nil)
	if out["first"] != nil {
		t.Errorf("tanpa entri, first harus null: %v", out)
	}
	d := time.Now().UTC().Format("2006-01-02")
	f.api("PUT", "/api/entri/"+d, csrf, validEntry())
	_, out = f.api("GET", "/api/entri/rentang", csrf, nil)
	if out["first"] != d {
		t.Errorf("first = %v, mau %s", out["first"], d)
	}
}
