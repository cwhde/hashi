package store

import (
	"context"
	"testing"
)

// Migration 00032 (bug 26): an install on the old request limit default (300 in 10 s) moves to the measured 1600; a
// limit the owner chose stays, and a challenge can come from the error trigger. Down puts both back.
func TestMigration32RateFromMeasurement(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	p, err := s.provider()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 31); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Writer.ExecContext(ctx, `INSERT INTO app_settings (key, value_json, updated_at) VALUES
		('engine', '{"rate_requests":300,"rate_window_seconds":10,"waf_violations":5}', 1)`); err != nil {
		t.Fatal(err)
	}
	rate := func() (n int) {
		t.Helper()
		if err := s.Writer.QueryRowContext(ctx,
			`SELECT json_extract(value_json, '$.rate_requests') FROM app_settings WHERE key = 'engine'`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if _, err := p.UpTo(ctx, 32); err != nil {
		t.Fatal(err)
	}
	if n := rate(); n != 1600 {
		t.Fatalf("old default became %d, want 1600", n)
	}
	if _, err := p.DownTo(ctx, 31); err != nil {
		t.Fatal(err)
	}
	if n := rate(); n != 300 {
		t.Fatalf("down left %d, want 300", n)
	}
	if _, err := s.Writer.ExecContext(ctx,
		`UPDATE app_settings SET value_json = json_set(value_json, '$.rate_requests', 500) WHERE key = 'engine'`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 32); err != nil {
		t.Fatal(err)
	}
	if n := rate(); n != 500 {
		t.Fatalf("a chosen limit changed to %d", n)
	}
}
