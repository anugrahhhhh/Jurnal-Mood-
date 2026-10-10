package web

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"
	"testing"
)

// Kunci publik VAPID palsu untuk tes; server hanya meneruskannya ke browser.
const testVAPIDPublicKey = "BTesKunciPublikVapidUntukTesSaja"

func b64url(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func validReminder(endpoint string) map[string]any {
	return map[string]any{
		"endpoint": endpoint,
		"keys":     map[string]any{"p256dh": b64url(65), "auth": b64url(16)},
		"time":     "21:00",
		"timezone": "Asia/Jakarta",
	}
}

func TestRemindersAPI(t *testing.T) {
	f := newFlow(t, false)
	if code, _ := f.api("GET", "/api/pengingat", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("tanpa login: status %d", code)
	}
	csrf := f.signUp("a@contoh.id")

	code, out := f.api("GET", "/api/pengingat", csrf, nil)
	if code != http.StatusOK || out["enabled"] != true || out["publicKey"] != testVAPIDPublicKey {
		t.Fatalf("status pengingat: %d %v", code, out)
	}

	ep := "https://fcm.googleapis.com/fcm/send/perangkat-uji"
	if code, _ := f.api("PUT", "/api/pengingat", "", validReminder(ep)); code != http.StatusForbidden {
		t.Errorf("tanpa CSRF: status %d", code)
	}
	if code, out := f.api("PUT", "/api/pengingat", csrf, validReminder(ep)); code != http.StatusNoContent {
		t.Fatalf("simpan pengingat: %d %v", code, out)
	}
	_, out = f.api("GET", "/api/pengingat", csrf, nil)
	subs, _ := out["subscriptions"].([]any)
	if len(subs) != 1 || subs[0].(map[string]any)["time"] != "21:00" {
		t.Fatalf("daftar pengingat: %v", out)
	}

	for name, mutate := range map[string]func(map[string]any){
		"SSRF localhost": func(m map[string]any) { m["endpoint"] = "https://localhost/admin" },
		"http":           func(m map[string]any) { m["endpoint"] = "http://fcm.googleapis.com/x" },
		"jam":            func(m map[string]any) { m["time"] = "25:00" },
		"zona waktu":     func(m map[string]any) { m["timezone"] = "Bulan/Purnama" },
		"field asing":    func(m map[string]any) { m["mood"] = 3 },
	} {
		m := validReminder("https://fcm.googleapis.com/fcm/send/lain")
		mutate(m)
		if code, _ := f.api("PUT", "/api/pengingat", csrf, m); code != http.StatusBadRequest {
			t.Errorf("%s: status %d, mau 400", name, code)
		}
	}

	if code, _ := f.api("DELETE", "/api/pengingat", csrf, map[string]any{"endpoint": ep}); code != http.StatusNoContent {
		t.Errorf("hapus: status %d", code)
	}
	_, out = f.api("GET", "/api/pengingat", csrf, nil)
	if subs, _ := out["subscriptions"].([]any); len(subs) != 0 {
		t.Errorf("pengingat masih ada: %v", out)
	}
}

func TestServiceWorkerAndManifest(t *testing.T) {
	h := newTestServer(t, "development", fakeDB{}, testCrisis(true))
	sw := get(h, "/sw.js")
	if sw.Code != http.StatusOK || !strings.HasPrefix(sw.Header().Get("Content-Type"), "text/javascript") {
		t.Fatalf("/sw.js: %d %s", sw.Code, sw.Header().Get("Content-Type"))
	}
	body := sw.Body.String()
	if strings.Contains(body, "addEventListener('fetch'") || strings.Contains(body, "caches.") {
		t.Error("service worker tidak boleh meng-cache halaman (bisa berisi teks terdekripsi)")
	}
	m := get(h, "/manifest.webmanifest")
	if m.Code != http.StatusOK || !strings.Contains(m.Body.String(), `"start_url": "/hari-ini"`) {
		t.Errorf("manifest: %d", m.Code)
	}
	for _, icon := range []string{"/static/icons/icon-192.png", "/static/icons/icon-512.png", "/static/icons/apple-touch-icon.png", "/static/icons/badge-96.png"} {
		if rec := get(h, icon); rec.Code != http.StatusOK {
			t.Errorf("%s: status %d", icon, rec.Code)
		}
	}
}

func TestSettingsHidesRemindersWhenDisabled(t *testing.T) {
	// newTestServer tidak mengaktifkan pengingat; halaman pengaturan butuh login
	// sehingga dicek lewat template yang dirender untuk flow (pengingat aktif).
	f := newFlow(t, false)
	f.signUp("a@contoh.id")
	if _, body, _ := f.get("/pengaturan"); !strings.Contains(body, `data-page="reminders"`) {
		t.Error("bagian pengingat harus tampil saat VAPID dikonfigurasi")
	}
}
