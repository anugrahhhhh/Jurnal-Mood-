package web

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(accessLog)
	r.Use(middleware.Recoverer)
	r.Use(s.securityHeaders)

	r.Handle("/static/*", http.StripPrefix("/static/", s.staticHandler()))
	r.Get("/healthz", s.handleHealth)
	r.Get("/sw.js", s.handleServiceWorker)
	r.Get("/manifest.webmanifest", s.handleManifest)

	r.Group(func(r chi.Router) {
		r.Use(s.csrf, s.loadUser)

		r.Get("/", s.handleHome)
		r.Get("/bantuan", s.handleHelp) // sengaja bisa dibuka tanpa login
		r.Get("/privasi", s.handlePrivacy)
		r.Get("/akun-dihapus", s.handleAccountDeleted)

		r.Group(func(r chi.Router) {
			r.Use(requireGuest)
			r.Get("/masuk", s.handleLoginForm)
			r.Post("/masuk", s.handleLogin)
			r.Get("/daftar", s.handleRegisterForm)
			r.Post("/daftar", s.handleRegister)
		})

		r.Group(func(r chi.Router) {
			r.Use(requireUser)
			r.Get("/beranda", s.handleDashboard)
			r.Post("/keluar", s.handleLogout)
			r.Get("/jurnal/buat", s.handleJournalCreatePage)
			r.Get("/jurnal/buka", s.handleJournalUnlockPage)
			r.Get("/jurnal/ganti", s.handleJournalChangePage)
			r.Get("/hari-ini", s.handleEntryPage)
			r.Get("/riwayat", s.handleHistoryPage)
			r.Get("/pengaturan", s.handleSettings)
			r.Post("/akun/hapus", s.handleDeleteAccount)
			r.Get(passwordPath, s.handlePasswordForm)
			r.Post(passwordPath, s.handleChangePassword)
		})

		r.Route("/api", func(r chi.Router) {
			r.Use(requireUserAPI)
			r.Get("/jurnal/kunci", s.handleGetKey)
			r.Post("/jurnal/kunci", s.handleCreateKey)
			r.Put("/jurnal/kunci", s.handleRewrapKey)
			r.Get("/entri", s.handleListEntries)
			r.Put("/entri/{tanggal}", s.handleSaveEntry)
			r.Get("/aktivitas", s.handleGetActivities)
			r.Put("/aktivitas", s.handleSaveActivities)
			r.Get("/pengingat", s.handleGetReminders)
			r.Put("/pengingat", s.handleSaveReminder)
			r.Delete("/pengingat", s.handleDeleteReminder)
		})
	})

	r.NotFound(s.handleNotFound)
	return r
}

func (s *Server) staticHandler() http.Handler {
	files := http.FileServer(http.FS(s.static))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.Production() {
			w.Header().Set("Cache-Control", "no-cache")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=3600")
		}
		files.ServeHTTP(w, r)
	})
}
