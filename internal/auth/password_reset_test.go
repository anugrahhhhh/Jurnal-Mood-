package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestTemporaryPasswordAndChange(t *testing.T) {
	s := newService(t, false)
	ctx := context.Background()
	id, err := s.Register(ctx, "a@contoh.id", goodPassword, "")
	if err != nil {
		t.Fatal(err)
	}
	oldSession, _, _ := s.CreateSession(ctx, id)

	if _, err := s.SetTemporaryPassword(ctx, "tidak@ada.id"); !errors.Is(err, ErrUserNotFound) {
		t.Errorf("email tidak ada: %v", err)
	}
	temp, err := s.SetTemporaryPassword(ctx, " A@Contoh.id ")
	if err != nil {
		t.Fatal(err)
	}
	if len(temp) != 19 || strings.Count(temp, "-") != 3 {
		t.Errorf("format password sementara tidak terduga: %q", temp)
	}
	if _, err := s.UserBySession(ctx, oldSession); !errors.Is(err, ErrNoSession) {
		t.Error("semua sesi lama harus diakhiri saat password direset")
	}
	if _, err := s.Login(ctx, "a@contoh.id", goodPassword, "1.1.1.1"); !errors.Is(err, ErrInvalidCredentials) {
		t.Error("password lama tidak boleh berlaku lagi")
	}
	if _, err := s.Login(ctx, "a@contoh.id", temp, "1.1.1.1"); err != nil {
		t.Fatalf("password sementara harus bisa dipakai masuk: %v", err)
	}
	keep, _, _ := s.CreateSession(ctx, id)
	other, _, _ := s.CreateSession(ctx, id)
	u, _ := s.UserBySession(ctx, keep)
	if !u.MustChangePassword {
		t.Fatal("pengguna wajib ganti password setelah reset")
	}

	if err := s.ChangePassword(ctx, id, "salah-salah-salah", "password-baru-123", keep); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("password saat ini salah: %v", err)
	}
	if err := s.ChangePassword(ctx, id, temp, "pendek", keep); !errors.Is(err, ErrWeakPassword) {
		t.Errorf("password baru pendek: %v", err)
	}
	if err := s.ChangePassword(ctx, id, temp, temp, keep); !errors.Is(err, ErrSamePassword) {
		t.Errorf("password baru sama: %v", err)
	}
	if err := s.ChangePassword(ctx, id, temp, "password-baru-123", keep); err != nil {
		t.Fatal(err)
	}
	u, err = s.UserBySession(ctx, keep)
	if err != nil || u.MustChangePassword {
		t.Errorf("sesi perangkat ini tetap, dan kewajiban ganti password hilang: %+v %v", u, err)
	}
	if _, err := s.UserBySession(ctx, other); !errors.Is(err, ErrNoSession) {
		t.Error("sesi di perangkat lain harus diakhiri setelah ganti password")
	}
	if _, err := s.Login(ctx, "a@contoh.id", "password-baru-123", "1.1.1.1"); err != nil {
		t.Errorf("password baru harus berlaku: %v", err)
	}
}
