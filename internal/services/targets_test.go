package services

import (
	"context"
	"errors"
	"fmt"
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

// A stream needs no name, and several may go without one; an HTTP service still needs its name.
func TestStreamWithoutName(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	for i, port := range []int{222, 2222} {
		s := web()
		s.Slug, s.Kind, s.HostName, s.DomainMode, s.PublicPort = fmt.Sprintf("s%d", i), KindTCP, "", ModeNone, port
		if _, err := Create(ctx, st.Q, s, now); err != nil {
			t.Fatal(err)
		}
	}
	s := web()
	s.HostName, s.DomainMode = "", ModeNone
	if _, err := Create(ctx, st.Q, s, now); err == nil {
		t.Fatal("an HTTP service without a name")
	}
}

// Bug 17: a service targets a Pulse agent directly. It is stored as the agent's hidden set and read back as the
// agent; the hidden set goes once nothing uses it; routes and an unknown agent are refused.
func TestAgentTarget(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	if err := st.Q.InsertPulseAgent(ctx, db.InsertPulseAgentParams{ID: "A1", Name: "nas", TokenHash: []byte("x"), InstallType: "linux", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	s := web()
	s.Target = Target{AgentID: "missing", Port: 80}
	var inv *InvalidError
	if _, err := Create(ctx, st.Q, s, now); !errors.As(err, &inv) || inv.Field != "target.agent_id" {
		t.Fatalf("unknown agent: %v", err)
	}
	s.Target = Target{AgentID: "A1", Address: "192.0.2.1", Port: 80}
	if _, err := Create(ctx, st.Q, s, now); !errors.As(err, &inv) {
		t.Fatalf("agent and address together: %v", err)
	}
	s.Target = Target{AgentID: "A1", Port: 80}
	created, err := Create(ctx, st.Q, s, now)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Get(ctx, st.Q, created.ID)
	if err != nil || got.Target.AgentID != "A1" || got.Target.SetID != ipsets.AgentSetID("A1") {
		t.Fatalf("read back %+v %v", got.Target, err)
	}
	// Saving what was read (both fields filled) is one target, not two.
	if _, err := Update(ctx, st.Q, got, now); err != nil {
		t.Fatalf("save as read: %v", err)
	}
	route := got
	route.AdvancedRoutes = true
	route.Routes = []Route{{Enabled: true, Targets: []Target{{AgentID: "A1", Port: 80}}, PathMatch: MatchPrefix, PathValue: "/x", Priority: 5, RewriteMode: RewriteNone}}
	if _, err := Update(ctx, st.Q, route, now); !errors.As(err, &inv) || !strings.HasPrefix(inv.Field, "routes[0]") {
		t.Fatalf("agent in a route: %v", err)
	}
	got.Target = Target{Address: "192.0.2.5", Port: 80}
	if _, err := Update(ctx, st.Q, got, now); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Q.GetIPSet(ctx, ipsets.AgentSetID("A1")); err == nil {
		t.Fatal("the unused hidden set stayed")
	}
}
