package web

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"moodjournal/internal/auth"
)

const deleteConfirmWord = "HAPUS"

type settingsData struct {
	Notice           string
	Error            string
	ConfirmWord      string
	HasJournalKey    bool
	RemindersEnabled bool
}

// handlePrivacy sengaja bisa dibuka tanpa login, agar calon pengguna bisa
// membacanya sebelum mendaftar.
func (s *Server) handlePrivacy(w http.ResponseWriter, r *http.Request) {
	s.render.Page(w, http.StatusOK, "privasi", page(r, "Privasi", nil))
}

func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	exists, ok := s.journalExists(w, r)
	if !ok {
		return
	}
	data := settingsData{ConfirmWord: deleteConfirmWord, HasJournalKey: exists, RemindersEnabled: s.remindersEnabled}
	if r.URL.Query().Get("password") == "diganti" {
		data.Notice = "Password login berhasil diganti. Perangkat lain perlu masuk lagi."
	}
	s.render.Page(w, http.StatusOK, "pengaturan", page(r, "Pengaturan", data))
}

type passwordData struct {
	Forced         bool // password sementara dari pengelola
	Error          string
	MinPasswordLen int
}

func (s *Server) handlePasswordForm(w http.ResponseWriter, r *http.Request) {
	s.render.Page(w, http.StatusOK, "akun_password", page(r, "Ganti password login",
		passwordData{Forced: currentUser(r).MustChangePassword, MinPasswordLen: auth.MinPasswordLen}))
}

func (s *Server) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	fail := func(status int, msg string) {
		s.render.Page(w, status, "akun_password", page(r, "Ganti password login",
			passwordData{Forced: user.MustChangePassword, Error: msg, MinPasswordLen: auth.MinPasswordLen}))
	}
	next := r.PostFormValue("new_password")
	if next != r.PostFormValue("new_password_confirm") {
		fail(http.StatusUnprocessableEntity, "Konfirmasi password baru tidak sama.")
		return
	}
	token := ""
	if c, err := r.Cookie(s.sessionCookieName()); err == nil {
		token = c.Value
	}
	err := s.auth.ChangePassword(r.Context(), user.ID, r.PostFormValue("current_password"), next, token)
	switch {
	case err == nil:
		http.Redirect(w, r, "/pengaturan?password=diganti", http.StatusSeeOther)
	case errors.Is(err, auth.ErrInvalidCredentials):
		fail(http.StatusUnprocessableEntity, "Password saat ini salah.")
	case errors.Is(err, auth.ErrTooManyAttempts):
		fail(http.StatusTooManyRequests, err.Error())
	case errors.Is(err, auth.ErrWeakPassword), errors.Is(err, auth.ErrPasswordTooLong), errors.Is(err, auth.ErrSamePassword):
		fail(http.StatusUnprocessableEntity, err.Error())
	default:
		slog.Error("ganti password", "err", err)
		fail(http.StatusInternalServerError, "Terjadi kesalahan. Password belum diganti.")
	}
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	user := currentUser(r)
	fail := func(status int, msg string) {
		exists, _ := s.journal.Exists(r.Context(), user.ID)
		s.render.Page(w, status, "pengaturan", page(r, "Pengaturan",
			settingsData{Error: msg, ConfirmWord: deleteConfirmWord, HasJournalKey: exists, RemindersEnabled: s.remindersEnabled}))
	}

	if r.PostFormValue("confirm") != deleteConfirmWord {
		fail(http.StatusUnprocessableEntity, "Ketik "+deleteConfirmWord+" (huruf besar) untuk mengonfirmasi.")
		return
	}
	switch err := s.auth.DeleteAccount(r.Context(), user.ID, r.PostFormValue("password")); {
	case errors.Is(err, auth.ErrInvalidCredentials):
		fail(http.StatusUnprocessableEntity, "Password login salah.")
		return
	case errors.Is(err, auth.ErrTooManyAttempts):
		fail(http.StatusTooManyRequests, err.Error())
		return
	case err != nil:
		slog.Error("hapus akun", "err", err)
		fail(http.StatusInternalServerError, "Terjadi kesalahan. Akun belum dihapus.")
		return
	}

	s.setCookie(w, s.sessionCookieName(), "", time.Time{})
	s.setCookie(w, s.csrfCookieName(), "", time.Time{})
	// ID dikirim agar halaman berikutnya bisa membersihkan draf terenkripsi
	// milik akun ini dari penyimpanan browser. ID bukan rahasia.
	http.Redirect(w, r, "/akun-dihapus?u="+strconv.FormatInt(user.ID, 10), http.StatusSeeOther)
}

func (s *Server) handleAccountDeleted(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.ParseInt(r.URL.Query().Get("u"), 10, 64)
	s.render.Page(w, http.StatusOK, "akun_dihapus", page(r, "Akun dihapus", id))
}
