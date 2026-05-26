package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

type heartbeatPayload struct {
	Token                   string   `json:"token"`
	Version                 string   `json:"version"`
	Hostname                string   `json:"hostname"`
	PrivateIpv4Candidates   []string `json:"privateIpv4Candidates"`
}

func main() {
	apiURL := flag.String("api", os.Getenv("HASHI_PULSE_API"), "Hashi API base URL")
	agentID := flag.String("agent-id", os.Getenv("HASHI_PULSE_AGENT_ID"), "Pulse agent ID")
	token := flag.String("token", os.Getenv("HASHI_PULSE_TOKEN"), "Pulse agent token")
	interval := flag.Duration("interval", 60*time.Second, "Heartbeat interval")
	flag.Parse()

	if *apiURL == "" || *agentID == "" || *token == "" {
		fmt.Fprintln(os.Stderr, "api, agent-id, and token are required")
		os.Exit(1)
	}

	hostname, _ := os.Hostname()
	client := &http.Client{Timeout: 15 * time.Second}
	endpoint := fmt.Sprintf("%s/api/pulse/%s/heartbeat", trimSlash(*apiURL), *agentID)

	for {
		payload := heartbeatPayload{
			Token:                 *token,
			Version:               "0.1.0",
			Hostname:              hostname,
			PrivateIpv4Candidates: collectPrivateIPv4(),
		}
		body, _ := json.Marshal(payload)
		req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
			}
		}
		time.Sleep(*interval)
	}
}

func collectPrivateIPv4() []string {
	var ips []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return ips
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip = ip.To4()
			if ip == nil {
				continue
			}
			if isPrivateIPv4(ip) {
				ips = append(ips, ip.String())
			}
		}
	}
	return ips
}

func isPrivateIPv4(ip net.IP) bool {
	return ip[0] == 10 ||
		(ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31) ||
		(ip[0] == 192 && ip[1] == 168)
}

func trimSlash(value string) string {
	for len(value) > 0 && value[len(value)-1] == '/' {
		value = value[:len(value)-1]
	}
	return value
}
