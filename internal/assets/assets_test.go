package assets

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
)

var png = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89")

func contract(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "assets", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseListingFromTheCapturedSource(t *testing.T) {
	l := ParseListing(contract(t, "listing-logo-2k.html"))
	if len(l.Dirs) != 0 || len(l.Files) != 16 || !slices.Contains(l.Files, "logo_w_bd.webp") || !slices.Contains(l.Files, "logo_b_xb.png") {
		t.Fatalf("logo/2k: %+v", l)
	}
	l = ParseListing(contract(t, "listing-icon-default.html"))
	if !slices.Contains(l.Files, "favicon.ico") || !slices.Contains(l.Files, "icon-512.webp") || slices.Contains(l.Files, "/icon/") {
		t.Fatalf("icon/default: %+v", l)
	}
	l = ParseListing([]byte(`<a href="?C=N;O=D">Name</a><a href="/">Parent</a><a href="../">Up</a><a href="2k/">2k/</a>` +
		`<a href="https://x.example/a.png">x</a><a href="notes.txt">n</a><a href="bg%20dark.webp">b</a><a href="sub/a.png">s</a>`))
	if !slices.Equal(l.Dirs, []string{"2k"}) || !slices.Equal(l.Files, []string{"bg dark.webp"}) {
		t.Fatalf("filtering: %+v", l)
	}
}

func TestSniffTakesOnlyImages(t *testing.T) {
	for name, c := range map[string]struct {
		data []byte
		want string
	}{
		"png":         {png, "image/png"},
		"webp":        {[]byte("RIFF\x10\x00\x00\x00WEBPVP8 "), "image/webp"},
		"ico":         {[]byte("\x00\x00\x01\x00\x01\x00\x10\x10"), "image/x-icon"},
		"svg":         {[]byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), "image/svg+xml"},
		"svg prolog":  {[]byte("\xef\xbb\xbf<?xml version=\"1.0\"?>\n<!-- made by hand -->\n<svg viewBox=\"0 0 1 1\"></svg>"), "image/svg+xml"},
		"html":        {[]byte(`<html><svg></svg></html>`), ""},
		"script text": {[]byte(`alert(1)`), ""},
	} {
		got, err := Sniff(c.data)
		if got != c.want || (c.want == "") != (err != nil) {
			t.Errorf("%s: %q %v", name, got, err)
		}
	}
}

func TestStorePutLimitsAndRemove(t *testing.T) {
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ct, etag, err := s.Put(Icon, png)
	if err != nil || ct != "image/png" || len(etag) != 16 {
		t.Fatalf("put: %q %q %v", ct, etag, err)
	}
	f, err := s.Open(Icon)
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	big := append(slices.Clone(png), make([]byte, 1<<20)...)
	if _, _, err := s.Put(LogoDark, big); !errors.Is(err, ErrInvalid) {
		t.Fatalf("1 MB limit: %v", err)
	}
	if _, _, err := s.Put(Background, big); err != nil {
		t.Fatalf("background takes 5 MB: %v", err)
	}
	if _, _, err := s.Put("../x", png); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown slot: %v", err)
	}
	if err := s.Remove(Icon); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(Icon); err != nil {
		t.Fatal("removing an empty slot:", err)
	}
	if _, err := s.Open(Icon); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("open empty: %v", err)
	}
}

func TestSourcePathsStayInsideTheSource(t *testing.T) {
	for _, bad := range []string{"../etc/passwd", "logo/../../x.png", `logo\x.png`, "logo/", "logo/x.txt", ""} {
		if _, err := CleanFile(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("CleanFile(%q) = %v", bad, err)
		}
	}
	if f, err := CleanFile("/logo/2k/logo_w_bd.webp"); err != nil || f != "logo/2k/logo_w_bd.webp" {
		t.Errorf("CleanFile: %q %v", f, err)
	}
	if d, err := CleanDir("logo//2k"); err != nil || d != "logo/2k/" {
		t.Errorf("CleanDir: %q %v", d, err)
	}
	if _, err := (Source{Client: http.DefaultClient}).List(context.Background(), "ftp://x", ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("non-http source: %v", err)
	}
}

func TestSourceListsAndFetches(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/assets/logo/2k/":
			_, _ = w.Write(contract(t, "listing-logo-2k.html"))
		case "/assets/logo/2k/logo_w_bd.png":
			_, _ = w.Write(png)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	src := Source{Client: srv.Client()}
	base := srv.URL + "/assets/"
	l, err := src.List(context.Background(), base, "logo/2k")
	if err != nil || len(l.Files) != 16 {
		t.Fatalf("list: %+v %v", l, err)
	}
	b, err := src.Fetch(context.Background(), base, "logo/2k/logo_w_bd.png", LogoDark)
	if err != nil || string(b) != string(png) {
		t.Fatalf("fetch: %v", err)
	}
	if _, err := src.Fetch(context.Background(), base, "logo/2k/missing.png", LogoDark); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("missing: %v", err)
	}
	if hits.Load() != 3 {
		t.Fatalf("requests: %d", hits.Load())
	}
}
