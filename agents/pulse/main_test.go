package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"debug/elf"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
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
	var got map[string]any
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
	b := beat{Version: "1", Hostname: "h\"\\\n\xff", Interval: 60, SentAt: time.Unix(1, 0).UTC(), Candidates: []candidate{{"eth0", "10.0.0.1"}}, Selected: "eth0", InDocker: true, ReportedPublic: "198.51.100.1"}
	r, _, err := send(t.Context(), c, b, nil)
	if err != nil || !r.CheckPublicIP || r.IntervalSeconds != 120 {
		t.Fatalf("%+v %v", r, err)
	}
	if auth != "Bearer t /api/pulse/a1/heartbeat" || got["hostname"] != "h\"\\\n�" || got["selected_interface"] != "eth0" || got["in_docker"] != true || got["reported_public_ip"] != "198.51.100.1" || got["interval_seconds"] != float64(60) || got["sent_at"] != "1970-01-01T00:00:01Z" {
		t.Fatalf("%s %+v", auth, got)
	}
	if cs := got["candidates"].([]any); len(cs) != 1 || cs[0].(map[string]any)["address"] != "10.0.0.1" || cs[0].(map[string]any)["interface"] != "eth0" {
		t.Fatalf("%+v", got)
	}
	code = http.StatusUnauthorized
	if _, _, err := send(t.Context(), c, beat{}, nil); err == nil {
		t.Fatal("401 must be an error")
	}
}

// A reverse proxy may answer in chunks or without a length.
func TestParseHTTP(t *testing.T) {
	for raw, want := range map[string]string{
		"HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nokEXTRA":                                     "ok",
		"HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n2\r\nok\r\n1;x=y\r\n!\r\n0\r\n\r\n": "ok!",
		"HTTP/1.0 200 OK\r\n\r\nto the end":                                                       "to the end",
	} {
		if code, body, err := parseHTTP([]byte(raw)); err != nil || code != 200 || body != want {
			t.Errorf("%q: %d %q %v", raw, code, body, err)
		}
	}
	for _, raw := range []string{"", "HTTP/1.1 200 OK\r\n", "garbage\r\n\r\n", "HTTP/1.1 200 OK\r\nContent-Length: 9\r\n\r\nshort", "HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\n\r\n9\r\nshort"} {
		if _, _, err := parseHTTP([]byte(raw)); err == nil {
			t.Errorf("%q must fail", raw)
		}
	}
}

func TestParseReply(t *testing.T) {
	r, err := parseReply(" {\"new\": \"x\", \"interval_seconds\" : 90, \"check_public_ip\": true}\n")
	if err != nil || !r.CheckPublicIP || r.IntervalSeconds != 90 {
		t.Fatalf("%+v %v", r, err)
	}
	if r, err := parseReply(`{}`); err != nil || r.CheckPublicIP || r.IntervalSeconds != 0 {
		t.Fatalf("%+v %v", r, err)
	}
	for _, s := range []string{"", "<html></html>", `{"interval_seconds":"x"}`} {
		if _, err := parseReply(s); err == nil {
			t.Errorf("%q must fail", s)
		}
	}
}

func TestStateSurvivesTheSleeper(t *testing.T) {
	in := state{Due: true, Failures: 2, Check: true, Checked: 5, Reported: "198.51.100.1", Interval: 60, Root: []byte{1, 2}, RootAt: 7}
	if got := decodeState(in.encode()); got.Due != in.Due || got.Failures != 2 || !got.Check || got.Checked != 5 || got.Reported != in.Reported || got.Interval != 60 || string(got.Root) != string(in.Root) || got.RootAt != 7 {
		t.Fatalf("%+v", got)
	}
	for _, bad := range []string{"", "x", "1,x,0,0,,0,,0", "1,0,0,0,,0,!!,0"} {
		if got := decodeState(bad); got.Due || got.Root != nil {
			t.Errorf("%q: %+v", bad, got)
		}
	}
}

