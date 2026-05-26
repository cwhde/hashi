package main

import (
	"net"
	"testing"
)

func TestIsPrivateIPv4(t *testing.T) {
	tests := []struct {
		ip   string
		want bool
	}{
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"8.8.8.8", false},
	}
	for _, tt := range tests {
		ip := net.ParseIP(tt.ip).To4()
		if ip == nil {
			t.Fatalf("invalid ip %s", tt.ip)
		}
		if got := isPrivateIPv4(ip); got != tt.want {
			t.Fatalf("isPrivateIPv4(%s) = %v, want %v", tt.ip, got, tt.want)
		}
	}
}

func TestTrimSlash(t *testing.T) {
	if trimSlash("https://hashi/") != "https://hashi" {
		t.Fatalf("unexpected trim result")
	}
}
