package firewall

import (
	"context"
	"git.juzo.io/juzo/hashi/internal/adguard"
	"net/netip"
	"slices"
	"strings"

	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
)

// Records is the DNS generator (03 §5.1–5.2): host records for every firewall host with a public IPv4 address and a
// record for every renderable service. Service targets resolve through resolve (address sets, invariant 21).
func (m *Manager) Records(resolve services.TargetResolver) dns.Generator {
	return func(ctx context.Context, _ string) ([]dns.Generated, error) {
		g, err := settings.Get[settings.General](ctx, m.st.R)
		if err != nil || g.RootDomain == "" {
			return nil, err
		}
		rows, err := m.st.R.ListFirewallHosts(ctx)
		if err != nil {
			return nil, err
		}
		hosts := make([]Effective, 0, len(rows))
		for _, r := range rows {
			hosts = append(hosts, EffectiveOf(r))
		}
		svcs, err := services.List(ctx, m.st.R)
		if err != nil {
			return nil, err
		}
		if resolve != nil {
			if svcs, err = services.Resolve(ctx, svcs, resolve); err != nil {
				return nil, err
			}
		}
		return GenerateRecords(g.RootDomain, hosts, svcs), nil
	}
}

// Rewrites lists the internal DNS rewrites services want (03 §8): each service behind a firewall host answers with that
// host's internal Traefik address. The routing override is public-only and never applies here.
func (m *Manager) Rewrites(resolve services.TargetResolver) func(ctx context.Context) ([]adguard.Desired, error) {
	return func(ctx context.Context) ([]adguard.Desired, error) {
		g, err := settings.Get[settings.General](ctx, m.st.R)
		if err != nil || g.RootDomain == "" {
			return nil, err
		}
		rows, err := m.st.R.ListFirewallHosts(ctx)
		if err != nil {
			return nil, err
		}
		hosts := make([]Effective, 0, len(rows))
		for _, r := range rows {
			hosts = append(hosts, EffectiveOf(r))
		}
		svcs, err := services.List(ctx, m.st.R)
		if err != nil {
			return nil, err
		}
		if svcs, err = services.Resolve(ctx, svcs, resolve); err != nil {
			return nil, err
		}
		return DesiredRewrites(strings.TrimSuffix(strings.ToLower(g.RootDomain), "."), hosts, svcs), nil
	}
}

// DesiredRewrites derives rewrites from hosts and services whose targets are resolved. A host without a known internal
// Traefik address yields none: there is nothing true to answer with.
func DesiredRewrites(root string, hosts []Effective, svcs []services.Service) []adguard.Desired {
	var out []adguard.Desired
	for _, s := range svcs {
		if !s.Renderable() || !s.AdGuardRewrite || s.HostName == "" || s.Stream() {
			continue
		}
		home, ok := homeHost(s, hosts, root)
		if !ok {
			continue
		}
		if a, err := netip.ParseAddr(home.Row.InternalTraefikIp); err == nil {
			out = append(out, adguard.Desired{Domain: strings.TrimSuffix(s.HostName, "."), Answer: a.String(), Origin: "service " + s.ID})
		}
	}
	return out
}

