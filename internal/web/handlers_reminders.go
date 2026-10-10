package web

import (
	"errors"
	"io/fs"
	"net/http"

	"moodjournal/internal/reminders"
)

type reminderJSON struct {
	Endpoint string `json:"endpoint"`
	Time     string `json:"time"`
}

type remindersStatusJSON struct {
	Enabled       bool           `json:"enabled"`
	PublicKey     string         `json:"publicKey,omitempty"`
	Subscriptions []reminderJSON `json:"subscriptions"`
}

type saveReminderJSON struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	Time     string `json:"time"`
	Timezone string `json:"timezone"`
}

type deleteReminderJSON struct {
	Endpoint string `json:"endpoint"`
}

func (s *Server) handleGetReminders(w http.ResponseWriter, r *http.Request) {
	out := remindersStatusJSON{Enabled: s.remindersEnabled, Subscriptions: []reminderJSON{}}
	if !s.remindersEnabled {
		writeJSON(w, http.StatusOK, out)
		return
	}
	out.PublicKey = s.vapidPublicKey
	subs, err := s.reminders.ForUser(r.Context(), currentUser(r).ID)
	if s.reminderError(w, err) {
		return
	}
	for _, sub := range subs {
		out.Subscriptions = append(out.Subscriptions, reminderJSON{Endpoint: sub.Endpoint, Time: reminders.FormatTime(sub.RemindMinute)})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleSaveReminder(w http.ResponseWriter, r *http.Request) {
	if !s.remindersEnabled {
		writeJSONError(w, http.StatusNotFound, "Pengingat belum diaktifkan oleh pengelola.")
		return
	}
	var in saveReminderJSON
	if !decodeJSON(w, r, &in) {
		return
	}
	minute, err := reminders.ParseTime(in.Time)
	if s.reminderError(w, err) {
		return
	}
	err = s.reminders.Save(r.Context(), reminders.Subscription{
		UserID: currentUser(r).ID, Endpoint: in.Endpoint, P256dh: in.Keys.P256dh, Auth: in.Keys.Auth,
		RemindMinute: minute, Timezone: in.Timezone,
	})
	if s.reminderError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteReminder(w http.ResponseWriter, r *http.Request) {
	var in deleteReminderJSON
	if !decodeJSON(w, r, &in) {
		return
	}
	err := s.reminders.Delete(r.Context(), currentUser(r).ID, in.Endpoint)
	if errors.Is(err, reminders.ErrNotFound) {
		w.WriteHeader(http.StatusNoContent) // sudah tidak ada: hasil yang diinginkan tercapai
		return
	}
	if s.reminderError(w, err) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) reminderError(w http.ResponseWriter, err error) bool {
	var verr reminders.ValidationError
	switch {
	case err == nil:
		return false
	case errors.As(err, &verr):
		writeJSONError(w, http.StatusBadRequest, verr.Error())
	default:
		return s.entryError(w, err) // log + 500 generik
	}
	return true
}

// handleServiceWorker menyajikan service worker dari root agar cakupannya
// seluruh situs. Service worker ini hanya menampilkan notifikasi; ia tidak
// menyimpan halaman apa pun ke cache (halaman bisa berisi teks terdekripsi).
func (s *Server) handleServiceWorker(w http.ResponseWriter, r *http.Request) {
	s.serveStaticFile(w, r, "js/sw.js", "text/javascript; charset=utf-8")
}

func (s *Server) handleManifest(w http.ResponseWriter, r *http.Request) {
	s.serveStaticFile(w, r, "manifest.webmanifest", "application/manifest+json")
}

func (s *Server) serveStaticFile(w http.ResponseWriter, r *http.Request, name, contentType string) {
	b, err := fs.ReadFile(s.static, name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(b)
}
