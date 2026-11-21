package appicons

import (
	"bytes"
	"context"
	"image"
	"image/color"
	pngenc "image/png"
	"io"
	"os"
	"testing"
)

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	m := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			m.Set(x, y, color.NRGBA{R: uint8(x * 7), G: uint8(y * 13), B: uint8(x ^ y), A: 255})
		}
	}
	var b bytes.Buffer
	if err := pngenc.Encode(&b, m); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func storedSize(t *testing.T, s *Service, key string) (int, image.Config) {
	t.Helper()
	f, _, _, err := s.Open(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(f)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := pngenc.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return len(b), cfg
}

// Pages show app icons at 40 px; a 512 px upload is stored at 128 px, keeping its shape, and a small one as it is.
// Icons stored at full size before this are scaled down once when the service starts.
func TestIconsAreStoredSmall(t *testing.T) {
	ctx := context.Background()
	srv, _ := library(t)
	s, key := appFixture(t, srv.URL, srv.Client())

	big := pngOf(t, 512, 256)
	if _, err := s.Upload(ctx, key, big); err != nil {
		t.Fatal(err)
	}
	if n, cfg := storedSize(t, s, key); cfg.Width != 128 || cfg.Height != 64 || n >= len(big) {
		t.Fatalf("stored %d×%d, %d bytes (upload %d)", cfg.Width, cfg.Height, n, len(big))
	}

	small := pngOf(t, 64, 64)
	if _, err := s.Upload(ctx, key, small); err != nil {
		t.Fatal(err)
	}
	if n, _ := storedSize(t, s, key); n != len(small) {
		t.Fatalf("small icon changed: %d bytes, was %d", n, len(small))
	}

	// An icon stored at full size by an older Hashi.
	if err := os.WriteFile(s.path(key, false), big, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := s.Icons(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.shrinkStored(ctx); err != nil {
		t.Fatal(err)
	}
	if _, cfg := storedSize(t, s, key); cfg.Width != 128 {
		t.Fatalf("stored icon not shrunk: %d px wide", cfg.Width)
	}
	after, err := s.Icons(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before[key].Etag == after[key].Etag {
		t.Fatal("the version must change with the file, or browsers keep the old one for a year")
	}
}
