// Package imgfit scales images down to the size Hashi shows them at: app icons and the public pages' logos are often
// stored far larger than any screen draws them (a 3840 px logo shown 120 px wide at most).
package imgfit

import (
	"image"
	"image/color"
)

// Fit scales src down to fit inside w×h, keeping its shape. An image that fits already comes back as it is.
func Fit(src image.Image, w, h int) image.Image {
	r := src.Bounds()
	sw, sh := r.Dx(), r.Dy()
	if sw <= w && sh <= h {
		return src
	}
	// The side that overflows more decides the scale.
	tw, th := w, max(1, sh*w/sw)
	if th > h {
		tw, th = max(1, sw*h/sh), h
	}
	return scale(src, tw, th)
}

// scale averages the source pixels under each target pixel (area averaging, in premultiplied colour so transparent
// edges don't darken).
func scale(src image.Image, tw, th int) image.Image {
	r := src.Bounds()
	w, h := r.Dx(), r.Dy()
	dst := image.NewNRGBA(image.Rect(0, 0, tw, th))
	for y := range th {
		y0, y1 := r.Min.Y+y*h/th, r.Min.Y+max((y+1)*h/th, y*h/th+1)
		for x := range tw {
			x0, x1 := r.Min.X+x*w/tw, r.Min.X+max((x+1)*w/tw, x*w/tw+1)
			var sr, sg, sb, sa, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					cr, cg, cb, ca := src.At(sx, sy).RGBA() // premultiplied, 16 bit
					sr, sg, sb, sa, n = sr+uint64(cr), sg+uint64(cg), sb+uint64(cb), sa+uint64(ca), n+1
				}
			}
			// Each sum holds n values of at most 0xffff, so every average fits 16 bits.
			px := color.RGBA64{R: uint16(sr / n), G: uint16(sg / n), B: uint16(sb / n), A: uint16(sa / n)} //nolint:gosec // see above
			dst.Set(x, y, px)
		}
	}
	return dst
}
