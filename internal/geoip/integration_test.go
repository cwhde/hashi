//go:build integration

package geoip

import (
	"net/http"
	"net/netip"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/ipsets"
)

// The real files through the real download path (a public mirror of the GeoLite2 files): archive, unpack, edition check,
// swap, lookups and prefix lists.
func TestIntegrationDownloadAndRead(t *testing.T) {
	dir := t.TempDir()
	s := New(dir, &http.Client{Timeout: 3 * time.Minute}, time.Now)
	src := Source{URL: "https://github.com/GitSquared/node-geolite2-redist/raw/refs/heads/master/redist/{edition}.tar.gz"}
	changed, err := s.Update(t.Context(), src, false)
	if err != nil || !changed {
		t.Fatalf("update: %v, changed %v", err, changed)
	}
	if got := len(s.Statuses()); got != 2 {
		t.Fatalf("%d databases", got)
	}
	country, _, asn, ok := s.Lookup(netip.MustParseAddr("8.8.8.8"))
	if !ok || country != "US" || asn != 15169 {
		t.Fatalf("lookup: %q %d %v", country, asn, ok)
	}
	ch, err := s.Prefixes(ipsets.Geo{Kind: ipsets.GeoCountry, Value: "CH"})
	if err != nil || len(ch) < 1000 {
		t.Fatalf("CH: %d prefixes, %v", len(ch), err)
	}
	var v4, v6 int
	for _, p := range ch {
		if p.Addr().Is4() {
			v4++
		} else {
			v6++
		}
		if p != p.Masked() || p.Addr().Is4In6() {
			t.Fatalf("not normal: %s", p)
		}
	}
	t.Logf("CH: %d v4, %d v6", v4, v6)
	as, err := s.Prefixes(ipsets.Geo{Kind: ipsets.GeoASN, Value: "15169"})
	if err != nil || len(as) == 0 {
		t.Fatalf("AS: %d, %v", len(as), err)
	}
	// Fresh files are not fetched again.
	if changed, err := s.Update(t.Context(), src, false); err != nil || changed {
		t.Fatalf("second update: %v, %v", changed, err)
	}
	// A restart reads what is on disk.
	if len(New(dir, http.DefaultClient, time.Now).Statuses()) != 2 {
		t.Fatal("not found after a restart")
	}
}
