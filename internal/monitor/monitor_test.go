package monitor

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

func run(t *testing.T, c Check) Result {
	t.Helper()
	return Executor{}.Run(t.Context(), c, 3*time.Second)
}

func TestHTTPChecks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			w.WriteHeader(500)
			return
		}
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/nowhere", http.StatusFound)
		}
	}))
	defer srv.Close()
	if r := run(t, Check{Type: TypeHTTP, Target: srv.URL}); !r.OK {
		t.Fatalf("200: %+v", r)
	}
	if r := run(t, Check{Type: TypeHTTP, Target: srv.URL + "/redirect"}); !r.OK {
		t.Fatalf("a redirect is in the default range and is not followed: %+v", r)
	}
	if r := run(t, Check{Type: TypeHTTP, Target: srv.URL + "/bad"}); r.OK {
		t.Fatal("500 passed")
	}
	if r := run(t, Check{Type: TypeHTTP, Target: srv.URL + "/bad", CodeMin: 500, CodeMax: 500}); !r.OK {
		t.Fatalf("allowed code: %+v", r)
	}
	tlsSrv := httptest.NewTLSServer(http.NotFoundHandler())
	defer tlsSrv.Close()
	if r := run(t, Check{Type: TypeHTTPS, Target: tlsSrv.URL}); r.OK {
		t.Fatal("an untrusted certificate passed")
	}
	if r := run(t, Check{Type: TypeHTTPS, Target: tlsSrv.URL, SkipVerify: true, CodeMin: 404, CodeMax: 404}); !r.OK {
		t.Fatalf("skip verify: %+v", r)
	}
}

func TestH2CCheck(t *testing.T) {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 {
			w.WriteHeader(505)
		}
	}))
	srv.Config.Protocols = new(http.Protocols)
	srv.Config.Protocols.SetUnencryptedHTTP2(true)
	srv.Start()
	defer srv.Close()
	if r := run(t, Check{Type: TypeH2C, Target: srv.URL}); !r.OK {
		t.Fatalf("h2c: %+v", r)
	}
}

func TestTCPCheck(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if r := run(t, Check{Type: TypeTCP, Target: addr}); !r.OK {
		t.Fatalf("open: %+v", r)
	}
	_ = ln.Close()
	if r := run(t, Check{Type: TypeTCP, Target: addr}); r.OK {
		t.Fatal("closed port passed")
	}
}

// A silent UDP port is never healthy (07 §2): without a probe nothing is sent, and no answer fails.
func TestUDPCheck(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pc.Close() }()
	go func() {
		buf := make([]byte, 64)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if string(buf[:n]) == "ping" {
				_, _ = pc.WriteTo([]byte("pong"), from)
			}
		}
	}()
	addr := pc.LocalAddr().String()
	if r := run(t, Check{Type: TypeUDP, Target: addr}); r.OK {
		t.Fatal("no probe passed")
	}
	if r := run(t, Check{Type: TypeUDP, Target: addr, Probe: "ping", Expect: "pong"}); !r.OK {
		t.Fatalf("answer: %+v", r)
	}
	if r := (Executor{}).Run(t.Context(), Check{Type: TypeUDP, Target: addr, Probe: "other", Expect: "pong"}, 200*time.Millisecond); r.OK {
		t.Fatal("no response passed")
	}
}

func TestDNSCheckAgainstLocalServer(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pc.Close() }()
	go func() {
		buf := make([]byte, 512)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			q := append([]byte(nil), buf[:n]...)
			q[2], q[3] = 0x81, 0x80 // response, no error
			q[6], q[7] = 0, 1       // one answer
			q = q[:12+questionLen(q[12:])]
			q = append(q, 0xc0, 12, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, 192, 0, 2, 7)
			_, _ = pc.WriteTo(q, from)
		}
	}()
	target := "app.example.com@" + pc.LocalAddr().String()
	if r := run(t, Check{Type: TypeDNS, Target: target, Probe: "A", Expect: "192.0.2.7"}); !r.OK {
		t.Fatalf("expected answer: %+v", r)
	}
	if r := run(t, Check{Type: TypeDNS, Target: target, Probe: "A", Expect: "192.0.2.8"}); r.OK {
		t.Fatal("wrong answer passed")
	}
}

func questionLen(b []byte) int {
	i := 0
	for b[i] != 0 {
		i += int(b[i]) + 1
	}
	return i + 5
}

// Certificate expiry: over 21 days fine, under 21 warns, under 7 fails (07 §2).
func TestTLSExpiryThresholds(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(100 * 24 * time.Hour), DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))
	if err != nil {
		t.Fatal(err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.(*tls.Conn).Handshake()
			_ = c.Close()
		}
	}()
	check := Check{Type: TypeTLS, Target: ln.Addr().String(), SkipVerify: true}
	at := func(daysLeft int) Result {
		e := Executor{now: func() time.Time { return tmpl.NotAfter.Add(-time.Duration(daysLeft) * 24 * time.Hour) }}
		return e.Run(context.Background(), check, 3*time.Second)
	}
	if r := at(30); !r.OK || r.Degraded {
		t.Fatalf("30 days: %+v", r)
	}
	if r := at(14); !r.OK || !r.Degraded {
		t.Fatalf("14 days: %+v", r)
	}
	if r := at(3); r.OK {
		t.Fatalf("3 days: %+v", r)
	}
}

