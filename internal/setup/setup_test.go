package setup

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	"git.juzo.io/juzo/hashi/internal/vault"
)

var owner = []byte("session-hash")

func newService(t *testing.T, st *store.Store) *Service {
	t.Helper()
	v, err := vault.New(bytes.Repeat([]byte{1}, vault.KeySize), 1)
	if err != nil {
		t.Fatal(err)
	}
	s := New(st, v, audit.New([]byte("salt")))
	s.RPID = func(context.Context) (string, error) { return "example.com", nil }
	return s
}

func addPasskey(t *testing.T, st *store.Store) {
	t.Helper()
	if err := st.Q.InsertPasskey(context.Background(), db.InsertPasskeyParams{
		ID: "pk1", CredentialID: []byte("cred"), CredentialJson: "{}", Name: "Laptop", RpID: "example.com", CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
}

func markDone(t *testing.T, s *Service, st *store.Store, steps ...int) {
	t.Helper()
	for _, n := range steps {
		if err := s.MarkDone(context.Background(), st.Q, n, nil); err != nil {
			t.Fatal(err)
		}
	}
}

func lastGroup(display string) string {
	g := strings.Split(display, "-")
	return g[len(g)-1]
}

func TestFreshStateListsElevenSteps(t *testing.T) {
	st := storetest.New(t)
	s := newService(t, st)
	state, err := s.Load(context.Background(), st.Q)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Steps) != 11 || state.Completed {
		t.Fatalf("%d steps, completed %v", len(state.Steps), state.Completed)
	}
	var builtSteps []int
	for _, ss := range state.Steps {
		if ss.Status != Pending {
			t.Fatalf("step %d starts %s", ss.N, ss.Status)
		}
		if ss.Built {
			builtSteps = append(builtSteps, ss.N)
		}
	}
	if len(builtSteps) != 10 {
		t.Fatalf("built steps %v, want 1–9, 11", builtSteps)
	}
}

func TestRecoveryKeyNeedsPasskeyAndCorrectChecksum(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	s := newService(t, st)
	if _, err := s.GenerateRecoveryKey(ctx, owner); !errors.Is(err, ErrNoPasskey) {
		t.Fatalf("without passkey: %v", err)
	}
	addPasskey(t, st)
	if err := s.ConfirmRecoveryKey(ctx, owner, "AAAA"); !errors.Is(err, ErrNoPendingKey) {
		t.Fatalf("confirm before generate: %v", err)
	}
	d, err := s.GenerateRecoveryKey(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ConfirmRecoveryKey(ctx, []byte("other"), lastGroup(d.Reveal())); !errors.Is(err, ErrNoPendingKey) {
		t.Fatalf("other session confirms: %v", err)
	}
	if err := s.ConfirmRecoveryKey(ctx, owner, "ZZZZ"); !errors.Is(err, ErrChecksumMismatch) {
		t.Fatalf("wrong checksum: %v", err)
	}
	if err := s.ConfirmRecoveryKey(ctx, owner, strings.ToLower(lastGroup(d.Reveal()))); err != nil {
		t.Fatal(err)
	}
	state, _ := s.Load(ctx, st.Q)
	if state.Steps[StepPasskey-1].Status != Done {
		t.Fatalf("step 9 %s", state.Steps[StepPasskey-1].Status)
	}
	rk, err := st.Q.GetRecoveryKey(ctx)
	if err != nil || !rk.ConfirmedAt.Valid {
		t.Fatalf("recovery key not confirmed: %v", err)
	}
	if bytes.Contains(rk.WrappedMaster, bytes.Repeat([]byte{1}, vault.KeySize)) {
		t.Fatal("master key stored in clear")
	}
}

// Invariant 14: setup never reports complete before passkey + confirmed recovery key, and its state survives restart.
func TestInvariant_14_CompleteRequiresPasskeyAndRecoveryKey(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "hashi.db")
	st := storetest.Open(t, path)
	s := newService(t, st)
	s.SkipUnbuilt = true // isolate the passkey/recovery rule from steps of later slices
	markDone(t, s, st, StepBootstrap, StepBase, StepDNS, StepCert, StepTraefik, StepFirewall, StepSystemSvc, StepHTTPS)
	// Even a forged "done" flag on step 9 does not complete setup without the data behind it.
	markDone(t, s, st, StepPasskey)
	if err := s.Complete(ctx, "test", ""); !errors.Is(err, ErrNotReady) {
		t.Fatalf("complete without passkey: %v", err)
	}
	addPasskey(t, st)
	if err := s.Complete(ctx, "test", ""); !errors.Is(err, ErrNotReady) {
		t.Fatalf("complete without recovery key: %v", err)
	}
	d, err := s.GenerateRecoveryKey(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Complete(ctx, "test", ""); !errors.Is(err, ErrNotReady) {
		t.Fatalf("complete with unconfirmed recovery key: %v", err)
	}
	if err := s.ConfirmRecoveryKey(ctx, owner, lastGroup(d.Reveal())); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	// Restart: the state is still there, and completion works now.
	st = storetest.Open(t, path)
	s = newService(t, st)
	s.SkipUnbuilt = true
	state, err := s.Load(ctx, st.Q)
	if err != nil || state.Steps[StepPasskey-1].Status != Done || state.Completed {
		t.Fatalf("after restart: %+v %v", state.Steps[StepPasskey-1], err)
	}
	if err := s.Complete(ctx, "test", ""); err != nil {
		t.Fatal(err)
	}
	if done, _ := s.Completed(ctx); !done {
		t.Fatal("not completed")
	}
}

func TestCompleteNeedsUnbuiltStepsInProductionBuilds(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	s := newService(t, st)
	addPasskey(t, st)
	markDone(t, s, st, StepBootstrap, StepBase, StepDNS, StepCert, StepTraefik, StepSystemSvc, StepHTTPS)
	d, _ := s.GenerateRecoveryKey(ctx, owner)
	if err := s.ConfirmRecoveryKey(ctx, owner, lastGroup(d.Reveal())); err != nil {
		t.Fatal(err)
	}
	if err := s.Complete(ctx, "test", ""); !errors.Is(err, ErrNotReady) || !strings.Contains(err.Error(), "Linux firewall host") {
		t.Fatalf("got %v, want step 6 missing", err)
	}
}

func TestCompleteDestroysBootstrapAndRevokesBootstrapSessions(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	s := newService(t, st)
	s.SkipUnbuilt = true
	now := time.Now().UnixMilli()
	if err := st.Q.UpsertBootstrapCredentials(ctx, db.UpsertBootstrapCredentialsParams{Username: "u", PasswordHash: "h", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	for i, m := range []string{"bootstrap", "passkey"} {
		if err := st.Q.InsertSession(ctx, db.InsertSessionParams{
			IDHash: []byte{byte(i)}, AuthMethod: m, BoundIp: "192.0.2.1", ScopesJson: "[]", CreatedAt: now, LastSeenAt: now,
			IdleExpiresAt: now + 1000, AbsoluteExpiresAt: now + 1000,
		}); err != nil {
			t.Fatal(err)
		}
	}
	addPasskey(t, st)
	markDone(t, s, st, StepBootstrap, StepBase, StepDNS, StepCert, StepTraefik, StepFirewall, StepSystemSvc, StepHTTPS)
	d, _ := s.GenerateRecoveryKey(ctx, owner)
	if err := s.ConfirmRecoveryKey(ctx, owner, lastGroup(d.Reveal())); err != nil {
		t.Fatal(err)
	}
	if err := s.Complete(ctx, "test", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Q.GetBootstrapCredentials(ctx); err == nil {
		t.Fatal("bootstrap credentials survive completion")
	}
	b, _ := st.Q.GetSession(ctx, []byte{0})
	p, _ := st.Q.GetSession(ctx, []byte{1})
	if !b.RevokedAt.Valid || p.RevokedAt.Valid {
		t.Fatalf("bootstrap revoked %v, passkey revoked %v", b.RevokedAt.Valid, p.RevokedAt.Valid)
	}
	if err := s.Complete(ctx, "test", ""); !errors.Is(err, ErrCompleted) {
		t.Fatalf("second completion: %v", err)
	}
	if _, err := s.GenerateRecoveryKey(ctx, owner); !errors.Is(err, ErrCompleted) {
		t.Fatalf("recovery key after completion: %v", err)
	}
}

// Review finding: passkeys are bound to the root domain; completion needs one for the current root domain.
func TestCompleteNeedsPasskeyForCurrentRPID(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	s := newService(t, st)
	s.SkipUnbuilt = true
	addPasskey(t, st) // registered for example.com
	markDone(t, s, st, StepBootstrap, StepBase)
	d, _ := s.GenerateRecoveryKey(ctx, owner)
	if err := s.ConfirmRecoveryKey(ctx, owner, lastGroup(d.Reveal())); err != nil {
		t.Fatal(err)
	}
	s.RPID = func(context.Context) (string, error) { return "example.org", nil }
	if err := s.Complete(ctx, "test", ""); !errors.Is(err, ErrNotReady) {
		t.Fatalf("passkey for another RP ID accepted: %v", err)
	}
}

func TestRootDomainChangeResetsPasskeyStep(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	s := newService(t, st)
	addPasskey(t, st)
	markDone(t, s, st, StepBootstrap, StepBase, StepPasskey)
	if err := st.Tx(ctx, func(q *db.Queries) error { return s.RootDomainChanged(ctx, q, "example.org") }); err != nil {
		t.Fatal(err)
	}
	if n, _ := st.Q.CountPasskeys(ctx); n != 0 {
		t.Fatalf("%d passkeys left for the old root domain", n)
	}
	state, _ := s.Load(ctx, st.Q)
	if state.Steps[StepPasskey-1].Status != Pending {
		t.Fatalf("step 9 %s, want pending", state.Steps[StepPasskey-1].Status)
	}
}

func TestHTTPSNonceOnceAndExpiring(t *testing.T) {
	s := &Service{now: time.Now}
	n, exp, err := s.NewHTTPSNonce()
	if err != nil || n == "" || time.Until(exp) > HTTPSNonceTTL {
		t.Fatalf("%q %v %v", n, exp, err)
	}
	if err := s.ConsumeHTTPSNonce("wrong"); !errors.Is(err, ErrNonce) {
		t.Errorf("wrong nonce: %v", err)
	}
	if err := s.ConsumeHTTPSNonce(n); err != nil {
		t.Fatal(err)
	}
	if err := s.ConsumeHTTPSNonce(n); !errors.Is(err, ErrNonce) {
		t.Errorf("reused nonce: %v", err)
	}
	old, _, _ := s.NewHTTPSNonce()
	latest, _, _ := s.NewHTTPSNonce()
	if err := s.ConsumeHTTPSNonce(old); !errors.Is(err, ErrNonce) {
		t.Errorf("superseded nonce: %v", err)
	}
	s.now = func() time.Time { return time.Now().Add(HTTPSNonceTTL + time.Second) }
	if err := s.ConsumeHTTPSNonce(latest); !errors.Is(err, ErrNonce) {
		t.Errorf("expired nonce: %v", err)
	}
}
