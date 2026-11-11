package firewall

import (
	"bytes"
	"encoding/json"
	"flag"
	"git.juzo.io/juzo/hashi/internal/security"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	"io"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite golden files")

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	p := filepath.Join("testdata", "golden", name)
	if *update {
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("%v (run go test ./internal/firewall -update)", err)
	}
	if !bytes.Equal(want, got) {
		t.Errorf("%s differs from the golden file (run go test ./internal/firewall -update and review the diff)", name)
	}
}

// Host shapes every golden file covers (05 §10): managed with NetBird routing and ports, no NetBird and no ports,
// meters on with IPv6 and inbound drop, and many ports.
func shapes() map[string]Spec {
	addr := netip.MustParseAddr
	routing := Spec{WAN: "vmbr0", PublicIP: addr("192.0.2.1"), Subnets: pfx("10.20.0.0/24"), TraefikIP: addr("10.20.0.10"),
		Ports: []Port{{Proto: "tcp", Port: 443, TargetPort: 443}, {Proto: "tcp", Port: 80, TargetPort: 80},
			{Proto: "tcp", Port: 2022, TargetPort: 2022}, {Proto: "udp", Port: 19132, TargetPort: 19132, Metered: true}},
		Folded:     Fold(pfx("198.51.100.7/32", "203.0.113.0/28"), pfx("203.0.113.0/24")),
		HostAccess: pfx("198.51.100.0/28"), NetBird: NetBird{Mode: NetBirdRouting, Iface: "wt0", Port: 51820}, Meters: DefaultMeters}
	bare := Spec{WAN: "eth0", Subnets: pfx("10.20.0.0/24"), NetBird: NetBird{Mode: NetBirdDisabled}, Meters: DefaultMeters}
	meters := routing
	meters.Meters.Enabled = true
	meters.DropOtherInbound = true
	meters.Subnets = pfx("10.20.0.0/24", "fd00:20::/64")
	meters.Folded = Fold(pfx("198.51.100.7/32", "2001:db8:1::/48"), pfx("203.0.113.0/24", "2001:db8::/32"))
	meters.HostAccess = pfx("198.51.100.0/28", "2001:db8:5::/48")
	meters.NetBird = NetBird{Mode: NetBirdPeer, Iface: "wt0", Port: 51820}
	many := routing
	many.Ports = nil
	for p := 20000; p < 20040; p++ {
		many.Ports = append(many.Ports, Port{Proto: "tcp", Port: p, TargetPort: p}, Port{Proto: "udp", Port: p, TargetPort: p, Metered: true})
	}
	many.Ports = append(many.Ports, Port{Proto: "udp", Port: 443, TargetPort: 443, Metered: true}) // HTTP/3: never metered
	return map[string]Spec{"netbird_routing.json": routing, "bare.json": bare, "meters_ipv6_drop.json": meters, "many_ports.json": many}
}

func TestRenderGolden(t *testing.T) {
	for name, s := range shapes() {
		out, err := Render(s)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		golden(t, name, out)
	}
}

func TestRenderStructure(t *testing.T) {
	out, err := Render(shapes()["meters_ipv6_drop.json"])
	if err != nil {
		t.Fatal(err)
	}
	var b struct {
		Nftables []map[string]map[string]json.RawMessage `json:"nftables"`
	}
	if err := json.Unmarshal(out, &b); err != nil {
		t.Fatal(err)
	}
	flushedSets := map[string]bool{}
	var chainOrder []string
	for _, c := range b.Nftables {
		if f, ok := c["flush"]["set"]; ok {
			var r ref
			_ = json.Unmarshal(f, &r)
			flushedSets[r.Name] = true
		}
		if a, ok := c["add"]["chain"]; ok {
			var ch chain
			_ = json.Unmarshal(a, &ch)
			chainOrder = append(chainOrder, ch.Name)
		}
		if _, ok := c["delete"]; ok {
			t.Fatal("an apply never deletes")
		}
	}
	for _, never := range []string{"dyn4", "dyn6", "staging4", "staging6", "m_tcp_new4", "m_tcp_conc4", "m_udp_pps6"} {
		if flushedSets[never] {
			t.Errorf("%s is flushed; dynamic and meter sets keep their elements (05 §3.3)", never)
		}
	}
	for _, s := range planSets {
		if !flushedSets[s] {
			t.Errorf("%s is not replaced atomically", s)
		}
	}
	if strings.Join(chainOrder, ",") != "pre_guard,pre_meter,nat_pre,nat_out,input,forward,nat_post,mangle_fwd" {
		t.Errorf("chains %v", chainOrder)
	}
	text := string(out)
	for _, want := range []string{`"name": "hashi"`, `"ct count"`, `"@staging6"`, `"prio": -300`, `"right": "@allow4"`} {
		if !strings.Contains(text, want) {
			t.Errorf("rendered table lacks %s", want)
		}
	}
	if strings.Contains(text, `"flush": {"ruleset"`) || strings.Contains(text, `"name": "filter"`) {
		t.Fatal("never flush ruleset or name a table filter (invariant 12, NetBird)")
	}
}

