// Package entries menyimpan entri harian. Isi terenkripsi (aktivitas dan
// catatan) disimpan apa adanya; server tidak bisa membacanya.
package entries

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	FormatVersion = 1
	ivLen         = 12
	minCTLen      = 16       // tag GCM
	maxCTLen      = 32 << 10 // 32 KiB: catatan panjang tetap muat
	MaxRangeDays  = 400      // batas satu permintaan riwayat
	dateLayout    = "2006-01-02"
)

var ErrNotFound = errors.New("entri tidak ditemukan")

// ValidationError menandai input yang ditolak karena bentuknya salah.
type ValidationError struct{ Err error }

func (e ValidationError) Error() string { return e.Err.Error() }
func (e ValidationError) Unwrap() error { return e.Err }

type Entry struct {
	Date         time.Time
	Mood         int
	SleepMinutes int
	Version      int
	PayloadCT    []byte
	PayloadIV    []byte
	UpdatedAt    time.Time
}

func (e Entry) DateString() string { return e.Date.Format(dateLayout) }

func ParseDate(s string) (time.Time, error) {
	d, err := time.Parse(dateLayout, s)
	if err != nil {
		return time.Time{}, ValidationError{errors.New("tanggal harus berformat YYYY-MM-DD")}
	}
	return d, nil
}

func (e Entry) validate() error {
	switch {
	case e.Mood < 1 || e.Mood > 5:
		return errors.New("skor mood harus 1 sampai 5")
	case e.SleepMinutes < 0 || e.SleepMinutes > 1440 || e.SleepMinutes%30 != 0:
		return errors.New("jam tidur harus 0–24 jam dengan kelipatan 30 menit")
	case e.Version != FormatVersion:
		return fmt.Errorf("versi format harus %d", FormatVersion)
	case len(e.PayloadIV) != ivLen:
		return fmt.Errorf("IV harus %d byte", ivLen)
	case len(e.PayloadCT) < minCTLen || len(e.PayloadCT) > maxCTLen:
		return fmt.Errorf("isi terenkripsi harus %d–%d byte", minCTLen, maxCTLen)
	}
	return nil
}

type Store struct {
	db  *pgxpool.Pool
	now func() time.Time
}

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db, now: time.Now} }

// EditableDate memeriksa tanggal yang boleh diisi. "Hari ini" mengikuti zona
// waktu browser (SPEC 5), yang tidak diketahui server; jadi server menerima
// rentang UTC yang sedikit lebih lebar dan browser membatasi ke hari ini/kemarin.
func (s *Store) EditableDate(d time.Time) error {
	todayUTC := s.now().UTC().Truncate(24 * time.Hour)
	earliest := todayUTC.AddDate(0, 0, -2)
	latest := todayUTC.AddDate(0, 0, 1)
	if d.Before(earliest) || d.After(latest) {
		return ValidationError{errors.New("hanya entri hari ini dan kemarin yang bisa diisi atau diubah")}
	}
	return nil
}

// Save membuat atau memperbarui entri untuk satu tanggal (satu entri per hari).
func (s *Store) Save(ctx context.Context, userID int64, e Entry) error {
	if err := e.validate(); err != nil {
		return ValidationError{err}
	}
	if err := s.EditableDate(e.Date); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO entries (user_id, entry_date, mood, sleep_minutes, format_version, payload_ct, payload_iv)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, entry_date) DO UPDATE SET
			mood = EXCLUDED.mood,
			sleep_minutes = EXCLUDED.sleep_minutes,
			format_version = EXCLUDED.format_version,
			payload_ct = EXCLUDED.payload_ct,
			payload_iv = EXCLUDED.payload_iv,
			updated_at = now()`,
		userID, e.Date, e.Mood, e.SleepMinutes, e.Version, e.PayloadCT, e.PayloadIV)
	return err
}

// Range mengembalikan entri dari..sampai (inklusif), urut tanggal naik.
func (s *Store) Range(ctx context.Context, userID int64, from, to time.Time) ([]Entry, error) {
	if to.Before(from) {
		return nil, ValidationError{errors.New("tanggal akhir sebelum tanggal awal")}
	}
	if to.Sub(from) > MaxRangeDays*24*time.Hour {
		return nil, ValidationError{fmt.Errorf("rentang maksimal %d hari", MaxRangeDays)}
	}
	rows, err := s.db.Query(ctx, `
		SELECT entry_date, mood, sleep_minutes, format_version, payload_ct, payload_iv, updated_at
		FROM entries WHERE user_id = $1 AND entry_date BETWEEN $2 AND $3
		ORDER BY entry_date`, userID, from, to)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Entry, error) {
		var e Entry
		err := r.Scan(&e.Date, &e.Mood, &e.SleepMinutes, &e.Version, &e.PayloadCT, &e.PayloadIV, &e.UpdatedAt)
		return e, err
	})
}

// FirstDate mengembalikan tanggal entri paling awal, atau nil jika belum ada.
func (s *Store) FirstDate(ctx context.Context, userID int64) (*time.Time, error) {
	var d *time.Time
	err := s.db.QueryRow(ctx, `SELECT min(entry_date) FROM entries WHERE user_id = $1`, userID).Scan(&d)
	return d, err
}

// LastSleep mengembalikan jam tidur dari entri terakhir sebelum tanggal d,
// sebagai nilai awal tombol tidur (SPEC 5). ok=false jika belum ada entri.
func (s *Store) LastSleep(ctx context.Context, userID int64, before time.Time) (minutes int, ok bool, err error) {
	err = s.db.QueryRow(ctx, `
		SELECT sleep_minutes FROM entries WHERE user_id = $1 AND entry_date < $2
		ORDER BY entry_date DESC LIMIT 1`, userID, before).Scan(&minutes)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return minutes, err == nil, err
}

// ---- Daftar chip aktivitas (terenkripsi) ----

type Blob struct {
	CT []byte
	IV []byte
}

func (b Blob) validate() error {
	if len(b.IV) != ivLen || len(b.CT) < minCTLen || len(b.CT) > maxCTLen {
		return errors.New("data terenkripsi tidak valid")
	}
	return nil
}

func (s *Store) Activities(ctx context.Context, userID int64) (*Blob, error) {
	var b Blob
	err := s.db.QueryRow(ctx, `SELECT activities_ct, activities_iv FROM user_settings WHERE user_id = $1`, userID).
		Scan(&b.CT, &b.IV)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Store) SaveActivities(ctx context.Context, userID int64, b Blob) error {
	if err := b.validate(); err != nil {
		return ValidationError{err}
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO user_settings (user_id, activities_ct, activities_iv) VALUES ($1, $2, $3)
		ON CONFLICT (user_id) DO UPDATE SET
			activities_ct = EXCLUDED.activities_ct, activities_iv = EXCLUDED.activities_iv, updated_at = now()`,
		userID, b.CT, b.IV)
	return err
}
