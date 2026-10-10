# Panduan Deploy Jurnal Mood

Panduan ini untuk memasang Jurnal Mood di satu server (VPS) bagi komunitas kecil. Susunannya:

```
internet ──HTTPS──> Caddy (sertifikat otomatis) ──> app (Go) ──> PostgreSQL
                                                        backup harian ──> deploy/backups/
```

Semua berjalan dengan Docker Compose dari folder `deploy/`. Susunan ini sudah diuji lokal
pada 10 Okt 2026: build image, HTTPS lewat Caddy, healthcheck, kode undangan, backup, dan restore.

## 1. Yang dibutuhkan

- **VPS Linux** dengan Docker dan Docker Compose v2. 1 vCPU dan 1 GB RAM cukup untuk komunitas kecil.
- **Domain** (atau subdomain), misalnya `jurnal.contoh.id`, dengan DNS A/AAAA record mengarah ke IP VPS.
- Port **80 dan 443** terbuka dari internet. Caddy butuh port 80 untuk mendapatkan sertifikat.
- Akses SSH dengan kunci, bukan password.

## 2. Persiapan server (sekali)

```bash
# Firewall: hanya SSH, HTTP, HTTPS.
sudo ufw allow OpenSSH && sudo ufw allow 80/tcp && sudo ufw allow 443/tcp && sudo ufw enable
# Update keamanan otomatis (Ubuntu/Debian).
sudo apt install -y unattended-upgrades
```

## 3. Memasang aplikasi

```bash
git clone <repo-ini> moodjournal && cd moodjournal

# 3a. Kontak krisis. WAJIB diperiksa sebelum dipakai orang lain.
cp config/crisis.example.yaml config/crisis.yaml
nano config/crisis.yaml        # periksa setiap nomor, lalu ubah: verified: true

# 3b. Rahasia dan domain.
cp deploy/.env.prod.example deploy/.env.prod
openssl rand -hex 24           # pakai hasilnya untuk DB_PASSWORD
nano deploy/.env.prod          # isi DOMAIN dan DB_PASSWORD
chmod 600 deploy/.env.prod

# 3c. Jalankan.
cd deploy
docker compose --env-file .env.prod up -d --build
docker compose ps              # app dan db harus "healthy"
```

Buka `https://<DOMAIN>/bantuan`. Kalau masih ada peringatan "belum diverifikasi", berarti
`config/crisis.yaml` belum ada atau `verified` belum `true`.

> `config/crisis.yaml` harus sudah ada **sebelum** `up`. Kalau belum, Docker membuat folder
> kosong dengan nama itu dan aplikasi gagal start.

## 4. Pendaftaran anggota

Secara bawaan pendaftaran **terbuka** (`REGISTRATION=open`): siapa pun bisa daftar di `/daftar`.
Untuk menahan pembuatan akun massal, setiap IP dibatasi 5 pendaftaran per jam
(`SIGNUP_LIMIT_PER_HOUR`).

Ingin khusus undangan? Isi `REGISTRATION=invite` di `.env.prod`, jalankan ulang, lalu buat kode:

```bash
cd deploy
docker compose exec app /app/moodjournal invite -hari 7
```

Kirim kodenya lewat jalur pribadi, atau kirim tautan `https://<DOMAIN>/daftar?kode=<KODE>` yang
mengisi kode otomatis. Kode hanya bisa dipakai sekali dan tidak disimpan dalam bentuk asli.

## 4b. Perintah pengelola

Tidak ada halaman admin di website (agar tidak ada sasaran serangan tambahan). Semua tugas pengelola
dijalankan dari terminal server:

```bash
cd deploy
docker compose exec app /app/moodjournal admin                     # bantuan
docker compose exec app /app/moodjournal admin stats               # angka ringkas, tanpa data per orang
docker compose exec app /app/moodjournal admin recent-signups -hari 7
docker compose exec app /app/moodjournal admin reset-password teman@contoh.id
docker compose exec app /app/moodjournal admin delete-user teman@contoh.id      # pratinjau
docker compose exec app /app/moodjournal admin delete-user teman@contoh.id -ya  # hapus permanen
```

**Lupa password login.** Pastikan dulu yang meminta benar pemilik akun (mis. lewat chat pribadi yang
sudah kamu kenal), lalu jalankan `reset-password`. Kirim password sementara lewat jalur pribadi. Saat
masuk, ia wajib membuat password baru; semua sesi lama diakhiri. **Passphrase jurnal tidak bisa
direset oleh siapa pun**, dan catatannya tetap terenkripsi.

