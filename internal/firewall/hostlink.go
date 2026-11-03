package firewall

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"sync"
	"time"

	"git.juzo.io/juzo/hashi/internal/security"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Host link (05 §6): one persistent SSH connection per managed host that drains the kernel's staging sets into bans
// and keeps the dyn sets equal to dynamic-blocked. Its writes are the one exception to plan → apply (invariant 7).

// ReconcileEvery is how often the link reads dyn back from the host and repairs differences.
const ReconcileEvery = 5 * time.Minute

// Link runs the host links.
type Link struct {
	m   *Manager
	eng *security.Engine
	log *slog.Logger

	wake chan struct{}
	mu   sync.Mutex
	runs map[string]context.CancelFunc
}

// NewLink returns the host links for m's managed hosts.
func NewLink(m *Manager, eng *security.Engine, log *slog.Logger) *Link {
	return &Link{m: m, eng: eng, log: log, wake: make(chan struct{}, 1), runs: map[string]context.CancelFunc{}}
}

// Wake makes every link push now instead of at its next tick (a ban was created or lifted).
func (l *Link) Wake() {
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

// Run starts a link per managed host with Hashi's table applied and stops links of hosts that are gone, until ctx ends.
func (l *Link) Run(ctx context.Context) {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	wakes := map[string]chan struct{}{}
	for {
		hosts, err := l.m.st.R.ListFirewallHosts(ctx)
		if err != nil && ctx.Err() == nil {
			l.log.Warn("host link: list hosts", "err", err)
		}
		want := map[string]bool{}
		for _, h := range hosts {
			if h.EdgeMode == ModeManaged && h.LastAppliedAt.Valid {
				want[h.ID] = true
			}
		}
		l.mu.Lock()
		for id, stop := range l.runs {
			if !want[id] {
				stop()
				delete(l.runs, id)
				delete(wakes, id)
			}
		}
		for id := range want {
			if _, ok := l.runs[id]; !ok {
				hctx, stop := context.WithCancel(ctx)
				l.runs[id] = stop
				w := make(chan struct{}, 1)
				wakes[id] = w
				go l.host(hctx, id, w)
			}
		}
		l.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-l.wake:
			for _, w := range wakes {
				select {
				case w <- struct{}{}:
				default:
				}
			}
		}
	}
}

func (l *Link) poll() time.Duration {
	return time.Duration(l.eng.Settings().PollIntervalSecs) * time.Second
}

func (l *Link) state(ctx context.Context, id, state string, poll, push bool, lag time.Duration, msg string) {
	now := l.m.now().UnixMilli()
	p := db.PutFirewallHostLinkParams{FirewallHostID: id, State: state, LagMs: lag.Milliseconds(), Error: msg, UpdatedAt: now}
	if poll {
		p.LastPollAt.Int64, p.LastPollAt.Valid = now, true
	}
	if push {
		p.LastPushAt.Int64, p.LastPushAt.Valid = now, true
	}
	if err := l.m.st.Q.PutFirewallHostLink(ctx, p); err != nil && ctx.Err() == nil {
		l.log.Warn("host link state", "host", id, "err", err)
	}
}

// host keeps one host's link up: connect, tick every poll interval, reconnect with backoff after a failure.
func (l *Link) host(ctx context.Context, id string, wake <-chan struct{}) {
	backoff := time.Second
	for ctx.Err() == nil {
		row, err := l.m.st.R.GetFirewallHost(ctx, id)
		if err != nil {
			return
		}
		l.state(ctx, id, "connecting", false, false, 0, "")
		conn, err := l.m.dial(ctx, row)
		if err != nil {
			l.state(ctx, id, "down", false, false, 0, err.Error())
			if !sleep(ctx, backoff) {
				return
			}
			backoff = min(2*backoff, time.Minute)
			continue
		}
		backoff = time.Second
		err = l.session(ctx, row, conn, wake)
		_ = conn.Close()
		if ctx.Err() != nil {
			return
		}
		l.state(ctx, id, "down", false, false, 0, err.Error())
		l.log.Warn("host link down", "host", row.Name, "err", err)
		if !sleep(ctx, backoff) {
			return
		}
	}
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// session runs ticks on one connection until an error. The first tick reads dyn back (step 5 on every reconnect).
func (l *Link) session(ctx context.Context, row db.FirewallHost, h Host, wake <-chan struct{}) error {
	nft := EffectiveOf(row).Facts.NftPath
	if nft == "" {
		nft = "nft"
	}
	s := &linkSession{l: l, row: row, h: h, nft: nft}
	for {
		if err := s.tick(ctx); err != nil {
			return err
		}
		t := time.NewTimer(l.poll())
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		case <-wake:
			t.Stop()
		}
	}
}

