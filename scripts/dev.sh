#!/usr/bin/env bash
# Perintah pengembangan di WSL (Ubuntu). Di laptop ini Smart App Control
# memblokir program Go buatan sendiri di Windows, jadi server dan tes
# dijalankan di Linux. Dari Windows:
#
#   wsl -d Ubuntu -- bash scripts/dev.sh run       menjalankan server (http://127.0.0.1:8080)
#   wsl -d Ubuntu -- bash scripts/dev.sh test      gofmt + vet + semua tes Go + tes JS
#   wsl -d Ubuntu -- bash scripts/dev.sh invite 7  membuat kode undangan (berlaku 7 hari)
#   wsl -d Ubuntu -- bash scripts/dev.sh vapid     membuat kunci VAPID untuk pengingat
#   wsl -d Ubuntu -- bash scripts/dev.sh admin ... perintah pengelola (stats, reset-password, ...)
#   wsl -d Ubuntu -- bash scripts/dev.sh stop      menghentikan server
#   npm run e2e                                    tes end-to-end di Chrome (HP, iPad, laptop)
#
# Butuh networkingMode=mirrored di %USERPROFILE%\.wslconfig agar localhost
# WSL sama dengan localhost Windows (PostgreSQL Laragon & browser).
set -euo pipefail
cd "$(dirname "$0")/.."

export GOFLAGS=-buildvcs=false
export TEST_DATABASE_URL="${TEST_DATABASE_URL:-postgres://postgres@127.0.0.1:5432/moodjournal_test?sslmode=disable}"
[ -f .env ] || cp .env.example .env

case "${1:-run}" in
  run)
    go run ./cmd/server
    ;;
  test)
    unformatted=$(gofmt -l cmd internal web)
    if [ -n "$unformatted" ]; then
      echo "Belum diformat (jalankan gofmt -w):"; echo "$unformatted"; exit 1
    fi
    go vet ./...
    go test -p 1 -count=1 ./...
    if command -v node >/dev/null; then
      node --test tests/js/*.test.mjs
    else
      echo "(node tidak ada di WSL; jalankan tes JS dari Windows: node --test \"tests/js/*.test.mjs\")"
    fi
    ;;
  invite)
    go run ./cmd/server invite -hari "${2:-7}"
    ;;
  vapid)
    go run ./cmd/server vapid
    ;;
  admin)
    shift
    go run ./cmd/server admin "$@"
    ;;
  e2e-server)
    # Server khusus tes end-to-end: port 8090, database moodjournal_e2e,
    # pendaftaran terbuka tanpa batas praktis. Tidak menyentuh data pengembangan.
    go build -o /tmp/moodjournal-e2e ./cmd/server
    ADDR=127.0.0.1:8090     DATABASE_URL="postgres://postgres@127.0.0.1:5432/moodjournal_e2e?sslmode=disable"     REGISTRATION=open SIGNUP_LIMIT_PER_HOUR=10000     VAPID_PUBLIC_KEY= VAPID_PRIVATE_KEY= VAPID_SUBJECT=     exec /tmp/moodjournal-e2e
    ;;
  e2e-stop)
    pkill -f /tmp/moodjournal-e2e || true
    ;;
  stop)
    pkill -f "go run ./cmd/server" || true
    pkill -f "/exe/server" || true
    ;;
  *)
    echo "Tugas tidak dikenal: $1 (pilihan: run, test, invite, vapid, admin, e2e-server, e2e-stop, stop)"; exit 1
    ;;
esac
