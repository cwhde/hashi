package pulse

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/netip"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	"git.juzo.io/juzo/hashi/internal/vault"
)

func fixture(t *testing.T) (*Service, *time.Time) {
	t.Helper()
	st := storetest.New(t)
	v, err := vault.New([]byte("0123456789abcdef0123456789abcdef"), 1)
	if err != nil {
		t.Fatal(err)
	}
	s := New(st, v, nil)
	now := time.Now()
	s.now = func() time.Time { return now }
	return s, &now
}

func TestTokenLifecycle(t *testing.T) {
	s, now := fixture(t)
	ctx := t.Context()
	a, tok, err := s.Create(ctx, "box", "linux")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(a.TokenHash), tok.Reveal()) || len(tok.Reveal()) < 64 {
		t.Fatal("the token must be long and stored only as a hash")
	}
	beat := Beat{Version: "1", Candidates: []Candidate{{"eth0", "10.0.0.5"}}}
	if err := s.Heartbeat(ctx, a.ID, "wrong", beat, netip.Addr{}); !errors.Is(err, ErrToken) {
		t.Fatalf("wrong token: %v", err)
	}
	if err := s.Heartbeat(ctx, "nobody", tok.Reveal(), beat, netip.Addr{}); !errors.Is(err, ErrToken) {
		t.Fatalf("unknown agent: %v", err)
	}
	if err := s.Heartbeat(ctx, a.ID, tok.Reveal(), beat, netip.MustParseAddr("192.0.2.9")); err != nil {
		t.Fatal(err)
	}
	if err := s.Heartbeat(ctx, a.ID, tok.Reveal(), beat, netip.Addr{}); !errors.Is(err, ErrRate) {
		t.Fatalf("rate: %v", err)
	}
	*now = now.Add(time.Minute)
	_, tok2, err := s.Rotate(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Heartbeat(ctx, a.ID, tok.Reveal(), beat, netip.Addr{}); !errors.Is(err, ErrToken) {
		t.Fatal("the old token must stop working")
	}
	if err := s.Heartbeat(ctx, a.ID, tok2.Reveal(), beat, netip.Addr{}); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Minute)
	if _, err := s.Revoke(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Heartbeat(ctx, a.ID, tok2.Reveal(), beat, netip.Addr{}); !errors.Is(err, ErrToken) {
		t.Fatal("a revoked agent must be refused")
	}
	if _, err := s.Addrs(ctx, s.st.R, a.ID, ipsets.ScopeInternal, ""); err == nil {
		t.Fatal("a revoked agent resolves to nothing")
	}
}

func TestHeartbeatValidates(t *testing.T) {
	s, _ := fixture(t)
	ctx := t.Context()
	a, tok, err := s.Create(ctx, "box", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range []Beat{{Candidates: []Candidate{{"eth0", "not-an-address"}}}, {Candidates: make([]Candidate, 65)}, {Candidates: []Candidate{{"bad name;", "10.0.0.1"}}}} {
		if err := s.Heartbeat(ctx, a.ID, tok.Reveal(), b, netip.Addr{}); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: %v", b, err)
		}
	}
}

func TestAddressesPerScope(t *testing.T) {
	s, _ := fixture(t)
	ctx := t.Context()
	a, tok, err := s.Create(ctx, "box", "")
	if err != nil {
		t.Fatal(err)
	}
	changes := make(chan struct{}, 4)
	s.OnChange(func(context.Context) { changes <- struct{}{} })
	cands := []Candidate{{"wg0", "100.64.0.2"}, {"eth0", "10.0.0.5"}, {"eth1", "10.1.0.5"}, {"lo", "127.0.0.1"}}
	if err := s.Heartbeat(ctx, a.ID, tok.Reveal(), Beat{Candidates: cands}, netip.MustParseAddr("192.0.2.9")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-changes:
	case <-time.After(time.Second):
		t.Fatal("a new address must reach the sets")
	}
	str := func(scope, sel string) string {
		got, err := s.Addrs(ctx, s.st.R, a.ID, scope, sel)
		if err != nil {
			t.Fatal(err)
		}
		parts := []string{}
		for _, x := range got {
			parts = append(parts, x.String())
		}
		return strings.Join(parts, ",")
	}
	for _, c := range []struct{ scope, sel, want string }{
		{ipsets.ScopePublic, "", "192.0.2.9"},
		{ipsets.ScopeInternal, "", "10.0.0.5,100.64.0.2,10.1.0.5"},
		{ipsets.ScopeInternal, "eth1", "10.1.0.5"},
		{ipsets.ScopeInternal, "10.0.0.0/24", "10.0.0.5"},
		{ipsets.ScopeAll, "", "192.0.2.9,10.0.0.5,100.64.0.2,10.1.0.5"},
	} {
		if got := str(c.scope, c.sel); got != c.want {
			t.Errorf("%s/%s: %s", c.scope, c.sel, got)
		}
	}
}

