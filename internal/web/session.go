package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"moodjournal/internal/auth"
)

type ctxKey int

const (
	ctxUser ctxKey = iota
	ctxCSRF
)

// Di production cookie memakai awalan __Host- (wajib Secure, tanpa Domain,
// Path=/) agar tidak bisa ditimpa subdomain lain.
func (s *Server) sessionCookieName() string {
	if s.cfg.Production() {
		return "__Host-mj_session"
	}
	return "mj_session"
}

func (s *Server) csrfCookieName() string {
	if s.cfg.Production() {
		return "__Host-mj_csrf"
	}
	return "mj_csrf"
}

func (s *Server) setCookie(w http.ResponseWriter, name, value string, expires time.Time) {
	c := &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.Production(),
		SameSite: http.SameSiteLaxMode,
	}
	if expires.IsZero() {
		c.MaxAge = -1
	} else {
		c.Expires = expires
	}
	http.SetCookie(w, c)
}

// loadUser membaca cookie sesi dan menaruh pengguna (jika ada) di context.
func (s *Server) loadUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(s.sessionCookieName())
		if err == nil && c.Value != "" {
			u, err := s.auth.UserBySession(r.Context(), c.Value)
			switch {
			case err == nil:
				r = r.WithContext(context.WithValue(r.Context(), ctxUser, u))
			case errors.Is(err, auth.ErrNoSession):
				s.setCookie(w, s.sessionCookieName(), "", time.Time{})
			default:
				slog.Error("memuat sesi", "err", err)
			}
		}
		next.ServeHTTP(w, r)
	})
}

func currentUser(r *http.Request) *auth.User {
	u, _ := r.Context().Value(ctxUser).(*auth.User)
	return u
}

// requireUser mengarahkan tamu ke halaman masuk.
// Pengguna dengan password sementara dari pengelola hanya boleh membuka
// halaman ganti password (dan keluar) sampai membuat password baru.
func requireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if u == nil {
			http.Redirect(w, r, "/masuk", http.StatusSeeOther)
			return
		}
		if u.MustChangePassword && r.URL.Path != passwordPath && r.URL.Path != "/keluar" {
			http.Redirect(w, r, passwordPath, http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

const passwordPath = "/akun/password"

// requireGuest mengarahkan pengguna yang sudah masuk ke beranda.
func requireGuest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if currentUser(r) != nil {
			http.Redirect(w, r, "/beranda", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

const csrfField = "csrf"

// csrf memakai pola double-submit: token acak di cookie HttpOnly harus sama
// dengan token di form (atau header X-CSRF-Token untuk HTMX). Origin juga
// diperiksa bila dikirim browser.
func (s *Server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := ""
		if c, err := r.Cookie(s.csrfCookieName()); err == nil && len(c.Value) >= 32 {
			token = c.Value
		} else {
			token = newCSRFToken()
			s.setCookie(w, s.csrfCookieName(), token, time.Now().Add(auth.SessionTTL))
		}

		if !isSafeMethod(r.Method) {
			if !sameOrigin(r) {
				http.Error(w, "Permintaan ditolak (origin tidak cocok).", http.StatusForbidden)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
			sent := r.Header.Get("X-CSRF-Token")
			if sent == "" {
				sent = r.PostFormValue(csrfField)
			}
			if subtle.ConstantTimeCompare([]byte(sent), []byte(token)) != 1 {
				http.Error(w, "Sesi formulir kedaluwarsa. Muat ulang halaman lalu coba lagi.", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxCSRF, token)))
	})
}

func isSafeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Browser modern selalu mengirim Origin untuk POST; jika tidak ada,
		// andalkan token CSRF saja.
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == r.Host
}

func newCSRFToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand tidak gagal pada sistem yang sehat
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func csrfToken(r *http.Request) string {
	t, _ := r.Context().Value(ctxCSRF).(string)
	return t
}

// startSession membuat sesi baru dan mengganti token CSRF (mencegah fixation).
func (s *Server) startSession(w http.ResponseWriter, r *http.Request, userID int64) error {
	token, expires, err := s.auth.CreateSession(r.Context(), userID)
	if err != nil {
		return err
	}
	s.setCookie(w, s.sessionCookieName(), token, expires)
	s.setCookie(w, s.csrfCookieName(), newCSRFToken(), expires)
	return nil
}
