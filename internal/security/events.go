package security

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// perMinute is how many events one address may store per minute; more add to the last one's suppressed count (06 §6.1).
const perMinute = 10

type event struct {
	at                    time.Time
	key                   netip.Prefix
	typ, severity         string
	decision, source      string
	reason, service, host string
	method, path          string
	status                int
	meta                  map[string]any
}

type budget struct {
	minute   int64
	count    int
	lastID   string
	overflow int64
}

// events writes security events off the decision path through a bounded queue; a full queue drops and counts.
type events struct {
	st      *store.Store
	log     *slog.Logger
	now     func() time.Time
	q       chan event
	dropped atomic.Int64

	mu  sync.Mutex
	per map[netip.Prefix]*budget
}

func newEvents(st *store.Store, log *slog.Logger, now func() time.Time) *events {
	return &events{st: st, log: log, now: now, q: make(chan event, 4096), per: map[netip.Prefix]*budget{}}
}

func (v *events) add(e event) {
	select {
	case v.q <- e:
	default:
		v.dropped.Add(1)
	}
}

func (v *events) run(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-v.q:
			v.write(ctx, e)
		case <-tick.C:
			v.flush(ctx)
		}
	}
}

// drain writes everything queued now (tests, shutdown).
func (v *events) drain(ctx context.Context) {
	for {
		select {
		case e := <-v.q:
			v.write(ctx, e)
		default:
			v.flush(ctx)
			return
		}
	}
}

func (v *events) write(ctx context.Context, e event) {
	minute := e.at.Unix() / 60
	v.mu.Lock()
	b := v.per[e.key]
	if b == nil || b.minute != minute {
		b = &budget{minute: minute}
		v.per[e.key] = b
	}
	if b.count >= perMinute && b.lastID != "" {
		b.overflow++
		v.mu.Unlock()
		return
	}
	b.count++
	id := ulid.Make().String()
	b.lastID = id
	v.mu.Unlock()

	meta := "{}"
	if len(e.meta) > 0 {
		if j, err := json.Marshal(e.meta); err == nil {
			meta = string(j)
		}
	}
	addr := ""
	if e.key.IsValid() {
		addr = prefixKey(e.key)
	}
	err := v.st.Q.InsertSecurityEvent(ctx, db.InsertSecurityEventParams{ID: id, OccurredAt: e.at.UnixMilli(), Address: addr,
		ServiceID: nullable(e.service), FirewallHostID: nullable(e.host), EventType: e.typ, Severity: e.severity,
		Decision: e.decision, Source: e.source, Reason: e.reason, RequestMethod: e.method, RequestPath: e.path,
		StatusCode: int64(e.status), MetadataJson: meta})
	if err != nil {
		v.log.Warn("security event not stored", "type", e.typ, "err", err)
	}
}

// flush adds suppressed counts to the last stored event of each address and forgets finished minutes.
func (v *events) flush(ctx context.Context) {
	minute := v.now().Unix() / 60
	type pending struct {
		id string
		n  int64
	}
	var out []pending
	v.mu.Lock()
	for k, b := range v.per {
		if b.overflow > 0 {
			out = append(out, pending{b.lastID, b.overflow})
			b.overflow = 0
		}
		if b.minute < minute {
			delete(v.per, k)
		}
	}
	v.mu.Unlock()
	for _, p := range out {
		if err := v.st.Q.AddSecurityEventSuppressed(ctx, db.AddSecurityEventSuppressedParams{Suppressed: p.n, ID: p.id}); err != nil {
			v.log.Warn("suppressed count not stored", "err", err)
		}
	}
}
