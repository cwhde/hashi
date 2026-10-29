// Package firewall manages Linux firewall hosts (05-firewall.md): Hashi's own nftables table, the folded global
// allow/block sets, ports and the rescue rollback.
package firewall

import (
	"net/netip"
	"sort"

	"go4.org/netipx"
)

// Verdicts of folding (05 §4.2).
const (
	Neutral = "neutral"
	Allowed = "allowed"
	Blocked = "blocked"
)

// Folded is the partition the kernel's allow and block sets receive; everything else is neutral.
type Folded struct {
	Allow4, Allow6, Block4, Block6 []netip.Prefix
}

// Fold resolves combined manual allow against block by longest prefix match, tie → block (05 §4.2). Inputs may mix
// families and overlap; the output is a minimal, sorted prefix list per verdict and family. Prefixes always nest or
// are disjoint, so a sweep over the interval boundaries with one stack per side finds the longest containing prefix
// of each side in O(n log n).
func Fold(allow, block []netip.Prefix) Folded {
	var f Folded
	f.Allow4, f.Block4 = foldFamily(only(allow, true), only(block, true))
	f.Allow6, f.Block6 = foldFamily(only(allow, false), only(block, false))
	return f
}

// Verdict answers one address with the deciding prefixes (invalid when a side has none); the address page and the
// tests use it.
func Verdict(allow, block []netip.Prefix, a netip.Addr) (verdict string, byAllow, byBlock netip.Prefix) {
	a = a.Unmap()
	byAllow, byBlock = longest(allow, a), longest(block, a)
	switch {
	case !byAllow.IsValid() && !byBlock.IsValid():
		return Neutral, byAllow, byBlock
	case !byBlock.IsValid():
		return Allowed, byAllow, byBlock
	case !byAllow.IsValid():
		return Blocked, byAllow, byBlock
	case byAllow.Bits() > byBlock.Bits():
		return Allowed, byAllow, byBlock
	}
	return Blocked, byAllow, byBlock
}

func longest(ps []netip.Prefix, a netip.Addr) netip.Prefix {
	var best netip.Prefix
	for _, p := range ps {
		p = p.Masked()
		if p.Contains(a) && (!best.IsValid() || p.Bits() > best.Bits()) {
			best = p
		}
	}
	return best
}

func only(ps []netip.Prefix, v4 bool) []netip.Prefix {
	var out []netip.Prefix
	for _, p := range ps {
		if !p.IsValid() {
			continue
		}
		if p.Addr().Is4In6() {
			p = netip.PrefixFrom(p.Addr().Unmap(), max(p.Bits()-96, 0))
		}
		if p.Addr().Is4() == v4 {
			out = append(out, p.Masked())
		}
	}
	return out
}

// side walks one list's prefixes in boundary order and keeps the chain of prefixes containing the current point.
type side struct {
	ps    []netip.Prefix // sorted by start, then shorter first (the outer one is pushed first)
	next  int
	stack []netip.Prefix
}

func newSide(ps []netip.Prefix) *side {
	sort.Slice(ps, func(i, j int) bool {
		if c := ps[i].Addr().Compare(ps[j].Addr()); c != 0 {
			return c < 0
		}
		return ps[i].Bits() < ps[j].Bits()
	})
	return &side{ps: ps}
}

// at moves to point x and returns the bits of the longest prefix containing it, or -1.
func (s *side) at(x netip.Addr) int {
	for len(s.stack) > 0 && netipx.PrefixLastIP(s.stack[len(s.stack)-1]).Less(x) {
		s.stack = s.stack[:len(s.stack)-1]
	}
	for s.next < len(s.ps) && !x.Less(s.ps[s.next].Addr()) {
		p := s.ps[s.next]
		s.next++
		if netipx.PrefixLastIP(p).Less(x) {
			continue // ended before x (only possible for duplicates skipped between boundaries)
		}
		for len(s.stack) > 0 && netipx.PrefixLastIP(s.stack[len(s.stack)-1]).Less(x) {
			s.stack = s.stack[:len(s.stack)-1]
		}
		s.stack = append(s.stack, p)
	}
	if len(s.stack) == 0 {
		return -1
	}
	return s.stack[len(s.stack)-1].Bits()
}

func foldFamily(allow, block []netip.Prefix) (allowed, blocked []netip.Prefix) {
	if len(allow)+len(block) == 0 {
		return nil, nil
	}
	// Boundaries: every start and every address after an end. The verdict is constant between two boundaries.
	var points []netip.Addr
	for _, p := range append(append([]netip.Prefix(nil), allow...), block...) {
		points = append(points, p.Addr())
		if n := netipx.PrefixLastIP(p).Next(); n.IsValid() {
			points = append(points, n)
		}
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Less(points[j]) })
	points = dedupAddrs(points)

	a, b := newSide(allow), newSide(block)
	var ab, bb netipx.IPSetBuilder
	for i, x := range points {
		last := maxAddr(x)
		if i+1 < len(points) {
			last = points[i+1].Prev()
		}
		la, lb := a.at(x), b.at(x)
		r := netipx.IPRangeFrom(x, last)
		switch {
		case la < 0 && lb < 0:
		case lb < 0 || la > lb:
			ab.AddRange(r)
		default:
			bb.AddRange(r)
		}
	}
	as, _ := ab.IPSet() // only invalid ranges error; boundaries are valid by construction
	bs, _ := bb.IPSet()
	return as.Prefixes(), bs.Prefixes()
}

func dedupAddrs(in []netip.Addr) []netip.Addr {
	out := in[:0]
	for i, a := range in {
		if i == 0 || a != in[i-1] {
			out = append(out, a)
		}
	}
	return out
}

func maxAddr(x netip.Addr) netip.Addr {
	if x.Is4() {
		return netip.AddrFrom4([4]byte{255, 255, 255, 255})
	}
	var b [16]byte
	for i := range b {
		b[i] = 0xff
	}
	return netip.AddrFrom16(b)
}
