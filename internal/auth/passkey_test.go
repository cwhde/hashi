package auth

import (
	"github.com/go-webauthn/webauthn/protocol"

	"context"
	"errors"
	"testing"

	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

var testRP = RPFromDomains("example.com", "hashi.example.com")

func newPasskeys(t *testing.T) (*Passkeys, *store.Store) {
	t.Helper()
	st := storetest.New(t)
	return NewPasskeys(st, func(context.Context) (RelyingParty, error) { return testRP, nil }), st
}

// registerSoft runs a full registration ceremony for owner and returns the authenticator and passkey id.
func registerSoft(t *testing.T, p *Passkeys, st *store.Store, owner []byte, origin string) (*softAuthenticator, string, error) {
	t.Helper()
	ctx := context.Background()
	id, opts, err := p.BeginRegistration(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Response.RelyingParty.ID != "example.com" {
		t.Fatalf("RP ID %q, want the root domain", opts.Response.RelyingParty.ID)
	}
	a := newSoftAuthenticator(t)
	resp := a.register(testRP.ID, origin, opts.Response.Challenge, []byte(opts.Response.User.ID.(protocol.URLEncodedBase64)))
	pk, err := p.FinishRegistration(ctx, st.Q, owner, id, "Laptop", resp)
	return a, pk.ID, err
}

func TestPasskeyRegisterLoginReauth(t *testing.T) {
	ctx := context.Background()
	p, st := newPasskeys(t)
	owner := []byte("session-1")
	a, pkID, err := registerSoft(t, p, st, owner, testRP.Origin)
	if err != nil {
		t.Fatal(err)
	}

	cid, assertion, err := p.BeginLogin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.FinishLogin(ctx, st.Q, cid, a.assert(testRP.ID, testRP.Origin, assertion.Response.Challenge))
	if err != nil || got != pkID {
		t.Fatalf("login: %q %v", got, err)
	}
	// A ceremony completes once.
	if _, err := p.FinishLogin(ctx, st.Q, cid, a.assert(testRP.ID, testRP.Origin, assertion.Response.Challenge)); !errors.Is(err, ErrCeremony) {
		t.Fatalf("replayed ceremony: %v", err)
	}

	rid, ra, err := p.BeginReauth(ctx, owner)
	if err != nil {
		t.Fatal(err)
	}
	// Another session cannot finish this session's reauth.
	if _, err := p.FinishReauth(ctx, st.Q, []byte("session-2"), rid, a.assert(testRP.ID, testRP.Origin, ra.Response.Challenge)); !errors.Is(err, ErrCeremony) {
		t.Fatalf("foreign owner: %v", err)
	}
	rid, ra, _ = p.BeginReauth(ctx, owner)
	if _, err := p.FinishReauth(ctx, st.Q, owner, rid, a.assert(testRP.ID, testRP.Origin, ra.Response.Challenge)); err != nil {
		t.Fatal(err)
	}
	list, _ := p.List(ctx)
	if len(list) != 1 || list[0].LastUsedAt.IsZero() || list[0].Name != "Laptop" {
		t.Fatalf("list %+v", list)
	}
}

// 02 §2: assertions are accepted only from exactly https://<admin domain>, even though the RP ID is the root domain.
func TestPasskeyRejectsOtherOrigins(t *testing.T) {
	ctx := context.Background()
	p, st := newPasskeys(t)
	for _, origin := range []string{"https://other.example.com", "http://hashi.example.com", "https://hashi.example.com.evil.test"} {
		if _, _, err := registerSoft(t, p, st, []byte("s"), origin); !errors.Is(err, ErrCeremony) {
			t.Errorf("register from %s: %v", origin, err)
		}
	}
	a, _, err := registerSoft(t, p, st, []byte("s"), testRP.Origin)
	if err != nil {
		t.Fatal(err)
	}
	cid, as, _ := p.BeginLogin(ctx)
	if _, err := p.FinishLogin(ctx, st.Q, cid, a.assert(testRP.ID, "https://app.example.com", as.Response.Challenge)); !errors.Is(err, ErrCeremony) {
		t.Fatalf("login from sibling subdomain: %v", err)
	}
	cid, as, _ = p.BeginLogin(ctx)
	if _, err := p.FinishLogin(ctx, st.Q, cid, a.assert("evil.test", testRP.Origin, as.Response.Challenge)); !errors.Is(err, ErrCeremony) {
		t.Fatalf("wrong RP ID hash: %v", err)
	}
}

func TestPasskeyUnavailableBeforeBaseSettings(t *testing.T) {
	st := storetest.New(t)
	p := NewPasskeys(st, func(context.Context) (RelyingParty, error) { return RelyingParty{}, nil })
	if _, _, err := p.BeginLogin(context.Background()); !errors.Is(err, ErrPasskeyUnavailable) {
		t.Fatalf("got %v", err)
	}
}

func TestLastPasskeyCannotBeDeleted(t *testing.T) {
	ctx := context.Background()
	p, st := newPasskeys(t)
	_, first, err := registerSoft(t, p, st, []byte("s"), testRP.Origin)
	if err != nil {
		t.Fatal(err)
	}
	if err := Delete(ctx, st.Q, first); !errors.Is(err, ErrLastPasskey) {
		t.Fatalf("deleting the only passkey: %v", err)
	}
	if _, _, err := registerSoft(t, p, st, []byte("s"), testRP.Origin); err != nil {
		t.Fatal(err)
	}
	if err := Delete(ctx, st.Q, first); err != nil {
		t.Fatal(err)
	}
}
