package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"sync/atomic"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Lifetimes from 02 §3.2.
const (
	MaxIdle        = 4 * time.Hour
	MaxAbsolute    = 8 * time.Hour
	ReauthValidity = 5 * time.Minute
	touchCadence   = 5 * time.Minute
)

// Method is how a session was authenticated.
type Method string

const (
	MethodBootstrap Method = "bootstrap"
	MethodPasskey   Method = "passkey"
	MethodRecovery  Method = "recovery"
)

// Revocation reasons stored on the row and in the audit log.
const (
	ReasonLogout      = "logout"
	ReasonManual      = "revoked"
	ReasonIPMismatch  = "ip_mismatch"
	ReasonIdle        = "idle_expired"
	ReasonAbsolute    = "absolute_expired"
	ReasonSetupDone   = "setup_complete"
	ReasonPasskeyGone = "passkey_deleted"
	ReasonBulk        = "revoke_others"
)

// ErrNoSession covers malformed, unknown, expired and revoked tokens alike: the caller is unauthenticated.
var ErrNoSession = errors.New("no valid session")

// Session is a validated admin session. It never holds the raw token.
type Session struct {
	IDHash        []byte
	Correlation   string // salted hash prefix, safe to show and log
	Method        Method
	CredentialID  string
	BoundIP       netip.Addr
	Scopes        []Scope
	CreatedAt     time.Time
	LastSeenAt    time.Time
	IdleExpiresAt time.Time
	AbsoluteAt    time.Time
	ReauthAt      time.Time // zero when never reauthenticated
}

// Has reports whether the session carries every required scope.
func (s *Session) Has(required ...Scope) bool { return HasAll(s.Scopes, required...) }

// ReauthValid reports whether a passkey reauthentication happened within the last five minutes.
func (s *Session) ReauthValid(now time.Time) bool {
	return !s.ReauthAt.IsZero() && now.Before(s.ReauthAt.Add(ReauthValidity)) && !now.Before(s.ReauthAt)
}

// Manager issues and validates sessions.
type Manager struct {
	st       *store.Store
	audit    *audit.Log
	csrfKey  []byte
	now      func() time.Time
	idle     atomic.Int64 // configured idle lifetime in ns (settings may lower it)
	mu       sync.Mutex
	onRevoke []func(idHash []byte)
}

// NewManager returns a session manager. csrfKey is an operational key from the vault.
func NewManager(st *store.Store, a *audit.Log, csrfKey []byte) *Manager {
	m := &Manager{st: st, audit: a, csrfKey: csrfKey, now: time.Now}
	m.idle.Store(int64(MaxIdle))
	return m
}

// SetIdle sets the idle lifetime, capped at four hours.
func (m *Manager) SetIdle(d time.Duration) { m.idle.Store(int64(min(d, MaxIdle))) }

// OnRevoke registers fn to drop in-memory material (ceremony state, caches) belonging to a revoked session.
func (m *Manager) OnRevoke(fn func(idHash []byte)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onRevoke = append(m.onRevoke, fn)
}

func (m *Manager) dropMaterial(idHash []byte) {
	m.mu.Lock()
	fns := append([]func([]byte){}, m.onRevoke...)
	m.mu.Unlock()
	for _, fn := range fns {
		fn(idHash)
	}
}

func hashToken(raw []byte) []byte {
	h := sha256.Sum256(raw)
	return h[:]
}

