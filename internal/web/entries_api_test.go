package web

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

func today() string { return time.Now().UTC().Format("2006-01-02") }

func validEntry() map[string]any {
	return map[string]any{"mood": 4, "sleepMinutes": 450, "version": 1, "payloadCt": b64(80), "payloadIv": b64(12)}
}

func TestEntriesAPI(t *testing.T) {
	f := newFlow(t, false)
	if code, _ := f.api("GET", "/api/entri?dari=2026-10-01&sampai=2026-10-10", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("tanpa login: status %d", code)
	}
	csrf := f.signUp("a@contoh.id")
	d := today()

	if code, _ := f.api("PUT", "/api/entri/"+d, "", validEntry()); code != http.StatusForbidden {
		t.Errorf("PUT tanpa CSRF: status %d, mau 403", code)
	}

	e := validEntry()
	if code, out := f.api("PUT", "/api/entri/"+d, csrf, e); code != http.StatusNoContent {
		t.Fatalf("simpan entri: status %d %v", code, out)
	}
	e2 := validEntry()
	e2["mood"] = 2
	if code, _ := f.api("PUT", "/api/entri/"+d, csrf, e2); code != http.StatusNoContent {
		t.Fatalf("edit entri: status %d", code)
	}

	code, out := f.api("GET", "/api/entri?dari="+d+"&sampai="+d, csrf, nil)
	list, _ := out["entries"].([]any)
	if code != http.StatusOK || len(list) != 1 {
		t.Fatalf("GET entri: status %d %v", code, out)
	}
	got := list[0].(map[string]any)
	if got["mood"].(float64) != 2 || got["payloadCt"] != e2["payloadCt"] || got["date"] != d {
		t.Errorf("entri tersimpan tidak sesuai: %v", got)
	}
	if out["sleepBeforeFrom"] != nil {
		t.Errorf("sleepBeforeFrom harus null tanpa entri sebelumnya: %v", out["sleepBeforeFrom"])
	}

	// Pengguna lain tidak melihat entri ini.
	f2 := newFlowSharedDB(t, f)
	csrf2 := f2.signUp("b@contoh.id")
	_, out = f2.api("GET", "/api/entri?dari="+d+"&sampai="+d, csrf2, nil)
	if l, _ := out["entries"].([]any); len(l) != 0 {
		t.Errorf("pengguna lain melihat entri: %v", out)
	}
}

func TestEntriesAPIRejectsPlaintextAndBadInput(t *testing.T) {
	f := newFlow(t, false)
	csrf := f.signUp("a@contoh.id")
	d := today()

	for name, mutate := range map[string]func(map[string]any){
		"catatan biasa":   func(e map[string]any) { e["note"] = "rahasia" },
		"aktivitas biasa": func(e map[string]any) { e["activities"] = []string{"Olahraga"} },
		"mood 9":          func(e map[string]any) { e["mood"] = 9 },
		"tidur 45 menit":  func(e map[string]any) { e["sleepMinutes"] = 45 },
		"iv pendek":       func(e map[string]any) { e["payloadIv"] = b64(4) },
		"tanpa payload":   func(e map[string]any) { delete(e, "payloadCt") },
	} {
		e := validEntry()
		mutate(e)
		if code, _ := f.api("PUT", "/api/entri/"+d, csrf, e); code != http.StatusBadRequest {
			t.Errorf("%s: status %d, mau 400", name, code)
		}
	}

	old := time.Now().UTC().AddDate(0, 0, -10).Format("2006-01-02")
	if code, _ := f.api("PUT", "/api/entri/"+old, csrf, validEntry()); code != http.StatusBadRequest {
		t.Errorf("tanggal 10 hari lalu: status %d, mau 400", code)
	}
	if code, _ := f.api("PUT", "/api/entri/kemarin", csrf, validEntry()); code != http.StatusBadRequest {
		t.Errorf("tanggal tidak valid: status %d", code)
	}
	if code, _ := f.api("GET", "/api/entri?dari=x&sampai=y", csrf, nil); code != http.StatusBadRequest {
		t.Errorf("parameter rentang tidak valid: status %d", code)
	}

	_, out := f.api("GET", "/api/entri?dari="+old+"&sampai="+d, csrf, nil)
	if l, _ := out["entries"].([]any); len(l) != 0 {
		t.Errorf("tidak boleh ada entri tersimpan: %v", out)
	}
}

