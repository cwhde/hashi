package auth

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

var (
	ipA = netip.MustParseAddr("192.0.2.10")
	ipB = netip.MustParseAddr("198.51.100.20")
	t0  = time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC)
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time          { return c.t }
func (c *clock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newManager(t *testing.T, st *store.Store, c *clock) *Manager {
	t.Helper()
	m := NewManager(st, audit.New([]byte("salt")), []byte("csrf-key"))
	m.now = c.now
	return m
}

func issue(t *testing.T, m *Manager, st *store.Store, ip netip.Addr) (string, *Session) {
	t.Helper()
	var tok string
	var s *Session
	err := st.Tx(context.Background(), func(q *db.Queries) error {
		var err error
		tok, s, err = m.Issue(context.Background(), q, MethodPasskey, "", ip, PasskeyScopes, "test-agent")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return tok, s
}

func mustValidate(t *testing.T, m *Manager, tok string, ip netip.Addr) *Session {
	t.Helper()
	s, err := m.Validate(context.Background(), tok, ip)
	if err != nil {
		t.Fatalf("validate from %s: %v", ip, err)
	}
	return s
}

func mustReject(t *testing.T, m *Manager, tok string, ip netip.Addr) {
	t.Helper()
	if _, err := m.Validate(context.Background(), tok, ip); !errors.Is(err, ErrNoSession) {
		t.Fatalf("validate from %s: got %v, want ErrNoSession", ip, err)
	}
}

func TestTokenWorksOnlyFromIssuanceIP(t *testing.T) {
	st, c := storetest.New(t), &clock{t0}
	m := newManager(t, st, c)
	tok, _ := issue(t, m, st, ipA)
	mustValidate(t, m, tok, ipA)
	mustReject(t, m, tok, ipB)
	// The mismatch revoked it: the legitimate IP can no longer use it either.
	mustReject(t, m, tok, ipA)
	row, err := st.Q.GetSession(context.Background(), hashToken(decode(t, tok)))
	if err != nil || row.RevocationReason.String != ReasonIPMismatch {
		t.Fatalf("row %+v %v", row, err)
	}
}

func TestTwoLoginsFromTwoIPsAreIndependent(t *testing.T) {
	st, c := storetest.New(t), &clock{t0}
	m := newManager(t, st, c)
	tokA, _ := issue(t, m, st, ipA)
	tokB, _ := issue(t, m, st, ipB)
	if tokA == tokB {
		t.Fatal("same token twice")
	}
	mustValidate(t, m, tokA, ipA)
	mustValidate(t, m, tokB, ipB)
	mustReject(t, m, tokA, ipB) // revokes A only
	mustValidate(t, m, tokB, ipB)
}

func TestActivityExtendsIdleButNotAbsolute(t *testing.T) {
	st, c := storetest.New(t), &clock{t0}
	m := newManager(t, st, c)
	tok, s := issue(t, m, st, ipA)
	if got := s.IdleExpiresAt.Sub(t0); got != 4*time.Hour {
		t.Fatalf("initial idle %s", got)
	}
	// Active every 3 h 50 min: idle never trips, but the 8 h absolute limit does.
	for range 2 {
		c.advance(3*time.Hour + 50*time.Minute)
		s = mustValidate(t, m, tok, ipA)
		if s.AbsoluteAt != t0.Add(8*time.Hour) {
			t.Fatalf("absolute moved to %s", s.AbsoluteAt)
		}
		if s.IdleExpiresAt.After(s.AbsoluteAt) {
			t.Fatalf("idle %s beyond absolute %s", s.IdleExpiresAt, s.AbsoluteAt)
		}
	}
	c.advance(20 * time.Minute) // 8 h total
	mustReject(t, m, tok, ipA)
}

func TestIdleExpiry(t *testing.T) {
	st, c := storetest.New(t), &clock{t0}
	m := newManager(t, st, c)
	tok, _ := issue(t, m, st, ipA)
	c.advance(4*time.Hour - time.Second)
	mustValidate(t, m, tok, ipA)
	c.advance(4 * time.Hour)
	mustReject(t, m, tok, ipA)
}

func TestIdleNeverExceedsConfiguredValueBetweenPersistedTouches(t *testing.T) {
	st, c := storetest.New(t), &clock{t0}
	m := newManager(t, st, c)
	tok, _ := issue(t, m, st, ipA)
	m.SetIdle(30 * time.Minute) // lowered after issuance
	c.advance(31 * time.Minute)
	mustReject(t, m, tok, ipA)
	m.SetIdle(10 * time.Hour) // capped at 4 h
	if time.Duration(m.idle.Load()) != MaxIdle {
		t.Fatalf("idle not capped: %s", time.Duration(m.idle.Load()))
	}
}

func TestReauthWindowAndAbsoluteUnchanged(t *testing.T) {
	ctx := context.Background()
	st, c := storetest.New(t), &clock{t0}
	m := newManager(t, st, c)
	tok, s := issue(t, m, st, ipA)
	if s.ReauthValid(c.t) {
		t.Fatal("fresh session counts as reauthenticated")
	}
	c.advance(3*time.Hour + 30*time.Minute)
	mustValidate(t, m, tok, ipA)
	c.advance(3*time.Hour + 30*time.Minute)
	s = mustValidate(t, m, tok, ipA)
	if err := m.MarkReauth(ctx, st.Q, s); err != nil {
		t.Fatal(err)
	}
	c.advance(4*time.Minute + 59*time.Second)
	if s = mustValidate(t, m, tok, ipA); !s.ReauthValid(c.t) {
		t.Fatal("reauth expired early")
	}
	if s.AbsoluteAt != t0.Add(8*time.Hour) {
		t.Fatalf("reauth moved absolute expiry to %s", s.AbsoluteAt)
	}
	c.advance(2 * time.Second)
	if s = mustValidate(t, m, tok, ipA); s.ReauthValid(c.t) {
		t.Fatal("reauth valid after 5 minutes")
	}
	c.advance(time.Hour)
	mustReject(t, m, tok, ipA)
}

func TestRevocationIsImmediate(t *testing.T) {
	ctx := context.Background()
	st, c := storetest.New(t), &clock{t0}
	m := newManager(t, st, c)
	tok, s := issue(t, m, st, ipA)
	var dropped []byte
	m.OnRevoke(func(h []byte) { dropped = h })
	if _, err := m.Revoke(ctx, st.Q, s.IDHash, ReasonLogout, "test", ipA); err != nil {
		t.Fatal(err)
	}
	mustReject(t, m, tok, ipA)
	if !bytes.Equal(dropped, s.IDHash) {
		t.Fatal("in-memory material not dropped")
	}
}

func TestRestartKeepsSessionAndReauth(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "hashi.db")
	c := &clock{t0}
	st := storetest.Open(t, path)
	m := newManager(t, st, c)
	tok, s := issue(t, m, st, ipA)
	if err := m.MarkReauth(ctx, st.Q, s); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	st2 := storetest.Open(t, path)
	m2 := newManager(t, st2, c)
	c.advance(time.Minute)
	if s2 := mustValidate(t, m2, tok, ipA); !s2.ReauthValid(c.t) {
		t.Fatal("reauth lost across restart")
	}
}

func TestMalformedAndUnknownFailClosed(t *testing.T) {
	st, c := storetest.New(t), &clock{t0}
	m := newManager(t, st, c)
	unknown := base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	for _, tok := range []string{"", "x", "!!!!", unknown, unknown + "AA", base64.RawURLEncoding.EncodeToString([]byte("short"))} {
		mustReject(t, m, tok, ipA)
	}
	tok, _ := issue(t, m, st, ipA)
	mustReject(t, m, tok, netip.Addr{})
}

func TestCSRFTokenIsSessionBound(t *testing.T) {
	st, c := storetest.New(t), &clock{t0}
	m := newManager(t, st, c)
	_, a := issue(t, m, st, ipA)
	_, b := issue(t, m, st, ipA)
	if !m.CheckCSRF(a, m.CSRFToken(a)) || m.CheckCSRF(a, m.CSRFToken(b)) || m.CheckCSRF(a, "") {
		t.Fatal("csrf check wrong")
	}
}

// Raw tokens never appear in audit events or the database (02 §6.3, invariant 5).
func TestInvariant_5_RawTokenNotStoredOrAudited(t *testing.T) {
	ctx := context.Background()
	st, c := storetest.New(t), &clock{t0}
	m := newManager(t, st, c)
	tok, _ := issue(t, m, st, ipA)
	mustValidate(t, m, tok, ipA)
	mustReject(t, m, tok, ipB)
	events, err := st.Q.ListAuditEvents(ctx, db.ListAuditEventsParams{Lim: 100})
	if err != nil || len(events) < 2 {
		t.Fatalf("events %d %v", len(events), err)
	}
	raw := decode(t, tok)
	for _, e := range events {
		all := e.Actor + e.Target + e.DetailJson
		if strings.Contains(all, tok) || strings.Contains(all, string(raw)) {
			t.Fatalf("token in audit event %+v", e)
		}
	}
	dump := filepath.Join(t.TempDir(), "dump.db")
	if err := st.BackupTo(ctx, dump); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(dump) //nolint:gosec // test file
	if bytes.Contains(b, raw) || bytes.Contains(b, []byte(tok)) {
		t.Fatal("raw token in database")
	}
}

func TestScopes(t *testing.T) {
	if !HasAll(PasskeyScopes, AdminRead, SecurityManage) {
		t.Fatal("passkey scopes incomplete")
	}
	if HasAll(BootstrapScopes, AdminRead) {
		t.Fatal("bootstrap grants admin.read")
	}
	if HasAll(PasskeyScopes, SetupManage) {
		t.Fatal("passkey sessions grant setup scopes")
	}
	if HasAll([]Scope{"made.up"}, "made.up") {
		t.Fatal("unknown scope granted")
	}
	if HasAll(nil, AdminRead) {
		t.Fatal("missing scope granted")
	}
	if !HasAll(nil) {
		t.Fatal("no requirement should pass")
	}
}

func decode(t *testing.T, tok string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Review finding: with the minimum idle timeout (5 min), steady activity must keep the session alive.
func TestMinimumIdleWithContinuousActivity(t *testing.T) {
	st, c := storetest.New(t), &clock{t0}
	m := newManager(t, st, c)
	m.SetIdle(5 * time.Minute)
	tok, _ := issue(t, m, st, ipA)
	for i := range 20 {
		c.advance(time.Minute)
		if _, err := m.Validate(context.Background(), tok, ipA); err != nil {
			t.Fatalf("active every minute, rejected after %d min: %v", i+1, err)
		}
	}
}

func TestRevokeByCredential(t *testing.T) {
	ctx := context.Background()
	st, c := storetest.New(t), &clock{t0}
	m := newManager(t, st, c)
	for _, id := range []string{"pk1", "pk2"} {
		if err := st.Q.InsertPasskey(ctx, db.InsertPasskeyParams{ID: id, CredentialID: []byte(id), CredentialJson: "{}", Name: id, RpID: "example.com", CreatedAt: 1}); err != nil {
			t.Fatal(err)
		}
	}
	issueWith := func(cred string) string {
		var tok string
		if err := st.Tx(ctx, func(q *db.Queries) error {
			var err error
			tok, _, err = m.Issue(ctx, q, MethodPasskey, cred, ipA, PasskeyScopes, "")
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return tok
	}
	a, b := issueWith("pk1"), issueWith("pk2")
	if err := st.Tx(ctx, func(q *db.Queries) error {
		_, err := m.RevokeByCredential(ctx, q, "pk1", "test", ipA)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	mustReject(t, m, a, ipA)
	mustValidate(t, m, b, ipA)
}
