package firewall

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// ProbeCommand is the read-only command validation runs on a firewall host (05 §2). Its output is the contract in
// testdata/contracts/firewall/: every command framed by "### <command>" and "### rc=<exit code>", then the tools.
const ProbeCommand = `for c in "uname -r" "cat /etc/os-release" "nft --version" "nft -j list ruleset" "iptables -V" ` +
	`"ip -j addr show" "ip -j route show default" "ip -j -6 route show default" "ip -j link show" "ss -Hlntup" ` +
	`"systemctl is-enabled nftables" "systemctl is-active nftables" "systemctl is-active firewalld" ` +
	`"netbird status --json" "netbird status" "sysctl -n net.ipv4.ip_forward net.ipv6.conf.all.forwarding" ` +
	`"systemd-run --version"; do echo "### $c"; timeout 10 sh -c "$c" 2>&1; echo "### rc=$?"; done` + "\n" +
	`echo "### tools"; for t in nft iptables iptables-legacy iptables-nft iptables-save iptables-legacy-save ipset ` +
	`netbird systemd-run systemctl rc-service at crontab curl; do command -v $t || echo "missing:$t"; done; echo "### rc=0"`

// Iface is one network interface with its addresses (prefix = address + its network length).
type Iface struct {
	Name  string
	Up    bool
	Addrs []netip.Prefix
}

// Listener is one listening socket from ss.
type Listener struct {
	Proto   string
	Port    int
	Process string
	// Loopback: reachable from the host itself only, so DNAT takes nothing away from it.
	Loopback bool
}

// NetBirdFacts is the NetBird daemon as `netbird status` reports it.
type NetBirdFacts struct {
	Version   string
	Connected bool         // management and signal connected
	Overlay   netip.Prefix // netbirdIp: this peer's address with the overlay network's length
	Overlay6  netip.Prefix
	Port      int // WireGuard listen port
	Kernel    bool
	Iface     string         // the interface holding the overlay address (ip -j addr)
	Routed    []netip.Prefix // networks this peer routes for others (text "Networks:", without default routes)
}

// Facts is what validation learned about a firewall host.
type Facts struct {
	Kernel, OSID, OSPretty string
	NftVersion             string // "1.1.3"; "" when nft is missing
	NftJSON                bool
	IptablesBackend        string   // "legacy", "nf_tables" or "" (no iptables)
	Tables                 []string // existing nft tables as "family name"
	Ifaces                 []Iface
	WAN                    string // interface of the IPv4 default route
	Gateway                netip.Addr
	PublicIP, PublicIP6    netip.Addr
	Listeners              []Listener
	NftablesUnitEnabled    bool // Debian's nftables.service: its config starts with "flush ruleset" (05 §2)
	FirewalldActive        bool
	NetBird                *NetBirdFacts
	IPForward, IP6Forward  bool
	Systemd                bool // systemd-run present: the rescue timer works (05 §7)
	Missing                []string
	NftPath                string // from the tools section; the boot unit runs it
	Ruleset                string // nft -j list ruleset as read
	// SSHFrom is the address the host sees Hashi's SSH connection from ($SSH_CONNECTION): the rescue path that a
	// change must never block (05 §4.4). Set by validation, not by the probe.
	SSHFrom netip.Addr
}

// FilterTable reports an nft table named "filter" in any family: NetBird switches to nftables when it finds one.
func (f Facts) FilterTable() bool {
	return slices.ContainsFunc(f.Tables, func(t string) bool { return strings.HasSuffix(t, " filter") })
}

// HashiTable reports whether Hashi's table already exists.
func (f Facts) HashiTable() bool { return slices.Contains(f.Tables, "inet "+TableName) }

// Candidates are the IPv4 networks on bridges and other interfaces that are not the WAN, loopback or NetBird: the
// managed subnets the setup form proposes.
func (f Facts) Candidates() []netip.Prefix {
	var out []netip.Prefix
	for _, i := range f.Ifaces {
		if i.Name == "lo" || i.Name == f.WAN || (f.NetBird != nil && i.Name == f.NetBird.Iface) {
			continue
		}
		for _, a := range i.Addrs {
			if a.Addr().Is4() && !a.Addr().IsLinkLocalUnicast() {
				out = append(out, a.Masked())
			}
		}
	}
	return out
}

