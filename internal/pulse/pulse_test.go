package pulse

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"os"
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
	s.debounce = time.Millisecond
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
	if _, err := s.Heartbeat(ctx, a.ID, "wrong", beat, netip.Addr{}); !errors.Is(err, ErrToken) {
		t.Fatalf("wrong token: %v", err)
	}
	if _, err := s.Heartbeat(ctx, "nobody", tok.Reveal(), beat, netip.Addr{}); !errors.Is(err, ErrToken) {
		t.Fatalf("unknown agent: %v", err)
	}
	if _, err := s.Heartbeat(ctx, a.ID, tok.Reveal(), beat, netip.MustParseAddr("192.0.2.9")); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Heartbeat(ctx, a.ID, tok.Reveal(), beat, netip.Addr{}); !errors.Is(err, ErrRate) {
		t.Fatalf("rate: %v", err)
	}
	*now = now.Add(time.Minute)
	_, tok2, err := s.Rotate(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Heartbeat(ctx, a.ID, tok.Reveal(), beat, netip.Addr{}); !errors.Is(err, ErrToken) {
		t.Fatal("the old token must stop working")
	}
	if _, err := s.Heartbeat(ctx, a.ID, tok2.Reveal(), beat, netip.Addr{}); err != nil {
		t.Fatal(err)
	}
	*now = now.Add(time.Minute)
	if _, err := s.Revoke(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Heartbeat(ctx, a.ID, tok2.Reveal(), beat, netip.Addr{}); !errors.Is(err, ErrToken) {
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
		if _, err := s.Heartbeat(ctx, a.ID, tok.Reveal(), b, netip.Addr{}); !errors.Is(err, ErrInvalid) {
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
	cands := []Candidate{{"wg0", "100.64.0.2"}, {"eth0", "10.0.0.5"}, {"eth1", "10.1.0.5"}, {"lo", "127.0.0.1"}, {"wan", "8.8.8.8"}}
	if _, err := s.Heartbeat(ctx, a.ID, tok.Reveal(), Beat{Candidates: cands}, netip.MustParseAddr("192.0.2.9")); err != nil {
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
	if Status(a, *now, 3) != "waiting" {
		t.Fatal(Status(a, *now, 3))
	}
	a.LastSeenAt = sql.NullInt64{Int64: now.UnixMilli(), Valid: true}
	if Status(a, *now, 3) != "fresh" || Status(a, now.Add(4*time.Minute), 3) != "stale" {
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
	if _, err := s.Heartbeat(ctx, a.ID, tok.Reveal(), Beat{Candidates: []Candidate{{"eth0", "10.0.0.5"}}}, netip.Addr{}); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Rewrites(ctx); err != nil || len(got) != 0 {
		t.Fatalf("off by default: %+v %v", got, err)
	}
	if err := settings.Put(ctx, s.st.Q, settings.AgentDNS{Enabled: true, Domain: "hashi.home.arpa", IPMode: settings.AgentIPSelected, RemoveStaleAfterHours: 1}); err != nil {
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
		if _, err := s.Heartbeat(ctx, a.ID, tok.Reveal(), Beat{Candidates: []Candidate{{"eth0", addr}}}, netip.Addr{}); err != nil {
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

// A connection from a public address is proof; an agent's own word counts only if the admin trusts it for that.
func TestPublicAddressSources(t *testing.T) {
	s, now := fixture(t)
	ctx := t.Context()
	a, tok, err := s.Create(ctx, "home", "")
	if err != nil {
		t.Fatal(err)
	}
	beat := func(source, reported string) bool {
		t.Helper()
		*now = now.Add(time.Minute)
		var src netip.Addr
		if source != "" {
			src = netip.MustParseAddr(source)
		}
		r, err := s.Heartbeat(ctx, a.ID, tok.Reveal(), Beat{ReportedPublic: reported}, src)
		if err != nil {
			t.Fatal(err)
		}
		return r.CheckPublicIP
	}
	public := func() string {
		r, err := s.st.R.GetPulseAgent(ctx, a.ID)
		if err != nil {
			t.Fatal(err)
		}
		return r.PublicIp + "/" + r.PublicIpSource
	}
	if beat("10.0.0.1", "203.0.113.5") || public() != "/" {
		t.Fatalf("untrusted report used: %s", public())
	}
	if _, err := s.SetDNS(ctx, a.ID, "home", true, "", true, 0); err != nil {
		t.Fatal(err)
	}
	if !beat("10.0.0.1", "") {
		t.Fatal("a trusted agent behind a private path should be asked to check")
	}
	beat("10.0.0.1", "203.0.113.5")
	if public() != "203.0.113.5/reported" {
		t.Fatal(public())
	}
	for _, bad := range []string{"10.1.1.1", "100.64.0.9", "nonsense"} {
		beat("10.0.0.1", bad)
		if public() != "203.0.113.5/reported" {
			t.Fatalf("%s accepted: %s", bad, public())
		}
	}
	if beat("198.51.100.8", "203.0.113.5") || public() != "198.51.100.8/connection" {
		t.Fatalf("the connection must win and needs no check: %s", public())
	}
}

func TestCheckReach(t *testing.T) {
	s, now := fixture(t)
	ctx := t.Context()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port
	a, _, err := s.Create(ctx, "box", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.st.Q.SetPulseHeartbeat(ctx, db.SetPulseHeartbeatParams{CandidatesJson: "[]", SelectedIp: "127.0.0.1", IntervalSeconds: 60, InstallType: "linux", UpdatedAt: now.UnixMilli(), ID: a.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetDNS(ctx, a.ID, "box", true, "", false, port); err != nil {
		t.Fatal(err)
	}
	reach := func() sql.NullInt64 {
		t.Helper()
		if _, err := s.CheckReach(ctx); err != nil {
			t.Fatal(err)
		}
		r, err := s.st.R.GetPulseAgent(ctx, a.ID)
		if err != nil {
			t.Fatal(err)
		}
		return r.ReachOk
	}
	if r := reach(); !r.Valid || r.Int64 != 1 {
		t.Fatalf("open port: %+v", r)
	}
	_ = ln.Close()
	if r := reach(); !r.Valid || r.Int64 != 0 {
		t.Fatalf("closed port: %+v", r)
	}
}

// Review of S8.
func TestTrustFollowsTheCurrentConnection(t *testing.T) {
	s, now := fixture(t)
	ctx := t.Context()
	a, tok, err := s.Create(ctx, "home", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetDNS(ctx, a.ID, "home", true, "", true, 0); err != nil {
		t.Fatal(err)
	}
	beat := func(source, reported string) bool {
		t.Helper()
		*now = now.Add(time.Minute)
		r, err := s.Heartbeat(ctx, a.ID, tok.Reveal(), Beat{ReportedPublic: reported}, netip.MustParseAddr(source))
		if err != nil {
			t.Fatal(err)
		}
		return r.CheckPublicIP
	}
	// Moving from a public path to a private one must start the checks, or the old address stays frozen.
	if beat("198.51.100.7", "") {
		t.Fatal("a public connection needs no check")
	}
	if !beat("10.0.0.1", "") {
		t.Fatal("after moving to a private path the agent must be asked")
	}
	beat("10.0.0.1", "203.0.113.66")
	// Distrusting the agent removes what it said, and the sets follow.
	changes := make(chan struct{}, 1)
	s.OnChange(func(context.Context) { changes <- struct{}{} })
	if _, err := s.SetDNS(ctx, a.ID, "home", true, "", false, 0); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Addrs(ctx, s.st.R, a.ID, ipsets.ScopePublic, ""); err != nil || len(got) != 0 {
		t.Fatalf("distrusted address still resolves: %v %v", got, err)
	}
	select {
	case <-changes:
	case <-time.After(time.Second):
		t.Fatal("the sets must recompile")
	}
}

// An agent on a longer interval is not stale between its heartbeats.
func TestStaleFollowsTheAgentsInterval(t *testing.T) {
	s, now := fixture(t)
	ctx := t.Context()
	a, tok, err := s.Create(ctx, "slow", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Heartbeat(ctx, a.ID, tok.Reveal(), Beat{IntervalSeconds: 300}, netip.Addr{}); err != nil {
		t.Fatal(err)
	}
	r, err := s.st.R.GetPulseAgent(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := Status(r, now.Add(200*time.Second), 3); got != "fresh" {
		t.Fatal(got)
	}
	if got := Status(r, now.Add(901*time.Second), 3); got != "stale" {
		t.Fatal(got)
	}
}

// 03 §9, §10: normalization, collisions shown and not silently dropped, an override resolving one, private mode, stale removal.
func TestAgentNames(t *testing.T) {
	s, now := fixture(t)
	ctx := t.Context()
	if err := settings.Put(ctx, s.st.Q, settings.AgentDNS{Enabled: true, Domain: "hashi.home.arpa", IPMode: settings.AgentIPPrivate, RemoveStaleAfterHours: 2}); err != nil {
		t.Fatal(err)
	}
	mk := func(name, addr string) string {
		t.Helper()
		a, tok, err := s.Create(ctx, name, "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.Heartbeat(ctx, a.ID, tok.Reveal(), Beat{Candidates: []Candidate{{"wg0", "203.0.113.9"}, {"eth0", addr}}}, netip.Addr{}); err != nil {
			t.Fatal(err)
		}
		return a.ID
	}
	mk("Media Box!", "10.0.0.5")
	second := mk("media box", "10.0.0.6")
	names, err := s.Names(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var wanted, collided int
	for _, n := range names {
		if n.Wanted {
			wanted++
			if n.Domain != "media-box.hashi.home.arpa" || n.Answer != "10.0.0.5" {
				t.Errorf("%+v", n)
			}
		}
		if n.Collision {
			collided++
		}
	}
	if wanted != 1 || collided != 1 {
		t.Fatalf("%+v", names)
	}
	if _, err := s.SetDNS(ctx, second, "media box", true, "Second", false, 0); err != nil {
		t.Fatal(err)
	}
	got, err := s.Rewrites(ctx)
	if err != nil || len(got) != 2 {
		t.Fatalf("override: %+v %v", got, err)
	}
	*now = now.Add(time.Hour)
	if got, _ := s.Rewrites(ctx); len(got) != 2 {
		t.Fatalf("a stale agent keeps its rewrite: %+v", got)
	}
	*now = now.Add(2 * time.Hour)
	if got, _ := s.Rewrites(ctx); len(got) != 0 {
		t.Fatalf("removed after the configured time: %+v", got)
	}
	if !s.OriginGone(ctx, "agent nobody") || s.OriginGone(ctx, "agent "+second) || s.OriginGone(ctx, "service x") {
		t.Fatal("OriginGone")
	}
}

func TestPulseSettingsAreUsed(t *testing.T) {
	s, _ := fixture(t)
	ctx := t.Context()
	a, tok, err := s.Create(ctx, "box", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Put(ctx, s.st.Q, settings.Pulse{HeartbeatIntervalSeconds: 120, StaleFactor: 5}); err != nil {
		t.Fatal(err)
	}
	r, err := s.Heartbeat(ctx, a.ID, tok.Reveal(), Beat{}, netip.Addr{})
	if err != nil || r.IntervalSeconds != 120 {
		t.Fatalf("%+v %v", r, err)
	}
}

// Review of S8: a public address cannot enter sets through the private list without the trust opt-in, and a stale agent's
// set says so while its addresses keep counting (05a §2).
func TestPublicCandidateRefusedAndStaleSetWarns(t *testing.T) {
	s, now := fixture(t)
	ctx := t.Context()
	a, tok, err := s.Create(ctx, "box", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Heartbeat(ctx, a.ID, tok.Reveal(), Beat{Candidates: []Candidate{{"wan", "8.8.8.8"}, {"eth0", "10.0.0.5"}}}, netip.Addr{}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Addrs(ctx, s.st.R, a.ID, ipsets.ScopeInternal, "")
	if err != nil || len(got) != 1 || got[0].String() != "10.0.0.5" {
		t.Fatalf("%v %v", got, err)
	}
	*now = now.Add(time.Hour)
	got, err = s.Addrs(ctx, s.st.R, a.ID, ipsets.ScopeInternal, "")
	var stale ipsets.Stale
	if !errors.As(err, &stale) || len(got) != 1 {
		t.Fatalf("stale: %v %v", got, err)
	}
}

// The captured release holds what the installer downloads.
func TestReleaseContract(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/contracts/gitea/release.json")
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		Prerelease bool `json:"prerelease"`
		Assets     []struct {
			Name string `json:"name"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, a := range r.Assets {
		names[a.Name] = true
	}
	for _, arch := range []string{"amd64", "arm64"} {
		if !names["hashi-pulse-linux-"+arch] || !names["hashi-pulse-linux-"+arch+".sha256"] {
			t.Fatalf("%s missing: %v", arch, names)
		}
	}
	if !strings.Contains(string(InstallScript), `"$from/hashi-pulse-linux-$arch"`) || !strings.Contains(string(Script()), ReleaseBase) {
		t.Fatal("the installer must fetch those names from this release")
	}
}
