package appicons

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	pngenc "image/png"
	"os"

	"git.juzo.io/juzo/hashi/internal/imgfit"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// maxSide is the largest side Hashi stores an app icon at. Pages show icons at 40 px, so 128 px stays sharp on a
// 3× screen; sources often name 512 px or larger files (one on prod was 204 KiB).
const maxSide = 128

// shrink scales a PNG or JPEG larger than maxSide down to it, keeping its shape. Any other file, one that is small
// already, one that doesn't decode, or one that would come out larger is kept as it is.
func shrink(f Found) Found {
	var enc func(*bytes.Buffer, image.Image) error
	switch f.ContentType {
	case "image/png":
		enc = func(b *bytes.Buffer, m image.Image) error {
			return (&pngenc.Encoder{CompressionLevel: pngenc.BestCompression}).Encode(b, m)
		}
	case "image/jpeg":
		enc = func(b *bytes.Buffer, m image.Image) error { return jpeg.Encode(b, m, &jpeg.Options{Quality: 90}) }
	default:
		return f
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(f.Data))
	if err != nil || max(cfg.Width, cfg.Height) <= maxSide {
		return f
	}
	src, _, err := image.Decode(bytes.NewReader(f.Data))
	if err != nil {
		return f
	}
	var b bytes.Buffer
	if err := enc(&b, imgfit.Fit(src, maxSide, maxSide)); err != nil || b.Len() >= len(f.Data) {
		return f
	}
	f.Data = b.Bytes()
	return f
}

// shrinkStored scales down the icons stored before Hashi shrank them (0.13.11): library icons and uploads are never
// fetched again, so they would keep their full size.
func (s *Service) shrinkStored(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.st.R.ListAppIcons(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if !keyForm.MatchString(r.App) {
			continue // as Open: a key never names a path outside the icon folder
		}
		changed := false
		for _, v := range []struct {
			light    bool
			ct, etag *string
		}{{false, &r.ContentType, &r.Etag}, {true, &r.LightType, &r.LightEtag}} {
			if *v.etag == "" {
				continue
			}
			p := s.path(r.App, v.light)
			b, err := os.ReadFile(p) //nolint:gosec // p is in the icon folder: keyForm allows no separators
			if err != nil {
				continue // a missing file serves 404 already; nothing to shrink
			}
			out := shrink(Found{Data: b, ContentType: *v.ct})
			if len(out.Data) == len(b) {
				continue
			}
			if err := write(p, out.Data); err != nil {
				return err
			}
			*v.etag, changed = etag(out.Data), true
		}
		if changed {
			if err := s.st.Q.UpsertAppIcon(ctx, db.UpsertAppIconParams(r)); err != nil {
				return err
			}
		}
	}
	return nil
}
