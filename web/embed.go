// Package web menyediakan template dan file statis yang di-embed ke binary.
package web

import "embed"

//go:embed templates static
var FS embed.FS
