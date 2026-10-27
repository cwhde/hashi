package auth

import (
	"context"
	"errors"
	"testing"

	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

func TestRecoveryLinkIsSingleUseAndExpires(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	tok, err := NewRecoveryLink(ctx, st.Q, t0)
	if err != nil {
		t.Fatal(err)
	}
	if err := UseRecoveryLink(ctx, st.Q, tok, t0.Add(RecoveryLinkTTL-1)); err != nil {
		t.Fatal(err)
	}
	if err := UseRecoveryLink(ctx, st.Q, tok, t0.Add(RecoveryLinkTTL-1)); !errors.Is(err, ErrRecoveryLink) {
		t.Fatalf("second use: %v", err)
	}
	late, _ := NewRecoveryLink(ctx, st.Q, t0)
	if err := UseRecoveryLink(ctx, st.Q, late, t0.Add(RecoveryLinkTTL)); !errors.Is(err, ErrRecoveryLink) {
		t.Fatalf("expired: %v", err)
	}
	for _, bad := range []string{"", "x", tok + "A"} {
		if err := UseRecoveryLink(ctx, st.Q, bad, t0); !errors.Is(err, ErrRecoveryLink) {
			t.Fatalf("%q: %v", bad, err)
		}
	}
	if HasAll(RecoveryScopes, AdminRead) || !HasAll(RecoveryScopes, RecoveryPasskey) {
		t.Fatal("recovery scopes wrong")
	}
}
