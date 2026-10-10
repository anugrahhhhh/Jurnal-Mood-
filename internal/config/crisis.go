package config

import (
	"errors"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// CrisisContact adalah satu kontak bantuan. Phone untuk ditampilkan,
// Dial untuk tautan tel: (boleh kosong jika kontak tidak punya nomor).
type CrisisContact struct {
	Name  string `yaml:"name"`
	Phone string `yaml:"phone"`
	Dial  string `yaml:"dial"`
	URL   string `yaml:"url"`
	Hours string `yaml:"hours"`
	Note  string `yaml:"note"`
}

type Crisis struct {
	Region string `yaml:"region"`
	// Verified harus diubah menjadi true oleh pengelola setelah setiap
	// nomor diperiksa. Selama false, halaman bantuan menampilkan peringatan.
	Verified   bool            `yaml:"verified"`
	Emergency  []CrisisContact `yaml:"emergency"`
	Counseling []CrisisContact `yaml:"counseling"`
}

func LoadCrisis(path string) (*Crisis, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Crisis
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("membaca %s: %w", path, err)
	}
	if len(c.Emergency) == 0 {
		return nil, errors.New(path + ": minimal satu kontak di 'emergency' wajib diisi")
	}
	return &c, nil
}
