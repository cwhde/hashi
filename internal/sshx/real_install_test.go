//go:build integration

package sshx_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/sshx"
)

// TestRealKeyInstall runs a key install on a disposable host over real sshd. HASHI_INSTALL_TARGET=addr names it;
// HASHI_INSTALL_KEY holds a passphrase-protected private key ("right") authorized for root there.
func TestRealKeyInstall(t *testing.T) {
	addr, keyFile := os.Getenv("HASHI_INSTALL_TARGET"), os.Getenv("HASHI_INSTALL_KEY")
	if addr == "" {
		t.Skip("set HASHI_INSTALL_TARGET and HASHI_INSTALL_KEY")
	}
	ctx := context.Background()
	hk, err := sshx.Scan(ctx, addr+":22", 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	locked, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	sp, _ := ssh.NewPublicKey(pub)
	block, _ := ssh.MarshalPrivateKey(priv, "")
	own := sshx.Auth{User: "root", PrivateKey: secrets.New(string(pem.EncodeToMemory(block)))}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sp))) + " hashi-test"
	dial := func(ctx context.Context, c sshx.Config) (sshx.Runner, error) { return sshx.Dial(ctx, c) }
	cfg := func(a sshx.Auth) sshx.Config { return sshx.Config{Addr: addr + ":22", Pinned: hk, Auth: a} }

	if err := sshx.InstallKey(ctx, dial, cfg(sshx.Auth{User: "root", Password: secrets.New("wrong")}), line, own); !errors.Is(err, sshx.ErrInstall) || // hashi:allow-secret-pattern
		!strings.Contains(err.Error(), "Retry with a private key") {
		t.Fatalf("password: %v", err)
	}
	if err := sshx.InstallKey(ctx, dial, cfg(sshx.Auth{User: "root", PrivateKey: secrets.New(string(locked))}), line, own); !errors.Is(err, sshx.ErrInstall) ||
		!strings.Contains(err.Error(), "Enter its passphrase") {
		t.Fatalf("no passphrase: %v", err)
	}
	if err := sshx.InstallKey(ctx, dial, cfg(sshx.Auth{User: "root", PrivateKey: secrets.New(string(locked)), Passphrase: secrets.New("right")}), line, own); err != nil { // hashi:allow-secret-pattern
		t.Fatal(err)
	}
	c, err := sshx.Dial(ctx, cfg(own))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	r, err := c.Run(ctx, "grep -cxF '"+line+"' ~/.ssh/authorized_keys", nil)
	if err != nil || strings.TrimSpace(string(r.Stdout)) != "1" {
		t.Fatalf("%v %s", err, r.Stdout)
	}
}
