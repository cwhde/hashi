//go:build integration

package firewall

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/itest"
	"git.juzo.io/juzo/hashi/internal/sshx"
)

const (
	fwImage = "hashi-itest-firewall"
	// stub stands in for Traefik and an outside client: BusyBox has httpd, wget and ip.
	stub = "busybox:1.37.0"
)

func fwContainer(t *testing.T, flags ...string) itest.Container {
	t.Helper()
	itest.Build(t, fwImage, filepath.Join("testdata", "itest"))
	return itest.RunIsolated(t, append([]string{"--cap-add", "NET_ADMIN", "--cap-add", "NET_RAW"}, flags...), fwImage)
}

// apply renders s, copies it in and loads it the way Hashi does (one nft -j -f transaction).
func apply(t *testing.T, c itest.Container, s Spec, check bool) string {
	t.Helper()
	out, err := Render(s)
	if err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(t.TempDir(), "hashi.nft.json")
	if err := os.WriteFile(f, out, 0o600); err != nil {
		t.Fatal(err)
	}
	c.CopyTo(t, f, "/tmp/hashi.nft.json")
	args := []string{"nft", "-j", "-f", "/tmp/hashi.nft.json"}
	if check {
		args = []string{"nft", "-c", "-j", "-f", "/tmp/hashi.nft.json"}
	}
	return c.Exec(t, args...)
}

// Every golden shape passes nft's check mode (05 §10).
func TestIntegrationRenderedTablesCheck(t *testing.T) {
	c := fwContainer(t)
	for name, s := range shapes() {
		t.Run(name, func(t *testing.T) { apply(t, c, s, true) })
	}
}

var counters = regexp.MustCompile(`\[\d+:\d+\]`)

// foreign is everything that is not Hashi's: iptables-legacy (v4 + v6), ipsets and the foreign nft table.
func foreign(t *testing.T, c itest.Container) string {
	t.Helper()
	var b strings.Builder
	for _, cmd := range [][]string{{"iptables-legacy-save"}, {"ip6tables-legacy-save"}, {"ipset", "save"},
		{"nft", "list", "table", "inet", "foreign"}} {
		out := c.Exec(t, cmd...)
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "#") {
				continue // save timestamps
			}
			b.WriteString(counters.ReplaceAllString(line, "") + "\n")
		}
	}
	return b.String()
}

// NetBird-like iptables-legacy chains, jumps and ipsets plus a foreign nft table (05 §10): Hashi's applies leave them
// byte-identical, keep its dynamic elements, and NetBird's backend choice stays iptables.
func TestIntegrationCoexistsWithNetBirdAndForeignTables(t *testing.T) {
	c := fwContainer(t)
	for _, cmd := range []string{
		"ipset create nb0000001 hash:net", "ipset add nb0000001 10.31.0.1",
		"iptables -N NETBIRD-ACL-INPUT", "iptables -A INPUT -i wt0 -j NETBIRD-ACL-INPUT",
		"iptables -A NETBIRD-ACL-INPUT -m set --match-set nb0000001 src -j ACCEPT", "iptables -A INPUT -i wt0 -j DROP",
		"iptables -N NETBIRD-RT-FWD-IN", "iptables -A FORWARD -i wt0 -j NETBIRD-RT-FWD-IN",
		"iptables -t nat -N NETBIRD-RT-NAT", "iptables -t nat -A POSTROUTING -j NETBIRD-RT-NAT",
		"iptables -t nat -A NETBIRD-RT-NAT -o wt0 -m mark --mark 0x1bd22 -j MASQUERADE",
		"iptables -t mangle -N NETBIRD-RT-MSSCLAMP", "iptables -t mangle -A FORWARD -j NETBIRD-RT-MSSCLAMP",
		"ip6tables -N NETBIRD-ACL-INPUT",
		"nft add table inet foreign", "nft 'add chain inet foreign input { type filter hook input priority 10 ; }'",
		"nft add rule inet foreign input tcp dport 8006 counter accept",
		"nft 'add set inet foreign keep { type ipv4_addr ; }'", "nft 'add element inet foreign keep { 192.0.2.99 }'",
	} {
		c.Exec(t, "sh", "-c", cmd)
	}
	before := foreign(t, c)

	s := shapes()["netbird_routing.json"]
	s.WAN = "eth0"
	apply(t, c, s, false)
	c.Exec(t, "nft", "add", "element", "inet", "hashi", "dyn4", "{ 203.0.113.50 timeout 1h }")
	c.Exec(t, "nft", "add", "element", "inet", "hashi", "staging4", "{ 203.0.113.51 }")
	// A changed allow list and meters switched on: a structural re-apply.
	s.Folded = Fold(pfx("198.51.100.8/32"), pfx("203.0.113.0/24"))
	s.Meters.Enabled = true
	apply(t, c, s, false)
	apply(t, c, s, false) // idempotent

	if after := foreign(t, c); after != before {
		t.Fatalf("foreign state changed:\n--- before\n%s\n--- after\n%s", before, after)
	}
	dyn := c.Exec(t, "nft", "list", "set", "inet", "hashi", "dyn4")
	stg := c.Exec(t, "nft", "list", "set", "inet", "hashi", "staging4")
	allow := c.Exec(t, "nft", "list", "set", "inet", "hashi", "allow4")
	if !strings.Contains(dyn, "203.0.113.50") || !strings.Contains(stg, "203.0.113.51") {
		t.Fatalf("dynamic elements lost:\n%s\n%s", dyn, stg)
	}
	if !strings.Contains(allow, "198.51.100.8") || strings.Contains(allow, "198.51.100.7") {
		t.Fatalf("allow4 not replaced:\n%s", allow)
	}
	if n := strings.Count(c.Exec(t, "nft", "list", "chain", "inet", "hashi", "pre_guard"), "drop"); n != 6 {
		t.Fatalf("pre_guard has %d drops after three applies, want 6 (no duplicates)", n)
	}

	// NetBird's own backend selection (v0.81.3, testdata/contracts/netbird/backend_check) still picks iptables.
	bin := filepath.Join(t.TempDir(), "nbcheck")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = filepath.Join("..", "..", "testdata", "contracts", "netbird", "backend_check")
	build.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+itest.Arch(t), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build NetBird backend replay: %v\n%s", err, out)
	}
	c.CopyTo(t, bin, "/usr/local/bin/nbcheck")
	if got := strings.TrimSpace(c.Exec(t, "nbcheck")); got != "IPTABLES" {
		t.Fatalf("NetBird would pick %s with Hashi's table present", got)
	}
}