func TestVerdict(t *testing.T) {
	ok, bad, warn := Result{OK: true}, Result{}, Result{OK: true, Degraded: true}
	for _, c := range []struct {
		name   string
		checks []RoundCheck
		slow   int64
		lat    int64
		want   string
	}{
		{"all pass", []RoundCheck{{Result: ok}, {Result: ok}}, 0, 5, StateUp},
		{"one of two paths", []RoundCheck{{Result: ok}, {Result: bad}}, 0, 5, StateDegraded},
		{"both paths", []RoundCheck{{Result: bad}, {Result: bad}}, 0, 5, StateDown},
		{"required fails", []RoundCheck{{Required: true, Result: bad}, {Result: ok}}, 0, 5, StateDown},
		{"optional fails beside required", []RoundCheck{{Required: true, Result: ok}, {Result: bad}}, 0, 5, StateDegraded},
		{"warning", []RoundCheck{{Result: ok}, {Result: warn}}, 0, 5, StateDegraded},
		{"slow", []RoundCheck{{Result: ok}}, 100, 101, StateDegraded},
		{"not slow without threshold", []RoundCheck{{Result: ok}}, 0, 9999, StateUp},
		{"no checks", nil, 0, 0, StateDown},
		// Review of S7: a dead upstream behind a healthy edge: both paths fail while the certificate still passes.
		{"dead paths beside a good certificate", []RoundCheck{{Result: bad}, {Result: bad}, {Advisory: true, Result: ok}}, 0, 5, StateDown},
		{"edge down: one path and the certificate", []RoundCheck{{Result: ok}, {Result: bad}, {Advisory: true, Result: bad}}, 0, 5, StateDegraded},
		{"certificate about to expire", []RoundCheck{{Result: ok}, {Advisory: true, Result: Result{Critical: true}}}, 0, 5, StateDown},
	} {
		if got := Verdict(c.checks, c.slow, c.lat); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

// 3 failures → down, 2 successes → up (07 §3); a blip changes nothing.
func TestStateMachineThresholds(t *testing.T) {
	m := Machine{State: StateUnknown}
	step := func(v, want string) {
		t.Helper()
		m = m.Next(v)
		if m.State != want {
			t.Fatalf("after %s: %s, want %s", v, m.State, want)
		}
	}
	step(StateUp, StateUnknown)
	step(StateUp, StateUp)
	step(StateDown, StateUp)
	step(StateDown, StateUp)
	step(StateUp, StateUp) // the blip resets the count
	step(StateDown, StateUp)
	step(StateDown, StateUp)
	step(StateDown, StateDown)
	step(StateUp, StateDown)
	step(StateDown, StateDown)
	step(StateUp, StateDown)
	step(StateUp, StateUp)
	step(StateDegraded, StateUp)
	step(StateDegraded, StateDegraded)
}

// Only closed buckets are rolled up, and their numbers are right (07 §4).
func TestRollupsClosedBucketsOnly(t *testing.T) {
	const minute = int64(60_000)
	var samples []db.MonitorSample
	for i := int64(1); i <= 20; i++ {
		ok := int64(1)
		if i == 20 {
			ok = 0
		}
		samples = append(samples, db.MonitorSample{EndpointID: "e", CheckID: "", At: minute + i*1000, Ok: ok, LatencyMs: i})
	}
	samples = append(samples, db.MonitorSample{EndpointID: "e", CheckID: "", At: 2*minute + 1000, Ok: 1, LatencyMs: 99})
	got := Rollups(samples, 60, 2*minute+5000)
	want := []db.UpsertMonitorRollupParams{{EndpointID: "e", CheckID: "", Resolution: 60, Bucket: minute, Count: 20, OkCount: 19, MinMs: 1, MaxMs: 20, AvgMs: 10, P95Ms: 19}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rollups\n got %+v\nwant %+v", got, want)
	}
	if len(Rollups(samples, 60, 3*minute)) != 2 {
		t.Fatal("the second bucket has closed")
	}
}

func TestServiceChecksByProtocol(t *testing.T) {
	http := services.Service{Kind: services.KindHTTP, HostName: "a.example.com", Target: services.Target{Address: "10.0.0.1", Port: 80}}
	if got := ServiceChecks(http); len(got) != 3 || got[0].Role != "internal" || got[1].Role != "public" || got[2].Type != TypeTLS || got[0].Required {
		t.Fatalf("http %+v", got)
	}
	tcp := services.Service{Kind: services.KindTCP, Target: services.Target{Address: "10.0.0.1", Port: 22}}
	if got := ServiceChecks(tcp); len(got) != 1 || got[0].Type != TypeTCP || !got[0].Required {
		t.Fatalf("tcp %+v", got)
	}
	if got := ServiceChecks(services.Service{Kind: services.KindUDP}); got != nil {
		t.Fatalf("udp needs a configured probe: %+v", got)
	}
}