**Akun spam.** `recent-signups` menandai akun yang belum membuat passphrase dan tanpa entri. Hapus
dengan `delete-user ... -ya`, atau sementara ganti ke `REGISTRATION=invite`.

## 5. Backup dan restore

Container `backup` membuat satu file `deploy/backups/moodjournal-YYYYMMDD-HHMM.dump` setiap hari
dan menyimpannya selama `BACKUP_KEEP_DAYS` hari (bawaan 14).

**Isi backup:** catatan dan aktivitas tetap terenkripsi, tapi **email, skor mood, jam tidur, dan
tanggal tidak terenkripsi**. Perlakukan file backup sebagai data pribadi:

- Simpan salinannya di luar VPS, misalnya dengan `rsync` ke komputer pribadi atau penyimpanan
  yang dienkripsi. Backup yang hanya ada di VPS yang sama ikut hilang kalau VPS rusak.
- Jangan unggah ke penyimpanan publik.

**Backup manual** (mis. sebelum update):

```bash
docker compose exec -T db pg_dump -U moodjournal -Fc moodjournal > backups/manual-$(date +%Y%m%d).dump
```

**Restore** (menimpa database yang sedang dipakai):

```bash
docker compose stop app
docker compose exec -T db dropdb -U moodjournal moodjournal
docker compose exec -T db createdb -U moodjournal moodjournal
docker compose exec -T db pg_restore -U moodjournal -d moodjournal < backups/<file>.dump
docker compose start app
```

Coba restore ke database sementara sekali sebulan untuk memastikan backup benar-benar bisa dipakai.

## 6. Update aplikasi

```bash
git pull
cd deploy
docker compose exec -T db pg_dump -U moodjournal -Fc moodjournal > backups/sebelum-update-$(date +%Y%m%d).dump
docker compose --env-file .env.prod up -d --build
```

Migrasi database berjalan otomatis saat aplikasi start.

## 7. Privasi di server

- Aplikasi mencatat metode, path, status, dan durasi, **tanpa alamat IP**. Caddy dikonfigurasi
  tanpa log akses. Lihat log dengan `docker compose logs app`.
- Container aplikasi berjalan sebagai pengguna non-root, dengan filesystem read-only dan tanpa
  capability Linux, serta tidak membuka port langsung ke internet.
- `TRUST_PROXY=true` hanya aman karena aplikasi **hanya** bisa dijangkau lewat Caddy. Jangan
  tambahkan `ports:` ke service `app`.
- Siapa pun yang punya akses root ke VPS bisa mengganti kode JavaScript yang dikirim ke browser
  (lihat halaman Privasi). Batasi akses SSH hanya untuk pengelola.

## 8. Checklist sebelum mengundang orang

**Pemeriksaan otomatis** (dari komputer mana pun yang punya bash & curl):

```bash
bash scripts/smoke.sh https://<DOMAIN>
```

Memeriksa HTTPS, pengalihan http→https, header keamanan, cookie aman, API tanpa login, kontak krisis
sudah diverifikasi, dan identitas pengelola sudah diisi.

**Uji manual di HP sungguhan** (minimal satu Android + Chrome dan satu iPhone + Safari):

- [ ] Daftar akun baru → centang persetujuan → buat passphrase → **simpan kunci pemulihan** (coba Salin & Unduh)
- [ ] Isi catatan pertama (mood, tidur, aktivitas, catatan) → Simpan → muncul "Tersimpan ✓"
- [ ] Beranda & Riwayat menampilkan catatan; ketuk tanggal di kalender → catatan terbaca
- [ ] Tekan 🔓 Kunci → buka lagi dengan passphrase
- [ ] "Lupa passphrase?" → pulihkan dengan kunci pemulihan → catatan lama tetap terbaca
- [ ] Pengaturan → Unduh JSON/CSV → file terbuka dan berisi catatan
- [ ] iPhone: Bagikan → Tambahkan ke Layar Utama → buka dari ikon → Pengaturan → aktifkan pengingat
      dengan jam 2 menit lagi → notifikasi muncul (pastikan hari itu belum ada catatan)
- [ ] Android: aktifkan pengingat dengan cara yang sama → notifikasi muncul
- [ ] Mode gelap HP: semua teks tetap terbaca
- [ ] Halaman /bantuan: nomor bisa diketuk dan langsung memanggil

**Lainnya:**

- [ ] Teks halaman `/privasi` dan `/syarat` sudah dibaca dan sesuai kenyataan
- [ ] Ada minimal satu file di `deploy/backups/`, dan sudah dicoba restore ke database sementara
- [ ] Salinan backup tersimpan di luar VPS
- [ ] Firewall hanya membuka 22, 80, 443