// A connection to a published port on the WAN address reaches the Traefik stand-in at its entry-point port, and the
// managed subnet's egress is masqueraded (05 §10 DNAT test). Three containers on two networks: client ↔ WAN ↔
// firewall ↔ LAN ↔ "Traefik" (whose default route is the firewall).
func TestIntegrationDNATReachesTraefik(t *testing.T) {
	itest.Build(t, fwImage, filepath.Join("testdata", "itest"))
	// Fresh names and ranges per run: CI runners share one Docker daemon, and a failed run may leave a network behind.
	n := 10 + rand.IntN(240)
	wanNet, lanNet := fmt.Sprintf("hashi-fw-wan-%d-%d", n, os.Getpid()), fmt.Sprintf("hashi-fw-lan-%d-%d", n, os.Getpid())
	w := func(h int) string { return fmt.Sprintf("198.18.%d.%d", n, h) }
	l := func(h int) string { return fmt.Sprintf("10.77.%d.%d", n, h) }
	itest.Network(t, wanNet, w(0)+"/24")
	itest.Network(t, lanNet, l(0)+"/24")
	fw := itest.RunOn(t, wanNet, w(2), []string{"--cap-add", "NET_ADMIN", "--cap-add", "NET_RAW",
		"--sysctl", "net.ipv4.ip_forward=1"}, fwImage)
	fw.Connect(t, lanNet, l(2))
	traefik := itest.RunOn(t, lanNet, l(10), []string{"--cap-add", "NET_ADMIN"}, stub,
		"sh", "-c", "mkdir -p /www && echo entrypoint > /www/index.html && httpd -f -p 8443 -h /www")
	client := itest.RunOn(t, wanNet, w(3), []string{"--cap-add", "NET_ADMIN"}, stub,
		"sh", "-c", "mkdir -p /www && echo outside > /www/index.html && httpd -f -p 8080 -h /www")
	traefik.Exec(t, "ip", "route", "replace", "default", "via", l(2))
	// Precondition, before any Hashi rule: plain routing client → firewall container → stand-in. Some Docker hosts
	// filter traffic routed between their bridges (br_netfilter); there the test cannot say anything about DNAT.
	client.Exec(t, "ip", "route", "add", l(0)+"/24", "via", w(2))
	routed := false
	for range 10 {
		if out, err := client.Try("wget", "-qO-", "-T", "1", "http://"+l(10)+":8443/"); err == nil && strings.Contains(out, "entrypoint") {
			routed = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !routed {
		t.Skip("this Docker host drops traffic routed between its bridges; DNAT is proven on a local Docker and at the S6 gate")
	}
	client.Exec(t, "ip", "route", "del", l(0)+"/24")

	wan := strings.TrimSpace(fw.Exec(t, "sh", "-c", "ip -o -4 addr show | awk '/"+w(2)+"\\// {print $2}'"))
	s := Spec{WAN: wan, PublicIP: netip.MustParseAddr(w(2)), Subnets: pfx(l(0) + "/24"),
		TraefikIP: netip.MustParseAddr(l(10)), Ports: []Port{{Proto: "tcp", Port: 443, TargetPort: 8443}},
		Folded: Fold(nil, pfx("203.0.113.0/24")), NetBird: NetBird{Mode: NetBirdDisabled}, Meters: DefaultMeters}
	apply(t, fw, s, false)

	var got string
	itest.WaitFor(t, 20*time.Second, "DNAT to the entry point", func() bool {
		got, _ = client.Try("wget", "-qO-", "-T", "2", "http://"+w(2)+":443/")
		return strings.Contains(got, "entrypoint")
	})
	if out, err := traefik.Try("wget", "-qO-", "-T", "3", "http://"+w(3)+":8080/"); err != nil || !strings.Contains(out, "outside") {
		t.Fatalf("managed subnet egress through the firewall: %v %s", err, out)
	}
	// Nothing else is published: another port on the WAN address does not reach the stand-in.
	if out, err := client.Try("wget", "-qO-", "-T", "2", "http://"+w(2)+":8443/"); err == nil {
		t.Fatalf("unpublished port reached something: %s", out)
	}
}

// containerHost is a Host over docker exec.
type containerHost struct{ c itest.Container }

func (h containerHost) Run(_ context.Context, cmd string, stdin io.Reader) (sshx.Result, error) {
	var in []byte
	if stdin != nil {
		in, _ = io.ReadAll(stdin)
	}
	o, e, code, err := h.c.Shell(cmd, in)
	return sshx.Result{Stdout: o, Stderr: e, ExitCode: code}, err
}

func (h containerHost) ReadFile(p string) ([]byte, error) {
	o, _, code, err := h.c.Shell("cat "+p, nil)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fs.ErrNotExist
	}
	return o, nil
}

func (h containerHost) WriteFileAtomic(p string, data []byte, mode fs.FileMode) (bool, error) {
	_, e, code, err := h.c.Shell(fmt.Sprintf("cat > %[1]s.tmp && chmod %[2]o %[1]s.tmp && mv %[1]s.tmp %[1]s", p, mode), data)
	if err == nil && code != 0 {
		err = fmt.Errorf("write %s: %s", p, e)
	}
	return true, err
}

func (h containerHost) MkdirAll(p string, mode fs.FileMode) error {
	_, _, _, err := h.c.Shell(fmt.Sprintf("mkdir -p %s && chmod %o %s", p, mode, p), nil)
	return err
}

// 05 §7 end to end: a first apply confirmed by rescue disarms the timer; a risky change whose confirmation fails is
// rolled back by the timer to the previous table, and foreign rules stay untouched throughout.
func TestIntegrationRescueRollback(t *testing.T) {
	old := RescueWindow
	RescueWindow = 3 * time.Second
	t.Cleanup(func() { RescueWindow = old })
	c := fwContainer(t, "--sysctl", "net.ipv4.ip_forward=1")
	c.Exec(t, "sh", "-c", "iptables -N NETBIRD-ACL-INPUT && iptables -A INPUT -j NETBIRD-ACL-INPUT && nft 'add table inet foreign'")
	before := strings.ReplaceAll(foreign(t, c), "nft list table inet foreign", "")
	h := containerHost{c}
	ctx := context.Background()

	s := shapes()["netbird_routing.json"]
	s.WAN = "eth0"
	plan := func(s Spec) (Desired, Diff) {
		d := desired(t, s)
		d.NftPath = "/usr/sbin/nft"
		diff, err := Observe(ctx, h, d)
		if err != nil {
			t.Fatal(err)
		}
		return d, diff
	}
	d, diff := plan(s)
	if !diff.First {
		t.Fatalf("not a first apply: %+v", diff)
	}
	if err := Apply(ctx, h, d, diff, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	confirmed := d
	time.Sleep(RescueWindow + 2*time.Second)
	if out := c.Exec(t, "nft", "list", "set", "inet", "hashi", "pub_tcp"); !strings.Contains(out, "2022") {
		t.Fatalf("confirmed first apply was rolled back:\n%s", out)
	}

	s.Ports = append(s.Ports, Port{Proto: "tcp", Port: 25565, TargetPort: 25565})
	d, diff = plan(s)
	if diff.Risk != "high" {
		t.Fatalf("opening a port is %s", diff.Risk)
	}
	err := Apply(ctx, h, d, diff, func(context.Context) error { return errors.New("no route to host") })
	if !errors.Is(err, ErrRescuePending) {
		t.Fatalf("err %v", err)
	}
	if out := c.Exec(t, "nft", "list", "set", "inet", "hashi", "pub_tcp"); !strings.Contains(out, "25565") {
		t.Fatalf("the change was not applied before the window:\n%s", out)
	}
	itest.WaitFor(t, RescueWindow+10*time.Second, "the rescue timer restores the previous table", func() bool {
		out, _ := c.Try("nft", "list", "set", "inet", "hashi", "pub_tcp")
		return strings.Contains(out, "2022") && !strings.Contains(out, "25565")
	})
	if cur, _ := h.ReadFile(TableFile); string(cur) != string(confirmed.Table) {
		t.Fatal("the rendered file on the host is not the confirmed one again (the boot unit would load the rolled-back table)")
	}
	if after := strings.ReplaceAll(foreign(t, c), "nft list table inet foreign", ""); after != before {
		t.Fatalf("foreign state changed:\n%s\n---\n%s", before, after)
	}
}
