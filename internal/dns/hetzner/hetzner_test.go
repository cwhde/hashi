package hetzner

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/dns/hetzner/hetznertest"
	"git.juzo.io/juzo/hashi/internal/secrets"
)

// fakeClock records sleeps instead of sleeping.
type fakeClock struct {
	mu    sync.Mutex
	t     time.Time
	slept []time.Duration
}

func (f *fakeClock) now() time.Time { f.mu.Lock(); defer f.mu.Unlock(); return f.t }

func (f *fakeClock) sleep(_ context.Context, d time.Duration) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.slept = append(f.slept, d)
	f.t = f.t.Add(d)
	return nil
}

func replay(t *testing.T) (*Client, *hetznertest.Server, *fakeClock) {
	t.Helper()
	srv := hetznertest.NewServer(t)
	// The capture's clock: RateLimit-Reset values lie just after it.
	clk := &fakeClock{t: time.Date(2026, 10, 28, 7, 58, 38, 0, time.UTC)}
	c := New(secrets.New(hetznertest.Token), WithBaseURL(srv.BaseURL()), WithClock(clk.now, clk.sleep))
	return c, srv, clk
}

func TestContract_ListZonesFollowsPagination(t *testing.T) {
	c, srv, _ := replay(t)
	zones, err := c.ListZones(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []dns.Zone{{ID: "100001", Name: "example.com", TTL: 3600}, {ID: "100002", Name: "example.org", TTL: 3600}}
	if len(zones) != len(want) {
		t.Fatalf("zones = %+v", zones)
	}
	for i := range want {
		if zones[i].Name != want[i].Name || zones[i].TTL != want[i].TTL || zones[i].ID == "" {
			t.Errorf("zone %d = %+v, want %+v", i, zones[i], want[i])
		}
	}
	reqs := srv.Requests()
	if len(reqs) != 2 || !strings.Contains(reqs[0], "page=1") || !strings.Contains(reqs[1], "page=2") {
		t.Errorf("requests = %v", reqs)
	}
}

func TestContract_ResolveZoneLongestSuffix(t *testing.T) {
	c, _, _ := replay(t)
	ctx := context.Background()
	for _, fqdn := range []string{"example.com", "svc1.example.com", "api.svc4.example.com.", "SVC1.Example.COM"} {
		z, err := c.ResolveZone(ctx, fqdn)
		if err != nil || z.Name != "example.com" {
			t.Errorf("ResolveZone(%q) = %+v, %v", fqdn, z, err)
		}
	}
	if _, err := c.ResolveZone(ctx, "notexample.com"); !errors.Is(err, dns.ErrZoneNotFound) {
		t.Errorf("notexample.com: err = %v, want ErrZoneNotFound", err)
	}
}

func TestContract_GetZone(t *testing.T) {
	c, _, _ := replay(t)
	z, err := c.GetZone(context.Background(), "100001")
	if err != nil || z.Name != "example.com" || z.TTL != 3600 {
		t.Fatalf("GetZone = %+v, %v", z, err)
	}
	// error_unknown_zone.json: 404 not_found.
	if _, err := c.GetZone(context.Background(), "999999999"); !errors.Is(err, dns.ErrZoneNotFound) {
		t.Errorf("unknown zone: err = %v", err)
	}
}

func TestContract_ListRRSets(t *testing.T) {
	c, srv, _ := replay(t)
	rrs, err := c.ListRRSets(context.Background(), dns.Zone{ID: "100001", Name: "example.com", TTL: 3600})
	if err != nil {
		t.Fatal(err)
	}
	// total_entries in the capture.
	if len(rrs) != 58 {
		t.Fatalf("got %d RRSets, want 58", len(rrs))
	}
	if n := len(srv.Requests()); n != 3 {
		t.Errorf("%d requests, want 3 pages", n)
	}
	by := map[string]dns.RRSet{}
	for _, r := range rrs {
		by[r.Name+"/"+r.Type] = r
	}
	apexA := by["@/A"]
	if apexA.TTL != nil || len(apexA.Values) != 1 || apexA.Values[0] != "192.0.2.1" || apexA.EffectiveTTL(3600) != 3600 {
		t.Errorf("@/A = %+v (ttl null must stay zone default)", apexA)
	}
	if soa := by["@/SOA"]; soa.TTL == nil || *soa.TTL != 300 {
		t.Errorf("@/SOA ttl = %v, want 300", soa.TTL)
	}
	if ns := by["@/NS"]; len(ns.Values) != 3 || ns.Values[0] != "helium.ns.hetzner.de." {
		t.Errorf("@/NS = %+v (values sorted)", ns.Values)
	}
	if mx := by["@/MX"]; len(mx.Values) != 4 || mx.Values[0] != "10 host4.ext4.example.net." {
		t.Errorf("@/MX = %+v", mx.Values)
	}
	if cn := by["svc2/CNAME"]; cn.TTL == nil || *cn.TTL != 3600 || cn.Values[0] != "on.edge-a.example.com." {
		t.Errorf("svc2/CNAME = %+v", cn)
	}
	// The DKIM value is two quoted strings in one record: decoded to one text.
	dkim := by["selector1._domainkey/TXT"]
	if len(dkim.Values) != 1 || strings.ContainsAny(dkim.Values[0], `" `) || len(dkim.Values[0]) < 300 {
		t.Errorf("DKIM TXT not decoded into one string: %q", dkim.Values)
	}
	if txt := by["@/TXT"]; len(txt.Values) != 2 || strings.Contains(txt.Values[0], `"`) {
		t.Errorf("@/TXT = %q", txt.Values)
	}
}

func TestContract_BadTokenIsActionable(t *testing.T) {
	srv := hetznertest.NewServer(t)
	c := New(secrets.New("legacy-dns-console-token"), WithBaseURL(srv.BaseURL()))
	_, err := c.ListZones(context.Background())
	if !errors.Is(err, dns.ErrUnauthorized) {
		t.Fatalf("err = %v, want ErrUnauthorized", err)
	}
	if !strings.Contains(err.Error(), "Cloud project API token") {
		t.Errorf("message not actionable: %v", err)
	}
	if strings.Contains(err.Error(), "legacy-dns-console-token") {
		t.Error("error contains the token (invariant 5)")
	}
}

// RateLimit-Remaining 0 makes the next request wait until RateLimit-Reset (headers as captured; no 429 was captured,
// so its body here is synthetic and only its status matters).
func TestRateLimitWaitsForReset(t *testing.T) {
	c, srv, clk := replay(t)
	reset := clk.now().Add(5 * time.Second).Unix()
	var calls atomic.Int32
	srv.Override = func(w http.ResponseWriter, r *http.Request) bool {
		if calls.Add(1) == 1 {
			w.Header().Set("RateLimit-Remaining", "0")
			w.Header().Set("RateLimit-Reset", strconv.FormatInt(reset, 10))
		}
		return false
	}
	if _, err := c.GetZone(context.Background(), "100001"); err != nil {
		t.Fatal(err)
	}
	if len(clk.slept) != 0 {
		t.Fatalf("slept before the budget was used up: %v", clk.slept)
	}
	if _, err := c.GetZone(context.Background(), "100001"); err != nil {
		t.Fatal(err)
	}
	if len(clk.slept) != 1 || clk.slept[0] != 5*time.Second {
		t.Errorf("slept %v, want one 5s wait", clk.slept)
	}
}

func TestRateLimit429RetriesAfterReset(t *testing.T) {
	c, srv, clk := replay(t)
	reset := clk.now().Add(3 * time.Second).Unix()
	var calls atomic.Int32
	srv.Override = func(w http.ResponseWriter, r *http.Request) bool {
		if calls.Add(1) > 1 {
			return false
		}
		w.Header().Set("RateLimit-Remaining", "0")
		w.Header().Set("RateLimit-Reset", strconv.FormatInt(reset, 10))
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":"rate_limited","message":"synthetic"}}`))
		return true
	}
	if _, err := c.GetZone(context.Background(), "100001"); err != nil {
		t.Fatalf("not retried after the reset: %v", err)
	}
	if calls.Load() != 2 || len(clk.slept) != 1 || clk.slept[0] != 3*time.Second {
		t.Errorf("calls=%d slept=%v", calls.Load(), clk.slept)
	}
}

func TestRateLimitTooFarAwayFails(t *testing.T) {
	c, srv, clk := replay(t)
	reset := clk.now().Add(time.Hour).Unix()
	srv.Override = func(w http.ResponseWriter, r *http.Request) bool {
		w.Header().Set("RateLimit-Remaining", "0")
		w.Header().Set("RateLimit-Reset", strconv.FormatInt(reset, 10))
		return false
	}
	if _, err := c.GetZone(context.Background(), "100001"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetZone(context.Background(), "100001"); !errors.Is(err, dns.ErrRateLimited) {
		t.Errorf("err = %v, want ErrRateLimited", err)
	}
	if len(clk.slept) != 0 {
		t.Errorf("slept %v for a reset an hour away", clk.slept)
	}
}

func TestServerErrorsAreRetried(t *testing.T) {
	c, srv, clk := replay(t)
	var calls atomic.Int32
	srv.Override = func(w http.ResponseWriter, r *http.Request) bool {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return true
		}
		return false
	}
	if _, err := c.GetZone(context.Background(), "100001"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 || len(clk.slept) != 2 {
		t.Errorf("calls=%d slept=%v", calls.Load(), clk.slept)
	}
}
