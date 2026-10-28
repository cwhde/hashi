package traefik

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/vault"
)

type fakeConn struct{ *fakeHost }

func (fakeConn) Close() error { return nil }

// The traefik subsystem end to end through the sync engine: a validated connection is planned (high risk: first
// install), approved, applied and verified; file hashes and health are stored; the next plan is empty.
func TestManagerPlansAndAppliesThroughTheEngine(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	v, err := vault.New(bytes.Repeat([]byte{7}, vault.KeySize), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Put(ctx, st.Q, settings.General{RootDomain: "example.com", AdminDomain: "hashi.example.com",
		InternalURL: "http://192.0.2.5:3340/setup", SyncIntervalSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	priv, _, err := sshx.GenerateKey("hashi@test")
	if err != nil {
		t.Fatal(err)
	}
	rec, err := v.Seal(vault.Service, IdentityPurpose, priv)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertSecret(ctx, rec); err != nil {
		t.Fatal(err)
	}
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	sp, _ := ssh.NewPublicKey(pub)
	if err := st.Q.InsertTraefikConnection(ctx, db.InsertTraefikConnectionParams{ID: "c1", Name: "edge", SshHost: "192.0.2.10",
		SshPort: 22, SshUser: "root", Auth: "hashi_key", HostKeyType: sp.Type(), HostKey: sp.Marshal(),
		HostKeyFingerprint: ssh.FingerprintSHA256(sp)}); err != nil {
		t.Fatal(err)
	}

	host := newFakeHost()
	var dialed sshx.Config
	m := NewManager(st, v, audit.New([]byte("k")), func(_ context.Context, c sshx.Config) (Conn, error) {
		dialed = c
		return fakeConn{host}, nil
	}, func(_ context.Context, arch string) ([]byte, error) { return []byte("ELF-" + arch), nil })
	m.pingTimeout = 1500 * time.Millisecond
	eng := hsync.New(st, audit.New([]byte("k")), m.Subsystems()...)

	if _, err := eng.Propose(ctx, Subsystem, "c1", hsync.TriggerManual, "admin"); !errors.Is(err, ErrNotReady) {
		t.Fatalf("unvalidated connection: err = %v", err)
	}
	probe, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "os", "os_debian13.txt"))
	if err := st.Q.SetTraefikConnectionDetected(ctx, db.SetTraefikConnectionDetectedParams{OsFamily: FamilyDebian,
		PackageManager: "apt-get", InitSystem: InitSystemd, OsProbe: string(probe), DiscoveryJson: `{}`, ID: "c1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Propose(ctx, Subsystem, "c1", hsync.TriggerManual, "admin"); !errors.Is(err, ErrNotReady) {
		t.Fatalf("ownership not confirmed: err = %v", err)
	}
	if err := st.Q.ConfirmTraefikOwnership(ctx, db.ConfirmTraefikOwnershipParams{OwnershipConfirmedAt: sql.NullInt64{Int64: 1, Valid: true}, ID: "c1"}); err != nil {
		t.Fatal(err)
	}

	p, err := eng.Propose(ctx, Subsystem, "c1", hsync.TriggerManual, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if p.Risk != hsync.High || p.Status != hsync.StatusPending {
		t.Fatalf("first install plan: risk %s status %s", p.Risk, p.Status)
	}
	if dialed.Pinned.Fingerprint != ssh.FingerprintSHA256(sp) || dialed.Auth.PrivateKey.IsZero() || dialed.Addr != "192.0.2.10:22" {
		t.Errorf("dialed %+v", dialed)
	}
	if err := eng.Approve(ctx, p.ID, "admin", "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	host.version = "" // the binary is uploaded by the apply
	done, err := eng.Apply(ctx, p.ID, "admin", "192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != hsync.StatusApplied {
		t.Fatalf("status %s, error %q", done.Status, done.Error)
	}
	if string(host.files[DefaultBinary]) != "ELF-amd64" {
		t.Errorf("binary %q", host.files[DefaultBinary])
	}
	if !bytes.Contains(host.files[DefaultConfigDir+"/dynamic/40-hashi-security.yml"], []byte("http://192.0.2.5:3340/api/edge-auth/forward")) {
		t.Error("forward auth does not use the internal URL's origin")
	}
	files, _ := st.R.ListTraefikFiles(ctx, "c1")
	conn, _ := st.R.GetTraefikConnection(ctx, "c1")
	if len(files) != 5 || conn.Health != "healthy" || conn.InstalledVersion != Version {
		t.Errorf("files %d health %s version %q", len(files), conn.Health, conn.InstalledVersion)
	}

	host.version = Version
	if again, err := eng.Propose(ctx, Subsystem, "c1", hsync.TriggerReconcile, "system"); err != nil || again != nil {
		t.Errorf("second plan: %+v %v", again, err)
	}
}