func TestActivitiesAPI(t *testing.T) {
	f := newFlow(t, false)
	csrf := f.signUp("a@contoh.id")
	if code, _ := f.api("GET", "/api/aktivitas", csrf, nil); code != http.StatusNoContent {
		t.Errorf("sebelum disimpan: status %d, mau 204 (bukan error)", code)
	}
	blob := map[string]any{"ct": b64(60), "iv": b64(12)}
	if code, out := f.api("PUT", "/api/aktivitas", csrf, blob); code != http.StatusNoContent {
		t.Fatalf("simpan: %d %v", code, out)
	}
	code, out := f.api("GET", "/api/aktivitas", csrf, nil)
	if code != http.StatusOK || out["ct"] != blob["ct"] {
		t.Errorf("GET aktivitas: %d %v", code, out)
	}
	if code, _ := f.api("PUT", "/api/aktivitas", csrf, map[string]any{"list": []string{"Olahraga"}}); code != http.StatusBadRequest {
		t.Errorf("daftar teks biasa harus ditolak: status %d", code)
	}
}

func TestHistoryPageAndGentleCard(t *testing.T) {
	f := newFlow(t, false)
	csrf := f.signUp("a@contoh.id")
	if code, _, _ := f.get("/riwayat"); code != http.StatusSeeOther {
		t.Errorf("/riwayat sebelum kunci jurnal dibuat harus redirect, status %d", code)
	}
	if code, _ := f.api("POST", "/api/jurnal/kunci", csrf, validMaterial()); code != http.StatusCreated {
		t.Fatal("membuat kunci gagal")
	}
	code, body, _ := f.get("/riwayat")
	if code != http.StatusOK || !strings.Contains(body, `data-page="history"`) {
		t.Fatalf("/riwayat: status %d", code)
	}

	for _, path := range []string{"/beranda", "/hari-ini", "/riwayat"} {
		_, body, _ := f.get(path)
		i := strings.Index(body, `data-role="gentle-card"`)
		if i < 0 {
			t.Errorf("%s: kartu pesan lembut tidak ada", path)
			continue
		}
		card := body[i:]
		card = card[:strings.Index(card, "</section>")]
		if !strings.Contains(card, " hidden>") {
			t.Errorf("%s: kartu harus tersembunyi secara default (ditampilkan oleh browser)", path)
		}
		if !strings.Contains(card, `href="/bantuan"`) {
			t.Errorf("%s: kartu harus menautkan ke halaman bantuan", path)
		}
		lower := strings.ToLower(card)
		for _, word := range []string{"depresi", "diagnosis", "darurat!", "bahaya", "peringatan"} {
			if strings.Contains(lower, word) {
				t.Errorf("%s: kartu tidak boleh memuat kata %q (tanpa label/nada alarmis)", path, word)
			}
		}
	}
}

func TestEntryPage(t *testing.T) {
	f := newFlow(t, false)
	csrf := f.signUp("a@contoh.id")
	if code, _, _ := f.get("/hari-ini"); code != http.StatusSeeOther {
		t.Errorf("/hari-ini sebelum kunci jurnal dibuat harus redirect, status %d", code)
	}
	if code, _ := f.api("POST", "/api/jurnal/kunci", csrf, validMaterial()); code != http.StatusCreated {
		t.Fatal("membuat kunci gagal")
	}
	code, body, _ := f.get("/hari-ini")
	if code != http.StatusOK || !strings.Contains(body, `data-page="entry"`) || !strings.Contains(body, `data-requires="unlocked"`) {
		t.Fatalf("/hari-ini: status %d", code)
	}
	start := strings.Index(body, `data-page="entry"`)
	if strings.Contains(body[start:], " name=") {
		t.Error("input di layar harian tidak boleh punya atribut name")
	}
}
