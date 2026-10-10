package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"text/tabwriter"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"moodjournal/internal/admin"
	"moodjournal/internal/auth"
	"moodjournal/internal/config"
	"moodjournal/internal/db"
)

const adminUsage = `Perintah pengelola Jurnal Mood (dijalankan di server, bukan lewat web):

  admin stats                         angka ringkas, tanpa data pribadi per pengguna
  admin recent-signups [-hari 7]      pendaftaran terbaru, untuk mendeteksi akun spam
  admin reset-password <email>        buat password login sementara (catatan tetap aman)
  admin delete-user <email> [-ya]     hapus akun + semua datanya (tanpa -ya: hanya pratinjau)

Contoh di Docker:  docker compose exec app /app/moodjournal admin stats
`

func runAdmin(args []string, out io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprint(out, adminUsage)
		return nil
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
	return adminCommand(ctx, pool, args, out, time.Now())
}

// adminCommand dipisah dari runAdmin agar bisa diuji dengan database tes.
func adminCommand(ctx context.Context, pool *pgxpool.Pool, args []string, out io.Writer, now time.Time) error {
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "stats":
		s, err := admin.GetStats(ctx, pool, now)
		if err != nil {
			return err
		}
		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintf(w, "Pengguna terdaftar\t%d\n", s.Users)
		fmt.Fprintf(w, "Sudah membuat passphrase jurnal\t%d\n", s.UsersWithJournal)
		fmt.Fprintf(w, "Aktif mencatat, 7 hari terakhir\t%d\n", s.ActiveLast7Days)
		fmt.Fprintf(w, "Aktif mencatat, 30 hari terakhir\t%d\n", s.ActiveLast30Days)
		fmt.Fprintf(w, "Entri disimpan, 7 hari terakhir\t%d\n", s.EntriesLast7Days)
		fmt.Fprintf(w, "Pendaftaran baru, 7 hari terakhir\t%d\n", s.SignupsLast7Days)
		fmt.Fprintf(w, "Perangkat dengan pengingat\t%d\n", s.ReminderDevices)
		fmt.Fprintf(w, "Menunggu ganti password sementara\t%d\n", s.MustChangePassword)
		return w.Flush()

	case "recent-signups":
		fs := flag.NewFlagSet("recent-signups", flag.ContinueOnError)
		fs.SetOutput(out)
		days := fs.Int("hari", 7, "rentang hari ke belakang")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if *days < 1 || *days > 365 {
			return errors.New("-hari harus 1 sampai 365")
		}
		list, err := admin.RecentSignups(ctx, pool, now.AddDate(0, 0, -*days))
		if err != nil {
			return err
		}
		if len(list) == 0 {
			fmt.Fprintf(out, "Tidak ada pendaftaran dalam %d hari terakhir.\n", *days)
			return nil
		}
		w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "DAFTAR (UTC)\tEMAIL\tPASSPHRASE\tENTRI")
		for _, s := range list {
			journal := "belum"
			if s.HasJournal {
				journal = "sudah"
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", s.CreatedAt.UTC().Format("2006-01-02 15:04"), s.Email, journal, s.Entries)
		}
		if err := w.Flush(); err != nil {
			return err
		}
		fmt.Fprintf(out, "\n%d akun. Akun spam biasanya: banyak sekaligus, belum membuat passphrase, tanpa entri.\n", len(list))
		return nil

	case "reset-password":
		if len(rest) != 1 {
			return errors.New("pemakaian: admin reset-password <email>")
		}
		svc, err := auth.NewService(pool, false)
		if err != nil {
			return err
		}
		temp, err := svc.SetTemporaryPassword(ctx, rest[0])
		if err != nil {
			return err
		}
		slog.Info("admin: password sementara dibuat", "email", rest[0])
		fmt.Fprintf(out, `Password sementara untuk %s:

    %s

- Pastikan dulu yang meminta benar pemilik akun ini (mis. lewat chat pribadi).
- Kirim password ini lewat jalur pribadi. Ia wajib membuat password baru saat masuk.
- Semua sesi login akun ini sudah diakhiri.
- Passphrase jurnal dan semua catatan TIDAK berubah dan tetap terenkripsi.
`, rest[0], temp)
		return nil

	case "delete-user":
		fs := flag.NewFlagSet("delete-user", flag.ContinueOnError)
		fs.SetOutput(out)
		yes := fs.Bool("ya", false, "benar-benar menghapus (tanpa ini hanya pratinjau)")
		// Izinkan "-ya" sebelum atau sesudah email.
		var email string
		for len(rest) > 0 {
			if err := fs.Parse(rest); err != nil {
				return err
			}
			rest = fs.Args()
			if len(rest) > 0 {
				if email != "" {
					return errors.New("pemakaian: admin delete-user <email> [-ya]")
				}
				email, rest = rest[0], rest[1:]
			}
		}
		if email == "" {
			return errors.New("pemakaian: admin delete-user <email> [-ya]")
		}
		sum, err := admin.DeleteUserByEmail(ctx, pool, email, *yes)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Akun %s (daftar %s): %d entri, %d perangkat pengingat.\n",
			sum.Email, sum.CreatedAt.UTC().Format("2006-01-02"), sum.Entries, sum.Devices)
		if !sum.Deleted {
			fmt.Fprintln(out, "PRATINJAU: belum ada yang dihapus. Ulangi dengan -ya untuk menghapus permanen.")
			return nil
		}
		slog.Info("admin: akun dihapus", "email", sum.Email)
		fmt.Fprintln(out, "Akun dan semua datanya sudah dihapus permanen.")
		return nil
	}
	fmt.Fprint(out, adminUsage)
	return fmt.Errorf("perintah admin tidak dikenal: %q", cmd)
}
