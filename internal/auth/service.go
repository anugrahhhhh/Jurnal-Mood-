// Package auth mengelola akun, kode undangan, dan sesi login.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	SessionTTL     = 14 * 24 * time.Hour
	MinPasswordLen = 10
	maxPasswordLen = 256 // byte; mencegah input raksasa ke Argon2
)

var (
	ErrInvalidCredentials = errors.New("email atau password salah")
	ErrTooManyAttempts    = errors.New("terlalu banyak percobaan masuk, coba lagi beberapa menit lagi")
	ErrInvalidEmail       = errors.New("alamat email tidak valid")
	ErrWeakPassword       = fmt.Errorf("password minimal %d karakter", MinPasswordLen)
	ErrPasswordTooLong    = fmt.Errorf("password maksimal %d byte", maxPasswordLen)
	ErrInviteInvalid      = errors.New("kode undangan tidak valid atau sudah dipakai")
	// ErrRegistrationFailed sengaja tidak menyebut "email sudah terdaftar",
	// agar orang lain tidak bisa mengecek siapa yang memakai aplikasi ini.
	ErrRegistrationFailed = errors.New("pendaftaran tidak bisa diproses, periksa kembali data kamu")
	ErrNoSession          = errors.New("sesi tidak ditemukan atau sudah berakhir")
	ErrTooManySignups     = errors.New("terlalu banyak pendaftaran dari jaringan ini, coba lagi dalam satu jam")
	ErrUserNotFound       = errors.New("akun dengan email itu tidak ditemukan")
	ErrSamePassword       = errors.New("password baru harus berbeda dari password saat ini")
)

type User struct {
	ID    int64
	Email string
	// MustChangePassword: pengelola memberi password sementara; pengguna wajib
	// membuat password baru sebelum memakai aplikasi lagi.
	MustChangePassword bool
}

type Service struct {
	db             *pgxpool.Pool
	inviteRequired bool
	now            func() time.Time
	byEmail        *limiter
	byIP           *limiter
	registrations  *limiter // pendaftaran per IP, menahan pembuatan akun massal
	dummyHash      string   // untuk menyamakan waktu respons saat email tidak ada
}

func NewService(db *pgxpool.Pool, inviteRequired bool) (*Service, error) {
	dummy, err := hashPassword("dummy-password-for-timing")
	if err != nil {
		return nil, err
	}
	return &Service{
		db:             db,
		inviteRequired: inviteRequired,
		now:            time.Now,
		byEmail:        newLimiter(5, 15*time.Minute),
		byIP:           newLimiter(30, 15*time.Minute),
		registrations:  newLimiter(5, time.Hour),
		dummyHash:      dummy,
	}, nil
}

func (s *Service) InviteRequired() bool { return s.inviteRequired }

func NormalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || len(email) > 254 {
		return "", ErrInvalidEmail
	}
	return email, nil
}

func checkPassword(pw string) error {
	if utf8.RuneCountInString(pw) < MinPasswordLen {
		return ErrWeakPassword
	}
	if len(pw) > maxPasswordLen {
		return ErrPasswordTooLong
	}
	return nil
}

// Register membuat akun baru. Jika undangan diwajibkan, kode undangan
// dipakai dalam transaksi yang sama sehingga tidak bisa dipakai dua kali.
func (s *Service) Register(ctx context.Context, email, password, invite string) (int64, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return 0, err
	}
	if err := checkPassword(password); err != nil {
		return 0, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return 0, err
	}

	var userID int64
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var inviteHash []byte
		if s.inviteRequired {
			inviteHash = hashInvite(invite)
			var found bool
			err := tx.QueryRow(ctx, `
				SELECT true FROM invites
				WHERE code_hash = $1 AND used_at IS NULL AND expires_at > $2
				FOR UPDATE`, inviteHash, s.now()).Scan(&found)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInviteInvalid
			}
			if err != nil {
				return err
			}
		}

		err := tx.QueryRow(ctx,
			`INSERT INTO users (email, password_hash, terms_accepted_at) VALUES ($1, $2, now()) RETURNING id`,
			email, hash).Scan(&userID)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation
			return ErrRegistrationFailed
		}
		if err != nil {
			return err
		}

		if s.inviteRequired {
			_, err = tx.Exec(ctx,
				`UPDATE invites SET used_at = $2, used_by = $3 WHERE code_hash = $1`,
				inviteHash, s.now(), userID)
		}
		return err
	})
	return userID, err
}

