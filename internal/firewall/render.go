package firewall

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"
	"time"
)

// TableName is Hashi's only nftables table (invariant 12). It must never be "filter": NetBird ≥ 0.81 switches to
// its nftables backend when an nft table of that name exists (testdata/contracts/nft/README.md).
const TableName = "hashi"

// NetBird modes (05 §5).
const (
	NetBirdDisabled = "disabled"
	NetBirdPeer     = "peer"
	NetBirdRouting  = "routing"
)

// Port is one published public port and where it goes: the linked Traefik's entry point (05 §3.2 chain 3).
type Port struct {
	Proto      string // "tcp" or "udp"
	Port       int
	TargetPort int
	// Metered: an L4 UDP stream port that the UDP meters watch (never 443/udp, 05 §3.2).
	Metered bool
}

// Meters are the L4 kernel meter thresholds (06 §11 `l4.*`). Off renders the chain without meter rules.
type Meters struct {
	Enabled                   bool
	TCPNewPerMinute, TCPBurst int
	TCPMaxConcurrent          int
	UDPNewPerMinute, UDPBurst int
	UDPPerSecond, UDPPPSBurst int
	StagingTimeout            time.Duration
}

// DefaultMeters are the spec defaults (06 §11).
var DefaultMeters = Meters{TCPNewPerMinute: 150, TCPBurst: 30, TCPMaxConcurrent: 250, UDPNewPerMinute: 200,
	UDPBurst: 50, UDPPerSecond: 5000, UDPPPSBurst: 1000, StagingTimeout: 5 * time.Minute}

// NetBird is the host's detected NetBird and the mode the user chose.
type NetBird struct {
	Mode  string
	Iface string // never assumed: detected or entered (05 §5)
	Port  int    // WireGuard listen port
}

// Spec is everything one managed host's table is rendered from.
type Spec struct {
	WAN       string
	PublicIP  netip.Addr     // IPv4; hairpin DNAT for internal clients matches it
	Subnets   []netip.Prefix // managed subnets
	TraefikIP netip.Addr     // the linked Traefik's internal address (IPv4)
	Ports     []Port
	Folded    Folded
	// HostAccess is the resolved host access sets (match position).
	HostAccess []netip.Prefix
	NetBird    NetBird
	// DropOtherInbound: the user accepted the "block all other inbound" warning for this host (05 §3.2 chain 4).
	DropOtherInbound bool
	Meters           Meters
}

// planSets are the sets whose elements a plan replaces; dyn, staging and meter sets are never flushed (05 §3.3).
var (
	planSets = []string{"allow4", "allow6", "block4", "block6", "host_access4", "host_access6", "pub_tcp", "pub_udp",
		"metered_udp"}
)

// Validate refuses a spec that cannot render a safe table.
func (s Spec) Validate() error {
	if s.WAN == "" {
		return fmt.Errorf("no WAN interface")
	}
	if !s.TraefikIP.Is4() && len(s.Ports) > 0 {
		return fmt.Errorf("the internal Traefik address must be IPv4")
	}
	if len(s.Subnets) == 0 {
		return fmt.Errorf("no managed subnet")
	}
	if s.NetBird.Mode != NetBirdDisabled && (s.NetBird.Iface == "" || s.NetBird.Port < 1 || s.NetBird.Port > 65535) {
		return fmt.Errorf("NetBird %s mode needs its interface and WireGuard port", s.NetBird.Mode)
	}
	if s.NetBird.Iface == s.WAN && s.NetBird.Iface != "" {
		return fmt.Errorf("the NetBird interface cannot be the WAN interface")
	}
	seen := map[string]bool{}
	for _, p := range s.Ports {
		if p.Proto != "tcp" && p.Proto != "udp" || p.Port < 1 || p.Port > 65535 || p.TargetPort < 1 || p.TargetPort > 65535 {
			return fmt.Errorf("bad port %s/%d → %d", p.Proto, p.Port, p.TargetPort)
		}
		key := fmt.Sprintf("%s/%d", p.Proto, p.Port)
		if seen[key] {
			return fmt.Errorf("port %s published twice", key)
		}
		seen[key] = true
		if s.NetBird.Mode != NetBirdDisabled && p.Proto == "udp" && p.Port == s.NetBird.Port {
			return fmt.Errorf("udp/%d is NetBird's WireGuard port", p.Port)
		}
	}
	return nil
}

