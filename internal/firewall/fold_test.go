package firewall

import (
	"fmt"
	"math/rand/v2"
	"net/netip"
	"strings"
	"testing"
)

func pfx(ss ...string) []netip.Prefix {
	var out []netip.Prefix
	for _, s := range ss {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}

func str(ps []netip.Prefix) string {
	var out []string
	for _, p := range ps {
		out = append(out, p.String())
	}
	return strings.Join(out, " ")
}

// The cases of 05 §4.2.
func TestFoldCases(t *testing.T) {
	cases := []struct {
		name          string
		allow, block  []netip.Prefix
		wantA, wantB4 string
	}{
		{"nothing", nil, nil, "", ""},
		{"allow only", pfx("192.0.2.0/24"), nil, "192.0.2.0/24", ""},
		{"block only", nil, pfx("192.0.2.0/24"), "", "192.0.2.0/24"},
		{"allowed /32 inside a blocked /24", pfx("192.0.2.7/32"), pfx("192.0.2.0/24"),
			"192.0.2.7/32", "192.0.2.0/30 192.0.2.4/31 192.0.2.6/32 192.0.2.8/29 192.0.2.16/28 192.0.2.32/27 192.0.2.64/26 192.0.2.128/25"},
		{"blocked /24 inside an allowed /16", pfx("198.51.0.0/16"), pfx("198.51.100.0/24"),
			"198.51.0.0/18 198.51.64.0/19 198.51.96.0/22 198.51.101.0/24 198.51.102.0/23 198.51.104.0/21 198.51.112.0/20 198.51.128.0/17", "198.51.100.0/24"},
		{"tie blocks", pfx("192.0.2.0/24"), pfx("192.0.2.0/24"), "", "192.0.2.0/24"},
		{"allow inside block inside allow", pfx("10.0.0.0/8", "10.1.2.0/24"), pfx("10.1.0.0/16"),
			"10.0.0.0/16 10.1.2.0/24 10.2.0.0/15 10.4.0.0/14 10.8.0.0/13 10.16.0.0/12 10.32.0.0/11 10.64.0.0/10 10.128.0.0/9",
			"10.1.0.0/23 10.1.3.0/24 10.1.4.0/22 10.1.8.0/21 10.1.16.0/20 10.1.32.0/19 10.1.64.0/18 10.1.128.0/17"},
		{"disjoint and duplicates", pfx("192.0.2.0/25", "192.0.2.0/25"), pfx("192.0.2.128/25"), "192.0.2.0/25", "192.0.2.128/25"},
		{"everything allowed but a block", pfx("0.0.0.0/0"), pfx("255.255.255.255/32"), "0.0.0.0/1 128.0.0.0/2 192.0.0.0/3 224.0.0.0/4 240.0.0.0/5 248.0.0.0/6 252.0.0.0/7 254.0.0.0/8 255.0.0.0/9 255.128.0.0/10 255.192.0.0/11 255.224.0.0/12 255.240.0.0/13 255.248.0.0/14 255.252.0.0/15 255.254.0.0/16 255.255.0.0/17 255.255.128.0/18 255.255.192.0/19 255.255.224.0/20 255.255.240.0/21 255.255.248.0/22 255.255.252.0/23 255.255.254.0/24 255.255.255.0/25 255.255.255.128/26 255.255.255.192/27 255.255.255.224/28 255.255.255.240/29 255.255.255.248/30 255.255.255.252/31 255.255.255.254/32", "255.255.255.255/32"},
	}
	for _, c := range cases {
		f := Fold(c.allow, c.block)
		if str(f.Allow4) != c.wantA || str(f.Block4) != c.wantB4 {
			t.Errorf("%s:\n allow %s\n block %s", c.name, str(f.Allow4), str(f.Block4))
		}
	}
}

func TestFoldIPv6AndMapped(t *testing.T) {
	f := Fold(pfx("2001:db8:1::/48", "::ffff:192.0.2.7/128"), pfx("2001:db8::/32", "192.0.2.0/24"))
	if str(f.Allow6) != "2001:db8:1::/48" || !strings.HasPrefix(str(f.Block6), "2001:db8::/48 2001:db8:2::/47") {
		t.Fatalf("v6: allow %s block %s", str(f.Allow6), str(f.Block6))
	}
	if str(f.Allow4) != "192.0.2.7/32" {
		t.Fatalf("mapped address folds as IPv4: %s", str(f.Allow4))
	}
}

func TestVerdict(t *testing.T) {
	allow, block := pfx("192.0.2.7/32", "198.51.100.0/24"), pfx("192.0.2.0/24", "198.51.100.0/24")
	for addr, want := range map[string]string{
		"192.0.2.7": "allowed 192.0.2.7/32 192.0.2.0/24", "192.0.2.8": "blocked invalid Prefix 192.0.2.0/24",
		"198.51.100.1": "blocked 198.51.100.0/24 198.51.100.0/24", "203.0.113.1": "neutral invalid Prefix invalid Prefix",
	} {
		v, a, b := Verdict(allow, block, netip.MustParseAddr(addr))
		if got := fmt.Sprintf("%s %s %s", v, a, b); got != want {
			t.Errorf("%s: %s, want %s", addr, got, want)
		}
	}
}

// The output is a partition (allow and block never overlap, each sorted and disjoint) and agrees with the
// brute-force verdict on random addresses, over random nested and overlapping inputs.
func TestFoldProperty(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 11))
	rnd := func() []netip.Prefix {
		var out []netip.Prefix
		for range rng.IntN(8) {
			a := netip.AddrFrom4([4]byte{10, byte(rng.IntN(3)), byte(rng.IntN(256)), byte(rng.IntN(256))})
			out = append(out, netip.PrefixFrom(a, 12+rng.IntN(21)).Masked())
		}
		return out
	}
	in := func(ps []netip.Prefix, a netip.Addr) bool {
		for _, p := range ps {
			if p.Contains(a) {
				return true
			}
		}
		return false
	}
	for round := range 400 {
		allow, block := rnd(), rnd()
		f := Fold(allow, block)
		for _, list := range [][]netip.Prefix{f.Allow4, f.Block4} {
			for i := 1; i < len(list); i++ {
				if !list[i-1].Addr().Less(list[i].Addr()) || list[i-1].Overlaps(list[i]) {
					t.Fatalf("round %d: not sorted/disjoint: %s", round, str(list))
				}
			}
		}
		for _, p := range f.Allow4 {
			if in(f.Block4, p.Addr()) {
				t.Fatalf("round %d: %s in both", round, p)
			}
		}
		for range 300 {
			x := netip.AddrFrom4([4]byte{10, byte(rng.IntN(3)), byte(rng.IntN(256)), byte(rng.IntN(256))})
			want, _, _ := Verdict(allow, block, x)
			got := Neutral
			if in(f.Allow4, x) {
				got = Allowed
			} else if in(f.Block4, x) {
				got = Blocked
			}
			if got != want {
				t.Fatalf("round %d: %s folded %s, brute force %s (allow %s / block %s)", round, x, got, want, str(allow), str(block))
			}
		}
	}
}

func BenchmarkFoldCountrySized(b *testing.B) {
	rng := rand.New(rand.NewPCG(1, 1))
	var block []netip.Prefix
	for range 30000 {
		a := netip.AddrFrom4([4]byte{byte(rng.IntN(224)), byte(rng.IntN(256)), byte(rng.IntN(256)), 0})
		block = append(block, netip.PrefixFrom(a, 16+rng.IntN(9)).Masked())
	}
	allow := pfx("192.0.2.0/24", "198.51.100.7/32")
	b.ResetTimer()
	for range b.N {
		Fold(allow, block)
	}
}