// GenerateRecords derives the records from hosts and services whose address-set targets are already resolved
// (services.Resolve). Generated records are IPv4 only (03 §4).
func GenerateRecords(root string, hosts []Effective, svcs []services.Service) []dns.Generated {
	root = strings.TrimSuffix(strings.ToLower(root), ".")
	fq := func(labels ...string) string { return strings.Join(append(labels, root), ".") + "." }
	byID := make(map[string]Effective, len(hosts))
	for _, h := range hosts {
		byID[h.Row.ID] = h
	}
	// entry is the host whose via. record on.<h> points at: the host override, else the host itself. An override to
	// a host without a public IPv4 address (not validated yet) would leave on.<h> dangling, so it waits.
	entry := func(h Effective) Effective {
		if v, ok := byID[h.Row.RouteViaHostID.String]; ok && h.Row.RouteViaHostID.Valid && v.PublicIP.Is4() {
			return v
		}
		return h
	}
	var out []dns.Generated
	for _, h := range hosts {
		if !h.PublicIP.Is4() {
			continue
		}
		src := "host " + h.Row.Name
		out = append(out,
			dns.Generated{Name: fq(h.Row.Name), Type: "A", Values: []string{h.PublicIP.String()}, Source: src},
			dns.Generated{Name: fq("via", h.Row.Name), Type: "CNAME", Values: []string{fq(h.Row.Name)}, Source: src},
			dns.Generated{Name: fq("on", h.Row.Name), Type: "CNAME", Values: []string{fq("via", entry(h).Row.Name)}, Source: src})
	}
	published := func(h Effective) bool { return entry(h).PublicIP.Is4() }
	hostNames := map[string]bool{}
	for _, g := range out {
		hostNames[g.Name] = true
	}
	for _, s := range svcs {
		name := strings.TrimSuffix(s.HostName, ".") + "."
		if !s.Renderable() || s.HostName == "" || hostNames[name] {
			continue // a service named like a host record leaves it to the host
		}
		src := "service " + s.Name
		home, matched := homeHost(s, hosts, root)
		if matched && !published(home) {
			continue // the host has no public address yet: nothing to point at
		}
		apex := strings.TrimSuffix(name, ".") == root
		switch {
		case matched:
			ent := entry(home)
			if o, ok := byID[s.EntryHostID]; ok && s.EntryHostID != "" && o.Row.ID != ent.Row.ID && o.PublicIP.Is4() {
				ent = o
				ovr := fq(ent.Row.Name, "ovr", "on", home.Row.Name)
				out = append(out, dns.Generated{Name: ovr, Type: "CNAME", Values: []string{fq("via", ent.Row.Name)}, Source: src})
				if !apex {
					out = append(out, dns.Generated{Name: name, Type: "CNAME", Values: []string{ovr}, Source: src})
				}
			} else if !apex {
				out = append(out, dns.Generated{Name: name, Type: "CNAME", Values: []string{fq("on", home.Row.Name)}, Source: src})
			}
			if apex { // no CNAME at the apex: flatten to the entry host's address
				out = append(out, dns.Generated{Name: name, Type: "A", Values: []string{ent.PublicIP.String()}, Source: src})
			}
		default:
			if vals := publicV4(s); len(vals) > 0 {
				out = append(out, dns.Generated{Name: name, Type: "A", Values: vals, Source: src})
			}
		}
	}
	return out
}

// homeHost finds the firewall host a service's upstreams sit behind (03 §5.2): the first target, in order, whose
// address lies in a host's managed subnets or is one of its addresses, or that names the host's own record.
func homeHost(s services.Service, hosts []Effective, root string) (Effective, bool) {
	for _, t := range resolvedTargets(s) {
		a, err := netip.ParseAddr(t.Address)
		for _, h := range hosts {
			if err != nil {
				if strings.TrimSuffix(t.Address, ".") == h.Row.Name+"."+root {
					return h, true
				}
				continue
			}
			a = a.Unmap()
			if a == h.PublicIP || a == h.PublicIP6 || slices.Contains(h.Internal, a) ||
				slices.ContainsFunc(h.Subnets, func(p netip.Prefix) bool { return p.Contains(a) }) {
				return h, true
			}
		}
	}
	return Effective{}, false
}

// publicV4 returns the public IPv4 addresses among a service's resolved targets (the A fallback, 03 §5.2).
func publicV4(s services.Service) []string {
	var out []string
	for _, t := range resolvedTargets(s) {
		a, err := netip.ParseAddr(t.Address)
		if err != nil || !a.Unmap().Is4() {
			continue
		}
		a = a.Unmap()
		if a.IsPrivate() || a.IsLoopback() || a.IsLinkLocalUnicast() || a.IsUnspecified() || cgnat.Contains(a) {
			continue
		}
		if v := a.String(); !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// resolvedTargets is the simple target (resolved when services.Resolve ran) followed by the enabled routes' targets.
func resolvedTargets(s services.Service) []services.Target {
	out := append([]services.Target(nil), s.Targets...)
	if out == nil {
		out = []services.Target{s.Target}
	}
	if s.AdvancedRoutes {
		for _, r := range s.Routes {
			if r.Enabled {
				out = append(out, r.Targets...)
			}
		}
	}
	return out
}