// SetSignupLimit mengubah batas pendaftaran per IP per jam (bawaan 5).
func (s *Service) SetSignupLimit(perHour int) {
	if perHour > 0 {
		s.registrations = newLimiter(perHour, time.Hour)
	}
}

// AllowSignup mencatat satu percobaan pendaftaran dari ip dan menolak jika
// sudah melewati batas per jam. Dipanggil sebelum Register.
func (s *Service) AllowSignup(ip string) error {
	now := s.now()
	key := "r:" + ip
	if s.registrations.blocked(key, now) {
		return ErrTooManySignups
	}
	s.registrations.fail(key, now)
	return nil
}

// Login memeriksa email dan password. ip dipakai untuk pembatasan percobaan.
func (s *Service) Login(ctx context.Context, email, password, ip string) (int64, error) {
	now := s.now()
	email, err := NormalizeEmail(email)
	if err != nil {
		return 0, ErrInvalidCredentials
	}
	emailKey, ipKey := "e:"+email, "i:"+ip
	if s.byEmail.blocked(emailKey, now) || s.byIP.blocked(ipKey, now) {
		return 0, ErrTooManyAttempts
	}
	if len(password) > maxPasswordLen {
		return 0, ErrInvalidCredentials
	}

	var id int64
	var hash string
	err = s.db.QueryRow(ctx, `SELECT id, password_hash FROM users WHERE email = $1`, email).Scan(&id, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		verifyPassword(password, s.dummyHash) // samakan waktu respons
		s.byEmail.fail(emailKey, now)
		s.byIP.fail(ipKey, now)
		return 0, ErrInvalidCredentials
	}
	if err != nil {
		return 0, err
	}
	ok, err := verifyPassword(password, hash)
	if err != nil {
		return 0, err
	}
	if !ok {
		s.byEmail.fail(emailKey, now)
		s.byIP.fail(ipKey, now)
		return 0, ErrInvalidCredentials
	}
	s.byEmail.clear(emailKey)
	return id, nil
}