// invariant 20 in the rendered table: in pre_guard and pre_meter the allow sets return before any drop or meter.
func TestInvariant_20_AllowBeforeDropInKernel(t *testing.T) {
	out, err := Render(shapes()["meters_ipv6_drop.json"])
	if err != nil {
		t.Fatal(err)
	}
	var b struct {
		Nftables []map[string]map[string]rule `json:"nftables"`
	}
	if err := json.Unmarshal(out, &b); err != nil {
		t.Fatal(err)
	}
	for _, chainName := range []string{"pre_guard", "pre_meter"} {
		allowSeen, drops := map[string]bool{}, 0
		for _, c := range b.Nftables {
			r, ok := c["add"]["rule"]
			if !ok || r.Chain != chainName {
				continue
			}
			e, _ := json.Marshal(r.Expr)
			s := string(e)
			for _, v := range []string{"4", "6"} {
				// Expressions marshal with sorted keys: a source match reads {"field":"saddr","protocol":"ip"}.
				family := strings.Contains(s, `"field":"saddr","protocol":"`+fam(v == "4")+`"}`)
				if strings.Contains(s, `"@allow`+v+`"`) {
					allowSeen[v] = true
					continue
				}
				if family && (strings.Contains(s, `"drop":null`) || strings.Contains(s, `"@staging`+v+`"`)) && !allowSeen[v] {
					t.Errorf("%s: a drop for IPv%s comes before the allow%s return: %s", chainName, v, v, s)
				}
				if family && strings.Contains(s, `"drop":null`) {
					drops++
				}
			}
		}
		if !allowSeen["4"] || !allowSeen["6"] || drops == 0 {
			t.Errorf("%s: allow returns %v, %d source drops checked", chainName, allowSeen, drops)
		}
	}
}

func TestRenderRefusesBadSpecs(t *testing.T) {
	ok := shapes()["netbird_routing.json"]
	bad := map[string]func(s *Spec){
		"no WAN":               func(s *Spec) { s.WAN = "" },
		"no subnet":            func(s *Spec) { s.Subnets = nil },
		"NetBird without port": func(s *Spec) { s.NetBird.Port = 0 },
		"NetBird on WAN":       func(s *Spec) { s.NetBird.Iface = s.WAN },
		"duplicate port":       func(s *Spec) { s.Ports = append(s.Ports, Port{Proto: "tcp", Port: 443, TargetPort: 8443}) },
		"WireGuard port":       func(s *Spec) { s.Ports = append(s.Ports, Port{Proto: "udp", Port: 51820, TargetPort: 1}) },
		"bad proto":            func(s *Spec) { s.Ports = []Port{{Proto: "sctp", Port: 1, TargetPort: 1}} },
		"IPv6 Traefik":         func(s *Spec) { s.TraefikIP = netip.MustParseAddr("fd00::10") },
	}
	for name, mut := range bad {
		s := ok
		s.Ports = append([]Port(nil), ok.Ports...)
		mut(&s)
		if _, err := Render(s); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

// The Firewall settings' MSS clamp switch (09 §2): a routing peer clamps by default and not when it is off.
func TestRenderMSSClampSetting(t *testing.T) {
	routing := shapes()["netbird_routing.json"]
	on, err := Render(routing)
	if err != nil {
		t.Fatal(err)
	}
	routing.NoMSSClamp = true
	off, err := Render(routing)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(on, []byte(`"maxseg"`)) || bytes.Contains(off, []byte(`"maxseg"`)) {
		t.Fatalf("clamp on %v, off %v", bytes.Contains(on, []byte(`"maxseg"`)), bytes.Contains(off, []byte(`"maxseg"`)))
	}
}

// The L4 meter and staging settings (06 §10-11) reach the rendered table through MetersFrom; off renders no meter rule.
func TestRenderMeterSettings(t *testing.T) {
	e := settings.DefaultOf[settings.Engine]()
	e.MetersEnabled, e.TCPNewPerMinute, e.TCPBurst, e.TCPMaxConcurrent = true, 777, 77, 333
	e.UDPNewPerMinute, e.UDPBurst, e.UDPPerSecond, e.UDPPPSBurst, e.StagingSeconds = 222, 22, 4444, 444, 123
	s := shapes()["netbird_routing.json"]
	s.Meters = MetersFrom(e)
	out, err := Render(s)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"777", "77", "333", "222", "22", "4444", "444", "123"} {
		if !regexp.MustCompile(`[:\s\[]` + n + `[,}\]\s]`).Match(out) {
			t.Errorf("%s from the settings is not in the table", n)
		}
	}
	e.MetersEnabled = false
	s.Meters = MetersFrom(e)
	if out, _ := Render(s); regexp.MustCompile(`[:\s\[]4444[,}\]\s]`).Match(out) { // the sets stay declared, the rules go
		t.Error("meters off still render meter rules")
	}
}

// The host link polls at the engine's poll interval (06 §10).
func TestHostLinkPollSetting(t *testing.T) {
	eng := security.NewEngine(storetest.New(t), slog.New(slog.NewTextHandler(io.Discard, nil)), time.Now)
	cfg := eng.Settings()
	cfg.PollIntervalSecs = 7
	eng.UseSettings(cfg)
	if got := NewLink(nil, eng, nil).poll(); got != 7*time.Second {
		t.Fatalf("poll %v", got)
	}
}