// Render returns the full table as one libnftables JSON transaction (05 §3.3): create the table if missing, declare
// every set (an identical existing set keeps its elements), add + flush + refill every chain, and replace the
// elements of the plan-owned sets. Dynamic and meter sets are never flushed.
func Render(s Spec) ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	var cmds []cmd
	cmds = append(cmds, add(obj{"table": table{Family: "inet", Name: TableName}}))
	for _, st := range sets(s) {
		cmds = append(cmds, add(obj{"set": st}))
	}
	chains := chains(s)
	for _, c := range chains {
		cmds = append(cmds, add(obj{"chain": c.def}), cmd{"flush": obj{"chain": ref{Family: "inet", Table: TableName, Name: c.def.Name}}})
	}
	for _, c := range chains {
		for _, r := range c.rules {
			cmds = append(cmds, add(obj{"rule": rule{Family: "inet", Table: TableName, Chain: c.def.Name, Expr: r}}))
		}
	}
	for _, name := range planSets {
		cmds = append(cmds, cmd{"flush": obj{"set": ref{Family: "inet", Table: TableName, Name: name}}})
		if el := elements(s, name); len(el) > 0 {
			cmds = append(cmds, add(obj{"element": element{Family: "inet", Table: TableName, Name: name, Elem: el}}))
		}
	}
	return json.MarshalIndent(batch{Nftables: cmds}, "", " ")
}

// ---- libnftables JSON objects (shapes from testdata/contracts/nft/) ----

type (
	obj   map[string]any
	cmd   map[string]any
	batch struct {
		Nftables []cmd `json:"nftables"`
	}
	table struct {
		Family string `json:"family"`
		Name   string `json:"name"`
	}
	ref struct {
		Family string `json:"family"`
		Table  string `json:"table"`
		Name   string `json:"name"`
	}
	set struct {
		Family    string   `json:"family"`
		Table     string   `json:"table"`
		Name      string   `json:"name"`
		Type      string   `json:"type"`
		Flags     []string `json:"flags,omitempty"`
		Timeout   int      `json:"timeout,omitempty"`
		Size      int      `json:"size,omitempty"`
		AutoMerge bool     `json:"auto-merge,omitempty"`
	}
	chain struct {
		Family string `json:"family"`
		Table  string `json:"table"`
		Name   string `json:"name"`
		Type   string `json:"type"`
		Hook   string `json:"hook"`
		Prio   int    `json:"prio"`
		Policy string `json:"policy"`
	}
	rule struct {
		Family string `json:"family"`
		Table  string `json:"table"`
		Chain  string `json:"chain"`
		Expr   []any  `json:"expr"`
	}
	element struct {
		Family string `json:"family"`
		Table  string `json:"table"`
		Name   string `json:"name"`
		Elem   []any  `json:"elem"`
	}
)

func add(o obj) cmd { return cmd{"add": o} }

