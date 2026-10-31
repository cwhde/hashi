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

func send(ctx context.Context, h *http.Client, c config, b beat) error {
	body, err := json.Marshal(b)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+"/api/pulse/"+c.AgentID+"/heartbeat", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	res, err := h.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Hashi answered HTTP %d", res.StatusCode) //nolint:staticcheck // a name
	}
	return nil
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
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}
		cs, err := candidates()
		if err == nil {
			err = send(ctx, h, c, beat{Version: Version, Hostname: host, Candidates: cs, Selected: c.Interface})
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
