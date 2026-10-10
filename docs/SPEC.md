# Spesifikasi: Mood Tracker & Jurnal Harian

Status: draf hasil sesi brainstorming, 10 Oktober 2026.
Penanda: **[Keputusan]** = diputuskan pemilik proyek. **[Usulan]** = keputusan pembuat spesifikasi yang boleh diubah. **[Terbuka]** = belum dibahas.

## 1. Tujuan dan alasan

Website pencatat mood dan jurnal harian yang sederhana dan privat, untuk dipakai sungguhan oleh sekelompok kecil pengguna (teman/komunitas). Proyek ini dibuat karena ada kebutuhan akan pencatat mood yang sederhana tanpa mengorbankan privasi.

Prioritas, berurutan: (1) keamanan dan privasi data, (2) tampilan sederhana, (3) mudah dipakai.

### Bukan tujuan (MVP)
- Diagnosis, penilaian klinis, atau deteksi keadaan darurat.
- Pencarian isi catatan di server.
- Pemulihan catatan jika passphrase jurnal hilang.
- Kontak tepercaya pilihan pengguna (kandidat tahap berikutnya, opt-in).
- Aplikasi native. Ini web mobile-first.

### Anti-tujuan (gagal meskipun "berfungsi")
- Admin, penyedia hosting, atau pemegang backup bisa membaca catatan atau aktivitas.
- Website mengklaim bisa mendeteksi krisis.
- Nada alarmis, label, atau diagnosis.
- Tekanan streak, terutama di hari-hari buruk.
- Peringatan otomatis ke admin atau siapa pun tanpa persetujuan pengguna.

## 2. Stack **[Keputusan]**
- Go dengan router **Chi**
- **PostgreSQL**
- **HTMX + Go `html/template`**
- Kriptografi browser: **Web Crypto API** bawaan, tanpa library kripto pihak ketiga.

## 3. Model privasi

### 3.1 Apa yang dilihat siapa

| Data | Disimpan sebagai | Bisa dibaca admin/hosting/backup? |
|---|---|---|
| Teks catatan | terenkripsi di browser | Tidak |
| Aktivitas (termasuk daftar chip buatan pengguna) | terenkripsi di browser | Tidak |
| Draf otomatis | terenkripsi, di penyimpanan browser | Tidak (dan tidak dikirim ke server) |
| Skor mood (1–5) | bilangan bulat biasa | **Ya** |
| Jam tidur | bilangan bulat biasa (menit) | **Ya** |
| Metadata: tanggal entri, waktu simpan, ukuran data terenkripsi, email akun | biasa | **Ya** |

**[Keputusan]** Halaman privasi wajib menyebut dengan jujur bahwa skor mood, jam tidur, dan metadata bisa dibaca admin.

### 3.2 Skema enkripsi **[Keputusan]**, detail angka **[Usulan]**
- **Passphrase jurnal terpisah dari password login.** Password login bisa di-reset seperti biasa. Passphrase jurnal tidak bisa dipulihkan.
- **Kunci data (DEK):** kunci acak AES-256-GCM yang dibuat di browser satu kali per pengguna. Semua entri dienkripsi dengan DEK ini.
- **Kunci pembungkus (KEK):** diturunkan dari passphrase dengan **PBKDF2-SHA-256, iterasi tinggi** (usulan: 600.000), dengan salt acak 16 byte per pengguna. DEK disimpan di server dalam bentuk terbungkus oleh KEK (AES-GCM, IV acak).
- **Mengganti passphrase** cukup membungkus ulang DEK dengan KEK baru. Entri tidak dienkripsi ulang.
- **IV acak 12 byte per entri**, dibuat baru setiap kali entri disimpan, termasuk saat diedit.
- **Nilai uji (verifier):** sebuah konstanta yang dienkripsi dengan DEK dan disimpan di server, agar browser bisa memastikan passphrase benar sebelum dipakai. (Catatan: kegagalan membuka bungkus AES-GCM sendiri sudah menandakan passphrase salah. Verifier tetap disimpan sesuai keputusan, sebagai pemeriksaan eksplisit.)
- **[Usulan]** Setiap blob terenkripsi diberi nomor versi format serta parameter KDF (iterasi, salt) yang disimpan di samping blob, agar parameternya bisa dinaikkan di masa depan.
- **[Usulan]** Isi terenkripsi per entri berupa satu objek JSON `{activities: [...], note: "..."}`, sehingga hanya ada satu blob per entri.
- **[Usulan]** Untuk associated data (AAD) AES-GCM dipakai `user_id + entry_date`, supaya blob tidak bisa dipindah diam-diam ke tanggal atau pengguna lain.

