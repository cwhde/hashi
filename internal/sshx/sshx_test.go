package sshx_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/sshx/sshxtest"
)

func echoExec(cmd string, stdin io.Reader, stdout, stderr io.Writer) int {
	switch cmd {
	case "fail":
		_, _ = fmt.Fprint(stderr, "nope")
		return 3
	case "cat":
		_, _ = io.Copy(stdout, stdin)
		return 0
	case "big":
		_, _ = stdout.Write(bytes.Repeat([]byte("x"), sshx.MaxOutput+10))
		return 0
	}
	_, _ = fmt.Fprint(stdout, "ran: "+cmd)
	return 0
}

func pin(s *sshxtest.Server) sshx.HostKey {
	return sshx.HostKey{Type: s.HostKey.Type(), Key: s.HostKey.Marshal(), Fingerprint: ssh.FingerprintSHA256(s.HostKey)}
}

func dial(t *testing.T, s *sshxtest.Server, auth sshx.Auth) *sshx.Client {
	t.Helper()
	c, err := sshx.Dial(context.Background(), sshx.Config{Addr: s.Addr, Auth: auth, Pinned: pin(s), Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestScanReturnsTheHostKeyWithoutLoggingIn(t *testing.T) {
	s := sshxtest.Start(t, "root", testPassword, nil, echoExec)
	hk, err := sshx.Scan(context.Background(), s.Addr, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if hk.Fingerprint != ssh.FingerprintSHA256(s.HostKey) || !strings.HasPrefix(hk.Fingerprint, "SHA256:") || hk.Type != "ssh-ed25519" {
		t.Fatalf("scan = %+v", hk)
	}
	back, err := sshx.ParseHostKey(hk.Key)
	if err != nil || back.Fingerprint != hk.Fingerprint {
		t.Fatalf("round trip: %+v %v", back, err)
	}
	if len(s.Ran()) != 0 {
		t.Fatal("scan ran something")
	}
}

func TestDialRequiresAPinAndRefusesAnotherKey(t *testing.T) {
	s := sshxtest.Start(t, "root", testPassword, nil, echoExec)
	auth := pwAuth(testPassword)
	if _, err := sshx.Dial(context.Background(), sshx.Config{Addr: s.Addr, Auth: auth}); err == nil {
		t.Fatal("dial without a pin succeeded")
	}
	other := sshxtest.Start(t, "root", testPassword, nil, echoExec)
	_, err := sshx.Dial(context.Background(), sshx.Config{Addr: s.Addr, Auth: auth, Pinned: pin(other)})
	if !errors.Is(err, sshx.ErrHostKeyMismatch) {
		t.Fatalf("err = %v, want host key mismatch", err)
	}
	if len(s.Ran()) != 0 {
		t.Fatal("a command ran on a host with the wrong key")
	}
	if _, err := sshx.Dial(context.Background(), sshx.Config{Addr: s.Addr, Pinned: pin(s),
		Auth: pwAuth(testPassword + "-wrong")}); err == nil {
		t.Fatal("wrong password accepted")
	}
}

func TestKeyLoginWithHashiGeneratedKey(t *testing.T) {
	priv, line, err := sshx.GenerateKey("hashi@test")
	if err != nil {
		t.Fatal(err)
	}
	pub, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line))
	if err != nil {
		t.Fatalf("authorized_keys line %q: %v", line, err)
	}
	if !strings.HasSuffix(line, " hashi@test") || strings.Contains(fmt.Sprint(priv), "PRIVATE") {
		t.Fatalf("line %q, private renders %q", line, fmt.Sprint(priv))
	}
	s := sshxtest.Start(t, "hashi", "", pub, echoExec)
	c := dial(t, s, sshx.Auth{User: "hashi", PrivateKey: priv})
	r, err := c.Run(context.Background(), "uname", nil)
	if err != nil || string(r.Stdout) != "ran: uname" || !r.OK() {
		t.Fatalf("run = %+v %v", r, err)
	}
}

func TestEncryptedKeyNeedsItsPassphrase(t *testing.T) {
	_, edPriv, _ := ed25519.GenerateKey(rand.Reader)
	block, err := ssh.MarshalPrivateKeyWithPassphrase(edPriv, "", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	pemKey := secrets.New(string(pemEncode(block)))
	signer, _ := ssh.NewSignerFromKey(edPriv)
	s := sshxtest.Start(t, "root", "", signer.PublicKey(), echoExec)
	if _, err := sshx.Dial(context.Background(), sshx.Config{Addr: s.Addr, Pinned: pin(s),
		Auth: sshx.Auth{User: "root", PrivateKey: pemKey}}); err == nil {
		t.Fatal("encrypted key without passphrase accepted")
	}
	dial(t, s, sshx.Auth{User: "root", PrivateKey: pemKey, Passphrase: secrets.New("secret")})
}

func TestRunExitCodesStdinAndCap(t *testing.T) {
	s := sshxtest.Start(t, "root", testPassword, nil, echoExec)
	c := dial(t, s, pwAuth(testPassword))
	ctx := context.Background()
	r, err := c.Run(ctx, "fail", nil)
	if err != nil || r.ExitCode != 3 || string(r.Stderr) != "nope" || r.OK() {
		t.Fatalf("fail = %+v %v", r, err)
	}
	r, err = c.Run(ctx, "cat", strings.NewReader("hello"))
	if err != nil || string(r.Stdout) != "hello" {
		t.Fatalf("cat = %+v %v", r, err)
	}
	r, err = c.Run(ctx, "big", nil)
	if err != nil || len(r.Stdout) != sshx.MaxOutput || !r.Truncated {
		t.Fatalf("big: %d bytes truncated=%v err=%v", len(r.Stdout), r.Truncated, err)
	}
}

func TestQuote(t *testing.T) {
	for in, want := range map[string]string{"a b": "'a b'", "it's": `'it'\''s'`, "": "''", "$(x)": "'$(x)'"} {
		if got := sshx.Quote(in); got != want {
			t.Errorf("Quote(%q) = %s, want %s", in, got, want)
		}
	}
}

// TestInvariant_8_AtomicWriteOnlyOnChange: identical content is never rewritten; a change goes through a temp file
// in the same directory and a rename, leaving no temp file behind.
func TestInvariant_8_AtomicWriteOnlyOnChange(t *testing.T) {
	s := sshxtest.Start(t, "root", testPassword, nil, echoExec)
	c := dial(t, s, pwAuth(testPassword))
	dir := t.TempDir()
	p := filepath.Join(dir, "00-hashi-core.yml")

	if h, err := c.FileHash(p); err != nil || h != "" {
		t.Fatalf("missing file hash = %q %v", h, err)
	}
	if _, err := c.ReadFile(p); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("read missing: %v", err)
	}
	wrote, err := c.WriteFileAtomic(p, []byte("a: 1\n"), 0o640)
	if err != nil || !wrote {
		t.Fatalf("first write = %v %v", wrote, err)
	}
	st1, _ := os.Stat(p)
	if st1.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v", st1.Mode())
	}
	ino := inode(t, p)
	wrote, err = c.WriteFileAtomic(p, []byte("a: 1\n"), 0o640)
	if err != nil || wrote {
		t.Fatalf("identical write = %v %v", wrote, err)
	}
	if inode(t, p) != ino {
		t.Fatal("identical content replaced the file")
	}
	wrote, err = c.WriteFileAtomic(p, []byte("a: 2\n"), 0o600)
	if err != nil || !wrote {
		t.Fatalf("changed write = %v %v", wrote, err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "a: 2\n" || inode(t, p) == ino {
		t.Fatalf("content %q, replaced=%v", got, inode(t, p) != ino)
	}
	if h, _ := c.FileHash(p); h != sshx.Hash([]byte("a: 2\n")) {
		t.Fatal("hash mismatch after write")
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 1 {
		t.Fatalf("leftover files: %v", ents)
	}
}

func TestMkdirAll(t *testing.T) {
	s := sshxtest.Start(t, "root", testPassword, nil, echoExec)
	c := dial(t, s, pwAuth(testPassword))
	p := filepath.Join(t.TempDir(), "etc", "hashi", "traefik", "certs")
	if err := c.MkdirAll(p, 0o700); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil || !st.IsDir() || st.Mode().Perm() != 0o700 {
		t.Fatalf("stat = %v %v", st, err)
	}
}

func TestHTTPThroughTheTunnel(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, "OK "+r.URL.Path)
	}))
	defer api.Close()
	s := sshxtest.Start(t, "root", testPassword, nil, echoExec)
	s.Tunnels["127.0.0.1:8080"] = api.Listener.Addr().String()
	c := dial(t, s, pwAuth(testPassword))
	resp, err := c.HTTPClient(8080, 5*time.Second).Get("http://traefik/ping")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if string(b) != "OK /ping" {
		t.Fatalf("body %q", b)
	}
	if _, err := c.DialLoopback(9999); err == nil {
		t.Fatal("tunnel to an unknown port succeeded")
	}
}

