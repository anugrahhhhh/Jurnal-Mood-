package web

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"moodjournal/internal/auth"
)

// Renderer menyimpan satu set template per halaman: layout dasar + halaman itu.
// Setiap halaman mendefinisikan blok "title" dan "content".
type Renderer struct {
	pages map[string]*template.Template
}

var funcs = template.FuncMap{
	"tel": telURL,
}

func NewRenderer(assets fs.FS) (*Renderer, error) {
	files, err := fs.Glob(assets, "templates/pages/*.html")
	if err != nil {
		return nil, err
	}
	r := &Renderer{pages: make(map[string]*template.Template, len(files))}
	for _, f := range files {
		name := strings.TrimSuffix(path.Base(f), ".html")
		t, err := template.New(name).Funcs(funcs).ParseFS(assets, "templates/layouts/base.html", f)
		if err != nil {
			return nil, fmt.Errorf("template %s: %w", name, err)
		}
		r.pages[name] = t
	}
	return r, nil
}

// PageData adalah data yang tersedia di setiap halaman.
type PageData struct {
	Title string
	User  *auth.User // nil untuk tamu
	CSRF  string     // token untuk field tersembunyi "csrf" di setiap form POST
	Nav   string     // menu yang aktif di sidebar/tab bar, lihat navFor
	Data  any
}

// page menyiapkan PageData dengan pengguna dan token CSRF dari request.
func page(r *http.Request, title string, data any) PageData {
	return PageData{Title: title, User: currentUser(r), CSRF: csrfToken(r), Nav: navFor(r.URL.Path), Data: data}
}

// navFor memetakan path ke menu navigasi yang ditandai aktif.
func navFor(path string) string {
	switch {
	case path == "/beranda":
		return "beranda"
	case path == "/hari-ini":
		return "hari-ini"
	case path == "/riwayat":
		return "riwayat"
	case path == "/pengaturan", strings.HasPrefix(path, "/jurnal/ganti"), path == "/akun/password":
		return "pengaturan"
	case path == "/privasi":
		return "privasi"
	}
	return ""
}

func (r *Renderer) Page(w http.ResponseWriter, status int, name string, data PageData) {
	t, ok := r.pages[name]
	if !ok {
		slog.Error("template tidak ditemukan", "page", name)
		http.Error(w, "Terjadi kesalahan", http.StatusInternalServerError)
		return
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "base", data); err != nil {
		slog.Error("render template", "page", name, "err", err)
		http.Error(w, "Terjadi kesalahan", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	buf.WriteTo(w)
}

// telURL membuat tautan tel: hanya dari digit dan '+', sehingga aman
// ditandai sebagai URL tepercaya untuk html/template.
func telURL(number string) template.URL {
	var b strings.Builder
	for _, c := range number {
		if (c >= '0' && c <= '9') || (c == '+' && b.Len() == 0) {
			b.WriteRune(c)
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return template.URL("tel:" + b.String())
}