### 3.3 Siklus hidup kunci di browser **[Keputusan]**
- Jurnal dibuka sekali per sesi dengan passphrase. Passphrase dan kunci hanya ada di memori: tidak pernah dikirim ke server, tidak disimpan di localStorage.
- **[Usulan]** DEK di-unwrap sebagai `CryptoKey` yang tidak bisa diekspor (`extractable: false`).
- Jurnal terkunci otomatis setelah lama tidak aktif (**[Usulan]** 15 menit), dan kunci dibuang dari memori.
- Draf otomatis dienkripsi dengan DEK sebelum disimpan di penyimpanan browser, dan dihapus setelah entri tersimpan.

### 3.4 Batasan yang harus dijelaskan jujur **[Keputusan]**
Kode enkripsi (JavaScript) dikirim oleh server sendiri. Admin atau penyedia hosting yang berniat jahat secara teori bisa mengubah kode itu untuk mencuri passphrase saat diketik. Enkripsi ini melindungi dari kebocoran database, backup, dan rasa penasaran, tetapi tidak dari server yang sengaja diubah. Halaman privasi wajib menjelaskan ini dengan bahasa awam.

### 3.5 Pengamanan pendukung **[Usulan]**
- Content Security Policy yang ketat: semua JS dan CSS di-host sendiri (tanpa CDN), tanpa inline script, tanpa analitik atau skrip pihak ketiga.
- Password login di-hash dengan **Argon2id** di server.
- Sesi berupa cookie `HttpOnly`, `Secure`, `SameSite=Lax`, dengan token acak yang disimpan di server dalam bentuk hash. Ada perlindungan CSRF pada semua POST dan pembatasan percobaan login.
- HTTPS wajib di produksi.
- Pengguna bisa menghapus akun beserta seluruh datanya.

## 4. Keselamatan **[Keputusan]**

Prinsip dasarnya: website tidak bisa membaca catatan terenkripsi, jadi tidak boleh mengklaim bisa mendeteksi keadaan darurat.

### Yang harus ada
- **Tautan bantuan krisis yang selalu terlihat** di setiap halaman, termasuk halaman login dan halaman buka-kunci. Tautan ini membuka daftar nomor darurat dan layanan konseling setempat yang **diambil dari file konfigurasi**, bukan ditulis langsung di kode.
  - **[Usulan]** Halaman bantuan tetap bisa dibuka tanpa login.
  - Nomor-nomornya wajib diverifikasi pemilik proyek sebelum peluncuran.
- **Disclaimer**: ini bukan layanan darurat dan bukan pengganti tenaga profesional. Disclaimer tampil saat pendaftaran, di halaman bantuan, dan di footer.
- **Kartu pesan lembut** yang bisa ditutup, muncul setelah skor mood rendah selama **3 hari berturut-turut**.
  - Dihitung **di browser**, sehingga server tidak tahu siapa yang memicunya.
  - **[Usulan]** "Rendah" berarti skor ≤ 2. "Berturut-turut" berarti tiga tanggal kalender berurutan yang semuanya punya entri dengan skor ≤ 2.
  - **[Usulan]** Status "sudah ditutup" disimpan di browser saja, per rangkaian hari. Kartu muncul lagi hanya untuk rangkaian rendah yang baru.
  - Nadanya hangat dan tidak menghakimi, dan berisi tautan ke halaman bantuan.

### Yang tidak boleh
- Peringatan otomatis ke admin atau orang lain tanpa persetujuan pengguna.
- Label atau diagnosis.
- Nada alarmis.
- Penguncian akun.
- Tekanan streak untuk hari buruk.
- Menyimpan flag "berisiko" per pengguna di database atau log.

### Tahap berikutnya (bukan MVP)
- Kontak tepercaya yang dipilih pengguna sendiri, bersifat opt-in.

## 5. Alur harian **[Keputusan]**

