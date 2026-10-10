package reminders

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"testing"
	"time"

	"moodjournal/internal/auth"
	"moodjournal/internal/testdb"
)

func b64url(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func validSub(userID int64, endpoint string) Subscription {
	return Subscription{UserID: userID, Endpoint: endpoint, P256dh: b64url(65), Auth: b64url(16),
		RemindMinute: 21 * 60, Timezone: "Asia/Jakarta"}
}

func TestValidEndpoint(t *testing.T) {
	for _, ok := range []string{
		"https://fcm.googleapis.com/fcm/send/abc",
		"https://updates.push.services.mozilla.com/wpush/v2/abc",
		"https://web.push.apple.com/QH1abc",
		"https://wns2-par02p.notify.windows.com/w/?token=abc",
	} {
		if err := validEndpoint(ok); err != nil {
			t.Errorf("%s ditolak: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"http://fcm.googleapis.com/fcm/send/abc",    // bukan https
		"https://127.0.0.1/admin",                   // alamat internal (SSRF)
		"https://localhost:8080/api",                // alamat internal
		"https://fcm.googleapis.com.evil.example/x", // akhiran palsu
		"https://evilpush.apple.com.example/x",      // akhiran palsu
		"https://user:pw@fcm.googleapis.com/x",      // kredensial di URL
		"https://fcm.googleapis.com:8443/x",         // port lain
		"https://notpush.apple.com",                 // bukan subdomain .push.apple.com
		"javascript:alert(1)",
	} {
		if validEndpoint(bad) == nil {
			t.Errorf("%s diterima padahal harus ditolak", bad)
		}
	}
}

func TestSubscriptionValidate(t *testing.T) {
	base := validSub(1, "https://fcm.googleapis.com/fcm/send/abc")
	if err := base.validate(); err != nil {
		t.Fatalf("langganan valid ditolak: %v", err)
	}
	for name, mutate := range map[string]func(*Subscription){
		"p256dh pendek": func(s *Subscription) { s.P256dh = b64url(32) },
		"auth pendek":   func(s *Subscription) { s.Auth = b64url(8) },
		"zona waktu":    func(s *Subscription) { s.Timezone = "Mars/Olympus" },
		"Local":         func(s *Subscription) { s.Timezone = "Local" },
		"menit":         func(s *Subscription) { s.RemindMinute = 1440 },
	} {
		s := base
		mutate(&s)
		if s.validate() == nil {
			t.Errorf("%s: diterima padahal tidak valid", name)
		}
	}
}

func TestParseTime(t *testing.T) {
	if m, err := ParseTime("21:30"); err != nil || m != 21*60+30 {
		t.Errorf("ParseTime = %d, %v", m, err)
	}
	if FormatTime(9*60+5) != "09:05" {
		t.Error("FormatTime")
	}
	for _, bad := range []string{"25:00", "9", "", "21.30"} {
		if _, err := ParseTime(bad); err == nil {
			t.Errorf("%q harus ditolak", bad)
		}
	}
}

func TestDue(t *testing.T) {
	jakarta, _ := time.LoadLocation("Asia/Jakarta")
	at := func(h, m int) time.Time { return time.Date(2026, 10, 10, h, m, 0, 0, jakarta) }
	s := Subscription{RemindMinute: 21 * 60, Timezone: "Asia/Jakarta"}

	if due, _ := Due(at(20, 59), s); due {
		t.Error("belum waktunya")
	}
	due, date := Due(at(21, 0), s)
	if !due || date.Format("2006-01-02") != "2026-10-10" {
		t.Errorf("tepat waktu: due=%v date=%v", due, date)
	}
	if due, _ := Due(at(21, 59), s); !due {
		t.Error("masih dalam jendela 60 menit")
	}
	if due, _ := Due(at(22, 0), s); due {
		t.Error("lewat jendela 60 menit: dilewati")
	}

	sent := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	s.LastSentOn = &sent
	if due, _ := Due(at(21, 5), s); due {
		t.Error("sudah dikirim hari ini")
	}
	if due, _ := Due(at(21, 5).AddDate(0, 0, 1), s); !due {
		t.Error("hari berikutnya harus dikirim lagi")
	}

	// Zona waktu: 21:00 WIB = 14:00 UTC.
	s.LastSentOn = nil
	if due, _ := Due(time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC), s); !due {
		t.Error("waktu server UTC harus dikonversi ke zona waktu pengguna")
	}
	// Tanggal lokal bisa berbeda dari tanggal UTC.
	late := Subscription{RemindMinute: 6 * 60, Timezone: "Asia/Jakarta"}
	_, d := Due(time.Date(2026, 10, 9, 23, 30, 0, 0, time.UTC), late) // = 10 Okt 06:30 WIB
	if d.Format("2006-01-02") != "2026-10-10" {
		t.Errorf("tanggal lokal = %v, mau 2026-10-10", d)
	}
}

type fakeSender struct {
	sent   []string
	status int
	err    error
}

func (f *fakeSender) Send(_ context.Context, s Subscription) (int, error) {
	f.sent = append(f.sent, s.Endpoint)
	if f.status == 0 {
		return http.StatusCreated, f.err
	}
	return f.status, f.err
}

func setup(t *testing.T) (*Store, int64, int64) {
	t.Helper()
	pool := testdb.Open(t)
	svc, _ := auth.NewService(pool, false)
	a, err := svc.Register(context.Background(), "a@contoh.id", "kuda-laut-biru-42", "")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := svc.Register(context.Background(), "b@contoh.id", "kuda-laut-biru-42", "")
	return NewStore(pool), a, b
}

func TestSaveForUserDelete(t *testing.T) {
	st, a, b := setup(t)
	ctx := context.Background()
	ep := "https://fcm.googleapis.com/fcm/send/perangkat-1"
	if err := st.Save(ctx, validSub(a, ep)); err != nil {
		t.Fatal(err)
	}
	upd := validSub(a, ep)
	upd.RemindMinute = 20 * 60
	if err := st.Save(ctx, upd); err != nil {
		t.Fatal(err)
	}
	subs, _ := st.ForUser(ctx, a)
	if len(subs) != 1 || subs[0].RemindMinute != 20*60 {
		t.Fatalf("ForUser: %+v", subs)
	}

	// Browser yang sama dipakai akun lain: langganan berpindah pemilik.
	if err := st.Save(ctx, validSub(b, ep)); err != nil {
		t.Fatal(err)
	}
	if subs, _ := st.ForUser(ctx, a); len(subs) != 0 {
		t.Error("langganan harus berpindah ke akun b")
	}
	if err := st.Delete(ctx, a, ep); !errors.Is(err, ErrNotFound) {
		t.Errorf("akun a tidak boleh menghapus langganan akun b: %v", err)
	}
	if err := st.Delete(ctx, b, ep); err != nil {
		t.Fatal(err)
	}

	var verr ValidationError
	if err := st.Save(ctx, validSub(a, "https://169.254.169.254/latest/meta-data")); !errors.As(err, &verr) {
		t.Errorf("endpoint internal harus ditolak: %v", err)
	}
}

func TestRunOnce(t *testing.T) {
	st, a, b := setup(t)
	ctx := context.Background()
	epA := "https://fcm.googleapis.com/fcm/send/a"
	epB := "https://updates.push.services.mozilla.com/wpush/v2/b"
	st.Save(ctx, validSub(a, epA))
	st.Save(ctx, validSub(b, epB))

	// Pengguna b sudah mencatat hari ini (tanggal lokal Jakarta): tidak diingatkan.
	if _, err := st.db.Exec(ctx, `INSERT INTO entries (user_id, entry_date, mood, sleep_minutes, format_version, payload_ct, payload_iv)
		VALUES ($1, DATE '2026-10-10', 3, 420, 1, decode(repeat('ab', 20), 'hex'), decode(repeat('cd', 12), 'hex'))`, b); err != nil {
		t.Fatal(err)
	}

	jakarta, _ := time.LoadLocation("Asia/Jakarta")
	now := time.Date(2026, 10, 10, 21, 1, 0, 0, jakarta)

	f := &fakeSender{}
	if err := st.RunOnce(ctx, now, f); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 1 || f.sent[0] != epA {
		t.Fatalf("terkirim: %v, mau hanya ke a", f.sent)
	}

	// Menit berikutnya: tidak dikirim dua kali.
	if err := st.RunOnce(ctx, now.Add(time.Minute), f); err != nil {
		t.Fatal(err)
	}
	if len(f.sent) != 1 {
		t.Errorf("pengingat terkirim dua kali: %v", f.sent)
	}

	// Besok: langganan a ditolak layanan push (410) dan dihapus.
	gone := &fakeSender{status: http.StatusGone, err: errors.New("gone")}
	if err := st.RunOnce(ctx, now.AddDate(0, 0, 1), gone); err != nil {
		t.Fatal(err)
	}
	if subs, _ := st.ForUser(ctx, a); len(subs) != 0 {
		t.Error("langganan yang dijawab 410 harus dihapus")
	}

	// Error lain (mis. 500): dicatat dan ditandai, tidak dicoba ulang tiap menit.
	st.Save(ctx, validSub(a, epA))
	failing := &fakeSender{status: http.StatusInternalServerError, err: errors.New("500")}
	day3 := now.AddDate(0, 0, 2)
	st.RunOnce(ctx, day3, failing)
	st.RunOnce(ctx, day3.Add(time.Minute), failing)
	if len(failing.sent) != 1 {
		t.Errorf("layanan push error dicoba ulang %d kali, mau 1", len(failing.sent))
	}
	if subs, _ := st.ForUser(ctx, a); len(subs) != 1 {
		t.Error("error sementara tidak boleh menghapus langganan")
	}
}
