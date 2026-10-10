// Package web berisi router HTTP, middleware, dan handler halaman.
package web

import (
	"context"
	"errors"
	"fmt"
	"io/fs"

	"moodjournal/internal/auth"
	"moodjournal/internal/config"
	"moodjournal/internal/entries"
	"moodjournal/internal/journal"
	"moodjournal/internal/reminders"
)

// Pinger adalah bagian database yang dibutuhkan health check.
type Pinger interface {
	Ping(ctx context.Context) error
}

type Deps struct {
	DB      Pinger
	Auth    *auth.Service
	Journal *journal.Store
	Entries *entries.Store
	Crisis  *config.Crisis
	Assets  fs.FS // harus berisi folder templates/ dan static/

	// Pengingat Web Push (opsional). RemindersEnabled false menyembunyikan fitur.
	Reminders        *reminders.Store
	RemindersEnabled bool
	VAPIDPublicKey   string
}

type Server struct {
	cfg     config.Config
	db      Pinger
	auth    *auth.Service
	journal *journal.Store
	entries *entries.Store
	crisis  *config.Crisis

	reminders        *reminders.Store
	remindersEnabled bool
	vapidPublicKey   string

	render *Renderer
	static fs.FS
}

func New(cfg config.Config, d Deps) (*Server, error) {
	if d.DB == nil || d.Auth == nil || d.Journal == nil || d.Entries == nil || d.Reminders == nil || d.Crisis == nil || d.Assets == nil {
		return nil, errors.New("web.New: semua dependensi wajib diisi")
	}
	r, err := NewRenderer(d.Assets)
	if err != nil {
		return nil, err
	}
	minAge := cfg.MinAge
	if minAge == 0 {
		minAge = 18
	}
	r.site = Site{OperatorName: cfg.OperatorName, OperatorContact: cfg.OperatorContact, MinAge: minAge}
	static, err := fs.Sub(d.Assets, "static")
	if err != nil {
		return nil, fmt.Errorf("folder static: %w", err)
	}
	return &Server{
		cfg: cfg, db: d.DB, auth: d.Auth, journal: d.Journal, entries: d.Entries, crisis: d.Crisis,
		reminders: d.Reminders, remindersEnabled: d.RemindersEnabled, vapidPublicKey: d.VAPIDPublicKey,
		render: r, static: static,
	}, nil
}
