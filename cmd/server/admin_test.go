package main

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"moodjournal/internal/auth"
	"moodjournal/internal/testdb"
)

func TestAdminCommands(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	svc, _ := auth.NewService(pool, false)
	if _, err := svc.Register(ctx, "teman@contoh.id", "kuda-laut-biru-42", ""); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (string, error) {
		var out bytes.Buffer
		err := adminCommand(ctx, pool, args, &out, time.Now())
		return out.String(), err
	}

	out, err := run("stats")
	if err != nil || !strings.Contains(out, "Pengguna terdaftar") {
		t.Errorf("stats: %v\n%s", err, out)
	}
	if strings.Contains(out, "teman@contoh.id") {
		t.Error("stats tidak boleh menampilkan data per pengguna")
	}

	out, err = run("recent-signups", "-hari", "3")
	if err != nil || !strings.Contains(out, "teman@contoh.id") || !strings.Contains(out, "belum") {
		t.Errorf("recent-signups: %v\n%s", err, out)
	}
	if _, err := run("recent-signups", "-hari", "0"); err == nil {
		t.Error("-hari 0 harus ditolak")
	}

	out, err = run("reset-password", "teman@contoh.id")
	if err != nil {
		t.Fatal(err)
	}
	temp := regexp.MustCompile(`[0-9A-Z]{4}-[0-9A-Z]{4}-[0-9A-Z]{4}-[0-9A-Z]{4}`).FindString(out)
	if temp == "" || !strings.Contains(out, "TIDAK berubah") {
		t.Fatalf("reset-password harus mencetak password sementara dan penjelasan:\n%s", out)
	}
	if _, err := svc.Login(ctx, "teman@contoh.id", temp, "1.1.1.1"); err != nil {
		t.Errorf("password sementara dari perintah harus berlaku: %v", err)
	}
	if _, err := run("reset-password"); err == nil {
		t.Error("reset-password tanpa email harus gagal")
	}

	out, err = run("delete-user", "teman@contoh.id")
	if err != nil || !strings.Contains(out, "PRATINJAU") {
		t.Fatalf("delete-user tanpa -ya harus pratinjau: %v\n%s", err, out)
	}
	out, err = run("delete-user", "-ya", "teman@contoh.id")
	if err != nil || !strings.Contains(out, "sudah dihapus") {
		t.Fatalf("delete-user -ya: %v\n%s", err, out)
	}
	if _, err := run("delete-user", "teman@contoh.id", "-ya"); err == nil {
		t.Error("menghapus akun yang sudah tidak ada harus gagal")
	}
	if _, err := run("delete-user"); err == nil {
		t.Error("delete-user tanpa email harus gagal")
	}

	if _, err := run("tidak-ada"); err == nil {
		t.Error("perintah tidak dikenal harus gagal")
	}
	var help bytes.Buffer
	if err := runAdmin(nil, &help); err != nil || !strings.Contains(help.String(), "reset-password") {
		t.Errorf("bantuan: %v\n%s", err, help.String())
	}
}
