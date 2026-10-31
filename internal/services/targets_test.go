package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Targets take address sets only (04 §1.5, 05a §5.2): a set with a CIDR is refused; once a service uses a set, the set
// cannot gain a CIDR or be deleted, and the error names the service; the entry host must exist.
func TestAddressSetTargets(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	sets := ipsets.NewService(st, audit.New([]byte("k")), slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	sets.AddConsumers(Consumers)
	mk := func(slug, item string) string {
		s, err := ipsets.CreateSet(ctx, st.Q, ipsets.SetInput{Slug: slug, Name: slug}, now)
		if err != nil {
			t.Fatal(err)
		}
		it, _ := ipsets.ParseIPOrCIDR(item)
		if _, err := sets.Editor().AddElement(ctx, st.Q, s.ID, ipsets.ElementInput{Item: it}, "t", now); err != nil {
			t.Fatal(err)
		}
		return s.ID
	}
	nets, addrs := mk("nets", "192.0.2.0/24"), mk("addrs", "192.0.2.20")

	s := web()
	s.Target = Target{SetID: nets, Port: 80}
	var inv *InvalidError
	if _, err := Create(ctx, st.Q, s, now); !errors.As(err, &inv) || inv.Field != "target.set_id" {
		t.Fatalf("a CIDR set as target: %v", err)
	}
	s.Target = Target{Address: "192.0.2.1", SetID: addrs, Port: 80}
	if _, err := Create(ctx, st.Q, s, now); !errors.As(err, &inv) {
		t.Fatalf("address and set together: %v", err)
	}
	s.Target, s.EntryHostID = Target{SetID: addrs, Port: 80}, "missing"
	if _, err := Create(ctx, st.Q, s, now); !errors.As(err, &inv) || inv.Field != "entry_host_id" {
		t.Fatalf("unknown entry host: %v", err)
	}
	s.EntryHostID = ""
	created, err := Create(ctx, st.Q, s, now)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := Get(ctx, st.Q, created.ID); got.Target.SetID != addrs || got.Target.Address != "" {
		t.Fatalf("stored target %+v", got.Target)
	}
	it, _ := ipsets.ParseIPOrCIDR("198.51.100.0/24")
	if _, err := sets.Editor().AddElement(ctx, st.Q, addrs, ipsets.ElementInput{Item: it}, "t", now); !errors.Is(err, ipsets.ErrInvalid) ||
		!strings.Contains(err.Error(), "Service Search") {
		t.Fatalf("a CIDR into a target set: %v", err)
	}
	if _, err := ipsets.DeleteSet(ctx, st.Q, addrs, sets.Sources()); err == nil || !strings.Contains(err.Error(), "Service Search") {
		t.Fatalf("delete of a target set: %v", err)
	}
}

// Review of PR C: only rendered positions count. A disabled route (or any route while advanced routes are off) whose
// set never resolved changes nothing; an enabled one that never resolved is left out, the service keeps serving.
func TestResolveIgnoresUnrenderedRoutes(t *testing.T) {
	never := func(context.Context, string, string) (ipsets.Target, error) {
		return ipsets.Target{Unresolved: "agent never seen"}, nil
	}
	s := web()
	s.ID = "s1"
	s.Routes = []Route{{ID: "r1", Enabled: false, Targets: []Target{{SetID: "x", Port: 80}}}}
	got, err := Resolve(context.Background(), []Service{s}, never)
	if err != nil || !got[0].Renderable() {
		t.Fatalf("a disabled route took the service out: %v %q", err, got[0].InvalidReason)
	}
	s.AdvancedRoutes, s.Routes[0].Enabled = true, true
	got, _ = Resolve(context.Background(), []Service{s}, never)
	if !got[0].Renderable() || got[0].Routes[0].Enabled {
		t.Fatalf("an unresolved route must be left out, not the service: %q %+v", got[0].InvalidReason, got[0].Routes)
	}
	s.Target = Target{SetID: "x", Port: 80}
	if got, _ = Resolve(context.Background(), []Service{s}, never); got[0].Renderable() {
		t.Fatal("an unresolved simple target has nothing to serve")
	}
}

// Review of PR D: a TCP stream on a connection's API or forward-auth port is refused at save.
func TestStreamOnTraefikAPIPortRefused(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	if err := st.Q.InsertTraefikConnection(ctx, db.InsertTraefikConnectionParams{ID: "t1", Name: "edge", SshHost: "192.0.2.1", SshPort: 22, SshUser: "root",
		Auth: "hashi_key", HostKeyType: "ssh-ed25519", HostKey: []byte{1}, HostKeyFingerprint: "x"}); err != nil {
		t.Fatal(err)
	}
	c, err := st.R.GetTraefikConnection(ctx, "t1")
	if err != nil {
		t.Fatal(err)
	}
	s := web()
	s.Kind, s.PublicPort = KindTCP, int(c.ApiPort)+1
	var inv *InvalidError
	if _, err := Create(ctx, st.Q, s, now); !errors.As(err, &inv) || inv.Field != "public_port" {
		t.Fatalf("%v", err)
	}
	s.PublicPort = 15432
	if _, err := Create(ctx, st.Q, s, now); err != nil {
		t.Fatal(err)
	}
}
