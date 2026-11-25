package firewall

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"regexp"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/vault"
)

var now = time.Now()

type fakeConn struct{ *fakeHost }

func (fakeConn) Close() error { return nil }

// probeHost answers like the recorded NetBird host before Hashi's first apply, and like a live host afterwards.
func probeHost(t *testing.T) *fakeHost {
	t.Helper()
	probe := fixture(t, "probe_debian13_pve_netbird.txt")
	nbJSON := sections(probe)["netbird status --json"].out
	h := newFake()
	h.run = func(cmd string) sshx.Result {
		switch {
		case cmd == ProbeCommand:
			return sshx.Result{Stdout: []byte(probe)}
		case cmd == `echo "$SSH_CONNECTION"`:
			return sshx.Result{Stdout: []byte("10.32.0.6 50122 198.18.3.1 22\n")}
		case cmd == SetupRulesetCommand:
			return sshx.Result{Stdout: []byte("### nft -j list ruleset\n{\"nftables\": []}\n")}
		case cmd == "netbird status --json":
			return sshx.Result{Stdout: []byte(nbJSON)}
		case cmd == "nft list tables" && h.files[TableFile] == nil:
			return sshx.Result{}
		case strings.HasPrefix(cmd, "systemctl is-enabled") && h.files[TableFile] == nil:
			return sshx.Result{Stdout: []byte("disabled\n"), ExitCode: 1}
		}
		return liveHost(h, cmd)
	}
	return h
}

type env struct {
	st   *store.Store
	m    *Manager
	sets *ipsets.Service
	eng  *hsync.Engine
	host *fakeHost
}

