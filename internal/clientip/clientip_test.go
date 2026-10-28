package clientip

import (
	"net/http"
	"net/netip"
	"testing"
)

func req(peer string, h map[string]string) *http.Request {
	r, _ := http.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = peer
	for k, v := range h {
		r.Header.Set(k, v)
	}
	return r
}

func TestCanonical(t *testing.T) {
	for in, want := range map[string]string{
		"192.0.2.1":               "192.0.2.1",
		"192.0.2.1:4444":          "192.0.2.1",
		"[::ffff:192.0.2.1]:4444": "192.0.2.1", // IPv4-mapped IPv6 → IPv4
		"::ffff:c000:201":         "192.0.2.1",
		"[2001:db8::1]:443":       "2001:db8::1",
		"2001:DB8:0:0::1":         "2001:db8::1",
		"fe80::1%eth0":            "fe80::1",
		"[fe80::1%25eth0]:80":     "fe80::1",
		" 198.51.100.7 ":          "198.51.100.7",
	} {
		a, ok := Canonical(in)
		if !ok || a.String() != want {
			t.Errorf("Canonical(%q) = %v %v, want %s", in, a, ok, want)
		}
	}
	for _, bad := range []string{"", "unknown", "192.0.2", "_hidden", "300.1.1.1"} {
		if _, ok := Canonical(bad); ok {
			t.Errorf("Canonical(%q) accepted", bad)
		}
	}
}

func TestResolve(t *testing.T) {
	trusted, _ := ParsePrefixes([]string{"172.31.9.0/24", "::1"})
	r := New(trusted)
	cases := []struct {
		name, peer string
		h          map[string]string
		want       string
	}{
		{"untrusted peer ignores XFF", "192.0.2.9:1", map[string]string{"X-Forwarded-For": "198.51.100.1"}, "192.0.2.9"},
		{"untrusted peer ignores X-Real-IP", "192.0.2.9:1", map[string]string{"X-Real-IP": "198.51.100.1"}, "192.0.2.9"},
		{"trusted peer, no header", "172.31.9.20:1", nil, "172.31.9.20"},
		{"trusted peer, single hop", "172.31.9.20:1", map[string]string{"X-Forwarded-For": "198.51.100.1"}, "198.51.100.1"},
		{"spoofed left entries are skipped", "172.31.9.20:1", map[string]string{"X-Forwarded-For": "1.1.1.1, 198.51.100.1"}, "198.51.100.1"},
		{"trusted hops skipped right to left", "172.31.9.20:1", map[string]string{"X-Forwarded-For": "198.51.100.1, 172.31.9.7"}, "198.51.100.1"},
		{"mapped address in header", "172.31.9.20:1", map[string]string{"X-Forwarded-For": "::ffff:198.51.100.1"}, "198.51.100.1"},
		{"Forwarded header", "[::1]:1", map[string]string{"Forwarded": `for="[2001:db8::7]:4711";proto=https`}, "2001:db8::7"},
		{"X-Real-IP from trusted", "172.31.9.20:1", map[string]string{"X-Real-IP": "198.51.100.3"}, "198.51.100.3"},
		{"mapped peer", "[::ffff:192.0.2.9]:1", nil, "192.0.2.9"},
	}
	for _, c := range cases {
		got, err := r.Resolve(req(c.peer, c.h))
		if err != nil || got.String() != c.want {
			t.Errorf("%s: got %v %v, want %s", c.name, got, err, c.want)
		}
	}
}

func TestResolveFailsClosed(t *testing.T) {
	trusted, _ := ParsePrefixes([]string{"172.31.9.0/24"})
	r := New(trusted)
	for name, rq := range map[string]*http.Request{
		"bad peer":              req("nonsense", nil),
		"garbage hop":           req("172.31.9.20:1", map[string]string{"X-Forwarded-For": "198.51.100.1, garbage"}),
		"obfuscated Forwarded":  req("172.31.9.20:1", map[string]string{"Forwarded": "for=_hidden"}),
		"Forwarded without for": req("172.31.9.20:1", map[string]string{"Forwarded": "proto=https"}),
	} {
		if a, err := r.Resolve(rq); err == nil {
			t.Errorf("%s: resolved to %v", name, a)
		}
	}
}

func TestSetTrustedTakesEffect(t *testing.T) {
	r := New(nil)
	rq := req("172.31.9.20:1", map[string]string{"X-Forwarded-For": "198.51.100.1"})
	if a, _ := r.Resolve(rq); a.String() != "172.31.9.20" {
		t.Fatalf("before: %v", a)
	}
	r.SetTrusted([]netip.Prefix{netip.MustParsePrefix("172.31.9.20/32")})
	if a, _ := r.Resolve(rq); a.String() != "198.51.100.1" {
		t.Fatalf("after: %v", a)
	}
}

// A request through Hashi-managed Traefik behind NAT arrives from the gateway address; the edge token, not the address,
// makes its forwarding headers count (01 §8). A wrong, repeated or missing token changes nothing.
func TestEdgeTokenTrustsTheEdge(t *testing.T) {
	r := New(nil)
	r.SetEdgeToken("tok")
	fwd := map[string]string{"X-Forwarded-For": "198.51.100.7", "X-Forwarded-Proto": "https"}
	edge := req("192.168.50.1:5000", map[string]string{"X-Forwarded-For": "198.51.100.7", "X-Forwarded-Proto": "https", EdgeHeader: "tok"})
	if a, _ := r.Resolve(edge); a.String() != "198.51.100.7" || !r.IsTLS(edge) || !r.FromEdge(edge) {
		t.Errorf("edge request: %v tls=%v", a, r.IsTLS(edge))
	}
	for name, rq := range map[string]*http.Request{
		"no token":    req("192.168.50.1:5000", fwd),
		"wrong token": req("192.168.50.1:5000", map[string]string{"X-Forwarded-For": "198.51.100.7", EdgeHeader: "tox"}),
	} {
		if a, _ := r.Resolve(rq); a.String() != "192.168.50.1" || r.IsTLS(rq) || r.FromEdge(rq) {
			t.Errorf("%s: %v", name, a)
		}
	}
	twice := req("192.168.50.1:5000", fwd)
	twice.Header.Add(EdgeHeader, "tok")
	twice.Header.Add(EdgeHeader, "tok")
	if r.FromEdge(twice) {
		t.Error("a repeated header counted")
	}
	r.SetEdgeToken("")
	if r.FromEdge(edge) {
		t.Error("empty token still trusts")
	}
}

// Forward auth through the edge's loopback hop: the visitor is the hop before 127.0.0.1. Without the token a loopback
// hop is not skipped.
func TestEdgeLoopbackHopIsSkipped(t *testing.T) {
	r := New(nil)
	r.SetEdgeToken("tok")
	h := map[string]string{"X-Forwarded-For": "198.51.100.7, 127.0.0.1", EdgeHeader: "tok"}
	if a, _ := r.Resolve(req("192.168.50.1:5000", h)); a.String() != "198.51.100.7" {
		t.Errorf("with token: %v", a)
	}
	r.SetTrusted([]netip.Prefix{netip.MustParsePrefix("192.168.50.1/32")})
	if a, _ := r.Resolve(req("192.168.50.1:5000", map[string]string{"X-Forwarded-For": "198.51.100.7, 127.0.0.1"})); a.String() != "127.0.0.1" {
		t.Errorf("trusted proxy without token: %v", a)
	}
}
