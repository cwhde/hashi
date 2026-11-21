package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/assets"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/settings"
)

// The public pages' icon, logos and background (07 §9, bug 30). The setting says what each slot holds; the files live
// in s.Assets. Every page, public or admin, loads them from Hashi at /assets/<slot>, never from the source.

func assetURL(slot string, etag string) string { return "/assets/" + slot + "?v=" + etag }

func publicAssetsBody(a settings.PublicAssets) gen.PublicAssets {
	out := gen.PublicAssets{Source: a.Source, Background: a.Background, Slots: []gen.PublicAssetSlot{}}
	for _, slot := range assets.Slots {
		v, ok := a.Slots[string(slot)]
		if !ok {
			continue
		}
		g := gen.PublicAssetSlot{Slot: gen.AssetSlotName(slot), Origin: gen.PublicAssetSlotOrigin(v.Origin), ContentType: v.ContentType, Url: assetURL(string(slot), v.ETag)}
		if v.Path != "" {
			g.Path = &v.Path
		}
		out.Slots = append(out.Slots, g)
	}
	return out
}

// pageAssets is what a public page shows: the filled slots, the background only when it is switched on.
func pageAssets(a settings.PublicAssets) *gen.PublicPageAssets {
	at := func(slot assets.Slot) *string {
		v, ok := a.Slots[string(slot)]
		if !ok {
			return nil
		}
		u := assetURL(string(slot), v.ETag)
		return &u
	}
	out := &gen.PublicPageAssets{Icon: at(assets.Icon), LogoDark: at(assets.LogoDark), LogoLight: at(assets.LogoLight)}
	if a.Background {
		out.Background = at(assets.Background)
	}
	return out
}

