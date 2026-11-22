package assets

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"io"
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

// A logo is stored at the size pages draw it: a 3840 px WebP (a real one on prod was 227 KiB) comes out a PNG inside
// 768×120, whether it was uploaded or copied from the source (both go through Put). Icons and backgrounds stay as given.
func TestLogoIsStoredAtItsShownSize(t *testing.T) {
	big, err := os.ReadFile("testdata/wide-logo.webp")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ct, etag, err := s.Put(LogoDark, big)
	if err != nil {
		t.Fatal(err)
	}
	f, err := s.Open(LogoDark)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	stored, _ := io.ReadAll(f)
	cfg, format, err := image.DecodeConfig(bytes.NewReader(stored))
	if err != nil {
		t.Fatal(err)
	}
	if ct != "image/png" || format != "png" || cfg.Width > 768 || cfg.Height > 120 || len(stored) >= len(big)/2 {
		t.Errorf("stored %s (%s) %d×%d, %d bytes from %d", ct, format, cfg.Width, cfg.Height, len(stored), len(big))
	}
	if sum := sha256.Sum256(stored); etag != hex.EncodeToString(sum[:8]) {
		t.Errorf("etag %s is not the stored file's", etag)
	}
	// A background keeps its pixels.
	if ct, _, err := s.Put(Background, big); err != nil || ct != "image/webp" {
		t.Errorf("background: %s %v", ct, err)
	}
}

// A logo stored before Hashi fitted logos is fitted once; a second pass and an empty slot change nothing.
func TestRefitStoredLogo(t *testing.T) {
	big, err := os.ReadFile("testdata/wide-logo.webp")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, string(LogoLight)), big, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ct, etag, changed, err := s.Refit(LogoLight)
	if err != nil || !changed || ct != "image/png" || etag == "" {
		t.Fatalf("first refit: %s %s %v %v", ct, etag, changed, err)
	}
	if _, _, changed, err := s.Refit(LogoLight); err != nil || changed {
		t.Errorf("second refit changed it again: %v %v", changed, err)
	}
	if _, _, changed, err := s.Refit(LogoDark); err != nil || changed {
		t.Errorf("empty slot: %v %v", changed, err)
	}
}
