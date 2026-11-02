package security

import (
	"context"
	"fmt"
	"net/netip"
	"time"

	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Step is Stage A's answer for one request (06 §3).
type Step int

// Stage A outcomes. Pass continues to Stage B.
const (
	Pass       Step = iota
	Blocked         // A1: folded block or dynamic-blocked
	Challenged      // A3: must solve a CAPTCHA first
	BannedNow       // A3 reached F_max: escalated
)

// Request is what Stage A needs from one forward-auth call.
type Request struct {
	Addr netip.Addr
	// SSO: the request carries a valid edge session, so it does not count towards the rate trigger (06 §3.2).
	SSO bool
	// ChallengeFlow: the challenge page, its assets or verify. Only A1 applies, and nothing counts as a failure.
	ChallengeFlow bool
	// StaticBlock: the address is in the folded block ranges (A1's static half, from the firewall partition).
	StaticBlock bool
	ServiceID   string
}

// Outcome is the step with what the trace and the challenge page show.
type Outcome struct {
	Step      Step
	Stage     string // "A1", "A2", "A3" or "" when Stage A passed
	Failures  int
	MaxFailed int
	Expires   time.Time
}

// window counts events in a sliding window, approximated from the current and the previous fixed bucket.
type window struct {
	start     int64
	cur, prev int
	last      int64
}

func (w *window) hit(now time.Time, size time.Duration) float64 {
	t, s := now.UnixNano(), int64(size)
	b := t - t%s
	if b != w.start {
		if b-w.start == s {
			w.prev = w.cur
		} else {
			w.prev = 0
		}
		w.cur, w.start = 0, b
	}
	w.cur++
	w.last = t
	frac := float64(t-b) / float64(s)
	return float64(w.prev)*(1-frac) + float64(w.cur)
}

// StageA runs A1, A2 and A3 in that order for one request. An allowed address passes untouched (invariant 20).
func (e *Engine) StageA(ctx context.Context, r Request) (Outcome, error) {
	a := r.Addr.Unmap()
	if e.Allowed(a) {
		return Outcome{Step: Pass}, nil
	}
	// A1 — block guard.
	if r.StaticBlock || e.Banned(a) {
		return Outcome{Step: Blocked, Stage: "A1"}, nil
	}
	if r.ChallengeFlow {
		return Outcome{Step: Pass}, nil
	}
	cfg := e.Settings()
	k, now := Key(a), e.now()

	// A2 — request rate. Only counted while not already challenged: a challenged address is A3's.
	e.mu.Lock()
	ch, was := e.challenged[k]
	if was && !ch.live(now) {
		delete(e.challenged, k) // the element goes with the IP set expiry job
		was = false
	}
	trigger := false
	if !was && !r.SSO {
		w := e.rate[k]
		if w == nil {
			w = &window{}
			e.rate[k] = w
		}
		trigger = w.hit(now, time.Duration(cfg.RateWindowSeconds)*time.Second) > float64(cfg.RateRequests)
	}
	e.mu.Unlock()

	if trigger {
		if _, err := e.Challenge(ctx, a, Cause{Source: SourceRate, ServiceID: r.ServiceID,
			Reason: fmt.Sprintf("more than %d requests within %d s", cfg.RateRequests, cfg.RateWindowSeconds)}); err != nil {
			return Outcome{}, err
		}
		e.mu.Lock()
		delete(e.rate, k)
		x := e.challenged[k]
		e.mu.Unlock()
		o := Outcome{Step: Challenged, Stage: "A2", MaxFailed: cfg.ChallengeMaxFailed}
		if x != nil {
			o.Expires = x.Expires
		}
		return o, nil
	}
	if !was {
		return Outcome{Step: Pass}, nil
	}

	// A3 — challenge enforcement: every other request is a failure.
	e.mu.Lock()
	ch, ok := e.challenged[k]
	if !ok {
		e.mu.Unlock()
		return Outcome{Step: Pass}, nil
	}
	ch.Failures++
	ch.dirty = true
	f, exp := ch.Failures, ch.Expires
	e.mu.Unlock()
	if f >= cfg.ChallengeMaxFailed {
		if _, err := e.Ban(ctx, a, Cause{Source: SourceChallengeFailed, ServiceID: r.ServiceID,
			Reason: fmt.Sprintf("%d requests without solving the challenge", f)}); err != nil {
			return Outcome{}, err
		}
		return Outcome{Step: BannedNow, Stage: "A3", Failures: f, MaxFailed: cfg.ChallengeMaxFailed}, nil
	}
	return Outcome{Step: Challenged, Stage: "A3", Failures: f, MaxFailed: cfg.ChallengeMaxFailed, Expires: exp}, nil
}

// WAFMatch counts one WAF match from the access log (06 §3.2); SSO sessions do not exempt it.
func (e *Engine) WAFMatch(ctx context.Context, a netip.Addr, serviceID string) error {
	a = a.Unmap()
	if e.Allowed(a) {
		return nil
	}
	cfg := e.Settings()
	k, now := Key(a), e.now()
	e.mu.Lock()
	w := e.waf[k]
	if w == nil {
		w = &window{}
		e.waf[k] = w
	}
	n := w.hit(now, time.Duration(cfg.WAFWindowSeconds)*time.Second)
	e.mu.Unlock()
	e.ev.add(event{at: now, key: k, typ: "waf_match", severity: "warning", source: SourceWAF, service: serviceID})
	if n < float64(cfg.WAFViolations) {
		return nil
	}
	if _, err := e.Challenge(ctx, a, Cause{Source: SourceWAF, ServiceID: serviceID,
		Reason: fmt.Sprintf("%d WAF matches within %d s", cfg.WAFViolations, cfg.WAFWindowSeconds)}); err != nil {
		return err
	}
	e.mu.Lock()
	delete(e.waf, k)
	e.mu.Unlock()
	return nil
}

// Solve ends a's challenge after a verified CAPTCHA (scope global). The ban counter stays (06 §3.3).
func (e *Engine) Solve(ctx context.Context, a netip.Addr) (bool, error) {
	k := Key(a)
	e.mu.Lock()
	x, ok := e.challenged[k]
	delete(e.challenged, k)
	e.mu.Unlock()
	if !ok {
		return false, nil
	}
	if err := e.st.Q.DeleteDynamicElement(ctx, x.ElementID); err != nil {
		return false, fmt.Errorf("solve %s: %w", k, err)
	}
	e.ev.add(event{at: e.now(), key: k, typ: "challenge_solved", severity: "info", source: x.Source, service: x.ServiceID,
		meta: map[string]any{"failures": x.Failures}})
	e.notify()
	return true, nil
}

// SweepJob drops ended entries and idle windows from memory and stores failure counts (every minute).
const (
	SweepJobKind     = "security.sweep"
	SweepJobInterval = time.Minute
)

// Sweep is the periodic job.
func (e *Engine) Sweep(ctx context.Context) (string, error) {
	now := e.now()
	cfg := e.Settings()
	idleRate := 2 * time.Duration(cfg.RateWindowSeconds) * time.Second
	idleWAF := 2 * time.Duration(cfg.WAFWindowSeconds) * time.Second
	type fc struct {
		id string
		f  int
	}
	var dirty []fc
	ended := 0
	e.mu.Lock()
	for k, x := range e.challenged {
		if !x.live(now) {
			delete(e.challenged, k)
			ended++
			continue
		}
		if x.dirty {
			dirty = append(dirty, fc{x.ElementID, x.Failures})
			x.dirty = false
		}
	}
	for k, x := range e.banned {
		if !x.live(now) {
			delete(e.banned, k)
			ended++
		}
	}
	for k, w := range e.rate {
		if now.UnixNano()-w.last > int64(idleRate) {
			delete(e.rate, k)
		}
	}
	for k, w := range e.waf {
		if now.UnixNano()-w.last > int64(idleWAF) {
			delete(e.waf, k)
		}
	}
	e.mu.Unlock()
	for _, d := range dirty {
		if err := e.st.Q.SetDynamicEntryFailures(ctx, db.SetDynamicEntryFailuresParams{FailureCount: int64(d.f), ElementID: d.id}); err != nil {
			return "", fmt.Errorf("store failures: %w", err)
		}
	}
	e.ev.flush(ctx)
	if ended == 0 {
		return "", nil
	}
	return fmt.Sprintf("%d dynamic entries ended", ended), nil
}