// Every agent keeps its own point in the interval, wherever it starts, and agents spread out over it.
func TestSlots(t *testing.T) {
	iv := time.Minute
	base := time.Unix(1_800_000_000, 0)
	want := base.Add(slotWait("a", iv, base))
	for _, off := range []time.Duration{time.Second, 17 * time.Second, 59 * time.Second} {
		at := base.Add(off)
		got := at.Add(slotWait("a", iv, at))
		if d := got.Sub(want) % iv; d != 0 {
			t.Errorf("start %v: slot moved by %v", off, d)
		}
	}
	if w := nextBeat("a", iv, want); w != iv {
		t.Errorf("the next beat comes after a full interval, got %v", w)
	}
	if w := nextBeat("a", iv, want.Add(-time.Second)); w != iv+time.Second {
		t.Errorf("a slot closer than half an interval is skipped, got %v", w)
	}
	seen := map[int64]int{}
	for i := range 50 {
		seen[int64(slotWait("11111111-2222-4333-8444-"+strconv.Itoa(100000000000+i), iv, base)/(5*time.Second))]++
	}
	for k, n := range seen {
		if n > 15 {
			t.Errorf("%d of 50 agents share 5 seconds (%d)", n, k)
		}
	}
}

// The kept root is used, replaced when the server's certificate no longer fits it, and dropped when it is old.
func TestKeptRoot(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"check_public_ip":false,"interval_seconds":60}`))
	}))
	defer srv.Close()
	file := filepath.Join(t.TempDir(), "roots.pem")
	if err := os.WriteFile(file, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", file)
	c := config{URL: srv.URL, AgentID: "a", Token: "t", Interval: time.Minute}
	st := state{}
	if _, err := deliver(t.Context(), c, &st, beat{}); err != nil {
		t.Skipf("this machine's trust list cannot be replaced in a test: %v", err)
	}
	if st.Root == nil {
		t.Fatal("the root was not kept")
	}
	if p := st.pool(time.Now().Add(rootMaxAge)); p != nil || st.Root != nil {
		t.Fatal("an old root must be dropped")
	}
	other, _ := x509.ParseCertificate(srv.Certificate().Raw)
	st = state{Root: wrongRoot(t), RootAt: time.Now().Unix()}
	if _, err := deliver(t.Context(), c, &st, beat{}); err != nil {
		t.Fatalf("a root that no longer fits must fall back to the full list: %v", err)
	}
	if string(st.Root) != string(other.Raw) {
		t.Fatal("the fitting root was not kept")
	}
}

func wrongRoot(t *testing.T) []byte {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "other"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true}
	d, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &k.PublicKey, k)
	if err != nil {
		t.Fatal(err)
	}
	return d
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
	var seen state
	go func() {
		done <- run(ctx, c, state{Due: true}, func(ctx context.Context, wait time.Duration, st state) bool {
			seen = st
			return residentPause(ctx, wait, st)
		})
	}()
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
	if !seen.Due {
		t.Fatal("a pause must leave the next beat due")
	}
}

// A fresh start waits for this agent's slot before doing anything.
func TestFreshStartWaitsForSlot(t *testing.T) {
	var beats atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { beats.Add(1) }))
	defer srv.Close()
	c := config{URL: srv.URL, AgentID: "a", Token: "t", Interval: 10 * time.Second}
	var waited time.Duration
	err := run(t.Context(), c, state{}, func(_ context.Context, wait time.Duration, st state) bool {
		waited = wait
		if !st.Due {
			t.Error("the sleeper must hand over a due beat")
		}
		return false
	})
	if err != nil || beats.Load() != 0 || waited <= 0 || waited > c.Interval {
		t.Fatalf("%v, %d beats, waited %v", err, beats.Load(), waited)
	}
}

// The committed sleepers are what they claim to be: small programs for the right processors.
func TestSleeperBinaries(t *testing.T) {
	for file, machine := range map[string]elf.Machine{"sleeper/sleeper_amd64.bin": elf.EM_X86_64, "sleeper/sleeper_arm64.bin": elf.EM_AARCH64} {
		f, err := elf.Open(file)
		if err != nil {
			t.Fatal(err)
		}
		if f.Machine != machine || f.Type != elf.ET_EXEC {
			t.Errorf("%s: %v %v", file, f.Machine, f.Type)
		}
		_ = f.Close()
		st, err := os.Stat(file)
		if err != nil || st.Size() > 8192 {
			t.Errorf("%s: %v %v", file, st, err)
		}
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
