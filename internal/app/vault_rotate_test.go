package app

import (
	"bytes"
	"context"
	"errors"
	"net/netip"
	"os"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/pulse"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
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
