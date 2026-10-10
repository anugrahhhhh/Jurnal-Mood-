# CATATAN: di laptop ini Smart App Control memblokir program Go buatan sendiri.
# Pakai WSL sebagai gantinya:  wsl -d Ubuntu -- bash scripts/dev.sh run|test|invite|stop
#
# Perintah pengembangan. Contoh:
#   .\run.ps1          menjalankan server
#   .\run.ps1 test     menjalankan semua tes
#   .\run.ps1 build    membuat bin\moodjournal.exe (untuk deploy; di laptop ini
#                      Smart App Control memblokir .exe yang belum ditandatangani,
#                      jadi saat pengembangan pakai "run")
param([string]$Task = "run")

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot

if (-not (Test-Path ".env")) {
    Copy-Item ".env.example" ".env"
    Write-Host "File .env dibuat dari .env.example"
}

switch ($Task) {
    "run"   { go run ./cmd/server }
    "test"  {
        # Tes database memakai moodjournal_test (dikosongkan setiap tes).
        # -p 1: paket dijalankan bergantian karena berbagi database yang sama.
        if (-not $env:TEST_DATABASE_URL) {
            $env:TEST_DATABASE_URL = "postgres://postgres@localhost:5432/moodjournal_test?sslmode=disable"
        }
        go vet ./...; go test -p 1 ./...
        # Tes kripto browser (Web Crypto) di Node.js 20+.
        node --test "tests/js/*.test.mjs"
    }
    "build" { go build -o bin/moodjournal.exe ./cmd/server }
    default { Write-Host "Tugas tidak dikenal: $Task (pilihan: run, test, build)"; exit 1 }
}
