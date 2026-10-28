package traefik

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/vault"
)

func hostKey(t *testing.T) []byte {
	t.Helper()
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	sp, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return sp.Marshal()
}

// Setup step 5 without a real host: Hashi's key, save with sealed credentials and exactly the confirmed host key,
// validation (OS, root, write check in Hashi's own directory), ownership confirmation.
func TestSaveValidateConfirm(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	v, err := vault.New(bytes.Repeat([]byte{7}, vault.KeySize), 1)
	if err != nil {
		t.Fatal(err)
	}
	host := newFakeHost()
	probe, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "os", "os_alpine.txt"))
	host.probe = string(probe)
	var dialed sshx.Config
	m := NewManager(st, v, audit.New([]byte("k")), func(_ context.Context, c sshx.Config) (Conn, error) {
		dialed = c
		return fakeConn{host}, nil
	}, nil)

	line, err := m.Identity(ctx, "setup", "")
	if err != nil || !strings.HasPrefix(line, "ssh-ed25519 ") {
		t.Fatalf("identity: %q %v", line, err)
	}
	if again, _ := m.Identity(ctx, "setup", ""); again != line {
		t.Error("identity changed on the second call")
	}

	hk := hostKey(t)
	in := ConnectionInput{Name: "edge", Host: "192.0.2.10", Port: 22, User: "root", Auth: AuthPassword, HostKey: hk}
	if _, err := m.Save(ctx, "setup", "", "", in); !errors.Is(err, ErrInvalid) {
		t.Fatalf("password missing: %v", err)
	}
	bad := in
	bad.HostKey = nil
	bad.Credential = secrets.New("pw")
	if _, err := m.Save(ctx, "setup", "", "", bad); !errors.Is(err, ErrInvalid) {
		t.Errorf("no host key: %v", err)
	}
	in.Credential = secrets.New("correct horse battery staple")
	row, err := m.Save(ctx, "setup", "", "", in)
	if err != nil {
		t.Fatal(err)
	}
	if row.TraefikBinary != DefaultBinary || row.ConfigDir != DefaultConfigDir || row.ApiPort != 8080 || !bytes.Equal(row.HostKey, hk) {
		t.Errorf("saved %+v", row)
	}
	if err := m.ConfirmOwnership(ctx, "setup", "", row.ID); !errors.Is(err, ErrNotReady) {
		t.Errorf("confirm before validation: %v", err)
	}

	row, p, err := m.Validate(ctx, row.ID)
	if err != nil {
		t.Fatal(err)
	}
	if dialed.Auth.Password.Reveal() != "correct horse battery staple" || !bytes.Equal(dialed.Pinned.Key, hk) {
		t.Errorf("dialed with %+v", dialed)
	}
	if row.OsFamily != FamilyAlpine || row.InitSystem != InitOpenRC || row.Health != "healthy" || p.Arch == "" {
		t.Errorf("validated %+v", row)
	}
	if !host.dirs[StateDir] || len(host.writes) != 1 || !strings.HasPrefix(host.writes[0], StateDir+"/.hashi-write-check-") {
		t.Errorf("write check: dirs %v writes %v", host.dirs, host.writes)
	}
	if err := m.ConfirmOwnership(ctx, "setup", "", row.ID); err != nil {
		t.Fatal(err)
	}

	// Invariant 5: the password is sealed, never in the row or the audit log.
	if strings.Contains(string(mustDump(t, st)), "correct horse battery staple") {
		t.Error("password stored in plain text")
	}

	// A different host key belongs to a different machine: validation and confirmation start over.
	in.HostKey, in.Credential = hostKey(t), secrets.Value{}
	row, err = m.Save(ctx, "setup", "", row.ID, in)
	if err != nil {
		t.Fatal(err)
	}
	if row.OwnershipConfirmedAt.Valid || row.OsProbe != "" || !row.CredentialSecretID.Valid {
		t.Errorf("after re-pin: confirmed %v probe %q cred %v", row.OwnershipConfirmedAt.Valid, row.OsProbe, row.CredentialSecretID.Valid)
	}

	// A non-root login is refused.
	host.probe = strings.Replace(string(probe), "--- id\n0\n", "--- id\n1000\n", 1)
	if _, _, err := m.Validate(ctx, row.ID); !errors.Is(err, ErrUnsupported) {
		t.Errorf("non-root: %v", err)
	}
}

func mustDump(t *testing.T, st interface {
	BackupTo(context.Context, string) error
}) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dump.db")
	if err := st.BackupTo(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path) //nolint:gosec // test file
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// Invariant 13: an apply is verified through Traefik's API; when Traefik does not report Hashi's routers the plan
// fails and the connection says why.
func TestInvariant_13_TraefikApplyIsVerified(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	v, _ := vault.New(bytes.Repeat([]byte{7}, vault.KeySize), 1)
	if err := settings.Put(ctx, st.Q, settings.General{RootDomain: "example.com", AdminDomain: "hashi.example.com",
		InternalURL: "http://192.0.2.5:3340", SyncIntervalSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	host := newFakeHost()
	probe, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "os", "os_debian13.txt"))
	host.probe, host.version = string(probe), Version
	a := audit.New([]byte("k"))
	m := NewManager(st, v, a, func(context.Context, sshx.Config) (Conn, error) { return fakeConn{host}, nil }, nil)
	m.pingTimeout = 1500 * time.Millisecond
	eng := hsync.New(st, a, m.Subsystems()...)
	row, err := m.Save(ctx, "admin", "", "", ConnectionInput{Name: "edge", Host: "192.0.2.10", Port: 22, User: "root",
		Auth: AuthPassword, Credential: secrets.New("pw"), HostKey: hostKey(t)})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := m.Validate(ctx, row.ID); err != nil {
		t.Fatal(err)
	}
	if err := m.ConfirmOwnership(ctx, "admin", "", row.ID); err != nil {
		t.Fatal(err)
	}
	host.rawdata = []byte(`{"routers":{},"middlewares":{},"services":{}}`) // Traefik loaded nothing
	p, err := eng.Propose(ctx, Subsystem, row.ID, hsync.TriggerManual, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if err := eng.Approve(ctx, p.ID, "admin", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Apply(ctx, p.ID, "admin", ""); !errors.Is(err, ErrVerify) {
		t.Fatalf("apply: %v", err)
	}
	done, _ := eng.Get(ctx, p.ID)
	conn, _ := st.R.GetTraefikConnection(ctx, row.ID)
	if done.Status != hsync.StatusFailed || conn.Health != "failed" || !strings.Contains(conn.LastValidationError, "is missing") {
		t.Errorf("plan %s, health %s %q", done.Status, conn.Health, conn.LastValidationError)
	}
}
