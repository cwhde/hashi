//go:build integration

package firewall

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/sshx"
)

// TestRealFirewallOnDevTarget is the dev-target half of the S6 gate on a disposable Debian machine with real systemd:
// first apply, boot unit restart, confirmed takeover, a real rescue-timer rollback, foreign rules untouched. It changes
// the machine: roll it back afterwards. HASHI_FW_TARGET=user@addr, HASHI_FW_KEY=<private key file>.
func TestRealFirewallOnDevTarget(t *testing.T) {
	target, keyFile := os.Getenv("HASHI_FW_TARGET"), os.Getenv("HASHI_FW_KEY")
	if target == "" || keyFile == "" {
		t.Skip("set HASHI_FW_TARGET and HASHI_FW_KEY")
	}
	user, addr, _ := strings.Cut(target, "@")
	key, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	hk, err := sshx.Scan(ctx, addr+":22", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	dial := func(ctx context.Context) (*sshx.Client, error) {
		return sshx.Dial(ctx, sshx.Config{Addr: addr + ":22", Pinned: hk, Auth: sshx.Auth{User: user, PrivateKey: secrets.New(string(key))}})
	}
	h, err := dial(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.Close() }()
	sh := func(cmd string) string {
		t.Helper()
		r, err := h.Run(ctx, cmd, nil)
		if err != nil || !r.OK() {
			t.Fatalf("%s: %v\n%s%s", cmd, err, r.Stdout, r.Stderr)
		}
		return string(r.Stdout)
	}
	sh(`mkdir -p /scripts
ipset create nb-1 hash:net -exist; ipset create trusted_hosts hash:ip -exist; ipset add trusted_hosts 192.0.2.2 -exist
iptables-legacy -N NETBIRD-ACL-INPUT; iptables-legacy -A INPUT -i wt0 -j NETBIRD-ACL-INPUT
iptables-legacy -N CUSTOM_INPUT; iptables-legacy -A CUSTOM_INPUT -i lo -j ACCEPT
iptables-legacy -A INPUT -s 198.18.99.0/24 -j CUSTOM_INPUT
for i in 1 2 3; do iptables-legacy -A FORWARD -i wt0 -o vmbr1 -j ACCEPT; done
iptables-legacy -A FORWARD -m set --match-set trusted_hosts src -j ACCEPT
printf '*/15 * * * * /bin/true /scripts/firewall.sh\n0 3 * * * /bin/true keep\n' > /scripts/crontab.txt
crontab /scripts/crontab.txt; true`)
	counters := regexp.MustCompile(`\[\d+:\d+\]`)
	state := func() string {
		return counters.ReplaceAllString(sh("iptables-legacy-save | grep -v '^#' | sort; ipset list -n | sort; crontab -l; cat /scripts/crontab.txt"), "")
	}
	foreignTable := sh("nft list table inet filter")
	before := state()

	probe, err := h.Run(ctx, ProbeCommand, nil)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := ParseProbe(string(probe.Stdout))
	if err != nil {
		t.Fatal(err)
	}
	me := netip.MustParseAddr(strings.Fields(sh(`echo "$SSH_CONNECTION"`))[0])
	s := Spec{WAN: facts.WAN, PublicIP: facts.PublicIP, Subnets: pfx("10.0.0.0/22"), TraefikIP: netip.MustParseAddr("10.0.1.200"),
		Ports:  []Port{{Proto: "tcp", Port: 443, TargetPort: 443}, {Proto: "udp", Port: 15433, TargetPort: 15433, Metered: true}},
		Folded: Fold(pfx(me.String()+"/32"), pfx("203.0.113.0/24")), HostAccess: pfx(me.String() + "/32"),
		NetBird: NetBird{Mode: NetBirdDisabled}, DropOtherInbound: true, Meters: DefaultMeters}
	tk := Takeover{Chains: []string{"CUSTOM_INPUT"}, Rules: []string{"filter -A FORWARD -i wt0 -o vmbr1 -j ACCEPT"},
		IPSets: []string{"trusted_hosts"}, CronMatch: []string{"/scripts/firewall.sh"}, CronFiles: []string{"/scripts/crontab.txt"}}
	plan := func(s Spec) (Desired, Diff) {
		t.Helper()
		b, err := Render(s)
		if err != nil {
			t.Fatal(err)
		}
		d := Desired{Spec: s, Table: b, NftPath: facts.NftPath, Takeover: tk, Guard: Guard{RescueIP: me}}
		diff, err := Observe(ctx, h, d)
		if err != nil {
			t.Fatal(err)
		}
		return d, diff
	}
	// The real thing: a new SSH login that runs a command.
	rescue := func(ctx context.Context) error {
		c, err := dial(ctx)
		if err != nil {
			return err
		}
		defer func() { _ = c.Close() }()
		_, err = c.Run(ctx, "true", nil)
		return err
	}
	old := RescueWindow
	RescueWindow = 25 * time.Second
	t.Cleanup(func() { RescueWindow = old })

	// An unconfirmed first apply: the real timer puts everything back.
	d, diff := plan(s)
	if !diff.First || diff.Risk != "high" || diff.Takeover.Empty() {
		t.Fatalf("first plan: %+v", diff)
	}
	if err := Apply(ctx, h, d, diff, func(context.Context) error { return errors.New("simulated") }); !errors.Is(err, ErrRescuePending) {
		t.Fatalf("err %v", err)
	}
	if strings.Contains(state(), "CUSTOM_INPUT") {
		t.Fatal("not removed during the window")
	}
	deadline := time.Now().Add(RescueWindow + 20*time.Second)
	for state() != before || strings.Contains(sh("nft list tables"), "hashi") {
		if time.Now().After(deadline) {
			t.Fatalf("not restored:\n%s\nwas:\n%s\ntables: %s\n%s", state(), before, sh("nft list tables"),
				sh("journalctl -u hashi-fw-rescue --no-pager -n 20 || true"))
		}
		time.Sleep(2 * time.Second)
	}

	// Confirmed over a new SSH login.
	d, diff = plan(s)
	if err := Apply(ctx, h, d, diff, rescue); err != nil {
		t.Fatal(err)
	}
	time.Sleep(RescueWindow + 5*time.Second)
	got := state()
	for _, gone := range []string{"CUSTOM_", "-i wt0 -o vmbr1", "trusted_hosts", "firewall.sh"} {
		if strings.Contains(got, gone) {
			t.Errorf("%q still there:\n%s", gone, got)
		}
	}
	for _, stays := range []string{"NETBIRD-ACL-INPUT", "nb-1", "keep"} {
		if !strings.Contains(got, stays) {
			t.Errorf("%q removed:\n%s", stays, got)
		}
	}
	if sh("nft list table inet filter") != foreignTable {
		t.Error("the foreign nft table changed")
	}
	if !strings.Contains(sh("nft list tables"), "table inet hashi") || sh("systemctl is-enabled "+UnitName) != "enabled\n" {
		t.Error("table or boot unit missing")
	}

	// The table survives a restart of its unit, and a second plan finds nothing.
	sh("systemctl restart " + UnitName)
	if _, again := plan(s); len(again.Items()) != 0 {
		t.Fatalf("second plan: %+v", again.Items())
	}
	if err := Verify(ctx, h, d); err != nil {
		t.Fatal(err)
	}

	// A later unconfirmed change goes back to the previous table; the script stays gone.
	s.Ports = append(s.Ports, Port{Proto: "tcp", Port: 25565, TargetPort: 25565})
	d, diff = plan(s)
	if err := Apply(ctx, h, d, diff, func(context.Context) error { return errors.New("simulated") }); !errors.Is(err, ErrRescuePending) {
		t.Fatalf("err %v", err)
	}
	time.Sleep(RescueWindow + 8*time.Second)
	if out := sh("nft list set inet hashi pub_tcp"); strings.Contains(out, "25565") {
		t.Fatalf("not rolled back:\n%s", out)
	}
	if strings.Contains(state(), "CUSTOM_") {
		t.Fatal("a later rollback brought the script back")
	}
}