// Warnings are the findings the admin must resolve or accept before the first apply (05 §2).
func (f Facts) Warnings() []string {
	var w []string
	if !f.Systemd {
		w = append(w, "systemd-run is missing: the rescue timer needs systemd (Alpine/OpenRC firewall hosts arrive later)")
	}
	if f.FilterTable() {
		w = append(w, "an nftables table named \"filter\" exists: NetBird switches to its nftables backend at its next restart")
	}
	if f.NftablesUnitEnabled {
		w = append(w, "nftables.service is enabled: its config usually starts with \"flush ruleset\" and would wipe NetBird's and Proxmox's rules at boot; disable it")
	}
	if f.FirewalldActive {
		w = append(w, "firewalld is active: it keeps its own table and may drop forwarded traffic")
	}
	return w
}

// ErrNoNft means the host lacks nft ≥ 1.0 with JSON support (05 §2); the apply plan installs it.
var ErrNoNft = errors.New("nftables 1.0 or newer with JSON support is required")

type section struct {
	out string
	rc  int
}

func sections(out string) map[string]section {
	res := map[string]section{}
	var cmd string
	var body []string
	// A command without a final newline (netbird status --json) leaves "### rc=" at the end of its last line.
	out = regexp.MustCompile(`(?m)(\S)(### rc=\d+)$`).ReplaceAllString(out, "$1\n$2")
	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "### rc="):
			rc, _ := strconv.Atoi(strings.TrimPrefix(line, "### rc="))
			res[cmd] = section{out: strings.Join(body, "\n"), rc: rc}
			cmd, body = "", nil
		case strings.HasPrefix(line, "### "):
			cmd, body = strings.TrimPrefix(line, "### "), nil
		default:
			body = append(body, line)
		}
	}
	return res
}

var (
	nftVersion = regexp.MustCompile(`^nftables v(\d+)\.(\d+)\.(\d+)`)
	iptVersion = regexp.MustCompile(`\((legacy|nf_tables)\)`)
)

