package adguard

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/vault"
)

func svc(t *testing.T, addrs ...netip.Addr) (*Service, *store.Store) {
	t.Helper()
	st := storetest.New(t)
	v, err := vault.New([]byte("0123456789abcdef0123456789abcdef"), 1)
	if err != nil {
		t.Fatal(err)
	}
	resolve := func(context.Context, string, string) (ipsets.Target, error) {
		return ipsets.Target{Addresses: addrs}, nil
	}
	if err := st.Q.InsertIPSet(t.Context(), db.InsertIPSetParams{ID: "plain", Slug: "plain", Name: "plain", Kind: "user", Origin: "user"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertIPSet(t.Context(), db.InsertIPSetParams{ID: "agent", Slug: "agent", Name: "agent", Kind: "user", Origin: "user"}); err != nil {
		t.Fatal(err)
	}
	ed := ipsets.Editor{Agents: func(context.Context, *db.Queries, string) error { return nil }}
	if _, err := ed.AddElement(t.Context(), st.Q, "agent", ipsets.ElementInput{Item: ipsets.Pulse{AgentID: "a", Scope: ipsets.ScopeInternal}}, "t", time.Now()); err != nil {
		t.Fatal(err)
	}
	return New(st, v, resolve, func(context.Context) ([]Desired, error) { return nil, nil }), st
}

// The password never goes to an address an agent reported unless the instance proves who it is: a stolen agent token
// must not be a way to collect it (08 §3).
func TestAgentAddressNeedsProvenIdentity(t *testing.T) {
	var asked int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { asked++; w.WriteHeader(http.StatusUnauthorized) }))
	defer srv.Close()
	host, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	s, _ := svc(t, netip.MustParseAddr(host))
	ctx := t.Context()
	var p int
	for _, c := range port {
		p = p*10 + int(c-'0')
	}
	bad := []Input{
		{Name: "a", TargetSetID: "agent", Port: p, Scheme: "http", Username: "u", Password: secrets.New("pw"), SaveInactive: true},                                // hashi:allow-secret-pattern
		{Name: "b", TargetSetID: "agent", Port: p, Scheme: "https", ServerName: "ns.example.com", Username: "u", Password: secrets.New("pw"), SaveInactive: true}, // hashi:allow-secret-pattern
		{Name: "c", TargetSetID: "agent", Port: p, Scheme: "https", TLSVerify: true, Username: "u", Password: secrets.New("pw"), SaveInactive: true},              // hashi:allow-secret-pattern
	}
	for _, in := range bad {
		if _, err := s.Create(ctx, in); !errors.Is(err, ErrAgentTarget) {
			t.Errorf("%+v: %v", in.Name, err)
		}
	}
	// A set that gains an agent later is stopped at connect time.
	c, err := s.Create(ctx, Input{Name: "ok", TargetSetID: "plain", Port: p, Scheme: "http", Username: "u", Password: secrets.New("pw"), SaveInactive: true}) // hashi:allow-secret-pattern
	if err != nil {
		t.Fatal(err)
	}
	c.TargetSetID = "agent"
	if _, _, err := s.dial(ctx, c); !errors.Is(err, ErrAgentTarget) {
		t.Fatalf("connect: %v", err)
	}
	if asked != 0 {
		t.Fatal("the instance was contacted")
	}
}

// A save that cannot connect stops unless the admin says to keep it (08 §5).
func TestUnreachableNeedsExplicitSave(t *testing.T) {
	s, _ := svc(t, netip.MustParseAddr("127.0.0.1"))
	ctx := t.Context()
	in := Input{Name: "x", TargetSetID: "plain", Port: 1, Scheme: "http"}
	if _, err := s.Create(ctx, in); !errors.Is(err, ErrUnreachable) {
		t.Fatalf("%v", err)
	}
	in.SaveInactive = true
	row, err := s.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if row.Mode != ModeReadOnly {
		t.Fatal(row.Mode)
	}
}

