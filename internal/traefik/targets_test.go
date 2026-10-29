package traefik_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/netip"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	"git.juzo.io/juzo/hashi/internal/traefik"
)

// Invariant 21 for upstreams (05a §5.2): an address-set target renders one server per resolved address; when the
// set resolves to nothing it keeps the last good addresses and records why; a target that never resolved leaves its
// service out of the render with the reason instead of rendering an empty load balancer.
func TestInvariant_21_UpstreamKeepsLastGood(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	sets := ipsets.NewService(st, audit.New([]byte("k")), slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	var addrs []netip.Addr
	sets.SetHosts(func(context.Context, string, string) ([]netip.Addr, error) {
		if addrs == nil {
			return nil, errors.New("agent never seen")
		}
		return addrs, nil
	}, func(context.Context, *db.Queries, string) (string, error) { return "home", nil })
	set, err := ipsets.CreateSet(ctx, st.Q, ipsets.SetInput{Slug: "home", Name: "Home"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sets.Editor().AddElement(ctx, st.Q, set.ID, ipsets.ElementInput{Item: ipsets.Host{HostID: "h1", Scope: ipsets.ScopeInternal}}, "t", time.Now()); err != nil {
		t.Fatal(err)
	}
	svc := services.Service{ID: "s1", Name: "Wiki", Slug: "wiki", Kind: services.KindHTTP, HostName: "wiki.example.com", Enabled: true,
		Target: services.Target{SetID: set.ID, Port: 8080}}
	other := svc
	other.ID, other.Slug = "s2", "other"

	render := func(s services.Service) (services.Service, string) {
		t.Helper()
		got, err := services.Resolve(ctx, []services.Service{s}, sets.ResolveTarget)
		if err != nil {
			t.Fatal(err)
		}
		if got[0].InvalidReason != "" {
			return got[0], ""
		}
		y, err := traefik.EffectiveYAML(got[0])
		if err != nil {
			t.Fatal(err)
		}
		return got[0], string(y)
	}

	if s, _ := render(other); !strings.Contains(s.InvalidReason, "agent never seen") {
		t.Fatalf("never resolved: %q", s.InvalidReason)
	}
	addrs = []netip.Addr{netip.MustParseAddr("10.0.4.20"), netip.MustParseAddr("fd00::20")}
	_, y := render(svc)
	if !strings.Contains(y, "http://10.0.4.20:8080") || !strings.Contains(y, "http://[fd00::20]:8080") {
		t.Fatalf("resolved render:\n%s", y)
	}
	addrs = nil
	s, y2 := render(svc)
	if s.InvalidReason != "" || y2 != y {
		t.Fatalf("an empty resolution must keep the last good servers: %q\n%s", s.InvalidReason, y2)
	}
	un, err := st.R.ListUnresolvedIPSetTargets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(un) != 2 || !strings.Contains(un[1].UnresolvedReason, "agent never seen") {
		t.Fatalf("unresolved targets %+v", un)
	}
}
