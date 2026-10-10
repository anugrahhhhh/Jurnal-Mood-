package web

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	if currentUser(r) != nil {
		http.Redirect(w, r, "/beranda", http.StatusSeeOther)
		return
	}
	s.render.Page(w, http.StatusOK, "home", page(r, "Selamat datang", nil))
}

// handleHelp menampilkan kontak bantuan krisis. Sengaja bisa dibuka tanpa login.
func (s *Server) handleHelp(w http.ResponseWriter, r *http.Request) {
	s.render.Page(w, http.StatusOK, "bantuan", page(r, "Bantuan krisis", s.crisis))
}

type dashboardData struct {
	HasJournalKey bool
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	exists, ok := s.journalExists(w, r)
	if !ok {
		return
	}
	s.render.Page(w, http.StatusOK, "beranda", page(r, "Beranda", dashboardData{HasJournalKey: exists}))
}

func (s *Server) handleNotFound(w http.ResponseWriter, r *http.Request) {
	s.render.Page(w, http.StatusNotFound, "notfound", page(r, "Halaman tidak ditemukan", nil))
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	status, code := "ok", http.StatusOK
	if err := s.db.Ping(ctx); err != nil {
		status, code = "database tidak tersedia", http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"status": status})
}