func TestAddrBracketsIPv6(t *testing.T) {
	if got := sshx.Addr("2001:db8::1", 22); got != "[2001:db8::1]:22" {
		t.Fatal(got)
	}
	if _, err := net.ResolveTCPAddr("tcp", sshx.Addr("192.0.2.1", 2222)); err != nil {
		t.Fatal(err)
	}
}

func TestFetchFallsBackToCurlOrWget(t *testing.T) {
	var tool string
	s := sshxtest.Start(t, "root", testPassword, nil, func(cmd string, _ io.Reader, stdout, _ io.Writer) int {
		if !strings.Contains(cmd, "'http://127.0.0.1:8080/api/rawdata'") {
			return 2
		}
		if tool == "" {
			return 127
		}
		_, _ = fmt.Fprint(stdout, `{"routers":{}}`)
		return 0
	})
	s.NoForwarding = true
	c := dial(t, s, pwAuth(testPassword))
	if _, err := c.Fetch(context.Background(), 8080, "/api/rawdata"); !errors.Is(err, sshx.ErrNoFetch) {
		t.Fatalf("no tool: err = %v", err)
	}
	tool = "wget"
	b, err := c.Fetch(context.Background(), 8080, "/api/rawdata")
	if err != nil || string(b) != `{"routers":{}}` {
		t.Fatalf("fallback = %q %v", b, err)
	}
}

func TestFetchUsesTheTunnelAndReportsHTTPErrors(t *testing.T) {
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ping" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprint(w, "OK")
	}))
	defer api.Close()
	s := sshxtest.Start(t, "root", testPassword, nil, echoExec)
	s.Tunnels["127.0.0.1:8080"] = api.Listener.Addr().String()
	c := dial(t, s, pwAuth(testPassword))
	if b, err := c.Fetch(context.Background(), 8080, "/ping"); err != nil || string(b) != "OK" {
		t.Fatalf("ping = %q %v", b, err)
	}
	if _, err := c.Fetch(context.Background(), 8080, "/nope"); err == nil || !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("404: %v", err)
	}
	if len(s.Ran()) != 0 {
		t.Fatalf("ran %v although the tunnel works", s.Ran())
	}
}
