// Package clientip is the one place that decides a request's client IP (01-platform.md §8). Bootstrap restriction,
// admin session binding, forward auth, rate limiting and Pulse all use it, so issuance and validation always agree.
package clientip

import (
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
)

// ErrUnresolvable means no canonical client IP could be determined; callers fail closed.
var ErrUnresolvable = errors.New("client IP cannot be determined")

// Resolver honours forwarding headers only when the direct peer is a trusted proxy.
type Resolver struct {
	trusted atomic.Pointer[[]netip.Prefix]
}

// New returns a resolver trusting the given proxy prefixes.
func New(trusted []netip.Prefix) *Resolver {
	r := &Resolver{}
	r.SetTrusted(trusted)
	return r
}

// SetTrusted replaces the trusted-proxy list (the setting changed). Safe for concurrent use.
func (r *Resolver) SetTrusted(p []netip.Prefix) {
	cp := make([]netip.Prefix, len(p))
	for i, x := range p {
		cp[i] = x.Masked()
	}
	r.trusted.Store(&cp)
}

func (r *Resolver) isTrusted(a netip.Addr) bool {
	for _, p := range *r.trusted.Load() {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// Canonical parses an address the way every comparison in Hashi expects: IPv4-mapped IPv6 becomes IPv4,
// zones are stripped, brackets and ports are removed.
func Canonical(s string) (netip.Addr, bool) {
	s = strings.TrimSpace(s)
	if host, _, err := net.SplitHostPort(s); err == nil {
		s = host
	}
	s = strings.TrimSuffix(strings.TrimPrefix(s, "["), "]")
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, false
	}
	return a.Unmap().WithZone(""), true
}

// Resolve returns the canonical client IP of r.
//
// Without a trusted direct peer the answer is the peer itself and every header is ignored. With one, the
// forwarding chain (X-Forwarded-For, else Forwarded, else X-Real-IP) is walked right to left and the first
// hop that is not a trusted proxy is the client. Anything unparseable on that walk fails closed.
func (r *Resolver) Resolve(req *http.Request) (netip.Addr, error) {
	peer, ok := Canonical(req.RemoteAddr)
	if !ok {
		return netip.Addr{}, ErrUnresolvable
	}
	if !r.isTrusted(peer) {
		return peer, nil
	}
	chain, err := forwardedChain(req.Header)
	if err != nil {
		return netip.Addr{}, err
	}
	for i := len(chain) - 1; i >= 0; i-- {
		a, ok := Canonical(chain[i])
		if !ok {
			return netip.Addr{}, ErrUnresolvable
		}
		if !r.isTrusted(a) {
			return a, nil
		}
	}
	// Every hop is a trusted proxy (or there are no headers): the leftmost known hop is the client.
	if len(chain) > 0 {
		a, _ := Canonical(chain[0])
		return a, nil
	}
	return peer, nil
}

// forwardedChain returns the hops a trusted proxy reported, oldest first.
func forwardedChain(h http.Header) ([]string, error) {
	if v := h.Values("X-Forwarded-For"); len(v) > 0 {
		return splitList(v), nil
	}
	if v := h.Values("Forwarded"); len(v) > 0 {
		var out []string
		for _, elem := range splitList(v) {
			for pair := range strings.SplitSeq(elem, ";") {
				k, val, ok := strings.Cut(strings.TrimSpace(pair), "=")
				if ok && strings.EqualFold(k, "for") {
					out = append(out, strings.Trim(val, `"`))
				}
			}
		}
		if len(out) == 0 {
			return nil, ErrUnresolvable
		}
		return out, nil
	}
	if v := h.Get("X-Real-IP"); v != "" {
		return []string{v}, nil
	}
	return nil, nil
}

func splitList(values []string) []string {
	var out []string
	for _, v := range values {
		for p := range strings.SplitSeq(v, ",") {
			out = append(out, strings.TrimSpace(p))
		}
	}
	return out
}

// ParsePrefixes parses CIDRs or single addresses (as /32 or /128).
func ParsePrefixes(in []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if p, err := netip.ParsePrefix(s); err == nil {
			if p.Addr().Is4In6() {
				p = netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
			}
			out = append(out, p.Masked())
			continue
		}
		a, ok := Canonical(s)
		if !ok {
			return nil, errors.New("not an IP address or CIDR: " + s)
		}
		out = append(out, netip.PrefixFrom(a, a.BitLen()))
	}
	return out, nil
}

// InAny reports whether a lies in any prefix.
func InAny(a netip.Addr, ps []netip.Prefix) bool {
	for _, p := range ps {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
