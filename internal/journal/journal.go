// Package journal menyimpan material kunci jurnal yang dibuat di browser.
//
// Server tidak bisa membuka kunci ini: ia hanya memvalidasi bentuknya
// (panjang byte, parameter KDF) lalu menyimpannya apa adanya.
package journal

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	FormatVersion = 1
	KDF           = "PBKDF2-SHA256"
	// MinIterations adalah batas bawah yang diterima server; browser memakai
	// nilai di web/static/js/crypto.js (600.000, rekomendasi OWASP 2023).
	MinIterations = 600_000
	maxIterations = 10_000_000

	saltLen       = 16
	ivLen         = 12
	wrappedDEKLen = 32 + 16 // kunci AES-256 + tag GCM

	// MinRecoveryIterations: kunci pemulihan berentropi 120 bit, jadi batas
	// iterasinya lebih rendah daripada passphrase (lihat crypto.js).
	MinRecoveryIterations = 100_000
)

var (
	ErrNotFound = errors.New("kunci jurnal belum dibuat")
	ErrExists   = errors.New("kunci jurnal sudah ada")
)

// Wrap adalah bagian yang berubah saat passphrase diganti.
type Wrap struct {
	KDF        string
	Iterations int
	Salt       []byte
	WrappedDEK []byte
	WrapIV     []byte
}

// RecoveryWrap adalah DEK yang dibungkus kunci pemulihan.
type RecoveryWrap struct {
	Iterations int
	Salt       []byte
	WrappedDEK []byte
	WrapIV     []byte
}

func (r RecoveryWrap) validate() error {
	switch {
	case r.Iterations < MinRecoveryIterations || r.Iterations > maxIterations:
		return fmt.Errorf("iterasi kunci pemulihan harus antara %d dan %d", MinRecoveryIterations, maxIterations)
	case len(r.Salt) != saltLen:
		return fmt.Errorf("salt pemulihan harus %d byte", saltLen)
	case len(r.WrappedDEK) != wrappedDEKLen:
		return fmt.Errorf("kunci pemulihan terbungkus harus %d byte", wrappedDEKLen)
	case len(r.WrapIV) != ivLen:
		return fmt.Errorf("IV pemulihan harus %d byte", ivLen)
	}
	return nil
}

// Key adalah seluruh material kunci seorang pengguna.
type Key struct {
	Version int
	Wrap
	Verifier   []byte
	VerifierIV []byte
	Recovery   *RecoveryWrap // nil: belum punya kunci pemulihan (akun lama)
}

func (w Wrap) validate() error {
	switch {
	case w.KDF != KDF:
		return fmt.Errorf("kdf harus %s", KDF)
	case w.Iterations < MinIterations || w.Iterations > maxIterations:
		return fmt.Errorf("iterasi KDF harus antara %d dan %d", MinIterations, maxIterations)
	case len(w.Salt) != saltLen:
		return fmt.Errorf("salt harus %d byte", saltLen)
	case len(w.WrappedDEK) != wrappedDEKLen:
		return fmt.Errorf("kunci terbungkus harus %d byte", wrappedDEKLen)
	case len(w.WrapIV) != ivLen:
		return fmt.Errorf("IV pembungkus harus %d byte", ivLen)
	}
	return nil
}

func (k Key) validate() error {
	if k.Version != FormatVersion {
		return fmt.Errorf("versi format harus %d", FormatVersion)
	}
	if err := k.Wrap.validate(); err != nil {
		return err
	}
	if len(k.Verifier) < 17 || len(k.Verifier) > 256 {
		return errors.New("nilai uji tidak valid")
	}
	if len(k.VerifierIV) != ivLen {
		return fmt.Errorf("IV nilai uji harus %d byte", ivLen)
	}
	if k.Recovery != nil {
		return k.Recovery.validate()
	}
	return nil
}

// ValidationError menandai input yang ditolak karena bentuknya salah.
type ValidationError struct{ Err error }

func (e ValidationError) Error() string { return e.Err.Error() }
func (e ValidationError) Unwrap() error { return e.Err }

type Store struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

func (s *Store) Get(ctx context.Context, userID int64) (*Key, error) {
	var k Key
	var rIter *int
	var r RecoveryWrap
	err := s.db.QueryRow(ctx, `
		SELECT format_version, kdf, kdf_iterations, salt, wrapped_dek, wrap_iv, verifier_ct, verifier_iv,
			recovery_iterations, recovery_salt, recovery_wrapped_dek, recovery_wrap_iv
		FROM journal_keys WHERE user_id = $1`, userID).
		Scan(&k.Version, &k.KDF, &k.Iterations, &k.Salt, &k.WrappedDEK, &k.WrapIV, &k.Verifier, &k.VerifierIV,
			&rIter, &r.Salt, &r.WrappedDEK, &r.WrapIV)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if rIter != nil && r.Salt != nil {
		r.Iterations = *rIter
		k.Recovery = &r
	}
	return &k, nil
}

func (s *Store) Exists(ctx context.Context, userID int64) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM journal_keys WHERE user_id = $1)`, userID).Scan(&exists)
	return exists, err
}

// Create menyimpan kunci pertama. Kunci yang sudah ada tidak pernah ditimpa
// di sini, karena menimpanya akan membuat semua catatan lama tak terbaca.
func (s *Store) Create(ctx context.Context, userID int64, k Key) error {
	if err := k.validate(); err != nil {
		return ValidationError{err}
	}
	var rIter *int
	var rSalt, rDEK, rIV []byte
	if k.Recovery != nil {
		rIter, rSalt, rDEK, rIV = &k.Recovery.Iterations, k.Recovery.Salt, k.Recovery.WrappedDEK, k.Recovery.WrapIV
	}
	_, err := s.db.Exec(ctx, `
		INSERT INTO journal_keys
			(user_id, format_version, kdf, kdf_iterations, salt, wrapped_dek, wrap_iv, verifier_ct, verifier_iv,
			 recovery_iterations, recovery_salt, recovery_wrapped_dek, recovery_wrap_iv, recovery_updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, CASE WHEN $10::int IS NULL THEN NULL ELSE now() END)`,
		userID, k.Version, k.KDF, k.Iterations, k.Salt, k.WrappedDEK, k.WrapIV, k.Verifier, k.VerifierIV,
		rIter, rSalt, rDEK, rIV)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrExists
	}
	return err
}

// SetRecovery menyimpan atau mengganti bungkus kunci pemulihan. Kunci
// pemulihan lama otomatis tidak berlaku lagi.
func (s *Store) SetRecovery(ctx context.Context, userID int64, r RecoveryWrap) error {
	if err := r.validate(); err != nil {
		return ValidationError{err}
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE journal_keys SET recovery_iterations = $2, recovery_salt = $3, recovery_wrapped_dek = $4,
			recovery_wrap_iv = $5, recovery_updated_at = now()
		WHERE user_id = $1`, userID, r.Iterations, r.Salt, r.WrappedDEK, r.WrapIV)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Rewrap mengganti bungkus DEK (ganti passphrase). DEK dan nilai uji tetap,
// jadi entri lama tidak perlu dienkripsi ulang.
func (s *Store) Rewrap(ctx context.Context, userID int64, w Wrap) error {
	if err := w.validate(); err != nil {
		return ValidationError{err}
	}
	tag, err := s.db.Exec(ctx, `
		UPDATE journal_keys
		SET kdf = $2, kdf_iterations = $3, salt = $4, wrapped_dek = $5, wrap_iv = $6, updated_at = now()
		WHERE user_id = $1`,
		userID, w.KDF, w.Iterations, w.Salt, w.WrappedDEK, w.WrapIV)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