type linkSession struct {
	l   *Link
	row db.FirewallHost
	h   Host
	nft string
	// believed is what dyn holds on the host: prefix → expiry (zero: no timeout). nil until read back.
	believed   map[netip.Prefix]time.Time
	reconciled time.Time
}

// tick is one poll: read dyn back when due, drain staging into bans, push one batch (05 §6 steps 1–5).
func (s *linkSession) tick(ctx context.Context) error {
	l, now := s.l, s.l.m.now()
	start := time.Now()
	if s.believed == nil || now.Sub(s.reconciled) >= ReconcileEvery {
		got, err := s.list(ctx, "dyn4", "dyn6")
		if err != nil {
			return err
		}
		s.believed, s.reconciled = map[netip.Prefix]time.Time{}, now
		for _, e := range got {
			s.believed[e.prefix] = e.expiry(now)
		}
	}
	staged, err := s.list(ctx, "staging4", "staging6")
	if err != nil {
		return err
	}
	for _, e := range staged {
		if _, err := l.eng.Ban(ctx, e.prefix.Addr(), security.Cause{Source: security.SourceMeter, HostID: s.row.ID,
			Reason: "tripped an L4 meter on " + s.row.Name}); err != nil {
			return err // stays in staging until the next tick
		}
	}

	var batch []any
	desired := map[netip.Prefix]time.Time{}
	var added []security.Entry
	for _, b := range l.eng.Bans() {
		if b.Prefix.Addr().Is4() && l.eng.Allowed(b.Prefix.Addr()) {
			continue // dyn holds bans minus allowed ranges (05 §3.2)
		}
		desired[b.Prefix] = b.Expires
		have, ok := s.believed[b.Prefix]
		if ok && closeTo(have, b.Expires) {
			continue
		}
		el, ok := dynElem(b.Prefix, b.Expires, now)
		if !ok {
			continue // ends within the second
		}
		batch = append(batch, obj{"add": obj{"element": obj{"family": "inet", "table": TableName, "name": dynName(b.Prefix), "elem": []any{el}}}})
		added = append(added, b)
	}
	var removed []netip.Prefix
	for p := range s.believed {
		if _, ok := desired[p]; !ok {
			batch = append(batch, obj{"destroy": obj{"element": obj{"family": "inet", "table": TableName, "name": dynName(p), "elem": []any{prefixVal(p)}}}})
			removed = append(removed, p)
		}
	}
	for _, e := range staged {
		name := "staging4"
		if !e.prefix.Addr().Is4() {
			name = "staging6"
		}
		batch = append(batch, obj{"destroy": obj{"element": obj{"family": "inet", "table": TableName, "name": name, "elem": []any{e.prefix.Addr().String()}}}})
	}
	pushed := false
	if len(batch) > 0 {
		in, err := json.Marshal(obj{"nftables": batch})
		if err != nil {
			return err
		}
		r, err := s.h.Run(ctx, s.nft+" -j -f -", bytes.NewReader(in))
		if err != nil {
			return err
		}
		if r.ExitCode != 0 {
			for _, b := range added {
				s.mark(ctx, b.ElementID, "failed", string(r.Stderr))
				l.eng.HostEvent(b.Prefix, "ban_apply_failed", "warning", s.row.ID, string(bytes.TrimSpace(r.Stderr)))
			}
			s.believed = nil // read back next tick
			return fmt.Errorf("push to %s: %s", s.row.Name, bytes.TrimSpace(r.Stderr))
		}
		pushed = true
		// Invariant 7: each change to dyn made here, outside a plan, is a security event for this host.
		for _, b := range added {
			s.mark(ctx, b.ElementID, "applied", "")
			l.eng.HostEvent(b.Prefix, "ban_applied", "info", s.row.ID, "in the kernel ban set on "+s.row.Name)
		}
		for _, p := range removed {
			l.eng.HostEvent(p, "ban_removed_host", "info", s.row.ID, "taken off the kernel ban set on "+s.row.Name)
		}
		s.believed = desired
	}
	l.state(ctx, s.row.ID, "connected", true, pushed, time.Since(start), "")
	return nil
}

func (s *linkSession) mark(ctx context.Context, elementID, state, msg string) {
	p := db.InsertDynamicEntryHostParams{ElementID: elementID, FirewallHostID: s.row.ID, State: state, Error: msg}
	if state == "applied" {
		p.AppliedAt.Int64, p.AppliedAt.Valid = s.l.m.now().UnixMilli(), true
	}
	if err := s.l.m.st.Q.InsertDynamicEntryHost(ctx, p); err != nil && ctx.Err() == nil {
		s.l.log.Warn("host link: entry state", "host", s.row.Name, "err", err)
	}
}