func sets(s Spec) []set {
	ival := func(name, typ string) set {
		return set{Family: "inet", Table: TableName, Name: name, Type: typ, Flags: []string{"interval"}, AutoMerge: true}
	}
	dynamic := func(name, typ string, timeout time.Duration) set {
		st := set{Family: "inet", Table: TableName, Name: name, Type: typ, Size: 65535, Flags: []string{"dynamic"}}
		if timeout > 0 {
			st.Flags, st.Timeout = []string{"timeout", "dynamic"}, int(timeout.Seconds())
		}
		return st
	}
	staging := s.Meters.StagingTimeout
	if staging <= 0 {
		staging = DefaultMeters.StagingTimeout
	}
	svc := func(name string) set { return set{Family: "inet", Table: TableName, Name: name, Type: "inet_service"} }
	return []set{
		ival("allow4", "ipv4_addr"), ival("allow6", "ipv6_addr"), ival("block4", "ipv4_addr"), ival("block6", "ipv6_addr"),
		{Family: "inet", Table: TableName, Name: "dyn4", Type: "ipv4_addr", Flags: []string{"timeout"}},
		{Family: "inet", Table: TableName, Name: "dyn6", Type: "ipv6_addr", Flags: []string{"interval", "timeout"}},
		dynamic("staging4", "ipv4_addr", staging), dynamic("staging6", "ipv6_addr", staging),
		ival("host_access4", "ipv4_addr"), ival("host_access6", "ipv6_addr"),
		svc("pub_tcp"), svc("pub_udp"), svc("metered_udp"),
		dynamic("m_tcp_new4", "ipv4_addr", time.Minute), dynamic("m_tcp_new6", "ipv6_addr", time.Minute),
		dynamic("m_tcp_conc4", "ipv4_addr", 0), dynamic("m_tcp_conc6", "ipv6_addr", 0),
		dynamic("m_udp_new4", "ipv4_addr", time.Minute), dynamic("m_udp_new6", "ipv6_addr", time.Minute),
		dynamic("m_udp_pps4", "ipv4_addr", time.Minute), dynamic("m_udp_pps6", "ipv6_addr", time.Minute),
	}
}

func elements(s Spec, name string) []any {
	prefixes := func(ps []netip.Prefix, v4 bool) []any {
		var out []any
		for _, p := range ps {
			if p.Addr().Is4() != v4 {
				continue
			}
			out = append(out, prefixVal(p))
		}
		return out
	}
	ports := func(proto string, metered bool) []any {
		var ns []int
		for _, p := range s.Ports {
			if p.Proto == proto && (!metered || p.Metered && (p.Proto != "udp" || p.Port != 443)) {
				ns = append(ns, p.Port)
			}
		}
		slices.Sort(ns)
		out := make([]any, len(ns))
		for i, n := range ns {
			out[i] = n
		}
		return out
	}
	switch name {
	case "allow4":
		return prefixes(s.Folded.Allow4, true)
	case "allow6":
		return prefixes(s.Folded.Allow6, false)
	case "block4":
		return prefixes(s.Folded.Block4, true)
	case "block6":
		return prefixes(s.Folded.Block6, false)
	case "host_access4":
		return prefixes(s.HostAccess, true)
	case "host_access6":
		return prefixes(s.HostAccess, false)
	case "pub_tcp":
		return ports("tcp", false)
	case "pub_udp":
		return ports("udp", false)
	case "metered_udp":
		return ports("udp", true)
	}
	return nil
}

// prefixVal is a set element: a bare address for a single-address prefix, else {"prefix": …} (as nft lists them).
func prefixVal(p netip.Prefix) any {
	if p.IsSingleIP() {
		return p.Addr().String()
	}
	return obj{"prefix": obj{"addr": p.Addr().String(), "len": p.Bits()}}
}

// ---- expressions ----

func match(op string, left, right any) any {
	return obj{"match": obj{"op": op, "left": left, "right": right}}
}
func meta(key string) any              { return obj{"meta": obj{"key": key}} }
func payload(proto, field string) any  { return obj{"payload": obj{"protocol": proto, "field": field}} }
func ct(key string) any                { return obj{"ct": obj{"key": key}} }
func verdict(v string) any             { return obj{v: nil} }
func saddr(v4 bool) any                { return payload(fam(v4), "saddr") }
func setRef(name string) string        { return "@" + name }
func iif(name string) any              { return match("==", meta("iifname"), name) }
func notIif(name string) any           { return match("!=", meta("iifname"), name) }
func dport(proto string, port any) any { return match("==", payload(proto, "dport"), port) }
func inSet(v4 bool, name string) any   { return match("==", saddr(v4), setRef(name)) }
func ctState(states ...string) any {
	if len(states) == 1 {
		return match("in", ct("state"), states[0])
	}
	return match("in", ct("state"), states)
}

func fam(v4 bool) string {
	if v4 {
		return "ip"
	}
	return "ip6"
}

func family(p netip.Prefix) (proto string, v4 bool) {
	return fam(p.Addr().Is4()), p.Addr().Is4()
}

type chainOut struct {
	def   chain
	rules [][]any
}

