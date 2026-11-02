// Package security holds the abuse engine's state (06 §3, §5): the ban function, the step-decaying counters, the
// dynamic entries and their escalation. Forward auth (internal/edge) and the host link (internal/firewall) call it.
package security

import (
	"math"
	"net/netip"
	"time"

	"git.juzo.io/juzo/hashi/internal/settings"
)

// BanParams are the ban function's settings (06 §5.2).
type BanParams struct {
	Function       string
	Base, Max      time.Duration
	K              float64
	PermanentAfter int // 0: never permanent
}

// BanParamsFrom reads them from the engine settings.
func BanParamsFrom(e settings.Engine) BanParams {
	return BanParams{Function: e.BanFunction, Base: time.Duration(e.BanBaseSeconds) * time.Second,
		Max: time.Duration(e.BanMaxSeconds) * time.Second, K: e.BanK, PermanentAfter: e.BanPermanentAfter}
}

// Duration is T_ban(c) for the c-th ban (c ≥ 1). permanent reports T = ∞.
func (p BanParams) Duration(c int) (d time.Duration, permanent bool) {
	if p.PermanentAfter > 0 && c >= p.PermanentAfter {
		return 0, true
	}
	if c < 1 {
		c = 1
	}
	f := 1.0
	switch p.Function {
	case settings.BanLinear:
		f = 1 + p.K*float64(c-1)
	case settings.BanExponential:
		f = math.Pow(p.K, float64(c-1))
	}
	// Compare in float first: k^(c−1) overflows a Duration long before the cap stops it.
	if x := float64(p.Base) * f; x < float64(p.Max) {
		return time.Duration(x), false
	}
	return p.Max, false
}

// Effective is a counter's value after step decay: one less per full quiet period d since since (06 §5.1).
func Effective(n int, since, now time.Time, d time.Duration) int {
	if n <= 0 || !now.After(since) || d <= 0 {
		return max(n, 0)
	}
	return max(0, n-int(now.Sub(since)/d))
}

// NextDecrement is when the effective value drops next; zero when it is 0 or the quiet time has not started.
func NextDecrement(n int, since, now time.Time, d time.Duration) time.Time {
	e := Effective(n, since, now, d)
	if e == 0 || d <= 0 {
		return time.Time{}
	}
	if !now.After(since) {
		return since.Add(d)
	}
	steps := now.Sub(since)/d + 1
	return since.Add(steps * d)
}

// Key is the address a counter and a dynamic entry belong to: the address itself for IPv4, its /64 for IPv6 (06 §3.2).
func Key(a netip.Addr) netip.Prefix {
	a = a.Unmap()
	if a.Is4() {
		return netip.PrefixFrom(a, 32)
	}
	p, _ := a.Prefix(64)
	return p
}

// KeyString is Key in its stored form: a bare IPv4 address, or the IPv6 /64 in CIDR form.
func KeyString(a netip.Addr) string {
	k := Key(a)
	if k.Addr().Is4() {
		return k.Addr().String()
	}
	return k.String()
}