// Issue creates a new session bound to ip and returns the cookie token. The row and its audit event are written with q
// (the caller's transaction).
func (m *Manager) Issue(ctx context.Context, q *db.Queries, method Method, credentialID string, ip netip.Addr,
	scopes []Scope, userAgent string,
) (string, *Session, error) {
	if !ip.IsValid() {
		return "", nil, errors.New("issue session: no client IP")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, err
	}
	now := m.now()
	idle := time.Duration(m.idle.Load())
	abs := now.Add(MaxAbsolute)
	s := &Session{
		IDHash: hashToken(raw), Method: method, CredentialID: credentialID, BoundIP: ip, Scopes: scopes,
		CreatedAt: now, LastSeenAt: now, IdleExpiresAt: minTime(now.Add(idle), abs), AbsoluteAt: abs,
	}
	s.Correlation = m.audit.Correlate(s.IDHash)
	scopesJSON, err := json.Marshal(scopes)
	if err != nil {
		return "", nil, err
	}
	err = q.InsertSession(ctx, db.InsertSessionParams{
		IDHash: s.IDHash, AuthMethod: string(method), CredentialID: nullString(credentialID), BoundIp: ip.String(),
		ScopesJson: string(scopesJSON), CreatedAt: now.UnixMilli(), LastSeenAt: now.UnixMilli(),
		IdleExpiresAt: s.IdleExpiresAt.UnixMilli(), AbsoluteExpiresAt: abs.UnixMilli(),
		UserAgentHash: m.audit.Correlate([]byte(userAgent)),
	})
	if err != nil {
		return "", nil, fmt.Errorf("store session: %w", err)
	}
	if err := m.audit.Record(ctx, q, audit.Event{
		Actor: m.audit.SessionActor(s.IDHash), Action: "session.issued", IP: ip.String(),
		Detail: map[string]any{"method": method, "scopes": scopes},
	}); err != nil {
		return "", nil, err
	}
	return base64.RawURLEncoding.EncodeToString(raw), s, nil
}

// Validate checks token from ip. Every failure is ErrNoSession; an IP mismatch additionally revokes the session.
// Activity extends the idle deadline (persisted at most every five minutes), never the absolute one.
func (m *Manager) Validate(ctx context.Context, token string, ip netip.Addr) (*Session, error) {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return nil, ErrNoSession
	}
	idHash := hashToken(raw)
	row, err := m.st.R.GetSession(ctx, idHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoSession
	}
	if err != nil {
		return nil, fmt.Errorf("read session: %w", err)
	}
	s, err := m.fromRow(row)
	if err != nil {
		return nil, ErrNoSession // a row we cannot parse fails closed
	}
	if row.RevokedAt.Valid {
		return nil, ErrNoSession
	}
	now := m.now()
	idleDeadline := minTime(s.IdleExpiresAt, s.LastSeenAt.Add(time.Duration(m.idle.Load())))
	switch {
	case !now.Before(s.AbsoluteAt):
		m.revoke(ctx, s, ReasonAbsolute, audit.System, ip)
		return nil, ErrNoSession
	case !now.Before(idleDeadline):
		m.revoke(ctx, s, ReasonIdle, audit.System, ip)
		return nil, ErrNoSession
	case !ip.IsValid() || ip != s.BoundIP:
		m.revoke(ctx, s, ReasonIPMismatch, audit.System, ip)
		return nil, ErrNoSession
	}
	if now.Sub(s.LastSeenAt) >= touchCadence {
		s.LastSeenAt = now
		s.IdleExpiresAt = minTime(now.Add(time.Duration(m.idle.Load())), s.AbsoluteAt)
		err := m.st.Tx(ctx, func(q *db.Queries) error {
			if err := q.TouchSession(ctx, db.TouchSessionParams{
				LastSeenAt: now.UnixMilli(), IdleExpiresAt: s.IdleExpiresAt.UnixMilli(), IDHash: s.IDHash,
			}); err != nil {
				return err
			}
			return m.audit.Record(ctx, q, audit.Event{Actor: m.audit.SessionActor(s.IDHash), Action: "session.renewed", IP: ip.String()})
		})
		if err != nil {
			return nil, fmt.Errorf("renew session: %w", err)
		}
	}
	return s, nil
}

func (m *Manager) fromRow(r db.AdminSession) (*Session, error) {
	ip, err := netip.ParseAddr(r.BoundIp)
	if err != nil {
		return nil, err
	}
	var scopes []Scope
	if err := json.Unmarshal([]byte(r.ScopesJson), &scopes); err != nil {
		return nil, err
	}
	s := &Session{
		IDHash: r.IDHash, Correlation: m.audit.Correlate(r.IDHash), Method: Method(r.AuthMethod),
		CredentialID: r.CredentialID.String, BoundIP: ip, Scopes: scopes,
		CreatedAt: ms(r.CreatedAt), LastSeenAt: ms(r.LastSeenAt), IdleExpiresAt: ms(r.IdleExpiresAt), AbsoluteAt: ms(r.AbsoluteExpiresAt),
	}
	if r.ReauthAt.Valid {
		s.ReauthAt = ms(r.ReauthAt.Int64)
	}
	return s, nil
}

