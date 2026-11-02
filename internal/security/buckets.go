package security

import (
	"context"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Decisions counted per minute (06 §6.1).
const (
	DecisionAllow     = "allow"
	DecisionDeny      = "deny"
	DecisionChallenge = "challenge"
	DecisionBlocked   = "blocked"
	DecisionAuth      = "auth_required"
)

const (
	maxPrefixes = 50  // path prefixes per service per minute, then "other"
	topPerMin   = 500 // addresses kept per closed minute
)

// Hit is one forward-auth decision.
type Hit struct {
	At        time.Time
	Addr      netip.Addr
	ServiceID string
	Decision  string
	Path      string // normalized
}

type bucketKey struct {
	minute                     int64
	service, country, decision string
	prefix                     string
	asn                        int64
}

type bucketCount struct{ requests, denied, challenged, failed, banned int64 }

type topKey struct {
	minute  int64
	addr    string
	service string
}

type topCount struct{ requests, denied, challenged int64 }

// buckets aggregates hits in memory per minute; closed minutes are written by Flush. No address reaches the buckets.
type buckets struct {
	mu       sync.Mutex
	b        map[bucketKey]*bucketCount
	top      map[topKey]*topCount
	prefixes map[string]map[string]bool // minute/service → prefixes seen
}

func newBuckets() *buckets {
	return &buckets{b: map[bucketKey]*bucketCount{}, top: map[topKey]*topCount{}, prefixes: map[string]map[string]bool{}}
}

func firstSegment(p string) string {
	p = strings.TrimPrefix(p, "/")
	if i := strings.IndexByte(p, '/'); i >= 0 {
		p = p[:i]
	}
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	if len(p) > 64 {
		p = p[:64]
	}
	return "/" + p
}

// Record counts one decision.
func (e *Engine) Record(h Hit) {
	country, asn := e.lookup(h.Addr.Unmap())
	minute := h.At.Unix() / 60 * 60_000
	b := e.buckets
	b.mu.Lock()
	defer b.mu.Unlock()
	pk := fmt.Sprintf("%d/%s", minute, h.ServiceID)
	seen := b.prefixes[pk]
	if seen == nil {
		seen = map[string]bool{}
		b.prefixes[pk] = seen
	}
	prefix := firstSegment(h.Path)
	if !seen[prefix] {
		if len(seen) >= maxPrefixes {
			prefix = "other"
		} else {
			seen[prefix] = true
		}
	}
	k := bucketKey{minute: minute, service: h.ServiceID, country: country, asn: asn, decision: h.Decision, prefix: prefix}
	c := b.b[k]
	if c == nil {
		c = &bucketCount{}
		b.b[k] = c
	}
	c.requests++
	tk := topKey{minute: minute, addr: KeyString(h.Addr), service: h.ServiceID}
	t := b.top[tk]
	if t == nil {
		t = &topCount{}
		b.top[tk] = t
	}
	t.requests++
	switch h.Decision {
	case DecisionDeny, DecisionBlocked:
		c.denied++
		t.denied++
	case DecisionChallenge:
		c.challenged++
		t.challenged++
	}
}

// flushBuckets writes every closed minute and forgets it.
func (e *Engine) flushBuckets(ctx context.Context, now time.Time) error {
	open := now.Unix() / 60 * 60_000
	b := e.buckets
	b.mu.Lock()
	var rows []db.AddSecurityBucketParams
	for k, c := range b.b {
		if k.minute >= open {
			continue
		}
		rows = append(rows, db.AddSecurityBucketParams{BucketStart: k.minute, ServiceID: k.service, Country: k.country,
			Asn: k.asn, Decision: k.decision, PathPrefix: k.prefix, RequestCount: c.requests, DeniedCount: c.denied,
			ChallengedCount: c.challenged, ChallengeFailedCount: c.failed, BannedCount: c.banned})
		delete(b.b, k)
	}
	byMinute := map[int64][]db.AddSecurityTopParams{}
	for k, c := range b.top {
		if k.minute >= open {
			continue
		}
		byMinute[k.minute] = append(byMinute[k.minute], db.AddSecurityTopParams{BucketStart: k.minute, Address: k.addr,
			ServiceID: k.service, RequestCount: c.requests, DeniedCount: c.denied, ChallengedCount: c.challenged})
		delete(b.top, k)
	}
	for k := range b.prefixes {
		var m int64
		if _, err := fmt.Sscanf(k, "%d/", &m); err == nil && m < open {
			delete(b.prefixes, k)
		}
	}
	b.mu.Unlock()
	if len(rows) == 0 && len(byMinute) == 0 {
		return nil
	}
	return e.st.Tx(ctx, func(q *db.Queries) error {
		for _, r := range rows {
			if err := q.AddSecurityBucket(ctx, r); err != nil {
				return err
			}
		}
		for _, ts := range byMinute {
			sort.Slice(ts, func(i, j int) bool { return ts[i].RequestCount > ts[j].RequestCount })
			for _, t := range ts[:min(len(ts), topPerMin)] {
				if err := q.AddSecurityTop(ctx, t); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
