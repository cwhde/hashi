package edge

import (
	"context"
	"errors"
	"testing"
	"time"
)

// 06 §7: the monitor session is in memory, signed in with every provider and in every group, replaced every hour with
// the one before valid 5 more minutes, and never stored.
func TestMonitorSession(t *testing.T) {
	f := newIdP(t)
	s, st := newSSO(t, f)
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	ctx := context.Background()

	first := s.MonitorCookie()
	if s.MonitorCookie() != first {
		t.Fatal("replaced within the hour")
	}
	sess, err := s.Validate(ctx, first)
	if err != nil || !sess.Monitor || sess.Username != MonitorUser || !sess.SignedInWith("any-provider") || !sess.HasAnyGroup([]string{"admins"}) {
		t.Fatalf("%+v %v", sess, err)
	}
	var n int
	if err := st.Reader.QueryRowContext(ctx, "SELECT count(*) FROM edge_sessions").Scan(&n); err != nil || n != 0 {
		t.Fatalf("stored: %d %v", n, err)
	}

	now = now.Add(time.Hour)
	second := s.MonitorCookie()
	if second == first {
		t.Fatal("not replaced after an hour")
	}
	if _, err := s.Validate(ctx, first); err != nil {
		t.Fatalf("the one before is refused during the overlap: %v", err)
	}
	now = now.Add(6 * time.Minute)
	if _, err := s.Validate(ctx, first); !errors.Is(err, ErrNoSession) {
		t.Fatalf("the one before after the overlap: %v", err)
	}
	if _, err := s.Validate(ctx, second); err != nil {
		t.Fatal(err)
	}
	// Nobody asked for a new one: the current one ends too, after its hour and the overlap.
	now = now.Add(time.Hour)
	if _, err := s.Validate(ctx, second); !errors.Is(err, ErrNoSession) {
		t.Fatalf("a stale monitor session: %v", err)
	}
	if _, err := s.Validate(ctx, "monitor-guess"); !errors.Is(err, ErrNoSession) {
		t.Fatalf("a guess: %v", err)
	}
	if u, err := s.Logout(ctx, s.MonitorCookie(), ""); err != nil || u != "" {
		t.Fatalf("logout: %q %v", u, err)
	}
}

// A normal session is limited to its provider and its groups.
func TestSessionGroupsAndProvider(t *testing.T) {
	s := Session{ProviderID: "p", Groups: []string{"family"}}
	if s.SignedInWith("other") || s.HasAnyGroup([]string{"admins"}) || !s.HasAnyGroup([]string{"family"}) || !s.HasAnyGroup(nil) {
		t.Fatalf("%+v", s)
	}
}
