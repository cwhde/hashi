// Package monitor is Hashi's built-in monitoring (07): endpoints synced from what Hashi manages, check executors, the
// state machine, closed-bucket rollups and the public views.
package monitor

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Check types (07 §2). TypeSelf asks Hashi's own readiness function.
const (
	TypeHTTP  = "http"
	TypeHTTPS = "https"
	TypeH2C   = "h2c"
	TypeTCP   = "tcp"
	TypeUDP   = "udp"
	TypeDNS   = "dns"
	TypeTLS   = "tls"
	TypeSelf  = "self"
)

// Certificate thresholds (07 §2).
const (
	CertWarn     = 21 * 24 * time.Hour
	CertCritical = 7 * 24 * time.Hour
)

// Check is one executable check.
type Check struct {
	Type       string
	Target     string // URL for the HTTP family; host:port for tcp, udp and tls; a name, optionally name@server:port, for dns
	CodeMin    int
	CodeMax    int
	SkipVerify bool
	Probe      string // udp: payload; dns: record type (A, AAAA, CNAME, TXT)
	Expect     string // udp: expected response; dns: expected answer
}

// Result is one execution. Degraded marks a passing check with a warning (a certificate close to expiry); Critical
// a failure that means down whatever else passes (a certificate about to expire).
type Result struct {
	OK       bool
	Degraded bool
	Critical bool
	Latency  time.Duration
	Detail   string
}

// Executor runs checks. Self answers TypeSelf.
type Executor struct {
	Self func(context.Context) error
	now  func() time.Time
}

// Run executes c within timeout. It never panics on bad targets: they are failed results.
func (e Executor) Run(ctx context.Context, c Check, timeout time.Duration) Result {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	r := e.run(ctx, c)
	r.Latency = time.Since(start)
	return r
}

func (e Executor) run(ctx context.Context, c Check) Result {
	switch c.Type {
	case TypeHTTP, TypeHTTPS, TypeH2C:
		return runHTTP(ctx, c)
	case TypeTCP:
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", c.Target)
		if err != nil {
			return failed(err)
		}
		_ = conn.Close()
		return Result{OK: true}
	case TypeUDP:
		return runUDP(ctx, c)
	case TypeDNS:
		return runDNS(ctx, c)
	case TypeTLS:
		return e.runTLS(ctx, c)
	case TypeSelf:
		if e.Self == nil {
			return Result{Detail: "no readiness check configured"}
		}
		if err := e.Self(ctx); err != nil {
			return failed(err)
		}
		return Result{OK: true}
	}
	return Result{Detail: "unknown check type " + c.Type}
}

func failed(err error) Result { return Result{Detail: err.Error()} }

func runHTTP(ctx context.Context, c Check) Result {
	tr := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: c.SkipVerify}, DisableKeepAlives: true} // #nosec G402 -- the admin's per-check choice, matching the service's upstream setting
	if c.Type == TypeH2C {
		tr.Protocols = new(http.Protocols)
		tr.Protocols.SetUnencryptedHTTP2(true)
	}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Target, nil)
	if err != nil {
		return failed(err)
	}
	req.Header.Set("User-Agent", "Hashi-Monitor/1")
	res, err := client.Do(req)
	if err != nil {
		return failed(err)
	}
	defer func() { _ = res.Body.Close() }()
	_, _ = io.CopyN(io.Discard, res.Body, 4096)
	lo, hi := c.CodeMin, c.CodeMax
	if lo == 0 && hi == 0 {
		lo, hi = 200, 399
	}
	if res.StatusCode < lo || res.StatusCode > hi {
		return Result{Detail: fmt.Sprintf("HTTP %d, expected %d-%d", res.StatusCode, lo, hi)}
	}
	return Result{OK: true, Detail: "HTTP " + strconv.Itoa(res.StatusCode)}
}

// runUDP needs a payload and an expected response: "no response" is never healthy (07 §2).
func runUDP(ctx context.Context, c Check) Result {
	if c.Probe == "" || c.Expect == "" {
		return Result{Detail: "a UDP check needs a probe payload and an expected response"}
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "udp", c.Target)
	if err != nil {
		return failed(err)
	}
	defer func() { _ = conn.Close() }()
	if d, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(d)
	}
	if _, err := conn.Write([]byte(c.Probe)); err != nil {
		return failed(err)
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return failed(err)
	}
	if !strings.Contains(string(buf[:n]), c.Expect) {
		return Result{Detail: "unexpected response"}
	}
	return Result{OK: true}
}

func runDNS(ctx context.Context, c Check) Result {
	name, server, _ := strings.Cut(c.Target, "@")
	res := net.DefaultResolver
	if server != "" {
		res = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, server)
		}}
	}
	var got []string
	var err error
	switch strings.ToUpper(c.Probe) {
	case "", "A":
		got, err = lookupIP(ctx, res, "ip4", name)
	case "AAAA":
		got, err = lookupIP(ctx, res, "ip6", name)
	case "CNAME":
		var v string
		v, err = res.LookupCNAME(ctx, name)
		got = []string{v}
	case "TXT":
		got, err = res.LookupTXT(ctx, name)
	default:
		return Result{Detail: "unsupported record type " + c.Probe}
	}
	if err != nil {
		return failed(err)
	}
	if len(got) == 0 {
		return Result{Detail: "no answer"}
	}
	if c.Expect != "" && !slices.Contains(got, c.Expect) && !slices.Contains(got, strings.TrimSuffix(c.Expect, ".")+".") {
		return Result{Detail: "answer " + strings.Join(got, ", ") + ", expected " + c.Expect}
	}
	return Result{OK: true, Detail: strings.Join(got, ", ")}
}

func lookupIP(ctx context.Context, res *net.Resolver, network, name string) ([]string, error) {
	ips, err := res.LookupIP(ctx, network, name)
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.String())
	}
	return out, err
}

// runTLS reads the certificate's remaining life: under 21 days warns, under 7 days fails (07 §2). It verifies the
// chain unless the check says otherwise, so an invalid certificate also fails.
func (e Executor) runTLS(ctx context.Context, c Check) Result {
	host, _, err := net.SplitHostPort(c.Target)
	if err != nil {
		return failed(err)
	}
	d := tls.Dialer{Config: &tls.Config{ServerName: host, InsecureSkipVerify: c.SkipVerify}} // #nosec G402 -- the admin's per-check choice
	conn, err := d.DialContext(ctx, "tcp", c.Target)
	if err != nil {
		return failed(err)
	}
	defer func() { _ = conn.Close() }()
	tc, ok := conn.(*tls.Conn)
	if !ok {
		return Result{Detail: "not a TLS connection"}
	}
	certs := tc.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return Result{Detail: "no certificate"}
	}
	now := time.Now
	if e.now != nil {
		now = e.now
	}
	left := certs[0].NotAfter.Sub(now())
	days := int(left.Hours() / 24)
	detail := fmt.Sprintf("certificate expires in %d days", days)
	switch {
	case left < CertCritical:
		return Result{Critical: true, Detail: detail}
	case left < CertWarn:
		return Result{OK: true, Degraded: true, Detail: detail}
	}
	return Result{OK: true, Detail: detail}
}
