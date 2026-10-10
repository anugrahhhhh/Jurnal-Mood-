// Package reminders mengirim pengingat harian opsional lewat Web Push.
//
// Isi notifikasi selalu generik (ditulis di service worker), tidak pernah
// menyebut mood atau isi jurnal. Pengingat dilewati jika entri hari itu
// sudah diisi, dan tidak ada hitungan streak.
package reminders

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Window: pengingat dikirim paling lambat 60 menit setelah jamnya
// (mis. jika server sempat mati), setelah itu dilewati untuk hari itu.
const Window = 60

var ErrNotFound = errors.New("pengingat tidak ditemukan")

type ValidationError struct{ Err error }

func (e ValidationError) Error() string { return e.Err.Error() }
func (e ValidationError) Unwrap() error { return e.Err }

// allowedPushHosts membatasi endpoint ke layanan push resmi. Server mengirim
// HTTP POST ke endpoint ini, jadi tanpa batasan pengguna bisa menyuruh server
// menghubungi alamat internal (SSRF).
var allowedPushHosts = []string{
	"fcm.googleapis.com",                // Chrome, Edge (Android/desktop), Brave, Opera
	"updates.push.services.mozilla.com", // Firefox
	".push.apple.com",                   // Safari macOS/iOS (web.push.apple.com, dll.)
	".notify.windows.com",               // Edge lama di Windows
}

func validEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || len(raw) > 1024 {
		return errors.New("alamat layanan push tidak valid")
	}
	host := strings.ToLower(u.Hostname())
	for _, h := range allowedPushHosts {
		if host == h || (strings.HasPrefix(h, ".") && strings.HasSuffix(host, h)) {
			return nil
		}
	}
	return fmt.Errorf("layanan push %q tidak didukung", host)
}

func decodedLen(s string) int {
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
	if err != nil {
		return -1
	}
	return len(b)
}

// ParseTime mengubah "HH:MM" menjadi menit sejak tengah malam.
func ParseTime(s string) (int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, ValidationError{errors.New("jam pengingat harus berformat HH:MM")}
	}
	return t.Hour()*60 + t.Minute(), nil
}

func FormatTime(minute int) string { return fmt.Sprintf("%02d:%02d", minute/60, minute%60) }

type Subscription struct {
	ID           int64
	UserID       int64
	Endpoint     string
	P256dh       string
	Auth         string
	RemindMinute int
	Timezone     string
	LastSentOn   *time.Time
}

func (s Subscription) validate() error {
	if err := validEndpoint(s.Endpoint); err != nil {
		return err
	}
	if decodedLen(s.P256dh) != 65 || decodedLen(s.Auth) != 16 {
		return errors.New("kunci langganan push tidak valid")
	}
	if s.RemindMinute < 0 || s.RemindMinute > 1439 {
		return errors.New("jam pengingat tidak valid")
	}
	if len(s.Timezone) == 0 || len(s.Timezone) > 64 {
		return errors.New("zona waktu tidak valid")
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil || s.Timezone == "Local" {
		return errors.New("zona waktu tidak dikenal")
	}
	return nil
}

// Due menentukan apakah pengingat perlu dikirim sekarang, dan tanggal lokal
// yang dipakai. Fungsi murni agar mudah diuji.
func Due(now time.Time, s Subscription) (bool, time.Time) {
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return false, time.Time{}
	}
	local := now.In(loc)
	date := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	minute := local.Hour()*60 + local.Minute()
	if minute < s.RemindMinute || minute >= s.RemindMinute+Window {
		return false, date
	}
	if s.LastSentOn != nil && !s.LastSentOn.Before(date) {
		return false, date
	}
	return true, date
}

type Store struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

// Save menyimpan atau memperbarui langganan satu perangkat. Endpoint milik
// browser; jika orang lain lalu masuk di browser yang sama dan mengaktifkan
// pengingat, langganan itu berpindah ke akunnya.
func (st *Store) Save(ctx context.Context, s Subscription) error {
	if err := s.validate(); err != nil {
		return ValidationError{err}
	}
	_, err := st.db.Exec(ctx, `
		INSERT INTO push_subscriptions (user_id, endpoint, p256dh, auth, remind_minute, timezone)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (endpoint) DO UPDATE SET
			user_id = EXCLUDED.user_id, p256dh = EXCLUDED.p256dh, auth = EXCLUDED.auth,
			remind_minute = EXCLUDED.remind_minute, timezone = EXCLUDED.timezone,
			last_sent_on = CASE WHEN push_subscriptions.user_id = EXCLUDED.user_id
				THEN push_subscriptions.last_sent_on END,
			updated_at = now()`,
		s.UserID, s.Endpoint, s.P256dh, s.Auth, s.RemindMinute, s.Timezone)
	return err
}

