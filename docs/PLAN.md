# Rencana Implementasi Bertahap

Mengacu ke [SPEC.md](SPEC.md). Setiap tahap berakhir dengan sesuatu yang bisa dijalankan dan diuji, dan tahap berikutnya baru dimulai setelah tahap sebelumnya disetujui.

## Prasyarat
- Go 1.23+ (saat ini `go` belum ada di PATH mesin ini).
- PostgreSQL (versi 14 atau lebih baru), dari sumber mana saja: **Laragon** (Menu → Tools → Quick add → PostgreSQL), installer resmi, atau Docker. Docker tidak wajib. Aplikasi cukup membaca `DATABASE_URL` dari `.env`.
- Node.js (opsional), hanya untuk menjalankan tes modul kripto JavaScript.
- Di laptop ini: Smart App Control memblokir program Go buatan sendiri, jadi server dan tes Go dijalankan di WSL (Ubuntu) lewat `scripts/dev.sh`, dengan `networkingMode=mirrored` di `%USERPROFILE%\.wslconfig`.

## Struktur folder

```
Project-golang/
├── cmd/
│   └── server/
│       └── main.go              # baca config, sambung DB, jalankan HTTP server
├── internal/
│   ├── config/                  # env + file config (termasuk kontak krisis)
│   ├── db/                      # koneksi pgxpool, helper transaksi
│   ├── migrations/              # file SQL berurutan (goose), di-embed
│   ├── auth/                    # hash Argon2id, sesi, undangan, rate limit
│   ├── journal/                 # salt/DEK terbungkus/verifier per pengguna
│   ├── entries/                 # simpan/ambil entri, data grafik
│   ├── reminders/               # Web Push (tahap 6)
│   └── web/
│       ├── router.go            # rute Chi
│       ├── middleware.go        # CSP & security headers, CSRF, sesi, logging
│       ├── render.go            # parsing & render template
│       └── handlers_*.go        # satu file per area: auth, journal, entries, pages
├── web/
│   ├── templates/
│   │   ├── layouts/base.html    # header dengan tautan "Bantuan krisis" + footer disclaimer
│   │   ├── pages/               # login, register, unlock, today, history, help, privacy, settings
│   │   └── partials/            # fragmen HTMX: ringkasan 7 hari, form entri, dll.
│   ├── static/
│   │   ├── css/app.css          # tema variasi B, token warna per mood
│   │   ├── js/
│   │   │   ├── crypto.js        # PBKDF2, DEK, wrap/unwrap, enkripsi entri, verifier
│   │   │   ├── session-key.js   # kunci di memori, auto-lock, draf terenkripsi
│   │   │   ├── entry-form.js    # enkripsi sebelum kirim (hook htmx), UI mood/tidur/chip
│   │   │   ├── history.js       # grafik, korelasi aktivitas, kartu pesan lembut
│   │   │   └── sw.js            # service worker untuk pengingat
│   │   └── vendor/              # htmx.min.js, chart library: di-host sendiri
│   └── embed.go                 # go:embed templates & static
├── config/
│   └── crisis.example.yaml      # contoh kontak krisis, wajib diverifikasi
├── tests/js/                    # tes crypto.js (node --test)
├── docs/SPEC.md, docs/PLAN.md
├── compose.yaml                 # opsional: PostgreSQL via Docker
├── .env.example
├── Makefile                     # run, test, migrate
└── go.mod
```

Library Go: `go-chi/chi/v5`, `jackc/pgx/v5`, `pressly/goose/v3`, `golang.org/x/crypto/argon2`, `gopkg.in/yaml.v3`, `SherClockHolmes/webpush-go` (khusus tahap 6). CSRF memakai implementasi kecil sendiri (double-submit token yang terikat ke sesi), supaya dependensinya sedikit.

## Skema database (garis besar)

```
users         id, email (unik), password_hash, created_at
invites       code_hash, created_by, used_by, expires_at
sessions      token_hash, user_id, expires_at, created_at
journal_keys  user_id (PK), kdf, kdf_iterations, salt, wrapped_dek, wrap_iv,
              verifier_ct, verifier_iv, format_version, updated_at
entries       id, user_id, entry_date, mood SMALLINT CHECK (1..5),
              sleep_minutes INT CHECK (>=0 AND %30=0),
              payload_ct BYTEA, payload_iv BYTEA, format_version,
              created_at, updated_at, UNIQUE(user_id, entry_date)
user_settings user_id, activities_ct, activities_iv   -- daftar chip, terenkripsi
push_subs     user_id, endpoint, keys, remind_at       -- tahap 6
```

