package vault

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

// Strict mode: after a start the vault is locked; secrets don't open or seal, but the operational keys and the edge
// session key do. The recovery key's master key or a passkey's PRF output unlocks it; switching back needs the file.
func TestStrictModeLockUnlockAndBack(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	master := bytes.Repeat([]byte{9}, KeySize)
	v, err := Load(ctx, st.Q, master)
	if err != nil {
		t.Fatal(err)
	}
	token, err := v.Seal(Service, "dns.token", secrets.New("hunter2"))
	if err != nil {
		t.Fatal(err)
	}
	edgeKey, err := v.Seal(Service, KeptPurposes[0], secrets.New("cookie-key"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertSecret(ctx, token); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertSecret(ctx, edgeKey); err != nil {
		t.Fatal(err)
	}
	ops := map[string][]byte{}
	for _, n := range OperationalNames {
		ops[n] = v.Operational(n)
	}
	if err := v.EnableStrict(ctx, st.Q); err != nil {
		t.Fatal(err)
	}
	if m, err := ReadMode(ctx, st.Q); err != nil || m != ModeStrict {
		t.Fatalf("mode %q %v", m, err)
	}

	// A restart.
	lv, err := LoadStrict(ctx, st.Q)
	if err != nil {
		t.Fatal(err)
	}
	if !lv.Locked() {
		t.Fatal("strict mode must start locked")
	}
	tokRec, _ := st.Q.GetSecret(ctx, token.ID)
	if _, err := lv.Open(tokRec); !errors.Is(err, ErrLocked) {
		t.Fatalf("open while locked: %v", err)
	}
	if _, err := lv.Seal(Service, "x", secrets.New("y")); !errors.Is(err, ErrLocked) {
		t.Fatalf("seal while locked: %v", err)
	}
	edgeRec, _ := st.Q.GetSecret(ctx, edgeKey.ID)
	if got, err := lv.Open(edgeRec); err != nil || got.Reveal() != "cookie-key" {
		t.Fatalf("edge session key while locked: %v", err)
	}
	for n, k := range ops {
		if !bytes.Equal(lv.Operational(n), k) {
			t.Fatalf("operational key %s differs while locked", n)
		}
	}

	// Passkey unlocker: made while unlocked, opened with the same PRF output only.
	prf := bytes.Repeat([]byte{3}, 32)
	u, err := v.NewUnlocker("pk1", []byte("salt"), prf)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lv.NewUnlocker("pk1", []byte("salt"), prf); !errors.Is(err, ErrLocked) {
		t.Fatalf("unlocker from a locked vault: %v", err)
	}
	row := asUnlocker(u)
	if _, err := OpenUnlocker(row, bytes.Repeat([]byte{4}, 32)); err == nil {
		t.Fatal("another PRF output opened the unlocker")
	}
	if _, err := OpenUnlocker(row, nil); !errors.Is(err, ErrNoPRF) {
		t.Fatalf("no PRF output: %v", err)
	}
	got, err := OpenUnlocker(row, prf)
	if err != nil {
		t.Fatal(err)
	}

	if err := lv.Unlock(ctx, st.Q, bytes.Repeat([]byte{8}, KeySize)); !errors.Is(err, ErrWrongMasterKey) {
		t.Fatalf("wrong key unlocked: %v", err)
	}
	ran := false
	lv.OnUnlock(func(context.Context) { ran = true })
	if err := lv.Unlock(ctx, st.Q, got); err != nil {
		t.Fatal(err)
	}
	if lv.Locked() || !ran {
		t.Fatalf("locked=%v hook ran=%v", lv.Locked(), ran)
	}
	if s, err := lv.Open(tokRec); err != nil || s.Reveal() != "hunter2" {
		t.Fatalf("secret after unlock: %v", err)
	}

	// Back to service mode only with the right key in the file.
	if err := lv.DisableStrict(ctx, st.Q, nil); !errors.Is(err, ErrFileKeyMismatch) {
		t.Fatalf("no file: %v", err)
	}
	if err := lv.DisableStrict(ctx, st.Q, master); err != nil {
		t.Fatal(err)
	}
	if m, _ := ReadMode(ctx, st.Q); m != ModeService {
		t.Fatalf("mode %q", m)
	}
	if rows, _ := st.Q.ListVaultKept(ctx); len(rows) != 0 {
		t.Fatalf("kept keys left in service mode: %d", len(rows))
	}
}
