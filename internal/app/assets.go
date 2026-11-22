package app

import (
	"context"
	"fmt"
	"log/slog"

	"git.juzo.io/juzo/hashi/internal/assets"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store"
)

// refitLogos stores the public pages' logos again at their shown size once: logos uploaded or copied before Hashi
// fitted them (0.13.14) were kept as given, and one on prod was 227 KiB for a 131 px wide header. Each changed slot
// gets its new type and ETag, so browsers fetch the small file.
func refitLogos(ctx context.Context, st *store.Store, as *assets.Store, cfg *settings.Service, log *slog.Logger) error {
	a, err := settings.Get[settings.PublicAssets](ctx, st.R)
	if err != nil {
		return err
	}
	changed := false
	for _, slot := range []assets.Slot{assets.LogoDark, assets.LogoLight} {
		v, ok := a.Slots[string(slot)]
		if !ok {
			continue
		}
		ct, etag, done, err := as.Refit(slot)
		if err != nil {
			return err
		}
		if done {
			v.ContentType, v.ETag = ct, etag
			a.Slots[string(slot)] = v
			changed = true
			log.Info("logo stored at its shown size", "slot", slot)
		}
	}
	if !changed {
		return nil
	}
	if err := settings.Put(ctx, st.Q, a); err != nil {
		return fmt.Errorf("refit logos: %w", err)
	}
	cfg.Notify(a)
	return nil
}