Tidak ada kolom flag risiko di tabel mana pun.

## Tahapan

### Tahap 0: Fondasi ✅ selesai (10 Okt 2026)
- `go mod init`, config dari env (`DATABASE_URL`), `compose.yaml` opsional, migrasi goose di-embed dan dijalankan saat start.
- Router Chi dengan `/healthz` dan layout dasar yang sudah memuat tautan "Bantuan krisis" serta footer disclaimer.
- Middleware security headers: CSP ketat, HSTS di produksi, `X-Content-Type-Options`, `Referrer-Policy: no-referrer`.
- Halaman `/bantuan` dibaca dari `config/crisis.yaml` dan bisa dibuka tanpa login.
- **Selesai jika:** `make run` menampilkan halaman bantuan, tes header CSP lulus.

### Tahap 1: Akun ✅ selesai (10 Okt 2026)
- Register dengan kode undangan (bisa dinonaktifkan lewat config), login, logout.
- Argon2id, sesi di DB, CSRF, rate limit login per IP+email.
- Perintah CLI kecil untuk admin membuat kode undangan.
- **Selesai jika:** tes handler untuk register/login/logout/CSRF lulus terhadap DB uji.

### Tahap 2: Kunci jurnal (inti privasi) ✅ selesai, diuji di browser (10 Okt 2026)
- `crypto.js`: buat DEK, turunkan KEK (PBKDF2-SHA-256, 600k iterasi), wrap/unwrap, verifier, enkripsi/dekripsi payload dengan IV acak dan AAD `user_id|tanggal`, plus nomor versi format.
- Halaman buat passphrase (dengan peringatan jelas bahwa passphrase tidak bisa dipulihkan), buka kunci, ganti passphrase (re-wrap saja).
- `session-key.js`: kunci hanya di memori, auto-lock 15 menit.
- **Selesai jika:** tes `node --test` lulus. Tes harus memastikan: round-trip berhasil, passphrase salah ditolak, IV selalu berbeda, AAD yang salah ditolak, dan entri tetap terbaca setelah passphrase diganti. Tes Go harus memastikan server tidak pernah menerima field plaintext.

### Tahap 3: Entri harian (layar variasi B) ✅ selesai, diuji di browser (10 Okt 2026)
- Layar hari ini: mood (geseran + ketuk), tidur ±30 menit dengan nilai awal dari entri sebelumnya, chip aktivitas termasuk tambah chip sendiri, catatan opsional.
- Hook `htmx:configRequest`: aktivitas dan catatan dienkripsi sebelum dikirim. Input plaintext tidak punya atribut `name`, jadi tidak mungkin ikut terkirim.
- Simpan/edit hari ini dan kemarin, draf otomatis terenkripsi di localStorage, ringkasan 7 hari tanpa streak setelah menyimpan.
- Tema warna mengikuti mood, mendukung `prefers-reduced-motion`, kontras AA.
- **Selesai jika:** entri tersimpan, isi DB untuk aktivitas dan catatan hanya berupa ciphertext (diperiksa lewat tes), dan alur bisa diselesaikan di HP dalam waktu kurang dari 1 menit.

### Tahap 4: Riwayat & grafik ✅ selesai, diuji di browser (10 Okt 2026)
- Endpoint JSON mood/tidur per rentang 7/30/90 hari, dan grafik di browser.
- Dekripsi payload di browser untuk menghitung korelasi aktivitas dengan mood, ditampilkan dengan bahasa netral.
- Kartu pesan lembut: 3 hari kalender berturut-turut dengan skor ≤ 2, dihitung di browser, bisa ditutup, dan status tutupnya disimpan di browser.
- **Selesai jika:** tes JS untuk logika "3 hari berturut-turut" (termasuk kasus hari yang bolong dan pergantian bulan) dan untuk korelasi lulus. Juga diperiksa bahwa tidak ada request ke server saat kartu muncul atau ditutup.

