// Package sshx is Hashi's SSH/SFTP client for Traefik and firewall hosts (01-platform.md §2): every connection pins the
// host key, files are written atomically and only when their content changed (invariant 8), and local services on the
// host (Traefik's API and ping) are reached through the SSH connection instead of tools installed on the host.
package sshx

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"git.juzo.io/juzo/hashi/internal/secrets"
)

// MaxOutput caps what Run keeps of stdout and stderr each; the rest is discarded and Result.Truncated is set.
const MaxOutput = 1 << 20

// ErrHostKeyMismatch means the host presented a key other than the pinned one. Hashi never reconnects on its own after
// this: the user has to look at the new fingerprint and pin it again.
var ErrHostKeyMismatch = errors.New("ssh host key does not match the pinned key")

// HostKey is a server's public host key in SSH wire format plus its OpenSSH SHA256 fingerprint ("SHA256:…").
type HostKey struct {
	Type        string
	Key         []byte
	Fingerprint string
}

func hostKeyOf(k ssh.PublicKey) HostKey {
	return HostKey{Type: k.Type(), Key: k.Marshal(), Fingerprint: ssh.FingerprintSHA256(k)}
}

// ParseHostKey restores a pinned key stored as wire bytes.
func ParseHostKey(wire []byte) (HostKey, error) {
	k, err := ssh.ParsePublicKey(wire)
	if err != nil {
		return HostKey{}, fmt.Errorf("parse pinned host key: %w", err)
	}
	return hostKeyOf(k), nil
}

// Auth is how Hashi logs in: a private key (OpenSSH or PEM, optional passphrase) or a password. A key wins when both are set.
type Auth struct {
	User       string
	PrivateKey secrets.Value
	Passphrase secrets.Value
	Password   secrets.Value
}

func (a Auth) methods() ([]ssh.AuthMethod, error) {
	var out []ssh.AuthMethod
	if !a.PrivateKey.IsZero() {
		var (
			signer ssh.Signer
			err    error
		)
		if a.Passphrase.IsZero() {
			signer, err = ssh.ParsePrivateKey([]byte(a.PrivateKey.Reveal()))
		} else {
			signer, err = ssh.ParsePrivateKeyWithPassphrase([]byte(a.PrivateKey.Reveal()), []byte(a.Passphrase.Reveal()))
		}
		if err != nil {
			return nil, fmt.Errorf("parse ssh private key: %w", err)
		}
		out = append(out, ssh.PublicKeys(signer))
	}
	if !a.Password.IsZero() {
		pw := a.Password.Reveal()
		out = append(out, ssh.Password(pw), ssh.KeyboardInteractive(func(_, _ string, qs []string, _ []bool) ([]string, error) {
			answers := make([]string, len(qs))
			for i := range answers {
				answers[i] = pw
			}
			return answers, nil
		}))
	}
	if len(out) == 0 {
		return nil, errors.New("no ssh credentials: set a private key or a password")
	}
	return out, nil
}

// Addr joins host and port, bracketing IPv6 literals.
func Addr(host string, port int) string { return net.JoinHostPort(host, strconv.Itoa(port)) }

// Scan connects without authenticating and returns the host key the server presents, for the user to confirm
// (02-setup-auth.md §1.5). Nothing is trusted by scanning.
func Scan(ctx context.Context, addr string, timeout time.Duration) (HostKey, error) {
	var got *HostKey
	errStop := errors.New("host key captured")
	cfg := &ssh.ClientConfig{
		User: "hashi-scan",
		HostKeyCallback: func(_ string, _ net.Addr, k ssh.PublicKey) error {
			hk := hostKeyOf(k)
			got = &hk
			return errStop
		},
		Timeout: timeout,
	}
	conn, err := dialTCP(ctx, addr, timeout)
	if err != nil {
		return HostKey{}, err
	}
	defer func() { _ = conn.Close() }()
	_, _, _, err = ssh.NewClientConn(conn, addr, cfg)
	if got != nil {
		return *got, nil
	}
	return HostKey{}, fmt.Errorf("ssh handshake with %s: %w", addr, err)
}

func dialTCP(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", addr, err)
	}
	if timeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(timeout))
	}
	return conn, nil
}

// Config is one connection's target. Pinned is required: there is no "accept any key" mode.
type Config struct {
	Addr    string
	Auth    Auth
	Pinned  HostKey
	Timeout time.Duration // handshake timeout; default 15 s
}

// Client is an authenticated SSH connection with a lazily opened SFTP session.
type Client struct {
	ssh      *ssh.Client
	sftp     *sftp.Client
	noTunnel bool // sshd refused a forwarding request once; Fetch uses curl/wget from then on
}

