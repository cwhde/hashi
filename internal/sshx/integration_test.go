//go:build integration

package sshx_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/itest"
	"git.juzo.io/juzo/hashi/internal/sshx"
)

// sshdImage is the same Alpine release as the S4 dev edge; OpenSSH comes from its package repository.
const sshdImage = "alpine:3.23"

func startSSHD(t *testing.T, port int, authorizedKey string) itest.Container {
	t.Helper()
	script := "apk add -q openssh-server openssh-sftp-server >/dev/null && ssh-keygen -A >/dev/null && " +
		"echo \"root:$ITEST_PW\" | chpasswd && mkdir -p /root/.ssh && printf '%s\\n' \"$AUTHORIZED\" > /root/.ssh/authorized_keys && " +
		"chmod 700 /root/.ssh && chmod 600 /root/.ssh/authorized_keys && " +
		"exec /usr/sbin/sshd -D -e -p " + strconv.Itoa(port) + " -o PermitRootLogin=yes -o PasswordAuthentication=yes"
	c := itest.Run(t, port, []string{"-e", "AUTHORIZED=" + authorizedKey, "-e", "ITEST_PW=" + testPassword}, sshdImage, "sh", "-c", script)
	itest.WaitFor(t, 90*time.Second, "sshd", func() bool { return itest.SSHBanner(c.Addr) })
	return c
}

// TestRealSSHD runs the client against OpenSSH: scan + pin, key and password login, wrong key refused, commands,
// atomic SFTP writes (posix-rename) and a loopback tunnel.
func TestRealSSHD(t *testing.T) {
	priv, line, err := sshx.GenerateKey("hashi@itest")
	if err != nil {
		t.Fatal(err)
	}
	c := startSSHD(t, 22022, line)
	ctx := context.Background()
	hk, err := sshx.Scan(ctx, c.Addr, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("pinned %s %s", hk.Type, hk.Fingerprint)

	cli, err := sshx.Dial(ctx, sshx.Config{Addr: c.Addr, Pinned: hk, Auth: sshx.Auth{User: "root", PrivateKey: priv}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cli.Close() }()
	r, err := cli.Run(ctx, "cat /etc/os-release; exit 4", nil)
	if err != nil || r.ExitCode != 4 || !strings.Contains(string(r.Stdout), "ID=alpine") {
		t.Fatalf("run = %+v %v", r, err)
	}

	pw, err := sshx.Dial(ctx, sshx.Config{Addr: c.Addr, Pinned: hk, Auth: pwAuth(testPassword)})
	if err != nil {
		t.Fatalf("password login: %v", err)
	}
	_ = pw.Close()

	// Another host key for the same address (e.g. a reinstalled host) is refused before login.
	old := hk
	c.Exec(t, "sh", "-c", "rm /etc/ssh/ssh_host_*; ssh-keygen -A >/dev/null; kill -HUP 1")
	itest.WaitFor(t, 20*time.Second, "sshd restart", func() bool {
		k, err := sshx.Scan(ctx, c.Addr, 2*time.Second)
		return err == nil && k.Fingerprint != hk.Fingerprint
	})
	if _, err := sshx.Dial(ctx, sshx.Config{Addr: c.Addr, Pinned: old, Auth: sshx.Auth{User: "root", PrivateKey: priv}}); !errors.Is(err, sshx.ErrHostKeyMismatch) {
		t.Fatalf("changed host key: err = %v", err)
	}
	hk, _ = sshx.Scan(ctx, c.Addr, 5*time.Second)
	cli2, err := sshx.Dial(ctx, sshx.Config{Addr: c.Addr, Pinned: hk, Auth: sshx.Auth{User: "root", PrivateKey: priv}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cli2.Close() }()

	if err := cli2.MkdirAll("/etc/hashi/traefik/certs", 0o700); err != nil {
		t.Fatal(err)
	}
	p := "/etc/hashi/traefik/certs/root.key"
	if wrote, err := cli2.WriteFileAtomic(p, []byte("k1"), 0o600); err != nil || !wrote {
		t.Fatalf("write = %v %v", wrote, err)
	}
	ino := c.Exec(t, "stat", "-c", "%i %a", p)
	if wrote, err := cli2.WriteFileAtomic(p, []byte("k1"), 0o600); err != nil || wrote {
		t.Fatalf("identical write = %v %v", wrote, err)
	}
	if again := c.Exec(t, "stat", "-c", "%i %a", p); again != ino || !strings.HasSuffix(strings.TrimSpace(ino), " 600") {
		t.Fatalf("stat %q → %q", ino, again)
	}
	if wrote, err := cli2.WriteFileAtomic(p, []byte("k2"), 0o600); err != nil || !wrote {
		t.Fatalf("changed write = %v %v", wrote, err)
	}
	if got := c.Exec(t, "sh", "-c", "cat "+p+"; ls -a /etc/hashi/traefik/certs | wc -l"); got != "k23\n" {
		t.Fatalf("after change: %q (want content k2 and only ., .., root.key)", got)
	}

	// Loopback services: Alpine's sshd refuses forwarding by default, so Fetch falls back to busybox wget; after
	// allowing forwarding the same call goes through the SSH connection.
	serve := func() {
		c.Exec(t, "sh", "-c", "(printf 'HTTP/1.0 200 OK\\r\\n\\r\\nOK' | nc -l -p 28080 -s 127.0.0.1 >/dev/null 2>&1 &)")
		time.Sleep(300 * time.Millisecond)
	}
	serve()
	if b, err := cli2.Fetch(ctx, 28080, "/ping"); err != nil || string(b) != "OK" {
		t.Fatalf("fetch via wget: %q %v", b, err)
	}
	c.Exec(t, "sh", "-c", "sed -i 's/^AllowTcpForwarding no/AllowTcpForwarding local/' /etc/ssh/sshd_config && kill -HUP 1")
	itest.WaitFor(t, 20*time.Second, "sshd restart", func() bool { return itest.SSHBanner(c.Addr) })
	cli3, err := sshx.Dial(ctx, sshx.Config{Addr: c.Addr, Pinned: hk, Auth: sshx.Auth{User: "root", PrivateKey: priv}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cli3.Close() }()
	serve()
	if _, err := cli3.DialLoopback(28080); err != nil {
		t.Fatalf("tunnel after allowing forwarding: %v", err)
	}
}
