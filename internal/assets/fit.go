package assets

import (
	"bytes"
	"image"
	pngenc "image/png"

	_ "golang.org/x/image/webp" // logos often come as WebP; Go's library reads PNG only

	"git.juzo.io/juzo/hashi/internal/imgfit"
)

// The box a logo is stored in: the public pages draw it at most 40 px tall and 256 px wide, and 3× keeps it sharp on
// a phone. Owners' logos are often far larger (one on prod was 3840×1174, 227 KiB, for a 131 px wide header).
const logoW, logoH = 768, 120

// fit scales a logo larger than its box down to it and stores it as PNG. Any other slot, an SVG or icon file, one that
// fits already, one that doesn't decode, or one that would come out larger is kept as it is.
func fit(slot Slot, data []byte, ct string) ([]byte, string) {
	if (slot != LogoDark && slot != LogoLight) || (ct != "image/png" && ct != "image/webp") {
		return data, ct
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (cfg.Width <= logoW && cfg.Height <= logoH) {
		return data, ct
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return data, ct
	}
	var b bytes.Buffer
	enc := pngenc.Encoder{CompressionLevel: pngenc.BestCompression}
	if err := enc.Encode(&b, imgfit.Fit(src, logoW, logoH)); err != nil || b.Len() >= len(data) {
		return data, ct
	}
	return b.Bytes(), "image/png"
}
