package main

import (
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("# note\nurl: https://hashi.example.com/\nagent_id: a1\ntoken: \"t\"\ninterval: 30\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := readConfig(path, func(k string) string {
		if k == "HASHI_INTERFACE" {
			return "eth0"
		}
		return ""
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.URL != "https://hashi.example.com" || c.AgentID != "a1" || c.Token != "t" || c.Interval.Seconds() != 30 || c.Interface != "eth0" {
		t.Fatalf("%+v", c)
	}
	if _, err := readConfig(filepath.Join(t.TempDir(), "none"), func(string) string { return "" }); err == nil {
		t.Fatal("an empty config must be refused")
	}
}

func TestUsable(t *testing.T) {
	for s, want := range map[string]bool{"10.0.0.5": true, "2001:db8::1": true, "127.0.0.1": false, "fe80::1": false, "169.254.1.1": false} {
		if usable(netip.MustParseAddr(s)) != want {
			t.Errorf("%s", s)
		}
	}
}

// The captured answer (testdata/contracts/public-ip).
func TestParseTrace(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/contracts/public-ip/cloudflare-trace.txt")
	if err != nil {
		t.Fatal(err)
	}
	a, err := parseTrace(string(raw))
	if err != nil || a.String() != "198.51.100.7" {
		t.Fatalf("%v %v", a, err)
	}
	if _, err := parseTrace(strings.ReplaceAll(string(raw), "ip=", "x=")); err == nil {
		t.Fatal("an answer without an address must fail")
	}
}

// One wrong source must not move a firewall rule.
func TestAgreed(t *testing.T) {
	p := netip.MustParseAddr
	if ip, ok := agreed(p("8.8.8.8"), p("8.8.8.8")); !ok || ip != "8.8.8.8" {
		t.Fatal(ip)
	}
	for _, c := range [][2]netip.Addr{{p("8.8.8.8"), p("8.8.4.4")}, {p("10.0.0.1"), p("10.0.0.1")}, {{}, {}}} {
		if _, ok := agreed(c[0], c[1]); ok {
			t.Errorf("%v", c)
		}
	}
}
