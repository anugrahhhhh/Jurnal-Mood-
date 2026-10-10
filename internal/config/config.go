// Package config membaca pengaturan aplikasi dari variabel lingkungan
// dan file .env (opsional, untuk pengembangan).
package config

import (
	"bufio"
	"errors"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Addr        string // alamat HTTP server, mis. 127.0.0.1:8080
	Env         string // "development" atau "production"
	DatabaseURL string
	CrisisFile  string // file YAML berisi kontak bantuan krisis
	// InviteRequired: pendaftaran terbuka (REGISTRATION=open, default) atau
	// hanya dengan kode undangan (REGISTRATION=invite).
	InviteRequired bool
	// TrustProxy: aplikasi berjalan di balik reverse proxy (mis. Caddy) yang
	// menulis IP klien ke X-Forwarded-For. Jangan aktifkan tanpa proxy, karena
	// header itu bisa dipalsukan klien.
	TrustProxy bool
	// SignupsPerHour: batas pendaftaran per IP per jam (SIGNUP_LIMIT_PER_HOUR, bawaan 5).
	SignupsPerHour int
	// Kunci VAPID untuk pengingat Web Push (opsional). Jika kosong, fitur
	// pengingat disembunyikan. Buat dengan: go run ./cmd/server vapid
	VAPIDPublicKey  string
	VAPIDPrivateKey string
	VAPIDSubject    string // email atau URL https pengelola, dikirim ke layanan push
}

func (c Config) Production() bool { return c.Env == "production" }

// Load membaca .env (jika ada) lalu variabel lingkungan.
// Variabel lingkungan yang sudah diset tidak ditimpa oleh .env.
func Load() (Config, error) {
	if err := loadDotEnv(".env"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Config{}, err
	}
	c := Config{
		Addr:        getenv("ADDR", "127.0.0.1:8080"),
		Env:         getenv("APP_ENV", "development"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		CrisisFile:  getenv("CRISIS_FILE", "config/crisis.yaml"),

		VAPIDPublicKey:  os.Getenv("VAPID_PUBLIC_KEY"),
		VAPIDPrivateKey: os.Getenv("VAPID_PRIVATE_KEY"),
		VAPIDSubject:    os.Getenv("VAPID_SUBJECT"),
	}
	switch reg := getenv("REGISTRATION", "open"); reg {
	case "invite":
		c.InviteRequired = true
	case "open":
		c.InviteRequired = false
	default:
		return c, errors.New("REGISTRATION harus 'invite' atau 'open', bukan " + reg)
	}
	switch v := getenv("TRUST_PROXY", "false"); v {
	case "true":
		c.TrustProxy = true
	case "false":
	default:
		return c, errors.New("TRUST_PROXY harus 'true' atau 'false', bukan " + v)
	}
	c.SignupsPerHour = 5
	if v := os.Getenv("SIGNUP_LIMIT_PER_HOUR"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return c, errors.New("SIGNUP_LIMIT_PER_HOUR harus bilangan bulat >= 1")
		}
		c.SignupsPerHour = n
	}
	if c.DatabaseURL == "" {
		return c, errors.New("DATABASE_URL belum diisi (salin .env.example menjadi .env)")
	}
	return c, nil
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

// loadDotEnv mendukung format sederhana KEY=VALUE, komentar (#) dan baris kosong.
func loadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, value)
		}
	}
	return sc.Err()
}
