package journal

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"testing"

	"moodjournal/internal/auth"
	"moodjournal/internal/testdb"
)

func randBytes(n int) []byte {
	b := make([]byte, n)
	rand.Read(b)
	return b
}

func validWrap() Wrap {
	return Wrap{KDF: KDF, Iterations: MinIterations, Salt: randBytes(16), WrappedDEK: randBytes(48), WrapIV: randBytes(12)}
}

func validKey() Key {
	return Key{Version: FormatVersion, Wrap: validWrap(), Verifier: randBytes(40), VerifierIV: randBytes(12)}
}

func TestValidation(t *testing.T) {
	cases := map[string]func(*Key){
		"versi":        func(k *Key) { k.Version = 2 },
		"kdf":          func(k *Key) { k.KDF = "SHA1" },
		"iterasi":      func(k *Key) { k.Iterations = 100_000 },
		"iterasi maks": func(k *Key) { k.Iterations = 50_000_000 },
		"salt":         func(k *Key) { k.Salt = randBytes(8) },
		"dek":          func(k *Key) { k.WrappedDEK = randBytes(32) },
		"wrap iv":      func(k *Key) { k.WrapIV = randBytes(16) },
		"verifier":     func(k *Key) { k.Verifier = nil },
		"verifier iv":  func(k *Key) { k.VerifierIV = randBytes(0) },
	}
	if err := validKey().validate(); err != nil {
		t.Fatalf("kunci valid ditolak: %v", err)
	}
	for name, mutate := range cases {
		k := validKey()
		mutate(&k)
		if k.validate() == nil {
			t.Errorf("%s: input tidak valid diterima", name)
		}
	}
}

func newUser(t *testing.T) (*Store, int64) {
	t.Helper()
	pool := testdb.Open(t)
	svc, err := auth.NewService(pool, false)
	if err != nil {
		t.Fatal(err)
	}
	id, err := svc.Register(context.Background(), "a@contoh.id", "kuda-laut-biru-42", "")
	if err != nil {
		t.Fatal(err)
	}
	return NewStore(pool), id
}

func TestCreateGetRewrap(t *testing.T) {
	s, id := newUser(t)
	ctx := context.Background()

	if _, err := s.Get(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("sebelum dibuat: %v", err)
	}
	if err := s.Rewrap(ctx, id, validWrap()); !errors.Is(err, ErrNotFound) {
		t.Errorf("rewrap sebelum dibuat: %v", err)
	}

	k := validKey()
	if err := s.Create(ctx, id, k); err != nil {
		t.Fatal(err)
	}
	if err := s.Create(ctx, id, validKey()); !errors.Is(err, ErrExists) {
		t.Errorf("kunci yang ada tidak boleh ditimpa: %v", err)
	}

	got, err := s.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.WrappedDEK, k.WrappedDEK) || !bytes.Equal(got.Verifier, k.Verifier) {
		t.Error("data tersimpan berbeda dari yang dikirim")
	}

	w := validWrap()
	if err := s.Rewrap(ctx, id, w); err != nil {
		t.Fatal(err)
	}
	got, _ = s.Get(ctx, id)
	if !bytes.Equal(got.WrappedDEK, w.WrappedDEK) || !bytes.Equal(got.Salt, w.Salt) {
		t.Error("rewrap tidak menyimpan bungkus baru")
	}
	if !bytes.Equal(got.Verifier, k.Verifier) {
		t.Error("rewrap tidak boleh mengubah nilai uji")
	}

	bad := validWrap()
	bad.Iterations = 1000
	var verr ValidationError
	if err := s.Rewrap(ctx, id, bad); !errors.As(err, &verr) {
		t.Errorf("rewrap dengan iterasi rendah: %v", err)
	}
}