// ParseProbe reads ProbeCommand's output.
func ParseProbe(out string) (Facts, error) {
	s := sections(out)
	var f Facts
	f.Kernel = strings.TrimSpace(s["uname -r"].out)
	for _, line := range strings.Split(s["cat /etc/os-release"].out, "\n") {
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"`)
		switch k {
		case "ID":
			f.OSID = v
		case "PRETTY_NAME":
			f.OSPretty = v
		}
	}
	if m := nftVersion.FindStringSubmatch(strings.TrimSpace(s["nft --version"].out)); m != nil && s["nft --version"].rc == 0 {
		f.NftVersion = m[1] + "." + m[2] + "." + m[3]
	}
	if r := s["nft -j list ruleset"]; r.rc == 0 {
		var rs struct {
			Nftables []map[string]json.RawMessage `json:"nftables"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(r.out)), &rs); err == nil {
			f.NftJSON, f.Ruleset = true, strings.TrimSpace(r.out)
			for _, o := range rs.Nftables {
				if t, ok := o["table"]; ok {
					var tb struct{ Family, Name string }
					_ = json.Unmarshal(t, &tb)
					f.Tables = append(f.Tables, tb.Family+" "+tb.Name)
				}
			}
		}
	}
	if m := iptVersion.FindStringSubmatch(s["iptables -V"].out); m != nil && s["iptables -V"].rc == 0 {
		f.IptablesBackend = m[1]
	}
	if err := parseAddrs(&f, s["ip -j addr show"].out); err != nil {
		return f, err
	}
	var routes []struct{ Dev, Gateway string }
	if err := json.Unmarshal([]byte(strings.TrimSpace(s["ip -j route show default"].out)), &routes); err != nil {
		return f, fmt.Errorf("default route: %w", err)
	}
	if len(routes) > 0 {
		f.WAN = routes[0].Dev
		f.Gateway, _ = netip.ParseAddr(routes[0].Gateway)
	}
	for _, i := range f.Ifaces {
		if i.Name != f.WAN {
			continue
		}
		for _, a := range i.Addrs {
			switch {
			case a.Addr().Is4() && !f.PublicIP.IsValid():
				f.PublicIP = a.Addr()
			case a.Addr().Is6() && a.Addr().IsGlobalUnicast() && !f.PublicIP6.IsValid() && !isULA(a.Addr()):
				f.PublicIP6 = a.Addr()
			}
		}
	}
	f.Listeners = parseSS(s["ss -Hlntup"].out)
	f.NftablesUnitEnabled = strings.TrimSpace(s["systemctl is-enabled nftables"].out) == "enabled"
	f.FirewalldActive = s["systemctl is-active firewalld"].rc == 0
	if r := s["netbird status --json"]; r.rc == 0 {
		nb, err := parseNetBird(r.out, s["netbird status"].out, f.Ifaces)
		if err != nil {
			return f, err
		}
		f.NetBird = nb
	}
	fw := strings.Fields(s["sysctl -n net.ipv4.ip_forward net.ipv6.conf.all.forwarding"].out)
	f.IPForward = len(fw) > 0 && fw[0] == "1"
	f.IP6Forward = len(fw) > 1 && fw[1] == "1"
	for _, line := range strings.Split(s["tools"].out, "\n") {
		line = strings.TrimSpace(line)
		if t, ok := strings.CutPrefix(line, "missing:"); ok {
			f.Missing = append(f.Missing, t)
		}
		if strings.HasPrefix(line, "/") && strings.HasSuffix(line, "/nft") && f.NftPath == "" {
			f.NftPath = line
		}
	}
	f.Systemd = !slices.Contains(f.Missing, "systemd-run")
	if f.NftVersion == "" || strings.HasPrefix(f.NftVersion, "0.") || !f.NftJSON {
		return f, ErrNoNft
	}
	return f, nil
}

func parseAddrs(f *Facts, out string) error {
	var links []struct {
		Ifname   string   `json:"ifname"`
		Flags    []string `json:"flags"`
		AddrInfo []struct {
			Local     string `json:"local"`
			Prefixlen int    `json:"prefixlen"`
		} `json:"addr_info"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &links); err != nil {
		return fmt.Errorf("interfaces: %w", err)
	}
	for _, l := range links {
		i := Iface{Name: l.Ifname, Up: slices.Contains(l.Flags, "UP")}
		for _, a := range l.AddrInfo {
			if addr, err := netip.ParseAddr(a.Local); err == nil {
				i.Addrs = append(i.Addrs, netip.PrefixFrom(addr, a.Prefixlen))
			}
		}
		f.Ifaces = append(f.Ifaces, i)
	}
	return nil
}

// parseSS reads `ss -Hlntup`: "proto state recvq sendq local peer [users:((…))]".
func parseSS(out string) []Listener {
	var ls []Listener
	for _, line := range strings.Split(out, "\n") {
		fs := strings.Fields(line)
		if len(fs) < 5 {
			continue
		}
		local := fs[4]
		i := strings.LastIndex(local, ":")
		port, err := strconv.Atoi(local[i+1:])
		if i < 0 || err != nil {
			continue
		}
		l := Listener{Proto: fs[0], Port: port, Loopback: strings.HasPrefix(local, "127.") || strings.HasPrefix(local, "[::1]")}
		if len(fs) > 6 {
			if m := regexp.MustCompile(`\(\("([^"]+)"`).FindStringSubmatch(fs[6]); m != nil {
				l.Process = m[1]
			}
		}
		ls = append(ls, l)
	}
	return ls
}

func parseNetBird(jsonOut, text string, ifaces []Iface) (*NetBirdFacts, error) {
	var st struct {
		DaemonVersion      string `json:"daemonVersion"`
		NetbirdIP          string `json:"netbirdIp"`
		NetbirdIPv6        string `json:"netbirdIpv6"`
		UsesKernelIface    bool   `json:"usesKernelInterface"`
		WireguardPort      int    `json:"wireguardPort"`
		Management, Signal struct {
			Connected bool `json:"connected"`
		}
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(jsonOut)), &st); err != nil {
		return nil, fmt.Errorf("netbird status --json: %w", err)
	}
	nb := &NetBirdFacts{Version: st.DaemonVersion, Connected: st.Management.Connected && st.Signal.Connected,
		Port: st.WireguardPort, Kernel: st.UsesKernelIface}
	nb.Overlay, _ = netip.ParsePrefix(st.NetbirdIP)
	nb.Overlay6, _ = netip.ParsePrefix(st.NetbirdIPv6)
	for _, i := range ifaces {
		for _, a := range i.Addrs {
			if nb.Overlay.IsValid() && a.Addr() == nb.Overlay.Addr() {
				nb.Iface = i.Name
			}
		}
	}
	for _, line := range strings.Split(text, "\n") {
		v, ok := strings.CutPrefix(strings.TrimSpace(line), "Networks:")
		if !ok {
			continue
		}
		for _, n := range strings.Split(v, ",") {
			p, err := netip.ParsePrefix(strings.TrimSpace(n))
			if err == nil && p.Bits() > 0 {
				nb.Routed = append(nb.Routed, p.Masked())
			}
		}
	}
	return nb, nil
}

func isULA(a netip.Addr) bool { return netip.MustParsePrefix("fc00::/7").Contains(a) }
