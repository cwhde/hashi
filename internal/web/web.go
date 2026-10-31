// Package web serves the SvelteKit SPA embedded into the binary at build time (`make build-web`).
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler serves built assets and falls back to index.html for client-side routes.
// Paths under /api/ never fall back: an unknown API path must be a 404, not the SPA.
func Handler() http.Handler {
	root, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // the embed directive guarantees "dist" exists
	}
	return handler(root)
}

// Public serves the same SPA for a public listener. index.html names the page, so the SPA shows it at the root path
// (07 §7, §8); every other path is a static asset or a 404: public listeners have no client-side routes.
func Public(page string) http.Handler {
	root, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return publicHandler(root, page)
}

func publicHandler(root fs.FS, page string) http.Handler {
	files := http.FileServerFS(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
			if strings.HasPrefix(name, "_app/") {
				if st, err := fs.Stat(root, name); err == nil && !st.IsDir() {
					if strings.HasPrefix(name, "_app/immutable/") {
						w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
					}
					files.ServeHTTP(w, r)
					return
				}
			}
			http.NotFound(w, r)
			return
		}
		index, err := fs.ReadFile(root, "index.html")
		if err != nil {
			http.Error(w, "web UI not built into this binary (run make build-web)", http.StatusServiceUnavailable)
			return
		}
		meta := `<meta name="hashi-public" content="` + page + `"><meta name="robots" content="noindex">`
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(strings.Replace(string(index), "<head>", "<head>"+meta, 1)))
	})
}

func handler(root fs.FS) http.Handler {
	files := http.FileServerFS(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name != "" {
			if st, err := fs.Stat(root, name); err == nil && !st.IsDir() {
				if strings.HasPrefix(name, "_app/immutable/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		index, err := fs.ReadFile(root, "index.html")
		if err != nil {
			http.Error(w, "web UI not built into this binary (run make build-web)", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(index)
	})
}
