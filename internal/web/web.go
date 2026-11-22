// Package web serves the SvelteKit SPA embedded into the binary at build time (`make build-web`).
package web

import (
	"embed"
	htmlpkg "html"
	"io/fs"
	"net/http"
	"path"
	"regexp"
	"strings"
)

// markFile is Hashi's own icon (web/static), which every page links as its apple-touch-icon.
const markFile = "apple-touch-icon.png"

var markLink = regexp.MustCompile(`<link rel="apple-touch-icon" href="/` + regexp.QuoteMeta(markFile) + `"\s*/?>`)

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
// icon names the owner's uploaded icon ("/assets/icon?v=…"), or "" when there is none.
func Public(page string, icon func(*http.Request) string) http.Handler {
	root, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return publicHandler(root, page, icon)
}

// servicePage is a status page entry's own address (07 §7.2); the page itself says when the slug is unknown.
var servicePage = regexp.MustCompile(`^/s/[a-z0-9-]{1,80}$`)

func publicHandler(root fs.FS, page string, icon func(*http.Request) string) http.Handler {
	files := http.FileServerFS(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" && (page != "status" || !servicePage.MatchString(r.URL.Path)) {
			name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
			if strings.HasPrefix(name, "_app/") || name == markFile {
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
		html := string(index)
		// The icon is named in the HTML, not only by the page's script, so whatever reads the page without running it
		// (the app dashboard's icon finder, bug 160) finds the owner's icon, or Hashi's mark without one.
		// Without one the mark is the tab icon too, or the browser asks for /favicon.ico first: a 404 in every visitor's
		// console. The page's script then draws the mark in the theme's accent (bug 159).
		if u := icon(r); u != "" {
			u = htmlpkg.EscapeString(u)
			meta += `<link rel="icon" href="` + u + `"><link rel="apple-touch-icon" href="` + u + `">`
			html = markLink.ReplaceAllString(html, "")
		} else {
			meta += `<link rel="icon" href="/` + markFile + `">`
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(strings.Replace(html, "<head>", "<head>"+meta, 1)))
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
