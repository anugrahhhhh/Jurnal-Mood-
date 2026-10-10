// Package migrations berisi file SQL goose yang di-embed ke binary.
// Nama file: NNNNN_deskripsi.sql, dijalankan berurutan saat server start.
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
