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
	apiURL            string
	agentID           string
	token             string
	selectedInterface string
	interval          time.Duration
	timeout           time.Duration
	once              bool
}

type heartbeatRequest struct {
	Token                 string   `json:"token"`
	Version               string   `json:"version"`
	Hostname              string   `json:"hostname"`
	PrivateIpv4Candidates []string `json:"privateIpv4Candidates"`
	PrivateIpv6Candidates []string `json:"privateIpv6Candidates"`
	SelectedInterface     string   `json:"selectedInterface,omitempty"`
	SelectedIP            string   `json:"selectedIp,omitempty"`
	Timestamp             string   `json:"timestamp"`
	Docker                *dockerMetadata `json:"docker,omitempty"`
}

type dockerMetadata struct {
	ContainerID string `json:"containerId,omitempty"`
	Image       string `json:"image,omitempty"`
	NetworkMode string `json:"networkMode,omitempty"`
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
	flag.StringVar(&cfg.selectedInterface, "interface", getenv("HASHI_PULSE_INTERFACE", ""), "network interface to prefer")
	flag.DurationVar(&cfg.interval, "interval", durationEnv("HASHI_PULSE_INTERVAL", time.Minute), "heartbeat interval")
	flag.DurationVar(&cfg.timeout, "timeout", durationEnv("HASHI_PULSE_TIMEOUT", 10*time.Second), "HTTP timeout")
	flag.BoolVar(&cfg.once, "once", getenv("HASHI_PULSE_ONCE", "") == "1", "send one heartbeat and exit")
	flag.Parse()

	cfg.apiURL = strings.TrimRight(strings.TrimSpace(cfg.apiURL), "/")
	cfg.agentID = strings.TrimSpace(cfg.agentID)
	cfg.token = strings.TrimSpace(cfg.token)
	cfg.selectedInterface = strings.TrimSpace(cfg.selectedInterface)
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

	ipv4, ipv6, selectedInterface, selectedIP := privateIPCandidates(cfg.selectedInterface)
	payload := heartbeatRequest{
		Token:                 cfg.token,
		Version:               version,
		Hostname:              hostname,
		PrivateIpv4Candidates: ipv4,
		PrivateIpv6Candidates: ipv6,
		SelectedInterface:     selectedInterface,
		SelectedIP:            selectedIP,
		Timestamp:             time.Now().UTC().Format(time.RFC3339Nano),
		Docker:                detectDockerMetadata(),
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
	log.Printf("heartbeat accepted (%d private IPv4 candidate(s), %d private IPv6 candidate(s))", len(payload.PrivateIpv4Candidates), len(payload.PrivateIpv6Candidates))
	return nil
}

func privateIPv4Candidates() []string {
	ipv4, _, _, _ := privateIPCandidates("")
	return ipv4
}

func privateIPCandidates(preferredInterface string) ([]string, []string, string, string) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, "", ""
	}
	var ipv4 []string
	var ipv6 []string
	var selectedInterface string
	var selectedIP string
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
			if ip4 != nil {
				if !ip4.IsPrivate() {
					continue
				}
				value := ip4.String()
				ipv4 = appendIfMissing(ipv4, value)
				if selectedIP == "" && interfaceMatches(iface.Name, preferredInterface) {
					selectedInterface = iface.Name
					selectedIP = value
				}
				continue
			}

			ip16 := ip.To16()
			if ip16 == nil || !isPrivateIPv6(ip16) {
				continue
			}
			value := ip16.String()
			ipv6 = appendIfMissing(ipv6, value)
			if selectedIP == "" && interfaceMatches(iface.Name, preferredInterface) {
				selectedInterface = iface.Name
				selectedIP = value
			}
		}
	}
	return ipv4, ipv6, selectedInterface, selectedIP
}

func interfaceMatches(name, preferred string) bool {
	return preferred == "" || name == preferred
}

func appendIfMissing(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func isPrivateIPv6(ip net.IP) bool {
	return len(ip) == net.IPv6len && ip[0]&0xfe == 0xfc
}

func detectDockerMetadata() *dockerMetadata {
	if _, err := os.Stat("/.dockerenv"); err != nil && !strings.Contains(readFile("/proc/1/cgroup"), "docker") {
		return nil
	}

	metadata := &dockerMetadata{
		ContainerID: dockerContainerID(),
		Image:       getenv("HASHI_PULSE_DOCKER_IMAGE", ""),
		NetworkMode: getenv("HASHI_PULSE_DOCKER_NETWORK_MODE", ""),
	}
	if metadata.ContainerID == "" && metadata.Image == "" && metadata.NetworkMode == "" {
		return nil
	}
	return metadata
}

func dockerContainerID() string {
	cgroup := readFile("/proc/1/cgroup")
	for _, part := range strings.FieldsFunc(cgroup, func(r rune) bool {
		return r == '/' || r == ':' || r == '\n'
	}) {
		part = strings.TrimSpace(part)
		if len(part) >= 12 && isHex(part) {
			return part
		}
	}
	if hostname, err := os.Hostname(); err == nil && len(hostname) >= 12 && isHex(hostname) {
		return hostname
	}
	return ""
}

func isHex(value string) bool {
	for _, ch := range value {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') && (ch < 'A' || ch > 'F') {
			return false
		}
	}
	return true
}

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
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
