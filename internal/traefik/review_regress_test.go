package traefik

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// regressSetup is a validated, confirmed connection (Debian 13, Hashi's key) with the given discovery.
func regressSetup(t *testing.T, disc string) (*store.Store, *Manager, *hsync.Engine, *fakeHost) {
	t.Helper()
	ctx := context.Background()
	st := storetest.New(t)
	v, _ := vault.New(bytes.Repeat([]byte{7}, vault.KeySize), 1)
	_ = settings.Put(ctx, st.Q, settings.General{RootDomain: "example.com", AdminDomain: "hashi.example.com",
		InternalURL: "http://192.0.2.5:3340", SyncIntervalSeconds: 3600})
	priv, _, _ := sshx.GenerateKey("hashi@test")
	rec, _ := v.Seal(vault.Service, IdentityPurpose, priv)
	_ = st.Q.InsertSecret(ctx, rec)
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	sp, _ := ssh.NewPublicKey(pub)
	_ = st.Q.InsertTraefikConnection(ctx, db.InsertTraefikConnectionParams{ID: "c1", Name: "edge", SshHost: "192.0.2.10",
		SshPort: 22, SshUser: "root", Auth: "hashi_key", HostKeyType: sp.Type(), HostKey: sp.Marshal(),
		HostKeyFingerprint: ssh.FingerprintSHA256(sp)})
	host := newFakeHost()
	m := NewManager(st, v, audit.New([]byte("k")), func(_ context.Context, c sshx.Config) (Conn, error) {
		return fakeConn{host}, nil
	}, func(_ context.Context, arch string) ([]byte, error) { return []byte("ELF-" + arch), nil })
	m.pingTimeout = 1500 * time.Millisecond
	eng := hsync.New(st, audit.New([]byte("k")), m.Subsystems()...)
	probe, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "os", "os_debian13.txt"))
	_ = st.Q.SetTraefikConnectionDetected(ctx, db.SetTraefikConnectionDetectedParams{OsFamily: FamilyDebian,
		PackageManager: "apt-get", InitSystem: InitSystemd, OsProbe: string(probe), DiscoveryJson: disc, ID: "c1"})
	_ = st.Q.ConfirmTraefikOwnership(ctx, db.ConfirmTraefikOwnershipParams{OwnershipConfirmedAt: sql.NullInt64{Int64: 1, Valid: true}, ID: "c1"})
	return st, m, eng, host
}

func applyAll(t *testing.T, eng *hsync.Engine) {
	t.Helper()
	ctx := context.Background()
	p, err := eng.Propose(ctx, Subsystem, "c1", hsync.TriggerManual, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if p.Status == hsync.StatusPending {
		_ = eng.Approve(ctx, p.ID, "admin", "192.0.2.1")
	}
	done, err := eng.Apply(ctx, p.ID, "admin", "192.0.2.1")
	if err != nil || done.Status != hsync.StatusApplied {
		t.Fatalf("apply: %v %+v", err, done)
	}
}

// Review of PR #32, finding 1: existing config is backed up once, at takeover; the plan after it is empty.
func TestRegress_BackupOnlyOnce(t *testing.T) {
	st, _, eng, host := regressSetup(t, `{"existing_configs":["/etc/traefik"]}`)
	applyAll(t, eng)
	host.version = Version
	again, err := eng.Propose(context.Background(), Subsystem, "c1", hsync.TriggerReconcile, "system")
	if err != nil {
		t.Fatal(err)
	}
	if again != nil {
		items, _ := st.R.ListSyncPlanItems(context.Background(), again.ID)
		for _, it := range items {
			t.Logf("item: %s risk=%s", it.Object, it.Risk)
		}
		t.Fatalf("plan after successful install is not empty: risk=%s status=%s", again.Risk, again.Status)
	}
}

// Finding 2: a host that already runs the pinned version gets installed_version, so Distribute reaches it.
func TestRegress_InstalledVersionWhenBinaryMatches(t *testing.T) {
	st, _, eng, host := regressSetup(t, `{}`)
	host.version = Version // binary already at the pinned version
	applyAll(t, eng)
	conn, _ := st.R.GetTraefikConnection(context.Background(), "c1")
	if conn.InstalledVersion == "" {
		t.Fatalf("installed_version empty after a verified apply; Distribute skips this host")
	}
}

// Finding 3 (04 §2.1): the running binary is kept as traefik.previous and put back when the new one does not start.
func TestRegress_FailedUpgradeRestoresPreviousBinary(t *testing.T) {
	ctx := context.Background()
	host := newFakeHost()
	files, _ := Render(Options{Paths: Paths{DefaultBinary, DefaultConfigDir}, APIPort: 8080, ForwardAuthPort: 8081, HashiURL: "http://192.0.2.5:3340"})
	svc, _ := ServiceFor(InitSystemd, Paths{DefaultBinary, DefaultConfigDir})
	d := Desired{Paths: Paths{DefaultBinary, DefaultConfigDir}, APIPort: 8080, Files: files, Service: svc, PingTimeout: 1500 * time.Millisecond,
		Binary: func(context.Context) ([]byte, error) { return []byte("NEW"), nil }}
	// host already fully installed with an older Traefik
	host.files[DefaultBinary] = []byte("OLD")
	host.version = "v3.7.1"
	for _, dir := range d.Paths.Directories() {
		host.dirs[dir.Path] = true
	}
	for _, f := range files {
		host.files[f.Path] = f.Data
	}
	host.files[svc.Path] = svc.Data
	host.startOK = func([]byte) bool { return string(host.files[DefaultBinary]) == "OLD" } // the new binary does not come up
	diff, err := Observe(ctx, host, d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(ctx, host, d, diff, time.Now()); !errors.Is(err, ErrRestored) {
		t.Fatalf("apply: %v, want ErrRestored", err)
	}
	if string(host.files[DefaultBinary]) != "OLD" || string(host.files[DefaultBinary+".previous"]) != "OLD" || !host.up {
		t.Fatalf("binary %q, previous %q, up %v", host.files[DefaultBinary], host.files[DefaultBinary+".previous"], host.up)
	}
}

// Finding 2, end to end: a certificate issued after such an install is distributed.
func TestRegress_DistributeReachesHostWithMatchingBinary(t *testing.T) {
	ctx := context.Background()
	st, m, eng, host := regressSetup(t, `{}`)
	host.version = Version
	applyAll(t, eng)
	crt, key := testCert(t, "example.com")
	fp, _ := Cert{Name: "example.com", CertPEM: crt}.Fingerprint()
	krec, _ := m.vault.Seal(vault.Service, "certificate.key", secrets.New(string(key)))
	if err := st.Q.InsertSecret(ctx, krec); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertCertificate(ctx, db.InsertCertificateParams{ID: "cert1", Name: "example.com", NamesJson: `["example.com"]`}); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.SetCertificateIssued(ctx, db.SetCertificateIssuedParams{NamesJson: `["example.com"]`, CertPem: string(crt),
		KeySecretID: sql.NullString{String: krec.ID, Valid: true}, Fingerprint: fp, ID: "cert1"}); err != nil {
		t.Fatal(err)
	}
	dial, _ := tlsEdge(t, crt, key)
	m.tlsDial = func(ctx context.Context, _ string) (net.Conn, error) { return dial(ctx) }
	sum, err := m.Distribute(eng)(ctx)
	dists, _ := st.R.ListCertificateDistributions(ctx)
	if err != nil || len(dists) != 1 || dists[0].State != "verified" {
		t.Fatalf("distribute %q %v: %+v", sum, err, dists)
	}
}
