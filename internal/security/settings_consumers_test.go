package security

import (
	"context"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/settings"
)

// The WAF threshold and window (06 §10, 09 §2): WAFViolations matches within WAFWindowSeconds challenge the address;
// matches spread wider than the window do not.
func TestSettingsWAFViolationsAndWindow(t *testing.T) {
	ctx := context.Background()
	e, c := testEngine(t)
	cfg := e.Settings()
	cfg.WAFViolations, cfg.WAFWindowSeconds = 2, 30
	e.UseSettings(cfg)
	if err := e.WAFMatch(ctx, bad, "", WAFDetail{}); err != nil {
		t.Fatal(err)
	}
	c.add(40 * time.Second) // outside the window: the first match no longer counts
	if err := e.WAFMatch(ctx, bad, "", WAFDetail{}); err != nil {
		t.Fatal(err)
	}
	if len(e.Challenges()) != 0 {
		t.Fatal("two matches 40 s apart challenged with a 30 s window")
	}
	c.add(5 * time.Second)
	if err := e.WAFMatch(ctx, bad, "", WAFDetail{}); err != nil {
		t.Fatal(err)
	}
	if len(e.Challenges()) != 1 {
		t.Fatal("two matches within the window did not challenge")
	}
}

// ChallengeExpirySecs (06 §10): a challenge ends after the configured time; 0 keeps it until solved or banned.
func TestSettingsChallengeExpiry(t *testing.T) {
	ctx := context.Background()
	e, c := testEngine(t)
	cfg := e.Settings()
	cfg.ChallengeExpirySecs = 120
	e.UseSettings(cfg)
	if _, err := e.Challenge(ctx, bad, Cause{Source: SourceManual}); err != nil {
		t.Fatal(err)
	}
	c.add(119 * time.Second)
	if o := run(t, e, Request{Addr: bad}); o.Step == Pass {
		t.Fatal("challenge ended early")
	}
	c.add(2 * time.Second)
	if o := run(t, e, Request{Addr: bad}); o.Step != Pass {
		t.Fatalf("challenge outlived its 120 s: %+v", o)
	}
	cfg.ChallengeExpirySecs = 0
	e.UseSettings(cfg)
	other := bad.Next()
	if _, err := e.Challenge(ctx, other, Cause{Source: SourceManual}); err != nil {
		t.Fatal(err)
	}
	c.add(365 * 24 * time.Hour)
	if o := run(t, e, Request{Addr: other}); o.Step == Pass {
		t.Fatal("a challenge without expiry ended")
	}
}

// The ban curve settings (06 §5.2) reach T_ban: function, base, factor and cap.
func TestSettingsBanCurve(t *testing.T) {
	e := settings.DefaultOf[settings.Engine]()
	e.BanFunction, e.BanBaseSeconds, e.BanK, e.BanMaxSeconds = settings.BanLinear, 600, 3, 3600
	p := BanParamsFrom(e)
	for c, want := range map[int]time.Duration{1: 10 * time.Minute, 2: 40 * time.Minute, 3: time.Hour} { // 10, 10+30, capped
		if got, perm := p.Duration(c); got != want || perm {
			t.Errorf("linear T_ban(%d) = %v, want %v", c, got, want)
		}
	}
	e.BanFunction = settings.BanConstant
	if got, _ := BanParamsFrom(e).Duration(9); got != 10*time.Minute {
		t.Errorf("constant T_ban(9) = %v", got)
	}
}
