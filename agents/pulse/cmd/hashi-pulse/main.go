package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const version = "0.1.0"

type config struct {
	apiURL   string
	agentID  string
	token    string
	interval time.Duration
	timeout  time.Duration
	once     bool
}

type heartbeatRequest struct {
	Token                 string   `json:"token"`
	Version               string   `json:"version"`
	Hostname              string   `json:"hostname"`
	PrivateIpv4Candidates []string `json:"privateIpv4Candidates"`
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := &http.Client{Timeout: cfg.timeout}
	if cfg.once {
		if err := sendHeartbeat(ctx, client, cfg); err != nil {
			log.Fatalf("heartbeat failed: %v", err)
		}
		return
	}

	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()

	for {
		if err := sendHeartbeat(ctx, client, cfg); err != nil {
			log.Printf("heartbeat failed: %v", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func loadConfig() (config, error) {
	var cfg config
	flag.StringVar(&cfg.apiURL, "api", getenv("HASHI_PULSE_API", ""), "Hashi API base URL")
	flag.StringVar(&cfg.agentID, "agent-id", getenv("HASHI_PULSE_AGENT_ID", ""), "Pulse agent UUID")
	flag.StringVar(&cfg.token, "token", getenv("HASHI_PULSE_TOKEN", ""), "Pulse agent token")
	flag.DurationVar(&cfg.interval, "interval", durationEnv("HASHI_PULSE_INTERVAL", time.Minute), "heartbeat interval")
	flag.DurationVar(&cfg.timeout, "timeout", durationEnv("HASHI_PULSE_TIMEOUT", 10*time.Second), "HTTP timeout")
	flag.BoolVar(&cfg.once, "once", getenv("HASHI_PULSE_ONCE", "") == "1", "send one heartbeat and exit")
	flag.Parse()

	cfg.apiURL = strings.TrimRight(strings.TrimSpace(cfg.apiURL), "/")
	cfg.agentID = strings.TrimSpace(cfg.agentID)
	cfg.token = strings.TrimSpace(cfg.token)
	if cfg.apiURL == "" || cfg.agentID == "" || cfg.token == "" {
		return cfg, errors.New("HASHI_PULSE_API, HASHI_PULSE_AGENT_ID, and HASHI_PULSE_TOKEN are required")
	}
	if cfg.interval < 10*time.Second {
		cfg.interval = 10 * time.Second
	}
	if cfg.timeout < time.Second {
		cfg.timeout = time.Second
	}
	return cfg, nil
}

func sendHeartbeat(ctx context.Context, client *http.Client, cfg config) error {
	hostname, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostname) == "" {
		hostname = "unknown"
	}

	payload := heartbeatRequest{
		Token:                 cfg.token,
		Version:               version,
		Hostname:              hostname,
		PrivateIpv4Candidates: privateIPv4Candidates(),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/api/pulse/%s/heartbeat", cfg.apiURL, cfg.agentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("user-agent", "hashi-pulse/"+version)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected heartbeat status %s", resp.Status)
	}
	log.Printf("heartbeat accepted (%d private IPv4 candidate(s))", len(payload.PrivateIpv4Candidates))
	return nil
}

func privateIPv4Candidates() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var result []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch value := addr.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			ip4 := ip.To4()
			if ip4 == nil || !ip4.IsPrivate() {
				continue
			}
			result = append(result, ip4.String())
		}
	}
	return result
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func durationEnv(key string, fallback time.Duration) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