// revoke marks s revoked, audits why and drops in-memory material. Used on the validation path, where a failure to
// write cannot make the session valid (the caller already rejects it).
func (m *Manager) revoke(ctx context.Context, s *Session, reason, actor string, ip netip.Addr) {
	ipStr := ""
	if ip.IsValid() {
		ipStr = ip.String()
	}
	_ = m.st.Tx(ctx, func(q *db.Queries) error {
		n, err := q.RevokeSession(ctx, db.RevokeSessionParams{
			RevokedAt: sqlInt(m.now().UnixMilli()), RevocationReason: nullString(reason), IDHash: s.IDHash,
		})
		if err != nil || n == 0 {
			return err
		}
		detail := map[string]any{"reason": reason}
		if reason == ReasonIPMismatch {
			detail["bound_ip"] = s.BoundIP.String()
		}
		return m.audit.Record(ctx, q, audit.Event{
			Actor: actor, Action: "session.revoked", Target: m.audit.SessionActor(s.IDHash), IP: ipStr,
			Outcome: audit.Denied, Detail: detail,
		})
	})
	m.dropMaterial(s.IDHash)
}

// Revoke ends one session (logout or manual revoke) and audits it with q's transaction.
func (m *Manager) Revoke(ctx context.Context, q *db.Queries, idHash []byte, reason, actor string, ip netip.Addr) (bool, error) {
	n, err := q.RevokeSession(ctx, db.RevokeSessionParams{
		RevokedAt: sqlInt(m.now().UnixMilli()), RevocationReason: nullString(reason), IDHash: idHash,
	})
	if err != nil {
		return false, fmt.Errorf("revoke session: %w", err)
	}
	if n == 0 {
		return false, nil
	}
	if err := m.audit.Record(ctx, q, audit.Event{
		Actor: actor, Action: "session.revoked", Target: m.audit.SessionActor(idHash), IP: ip.String(),
		Detail: map[string]any{"reason": reason},
	}); err != nil {
		return false, err
	}
	m.dropMaterial(idHash)
	return true, nil
}

// MarkReauth records a fresh passkey assertion on the session. It changes neither bound IP nor absolute expiry.
func (m *Manager) MarkReauth(ctx context.Context, q *db.Queries, s *Session) error {
	now := m.now()
	if err := q.SetSessionReauth(ctx, db.SetSessionReauthParams{ReauthAt: sqlInt(now.UnixMilli()), IDHash: s.IDHash}); err != nil {
		return fmt.Errorf("record reauth: %w", err)
	}
	s.ReauthAt = now
	return nil
}

// CSRFToken is the session-bound token unsafe requests must echo in X-Hashi-CSRF (02 §4).
func (m *Manager) CSRFToken(s *Session) string {
	mac := hmac.New(sha256.New, m.csrfKey)
	mac.Write(s.IDHash)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// CheckCSRF compares a presented token in constant time.
func (m *Manager) CheckCSRF(s *Session, presented string) bool {
	return hmac.Equal([]byte(m.CSRFToken(s)), []byte(presented))
}

// Active lists sessions that are neither revoked nor expired.
func (m *Manager) Active(ctx context.Context) ([]*Session, error) {
	rows, err := m.st.R.ListActiveSessions(ctx, m.now().UnixMilli())
	if err != nil {
		return nil, err
	}
	out := make([]*Session, 0, len(rows))
	for _, r := range rows {
		if s, err := m.fromRow(r); err == nil {
			out = append(out, s)
		}
	}
	return out, nil
}

// AuditRetention is how long audit events (and revoked session rows referenced by them) are kept (01 §4.3).
const AuditRetention = 2 * 365 * 24 * time.Hour

// Cleanup deletes expired sessions and revoked ones older than the audit retention (02 §3.6). Validity never depends
// on this: an expired row that survives a failed cleanup is still rejected by Validate.
func (m *Manager) Cleanup(ctx context.Context) (string, error) {
	now := m.now()
	n, err := m.st.Q.DeleteDeadSessions(ctx, db.DeleteDeadSessionsParams{
		Now: now.UnixMilli(), RevokedBefore: now.Add(-AuditRetention).UnixMilli(),
	})
	if err != nil {
		return "", fmt.Errorf("delete dead sessions: %w", err)
	}
	return fmt.Sprintf("%d sessions removed", n), nil
}

// Now is the manager's clock (tests replace it).
func (m *Manager) Now() time.Time { return m.now() }

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func ms(v int64) time.Time { return time.UnixMilli(v).UTC() }

func sqlInt(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} }

func nullString(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