// CheckPassword memastikan pengguna yang sudah masuk mengetahui password
// login-nya, untuk tindakan sensitif seperti mengganti passphrase jurnal.
func (s *Service) CheckPassword(ctx context.Context, userID int64, password string) error {
	now := s.now()
	key := fmt.Sprintf("u:%d", userID)
	if s.byEmail.blocked(key, now) {
		return ErrTooManyAttempts
	}
	if len(password) > maxPasswordLen {
		return ErrInvalidCredentials
	}
	var hash string
	if err := s.db.QueryRow(ctx, `SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&hash); err != nil {
		return err
	}
	ok, err := verifyPassword(password, hash)
	if err != nil {
		return err
	}
	if !ok {
		s.byEmail.fail(key, now)
		return ErrInvalidCredentials
	}
	s.byEmail.clear(key)
	return nil
}

// DeleteAccount menghapus akun beserta semua datanya (entri, kunci jurnal,
// pengaturan, sesi) lewat ON DELETE CASCADE. Butuh password login.
func (s *Service) DeleteAccount(ctx context.Context, userID int64, password string) error {
	if err := s.CheckPassword(ctx, userID, password); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `DELETE FROM users WHERE id = $1`, userID)
	return err
}

// SetTemporaryPassword dipakai pengelola (perintah admin) saat pengguna lupa
// password login. Password sementara acak dibuat, semua sesi akun itu
// dihapus, dan pengguna wajib menggantinya saat masuk. Passphrase jurnal dan
// semua catatan tidak tersentuh.
func (s *Service) SetTemporaryPassword(ctx context.Context, email string) (string, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return "", err
	}
	raw := make([]byte, 10)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	code := inviteEncoding.EncodeToString(raw)[:16]
	temp := code[0:4] + "-" + code[4:8] + "-" + code[8:12] + "-" + code[12:16]
	hash, err := hashPassword(temp)
	if err != nil {
		return "", err
	}
	err = pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		var id int64
		err := tx.QueryRow(ctx, `
			UPDATE users SET password_hash = $2, must_change_password = true
			WHERE email = $1 RETURNING id`, email, hash).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrUserNotFound
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, id)
		return err
	})
	if err != nil {
		return "", err
	}
	s.byEmail.clear("e:" + email)
	return temp, nil
}

// ChangePassword mengganti password login. Sesi lain milik akun ini dihapus
// (perangkat lain harus masuk lagi); sesi keepToken (perangkat ini) tetap.
func (s *Service) ChangePassword(ctx context.Context, userID int64, current, next, keepToken string) error {
	if err := s.CheckPassword(ctx, userID, current); err != nil {
		return err
	}
	if err := checkPassword(next); err != nil {
		return err
	}
	if next == current {
		return ErrSamePassword
	}
	hash, err := hashPassword(next)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, s.db, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE users SET password_hash = $2, must_change_password = false WHERE id = $1`, userID, hash); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1 AND token_hash <> $2`, userID, hashToken(keepToken))
		return err
	})
}

// PurgeExpired menghapus sesi kedaluwarsa dan kode undangan kedaluwarsa yang
// tidak terpakai. Dipanggil berkala oleh server.
func (s *Service) PurgeExpired(ctx context.Context) error {
	now := s.now()
	if _, err := s.db.Exec(ctx, `DELETE FROM sessions WHERE expires_at <= $1`, now); err != nil {
		return err
	}
	_, err := s.db.Exec(ctx, `DELETE FROM invites WHERE used_at IS NULL AND expires_at <= $1`, now)
	return err
}

// CreateSession membuat token sesi acak. Hanya hash-nya yang disimpan.
func (s *Service) CreateSession(ctx context.Context, userID int64) (token string, expires time.Time, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	now := s.now()
	expires = now.Add(SessionTTL)

	if _, err := s.db.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1 AND expires_at <= $2`, userID, now); err != nil {
		return "", time.Time{}, err
	}
	_, err = s.db.Exec(ctx,
		`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		hashToken(token), userID, expires)
	return token, expires, err
}

func (s *Service) UserBySession(ctx context.Context, token string) (*User, error) {
	if token == "" {
		return nil, ErrNoSession
	}
	var u User
	err := s.db.QueryRow(ctx, `
		SELECT u.id, u.email, u.must_change_password FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1 AND s.expires_at > $2`, hashToken(token), s.now()).Scan(&u.ID, &u.Email, &u.MustChangePassword)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoSession
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Service) DeleteSession(ctx context.Context, token string) error {
	_, err := s.db.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hashToken(token))
	return err
}

// CreateInvite membuat kode undangan sekali pakai, mis. "K7QX-M2PA-9TFD".
func (s *Service) CreateInvite(ctx context.Context, ttl time.Duration) (string, error) {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	code := inviteEncoding.EncodeToString(raw)[:12]
	_, err := s.db.Exec(ctx,
		`INSERT INTO invites (code_hash, expires_at) VALUES ($1, $2)`,
		hashInvite(code), s.now().Add(ttl))
	if err != nil {
		return "", err
	}
	return code[0:4] + "-" + code[4:8] + "-" + code[8:12], nil
}

// Alfabet Crockford base32: tanpa I, L, O, U agar tidak tertukar saat diketik.
var inviteEncoding = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)

// normalizeInvite membuang pemisah dan memetakan huruf yang mirip angka
// (O→0, I/L→1), sehingga salah ketik kecil tetap diterima.
func normalizeInvite(code string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '-', ' ':
			return -1
		case 'O':
			return '0'
		case 'I', 'L':
			return '1'
		}
		return r
	}, strings.ToUpper(code))
}

func hashInvite(code string) []byte {
	sum := sha256.Sum256([]byte("invite:" + normalizeInvite(code)))
	return sum[:]
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte("session:" + token))
	return sum[:]
}
