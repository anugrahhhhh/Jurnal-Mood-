#!/usr/bin/env bash
# Pemeriksaan cepat setelah deploy. Tidak membuat akun dan tidak mengubah data.
#
#   bash scripts/smoke.sh https://jurnal.contoh.id
#
# Keluar dengan kode 1 jika ada pemeriksaan yang GAGAL.
# SMOKE_INSECURE=1 melewati cek sertifikat (HANYA untuk uji lokal dengan sertifikat buatan Caddy).
set -uo pipefail

BASE="${1:-}"
if [[ ! "$BASE" =~ ^https://[^/]+$ ]]; then
  echo "Pemakaian: bash scripts/smoke.sh https://domain-anda (tanpa garis miring di akhir)"
  exit 2
fi
HOST="${BASE#https://}"   # bisa berisi port, mis. localhost:8443
NAME="${HOST%%:*}"        # nama host tanpa port
K=()
[[ "${SMOKE_INSECURE:-}" == 1 ]] && K=(-k)
fail=0
ok()   { printf '  \033[32mOK\033[0m     %s\n' "$1"; }
bad()  { printf '  \033[31mGAGAL\033[0m  %s\n' "$1"; fail=1; }
warn() { printf '  \033[33mCEK\033[0m    %s\n' "$1"; }

headers() { curl "${K[@]}" -sS -o /dev/null -D - --max-time 15 "$1" | tr -d '\r'; }
body()    { curl "${K[@]}" -sS --max-time 15 "$1"; }
status()  { curl "${K[@]}" -sS -o /dev/null -w '%{http_code}' --max-time 15 "$1" || echo 000; }

echo "Memeriksa $BASE"

code=$(status "$BASE/healthz")
[[ "$code" == 200 ]] && ok "/healthz menjawab 200 (aplikasi & database hidup)" || bad "/healthz menjawab $code"

if [[ "$HOST" == *:* ]]; then
  warn "port khusus ($HOST): pengalihan http->https tidak diperiksa"
else
  loc=$(curl -sS -o /dev/null -w '%{redirect_url}' --max-time 15 "http://$NAME/" || true)
  [[ "$loc" == https://* ]] && ok "http:// dialihkan ke https://" || bad "http:// tidak dialihkan ke https:// (dapat: '${loc:-tidak ada}')"
fi

h=$(headers "$BASE/masuk")
grep -qi '^strict-transport-security:' <<<"$h" && ok "HSTS aktif" || bad "header Strict-Transport-Security tidak ada (APP_ENV=production?)"
grep -qi "^content-security-policy:.*script-src 'self'" <<<"$h" && ok "Content-Security-Policy ketat" || bad "Content-Security-Policy tidak sesuai"
grep -qi '^set-cookie: __Host-mj_csrf=.*Secure' <<<"$h" && ok "cookie __Host- dengan Secure" || bad "cookie bukan __Host-/Secure (APP_ENV=production?)"
grep -qi '^referrer-policy: same-origin' <<<"$h" && ok "Referrer-Policy same-origin" || bad "Referrer-Policy tidak sesuai"

b=$(body "$BASE/bantuan")
if grep -q 'belum diverifikasi' <<<"$b"; then bad "kontak krisis belum diverifikasi (config/crisis.yaml: verified: true)"; else ok "kontak krisis sudah ditandai terverifikasi"; fi

b=$(body "$BASE/privasi")
if grep -q 'belum diisi di konfigurasi' <<<"$b"; then bad "identitas pengelola belum diisi (OPERATOR_NAME, OPERATOR_CONTACT)"; else ok "identitas pengelola tampil di halaman privasi"; fi

h=$(headers "$BASE/sw.js")
grep -qi '^cache-control: no-cache' <<<"$h" && ok "/sw.js tersedia tanpa cache" || bad "/sw.js tidak tersedia"

code=$(status "$BASE/api/entri?dari=2026-01-01&sampai=2026-01-02")
[[ "$code" == 401 ]] && ok "API menolak permintaan tanpa login (401)" || bad "API tanpa login menjawab $code, mau 401"

hp="$HOST"; [[ "$hp" == *:* ]] || hp="$hp:443"
expiry=$(echo | openssl s_client -servername "$NAME" -connect "$hp" 2>/dev/null | openssl x509 -noout -enddate 2>/dev/null | cut -d= -f2)
[[ -n "$expiry" ]] && ok "sertifikat HTTPS berlaku sampai $expiry" || warn "tidak bisa membaca sertifikat (openssl tidak ada?)"

echo
if (( fail )); then echo "Ada pemeriksaan yang GAGAL. Perbaiki sebelum mengundang pengguna."; exit 1; fi
echo "Semua pemeriksaan otomatis lulus. Lanjutkan uji manual di HP (docs/DEPLOY.md bagian 8)."