func TestStatus(t *testing.T) {
	s, now := fixture(t)
	a, _, err := s.Create(t.Context(), "box", "")
	if err != nil {
		t.Fatal(err)
	}
	if Status(a, *now) != "waiting" {
		t.Fatal(Status(a, *now))
	}
	a.LastSeenAt = sql.NullInt64{Int64: now.UnixMilli(), Valid: true}
	if Status(a, *now) != "fresh" || Status(a, now.Add(4*time.Minute)) != "stale" {
		t.Fatal("freshness")
	}
}

func TestAgentDNS(t *testing.T) {
	s, _ := fixture(t)
	ctx := t.Context()
	a, tok, err := s.Create(ctx, "Media Box!", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Heartbeat(ctx, a.ID, tok.Reveal(), Beat{Candidates: []Candidate{{"eth0", "10.0.0.5"}}}, netip.Addr{}); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Rewrites(ctx); err != nil || len(got) != 0 {
		t.Fatalf("off by default: %+v %v", got, err)
	}
	if err := settings.Put(ctx, s.st.Q, settings.AgentDNS{Enabled: true, Domain: "hashi.home.arpa", IPMode: settings.AgentIPSelected}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Rewrites(ctx)
	if err != nil || len(got) != 1 || got[0].Domain != "media-box.hashi.home.arpa" || got[0].Answer != "10.0.0.5" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestDeleteRefusedWhileReferenced(t *testing.T) {
	s, _ := fixture(t)
	a, _, err := s.Create(t.Context(), "box", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(t.Context(), a.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(t.Context(), a.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

// A heartbeat with a new address changes every IP set holding the agent, nested ones too (08 §4): consumers hang off the compile.
func TestAddressChangeReachesNestedSets(t *testing.T) {
	s, now := fixture(t)
	ctx := t.Context()
	a, tok, err := s.Create(ctx, "box", "")
	if err != nil {
		t.Fatal(err)
	}
	sets := ipsets.NewService(s.st, audit.New([]byte("salt")), slog.Default(), nil)
	sets.SetPulse(func(ctx context.Context, id, scope, sel string) ([]netip.Addr, error) {
		return s.Addrs(ctx, s.st.R, id, scope, sel)
	},
		func(ctx context.Context, q *db.Queries, id string) error {
			_, err := q.GetPulseAgent(ctx, id)
			return err
		})
	ed := sets.Editor()
	mk := func(slug string) string {
		id := "set-" + slug
		if err := s.st.Q.InsertIPSet(ctx, db.InsertIPSetParams{ID: id, Slug: slug, Name: slug, Kind: ipsets.KindUser, Origin: "user"}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	inner, outer := mk("inner"), mk("outer")
	if _, err := ed.AddElement(ctx, s.st.Q, inner, ipsets.ElementInput{Item: ipsets.Pulse{AgentID: a.ID, Scope: ipsets.ScopeInternal}}, "t", *now); err != nil {
		t.Fatal(err)
	}
	if _, err := ed.AddElement(ctx, s.st.Q, outer, ipsets.ElementInput{Item: ipsets.SetRef{SetID: inner}}, "t", *now); err != nil {
		t.Fatal(err)
	}
	beat := func(addr string) {
		t.Helper()
		*now = now.Add(time.Minute)
		if err := s.Heartbeat(ctx, a.ID, tok.Reveal(), Beat{Candidates: []Candidate{{"eth0", addr}}}, netip.Addr{}); err != nil {
			t.Fatal(err)
		}
	}
	beat("10.0.0.5")
	first, err := sets.Compile(ctx, s.st.R, outer)
	if err != nil || !first.AddressSet || len(first.Addresses) != 1 || first.Addresses[0].String() != "10.0.0.5" {
		t.Fatalf("%+v %v", first, err)
	}
	if _, err := sets.Recompile(ctx); err != nil {
		t.Fatal(err)
	}
	beat("10.0.0.6")
	changed, err := sets.Recompile(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 2 {
		t.Fatalf("changed sets: %v", changed)
	}
	if err := s.Delete(ctx, a.ID); !errors.Is(err, ErrInUse) {
		t.Fatalf("delete while referenced: %v", err)
	}
}