// Dial connects, checks the host key against the pin and logs in.
func Dial(ctx context.Context, c Config) (*Client, error) {
	if len(c.Pinned.Key) == 0 {
		return nil, errors.New("ssh host key is not pinned")
	}
	pinned, err := ssh.ParsePublicKey(c.Pinned.Key)
	if err != nil {
		return nil, fmt.Errorf("parse pinned host key: %w", err)
	}
	methods, err := c.Auth.methods()
	if err != nil {
		return nil, err
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 15 * time.Second
	}
	cfg := &ssh.ClientConfig{
		User: c.Auth.User,
		Auth: methods,
		HostKeyCallback: func(_ string, _ net.Addr, k ssh.PublicKey) error {
			if !bytes.Equal(k.Marshal(), pinned.Marshal()) {
				return fmt.Errorf("%w (presented %s, pinned %s)", ErrHostKeyMismatch, ssh.FingerprintSHA256(k), ssh.FingerprintSHA256(pinned))
			}
			return nil
		},
		HostKeyAlgorithms: []string{pinned.Type()},
		Timeout:           timeout,
	}
	conn, err := dialTCP(ctx, c.Addr, timeout)
	if err != nil {
		return nil, err
	}
	sc, chans, reqs, err := ssh.NewClientConn(conn, c.Addr, cfg)
	if err != nil {
		_ = conn.Close()
		if errors.Is(err, ErrHostKeyMismatch) {
			return nil, err
		}
		return nil, fmt.Errorf("ssh login to %s: %w", c.Addr, err)
	}
	_ = conn.SetDeadline(time.Time{})
	return &Client{ssh: ssh.NewClient(sc, chans, reqs)}, nil
}

// Close ends the SFTP session and the connection.
func (c *Client) Close() error {
	if c.sftp != nil {
		_ = c.sftp.Close()
	}
	return c.ssh.Close()
}

// Result is one command's outcome. A non-zero exit is a Result, not an error.
type Result struct {
	Stdout, Stderr []byte
	ExitCode       int
	Truncated      bool
}

// OK reports exit code 0.
func (r Result) OK() bool { return r.ExitCode == 0 }

type capped struct {
	buf       bytes.Buffer
	truncated bool
}

func (w *capped) Write(p []byte) (int, error) {
	if room := MaxOutput - w.buf.Len(); room < len(p) {
		w.truncated = true
		if room > 0 {
			w.buf.Write(p[:room])
		}
		return len(p), nil
	}
	return w.buf.Write(p)
}

// Run executes one command. Callers pass fixed commands; anything variable must be quoted with Quote.
func (c *Client) Run(ctx context.Context, cmd string, stdin io.Reader) (Result, error) {
	s, err := c.ssh.NewSession()
	if err != nil {
		return Result{}, fmt.Errorf("open ssh session: %w", err)
	}
	defer func() { _ = s.Close() }()
	var out, errOut capped
	s.Stdout, s.Stderr, s.Stdin = &out, &errOut, stdin
	done := make(chan error, 1)
	go func() { done <- s.Run(cmd) }()
	select {
	case <-ctx.Done():
		_ = s.Signal(ssh.SIGKILL)
		_ = s.Close()
		return Result{}, ctx.Err()
	case err = <-done:
	}
	r := Result{Stdout: out.buf.Bytes(), Stderr: errOut.buf.Bytes(), Truncated: out.truncated || errOut.truncated}
	var exit *ssh.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		r.ExitCode = exit.ExitStatus()
	default:
		return r, fmt.Errorf("run remote command: %w", err)
	}
	return r, nil
}

// Quote makes s one POSIX shell word.
func Quote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func (c *Client) sftpClient() (*sftp.Client, error) {
	if c.sftp == nil {
		s, err := sftp.NewClient(c.ssh)
		if err != nil {
			return nil, fmt.Errorf("open sftp session: %w", err)
		}
		c.sftp = s
	}
	return c.sftp, nil
}

// ReadFile reads a remote file; a missing file is fs.ErrNotExist.
func (c *Client) ReadFile(p string) ([]byte, error) {
	s, err := c.sftpClient()
	if err != nil {
		return nil, err
	}
	f, err := s.Open(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read %s: %w", p, fs.ErrNotExist)
		}
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", p, err)
	}
	return b, nil
}

