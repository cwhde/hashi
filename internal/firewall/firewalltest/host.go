// Package firewalltest is an in-memory firewall host for tests, `make dev` and the E2E suite: it answers the probe from
// a captured fixture (testdata/contracts/firewall), keeps written files, "loads" the table when nft runs it, and
// reports NetBird as connected. Nothing here touches a real host.
package firewalltest

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"git.juzo.io/juzo/hashi/internal/firewall"
	"git.juzo.io/juzo/hashi/internal/sshx"
)

// Host is one fake firewall host; every address reaches it.
type Host struct {
	mu     sync.Mutex
	probe  string
	key    sshx.HostKey
	files  map[string][]byte
	loaded []byte // the table nft last loaded
	ran    []string
}

// NewHost returns a host whose probe prints probe (a testdata/contracts/firewall fixture).
func NewHost(probe string) (*Host, error) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	sp, err := ssh.NewPublicKey(pub)
	if err != nil {
		return nil, err
	}
	hk, err := sshx.ParseHostKey(sp.Marshal())
	if err != nil {
		return nil, err
	}
	return &Host{probe: probe, key: hk, files: map[string][]byte{}}, nil
}

// Scan returns the host key, like sshx.Scan.
func (h *Host) Scan(context.Context, string, time.Duration) (sshx.HostKey, error) { return h.key, nil }

// Key is the host key (tests pin it directly).
func (h *Host) Key() sshx.HostKey { return h.key }

// Connect logs in when the pinned key is this host's, like sshx.Dial.
func (h *Host) Connect(_ context.Context, c sshx.Config) (firewall.Conn, error) {
	if c.Pinned.Fingerprint != h.key.Fingerprint {
		return nil, sshx.ErrHostKeyMismatch
	}
	return conn{h}, nil
}

// Ran returns the commands run so far.
func (h *Host) Ran() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.ran...)
}

// Loaded is the table the host holds now (nil before the first apply).
func (h *Host) Loaded() []byte {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.loaded
}

type conn struct{ h *Host }

func (conn) Close() error { return nil }

func (c conn) Run(_ context.Context, cmd string, _ io.Reader) (sshx.Result, error) {
	h := c.h
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ran = append(h.ran, cmd)
	ok := func(out string) (sshx.Result, error) { return sshx.Result{Stdout: []byte(out)}, nil }
	switch {
	case cmd == firewall.ProbeCommand:
		return ok(h.probe)
	case cmd == `echo "$SSH_CONNECTION"`:
		return ok("10.32.0.6 50122 198.18.3.1 22\n")
	case cmd == firewall.SetupRulesetCommand:
		return ok("### nft -j list ruleset\n{\"nftables\": [{\"metainfo\": {\"json_schema_version\": 1}}]}\n")
	case cmd == "netbird status --json":
		return ok(`{"daemonVersion":"0.81.3","netbirdIp":"10.31.0.1/16","usesKernelInterface":true,"wireguardPort":51820,` +
			`"management":{"connected":true},"signal":{"connected":true}}`)
	case cmd == "nft list tables":
		if h.loaded == nil {
			return ok("")
		}
		return ok("table inet " + firewall.TableName + "\n")
	case strings.HasPrefix(cmd, "systemctl is-enabled "+firewall.UnitName):
		if _, ok := h.files[firewall.UnitFile]; !ok {
			return sshx.Result{Stdout: []byte("disabled\n"), ExitCode: 1}, nil
		}
		return ok("enabled\n")
	case strings.HasSuffix(cmd, " -j list table inet "+firewall.TableName):
		if h.loaded == nil {
			return sshx.Result{ExitCode: 1, Stderr: []byte("Error: No such file or directory")}, nil
		}
		return sshx.Result{Stdout: h.loaded}, nil // a rendered batch reads like a listing for Verify
	case strings.HasSuffix(cmd, " -j -f "+firewall.TableFile):
		h.loaded = h.files[firewall.TableFile]
		return ok("")
	}
	return ok("") // sysctl, systemctl, systemd-run: succeed
}

func (c conn) ReadFile(p string) ([]byte, error) {
	c.h.mu.Lock()
	defer c.h.mu.Unlock()
	b, ok := c.h.files[p]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return b, nil
}

func (c conn) WriteFileAtomic(p string, data []byte, _ fs.FileMode) (bool, error) {
	if p == "" {
		return false, errors.New("empty path")
	}
	c.h.mu.Lock()
	defer c.h.mu.Unlock()
	c.h.files[p] = append([]byte(nil), data...)
	return true, nil
}

func (conn) MkdirAll(string, fs.FileMode) error { return nil }
