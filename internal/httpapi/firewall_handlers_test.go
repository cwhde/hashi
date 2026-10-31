package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/auth"
	"git.juzo.io/juzo/hashi/internal/firewall"
	"git.juzo.io/juzo/hashi/internal/firewall/firewalltest"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/setup"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// Setup step 6 over the API (02 §1, 05 §2): scan, save + validate (read-only), confirm (step done), plan the first
// apply (high risk, nothing written), run it under the rescue timer; then the admin endpoints read the rendered
// table, change the bindings (an element change applies) and refuse a reserved name.
func TestSetupStep6FirewallAPI(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	al := audit.New([]byte("k"))
	v, err := vault.New(bytes.Repeat([]byte{7}, vault.KeySize), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Put(ctx, st.Q, settings.General{RootDomain: "example.com", AdminDomain: "hashi.example.com",
		InternalURL: "http://10.32.0.6:3340", SyncIntervalSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	priv, _, _ := sshx.GenerateKey("hashi@test")
	rec, _ := v.Seal(vault.Service, firewall.IdentityPurpose, priv)
	if err := st.Q.InsertSecret(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertTraefikConnection(ctx, db.InsertTraefikConnectionParams{ID: "t1", Name: "edge", SshHost: "10.32.0.5",
		SshPort: 22, SshUser: "root", Auth: "hashi_key", HostKeyType: "ssh-ed25519", HostKey: []byte{1}, HostKeyFingerprint: "x"}); err != nil {
		t.Fatal(err)
	}
	probe, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "firewall", "probe_debian13_pve_netbird.txt"))
	if err != nil {
		t.Fatal(err)
	}
	host, err := firewalltest.NewHost(string(probe))
	if err != nil {
		t.Fatal(err)
	}
	sets := ipsets.NewService(st, al, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	fw := firewall.NewManager(st, v, al, sets, host.Connect)
	fw.UseFakes(host.Connect, host.Scan)
	s := &Server{Store: st, Audit: al, Vault: v, Sessions: auth.NewManager(st, al, []byte("csrf")), Setup: setup.New(st, v, al),
		IPSets: sets, Firewall: fw, Sync: hsync.New(st, al, fw.Subsystems()...), Log: slog.Default()}

	scan, err := s.ScanSetupFirewallHost(ctx, gen.ScanSetupFirewallHostRequestObject{Body: &gen.HostKeyScanInput{Host: "198.18.3.1", Port: 22}})
	if err != nil {
		t.Fatal(err)
	}
	key := scan.(gen.ScanSetupFirewallHost200JSONResponse).Key
	mode, nb := gen.FirewallHostInputNetbirdModeRouting, 51820
	in := gen.FirewallHostInput{Name: "edge-a", EdgeMode: gen.FirewallHostInputEdgeModeManaged, Host: ptr("198.18.3.1"),
		User: ptr("root"), HostKey: &key, TraefikConnectionId: ptr("t1"), InternalTraefikIp: ptr("10.32.0.5"),
		Subnets: &[]string{"10.32.0.0/24"}, NetbirdMode: &mode, NetbirdIface: ptr("wt0"), NetbirdPort: &nb}

	if _, err := s.ConfirmSetupFirewall(ctx, gen.ConfirmSetupFirewallRequestObject{}); status(err) != http.StatusConflict {
		t.Fatalf("confirm before save: %v", err)
	}
	// A mistake can be undone before the host is confirmed; a changed login on a saved host is refused, not dropped.
	if _, err := s.PutSetupFirewallHost(ctx, gen.PutSetupFirewallHostRequestObject{Body: &in}); err != nil {
		t.Fatal(err)
	}
	wrong := in
	wrong.User = ptr("other")
	if _, err := s.PutSetupFirewallHost(ctx, gen.PutSetupFirewallHostRequestObject{Body: &wrong}); status(err) != http.StatusUnprocessableEntity {
		t.Fatalf("changed login: %v", err)
	}
	if _, err := s.DeleteSetupFirewallHost(ctx, gen.DeleteSetupFirewallHostRequestObject{}); err != nil {
		t.Fatal(err)
	}
	if rows, _ := st.R.ListFirewallHosts(ctx); len(rows) != 0 {
		t.Fatalf("start over left %d hosts", len(rows))
	}
	saved, err := s.PutSetupFirewallHost(ctx, gen.PutSetupFirewallHostRequestObject{Body: &in})
	if err != nil {
		t.Fatal(err)
	}
	h := gen.FirewallHost(saved.(gen.PutSetupFirewallHost200JSONResponse))
	if h.Health != gen.FirewallHostHealthHealthy || h.Facts == nil || h.Facts.Netbird == nil || h.Facts.Netbird.Iface != "wt0" ||
		h.PublicIp != "198.18.3.1" || h.WanIface != "vmbr0" || strings.Join(h.Facts.Candidates, ",") != "10.32.0.0/24" {
		t.Fatalf("validated host: %+v facts %+v", h, h.Facts)
	}
	if host.Loaded() != nil {
		t.Fatal("validation changed the host")
	}
	// Stream ports that belong to the host are refused (05 §8), from the captured listeners.
	for _, p := range []struct {
		proto string
		port  int
	}{{"tcp", 22}, {"tcp", 8006}, {"udp", 51820}} {
		if err := fw.CheckStreamPort(ctx, p.proto, p.port); !errors.Is(err, firewall.ErrInvalid) {
			t.Errorf("%d/%s: %v", p.port, p.proto, err)
		}
	}
	if err := fw.CheckStreamPort(ctx, "tcp", 15432); err != nil {
		t.Error(err)
	}
	// Setup can give the host's access list and the old script's list; saving twice replaces, and NetBird names are refused.
	for _, list := range [][]string{{"198.51.100.1"}, {"198.51.100.2", "10.0.0.0/8"}} {
		if _, err := s.PutSetupFirewallAccess(ctx, gen.PutSetupFirewallAccessRequestObject{Body: &gen.SetupFirewallAccess{Addresses: list}}); err != nil {
			t.Fatal(err)
		}
	}
	access, _ := fw.GetHostAccess(ctx, h.Id)
	if g, _ := ipsets.LoadGraph(ctx, st.R); len(access) != 1 || len(g[access[0]].Elements) != 2 {
		t.Fatalf("access %v", access)
	}
	if _, err := s.PutSetupFirewallAccess(ctx, gen.PutSetupFirewallAccessRequestObject{Body: &gen.SetupFirewallAccess{Addresses: []string{"nope"}}}); status(err) != http.StatusUnprocessableEntity {
		t.Fatalf("bad address: %v", err)
	}
	if _, err := s.PutSetupFirewallTakeover(ctx, gen.PutSetupFirewallTakeoverRequestObject{Body: &gen.FirewallTakeover{Chains: []string{"NETBIRD-X"}, Rules: []string{}, Ipsets: []string{}, CronMatch: []string{}, CronFiles: []string{}}}); status(err) != http.StatusUnprocessableEntity {
		t.Fatalf("netbird chain: %v", err)
	}
	state, err := s.ConfirmSetupFirewall(ctx, gen.ConfirmSetupFirewallRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	if st6 := state.(gen.ConfirmSetupFirewall200JSONResponse).Steps[5]; st6.Status != "done" {
		t.Fatalf("step 6: %+v", st6)
	}
	if _, err := s.DeleteSetupFirewallHost(ctx, gen.DeleteSetupFirewallHostRequestObject{}); status(err) != http.StatusConflict {
		t.Fatalf("start over after confirm: %v", err)
	}
	planned, err := s.PlanSetupFirewall(ctx, gen.PlanSetupFirewallRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	p := planned.(gen.PlanSetupFirewall200JSONResponse).Plan
	if p == nil || p.Risk != "high" || p.Status != "pending" || host.Loaded() != nil {
		t.Fatalf("first plan: %+v", p)
	}
	ran, err := s.RunSetupFirewallPlan(ctx, gen.RunSetupFirewallPlanRequestObject{Id: p.Id})
	if err != nil {
		t.Fatal(err)
	}
	if r := ran.(gen.RunSetupFirewallPlan200JSONResponse); r.Status != "applied" || !strings.Contains(r.Result, "rescue timer disarmed") {
		t.Fatalf("run: %+v", r)
	}
	if !strings.Contains(string(host.Loaded()), `"name": "hashi"`) {
		t.Fatal("no table loaded")
	}

	rendered, err := s.GetFirewallRendered(ctx, gen.GetFirewallRenderedRequestObject{Id: h.Id})
	if err != nil || rendered.(gen.GetFirewallRendered200JSONResponse).Allow == 0 {
		t.Fatalf("rendered %+v %v", rendered, err)
	}
	block, _ := ipsets.CreateSet(ctx, st.Q, ipsets.SetInput{Slug: "bad", Name: "bad"}, s.now())
	it, _ := ipsets.ParseIPOrCIDR("203.0.113.0/24")
	if _, err := (ipsets.Editor{}).AddElement(ctx, st.Q, block.ID, ipsets.ElementInput{Item: it}, "t", s.now()); err != nil {
		t.Fatal(err)
	}
	put, err := s.PutFirewallBindings(ctx, gen.PutFirewallBindingsRequestObject{Body: &gen.FirewallBindings{Allow: []string{}, Block: []string{block.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if plans := put.(gen.PutFirewallBindings200JSONResponse).Plans; len(plans) != 1 || plans[0].Status != "applied" {
		t.Fatalf("binding change plans: %+v", plans)
	}
	if !strings.Contains(string(host.Loaded()), `"203.0.113.0"`) {
		t.Fatal("the block binding is not in the loaded table")
	}
	// The block set is bound now: changing its elements needs a passkey confirmation (02 §2), like the binding itself.
	var more gen.IPItem
	_ = more.FromIPItemCIDR(gen.IPItemCIDR{Type: gen.Cidr, Prefix: "198.51.100.0/24"})
	if _, err := s.AddIPSetElement(ctx, gen.AddIPSetElementRequestObject{Id: block.ID, Body: &gen.IPSetElementInput{Item: more}}); status(err) != http.StatusForbidden {
		t.Fatalf("edit of a bound set without reauth: %v", err)
	}
	if req := gen.Requirements["PutFirewallBindings"]; !req.Reauth || !gen.Requirements["PutFirewallHostAccess"].Reauth {
		t.Fatal("putFirewallBindings must require reauth")
	}
	bad := in
	bad.Name = "ovr"
	if _, err := s.CreateFirewallHost(ctx, gen.CreateFirewallHostRequestObject{Body: &bad}); status(err) != http.StatusUnprocessableEntity {
		t.Fatalf("reserved name: %v", err)
	}
	list, _ := s.ListFirewallHosts(ctx, gen.ListFirewallHostsRequestObject{})
	if hosts := list.(gen.ListFirewallHosts200JSONResponse).Hosts; len(hosts) != 1 || !hosts[0].OwnershipConfirmed || hosts[0].LastAppliedAt == nil {
		t.Fatalf("list %+v", hosts)
	}
}