func (s *Server) GetPublicAssets(ctx context.Context, _ gen.GetPublicAssetsRequestObject) (gen.GetPublicAssetsResponseObject, error) {
	a, err := settings.Get[settings.PublicAssets](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	return gen.GetPublicAssets200JSONResponse(publicAssetsBody(a)), nil
}

func (s *Server) PutPublicAssets(ctx context.Context, req gen.PutPublicAssetsRequestObject) (gen.PutPublicAssetsResponseObject, error) {
	var out settings.PublicAssets
	err := s.updateAssets(ctx, func(a *settings.PublicAssets) error {
		a.Source, a.Background = req.Body.Source, req.Body.Background
		out = *a
		return nil
	})
	if err != nil {
		return nil, err
	}
	return gen.PutPublicAssets200JSONResponse(publicAssetsBody(out)), nil
}

func (s *Server) ListPublicAssetSource(ctx context.Context, req gen.ListPublicAssetSourceRequestObject) (gen.ListPublicAssetSourceResponseObject, error) {
	a, err := settings.Get[settings.PublicAssets](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	if a.Source == "" {
		return nil, conflict("Set the asset source first.")
	}
	dir := ""
	if req.Params.Dir != nil {
		dir = *req.Params.Dir
	}
	clean, err := assets.CleanDir(dir)
	if err != nil {
		return nil, assetProblem(err)
	}
	l, err := s.AssetSource.List(ctx, a.Source, clean)
	if err != nil {
		return nil, assetProblem(err)
	}
	out := gen.AssetFolder{Dir: clean, Dirs: l.Dirs, Files: l.Files}
	if out.Dirs == nil {
		out.Dirs = []string{}
	}
	if out.Files == nil {
		out.Files = []string{}
	}
	return gen.ListPublicAssetSource200JSONResponse(out), nil
}

func (s *Server) PutPublicAssetSlot(ctx context.Context, req gen.PutPublicAssetSlotRequestObject) (gen.PutPublicAssetSlotResponseObject, error) {
	slot := assets.Slot(req.Slot)
	if !slot.Valid() {
		return nil, invalid("unknown slot")
	}
	b := req.Body
	if (b.Path == nil) == (b.Data == nil) {
		return nil, invalid("Give either a file of the source or an upload.")
	}
	fill := settings.AssetSlot{Origin: settings.AssetUploaded}
	var data []byte
	if b.Path != nil {
		a, err := settings.Get[settings.PublicAssets](ctx, s.Store.R)
		if err != nil {
			return nil, err
		}
		if a.Source == "" {
			return nil, conflict("Set the asset source first.")
		}
		clean, err := assets.CleanFile(*b.Path)
		if err != nil {
			return nil, assetProblem(err)
		}
		if data, err = s.AssetSource.Fetch(ctx, a.Source, clean, slot); err != nil {
			return nil, assetProblem(err)
		}
		fill = settings.AssetSlot{Origin: settings.AssetFromSource, Path: clean}
	} else {
		data = *b.Data
	}
	ct, etag, err := s.Assets.Put(slot, data)
	if err != nil {
		return nil, assetProblem(err)
	}
	fill.ContentType, fill.ETag = ct, etag
	var out settings.PublicAssets
	err = s.updateAssets(ctx, func(a *settings.PublicAssets) error {
		a.Slots[string(slot)] = fill
		out = *a
		return nil
	})
	if err != nil {
		return nil, err
	}
	return gen.PutPublicAssetSlot200JSONResponse(publicAssetsBody(out)), nil
}

func (s *Server) DeletePublicAssetSlot(ctx context.Context, req gen.DeletePublicAssetSlotRequestObject) (gen.DeletePublicAssetSlotResponseObject, error) {
	slot := assets.Slot(req.Slot)
	if !slot.Valid() {
		return nil, invalid("unknown slot")
	}
	var out settings.PublicAssets
	err := s.updateAssets(ctx, func(a *settings.PublicAssets) error {
		delete(a.Slots, string(slot))
		out = *a
		return nil
	})
	if err != nil {
		return nil, err
	}
	// The file goes after the setting stops naming it, so no page links a missing file.
	if err := s.Assets.Remove(slot); err != nil {
		return nil, err
	}
	return gen.DeletePublicAssetSlot200JSONResponse(publicAssetsBody(out)), nil
}

// updateAssets changes the stored setting in one transaction with its audit event.
func (s *Server) updateAssets(ctx context.Context, change func(a *settings.PublicAssets) error) error {
	a, err := settings.Get[settings.PublicAssets](ctx, s.Store.R)
	if err != nil {
		return err
	}
	if a.Slots == nil {
		a.Slots = map[string]settings.AssetSlot{}
	}
	if err := change(&a); err != nil {
		return err
	}
	return saveSetting(ctx, s, a, nil)
}

func assetProblem(err error) error {
	if errors.Is(err, assets.ErrInvalid) {
		return invalid(strings.TrimPrefix(err.Error(), assets.ErrInvalid.Error()+": "))
	}
	return providerFailed("The asset source did not answer: " + err.Error())
}

// ServeAsset serves one slot's file to anyone: the public pages need them without a session. An SVG may carry
// scripts, so it is served as a document that can run none and load nothing.
func (s *Server) ServeAsset(w http.ResponseWriter, r *http.Request) {
	slot := assets.Slot(r.PathValue("slot"))
	a, err := settings.Get[settings.PublicAssets](r.Context(), s.Store.R)
	v, ok := a.Slots[string(slot)]
	if err != nil || !ok || s.Assets == nil {
		http.NotFound(w, r)
		return
	}
	f, err := s.Assets.Open(slot)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	h := w.Header()
	h.Set("Content-Type", v.ContentType)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	cacheAsset(h, r, v.ETag)
	http.ServeContent(w, r, "", time.Time{}, f)
}

// cacheAsset sets the caching headers for a stored file whose current version is tag. Pages name it as ?v=<tag>, and
// that URL never changes content, so browsers keep it a year without asking; any other URL is checked after five
// minutes.
func cacheAsset(h http.Header, r *http.Request, tag string) {
	if r.URL.Query().Get("v") == tag {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "public, max-age=300")
	}
	h.Set("ETag", `"`+tag+`"`)
}

// PublicIconURL is the owner's uploaded icon for the public pages' HTML (bug 160), or "" when there is none or the
// settings can't be read (the page then names Hashi's mark).
func (s *Server) PublicIconURL(r *http.Request) string {
	a, err := settings.Get[settings.PublicAssets](r.Context(), s.Store.R)
	if err != nil {
		return ""
	}
	if v, ok := a.Slots[string(assets.Icon)]; ok {
		return assetURL(string(assets.Icon), v.ETag)
	}
	return ""
}
