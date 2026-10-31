package ipsets

import (
	"net/netip"
	"sort"
	"strings"
	"time"
)

// Describe is a short human form of an item, as the address page shows it ("192.0.2.0/24 (cidr)").
func Describe(g Graph, it Item) string {
	switch v := it.(type) {
	case IP:
		return v.Addr.String() + " (ip)"
	case CIDR:
		return v.Prefix.String() + " (cidr)"
	case Host:
		return "host " + v.HostID + " (" + v.Scope + ")"
	case Pulse:
		name := v.Name
		if name == "" {
			name = v.AgentID
		}
		return name + " (pulse, " + v.Scope + ")"
	case DNS:
		return v.FQDN + " (dns)"
	case SetRef:
		if s, ok := g[v.SetID]; ok {
			return s.Slug + " (set)"
		}
		return v.SetID + " (set)"
	}
	return ""
}

// Match is one way an address is inside a set: the nesting path of slugs from the set to the element that holds it.
type Match struct {
	Path      []string
	SetID     string // the set the element belongs to
	ElementID string
	Item      Item
}

// Explain lists every leaf element that contains a, through nesting, for the set id (05a §6). Expired elements are
// left out, like in Compile.
func (g Graph) Explain(id string, a netip.Addr, r Resolver, now time.Time) []Match {
	a = a.Unmap()
	var out []Match
	var walk func(id string, path []string, seen map[string]bool)
	walk = func(id string, path []string, seen map[string]bool) {
		s, ok := g[id]
		if !ok || seen[id] {
			return
		}
		seen[id] = true
		defer delete(seen, id)
		path = append(append([]string(nil), path...), s.Slug)
		for _, e := range s.Elements {
			if e.Expired(now) {
				continue
			}
			if ref, ok := e.Item.(SetRef); ok {
				walk(ref.SetID, path, seen)
				continue
			}
			if leafContains(e.Item, a, r) {
				out = append(out, Match{Path: path, SetID: s.ID, ElementID: e.ID, Item: e.Item})
			}
		}
	}
	walk(id, nil, map[string]bool{})
	return out
}

func leafContains(it Item, a netip.Addr, r Resolver) bool {
	switch v := it.(type) {
	case IP:
		return v.Addr == a
	case CIDR:
		return v.Prefix.Contains(a)
	case Host, Pulse, DNS:
		if r == nil {
			return false
		}
		var addrs []netip.Addr
		switch x := v.(type) {
		case Host:
			addrs, _ = r.Host(x.HostID, x.Scope)
		case Pulse:
			addrs, _ = r.Pulse(x.AgentID, x.Scope, x.Selector)
		case DNS:
			addrs, _ = r.DNS(x.FQDN)
		}
		for _, x := range addrs {
			if x.Unmap() == a {
				return true
			}
		}
	}
	return false
}

// Search filters a set's own elements (05a §6): an address finds the elements that contain it (a set reference
// when the referenced set does), a prefix finds overlapping elements, anything else matches note, name or set slug
// text. An empty query returns every element.
func (g Graph) Search(id, q string, r Resolver, now time.Time) []Element {
	s, ok := g[id]
	if !ok {
		return nil
	}
	q = strings.TrimSpace(q)
	if q == "" {
		return s.Elements
	}
	var keep func(Element) bool
	if it, err := ParseIPOrCIDR(q); err == nil {
		var pre netip.Prefix
		switch v := it.(type) {
		case IP:
			pre = netip.PrefixFrom(v.Addr, v.Addr.BitLen())
		case CIDR:
			pre = v.Prefix
		}
		keep = func(e Element) bool {
			switch v := e.Item.(type) {
			case IP:
				return pre.Contains(v.Addr)
			case CIDR:
				return v.Prefix.Overlaps(pre)
			case SetRef:
				res := g.Compile(v.SetID, r, now)
				for _, p := range append(res.V4, res.V6...) {
					if p.Overlaps(pre) {
						return true
					}
				}
				return false
			default:
				return pre.IsSingleIP() && leafContains(e.Item, pre.Addr(), r)
			}
		}
	} else {
		low := strings.ToLower(q)
		keep = func(e Element) bool {
			return strings.Contains(strings.ToLower(e.Note), low) || strings.Contains(strings.ToLower(Describe(g, e.Item)), low)
		}
	}
	var out []Element
	for _, e := range s.Elements {
		if keep(e) {
			out = append(out, e)
		}
	}
	return out
}

// Containing returns the IDs of every set whose compiled prefixes contain a, sorted by slug.
func (g Graph) Containing(a netip.Addr, r Resolver, now time.Time) []string {
	var ids []string
	for id := range g {
		if g.Compile(id, r, now).Contains(a) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return g[ids[i]].Slug < g[ids[j]].Slug })
	return ids
}
