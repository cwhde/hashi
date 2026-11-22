package monitor

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"testing"
)

// Badges are well-formed SVG with the name escaped, a colour from the value, and no font or image loaded from
// anywhere; an entry without a TLS check has no certificate badge and an unknown kind none at all (07 §7.3).
func TestBadges(t *testing.T) {
	u := func(v float64) *float64 { return &v }
	ms := int64(151)
	days := 61
	p := PublicService{PublicEntry: PublicEntry{Name: `Home <Assistant> & "co"`, State: StateDown, Uptime24: u(97.92)},
		Uptime7: u(99.4), Uptime30: u(99.83), AvgMs24: &ms}
	for kind, want := range map[string]string{"status": "#cf222e", "uptime-24h": "#cf222e", "uptime-7d": "#9a6700", "uptime-30d": "#1a7f37",
		"response-24h": "#1a7f37"} {
		svg, ok := Badge(p, kind)
		if !ok {
			t.Fatalf("%s: none", kind)
		}
		d := xml.NewDecoder(bytes.NewReader(svg))
		for {
			if _, err := d.Token(); errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				t.Fatalf("%s: not XML: %v\n%s", kind, err, svg)
			}
		}
		s := string(svg)
		if !strings.Contains(s, `fill="`+want+`"`) {
			t.Errorf("%s: want colour %s in %s", kind, want, s)
		}
		if strings.Contains(s, "<Assistant>") || strings.Contains(s, "href") || strings.Contains(s, "@import") || strings.Contains(s, "url(http") {
			t.Errorf("%s: unescaped text or a link to elsewhere: %s", kind, s)
		}
	}
	if _, ok := Badge(p, "certificate"); ok {
		t.Error("a certificate badge without a TLS check")
	}
	p.CertDays = &days
	if svg, ok := Badge(p, "certificate"); !ok || !strings.Contains(string(svg), "61 days") {
		t.Errorf("certificate: %s", svg)
	}
	if _, ok := Badge(p, "stars"); ok {
		t.Error("an unknown kind")
	}
}
