package vault

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"testing"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

// A rotation re-seals every secret under a new master key, refuses the old key afterwards, keeps every operational
// key (Pulse tokens, CSRF, CAPTCHA clearances, audit correlation), and keeps the same recovery key working.
func TestRotateKeepsSecretsOperationalKeysAndRecovery(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	old := bytes.Repeat([]byte{7}, KeySize)
	v, err := Load(ctx, st.Q, old)
	if err != nil {
		t.Fatal(err)
	}
	p, err := v.Seal(Service, "dns.token", secrets.New("hunter2"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertSecret(ctx, p); err != nil {
		t.Fatal(err)
	}
	recovery, _, _, err := NewRecoveryKey()
	if err != nil {
		t.Fatal(err)
	}
	w, err := v.WrapMaster(recovery)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Q.UpsertRecoveryKey(ctx, w); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.ConfirmRecoveryKey(ctx, sql.NullInt64{Int64: 1, Valid: true}); err != nil {
		t.Fatal(err)
	}
	ops := map[string][]byte{}
	for _, n := range OperationalNames {
		ops[n] = v.Operational(n)
	}

	// The wrong recovery key changes nothing.
	other, _, _, _ := NewRecoveryKey()
	if err := st.Tx(ctx, func(q *db.Queries) error { _, _, err := v.Rotate(ctx, q, other); return err }); !errors.Is(err, ErrRecoveryKeyMismatch) {
		t.Fatalf("wrong recovery key: %v", err)
	}

	var fresh []byte
	for round := 2; round <= 3; round++ { // twice: the second rotation starts from stored operational keys
		if err := st.Tx(ctx, func(q *db.Queries) error {
			var err error
			_, fresh, err = v.Rotate(ctx, q, recovery)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(ctx, st.Q, old); !errors.Is(err, ErrWrongMasterKey) {
			t.Fatalf("round %d: old master key still opens the database: %v", round, err)
		}
		if v, err = Load(ctx, st.Q, fresh); err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		if v.KeyVersion() != int64(round) {
			t.Fatalf("round %d: key version %d", round, v.KeyVersion())
		}
		r, err := st.Q.GetSecret(ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := v.Open(r); err != nil || got.Reveal() != "hunter2" {
			t.Fatalf("round %d: secret after rotation %q, %v", round, got.Reveal(), err)
		}
		for n, k := range ops {
			if !bytes.Equal(v.Operational(n), k) {
				t.Fatalf("round %d: operational key %s changed", round, n)
			}
		}
		rec, err := st.Q.GetRecoveryKey(ctx)
		if err != nil {
			t.Fatal(err)
		}
		m, err := RecoverMaster(rec, recovery)
		if err != nil || !bytes.Equal(m, fresh) {
			t.Fatalf("round %d: recovery key does not give the new master key: %v", round, err)
		}
		if !rec.ConfirmedAt.Valid {
			t.Fatalf("round %d: rotation dropped the recovery key's confirmation", round)
		}
		old = fresh
	}
}

func TestOperationalRejectsUnknownName(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("an unlisted operational key would silently change after a rotation")
		}
	}()
	testVault(t, 1).Operational("not-listed")
}
