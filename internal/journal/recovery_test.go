package journal

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func validRecovery() *RecoveryWrap {
	return &RecoveryWrap{Iterations: MinRecoveryIterations, Salt: randBytes(16), WrappedDEK: randBytes(48), WrapIV: randBytes(12)}
}

func TestRecoveryValidation(t *testing.T) {
	if err := validRecovery().validate(); err != nil {
		t.Fatalf("bungkus pemulihan valid ditolak: %v", err)
	}
	for name, mutate := range map[string]func(*RecoveryWrap){
		"iterasi rendah": func(r *RecoveryWrap) { r.Iterations = 1000 },
		"salt":           func(r *RecoveryWrap) { r.Salt = randBytes(4) },
		"dek":            func(r *RecoveryWrap) { r.WrappedDEK = randBytes(32) },
		"iv":             func(r *RecoveryWrap) { r.WrapIV = nil },
	} {
		r := validRecovery()
		mutate(r)
		if r.validate() == nil {
			t.Errorf("%s: diterima padahal tidak valid", name)
		}
	}
}

func TestCreateWithRecoveryAndReplace(t *testing.T) {
	s, id := newUser(t)
	ctx := context.Background()

	k := validKey()
	k.Recovery = validRecovery()
	if err := s.Create(ctx, id, k); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, id)
	if err != nil || got.Recovery == nil || !bytes.Equal(got.Recovery.WrappedDEK, k.Recovery.WrappedDEK) {
		t.Fatalf("bungkus pemulihan tidak tersimpan: %+v %v", got, err)
	}

	r2 := validRecovery()
	if err := s.SetRecovery(ctx, id, *r2); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Get(ctx, id)
	if !bytes.Equal(got.Recovery.Salt, r2.Salt) {
		t.Error("kunci pemulihan baru harus menggantikan yang lama")
	}
	if !bytes.Equal(got.WrappedDEK, k.WrappedDEK) || !bytes.Equal(got.Verifier, k.Verifier) {
		t.Error("mengganti kunci pemulihan tidak boleh mengubah bungkus passphrase atau nilai uji")
	}

	if err := s.SetRecovery(ctx, id+999, *validRecovery()); !errors.Is(err, ErrNotFound) {
		t.Errorf("pengguna tanpa kunci jurnal: %v", err)
	}
	bad := validRecovery()
	bad.Iterations = 10
	var verr ValidationError
	if err := s.SetRecovery(ctx, id, *bad); !errors.As(err, &verr) {
		t.Errorf("iterasi rendah harus ditolak: %v", err)
	}
}

func TestKeyWithoutRecovery(t *testing.T) {
	s, id := newUser(t)
	ctx := context.Background()
	if err := s.Create(ctx, id, validKey()); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, id)
	if err != nil || got.Recovery != nil {
		t.Errorf("akun lama tanpa kunci pemulihan: %+v %v", got, err)
	}
}
