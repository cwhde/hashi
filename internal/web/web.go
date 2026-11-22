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

var (
	shellTitle = regexp.MustCompile(`<title>[^<]*</title>`)
	charset    = regexp.MustCompile(`<meta charset="[^"]*"\s*/?>`)
)

// Head is what a public page's HTML names before its script runs: the title as the page sets it ("Status ·
// example.com") and the owner's uploaded icon ("/assets/icon?v=…"), each "" when there is none.
type Head struct {
	Title string
	Icon  string
	// Missing: a service page for no entry shown on the status page. It answers 404 (07 §7.2) and is still drawn, so
	// the visitor reads why.
	Missing bool
	// The configured theme (10-ui §4, bug 178), in the page from the first paint: a built-in's id and mode, or
	// "custom" with its mode, canvas and text. Empty: the shell's own default and the browser's stored theme.
	Theme, Mode, Canvas, Text string
}

var htmlTag = regexp.MustCompile(`<html[^>]*>`)

// themed is the opening <html> tag with the page's theme; data-public-theme tells the shell's script to keep it.
func themed(hd Head) string {
	esc := htmlpkg.EscapeString
	tag := `<html lang="en" data-theme="` + esc(hd.Theme) + `" data-mode="` + esc(hd.Mode) + `" data-public-theme`
	if hd.Theme == "custom" {
		tag += ` style="--h-canvas: ` + esc(hd.Canvas) + `; --h-text: ` + esc(hd.Text) + `; color-scheme: ` + esc(hd.Mode) + `"`
	}
	return tag + ">"
}

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
// head names the page and its icon for the request.
func Public(page string, head func(*http.Request) Head) http.Handler {
	root, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return publicHandler(root, page, head)
}

// servicePage is a status page entry's own address (07 §7.2); the page itself says when the slug is unknown.
var servicePage = regexp.MustCompile(`^/s/[a-z0-9-]{1,80}$`)

func publicHandler(root fs.FS, page string, head func(*http.Request) Head) http.Handler {
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
		hd := head(r)
		// A shared link's preview and a bookmark read the page without running it, so the HTML names it too.
		if t := htmlpkg.EscapeString(hd.Title); t != "" {
			meta += `<title>` + t + `</title><meta property="og:title" content="` + t + `">`
			html = shellTitle.ReplaceAllString(html, "")
		}
		// The icon is named in the HTML, not only by the page's script, so whatever reads the page without running it
		// (the app dashboard's icon finder, bug 160) finds the owner's icon, or Hashi's mark without one.
		// Without one the mark is the tab icon too, or the browser asks for /favicon.ico first: a 404 in every visitor's
		// console. The page's script then draws the mark in the theme's accent (bug 159).
		if u := hd.Icon; u != "" {
			u = htmlpkg.EscapeString(u)
			meta += `<link rel="icon" href="` + u + `"><link rel="apple-touch-icon" href="` + u + `">`
			html = markLink.ReplaceAllString(html, "")
		} else {
			meta += `<link rel="icon" href="/` + markFile + `">`
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		// After <meta charset>, which has to open the head; at the head's start in a shell without one.
		if at := charset.FindStringIndex(html); at != nil {
			html = html[:at[1]] + meta + html[at[1]:]
		} else {
			html = strings.Replace(html, "<head>", "<head>"+meta, 1)
		}
		if at := htmlTag.FindStringIndex(html); hd.Theme != "" && at != nil {
			html = html[:at[0]] + themed(hd) + html[at[1]:]
		}
		if hd.Missing {
			w.WriteHeader(http.StatusNotFound)
		}
		_, _ = w.Write([]byte(html))
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
