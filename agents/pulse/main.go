// Command pulse reports this machine's current addresses to Hashi (08 §1). It holds one token that can only send this
// agent's heartbeat, keeps no local state and writes nothing to disk.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Version is set at build time.
var Version = "dev"

const defaultConfig = "/etc/hashi-pulse/config.yaml"

type config struct {
	URL, AgentID, Token, Interface string
	Interval                       time.Duration
}

// readConfig takes flat "key: value" lines from path, then environment variables over them. Nothing else in the format
// is used, so no YAML library is needed in a binary meant to stay small.
func readConfig(path string, env func(string) string) (config, error) {
	vals := map[string]string{}
	f, err := os.Open(path) //nolint:gosec // the admin's own config path
	switch {
	case err == nil:
		defer func() { _ = f.Close() }()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if k, v, ok := strings.Cut(line, ":"); ok {
				vals[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
			}
		}
		if err := sc.Err(); err != nil {
			return config{}, err
		}
	case !errors.Is(err, os.ErrNotExist):
		return config{}, err
	}
	for key, name := range map[string]string{"url": "HASHI_URL", "agent_id": "HASHI_AGENT_ID", "token": "HASHI_TOKEN", "interval": "HASHI_INTERVAL", "interface": "HASHI_INTERFACE"} {
		if v := env(name); v != "" {
			vals[key] = v
		}
	}
	c := config{URL: strings.TrimRight(vals["url"], "/"), AgentID: vals["agent_id"], Token: vals["token"], Interface: vals["interface"], Interval: time.Minute}
	if v := vals["interval"]; v != "" {
		secs, err := strconv.Atoi(v)
		if err != nil || secs < 10 {
			return c, fmt.Errorf("interval must be at least 10 seconds, got %q", v)
		}
		c.Interval = time.Duration(secs) * time.Second
	}
	if c.URL == "" || c.AgentID == "" || c.Token == "" {
		return c, errors.New("url, agent_id and token are required")
	}
	return c, nil
}

type candidate struct {
	Interface string `json:"interface"`
	Address   string `json:"address"`
}

type beat struct {
	Version    string      `json:"version"`
	Hostname   string      `json:"hostname"`
	Candidates []candidate `json:"candidates"`
	Selected   string      `json:"selected_interface,omitempty"`
	// ReportedPublic is this machine's public address as outside services see it. Sent only when Hashi asks for it.
	ReportedPublic string `json:"reported_public_ip,omitempty"`
}

// reply is Hashi's answer to a heartbeat.
type reply struct {
	CheckPublicIP bool `json:"check_public_ip"`
}

// Outside services are asked by address, not name: a filtering DNS server blocks the well-known names
// (testdata/contracts/public-ip). They are two separate companies, one over authenticated TLS and one over DNS.
const (
	traceURL        = "https://1.1.1.1/cdn-cgi/trace"
	openDNSServer   = "208.67.222.222:53"
	openDNSName     = "myip.opendns.com"
	publicIPRecheck = 5 * time.Minute
)

// parseTrace finds the address in Cloudflare's trace answer.
func parseTrace(body string) (netip.Addr, error) {
	for _, l := range strings.Split(body, "\n") {
		if v, ok := strings.CutPrefix(l, "ip="); ok {
			return netip.ParseAddr(strings.TrimSpace(v))
		}
	}
	return netip.Addr{}, errors.New("no address in the answer")
}

func traceIP(ctx context.Context, h *http.Client) (netip.Addr, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, traceURL, nil)
	if err != nil {
		return netip.Addr{}, err
	}
	res, err := h.Do(req)
	if err != nil {
		return netip.Addr{}, err
	}
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4096))
	if err != nil {
		return netip.Addr{}, err
	}
	return parseTrace(string(body))
}

func openDNSIP(ctx context.Context) (netip.Addr, error) {
	r := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, openDNSServer)
	}}
	addrs, err := r.LookupNetIP(ctx, "ip4", openDNSName)
	if err != nil {
		return netip.Addr{}, err
	}
	if len(addrs) != 1 {
		return netip.Addr{}, errors.New("expected one address")
	}
	return addrs[0].Unmap(), nil
}

// agreed returns the address only when both sources name the same public one: one wrong answer must not move a firewall rule.
func agreed(a, b netip.Addr) (string, bool) {
	if !a.IsValid() || a != b || !a.IsGlobalUnicast() || a.IsPrivate() {
		return "", false
	}
	return a.String(), true
}

func publicIP(ctx context.Context, h *http.Client) string {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	a, err := traceIP(ctx, h)
	if err != nil {
		log.Printf("public address check: %v", err)
		return ""
	}
	b, err := openDNSIP(ctx)
	if err != nil {
		log.Printf("public address check: %v", err)
		return ""
	}
	ip, ok := agreed(a, b)
	if !ok {
		log.Print("public address check: the two sources disagree, not reporting")
	}
	return ip
}

// usable leaves out what could never reach this machine from elsewhere.
func usable(a netip.Addr) bool {
	return a.IsValid() && !a.IsLoopback() && !a.IsLinkLocalUnicast() && !a.IsMulticast() && !a.IsUnspecified()
}

func candidates() ([]candidate, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	out := []candidate{}
	for _, i := range ifaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, x := range addrs {
			p, err := netip.ParsePrefix(x.String())
			if err == nil && usable(p.Addr().Unmap()) {
				out = append(out, candidate{Interface: i.Name, Address: p.Addr().Unmap().String()})
			}
		}
	}
	return out, nil
}

func send(ctx context.Context, h *http.Client, c config, b beat) (reply, error) {
	var out reply
	body, err := json.Marshal(b)
	if err != nil {
		return out, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+"/api/pulse/"+c.AgentID+"/heartbeat", bytes.NewReader(body))
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	res, err := h.Do(req)
	if err != nil {
		return out, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
		return out, fmt.Errorf("Hashi answered HTTP %d", res.StatusCode) //nolint:staticcheck // a name
	}
	return out, json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&out)
}

// jitter spreads many agents over time: ±10 %.
func jitter(d time.Duration) time.Duration {
	return d + time.Duration(rand.Int64N(int64(d/5)+1)) - d/10 //nolint:gosec // not security relevant
}

func run(ctx context.Context, c config) error {
	h := &http.Client{Timeout: 15 * time.Second}
	host, _ := os.Hostname()
	wait := time.Duration(0)
	failures := 0
	check, reported := false, ""
	var checked time.Time
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}
		if !check {
			reported = ""
		} else if time.Since(checked) >= publicIPRecheck { // outside services hear from this machine only this often
			reported, checked = publicIP(ctx, h), time.Now()
		}
		cs, err := candidates()
		if err == nil {
			var r reply
			r, err = send(ctx, h, c, beat{Version: Version, Hostname: host, Candidates: cs, Selected: c.Interface, ReportedPublic: reported})
			if err == nil {
				if r.CheckPublicIP && !check {
					wait = 0
				}
				check = r.CheckPublicIP
			}
		}
		if err == nil {
			failures, wait = 0, jitter(c.Interval)
			continue
		}
		failures++
		wait = jitter(min(c.Interval, 5*time.Second<<min(failures, 8)))
		log.Printf("heartbeat failed, retrying in %s: %v", wait.Round(time.Second), err)
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Println(Version)
		return
	}
	path := defaultConfig
	if v := os.Getenv("HASHI_PULSE_CONFIG"); v != "" {
		path = v
	}
	c, err := readConfig(path, os.Getenv)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, c); err != nil {
		log.Fatal(err)
	}
}
