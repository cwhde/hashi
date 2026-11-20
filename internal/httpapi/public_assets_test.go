package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"git.juzo.io/juzo/hashi/internal/assets"
	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

var tinyPNG = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89")

func assetServer(t *testing.T) *Server {
	t.Helper()
	st := storetest.New(t)
	store, err := assets.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &Server{Store: st, Audit: audit.New([]byte("k")), Settings: settings.New(st), Assets: store,
		AssetSource: assets.Source{Client: http.DefaultClient}, Log: slog.New(slog.DiscardHandler)}
}

func get(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /assets/{slot}", s.ServeAsset)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// 07 §9: a file of the owner's source is copied once; pages load Hashi's copy, so the source sees one request only.
func TestPublicAssetsFromTheSource(t *testing.T) {
	ctx := context.Background()
	listing, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "assets", "listing-logo-2k.html"))
	if err != nil {
		t.Fatal(err)
	}
	var fileHits atomic.Int32
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/static/logo/2k/":
			_, _ = w.Write(listing)
		case "/static/logo/2k/logo_w_bd.png":
			fileHits.Add(1)
			_, _ = w.Write(tinyPNG)
		default:
			http.NotFound(w, r)
		}
	}))
	defer src.Close()
	s := assetServer(t)

	if _, err := s.ListPublicAssetSource(ctx, gen.ListPublicAssetSourceRequestObject{}); status(err) != 409 {
		t.Fatalf("no source yet: %v", err)
	}
	if _, err := s.PutPublicAssets(ctx, gen.PutPublicAssetsRequestObject{Body: &gen.PutPublicAssetsJSONRequestBody{Source: "file:///etc"}}); status(err) != 422 {
		t.Fatalf("non-http source: %v", err)
	}
	if _, err := s.PutPublicAssets(ctx, gen.PutPublicAssetsRequestObject{Body: &gen.PutPublicAssetsJSONRequestBody{Source: src.URL + "/static"}}); err != nil {
		t.Fatal(err)
	}
	dir := "logo/2k"
	l, err := s.ListPublicAssetSource(ctx, gen.ListPublicAssetSourceRequestObject{Params: gen.ListPublicAssetSourceParams{Dir: &dir}})
	if err != nil {
		t.Fatal(err)
	}
	if f := l.(gen.ListPublicAssetSource200JSONResponse); f.Dir != "logo/2k/" || len(f.Files) != 16 {
		t.Fatalf("listing %+v", f)
	}
	bad := "../../etc/passwd.png"
	if _, err := s.PutPublicAssetSlot(ctx, gen.PutPublicAssetSlotRequestObject{Slot: "logo_dark", Body: &gen.AssetSlotFill{Path: &bad}}); status(err) != 422 {
		t.Fatalf("path outside: %v", err)
	}
	path := "logo/2k/logo_w_bd.png"
	res, err := s.PutPublicAssetSlot(ctx, gen.PutPublicAssetSlotRequestObject{Slot: "logo_dark", Body: &gen.AssetSlotFill{Path: &path}})
	if err != nil {
		t.Fatal(err)
	}
	got := res.(gen.PutPublicAssetSlot200JSONResponse).Slots
	if len(got) != 1 || got[0].Origin != "source" || *got[0].Path != path || !strings.HasPrefix(got[0].Url, "/assets/logo_dark?v=") {
		t.Fatalf("slot %+v", got)
	}
	for range 3 {
		if rec := get(t, s, got[0].Url); rec.Code != 200 || rec.Body.String() != string(tinyPNG) || rec.Header().Get("Content-Type") != "image/png" {
			t.Fatalf("serve %d %q", rec.Code, rec.Header())
		}
	}
	if fileHits.Load() != 1 {
		t.Fatalf("the source was asked %d times", fileHits.Load())
	}
	missing := "logo/2k/gone.png"
	if _, err := s.PutPublicAssetSlot(ctx, gen.PutPublicAssetSlotRequestObject{Slot: "icon", Body: &gen.AssetSlotFill{Path: &missing}}); status(err) != 422 {
		t.Fatalf("missing file: %v", err)
	}
}

// 07 §9: uploads take images only; the public pages name what is filled, the background only when it is on; an SVG
// is served so it can run nothing; an emptied slot answers 404.
func TestPublicPagesNameTheirAssets(t *testing.T) {
	ctx := context.Background()
	s := assetServer(t)
	put := func(slot gen.AssetSlotName, data []byte) error {
		_, err := s.PutPublicAssetSlot(ctx, gen.PutPublicAssetSlotRequestObject{Slot: slot, Body: &gen.AssetSlotFill{Data: &data}})
		return err
	}
	if err := put("icon", []byte("<html><script>alert(1)</script></html>")); status(err) != 422 {
		t.Fatalf("html upload: %v", err)
	}
	if err := put("icon", []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)); err != nil {
		t.Fatal(err)
	}
	if err := put("background", tinyPNG); err != nil {
		t.Fatal(err)
	}
	pa, err := settings.Get[settings.PublicAssets](ctx, s.Store.R)
	if err != nil {
		t.Fatal(err)
	}
	shown := pageAssets(pa)
	if shown.Icon == nil || shown.LogoDark != nil || shown.Background != nil {
		t.Fatalf("background off: %+v", shown)
	}
	if _, err := s.PutPublicAssets(ctx, gen.PutPublicAssetsRequestObject{Body: &gen.PutPublicAssetsJSONRequestBody{Background: true}}); err != nil {
		t.Fatal(err)
	}
	pa, _ = settings.Get[settings.PublicAssets](ctx, s.Store.R)
	if shown = pageAssets(pa); shown.Background == nil || len(pa.Slots) != 2 {
		t.Fatalf("background on: %+v %+v", shown, pa)
	}
	rec := get(t, s, *shown.Icon)
	csp := rec.Header().Get("Content-Security-Policy")
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "image/svg+xml" || !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "sandbox") ||
		rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("svg %d %q", rec.Code, rec.Header())
	}
	if _, err := s.DeletePublicAssetSlot(ctx, gen.DeletePublicAssetSlotRequestObject{Slot: "icon"}); err != nil {
		t.Fatal(err)
	}
	if rec := get(t, s, "/assets/icon"); rec.Code != 404 {
		t.Fatalf("emptied %d", rec.Code)
	}
	if rec := get(t, s, "/assets/hashi.db"); rec.Code != 404 {
		t.Fatalf("not a slot %d", rec.Code)
	}
	// Both public pages take their look, assets included, from publicLook.
	if _, _, a, err := s.publicLook(ctx); err != nil || a == nil || a.Icon != nil || a.Background == nil {
		t.Fatalf("public look %+v %v", a, err)
	}
}
