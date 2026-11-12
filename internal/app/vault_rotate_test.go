package app

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/pulse"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	"git.juzo.io/juzo/hashi/internal/traefik"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// `hashi vault rotate` needs the server stopped; afterwards the old key is refused, the server starts with the printed
// key, and an agent token issued before the rotation still works (no automation breaks).
func TestVaultRotateConsole(t *testing.T) {
	ctx := context.Background()
	cfg := newConfig(t)
	in := start(t, cfg) // creates and migrates the database
	if err := VaultRotate(ctx, cfg, strings.NewReader("x\n"), &bytes.Buffer{}); !errors.Is(err, store.ErrInUse) {
		t.Fatalf("rotate while the server runs: %v", err)
	}
	in.stop()

	st := storetest.Open(t, cfg.DBPath())
	oldKey, err := vault.LoadMasterKey(cfg.MasterKeyFile)
	if err != nil {
		t.Fatal(err)
	}
	v, err := vault.Load(ctx, st.Q, oldKey)
	if err != nil {
		t.Fatal(err)
	}
	recovery, display, _, err := vault.NewRecoveryKey()
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
	agent, token, err := pulse.New(st, v, audit.New(nil)).Create(ctx, "home", "systemd")
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	var out bytes.Buffer
	if err := VaultRotate(ctx, cfg, strings.NewReader(display.Reveal()+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	printed := lines[len(lines)-1]
	if err := os.WriteFile(cfg.MasterKeyFile+".new", []byte(printed), 0o600); err != nil {
		t.Fatal(err)
	}
	newKey, err := vault.LoadMasterKey(cfg.MasterKeyFile + ".new")
	if err != nil {
		t.Fatalf("printed key %q: %v", printed, err)
	}

	st = storetest.Open(t, cfg.DBPath())
	if _, err := vault.Load(ctx, st.Q, oldKey); !errors.Is(err, vault.ErrWrongMasterKey) {
		t.Fatalf("old key after rotation: %v", err)
	}
	nv, err := vault.Load(ctx, st.Q, newKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pulse.New(st, nv, audit.New(nil)).Heartbeat(ctx, agent.ID, token.Reveal(),
		pulse.Beat{Version: "v1", Hostname: "home", IntervalSeconds: 30}, netip.MustParseAddr("192.0.2.10")); err != nil {
		t.Fatalf("agent token from before the rotation: %v", err)
	}
	_ = st.Close()

	cfg.MasterKeyFile += ".new"
	start(t, cfg).stop()
}

// Disaster recovery (11-delivery.md §8): only a nightly copy and the recovery key survive. A fresh data folder with that
// copy, `hashi vault recover` and a start bring Hashi back with every secret readable.
func TestRestoreNightlyBackupOnFreshFolder(t *testing.T) {
	ctx := context.Background()
	cfg := newConfig(t)
	in := start(t, cfg)
	nightly := filepath.Join(cfg.DataDir, "backups", "hashi-"+time.Now().UTC().Format(time.DateOnly)+".db")
	deadline := time.Now().Add(readyWait)
	for {
		if _, err := os.Stat(nightly); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the nightly backup job did not run at the first start")
		}
		time.Sleep(50 * time.Millisecond)
	}
	in.stop()
	// Secrets and the recovery key go in after the copy, so the copy is written again below with them in it.
	st := storetest.Open(t, cfg.DBPath())
	key, err := vault.LoadMasterKey(cfg.MasterKeyFile)
	if err != nil {
		t.Fatal(err)
	}
	v, err := vault.Load(ctx, st.Q, key)
	if err != nil {
		t.Fatal(err)
	}
	recovery, display, _, err := vault.NewRecoveryKey()
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
	p, err := v.Seal(vault.Service, "dns.token", secrets.New("hunter2"))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertSecret(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := st.NightlyBackup(ctx, time.Now(), 14); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	// The new host: an empty folder, the copy as its database, no master key.
	fresh := t.TempDir()
	data, err := os.ReadFile(nightly) //nolint:gosec // test temp path
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fresh, "hashi.db"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	// The copy comes from an older Hashi: its schema is behind this binary's, which must not stop the recovery.
	old, err := store.Open(ctx, filepath.Join(fresh, "hashi.db")) // no migration: the copy as it was
	if err != nil {
		t.Fatal(err)
	}
	if _, err := old.Writer.ExecContext(ctx, `CREATE TABLE later_versions AS SELECT * FROM goose_db_version WHERE version_id > 20;
		DELETE FROM goose_db_version WHERE version_id > 20`); err != nil {
		t.Fatal(err)
	}
	_ = old.Close()
	restored := Config{DataDir: fresh}
	var out bytes.Buffer
	if err := VaultRecover(ctx, restored, strings.NewReader(display.Reveal()+"\n"), &out); err != nil {
		t.Fatal(err)
	}
	// Only the version rows were faked; the tables are current, so put the rows back before the server starts.
	if old, err = store.Open(ctx, filepath.Join(fresh, "hashi.db")); err != nil {
		t.Fatal(err)
	}
	if _, err := old.Writer.ExecContext(ctx, `INSERT INTO goose_db_version SELECT * FROM later_versions; DROP TABLE later_versions`); err != nil {
		t.Fatal(err)
	}
	_ = old.Close()
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	restored.MasterKeyFile = filepath.Join(fresh, "master.key")
	if err := os.WriteFile(restored.MasterKeyFile, []byte(lines[len(lines)-1]), 0o600); err != nil {
		t.Fatal(err)
	}
	start(t, restored).stop()
	var check bytes.Buffer
	if err := VaultCheck(ctx, restored, &check); err != nil || !strings.Contains(check.String(), "secrets open") {
		t.Fatalf("vault check: %v %q", err, check.String())
	}
	if strings.Contains(check.String(), "hunter2") {
		t.Fatal("vault check printed a secret")
	}
	wrong := restored
	wrong.MasterKeyFile = cfg.MasterKeyFile + ".other"
	if err := os.WriteFile(wrong.MasterKeyFile, []byte(vault.EncodeMasterKey(bytes.Repeat([]byte{1}, vault.KeySize))), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VaultCheck(ctx, wrong, &bytes.Buffer{}); !errors.Is(err, vault.ErrWrongMasterKey) {
		t.Fatalf("vault check with another key: %v", err)
	}
	st = storetest.Open(t, restored.DBPath())
	got, err := vault.LoadMasterKey(restored.MasterKeyFile)
	if err != nil {
		t.Fatal(err)
	}
	rv, err := vault.Load(ctx, st.Q, got)
	if err != nil {
		t.Fatal(err)
	}
	r, err := st.Q.GetSecret(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if s, err := rv.Open(r); err != nil || s.Reveal() != "hunter2" {
		t.Fatalf("secret after restore: %v", err)
	}
}

// Strict mode (02 §5.4): a restart starts locked and still serves; the master key file is ignored.
func TestStrictModeStartsLocked(t *testing.T) {
	ctx := context.Background()
	cfg := newConfig(t)
	start(t, cfg).stop() // first start creates the edge token and other start-up secrets
	st := storetest.Open(t, cfg.DBPath())
	key, err := vault.LoadMasterKey(cfg.MasterKeyFile)
	if err != nil {
		t.Fatal(err)
	}
	v, err := vault.Load(ctx, st.Q, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.EnableStrict(ctx, st.Q); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	in := start(t, cfg)
	c := in.client()
	if r := c.do(http.MethodGet, "/readyz", nil); r.status != 200 {
		t.Fatalf("readyz while locked: %d %s", r.status, r.raw)
	}
	if !strings.Contains(in.logs.String(), "vault locked") {
		t.Fatal("no locked warning in the log")
	}
}

func TestKeptPurposesNameRealSecrets(t *testing.T) {
	if !slices.Contains(vault.KeptPurposes, traefik.EdgeTokenPurpose) {
		t.Fatal("the edge token must stay readable while locked")
	}
}