// closeTo: the host reports whole seconds left, so expiries within a few seconds are the same entry.
func closeTo(a, b time.Time) bool {
	if a.IsZero() || b.IsZero() {
		return a.IsZero() == b.IsZero()
	}
	d := a.Sub(b)
	return d > -5*time.Second && d < 5*time.Second
}

func dynName(p netip.Prefix) string {
	if p.Addr().Is4() {
		return "dyn4"
	}
	return "dyn6"
}

// dynElem is one dyn element with its remaining time; ok is false when it has already ended.
func dynElem(p netip.Prefix, expires, now time.Time) (any, bool) {
	if expires.IsZero() {
		return prefixVal(p), true // permanent: no timeout
	}
	left := int(expires.Sub(now).Round(time.Second) / time.Second)
	if left < 1 {
		return nil, false
	}
	return obj{"elem": obj{"val": prefixVal(p), "timeout": left}}, true
}

type listedElem struct {
	prefix  netip.Prefix
	expires int // seconds left; -1 without a timeout
}

func (e listedElem) expiry(now time.Time) time.Time {
	if e.expires < 0 {
		return time.Time{}
	}
	return now.Add(time.Duration(e.expires) * time.Second)
}

// list reads sets of Hashi's table in one command. A missing set means the table is not loaded.
func (s *linkSession) list(ctx context.Context, names ...string) ([]listedElem, error) {
	cmd := ""
	for i, n := range names {
		if i > 0 {
			cmd += " && "
		}
		cmd += s.nft + " -j list set inet " + TableName + " " + n
	}
	r, err := s.h.Run(ctx, cmd, nil)
	if err != nil {
		return nil, err
	}
	if r.ExitCode != 0 {
		return nil, fmt.Errorf("Hashi's table on %s has no %v sets: %s", s.row.Name, names, bytes.TrimSpace(r.Stderr)) //nolint:staticcheck // names a table
	}
	return parseSetElems(r.Stdout)
}

// parseSetElems reads the elements of `nft -j list set` output, one or more documents in a row
// (testdata/contracts/nft/dynamic_sets.txt, finding 7): a bare value, or {"elem":{"val","timeout","expires"}}.
func parseSetElems(b []byte) ([]listedElem, error) {
	var out []listedElem
	dec := json.NewDecoder(bytes.NewReader(b))
	for {
		var doc struct {
			Nftables []struct {
				Set *struct {
					Elem []json.RawMessage `json:"elem"`
				} `json:"set"`
			} `json:"nftables"`
		}
		if err := dec.Decode(&doc); errors.Is(err, io.EOF) {
			return out, nil
		} else if err != nil {
			return nil, fmt.Errorf("read set listing: %w", err)
		}
		for _, o := range doc.Nftables {
			if o.Set == nil {
				continue
			}
			for _, raw := range o.Set.Elem {
				e, err := parseElem(raw)
				if err != nil {
					return nil, err
				}
				out = append(out, e)
			}
		}
	}
}

func parseElem(raw json.RawMessage) (listedElem, error) {
	var wrapped struct {
		Elem *struct {
			Val     json.RawMessage `json:"val"`
			Expires *int            `json:"expires"`
		} `json:"elem"`
	}
	val, left := raw, -1
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Elem != nil {
		val = wrapped.Elem.Val
		if wrapped.Elem.Expires != nil {
			left = *wrapped.Elem.Expires
		}
	}
	var s string
	if err := json.Unmarshal(val, &s); err == nil {
		a, err := netip.ParseAddr(s)
		if err != nil {
			return listedElem{}, fmt.Errorf("set element %q: %w", s, err)
		}
		return listedElem{prefix: netip.PrefixFrom(a, a.BitLen()), expires: left}, nil
	}
	var p struct {
		Prefix struct {
			Addr string `json:"addr"`
			Len  int    `json:"len"`
		} `json:"prefix"`
	}
	if err := json.Unmarshal(val, &p); err != nil {
		return listedElem{}, fmt.Errorf("set element %s: %w", val, err)
	}
	a, err := netip.ParseAddr(p.Prefix.Addr)
	if err != nil {
		return listedElem{}, fmt.Errorf("set element %s: %w", val, err)
	}
	return listedElem{prefix: netip.PrefixFrom(a, p.Prefix.Len).Masked(), expires: left}, nil
}
