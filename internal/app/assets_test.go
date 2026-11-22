package app

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"git.juzo.io/juzo/hashi/internal/assets"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

// A logo stored before Hashi fitted logos (prod's was 227 KiB) is stored again at its shown size on start, and its
// slot gets the new type and ETag, so the pages link the small file. Its origin and source path stay.
func TestStoredLogoIsFittedOnStart(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	big, err := os.ReadFile(filepath.Join("..", "assets", "testdata", "wide-logo.webp"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, string(assets.LogoDark)), big, 0o600); err != nil {
		t.Fatal(err)
	}
	as, err := assets.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	old := settings.AssetSlot{Origin: settings.AssetFromSource, Path: "logos/dark.webp", ContentType: "image/webp", ETag: "0123456789abcdef"}
	if err := settings.Put(ctx, st.Q, settings.PublicAssets{Source: "https://assets.example.com/", Slots: map[string]settings.AssetSlot{"logo_dark": old}}); err != nil {
		t.Fatal(err)
	}
	cfg := settings.New(st)
	if err := refitLogos(ctx, st, as, cfg, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	a, err := settings.Get[settings.PublicAssets](ctx, st.R)
	if err != nil {
		t.Fatal(err)
	}
	got := a.Slots["logo_dark"]
	if got.ContentType != "image/png" || got.ETag == old.ETag || got.Origin != old.Origin || got.Path != old.Path {
		t.Errorf("slot after start: %+v", got)
	}
	if fi, _ := os.Stat(filepath.Join(dir, string(assets.LogoDark))); fi == nil || fi.Size() >= int64(len(big))/2 {
		t.Errorf("stored logo not fitted: %v", fi)
	}
	// The next start changes nothing.
	if err := refitLogos(ctx, st, as, cfg, slog.New(slog.DiscardHandler)); err != nil {
		t.Fatal(err)
	}
	if b, _ := settings.Get[settings.PublicAssets](ctx, st.R); b.Slots["logo_dark"].ETag != got.ETag {
		t.Error("a second start stored the logo again")
	}
}
