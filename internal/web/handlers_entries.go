package web

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"moodjournal/internal/entries"
)

type entryJSON struct {
	Date         string    `json:"date"`
	Mood         int       `json:"mood"`
	SleepMinutes int       `json:"sleepMinutes"`
	Version      int       `json:"version"`
	PayloadCT    []byte    `json:"payloadCt"`
	PayloadIV    []byte    `json:"payloadIv"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// saveEntryJSON: tanggal diambil dari URL. Tidak ada field teks biasa untuk
// aktivitas atau catatan; field tak dikenal ditolak decodeJSON.
type saveEntryJSON struct {
	Mood         int    `json:"mood"`
	SleepMinutes int    `json:"sleepMinutes"`
	Version      int    `json:"version"`
	PayloadCT    []byte `json:"payloadCt"`
	PayloadIV    []byte `json:"payloadIv"`
}

type blobJSON struct {
	CT []byte `json:"ct"`
	IV []byte `json:"iv"`
}

func (s *Server) handleEntryPage(w http.ResponseWriter, r *http.Request) {
	exists, ok := s.journalExists(w, r)
	if !ok {
		return
	}
	if !exists {
		http.Redirect(w, r, "/jurnal/buat", http.StatusSeeOther)
		return
	}
	s.render.Page(w, http.StatusOK, "hari_ini", page(r, "Catatan hari ini", nil))
}

func (s *Server) handleHistoryPage(w http.ResponseWriter, r *http.Request) {
	exists, ok := s.journalExists(w, r)
	if !ok {
		return
	}
	if !exists {
		http.Redirect(w, r, "/jurnal/buat", http.StatusSeeOther)
		return
	}
	s.render.Page(w, http.StatusOK, "riwayat", page(r, "Riwayat", nil))
}

// GET /api/entri?dari=YYYY-MM-DD&sampai=YYYY-MM-DD
func (s *Server) handleListEntries(w http.ResponseWriter, r *http.Request) {
	from, err1 := entries.ParseDate(r.URL.Query().Get("dari"))
	to, err2 := entries.ParseDate(r.URL.Query().Get("sampai"))
	if err := errors.Join(err1, err2); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Parameter dari dan sampai wajib berformat YYYY-MM-DD.")
		return
	}
	userID := currentUser(r).ID
	list, err := s.entries.Range(r.Context(), userID, from, to)
	if s.entryError(w, err) {
		return
	}
	// Nilai awal tombol tidur jika belum ada entri dalam rentang ini.
	sleepBefore, hasSleepBefore, err := s.entries.LastSleep(r.Context(), userID, from)
	if s.entryError(w, err) {
		return
	}

	out := struct {
		Entries     []entryJSON `json:"entries"`
		SleepBefore *int        `json:"sleepBeforeFrom"`
	}{Entries: make([]entryJSON, 0, len(list))}
	for _, e := range list {
		out.Entries = append(out.Entries, entryJSON{
			Date: e.DateString(), Mood: e.Mood, SleepMinutes: e.SleepMinutes, Version: e.Version,
			PayloadCT: e.PayloadCT, PayloadIV: e.PayloadIV, UpdatedAt: e.UpdatedAt,
		})
	}
	if hasSleepBefore {
		out.SleepBefore = &sleepBefore
	}
	writeJSON(w, http.StatusOK, out)
}

// PUT /api/entri/{tanggal}
func (s *Server) handleSaveEntry(w http.ResponseWriter, r *http.Request) {
	date, err := entries.ParseDate(chi.URLParam(r, "tanggal"))
	if s.entryError(w, err) {
		return
	}
	var in saveEntryJSON
	if !decodeJSON(w, r, &in) {
		return
	}
	err = s.entries.Save(r.Context(), currentUser(r).ID, entries.Entry{
		Date: date, Mood: in.Mood, SleepMinutes: in.SleepMinutes, Version: in.Version,
		PayloadCT: in.PayloadCT, PayloadIV: in.PayloadIV,
	})
	if s.entryError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleGetActivities menjawab 204 jika pengguna belum menyimpan daftar
// aktivitas sendiri (browser lalu memakai daftar bawaan). Itu bukan error.
func (s *Server) handleGetActivities(w http.ResponseWriter, r *http.Request) {
	b, err := s.entries.Activities(r.Context(), currentUser(r).ID)
	if errors.Is(err, entries.ErrNotFound) {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if s.entryError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, blobJSON{CT: b.CT, IV: b.IV})
}

func (s *Server) handleSaveActivities(w http.ResponseWriter, r *http.Request) {
	var in blobJSON
	if !decodeJSON(w, r, &in) {
		return
	}
	err := s.entries.SaveActivities(r.Context(), currentUser(r).ID, entries.Blob{CT: in.CT, IV: in.IV})
	if s.entryError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// entryError menulis respons untuk err (jika ada) dan mengembalikan true.
func (s *Server) entryError(w http.ResponseWriter, err error) bool {
	var verr entries.ValidationError
	switch {
	case err == nil:
		return false
	case errors.Is(err, entries.ErrNotFound):
		writeJSONError(w, http.StatusNotFound, err.Error())
	case errors.As(err, &verr):
		writeJSONError(w, http.StatusBadRequest, verr.Error())
	default:
		slog.Error("entri", "err", err)
		writeJSONError(w, http.StatusInternalServerError, "Terjadi kesalahan")
	}
	return true
}
