package web

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"moodjournal/internal/auth"
)

type authForm struct {
	Email          string // diisi ulang saat ada error; password tidak pernah
	Invite         string // dari tautan undangan (?kode=) atau isian sebelumnya
	Error          string
	InviteRequired bool
	MinPasswordLen int
}

func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	s.render.Page(w, http.StatusOK, "masuk", page(r, "Masuk", authForm{}))
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	email := r.PostFormValue("email")
	userID, err := s.auth.Login(r.Context(), email, r.PostFormValue("password"), s.clientIP(r))
	if err != nil {
		status := http.StatusUnauthorized
		msg := auth.ErrInvalidCredentials.Error()
		switch {
		case errors.Is(err, auth.ErrTooManyAttempts):
			status, msg = http.StatusTooManyRequests, err.Error()
		case !errors.Is(err, auth.ErrInvalidCredentials):
			slog.Error("login", "err", err)
			status, msg = http.StatusInternalServerError, "Terjadi kesalahan. Coba lagi sebentar lagi."
		}
		s.render.Page(w, status, "masuk", page(r, "Masuk", authForm{Email: email, Error: msg}))
		return
	}
	if err := s.startSession(w, r, userID); err != nil {
		slog.Error("membuat sesi", "err", err)
		http.Error(w, "Terjadi kesalahan", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/beranda", http.StatusSeeOther)
}

func (s *Server) registerForm(email, invite, msg string) authForm {
	return authForm{Email: email, Invite: invite, Error: msg, InviteRequired: s.auth.InviteRequired(), MinPasswordLen: auth.MinPasswordLen}
}

func (s *Server) handleRegisterForm(w http.ResponseWriter, r *http.Request) {
	// Tautan undangan: /daftar?kode=XXXX-XXXX-XXXX mengisi kolom kode otomatis.
	s.render.Page(w, http.StatusOK, "daftar", page(r, "Daftar", s.registerForm("", r.URL.Query().Get("kode"), "")))
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	email := r.PostFormValue("email")
	password := r.PostFormValue("password")

	fail := func(status int, msg string) {
		s.render.Page(w, status, "daftar", page(r, "Daftar", s.registerForm(email, r.PostFormValue("invite"), msg)))
	}
	if password != r.PostFormValue("password_confirm") {
		fail(http.StatusUnprocessableEntity, "Konfirmasi password tidak sama.")
		return
	}

	if err := s.auth.AllowSignup(s.clientIP(r)); err != nil {
		fail(http.StatusTooManyRequests, err.Error())
		return
	}
	userID, err := s.auth.Register(r.Context(), email, password, r.PostFormValue("invite"))
	switch {
	case err == nil:
	case errors.Is(err, auth.ErrInvalidEmail), errors.Is(err, auth.ErrWeakPassword),
		errors.Is(err, auth.ErrPasswordTooLong), errors.Is(err, auth.ErrInviteInvalid),
		errors.Is(err, auth.ErrRegistrationFailed):
		fail(http.StatusUnprocessableEntity, err.Error())
		return
	default:
		slog.Error("registrasi", "err", err) // error database tidak ditampilkan ke pengguna
		fail(http.StatusInternalServerError, "Terjadi kesalahan. Coba lagi sebentar lagi.")
		return
	}

	if err := s.startSession(w, r, userID); err != nil {
		slog.Error("membuat sesi", "err", err)
		http.Error(w, "Terjadi kesalahan", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/jurnal/buat", http.StatusSeeOther) // langkah 2 onboarding
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(s.sessionCookieName()); err == nil {
		if err := s.auth.DeleteSession(r.Context(), c.Value); err != nil {
			slog.Error("menghapus sesi", "err", err)
		}
	}
	s.setCookie(w, s.sessionCookieName(), "", time.Time{})
	s.setCookie(w, s.csrfCookieName(), "", time.Time{})
	http.Redirect(w, r, "/masuk", http.StatusSeeOther)
}

// clientIP dipakai untuk membatasi percobaan login. Tanpa TRUST_PROXY, alamat
// koneksi langsung yang dipakai. Dengan TRUST_PROXY, dipakai entri terakhir
// X-Forwarded-For: entri yang ditambahkan proxy kita sendiri, sehingga entri
// palsu yang dikirim klien di depannya diabaikan.
func (s *Server) clientIP(r *http.Request) string {
	if s.cfg.TrustProxy {
		if values := r.Header.Values("X-Forwarded-For"); len(values) > 0 {
			parts := strings.Split(values[len(values)-1], ",")
			if ip := net.ParseIP(strings.TrimSpace(parts[len(parts)-1])); ip != nil {
				return ip.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
