package app

import (
	"context"
	"log/slog"
	"net/netip"
	"slices"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/firewall"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/pulse"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// 08 §7, 05a §10, 03 §10: one real heartbeat, through the real set, into what each consumer reads. The service upstream and
// the connection target follow, and the generated record is a CNAME while the agent sits behind the host and an A record
// once it does not (Pulse IP matching). When the agent is revoked the target keeps its last address (invariant 21).
func TestHeartbeatReachesConsumers(t *testing.T) {
	ctx := t.Context()
	st := storetest.New(t)
	v, err := vault.New([]byte("0123456789abcdef0123456789abcdef"), 1)
	if err != nil {
		t.Fatal(err)
	}
	al := audit.New([]byte("salt"))
	sets := ipsets.NewService(st, al, slog.New(slog.DiscardHandler), nil)
	p := pulse.New(st, v, al)
	sets.SetPulse(func(ctx context.Context, id, scope, sel string) ([]netip.Addr, error) {
		return p.Addrs(ctx, st.R, id, scope, sel)
	},
		func(ctx context.Context, q *db.Queries, id string) error {
			_, err := q.GetPulseAgent(ctx, id)
			return err
		})
	var changed []string
	sets.Subscribe(func(_ context.Context, c []string) { changed = append(changed, c...) })

	agent, tok, err := p.Create(ctx, "box", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertIPSet(ctx, db.InsertIPSetParams{ID: "set", Slug: "box", Name: "box", Kind: ipsets.KindUser, Origin: "user"}); err != nil {
		t.Fatal(err)
	}
	if _, err := sets.Editor().AddElement(ctx, st.Q, "set", ipsets.ElementInput{Item: ipsets.Pulse{AgentID: agent.ID, Scope: ipsets.ScopeInternal}}, "t", time.Now()); err != nil {
		t.Fatal(err)
	}
	beat := func(addr string) {
		t.Helper()
		time.Sleep(2100 * time.Millisecond) // the heartbeat rate limit
		if _, err := p.Heartbeat(ctx, agent.ID, tok.Reveal(), pulse.Beat{Candidates: []pulse.Candidate{{Interface: "eth0", Address: addr}}}, netip.Addr{}); err != nil {
			t.Fatal(err)
		}
		if _, err := sets.Recompile(ctx); err != nil {
			t.Fatal(err)
		}
	}
	svc := services.Service{ID: "s", Name: "s", Slug: "s", Enabled: true, HostName: "app.example.com", Kind: services.KindHTTP, AdGuardRewrite: true,
		Target: services.Target{SetID: "set", Port: 80}}
	resolved := func() services.Service {
		t.Helper()
		got, err := services.Resolve(ctx, []services.Service{svc}, sets.ResolveTarget)
		if err != nil {
			t.Fatal(err)
		}
		return got[0]
	}
	home := firewall.Effective{Row: db.FirewallHost{ID: "h", Name: "a", InternalTraefikIp: "10.20.0.2"}, PublicIP: netip.MustParseAddr("192.0.2.1"), Subnets: []netip.Prefix{netip.MustParsePrefix("10.20.0.0/24")}}

	beat("10.20.0.9")
	if !slices.Contains(changed, "set") {
		t.Fatalf("changed %v", changed)
	}
	r := resolved()
	if len(r.Targets) != 1 || r.Targets[0].Address != "10.20.0.9" {
		t.Fatalf("upstream: %+v", r.Targets)
	}
	gen := firewall.GenerateRecords("example.com", []firewall.Effective{home}, []services.Service{r})
	if !slices.ContainsFunc(gen, func(g dns.Generated) bool { return g.Name == "app.example.com." && g.Type == "CNAME" }) {
		t.Fatalf("behind the host: %+v", gen)
	}
	if w := firewall.DesiredRewrites("example.com", []firewall.Effective{home}, []services.Service{r}); len(w) != 1 || w[0].Answer != "10.20.0.2" {
		t.Fatalf("rewrites: %+v", w)
	}

	beat("10.77.0.9")
	r = resolved()
	if r.Targets[0].Address != "10.77.0.9" {
		t.Fatalf("upstream after a change: %+v", r.Targets)
	}
	if w := firewall.DesiredRewrites("example.com", []firewall.Effective{home}, []services.Service{r}); len(w) != 0 {
		t.Fatalf("no longer behind the host: %+v", w)
	}

	// The connection target reads the same set: AdGuard reconnects to the new address through it.
	tgt, err := sets.ResolveTarget(ctx, "adguard:c", "set")
	if err != nil || len(tgt.Addresses) != 1 || tgt.Addresses[0].String() != "10.77.0.9" {
		t.Fatalf("connection: %+v %v", tgt, err)
	}
	if _, err := p.Revoke(ctx, agent.ID); err != nil {
		t.Fatal(err)
	}
	tgt, err = sets.ResolveTarget(ctx, "adguard:c", "set")
	if err != nil || len(tgt.Addresses) != 1 || tgt.Addresses[0].String() != "10.77.0.9" || tgt.Unresolved == "" {
		t.Fatalf("invariant 21: %+v %v", tgt, err)
	}
}

// 08 §5: an agent's consumers include what follows it through a nested set, and the address page names it (05a §6).
func TestAgentConsumersThroughNesting(t *testing.T) {
	ctx := t.Context()
	st := storetest.New(t)
	v, err := vault.New([]byte("0123456789abcdef0123456789abcdef"), 1)
	if err != nil {
		t.Fatal(err)
	}
	al := audit.New([]byte("salt"))
	sets := ipsets.NewService(st, al, slog.New(slog.DiscardHandler), nil)
	p := pulse.New(st, v, al)
	sets.SetPulse(func(ctx context.Context, id, scope, sel string) ([]netip.Addr, error) {
		return p.Addrs(ctx, st.R, id, scope, sel)
	},
		func(ctx context.Context, q *db.Queries, id string) error {
			_, err := q.GetPulseAgent(ctx, id)
			return err
		})
	sets.AddConsumers(func(_ context.Context, _ *db.Queries, id string) ([]ipsets.Consumer, error) {
		if id == "outer" {
			return []ipsets.Consumer{{Type: "adguard", ID: "c", Name: "AdGuard home", Position: ipsets.PositionTarget}}, nil
		}
		return nil, nil
	})
	a, _, err := p.Create(ctx, "phone", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"inner", "outer"} {
		if err := st.Q.InsertIPSet(ctx, db.InsertIPSetParams{ID: id, Slug: id, Name: id, Kind: ipsets.KindUser, Origin: "user"}); err != nil {
			t.Fatal(err)
		}
	}
	ed := sets.Editor()
	if _, err := ed.AddElement(ctx, st.Q, "inner", ipsets.ElementInput{Item: ipsets.Pulse{AgentID: a.ID, Scope: ipsets.ScopePublic}}, "t", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := ed.AddElement(ctx, st.Q, "outer", ipsets.ElementInput{Item: ipsets.SetRef{SetID: "inner"}}, "t", time.Now()); err != nil {
		t.Fatal(err)
	}
	names, consumers := sets.Impact(ctx, []string{"inner"})
	if !slices.Contains(names, "outer") || !slices.Contains(consumers, "AdGuard home") {
		t.Fatalf("%v %v", names, consumers)
	}
	g, err := ipsets.LoadGraph(ctx, st.R)
	if err != nil {
		t.Fatal(err)
	}
	if got := ipsets.Describe(g, g["inner"].Elements[0].Item); got != "phone (pulse, public)" {
		t.Fatal(got)
	}
}