func newEnv(t *testing.T) env {
	t.Helper()
	ctx := context.Background()
	st := storetest.New(t)
	v, err := vault.New(bytes.Repeat([]byte{7}, vault.KeySize), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Put(ctx, st.Q, settings.General{RootDomain: "example.com", AdminDomain: "hashi.example.com",
		InternalURL: "http://10.32.0.6:3340", SyncIntervalSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	priv, _, err := sshx.GenerateKey("hashi@test")
	if err != nil {
		t.Fatal(err)
	}
	rec, err := v.Seal(vault.Service, IdentityPurpose, priv)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertSecret(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertTraefikConnection(ctx, db.InsertTraefikConnectionParams{ID: "t1", Name: "edge", SshHost: "10.32.0.5",
		SshPort: 22, SshUser: "root", Auth: "hashi_key", HostKeyType: "ssh-ed25519", HostKey: []byte{1}, HostKeyFingerprint: "x"}); err != nil {
		t.Fatal(err)
	}
	al := audit.New([]byte("k"))
	sets := ipsets.NewService(st, al, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	host := probeHost(t)
	m := NewManager(st, v, al, sets, func(context.Context, sshx.Config) (Conn, error) { return fakeConn{host}, nil })
	return env{st: st, m: m, sets: sets, eng: hsync.New(st, al, m.Subsystems()...), host: host}
}

func hostKey(t *testing.T) []byte {
	t.Helper()
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	sp, _ := ssh.NewPublicKey(pub)
	return sp.Marshal()
}

func managedInput(t *testing.T) HostInput {
	return HostInput{Name: "edge-a", EdgeMode: ModeManaged, Host: "198.18.3.1", User: "root", HostKey: hostKey(t),
		TraefikConnectionID: "t1", InternalTraefikIP: "10.32.0.5", Subnets: []string{"10.32.0.0/24"},
		NetBirdMode: NetBirdRouting, NetBirdIface: "wt0", NetBirdPort: 51820}
}

// Invariant 7 for the firewall subsystem: a host is saved, validated (facts and the existing ruleset kept),
// confirmed, planned high-risk without touching the host, approved, applied under the rescue timer and verified;
// the next plan is empty. hashi-hosts gets the host and folds into allow.
func TestInvariant_7_FirewallChangesOnlyThroughApprovedPlans(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	h, err := e.m.Save(ctx, "admin", "", "", managedInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.eng.Propose(ctx, Subsystem, h.ID, hsync.TriggerManual, "admin"); !errors.Is(err, ErrNotReady) {
		t.Fatalf("unconfirmed host: %v", err)
	}
	f, err := e.m.Validate(ctx, h.ID)
	if err != nil {
		t.Fatal(err)
	}
	if f.NetBird == nil || f.SSHFrom.String() != "10.32.0.6" || f.WAN != "vmbr0" {
		t.Fatalf("facts %+v", f)
	}
	row, _ := e.st.R.GetFirewallHost(ctx, h.ID)
	if row.PublicIp != "198.18.3.1" || row.Health != "healthy" || !strings.Contains(row.SetupRuleset, "nft -j list ruleset") {
		t.Fatalf("stored %+v", row)
	}
	if err := e.m.ConfirmOwnership(ctx, "admin", "", h.ID); err != nil {
		t.Fatal(err)
	}
	if len(e.host.ran) != 3 || len(e.host.files) != 0 {
		t.Fatalf("validation touched the host: %v %v", e.host.ran, e.host.files)
	}

	p, err := e.eng.Propose(ctx, Subsystem, h.ID, hsync.TriggerManual, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if p.Risk != hsync.High || p.Status != hsync.StatusPending || len(e.host.files) != 0 {
		t.Fatalf("plan %+v, files %v", p, e.host.files)
	}
	if err := e.eng.Approve(ctx, p.ID, "admin", ""); err != nil {
		t.Fatal(err)
	}
	done, err := e.eng.Apply(ctx, p.ID, "admin", "")
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != hsync.StatusApplied || !strings.Contains(done.Result, "rescue timer disarmed") {
		t.Fatalf("applied %+v", done)
	}
	ran := strings.Join(e.host.ran, "\n")
	if !strings.Contains(ran, "systemd-run") || !strings.Contains(ran, "netbird status --json") {
		t.Fatalf("no rescue:\n%s", ran)
	}
	if again, err := e.eng.Propose(ctx, Subsystem, h.ID, hsync.TriggerManual, "admin"); err != nil || again != nil {
		t.Fatalf("second plan %+v %v", again, err)
	}

	// hashi-hosts holds the host (public scope) and its address is allowed in the table.
	hh, _ := e.st.R.GetIPSetBySystemKey(ctx, sql.NullString{String: ipsets.SystemHashiHosts, Valid: true})
	res, err := e.sets.Compile(ctx, e.st.R, hh.ID)
	if err != nil || !res.Contains(netip.MustParseAddr("198.18.3.1")) {
		t.Fatalf("hashi-hosts %+v %v", res, err)
	}
	if !strings.Contains(string(e.host.files[TableFile]), `"198.18.3.1"`) {
		t.Fatal("the host's own public address is not in allow4")
	}
}

func TestBindingsFoldAndLockOutGuard(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	h, err := e.m.Save(ctx, "admin", "", "", managedInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.m.Validate(ctx, h.ID); err != nil {
		t.Fatal(err)
	}
	_ = e.m.ConfirmOwnership(ctx, "admin", "", h.ID)
	p, _ := e.eng.Propose(ctx, Subsystem, h.ID, hsync.TriggerManual, "admin")
	_ = e.eng.Approve(ctx, p.ID, "admin", "")
	if _, err := e.eng.Apply(ctx, p.ID, "admin", ""); err != nil {
		t.Fatal(err)
	}

	ed := ipsets.Editor{}
	block, _ := ipsets.CreateSet(ctx, e.st.Q, ipsets.SetInput{Slug: "bad", Name: "bad"}, now)
	allow, _ := ipsets.CreateSet(ctx, e.st.Q, ipsets.SetInput{Slug: "me", Name: "me"}, now)
	for set, item := range map[string]string{block.ID: "203.0.113.0/24", allow.ID: "203.0.113.7"} {
		it, _ := ipsets.ParseIPOrCIDR(item)
		if _, err := ed.AddElement(ctx, e.st.Q, set, ipsets.ElementInput{Item: it}, "t", now); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.m.PutBindings(ctx, "admin", "", Bindings{Allow: []string{allow.ID}, Block: []string{block.ID}}); err != nil {
		t.Fatal(err)
	}
	cons, _ := ipsets.Consumers(ctx, e.st.R, block.ID, e.sets.Sources())
	if len(cons) != 1 || cons[0].Name != "Global firewall block" {
		t.Fatalf("consumers %+v", cons)
	}
	if _, err := ipsets.DeleteSet(ctx, e.st.Q, block.ID, e.sets.Sources()); !errors.Is(err, ipsets.ErrInUse) {
		t.Fatalf("delete a bound set: %v", err)
	}
	plans, err := e.m.PlanAll(ctx, e.eng, "admin")
	if err != nil || len(plans) != 1 || plans[0].Status != hsync.StatusApplied {
		t.Fatalf("element-only change should apply: %+v %v", plans, err)
	}
	table := string(e.host.files[TableFile])
	if !strings.Contains(table, `"203.0.113.7"`) || !regexp.MustCompile(`"addr": "203\.0\.113\.0",\s+"len": 30`).MatchString(table) {
		t.Fatalf("folded sets not rendered: has .7 %v, has /30 %v", strings.Contains(table, `"203.0.113.7"`), strings.Contains(table, `"len": 30`))
	}

	// Removing the address the host sees Hashi from (rescue path) from host access is high risk.
	rescueSet, _ := ipsets.CreateSet(ctx, e.st.Q, ipsets.SetInput{Slug: "hashi", Name: "hashi"}, now)
	it, _ := ipsets.ParseIPOrCIDR("10.32.0.6")
	_, _ = ed.AddElement(ctx, e.st.Q, rescueSet.ID, ipsets.ElementInput{Item: it}, "t", now)
	_ = e.m.PutHostAccess(ctx, "admin", "", h.ID, []string{rescueSet.ID})
	plans, _ = e.m.PlanAll(ctx, e.eng, "admin")
	if len(plans) != 1 || plans[0].Status != hsync.StatusApplied {
		t.Fatalf("adding host access: %+v", plans)
	}
	_ = e.m.PutHostAccess(ctx, "admin", "", h.ID, nil)
	plans, _ = e.m.PlanAll(ctx, e.eng, "admin")
	if len(plans) != 1 || plans[0].Risk != hsync.High || plans[0].Status != hsync.StatusPending ||
		!strings.Contains(plans[0].Items[0].Note, "10.32.0.6 loses its access (host_access4)") {
		t.Fatalf("removing the rescue address: %+v", plans)
	}
}

func TestSaveRefusesBadHosts(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	bad := map[string]func(*HostInput){
		"reserved name":        func(in *HostInput) { in.Name = "via" },
		"bad name":             func(in *HostInput) { in.Name = "Edge A" },
		"no subnet":            func(in *HostInput) { in.Subnets = nil },
		"overlapping subnets":  func(in *HostInput) { in.Subnets = []string{"10.32.0.0/24", "10.32.0.128/25"} },
		"Traefik outside":      func(in *HostInput) { in.InternalTraefikIP = "10.99.0.5" },
		"no host key":          func(in *HostInput) { in.HostKey = nil },
		"NetBird without port": func(in *HostInput) { in.NetBirdPort = 0 },
		"password missing":     func(in *HostInput) { in.Auth = "password" },
	}
	for name, mut := range bad {
		in := managedInput(t)
		mut(&in)
		if _, err := e.m.Save(ctx, "admin", "", "", in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// An external_nat host needs no SSH: it is a name and addresses for DNS and hashi-hosts.
	home, err := e.m.Save(ctx, "admin", "", "", HostInput{Name: "edge-b", EdgeMode: ModeExternalNAT, PublicIPOverride: "198.51.100.44"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.m.Validate(ctx, home.ID); !errors.Is(err, ErrInvalid) {
		t.Fatalf("validate external_nat: %v", err)
	}
	addrs, err := e.m.hostAddrs(ctx, home.ID, ipsets.ScopePublic)
	if err != nil || len(addrs) != 1 || addrs[0].String() != "198.51.100.44" {
		t.Fatalf("external host addresses %v %v", addrs, err)
	}
}

// Bug 23: a host behind a router takes its public and internal addresses from a Pulse agent and follows its changes;
// a managed host and an unknown agent are refused; a hand-set address still wins.
func TestHostFollowsAgent(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	if err := e.st.Q.InsertPulseAgent(ctx, db.InsertPulseAgentParams{ID: "A1", Name: "home", TokenHash: []byte("x"), InstallType: "linux", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	beat := func(public string) {
		if err := e.st.Q.SetPulseHeartbeat(ctx, db.SetPulseHeartbeatParams{PublicIp: public, PublicIpSource: "connection",
			CandidatesJson: `[{"interface":"eth0","address":"10.77.0.5"},{"interface":"wg0","address":"100.64.0.9"}]`, SelectedIp: "100.64.0.9",
			IntervalSeconds: 60, InstallType: "linux", ID: "A1"}); err != nil {
			t.Fatal(err)
		}
	}
	beat("198.51.100.7")
	in := managedInput(t)
	in.AddressAgentID = "A1"
	if _, err := e.m.Save(ctx, "admin", "", "", in); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a managed host following an agent: %v", err)
	}
	if _, err := e.m.Save(ctx, "admin", "", "", HostInput{Name: "home", EdgeMode: ModeExternalNAT, AddressAgentID: "nope"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown agent: %v", err)
	}
	home, err := e.m.Save(ctx, "admin", "", "", HostInput{Name: "home", EdgeMode: ModeExternalNAT, AddressAgentID: "A1"})
	if err != nil {
		t.Fatal(err)
	}
	addrs := func(scope string) string {
		got, err := e.m.hostAddrs(ctx, home.ID, scope)
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprint(got)
	}
	if got := addrs(ipsets.ScopePublic); got != "[198.51.100.7]" {
		t.Fatalf("public from the agent: %s", got)
	}
	if got := addrs(ipsets.ScopeInternal); got != "[100.64.0.9 10.77.0.5]" {
		t.Fatalf("internal from the agent, main address first: %s", got)
	}
	beat("198.51.100.8")
	if err := e.m.FollowAgents(ctx); err != nil {
		t.Fatal(err)
	}
	if got := addrs(ipsets.ScopePublic); got != "[198.51.100.8]" {
		t.Fatalf("after the agent moved: %s", got)
	}
	if _, err := e.m.Save(ctx, "admin", "", home.ID, HostInput{Name: "home", EdgeMode: ModeExternalNAT, AddressAgentID: "A1", PublicIPOverride: "203.0.113.1"}); err != nil {
		t.Fatal(err)
	}
	if got := addrs(ipsets.ScopePublic); got != "[203.0.113.1]" {
		t.Fatalf("a hand-set address wins: %s", got)
	}
	if _, err := e.m.Save(ctx, "admin", "", home.ID, HostInput{Name: "home", EdgeMode: ModeExternalNAT, PublicIPOverride: "203.0.113.1"}); err != nil {
		t.Fatal(err)
	}
	if got, err := e.m.hostAddrs(ctx, home.ID, ipsets.ScopeInternal); err == nil {
		t.Fatalf("the agent's addresses stayed after it was let go: %v", got)
	}
}

// Bug 58: the address lookup gets each managed host's verdict with the deciding prefix and set, and whether host
// access lets the address reach the host itself; a host behind a router has no verdict.
func TestExplainAddress(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	h, err := e.m.Save(ctx, "admin", "", "", managedInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.m.Save(ctx, "admin", "", "", HostInput{Name: "edge-b", EdgeMode: ModeExternalNAT, PublicIPOverride: "198.51.100.44"}); err != nil {
		t.Fatal(err)
	}
	ed := ipsets.Editor{}
	block, _ := ipsets.CreateSet(ctx, e.st.Q, ipsets.SetInput{Slug: "bad", Name: "bad"}, now)
	allow, _ := ipsets.CreateSet(ctx, e.st.Q, ipsets.SetInput{Slug: "me", Name: "me"}, now)
	for set, item := range map[string]string{block.ID: "203.0.113.0/24", allow.ID: "203.0.113.7"} {
		it, _ := ipsets.ParseIPOrCIDR(item)
		if _, err := ed.AddElement(ctx, e.st.Q, set, ipsets.ElementInput{Item: it}, "t", now); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.m.PutBindings(ctx, "admin", "", Bindings{Allow: []string{allow.ID}, Block: []string{block.ID}}); err != nil {
		t.Fatal(err)
	}
	if err := e.m.PutHostAccess(ctx, "admin", "", h.ID, []string{allow.ID}); err != nil {
		t.Fatal(err)
	}
	for addr, want := range map[string]string{
		"203.0.113.7": "allowed 203.0.113.7/32 me true",
		"203.0.113.8": "blocked 203.0.113.0/24 bad false",
		"192.0.2.1":   "neutral invalid Prefix  false",
	} {
		vs, err := e.m.Explain(ctx, netip.MustParseAddr(addr))
		if err != nil || len(vs) != 1 || vs[0].HostID != h.ID {
			t.Fatalf("%s: %+v %v", addr, vs, err)
		}
		if got := fmt.Sprintf("%s %s %s %v", vs[0].Verdict, vs[0].Prefix, vs[0].SetName, vs[0].HostAccess); got != want {
			t.Errorf("%s: %s, want %s", addr, got, want)
		}
	}
}

// Bug 24: a host's SSH address or key can change from Connections. The host is then validated and confirmed again
// before any plan; a new key (another machine) also reads its ruleset again, a new address alone keeps it.
func TestHostMoves(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	in := managedInput(t)
	h, err := e.m.Save(ctx, "admin", "", "", in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.m.Validate(ctx, h.ID); err != nil {
		t.Fatal(err)
	}
	if err := e.m.ConfirmOwnership(ctx, "admin", "", h.ID); err != nil {
		t.Fatal(err)
	}
	in.Host = "198.18.3.2"
	moved, err := e.m.Save(ctx, "admin", "", h.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if moved.SshHost != "198.18.3.2" || moved.OwnershipConfirmedAt.Valid || moved.LastValidatedAt.Valid || moved.SetupRuleset == "" {
		t.Fatalf("new address: %+v", moved)
	}
	if _, err := e.eng.Propose(ctx, Subsystem, h.ID, hsync.TriggerManual, "admin"); err == nil {
		t.Fatal("a moved host was planned before it was confirmed again")
	}
	in.HostKey = hostKey(t)
	other, err := e.m.Save(ctx, "admin", "", h.ID, in)
	if err != nil || other.SetupRuleset != "" {
		t.Fatalf("new key keeps the old machine's ruleset: %q %v", other.SetupRuleset, err)
	}
}

// The rescue check catches a cut path to NetBird's management server: the daemon still says connected.
func TestCheckManagement(t *testing.T) {
	status := "Daemon version: 0.81.3\nManagement: Connected to https://grid.example.com:443\nSignal: Connected to https://grid.example.com:443\n"
	for _, tc := range []struct {
		status string
		exit   int
		bad    bool
	}{{status, 0, false}, {status, 1, true}, {"Management: Disconnected\n", 1, false}} {
		c := mgmtConn{status: tc.status, exit: tc.exit}
		if err := checkManagement(t.Context(), &c); (err != nil) != tc.bad {
			t.Errorf("%+v: %v", tc, err)
		}
	}
}

type mgmtConn struct {
	Conn
	status string
	exit   int
}

func (m *mgmtConn) Run(_ context.Context, cmd string, _ io.Reader) (sshx.Result, error) {
	if cmd == "netbird status -d" {
		return sshx.Result{Stdout: []byte(m.status)}, nil
	}
	return sshx.Result{ExitCode: m.exit}, nil
}

// Bug 198: Hashi's SSH connection to a host follows a Pulse agent directly, through the agent's hidden set; going back
// to the literal address lets the set go.
func TestSSHTargetFollowsAgent(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	if err := e.st.Q.InsertPulseAgent(ctx, db.InsertPulseAgentParams{ID: "A1", Name: "edge", TokenHash: []byte("x"), InstallType: "linux", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	h, err := e.m.Save(ctx, "admin", "", "", managedInput(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.m.SetSSHTarget(ctx, h.ID, "", "nope", true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown agent: %v", err)
	}
	if err := e.m.SetSSHTarget(ctx, h.ID, "some-set", "A1", true); !errors.Is(err, ErrInvalid) {
		t.Fatalf("agent and set together: %v", err)
	}
	if _, err := e.st.Q.GetIPSet(ctx, ipsets.AgentSetID("A1")); err == nil {
		t.Fatal("a refused save left the agent's set behind")
	}
	if err := e.m.SetSSHTarget(ctx, h.ID, "", "A1", true); err != nil {
		t.Fatal(err)
	}
	row, err := e.st.R.GetFirewallHost(ctx, h.ID)
	if err != nil || row.SshTargetSetID.String != ipsets.AgentSetID("A1") {
		t.Fatalf("ssh target %q %v", row.SshTargetSetID.String, err)
	}
	if err := e.m.SetSSHTarget(ctx, h.ID, "", "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.Q.GetIPSet(ctx, ipsets.AgentSetID("A1")); err == nil {
		t.Fatal("the unused hidden set stayed")
	}
}
