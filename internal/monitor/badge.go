package monitor

import (
	"fmt"
	"html"
	"math"
	"strings"
)

// Badges (07 §7.3): small SVG images for READMEs. The colours are fixed in the file, not theme tokens, because the
// image is shown on other sites, light and dark; each has at least 4.5:1 contrast with white. Text widths come from a
// width table for 11 px Helvetica/Arial, so no font is loaded and no font file is needed here. The look matches the
// approved mock (web/src/routes/design/badge.ts).

// Badge tones.
const (
	ToneGood    = "good"
	ToneWarn    = "warn"
	ToneBad     = "bad"
	ToneUnknown = "unknown"
)

var toneFill = map[string]string{ToneGood: "#1a7f37", ToneWarn: "#9a6700", ToneBad: "#cf222e", ToneUnknown: "#6e7781"}

const badgeLabelFill = "#3d444d"

// textWidth is the advance width at 11 px; anything not in the table counts as a lowercase letter.
func textWidth(s string, bold bool) int {
	w := 0.0
	for _, c := range s {
		switch {
		case strings.ContainsRune(`iljI.,:;|!'"`, c):
			w += 2.6
		case strings.ContainsRune("frt ()[]-/", c):
			w += 3.5
		case strings.ContainsRune("mwMW%@", c):
			w += 9.2
		case c >= '0' && c <= '9':
			w += 6.1
		case c >= 'A' && c <= 'Z':
			w += 7.2
		default:
			w += 5.9
		}
	}
	if bold {
		w *= 1.07
	}
	return int(math.Ceil(w))
}

// BadgeSVG draws one badge: a dark label and the value on its tone; dot puts a dot before the value (the status badge).
func BadgeSVG(label, value, tone string, dot bool) []byte {
	fill, ok := toneFill[tone]
	if !ok {
		fill = toneFill[ToneUnknown]
	}
	const pad = 7
	dotW := 0
	if dot {
		dotW = 10
	}
	lw := textWidth(label, false) + 2*pad
	vw := textWidth(value, true) + 2*pad + dotW
	w := lw + vw
	l, v := html.EscapeString(label), html.EscapeString(value)
	text := func(x int, s string, weight int) string {
		return fmt.Sprintf(`<text x="%d" y="15" fill="#010101" fill-opacity=".3" font-weight="%d">%s</text><text x="%d" y="14" font-weight="%d">%s</text>`,
			x, weight, s, x, weight, s)
	}
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20" role="img" aria-label="%s: %s"><title>%s: %s</title>`, w, l, v, l, v)
	b.WriteString(`<linearGradient id="s" x2="0" y2="100%"><stop offset="0" stop-color="#fff" stop-opacity=".1"/><stop offset="1" stop-opacity=".1"/></linearGradient>`)
	fmt.Fprintf(&b, `<clipPath id="r"><rect width="%d" height="20" rx="4" fill="#fff"/></clipPath>`, w)
	fmt.Fprintf(&b, `<g clip-path="url(#r)"><rect width="%d" height="20" fill="%s"/><rect x="%d" width="%d" height="20" fill="%s"/>`, lw, badgeLabelFill, lw, vw, fill)
	fmt.Fprintf(&b, `<rect width="%d" height="20" fill="url(#s)"/><rect x=".5" y=".5" width="%d" height="19" rx="3.5" fill="none" stroke="#000" stroke-opacity=".15"/></g>`, w, w-1)
	if dot {
		fmt.Fprintf(&b, `<circle cx="%d" cy="10" r="3" fill="#fff" fill-opacity=".95"/>`, lw+pad+3)
	}
	b.WriteString(`<g fill="#fff" font-family="Helvetica,Arial,sans-serif" font-size="11" text-rendering="geometricPrecision">`)
	b.WriteString(text(pad, l, 500))
	b.WriteString(text(lw+pad+dotW, v, 700))
	b.WriteString(`</g></svg>`)
	return []byte(b.String())
}

// BadgeKinds are the badges the status listener serves (07 §7.3); certificate only with a TLS check.
var BadgeKinds = []string{"status", "uptime-24h", "uptime-7d", "uptime-30d", "response-24h", "certificate"}

// Badge renders one kind for a service page's data; ok is false for an unknown kind or one the entry does not have.
func Badge(p PublicService, kind string) (svg []byte, ok bool) {
	pct := func(v *float64) string {
		if v == nil {
			return "no data"
		}
		if *v == 100 {
			return "100%"
		}
		return fmt.Sprintf("%.2f%%", *v)
	}
	uptime := func(label string, v *float64) ([]byte, bool) {
		tone := ToneUnknown
		if v != nil {
			tone = ToneBad
			if *v >= 99.5 {
				tone = ToneGood
			} else if *v >= 98 {
				tone = ToneWarn
			}
		}
		return BadgeSVG(label, pct(v), tone, false), true
	}
	switch kind {
	case "status":
		value, tone := "unknown", ToneUnknown
		switch p.State {
		case StateUp:
			value, tone = "up", ToneGood
		case StateDegraded:
			value, tone = "slow", ToneWarn
		case StateDown:
			value, tone = "down", ToneBad
		case StatePaused:
			value = "paused"
		}
		return BadgeSVG(p.Name, value, tone, true), true
	case "uptime-24h":
		return uptime("uptime 24h", p.Uptime24)
	case "uptime-7d":
		return uptime("uptime 7d", p.Uptime7)
	case "uptime-30d":
		return uptime("uptime 30d", p.Uptime30)
	case "response-24h":
		if p.AvgMs24 == nil {
			return BadgeSVG("response 24h", "no data", ToneUnknown, false), true
		}
		tone := ToneBad
		if *p.AvgMs24 < 300 {
			tone = ToneGood
		} else if *p.AvgMs24 < 1000 {
			tone = ToneWarn
		}
		return BadgeSVG("response 24h", fmt.Sprintf("%d ms", *p.AvgMs24), tone, false), true
	case "certificate":
		if p.CertDays == nil {
			return nil, false
		}
		d := *p.CertDays
		tone := ToneBad
		if d > 14 {
			tone = ToneGood
		} else if d > 7 {
			tone = ToneWarn
		}
		value := fmt.Sprintf("%d days", d)
		if d == 1 {
			value = "1 day"
		}
		return BadgeSVG("certificate", value, tone, false), true
	}
	return nil, false
}
