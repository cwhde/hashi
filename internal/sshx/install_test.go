package sshx_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/sshx"
)

// localShell runs commands in a real shell with HOME set to dir.
type localShell struct{ dir string }

func (l localShell) Close() error { return nil }
func (l localShell) Run(_ context.Context, cmd string, in io.Reader) (sshx.Result, error) {
	c := exec.Command("sh", "-c", cmd)
	c.Env, c.Stdin = []string{"HOME=" + l.dir, "PATH=" + os.Getenv("PATH")}, in
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	err := c.Run()
	r := sshx.Result{Stdout: out.Bytes(), Stderr: errb.Bytes()}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		r.ExitCode, err = ee.ExitCode(), nil
	}
	return r, err
}

func keys(t *testing.T) (plain, locked secrets.Value) {
	t.Helper()
	_, k, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := ssh.MarshalPrivateKey(k, "")
	l, _ := ssh.MarshalPrivateKeyWithPassphrase(k, "", []byte("right"))
	return secrets.New(string(pem.EncodeToMemory(p))), secrets.New(string(pem.EncodeToMemory(l)))
}

// The real command: it creates the file private, adds the line once, and never joins it to a last line without newline.
func TestInstallCommand(t *testing.T) {
	dir := t.TempDir()
	plain, _ := keys(t)
	dial := func(context.Context, sshx.Config) (sshx.Runner, error) { return localShell{dir}, nil }
	c := sshx.Config{Auth: sshx.Auth{User: "root", Password: secrets.New("pw")}} // hashi:allow-secret-pattern
	own := sshx.Auth{User: "root", PrivateKey: plain}
	f := filepath.Join(dir, ".ssh", "authorized_keys")
	for range 2 {
		if err := sshx.InstallKey(t.Context(), dial, c, "ssh-ed25519 AAA hashi", own); err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(f)
	if string(b) != "ssh-ed25519 AAA hashi\n" {
		t.Fatalf("%q", b)
	}
	if st, _ := os.Stat(f); st.Mode().Perm() != 0o600 {
		t.Errorf("mode %o", st.Mode().Perm())
	}
	if err := os.WriteFile(f, []byte("ssh-rsa OLD me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := sshx.InstallKey(t.Context(), dial, c, "ssh-ed25519 AAA hashi", own); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(f); string(b) != "ssh-rsa OLD me\nssh-ed25519 AAA hashi\n" {
		t.Fatalf("%q", b)
	}
}

// Every failure tells the admin what to do next, always including the manual way.
func TestInstallKeyMessages(t *testing.T) {
	plain, locked := keys(t)
	dial := func(_ context.Context, c sshx.Config) (sshx.Runner, error) {
		return nil, errors.New("ssh login: ssh: handshake failed: ssh: unable to authenticate, attempted methods [none password]")
	}
	for _, tc := range []struct {
		auth sshx.Auth
		want string
	}{
		{sshx.Auth{User: "root", Password: secrets.New("x")}, "Retry with a private key"}, // hashi:allow-secret-pattern
		{sshx.Auth{User: "root", PrivateKey: plain}, "Retry with another key"},
		{sshx.Auth{User: "root", PrivateKey: locked}, "Enter its passphrase"},
		{sshx.Auth{User: "root", PrivateKey: locked, Passphrase: secrets.New("wrong")}, "does not open this key"}, // hashi:allow-secret-pattern
		{sshx.Auth{User: "root", PrivateKey: secrets.New("not a key")}, "not a private key"},
	} {
		err := sshx.InstallKey(t.Context(), dial, sshx.Config{Auth: tc.auth}, "k", sshx.Auth{})
		if !errors.Is(err, sshx.ErrInstall) || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "I'll do it myself") {
			t.Errorf("%q: %v", tc.want, err)
		}
	}
	// A passphrase given for a key without one is ignored: the key reaches the host.
	err := sshx.InstallKey(t.Context(), dial, sshx.Config{Auth: sshx.Auth{User: "root", PrivateKey: plain, Passphrase: secrets.New("x")}}, "k", sshx.Auth{}) // hashi:allow-secret-pattern
	if err == nil || !strings.Contains(err.Error(), "refused this key") {
		t.Errorf("needless passphrase: %v", err)
	}
	// The right passphrase reaches the host.
	err = sshx.InstallKey(t.Context(), dial, sshx.Config{Auth: sshx.Auth{User: "root", PrivateKey: locked, Passphrase: secrets.New("right")}}, "k", sshx.Auth{}) // hashi:allow-secret-pattern
	if err == nil || !strings.Contains(err.Error(), "refused this key") {
		t.Errorf("right passphrase: %v", err)
	}
}

// The proof login failing leaves the line on the host: the caller must be able to tell, to audit it.
func TestInstallKeyProofFailure(t *testing.T) {
	dir := t.TempDir()
	first := true
	dial := func(context.Context, sshx.Config) (sshx.Runner, error) {
		if first {
			first = false
			return localShell{dir}, nil
		}
		return nil, errors.New("ssh: unable to authenticate")
	}
	err := sshx.InstallKey(t.Context(), dial, sshx.Config{Auth: sshx.Auth{User: "root", Password: secrets.New("x")}}, "ssh-ed25519 A b", sshx.Auth{}) // hashi:allow-secret-pattern
	if !errors.Is(err, sshx.ErrKeyAdded) || !errors.Is(err, sshx.ErrInstall) {
		t.Fatalf("%v", err)
	}
}