// Hash is the hex SHA-256 used for file comparisons and last_applied_hash.
func Hash(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// FileHash returns the remote file's Hash, or "" when it does not exist.
func (c *Client) FileHash(p string) (string, error) {
	b, err := c.ReadFile(p)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return Hash(b), nil
}

// WriteFileAtomic writes data to p only when the remote content differs (invariant 8): unique temp file in the same
// directory → chmod → posix rename over p. It reports whether it wrote. The directory must exist.
func (c *Client) WriteFileAtomic(p string, data []byte, mode fs.FileMode) (bool, error) {
	s, err := c.sftpClient()
	if err != nil {
		return false, err
	}
	if cur, err := c.FileHash(p); err != nil {
		return false, err
	} else if cur == Hash(data) {
		if st, err := s.Stat(p); err == nil && st.Mode().Perm() != mode.Perm() {
			if err := s.Chmod(p, mode); err != nil {
				return false, fmt.Errorf("chmod %s: %w", p, err)
			}
		}
		return false, nil
	}
	var rnd [8]byte
	if _, err := rand.Read(rnd[:]); err != nil {
		return false, err
	}
	tmp := path.Join(path.Dir(p), "."+path.Base(p)+".hashi-"+hex.EncodeToString(rnd[:]))
	f, err := s.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return false, fmt.Errorf("create %s: %w", tmp, err)
	}
	cleanup := func() { _ = s.Remove(tmp) }
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		cleanup()
		return false, fmt.Errorf("chmod %s: %w", tmp, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		cleanup()
		return false, fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := f.Close(); err != nil {
		cleanup()
		return false, fmt.Errorf("close %s: %w", tmp, err)
	}
	if err := s.PosixRename(tmp, p); err != nil {
		cleanup()
		return false, fmt.Errorf("rename %s over %s: %w", tmp, p, err)
	}
	return true, nil
}

// MkdirAll creates a remote directory and its parents, then sets mode on the leaf.
func (c *Client) MkdirAll(p string, mode fs.FileMode) error {
	s, err := c.sftpClient()
	if err != nil {
		return err
	}
	if err := s.MkdirAll(p); err != nil {
		return fmt.Errorf("mkdir %s: %w", p, err)
	}
	if err := s.Chmod(p, mode|fs.ModeDir); err != nil {
		return fmt.Errorf("chmod %s: %w", p, err)
	}
	return nil
}

// DialLoopback opens a TCP connection to 127.0.0.1:port on the remote host through the SSH connection.
func (c *Client) DialLoopback(port int) (net.Conn, error) {
	conn, err := c.ssh.Dial("tcp", Addr("127.0.0.1", port))
	if err != nil {
		return nil, fmt.Errorf("tunnel to remote 127.0.0.1:%d: %w", port, err)
	}
	return conn, nil
}

// HTTPClient talks HTTP to the remote host's loopback port through the SSH connection; request URLs use any host.
func (c *Client) HTTPClient(port int, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:       func(context.Context, string, string) (net.Conn, error) { return c.DialLoopback(port) },
			DisableKeepAlives: true,
		},
	}
}

// ErrNoFetch means Hashi cannot reach the host's loopback services: TCP forwarding is off in sshd (Alpine's default)
// and neither curl nor wget is installed.
var ErrNoFetch = errors.New("cannot reach local services on the host: allow TCP forwarding in sshd or install curl or wget")

// Fetch GETs http://127.0.0.1:<port><path> on the host and returns the body of a 2xx answer: through the SSH connection
// when sshd allows TCP forwarding, otherwise with the host's curl or wget (captured 2026-10-28: Alpine's sshd ships
// with `AllowTcpForwarding no`; busybox wget is always there; minimal Debian has neither tool).
func (c *Client) Fetch(ctx context.Context, port int, urlPath string) ([]byte, error) {
	url := "http://" + Addr("127.0.0.1", port) + urlPath
	if !c.noTunnel {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := c.HTTPClient(port, 30*time.Second).Do(req)
		if err == nil {
			defer func() { _ = resp.Body.Close() }()
			b, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", url, err)
			}
			if resp.StatusCode/100 != 2 {
				return b, fmt.Errorf("GET %s: HTTP %d", url, resp.StatusCode)
			}
			return b, nil
		}
		if !strings.Contains(err.Error(), "administratively prohibited") {
			return nil, err
		}
		c.noTunnel = true
	}
	q := Quote(url)
	r, err := c.Run(ctx, "if command -v curl >/dev/null 2>&1; then curl -fsS --max-time 30 "+q+
		"; elif command -v wget >/dev/null 2>&1; then wget -q -T 30 -O - "+q+"; else exit 127; fi", nil)
	if err != nil {
		return nil, err
	}
	switch {
	case r.ExitCode == 127:
		return nil, ErrNoFetch
	case !r.OK():
		return nil, fmt.Errorf("GET %s on the host failed (exit %d): %s", url, r.ExitCode, strings.TrimSpace(string(r.Stderr)))
	}
	return r.Stdout, nil
}

// GenerateKey creates Hashi's own ed25519 login key: the private half as an OpenSSH PEM (for the vault) and the public
// half as an authorized_keys line for the user to install.
func GenerateKey(comment string) (secrets.Value, string, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return secrets.Value{}, "", fmt.Errorf("generate ssh key: %w", err)
	}
	block, err := ssh.MarshalPrivateKey(priv, comment)
	if err != nil {
		return secrets.Value{}, "", fmt.Errorf("marshal ssh key: %w", err)
	}
	sp, err := ssh.NewPublicKey(pub)
	if err != nil {
		return secrets.Value{}, "", fmt.Errorf("ssh public key: %w", err)
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sp)))
	if comment != "" {
		line += " " + comment
	}
	return secrets.New(string(pem.EncodeToMemory(block))), line, nil
}

// PublicLine returns the authorized_keys line (comment "hashi") of a private key made by GenerateKey.
func PublicLine(priv secrets.Value) (string, error) {
	raw, err := ssh.ParseRawPrivateKey([]byte(priv.Reveal()))
	if err != nil {
		return "", fmt.Errorf("parse ssh key: %w", err)
	}
	signer, err := ssh.NewSignerFromKey(raw)
	if err != nil {
		return "", fmt.Errorf("ssh key: %w", err)
	}
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))) + " hashi", nil
}