Mobile-first, diisi sekitar jam tidur, target **di bawah 1 menit**.

1. **Buka kunci jurnal** sekali per sesi (lihat 3.3).
2. **Mood**: satu ketukan, skala **1–5** dengan ikon wajah dan label teks (Sangat buruk, Buruk, Biasa, Baik, Sangat baik). Disimpan sebagai bilangan bulat.
3. **Tidur**: tombol +/- per **30 menit**, nilai awalnya diambil dari entri sebelumnya.
4. **Aktivitas**: chip yang bisa dipilih, dan pengguna bisa menambah chip sendiri. Terenkripsi.
5. **Catatan**: opsional, terenkripsi.
6. **Simpan**, lalu tampil ringkasan 7 hari tanpa streak.

Aturan entri:
- Satu entri per hari per pengguna. Bisa diedit, dan bisa mengisi untuk hari kemarin.
- **[Usulan]** Hanya hari ini dan kemarin yang bisa diisi atau diedit dari layar harian. Riwayat lama hanya untuk dilihat di MVP.
- **[Usulan]** "Hari ini" mengikuti zona waktu browser pengguna, karena entri biasa diisi menjelang tengah malam.
- Draf otomatis terenkripsi (lihat 3.3).

**Pengingat harian opsional** dengan isi generik (misalnya "Waktunya mencatat hari ini"), tanpa menyebut mood atau isi jurnal. **[Usulan]** Diimplementasikan dengan Web Push. Jam pengingat menjadi metadata yang terlihat server dan disebut di halaman privasi. Dikerjakan di tahap terakhir MVP.

## 6. Tampilan **[Keputusan]**: variasi B, "warna mengikuti mood"

Referensi: `.superpowers/brainstorm/*/content/mood-entry-v2.html`, kartu B.
- Latar layar berganti warna lembut sesuai mood yang dipilih: hangat untuk mood baik, biru atau ungu lembut untuk mood rendah. Warna untuk mood rendah tidak boleh terasa suram atau alarmis.
- Wajah besar dan label di bagian atas. Mood dipilih dengan geseran 1–5 yang berhenti di setiap nilai, dan setiap wajah juga bisa diketuk langsung.
- Tidur ditampilkan sebagai cincin dengan angka besar dan tombol +/-.
- Aktivitas berupa chip di atas kartu semi-transparan ("kaca").
- Baris 7 hari terakhir berisi wajah, tanpa angka streak.
- Di bagian atas ada status kunci jurnal dan tautan "Bantuan krisis".
- **[Usulan]** Aksesibilitas:
  - Animasi latar dimatikan saat `prefers-reduced-motion` aktif.
  - Kontras teks memenuhi WCAG AA di semua warna mood.
  - Geseran tetap bisa dioperasikan dengan keyboard dan pembaca layar sebagai radio group 1–5.
  - Efek kaca diganti warna solid jika browser tidak mendukung `backdrop-filter`.

## 7. Fitur MVP
1. Register/login. **[Keputusan]** Pendaftaran terbuka (11 Okt 2026), dibatasi 5 pendaftaran per IP per jam. Mode undangan tetap tersedia lewat `REGISTRATION=invite`.
2. Buat, buka, kunci, dan ganti passphrase jurnal.
3. Input entri harian (bagian 5).
4. Riwayat dan grafik:
   - Grafik mood dan tidur per hari. **[Usulan]** Rentang 7, 30, dan 90 hari.
   - Korelasi aktivitas dengan mood **dihitung di browser** **[Keputusan]**, karena aktivitas terenkripsi. **[Usulan]** Bentuknya rata-rata mood pada hari dengan aktivitas X dibanding hari tanpa X, dengan bahasa netral dan tanpa klaim sebab-akibat.
   - **[Usulan]** Library grafik di-host sendiri (tidak lewat CDN).
5. Halaman bantuan krisis, disclaimer, dan halaman privasi (bagian 3 dan 4).
6. Hapus akun beserta seluruh data. **[Usulan]**
7. Pengingat harian opsional (bagian 5).

## 8. Diserahkan ke pembuat
Struktur kode, skema tabel, nama rute, pilihan tool migrasi, library pendukung, desain detail halaman selain layar harian, dan isi teks UI. Semua itu harus tetap mengikuti batasan di atas.
