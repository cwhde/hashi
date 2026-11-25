package app

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"
)

// Invariant 6: timestamps are UTC. Stored timestamps are Unix milliseconds (zone-free by construction,
// see internal/store/migrations); this covers the one rendered timestamp the process emits itself.
func TestInvariant_6_LogTimestampsAreUTC(t *testing.T) {
	var buf bytes.Buffer
	local := time.Date(2026, 10, 27, 15, 0, 0, 0, time.FixedZone("UTC+5", 5*3600))
	if err := NewLogger(&buf, slog.LevelInfo).Handler().Handle(context.Background(), slog.NewRecord(local, slog.LevelInfo, "x", 0)); err != nil {
		t.Fatal(err)
	}
	var line struct {
		Time string `json:"time"`
	}
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatal(err)
	}
	ts, err := time.Parse(time.RFC3339Nano, line.Time)
	if err != nil {
		t.Fatal(err)
	}
	if _, off := ts.Zone(); off != 0 || line.Time[len(line.Time)-1] != 'Z' || !ts.Equal(local) {
		t.Fatalf("log time %q is not UTC", line.Time)
	}
}
