package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCrisisExample(t *testing.T) {
	c, err := LoadCrisis(filepath.Join("..", "..", "config", "crisis.example.yaml"))
	if err != nil {
		t.Fatalf("contoh kontak krisis harus valid: %v", err)
	}
	if c.Verified {
		t.Error("file contoh tidak boleh ditandai verified")
	}
	if len(c.Emergency) == 0 {
		t.Error("contoh harus punya kontak darurat")
	}
}

func TestLoadCrisisRequiresEmergency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "crisis.yaml")
	os.WriteFile(path, []byte("region: X\nverified: true\n"), 0o600)
	if _, err := LoadCrisis(path); err == nil {
		t.Error("harus gagal jika tidak ada kontak darurat")
	}
}

func TestDotEnvDoesNotOverrideEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	os.WriteFile(path, []byte("# komentar\nMJ_A=dari_file\nMJ_B=\"dikutip\"\n"), 0o600)
	t.Setenv("MJ_A", "dari_env")
	os.Unsetenv("MJ_B")
	t.Cleanup(func() { os.Unsetenv("MJ_B") })

	if err := loadDotEnv(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("MJ_A"); got != "dari_env" {
		t.Errorf("MJ_A = %q, env tidak boleh ditimpa", got)
	}
	if got := os.Getenv("MJ_B"); got != "dikutip" {
		t.Errorf("MJ_B = %q", got)
	}
}
