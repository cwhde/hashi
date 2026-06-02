package main

import "testing"

func TestDurationEnvFallback(t *testing.T) {
	t.Setenv("HASHI_PULSE_INTERVAL", "not-a-duration")
	if got := durationEnv("HASHI_PULSE_INTERVAL", 42); got != 42 {
		t.Fatalf("durationEnv() = %v, want fallback", got)
	}
}

func TestPrivateIPv4CandidatesDoesNotPanic(t *testing.T) {
	_ = privateIPv4Candidates()
}

func TestHeartbeatPayloadIncludesTimestampIPv6AndSelection(t *testing.T) {
	payload := heartbeatRequest{
		Token:                 "token",
		Version:               version,
		Hostname:              "host",
		PrivateIpv4Candidates: []string{"10.0.0.5"},
		PrivateIpv6Candidates: []string{"fd00::5"},
		SelectedInterface:     "eth0",
		SelectedIP:            "fd00::5",
		Timestamp:             "2026-06-01T12:00:00Z",
		Docker:                &dockerMetadata{ContainerID: "abc123def456", Image: "hashi-pulse:latest", NetworkMode: "bridge"},
	}

	if payload.Timestamp == "" {
		t.Fatal("timestamp is required")
	}
	if len(payload.PrivateIpv6Candidates) != 1 || payload.PrivateIpv6Candidates[0] != "fd00::5" {
		t.Fatalf("PrivateIpv6Candidates = %#v, want fd00::5", payload.PrivateIpv6Candidates)
	}
	if payload.SelectedInterface != "eth0" || payload.SelectedIP != "fd00::5" {
		t.Fatalf("selection = %s/%s, want eth0/fd00::5", payload.SelectedInterface, payload.SelectedIP)
	}
	if payload.Docker == nil || payload.Docker.NetworkMode != "bridge" {
		t.Fatalf("Docker = %#v, want bridge metadata", payload.Docker)
	}
}

func TestIsPrivateIPv6(t *testing.T) {
	if !isPrivateIPv6([]byte{0xfd, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}) {
		t.Fatal("fd00::1 should be private")
	}
	if isPrivateIPv6([]byte{0x20, 0x01, 0x0d, 0xb8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}) {
		t.Fatal("2001:db8::1 should not be private")
	}
}
