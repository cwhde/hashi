//go:build integration

package firewall

import (
	"context"
	"io"
	"log/slog"
	"net/netip"
	"os"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/security"
	"git.juzo.io/juzo/hashi/internal/sshx"
)

// TestRealHostLinkOnDevTarget runs the host link over real SSH against a disposable machine (05 §6): Hashi's table from
// the captured shapes, a staged address drained into a ban with T_ban, a stranger in dyn removed, the unban pushed. It
// deletes its table afterwards. HASHI_FW_TARGET=user@addr, HASHI_FW_KEY=<private key file>.
func TestRealHostLinkOnDevTarget(t *testing.T) {
	target, keyFile := os.Getenv("HASHI_FW_TARGET"), os.Getenv("HASHI_FW_KEY")
	if target == "" || keyFile == "" {
		t.Skip("set HASHI_FW_TARGET and HASHI_FW_KEY")
	}
	user, addr, _ := strings.Cut(target, "@")
	key, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	hk, err := sshx.Scan(ctx, addr+":22", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	h, err := sshx.Dial(ctx, sshx.Config{Addr: addr + ":22", Pinned: hk, Auth: sshx.Auth{User: user, PrivateKey: secrets.New(string(key))}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.Close() }()
	shapes, err := os.ReadFile("../../testdata/contracts/nft/shapes.nft")
	if err != nil {
		t.Fatal(err)
	}
	sh := func(cmd string, stdin []byte) string {
		t.Helper()
		var in io.Reader
		if stdin != nil {
			in = strings.NewReader(string(stdin))
		}
		r, err := h.Run(ctx, cmd, in)
		if err != nil || !r.OK() {
			t.Fatalf("%s: %v\n%s%s", cmd, err, r.Stdout, r.Stderr)
		}
		return string(r.Stdout)
	}
	sh("nft delete table inet hashi 2>/dev/null; nft -f -", shapes)
	t.Cleanup(func() { _, _ = h.Run(context.Background(), "nft delete table inet hashi", nil) })
	sh("nft add element inet hashi staging4 '{ 198.51.100.7 }'", nil)

	e := newEnv(t)
	hrow, err := e.m.Save(ctx, "admin", "", "", managedInput(t))
	if err != nil {
		t.Fatal(err)
	}
	row, _ := e.st.R.GetFirewallHost(ctx, hrow.ID)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	eng := security.NewEngine(e.st, log, time.Now)
	ls := &linkSession{l: NewLink(e.m, eng, log), row: row, h: h, nft: "nft"}
	if err := ls.tick(ctx); err != nil {
		t.Fatal(err)
	}
	dyn, err := parseSetElems([]byte(sh("nft -j list set inet hashi dyn4", nil)))
	if err != nil || len(dyn) != 1 || dyn[0].prefix.Addr().String() != "198.51.100.7" || dyn[0].expires < 890 {
		t.Fatalf("dyn4: %+v %v", dyn, err) // shapes.nft's own 203.0.113.9 is not Hashi's and is gone
	}
	if s, _ := parseSetElems([]byte(sh("nft -j list set inet hashi staging4", nil))); len(s) != 0 {
		t.Fatalf("staging4 not drained: %+v", s)
	}
	if _, err := eng.Remove(ctx, netip.MustParsePrefix("198.51.100.7/32"), "admin", "test"); err != nil {
		t.Fatal(err)
	}
	if err := ls.tick(ctx); err != nil {
		t.Fatal(err)
	}
	if s, _ := parseSetElems([]byte(sh("nft -j list set inet hashi dyn4", nil))); len(s) != 0 {
		t.Fatalf("unban not pushed: %+v", s)
	}
	if s, _ := parseSetElems([]byte(sh("nft -j list set inet hashi dyn6", nil))); len(s) != 0 {
		t.Fatalf("stranger in dyn6 not removed: %+v", s)
	}
}
