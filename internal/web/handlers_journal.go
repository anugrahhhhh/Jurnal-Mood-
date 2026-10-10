package web

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"

	"moodjournal/internal/auth"
	"moodjournal/internal/journal"
)

// keyJSON adalah bentuk material kunci di API. Field []byte otomatis
// di-encode base64 oleh encoding/json. Tidak ada field untuk passphrase
// atau DEK asli: field yang tidak dikenal ditolak (decodeJSON).
type keyJSON struct {
	Version    int    `json:"version"`
	KDF        string `json:"kdf"`
	Iterations int    `json:"iterations"`
	Salt       []byte `json:"salt"`
	WrappedKey []byte `json:"wrappedKey"`
	WrapIV     []byte `json:"wrapIv"`
	Verifier   []byte `json:"verifier"`
	VerifierIV []byte `json:"verifierIv"`
}

type rewrapJSON struct {
	LoginPassword string `json:"loginPassword"`
	KDF           string `json:"kdf"`
	Iterations    int    `json:"iterations"`
	Salt          []byte `json:"salt"`
	WrappedKey    []byte `json:"wrappedKey"`
	WrapIV        []byte `json:"wrapIv"`
}

// ---- Halaman ----

func (s *Server) journalExists(w http.ResponseWriter, r *http.Request) (bool, bool) {
	exists, err := s.journal.Exists(r.Context(), currentUser(r).ID)
	if err != nil {
		slog.Error("cek kunci jurnal", "err", err)
		http.Error(w, "Terjadi kesalahan", http.StatusInternalServerError)
		return false, false
	}
	return exists, true
}

func (s *Server) handleJournalCreatePage(w http.ResponseWriter, r *http.Request) {
	exists, ok := s.journalExists(w, r)
	if !ok {
		return
	}
	if exists {
		http.Redirect(w, r, "/jurnal/buka", http.StatusSeeOther)
		return
	}
	s.render.Page(w, http.StatusOK, "jurnal_buat", page(r, "Buat passphrase jurnal", nil))
}

func (s *Server) handleJournalUnlockPage(w http.ResponseWriter, r *http.Request) {
	exists, ok := s.journalExists(w, r)
	if !ok {
		return
	}
	if !exists {
		http.Redirect(w, r, "/jurnal/buat", http.StatusSeeOther)
		return
	}
	s.render.Page(w, http.StatusOK, "jurnal_buka", page(r, "Buka jurnal", nil))
}

func (s *Server) handleJournalChangePage(w http.ResponseWriter, r *http.Request) {
	exists, ok := s.journalExists(w, r)
	if !ok {
		return
	}
	if !exists {
		http.Redirect(w, r, "/jurnal/buat", http.StatusSeeOther)
		return
	}
	s.render.Page(w, http.StatusOK, "jurnal_ganti", page(r, "Ganti passphrase jurnal", nil))
}

// ---- API ----

func (s *Server) handleGetKey(w http.ResponseWriter, r *http.Request) {
	k, err := s.journal.Get(r.Context(), currentUser(r).ID)
	if errors.Is(err, journal.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	if err != nil {
		slog.Error("ambil kunci jurnal", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "Terjadi kesalahan")
		return
	}
	writeJSON(w, http.StatusOK, keyJSON{
		Version: k.Version, KDF: k.KDF, Iterations: k.Iterations, Salt: k.Salt,
		WrappedKey: k.WrappedDEK, WrapIV: k.WrapIV, Verifier: k.Verifier, VerifierIV: k.VerifierIV,
	})
}

func (s *Server) handleCreateKey(w http.ResponseWriter, r *http.Request) {
	var in keyJSON
	if !decodeJSON(w, r, &in) {
		return
	}
	err := s.journal.Create(r.Context(), currentUser(r).ID, journal.Key{
		Version: in.Version,
		Wrap: journal.Wrap{
			KDF: in.KDF, Iterations: in.Iterations, Salt: in.Salt, WrappedDEK: in.WrappedKey, WrapIV: in.WrapIV,
		},
		Verifier: in.Verifier, VerifierIV: in.VerifierIV,
	})
	var verr journal.ValidationError
	switch {
	case err == nil:
		w.WriteHeader(http.StatusCreated)
	case errors.Is(err, journal.ErrExists):
		writeJSONError(w, http.StatusConflict, err.Error())
	case errors.As(err, &verr):
		writeJSONError(w, http.StatusBadRequest, verr.Error())
	default:
		slog.Error("simpan kunci jurnal", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "Terjadi kesalahan")
	}
}

func (s *Server) handleRewrapKey(w http.ResponseWriter, r *http.Request) {
	var in rewrapJSON
	if !decodeJSON(w, r, &in) {
		return
	}
	userID := currentUser(r).ID
	switch err := s.auth.CheckPassword(r.Context(), userID, in.LoginPassword); {
	case errors.Is(err, auth.ErrInvalidCredentials):
		writeJSONError(w, http.StatusForbidden, "Password login salah.")
		return
	case errors.Is(err, auth.ErrTooManyAttempts):
		writeJSONError(w, http.StatusTooManyRequests, err.Error())
		return
	case err != nil:
		slog.Error("cek password", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "Terjadi kesalahan")
		return
	}

	err := s.journal.Rewrap(r.Context(), userID, journal.Wrap{
		KDF: in.KDF, Iterations: in.Iterations, Salt: in.Salt, WrappedDEK: in.WrappedKey, WrapIV: in.WrapIV,
	})
	var verr journal.ValidationError
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, journal.ErrNotFound):
		writeJSONError(w, http.StatusNotFound, err.Error())
	case errors.As(err, &verr):
		writeJSONError(w, http.StatusBadRequest, verr.Error())
	default:
		slog.Error("ganti bungkus kunci", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "Terjadi kesalahan")
	}
}

// ---- Helper JSON ----

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "application/json" {
		writeJSONError(w, http.StatusUnsupportedMediaType, "Content-Type harus application/json")
		return false
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeJSONError(w, http.StatusBadRequest, "JSON tidak valid")
		return false
	}
	if dec.Decode(&struct{}{}) != io.EOF {
		writeJSONError(w, http.StatusBadRequest, "JSON tidak valid")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// requireUserAPI menjawab 401 JSON (bukan redirect) untuk fetch dari browser.
func requireUserAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u := currentUser(r)
		if u == nil {
			writeJSONError(w, http.StatusUnauthorized, "Sesi berakhir, silakan masuk lagi.")
			return
		}
		if u.MustChangePassword {
			writeJSONError(w, http.StatusForbidden, "Buat password login baru dulu.")
			return
		}
		next.ServeHTTP(w, r)
	})
}