// 03 §9.
func TestDeletedAgentsRewriteNeedsNoApproval(t *testing.T) {
	items := diff(nil, []db.AdguardRewritesOwned{{Domain: "a.hashi.home.arpa", Answer: "10.0.0.5", Origin: "agent gone"}},
		[]Rewrite{{Domain: "a.hashi.home.arpa", Answer: "10.0.0.5"}}, func(o string) bool { return o == "agent gone" })
	if len(items) != 1 || items[0].Risk != "low" {
		t.Fatalf("%+v", items)
	}
}

// A disabled connection is neither contacted nor planned (03 §10).
func TestDisabledConnectionUntouched(t *testing.T) {
	var asked int
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { asked++ }))
	defer srv.Close()
	host, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	s, _ := svc(t, netip.MustParseAddr(host))
	s.wanted = func(context.Context) ([]Desired, error) {
		return []Desired{{Domain: "a.example.com", Answer: "192.0.2.1"}}, nil
	}
	ctx := t.Context()
	var p int
	for _, c := range port {
		p = p*10 + int(c-'0')
	}
	row, err := s.Create(ctx, Input{Name: "off", TargetSetID: "plain", Port: p, Scheme: "http", Mode: ModeDisabled, Username: "u", Password: secrets.New("pw")}) // hashi:allow-secret-pattern
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Test(ctx, row.ID); err != nil {
		t.Fatal(err)
	}
	v, err := s.Rewrites(ctx, row.ID)
	if err != nil || len(v.Planned) != 0 {
		t.Fatalf("%+v %v", v, err)
	}
	d, err := sub{s}.Plan(ctx, row.ID)
	if err != nil || len(d.Items) != 0 {
		t.Fatalf("%+v %v", d, err)
	}
	if asked != 0 {
		t.Fatalf("contacted %d times", asked)
	}
}

// A removal waiting for approval must not hold back address updates: low-risk changes go first (S8 gate: no manual step).
func TestLowRiskGoesFirst(t *testing.T) {
	items := lowFirst([]hsync.Item{{Action: hsync.ActionDelete, Object: "old", Risk: hsync.High}, {Action: hsync.ActionUpdate, Object: "a", Risk: hsync.Low}, {Action: hsync.ActionConflict, Object: "m", Risk: hsync.Low}})
	if len(items) != 2 || items[0].Object != "a" || items[1].Object != "m" {
		t.Fatalf("%+v", items)
	}
	only := lowFirst([]hsync.Item{{Action: hsync.ActionDelete, Object: "old", Risk: hsync.High}, {Action: hsync.ActionConflict, Object: "m", Risk: hsync.Low}})
	if len(only) != 2 {
		t.Fatalf("with nothing else to do the removal is planned: %+v", only)
	}
}

// The rule protects a password: a connection without one may follow an agent over plain HTTP (08 §5, 03 §8).
func TestAgentTargetWithoutPassword(t *testing.T) {
	s, _ := svc(t, netip.MustParseAddr("127.0.0.1"))
	if _, err := s.Create(t.Context(), Input{Name: "open", TargetSetID: "agent", Port: 3000, Scheme: "http", SaveInactive: true}); err != nil {
		t.Fatal(err)
	}
}

// A rewrite taken over by a new owner carries the new origin, so the old owner's deletion rule no longer applies to it.
func TestRewriteFollowsItsNewOwner(t *testing.T) {
	owned := []db.AdguardRewritesOwned{{Domain: "a.home.arpa", Answer: "10.0.0.5", Origin: "agent old"}}
	remote := []Rewrite{{Domain: "a.home.arpa", Answer: "10.0.0.5"}}
	items := Diff([]Desired{{Domain: "a.home.arpa", Answer: "10.0.0.5", Origin: "agent new"}}, owned, remote)
	if len(items) != 1 || items[0].Action != hsync.ActionLabel {
		t.Fatalf("%+v", items)
	}
}
