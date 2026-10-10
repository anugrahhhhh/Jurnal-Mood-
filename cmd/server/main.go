// Perintah:
//
//	go run ./cmd/server               menjalankan server
//	go run ./cmd/server invite [-hari N]  membuat kode undangan sekali pakai
//	go run ./cmd/server healthcheck       cek /healthz (untuk HEALTHCHECK container)
//	go run ./cmd/server vapid             membuat kunci VAPID untuk pengingat Web Push
//	go run ./cmd/server admin <perintah>  perintah pengelola (jalankan tanpa argumen untuk bantuan)
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/SherClockHolmes/webpush-go"

	"moodjournal/internal/auth"
	"moodjournal/internal/config"
	"moodjournal/internal/db"
	"moodjournal/internal/entries"
	"moodjournal/internal/journal"
	"moodjournal/internal/reminders"
	"moodjournal/internal/web"
	assets "moodjournal/web"

	// Data zona waktu di dalam binary: image container tidak punya tzdata,
	// padahal pengingat dihitung menurut zona waktu pengguna.
	_ "time/tzdata"
)

func main() {
	var err error
	switch {
	case len(os.Args) > 1 && os.Args[1] == "invite":
		err = runInvite(os.Args[2:])
	case len(os.Args) > 1 && os.Args[1] == "healthcheck":
		err = runHealthcheck()
	case len(os.Args) > 1 && os.Args[1] == "vapid":
		err = runVAPID()
	case len(os.Args) > 1 && os.Args[1] == "admin":
		err = runAdmin(os.Args[2:], os.Stdout)
	default:
		err = runServer()
	}
	if err != nil {
		slog.Error("gagal", "err", err)
		os.Exit(1)
	}
}

func runServer() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	crisis, err := loadCrisis(cfg.CrisisFile)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := db.Migrate(ctx, cfg.DatabaseURL); err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	authSvc, err := auth.NewService(pool, cfg.InviteRequired)
	if err != nil {
		return err
	}
	authSvc.SetSignupLimit(cfg.SignupsPerHour)
	if cfg.OperatorName == "" || cfg.OperatorContact == "" {
		slog.Warn("identitas pengelola belum diisi (OPERATOR_NAME, OPERATOR_CONTACT); wajib sebelum dipakai orang lain")
	}

	go purgeLoop(ctx, authSvc)

	vapid := reminders.VAPID{PublicKey: cfg.VAPIDPublicKey, PrivateKey: cfg.VAPIDPrivateKey, Subject: cfg.VAPIDSubject}
	reminderStore := reminders.NewStore(pool)
	if vapid.Enabled() {
		go reminderStore.Run(ctx, reminders.NewWebPushSender(vapid))
	} else {
		slog.Info("pengingat Web Push nonaktif (VAPID_* belum diisi)")
	}

	srv, err := web.New(cfg, web.Deps{
		DB: pool, Auth: authSvc, Journal: journal.NewStore(pool), Entries: entries.NewStore(pool),
		Reminders: reminderStore, VAPIDPublicKey: vapid.PublicKey, RemindersEnabled: vapid.Enabled(),
		Crisis: crisis, Assets: assets.FS,
	})
	if err != nil {
		return err
	}

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		slog.Info("server berjalan", "url", "http://"+cfg.Addr, "env", cfg.Env, "undangan_wajib", cfg.InviteRequired)
		errc <- httpSrv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		slog.Info("mematikan server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpSrv.Shutdown(shutdownCtx)
	}
	return nil
}

// purgeLoop membersihkan sesi dan undangan kedaluwarsa setiap jam.
func purgeLoop(ctx context.Context, svc *auth.Service) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if err := svc.PurgeExpired(ctx); err != nil && ctx.Err() == nil {
			slog.Error("membersihkan sesi kedaluwarsa", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// runHealthcheck memanggil /healthz pada server lokal. Image container tidak
// punya curl, jadi binary ini sendiri yang dipakai untuk HEALTHCHECK.
func runHealthcheck() error {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:" + port + "/healthz")
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz: status %d", resp.StatusCode)
	}
	return nil
}

func runVAPID() error {
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return err
	}
	fmt.Println("Tambahkan ke .env (atau deploy/.env.prod). Kunci privat jangan dibagikan:")
	fmt.Println()
	fmt.Println("VAPID_PUBLIC_KEY=" + pub)
	fmt.Println("VAPID_PRIVATE_KEY=" + priv)
	fmt.Println("VAPID_SUBJECT=mailto:email-pengelola@contoh.id")
	return nil
}

func runInvite(args []string) error {
	fs := flag.NewFlagSet("invite", flag.ContinueOnError)
	days := fs.Int("hari", 7, "masa berlaku kode dalam hari")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *days < 1 {
		return errors.New("-hari minimal 1")
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	if err := db.Migrate(ctx, cfg.DatabaseURL); err != nil {
		return err
	}
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	svc, err := auth.NewService(pool, true)
	if err != nil {
		return err
	}
	code, err := svc.CreateInvite(ctx, time.Duration(*days)*24*time.Hour)
	if err != nil {
		return err
	}
	fmt.Printf("Kode undangan (sekali pakai, berlaku %d hari):\n\n    %s\n\n", *days, code)
	fmt.Println("Kode ini tidak disimpan dalam bentuk asli; catat sekarang.")
	return nil
}

// loadCrisis memakai file contoh jika file utama belum dibuat, agar halaman
// bantuan tetap ada. Halaman akan menampilkan peringatan "belum diverifikasi".
func loadCrisis(path string) (*config.Crisis, error) {
	c, err := config.LoadCrisis(path)
	if errors.Is(err, os.ErrNotExist) {
		example := filepath.Join(filepath.Dir(path), "crisis.example.yaml")
		slog.Warn("file kontak krisis belum ada, memakai contoh", "file", path, "contoh", example)
		return config.LoadCrisis(example)
	}
	if err == nil && !c.Verified {
		slog.Warn("kontak krisis belum diverifikasi; ubah 'verified: true' setelah memeriksa setiap nomor", "file", path)
	}
	return c, err
}
