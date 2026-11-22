package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSPAFallback(t *testing.T) {
	h := handler(fstest.MapFS{
		"index.html":               {Data: []byte("<html>spa</html>")},
		"_app/immutable/app.1a.js": {Data: []byte("js")},
	})
	cases := []struct {
		path, body, cache string
		code              int
	}{
		{"/", "spa", "no-cache", 200},
		{"/dns/records", "spa", "no-cache", 200},
		{"/_app/immutable/app.1a.js", "js", "immutable", 200},
		{"/api/unknown", "404", "", 404},
		{"/../../etc/passwd", "spa", "no-cache", 200},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, c.path, nil))
		if rec.Code != c.code || !strings.Contains(rec.Body.String(), c.body) || !strings.Contains(rec.Header().Get("Cache-Control"), c.cache) {
			t.Errorf("%s: code=%d cache=%q body=%q", c.path, rec.Code, rec.Header().Get("Cache-Control"), rec.Body.String())
		}
	}
}

func TestUnbuiltSPA(t *testing.T) {
	rec := httptest.NewRecorder()
	handler(fstest.MapFS{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d", rec.Code)
	}
}

// A public listener shows its page at the root and serves nothing else but assets: no client-side routes (07 §7, §8).
func TestPublicHandler(t *testing.T) {
	root := fstest.MapFS{
		"index.html":          {Data: []byte("<html><head><title>x</title></head></html>")},
		"_app/immutable/a.js": {Data: []byte("js")},
		"secret.txt":          {Data: []byte("no")},
	}
	h := publicHandler(root, "status", func(*http.Request) Head { return Head{} })
	get := func(p string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		return rec
	}
	if r := get("/"); r.Code != 200 || !strings.Contains(r.Body.String(), `<meta name="hashi-public" content="status">`) {
		t.Fatalf("root %d %s", r.Code, r.Body)
	}
	if r := get("/_app/immutable/a.js"); r.Code != 200 {
		t.Fatalf("asset %d", r.Code)
	}
	for _, p := range []string{"/login", "/settings", "/secret.txt", "/api/auth/session"} {
		if r := get(p); r.Code != 404 {
			t.Errorf("%s: %d", p, r.Code)
		}
	}
}

// Bug 160: a public page names its icon in the HTML itself, so the app dashboard finds it for the page's own service.
// Without an uploaded icon that is Hashi's mark, which the listener serves; with one, the owner's icon replaces it.
func TestPublicPageNamesItsIcon(t *testing.T) {
	root := fstest.MapFS{
		"index.html":           {Data: []byte(`<html><head><link rel="apple-touch-icon" href="/apple-touch-icon.png" /></head></html>`)},
		"apple-touch-icon.png": {Data: []byte("png")},
	}
	get := func(h http.Handler, p string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		return rec
	}
	plain := publicHandler(root, "apps", func(*http.Request) Head { return Head{} })
	if r := get(plain, "/apple-touch-icon.png"); r.Code != 200 || r.Body.String() != "png" {
		t.Fatalf("Hashi's mark: %d %q", r.Code, r.Body)
	}
	if r := get(plain, "/"); !strings.Contains(r.Body.String(), `href="/apple-touch-icon.png"`) {
		t.Fatalf("without an icon: %s", r.Body)
	}
	// Named as the tab icon too: a page without one makes the browser ask for /favicon.ico, a 404 here and an error
	// in every visitor's console (Lighthouse, 2026-11-22). The page's script then draws the mark in the theme's accent.
	if r := get(plain, "/"); !strings.Contains(r.Body.String(), `<link rel="icon" href="/apple-touch-icon.png">`) {
		t.Errorf("no tab icon without an uploaded one: %s", r.Body)
	}
	own := publicHandler(root, "apps", func(*http.Request) Head { return Head{Icon: "/assets/icon?v=ab12"} })
	body := get(own, "/").Body.String()
	for _, want := range []string{`<link rel="icon" href="/assets/icon?v=ab12">`, `<link rel="apple-touch-icon" href="/assets/icon?v=ab12">`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in %s", want, body)
		}
	}
	if strings.Contains(body, "/apple-touch-icon.png") {
		t.Errorf("Hashi's mark still named beside the owner's icon: %s", body)
	}
}

// A shared link's preview and a bookmark read the page without running it, so the HTML names the page as its script
// does ("Status · example.com"), escaped. Hashi's tags follow <meta charset>, which must open the head.
func TestPublicPageNamesItsTitle(t *testing.T) {
	root := fstest.MapFS{"index.html": {Data: []byte(`<html><head><meta charset="utf-8" /><title>x</title></head></html>`)}}
	h := publicHandler(root, "status", func(*http.Request) Head { return Head{Title: `Status · <b>"A&B"</b>`} })
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rec.Body.String()
	want := `<title>Status · &lt;b&gt;&#34;A&amp;B&#34;&lt;/b&gt;</title><meta property="og:title" content="Status · &lt;b&gt;&#34;A&amp;B&#34;&lt;/b&gt;">`
	if !strings.Contains(body, want) {
		t.Errorf("no title in %s", body)
	}
	if !strings.HasPrefix(body, `<html><head><meta charset="utf-8" />`) {
		t.Errorf("charset is not first: %s", body)
	}
	if strings.Contains(body, "<title>x</title>") {
		t.Errorf("two titles: %s", body)
	}
}
