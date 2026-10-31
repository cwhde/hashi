package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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

// send talks to Hashi as the contract says, and a failing answer is an error.
func TestSend(t *testing.T) {
	var got beat
	var auth string
	code := http.StatusOK
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization") + " " + r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(code)
		_, _ = w.Write([]byte(`{"check_public_ip":true,"interval_seconds":120}`))
	}))
	defer srv.Close()
	c := config{URL: srv.URL, AgentID: "a1", Token: "t", Interval: time.Minute}
	r, err := send(t.Context(), srv.Client(), c, beat{Version: "1", Hostname: "h", Interval: 60})
	if err != nil || !r.CheckPublicIP || r.IntervalSeconds != 120 {
		t.Fatalf("%+v %v", r, err)
	}
	if auth != "Bearer t /api/pulse/a1/heartbeat" || got.Hostname != "h" {
		t.Fatalf("%s %+v", auth, got)
	}
	code = http.StatusUnauthorized
	if _, err := send(t.Context(), srv.Client(), c, beat{}); err == nil {
		t.Fatal("401 must be an error")
	}
}

// run retries after a failure, adopts Hashi's interval unless it has its own, and stops when told to.
func TestRunRetriesAndStops(t *testing.T) {
	var beats atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if beats.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"check_public_ip":false,"interval_seconds":10}`))
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	c := config{URL: srv.URL, AgentID: "a", Token: "t", Interval: 10 * time.Second}
	done := make(chan error, 1)
	go func() { done <- run(ctx, c) }()
	deadline := time.Now().Add(7 * time.Second)
	for beats.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if beats.Load() < 2 {
		t.Fatal("no retry after a failure")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run did not stop")
	}
}

// The captured OpenDNS answer holds what the lookup expects: one A record for the name.
func TestOpenDNSContract(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/contracts/public-ip/opendns-answer.txt")
	if err != nil {
		t.Fatal(err)
	}
	var fields []string
	for _, l := range strings.Split(string(raw), "\n") {
		if l != "" && !strings.HasPrefix(l, ";") {
			fields = strings.Fields(l)
		}
	}
	if len(fields) != 5 || strings.TrimSuffix(fields[0], ".") != openDNSName || fields[3] != "A" {
		t.Fatalf("%v", fields)
	}
	if !netip.MustParseAddr(fields[4]).Is4() {
		t.Fatal(fields[4])
	}
}
