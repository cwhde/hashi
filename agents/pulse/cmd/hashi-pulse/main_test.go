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
