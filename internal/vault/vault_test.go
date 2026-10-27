package vault

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

func testVault(t *testing.T, fill byte) *Vault {
	t.Helper()
	v, err := New(bytes.Repeat([]byte{fill}, KeySize), 1)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestSealOpenRoundTrip(t *testing.T) {
	v := testVault(t, 1)
	r, err := v.Seal(Service, "dns.token", secrets.New("hunter2"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := v.Open(asRecord(r))
	if err != nil {
		t.Fatal(err)
	}
	if got.Reveal() != "hunter2" {
		t.Fatalf("got %q", got.Reveal())
	}
}

func TestOpenRejectsTamperingAndWrongKey(t *testing.T) {
	v := testVault(t, 1)
	p, err := v.Seal(Service, "x", secrets.New("payload"))
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(*Vault) error{
		"wrong master key": func(*Vault) error { _, err := testVault(t, 2).Open(asRecord(p)); return err },
		"flipped ciphertext bit": func(v *Vault) error {
			r := asRecord(p)
			r.Ciphertext = append([]byte(nil), r.Ciphertext...)
			r.Ciphertext[0] ^= 1
			_, err := v.Open(r)
			return err
		},
		"ciphertext moved to another id": func(v *Vault) error {
			r := asRecord(p)
			r.ID = "01OTHER"
			_, err := v.Open(r)
			return err
		},
		"class changed": func(v *Vault) error {
			r := asRecord(p)
			r.Class = string(Admin)
			_, err := v.Open(r)
			return err
		},
	}
	for name, f := range cases {
		t.Run(name, func(t *testing.T) {
			if f(v) == nil {
				t.Fatal("open succeeded")
			}
		})
	}
}

func TestCheckKeyCreatesCanaryThenRejectsOtherKey(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	if err := testVault(t, 1).CheckKey(ctx, st.Q); err != nil {
		t.Fatal(err)
	}
	if err := testVault(t, 1).CheckKey(ctx, st.Q); err != nil {
		t.Fatalf("same key on restart: %v", err)
	}
	if err := testVault(t, 2).CheckKey(ctx, st.Q); !errors.Is(err, ErrWrongMasterKey) {
		t.Fatalf("other key: got %v, want ErrWrongMasterKey", err)
	}
}

// Invariant 5 (storage side): a sealed secret never appears in clear in the database file, its WAL or a dump.
func TestInvariant_5_SecretsAreNotInDatabaseFile(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	v := testVault(t, 1)
	const plain = "plaintext-canary-7f3a9c"
	p, err := v.Seal(Service, "test", secrets.New(plain))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertSecret(ctx, p); err != nil {
		t.Fatal(err)
	}
	dump := filepath.Join(t.TempDir(), "dump.db")
	if err := st.BackupTo(ctx, dump); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{st.Path(), st.Path() + "-wal", dump} {
		b, err := os.ReadFile(f) //nolint:gosec // test temp files
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if bytes.Contains(b, []byte(plain)) {
			t.Fatalf("%s contains the plaintext", filepath.Base(f))
		}
	}
}

func TestLoadMasterKey(t *testing.T) {
	dir := t.TempDir()
	k, err := Keygen()
	if err != nil {
		t.Fatal(err)
	}
	good := filepath.Join(dir, "good")
	if err := os.WriteFile(good, []byte(k+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if m, err := LoadMasterKey(good); err != nil || len(m) != KeySize {
		t.Fatalf("good key: %v", err)
	}
	short := filepath.Join(dir, "short")
	if err := os.WriteFile(short, []byte("c2hvcnQ="), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"", filepath.Join(dir, "missing"), short} {
		if _, err := LoadMasterKey(p); err == nil {
			t.Fatalf("%q: accepted", p)
		}
	}
}

func TestRecoveryKeyRoundTrip(t *testing.T) {
	v := testVault(t, 1)
	key, display, check, err := NewRecoveryKey()
	if err != nil {
		t.Fatal(err)
	}
	shown := display.Reveal()
	if g := strings.Split(shown, "-"); len(g) != 14 || g[13] != check {
		t.Fatalf("display %d groups, last %q, check %q", len(g), g[len(g)-1], check)
	}
	parsed, err := ParseRecoveryKey(strings.ToLower(strings.ReplaceAll(shown, "-", " ")))
	if err != nil || !bytes.Equal(parsed, key) {
		t.Fatalf("parse: %v", err)
	}
	p, err := v.WrapMaster(key)
	if err != nil {
		t.Fatal(err)
	}
	r := recoveryRecord(p)
	m, err := RecoverMaster(r, parsed)
	if err != nil || !v.MatchesMaster(m) {
		t.Fatalf("recover: %v", err)
	}
	other, _, _, _ := NewRecoveryKey()
	if _, err := RecoverMaster(r, other); !errors.Is(err, ErrRecoveryKeyMismatch) {
		t.Fatalf("other key: %v", err)
	}
}

func TestParseRecoveryKeyCatchesTypos(t *testing.T) {
	_, display, _, err := NewRecoveryKey()
	if err != nil {
		t.Fatal(err)
	}
	s := []byte(display.Reveal())
	if s[0] == 'A' {
		s[0] = 'B'
	} else {
		s[0] = 'A'
	}
	for _, in := range []string{string(s), display.Reveal()[:20], "", "not-a-key"} {
		if _, err := ParseRecoveryKey(in); !errors.Is(err, ErrRecoveryKeyFormat) {
			t.Fatalf("%q: got %v", in, err)
		}
	}
}
