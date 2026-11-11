package settings

import (
	"math"
	"regexp"
	"strconv"
)

// ThemeCustom selects Appearance.CustomTheme (10-ui.md §2).
const ThemeCustom = "custom"

// Roundness of a custom theme.
const (
	RoundSharp   = "sharp"
	RoundDefault = "default"
	RoundRound   = "round"
)

// MinTextContrast is the WCAG AA ratio text and muted text need on the surface, in every built-in theme too.
const MinTextContrast = 4.5

// CustomTheme is the admin's own palette. Hashi derives the remaining roles (shell, second surface, raised surface,
// text on the accent, hover and focus) from these, in the browser and for mail alike.
type CustomTheme struct {
	Canvas    string   `json:"canvas"`
	Surface   string   `json:"surface"`
	Text      string   `json:"text"`
	Muted     string   `json:"muted"`
	Border    string   `json:"border"`
	Accent    string   `json:"accent"`
	Charts    []string `json:"charts,omitempty"` // up to 5 chart colours; none = accent and the status colours
	Roundness string   `json:"roundness"`
}

var hexRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func luminance(hex string) float64 {
	c := func(i int) float64 {
		v, _ := strconv.ParseUint(hex[i:i+2], 16, 8)
		s := float64(v) / 255
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*c(1) + 0.7152*c(3) + 0.0722*c(5)
}

// Contrast is the WCAG contrast ratio of two #rrggbb colours (the same formula as web/src/lib/design/contrast.ts).
func Contrast(a, b string) float64 {
	la, lb := luminance(a), luminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// Validate refuses a theme with a malformed colour or unreadable text (10-ui.md §2).
func (t CustomTheme) Validate() error {
	for name, c := range map[string]string{"canvas": t.Canvas, "surface": t.Surface, "text": t.Text, "muted": t.Muted,
		"border": t.Border, "accent": t.Accent} {
		if !hexRe.MatchString(c) {
			return invalid("custom theme: %s must be a colour like #1a2b3c", name)
		}
	}
	if len(t.Charts) > 5 {
		return invalid("custom theme: at most 5 chart colours")
	}
	for _, c := range t.Charts {
		if !hexRe.MatchString(c) {
			return invalid("custom theme: chart colour %q must look like #1a2b3c", c)
		}
	}
	switch t.Roundness {
	case RoundSharp, RoundDefault, RoundRound:
	default:
		return invalid("custom theme: roundness is sharp, default or round")
	}
	for _, p := range []struct{ what, fg, bg string }{
		{"text on the surface", t.Text, t.Surface}, {"muted text on the surface", t.Muted, t.Surface},
		{"text on the canvas", t.Text, t.Canvas},
	} {
		if r := Contrast(p.fg, p.bg); r < MinTextContrast {
			return invalid("custom theme: %s is too faint (%.1f:1, needs %.1f:1)", p.what, r, MinTextContrast)
		}
	}
	return nil
}

// AccentText is black or white, whichever reads better on the accent.
func (t CustomTheme) AccentText() string {
	if Contrast("#000000", t.Accent) >= Contrast("#ffffff", t.Accent) {
		return "#000000"
	}
	return "#ffffff"
}

// Light reports a light canvas (the browser's colour scheme and the status text shades follow it).
func (t CustomTheme) Light() bool { return luminance(t.Canvas) > 0.4 }