### Tahap 5: Halaman kepercayaan & kontrol data 🟡 kode selesai & diuji di browser; menunggu tinjauan teks oleh pemilik proyek
- Halaman privasi: apa yang terenkripsi, apa yang terlihat admin (skor, tidur, tanggal, ukuran entri, email, jam pengingat), dan batasan JavaScript yang dikirim server, semuanya dalam bahasa awam.
- Disclaimer saat pendaftaran, ditampilkan sebagai teks informasi. **[Usulan]** Tanpa checkbox paksaan.
- Hapus akun beserta semua data (konfirmasi dengan password login).
- **Selesai jika:** pemilik proyek sudah meninjau teks halaman privasi dan bantuan.

### Tahap 6: Pengingat harian (opsional) 🟡 kode & tes selesai; pengiriman push sungguhan belum diuji di browser
- Service worker, Web Push dengan kunci VAPID, pengaturan jam pengingat, dan isi notifikasi yang generik.
- **Selesai jika:** notifikasi muncul di Android Chrome. iOS hanya didukung jika situs dipasang ke layar utama.

### Tahap 7: Siap dipakai komunitas 🟡 berkas deploy selesai & diuji lokal (docs/DEPLOY.md); belum dipasang di server sungguhan
- Dockerfile, panduan deploy di balik HTTPS, backup Postgres terjadwal (isinya sudah aman karena catatan terenkripsi), log tanpa data sensitif.
- Checklist manual: uji di HP, CSP tanpa pelanggaran, verifikasi nomor krisis.

### Tahap 8: Tampilan responsif & dashboard ✅ selesai (11 Okt 2026)
- Sesuai mockup `.superpowers/brainstorm/*/content/responsive-v1.html` yang disetujui.
- Sidebar di layar >= 900px (laptop, iPad lanskap); bar atas + tab bar bawah dengan tombol ＋ di HP dan iPad potret.
- Mode gelap otomatis mengikuti perangkat (semua warna lewat token CSS).
- Landing page lengkap (fitur, cara kerja enkripsi, FAQ), masuk/daftar dua kolom, onboarding 3 langkah.
- Beranda menjadi dashboard: hari ini, grafik 7 hari, rata-rata tidur & mood, kalender bulan ini, pola aktivitas.
- Riwayat: kalender mood per bulan dengan detail hari, rentang Minggu/Bulan/3 bulan/Tahun.
- Pendaftaran terbuka (keputusan pemilik proyek) dengan batas 5 pendaftaran per IP per jam.
- Tes end-to-end Playwright (`npm run e2e`) di 4 ukuran layar: alur lengkap, tanpa error JS/CSP,
  tanpa scroll horizontal, navigasi yang benar per ukuran layar, dan mode gelap.

### Tahap 9: Kesiapan peluncuran ✅ selesai (11 Okt 2026)
- Git (repository lokal, akhir baris LF) dan `govulncheck`: 0 celah yang memengaruhi kode.
- Identitas pengelola (OPERATOR_NAME, OPERATOR_CONTACT), halaman /syarat, usia minimum (MIN_AGE, bawaan 18),
  kotak persetujuan wajib saat daftar dengan waktu persetujuan tercatat.
- Kunci pemulihan passphrase (120 bit, dibuat di browser, hanya bungkusnya yang disimpan server):
  ditampilkan sekali saat membuat jurnal, bisa dibuat ulang di Pengaturan, dipakai di /jurnal/pulihkan.
- Ekspor semua catatan (JSON & CSV, didekripsi di browser, aman dari CSV injection).
- Perintah admin (stats, recent-signups, reset-password, delete-user) dan ganti password login.
- `scripts/smoke.sh` untuk pemeriksaan otomatis setelah deploy + checklist uji manual di HP.
- **Masih perlu pemilik proyek:** isi identitas pengelola, verifikasi kontak krisis, deploy ke VPS, uji di HP sungguhan.

## Keputusan yang masih perlu Anda konfirmasi
1. Pendaftaran lewat **kode undangan** (usulan saya) atau terbuka untuk umum?
2. Daftar **kontak krisis** dan wilayahnya. Saya hanya menyiapkan contoh, dan nomornya wajib Anda verifikasi.
3. Apakah **pengingat (tahap 6)** tetap masuk MVP, atau ditunda?