func (st *Store) Delete(ctx context.Context, userID int64, endpoint string) error {
	tag, err := st.db.Exec(ctx, `DELETE FROM push_subscriptions WHERE user_id = $1 AND endpoint = $2`, userID, endpoint)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func scanSubs(rows pgx.Rows) ([]Subscription, error) {
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Subscription, error) {
		var s Subscription
		err := r.Scan(&s.ID, &s.UserID, &s.Endpoint, &s.P256dh, &s.Auth, &s.RemindMinute, &s.Timezone, &s.LastSentOn)
		return s, err
	})
}

const subColumns = `id, user_id, endpoint, p256dh, auth, remind_minute, timezone, last_sent_on`

func (st *Store) ForUser(ctx context.Context, userID int64) ([]Subscription, error) {
	rows, err := st.db.Query(ctx, `SELECT `+subColumns+` FROM push_subscriptions WHERE user_id = $1 ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	return scanSubs(rows)
}

func (st *Store) all(ctx context.Context) ([]Subscription, error) {
	rows, err := st.db.Query(ctx, `SELECT `+subColumns+` FROM push_subscriptions ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return scanSubs(rows)
}

func (st *Store) hasEntry(ctx context.Context, userID int64, date time.Time) (bool, error) {
	var ok bool
	err := st.db.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM entries WHERE user_id = $1 AND entry_date = $2)`, userID, date).Scan(&ok)
	return ok, err
}

func (st *Store) markSent(ctx context.Context, id int64, date time.Time) error {
	_, err := st.db.Exec(ctx, `UPDATE push_subscriptions SET last_sent_on = $2 WHERE id = $1`, id, date)
	return err
}

func (st *Store) deleteByID(ctx context.Context, id int64) error {
	_, err := st.db.Exec(ctx, `DELETE FROM push_subscriptions WHERE id = $1`, id)
	return err
}

// ---- Pengiriman ----

// Sender mengirim satu notifikasi. Status 404/410 berarti langganan sudah
// tidak berlaku (pengguna mencabut izin), dan akan dihapus.
type Sender interface {
	Send(ctx context.Context, s Subscription) (status int, err error)
}

// VAPID adalah kunci server untuk Web Push. Subject: email atau URL https pengelola.
type VAPID struct {
	PublicKey  string
	PrivateKey string
	Subject    string
}

func (v VAPID) Enabled() bool { return v.PublicKey != "" && v.PrivateKey != "" && v.Subject != "" }

type webPushSender struct {
	vapid  VAPID
	client *http.Client
}

func NewWebPushSender(v VAPID) Sender {
	return &webPushSender{vapid: v, client: &http.Client{Timeout: 15 * time.Second}}
}

// Pesan berisi penanda saja; teks notifikasi ditulis di service worker.
var reminderMessage = []byte(`{"type":"reminder"}`)

func (w *webPushSender) Send(ctx context.Context, s Subscription) (int, error) {
	resp, err := webpush.SendNotificationWithContext(ctx, reminderMessage, &webpush.Subscription{
		Endpoint: s.Endpoint,
		Keys:     webpush.Keys{P256dh: s.P256dh, Auth: s.Auth},
	}, &webpush.Options{
		HTTPClient:      w.client,
		Subscriber:      w.vapid.Subject,
		VAPIDPublicKey:  w.vapid.PublicKey,
		VAPIDPrivateKey: w.vapid.PrivateKey,
		TTL:             3600,
		Topic:           "pengingat",
		Urgency:         webpush.UrgencyNormal,
	})
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("layanan push menjawab %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

// RunOnce memeriksa semua langganan dan mengirim yang jatuh tempo.
func (st *Store) RunOnce(ctx context.Context, now time.Time, sender Sender) error {
	subs, err := st.all(ctx)
	if err != nil {
		return err
	}
	for _, s := range subs {
		due, date := Due(now, s)
		if !due {
			continue
		}
		filled, err := st.hasEntry(ctx, s.UserID, date)
		if err != nil {
			return err
		}
		if !filled {
			status, err := sender.Send(ctx, s)
			if status == http.StatusNotFound || status == http.StatusGone {
				if err := st.deleteByID(ctx, s.ID); err != nil {
					return err
				}
				continue
			}
			if err != nil {
				// Tetap ditandai terkirim agar layanan push yang error tidak
				// dicoba ulang setiap menit sepanjang jendela waktu.
				slog.Warn("mengirim pengingat gagal", "status", status, "err", err)
			}
		}
		if err := st.markSent(ctx, s.ID, date); err != nil {
			return err
		}
	}
	return nil
}

// Run menjalankan RunOnce setiap menit sampai ctx selesai.
func (st *Store) Run(ctx context.Context, sender Sender) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		if err := st.RunOnce(ctx, time.Now(), sender); err != nil && ctx.Err() == nil {
			slog.Error("pengingat", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