func base(name, typ, hook string, prio int) chain {
	return chain{Family: "inet", Table: TableName, Name: name, Type: typ, Hook: hook, Prio: prio, Policy: "accept"}
}

func chains(s Spec) []chainOut {
	nb := s.NetBird.Mode != NetBirdDisabled
	both := []bool{true, false}

	// 1. pre_guard: only WAN traffic; NetBird's WireGuard port first, manual allow before every drop (invariant 20).
	guard := chainOut{def: base("pre_guard", "filter", "prerouting", -300)}
	guard.rules = append(guard.rules, []any{notIif(s.WAN), verdict("return")})
	if nb {
		guard.rules = append(guard.rules, []any{dport("udp", s.NetBird.Port), verdict("return")})
	}
	for _, v4 := range both {
		guard.rules = append(guard.rules, []any{inSet(v4, "allow"+suffix(v4)), verdict("return")})
	}
	for _, name := range []string{"block", "dyn", "staging"} {
		for _, v4 := range both {
			guard.rules = append(guard.rules, []any{inSet(v4, name+suffix(v4)), verdict("drop")})
		}
	}

	// 2. pre_meter: after conntrack, before DNAT; allowed sources are never metered.
	meter := chainOut{def: base("pre_meter", "filter", "prerouting", -150)}
	meter.rules = append(meter.rules, []any{notIif(s.WAN), verdict("return")})
	for _, v4 := range both {
		meter.rules = append(meter.rules, []any{inSet(v4, "allow"+suffix(v4)), verdict("return")})
	}
	if m := s.Meters; m.Enabled {
		limit := func(rate, burst int, per string) any {
			return obj{"limit": obj{"rate": rate, "burst": burst, "per": per, "inv": true}}
		}
		update := func(op, set string, v4 bool, stmt any) any {
			return obj{"set": obj{"op": op, "elem": saddr(v4), "set": setRef(set + suffix(v4)), "stmt": []any{stmt}}}
		}
		stage := func(v4 bool) any {
			return obj{"set": obj{"op": "add", "elem": saddr(v4), "set": setRef("staging" + suffix(v4))}}
		}
		for _, v4 := range both {
			meter.rules = append(meter.rules,
				[]any{dport("tcp", setRef("pub_tcp")), match("in", payload("tcp", "flags"), "syn"), ctState("new"),
					update("update", "m_tcp_new", v4, limit(m.TCPNewPerMinute, m.TCPBurst, "minute")), stage(v4), verdict("drop")},
				[]any{dport("tcp", setRef("pub_tcp")), ctState("new"),
					update("add", "m_tcp_conc", v4, obj{"ct count": obj{"val": m.TCPMaxConcurrent, "inv": true}}), stage(v4), verdict("drop")},
				[]any{dport("udp", setRef("metered_udp")), ctState("new"),
					update("update", "m_udp_new", v4, limit(m.UDPNewPerMinute, m.UDPBurst, "minute")), stage(v4), verdict("drop")},
				[]any{dport("udp", setRef("metered_udp")),
					update("update", "m_udp_pps", v4, limit(m.UDPPerSecond, m.UDPPPSBurst, "second")), stage(v4), verdict("drop")})
		}
	}

	// 3. nat_pre: every published port to Traefik's entry point; internal clients reaching the public address
	// (hairpin) get the same DNAT.
	nat := chainOut{def: base("nat_pre", "nat", "prerouting", -100)}
	for _, p := range sortedPorts(s.Ports) {
		d := obj{"dnat": obj{"family": "ip", "addr": s.TraefikIP.String(), "port": p.TargetPort}}
		nat.rules = append(nat.rules, []any{iif(s.WAN), dport(p.Proto, p.Port), d})
		if s.PublicIP.Is4() {
			nat.rules = append(nat.rules, []any{notIif(s.WAN), match("==", payload("ip", "daddr"), s.PublicIP.String()),
				dport(p.Proto, p.Port), d})
		}
	}

	// 3b. nat_out: the host itself reaching its public address on a published port, as other machines do. NetBird's
	// management server can live behind the same edge (found on the first real host: no reconnect without this).
	natOut := chainOut{def: base("nat_out", "nat", "output", -100)}
	if s.PublicIP.Is4() {
		for _, p := range sortedPorts(s.Ports) {
			natOut.rules = append(natOut.rules, []any{match("==", payload("ip", "daddr"), s.PublicIP.String()), dport(p.Proto, p.Port),
				obj{"dnat": obj{"family": "ip", "addr": s.TraefikIP.String(), "port": p.TargetPort}}})
		}
	}

	// 4. input: drops only after the user accepted "block all other inbound".
	input := chainOut{def: base("input", "filter", "input", 0)}
	input.rules = append(input.rules, []any{iif("lo"), verdict("accept")}, []any{ctState("established", "related"), verdict("accept")})
	for _, n := range s.Subnets {
		proto, _ := family(n)
		input.rules = append(input.rules, []any{match("==", payload(proto, "saddr"), prefixVal(n)), verdict("accept")})
	}
	if nb {
		input.rules = append(input.rules, []any{iif(s.NetBird.Iface), verdict("accept")},
			[]any{dport("udp", s.NetBird.Port), verdict("accept")})
	}
	for _, v4 := range both {
		input.rules = append(input.rules, []any{inSet(v4, "host_access"+suffix(v4)), verdict("accept")})
	}
	if s.DropOtherInbound {
		input.rules = append(input.rules, []any{ctState("new"), verdict("drop")})
	}

	// 5. forward: DNATed flows, managed-subnet egress, replies.
	fwd := chainOut{def: base("forward", "filter", "forward", 0)}
	fwd.rules = append(fwd.rules, []any{match("in", ct("status"), "dnat"), verdict("accept")})
	for _, n := range s.Subnets {
		proto, _ := family(n)
		fwd.rules = append(fwd.rules, []any{match("==", payload(proto, "saddr"), prefixVal(n)),
			match("==", meta("oifname"), s.WAN), verdict("accept")})
	}
	fwd.rules = append(fwd.rules, []any{ctState("established", "related"), verdict("accept")})

	// 6. nat_post: subnets → WAN masquerade; hairpin replies come back through the host. NetBird's own routing-peer
	// rules masquerade traffic into and out of the overlay (testdata/contracts/firewall/), so Hashi adds nothing there.
	post := chainOut{def: base("nat_post", "nat", "postrouting", 100)}
	for _, n := range s.Subnets {
		if !n.Addr().Is4() {
			continue
		}
		post.rules = append(post.rules, []any{match("==", payload("ip", "saddr"), prefixVal(n)),
			match("==", meta("oifname"), s.WAN), obj{"masquerade": nil}})
		if s.TraefikIP.Is4() && len(s.Ports) > 0 {
			post.rules = append(post.rules, []any{match("==", payload("ip", "saddr"), prefixVal(n)),
				match("==", payload("ip", "daddr"), s.TraefikIP.String()), match("in", ct("status"), "dnat"), obj{"masquerade": nil}})
		}
	}

	// 7. mangle_fwd: MSS clamp on NetBird-routed traffic.
	mangle := chainOut{def: base("mangle_fwd", "filter", "forward", -150)}
	if s.NetBird.Mode == NetBirdRouting {
		clamp := obj{"mangle": obj{"key": obj{"tcp option": obj{"name": "maxseg", "field": "size"}}, "value": obj{"rt": obj{"key": "mtu"}}}}
		syn := match("in", payload("tcp", "flags"), "syn")
		mangle.rules = append(mangle.rules, []any{iif(s.NetBird.Iface), syn, clamp},
			[]any{match("==", meta("oifname"), s.NetBird.Iface), syn, clamp})
	}
	return []chainOut{guard, meter, nat, natOut, input, fwd, post, mangle}
}

func suffix(v4 bool) string {
	if v4 {
		return "4"
	}
	return "6"
}

func sortedPorts(ps []Port) []Port {
	out := slices.Clone(ps)
	slices.SortFunc(out, func(a, b Port) int {
		if a.Proto != b.Proto {
			if a.Proto < b.Proto {
				return -1
			}
			return 1
		}
		return a.Port - b.Port
	})
	return out
}
