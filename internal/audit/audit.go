// Package audit writes audit_events rows. Callers pass the transaction's queries so the event commits with the
// change it describes (01-platform.md §4.1). Bearer material never reaches an event: sessions are identified by a
// salted hash prefix only (02-setup-auth.md §3.5).
package audit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Outcome of an audited action.
type Outcome string

const (
	Success Outcome = "success"
	Failure Outcome = "failure"
	Denied  Outcome = "denied"
)

// Well-known actors besides sessions.
const (
	System    = "system"
	Console   = "console"
	Anonymous = "anonymous"
)

// Event is one audit entry. Detail must not contain secrets; it is stored and shown as-is.
type Event struct {
	Actor   string
	Action  string
	Target  string
	Outcome Outcome
	IP      string
	Detail  map[string]any
}

// Log records events. It holds only the correlation salt.
type Log struct {
	salt []byte
	now  func() time.Time
}

// New returns a log whose correlation IDs are keyed with salt (an operational key from the vault).
func New(salt []byte) *Log { return &Log{salt: salt, now: time.Now} }

// SessionActor returns the actor string for a session, from sha256(session id) — never the id itself.
func (l *Log) SessionActor(idHash []byte) string { return "session:" + l.Correlate(idHash) }

// Correlate returns a short salted fingerprint usable in logs and the UI to tell sessions apart.
func (l *Log) Correlate(b []byte) string {
	m := hmac.New(sha256.New, l.salt)
	m.Write(b)
	return hex.EncodeToString(m.Sum(nil))[:12]
}

// Record writes e using q (pass the transaction's queries when the event belongs to a change).
func (l *Log) Record(ctx context.Context, q *db.Queries, e Event) error {
	detail := "{}"
	if len(e.Detail) > 0 {
		b, err := json.Marshal(e.Detail)
		if err != nil {
			return fmt.Errorf("audit detail: %w", err)
		}
		detail = string(b)
	}
	if e.Actor == "" {
		e.Actor = Anonymous
	}
	if e.Outcome == "" {
		e.Outcome = Success
	}
	err := q.InsertAuditEvent(ctx, db.InsertAuditEventParams{
		ID: ulid.Make().String(), At: l.now().UnixMilli(), Actor: e.Actor, Action: e.Action, Target: e.Target,
		Outcome: string(e.Outcome), Ip: e.IP, DetailJson: detail,
	})
	if err != nil {
		return fmt.Errorf("write audit event %s: %w", e.Action, err)
	}
	return nil
}
