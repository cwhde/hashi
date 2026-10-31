// Package traefiktest is an in-memory Traefik host for `make dev` and the E2E suite (dev builds only): it answers the
// OS probe from a captured fixture, keeps written files, "starts" Traefik on the service commands, reports every
// router, middleware and service of its dynamic files as enabled, and serves the distributed certificates over TLS.
package traefiktest

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/traefik"
)

// Edge is one fake host. Every address reaches the same host.
type Edge struct {
	mu    sync.Mutex
	probe string
	key   sshx.HostKey
	files map[string][]byte
	up    bool
}

// NewEdge returns a host whose OS probe prints probe (a testdata/contracts/os fixture).
func NewEdge(probe string) (*Edge, error) {
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
	return &Edge{probe: probe, key: hk, files: map[string][]byte{}}, nil
}

// Scan returns the host key, like sshx.Scan.
func (e *Edge) Scan(context.Context, string, time.Duration) (sshx.HostKey, error) { return e.key, nil }

// Connect logs in when the pinned key is this host's, like sshx.Dial.
func (e *Edge) Connect(_ context.Context, c sshx.Config) (traefik.Conn, error) {
	if c.Pinned.Fingerprint != e.key.Fingerprint {
		return nil, sshx.ErrHostKeyMismatch
	}
	return conn{e}, nil
}

// Binary is the "release download".
func (e *Edge) Binary(context.Context, string) ([]byte, error) {
	return []byte("traefik " + traefik.Version), nil
}

// DialTLS connects to the host's websecure port: it presents whichever distributed certificate matches the SNI.
func (e *Edge) DialTLS(ctx context.Context, _ string) (net.Conn, error) {
	e.mu.Lock()
	var certs []tls.Certificate
	for p, crt := range e.files {
		if !strings.HasSuffix(p, ".crt") || !strings.Contains(p, "/certs/") {
			continue
		}
		if pair, err := tls.X509KeyPair(crt, e.files[strings.TrimSuffix(p, ".crt")+".key"]); err == nil {
			certs = append(certs, pair)
		}
	}
	up := e.up
	e.mu.Unlock()
	if !up || len(certs) == 0 {
		return nil, errors.New("connection refused")
	}
	client, server := net.Pipe()
	go func() {
		s := tls.Server(server, &tls.Config{Certificates: certs, MinVersion: tls.VersionTLS12})
		_ = s.HandshakeContext(context.WithoutCancel(ctx))
		_ = s.Close()
	}()
	return client, nil
}

type conn struct{ e *Edge }

func (conn) Close() error { return nil }

func (c conn) Run(_ context.Context, cmd string, _ io.Reader) (sshx.Result, error) {
	e := c.e
	e.mu.Lock()
	defer e.mu.Unlock()
	switch {
	case cmd == traefik.ProbeCommand:
		return sshx.Result{Stdout: []byte(e.probe)}, nil
	case strings.HasSuffix(cmd, " version"):
		if _, ok := e.files[strings.Trim(strings.TrimSuffix(cmd, " version"), "'")]; !ok {
			return sshx.Result{ExitCode: 127, Stderr: []byte("not found")}, nil
		}
		return sshx.Result{Stdout: []byte("Version:      " + traefik.Version + "\n")}, nil
	case strings.HasPrefix(cmd, "test -d "):
		return sshx.Result{}, nil // directories are created on demand
	case strings.Contains(cmd, "systemctl") || strings.Contains(cmd, "rc-service"):
		e.up = true
	case strings.HasPrefix(cmd, "cp -p "):
		f := strings.Fields(strings.TrimPrefix(cmd, "cp -p "))
		if b, ok := e.files[strings.Trim(f[0], "'")]; ok {
			e.files[strings.Trim(f[1], "'")] = append([]byte(nil), b...)
		}
	}
	return sshx.Result{}, nil
}

func (c conn) ReadFile(p string) ([]byte, error) {
	c.e.mu.Lock()
	defer c.e.mu.Unlock()
	b, ok := c.e.files[p]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), b...), nil
}

func (c conn) WriteFileAtomic(p string, data []byte, _ fs.FileMode) (bool, error) {
	c.e.mu.Lock()
	defer c.e.mu.Unlock()
	if old, ok := c.e.files[p]; ok && string(old) == string(data) {
		return false, nil
	}
	c.e.files[p] = append([]byte(nil), data...)
	return true, nil
}

func (conn) MkdirAll(string, fs.FileMode) error { return nil }

func (c conn) Fetch(_ context.Context, _ int, path string) ([]byte, error) {
	e := c.e
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.up {
		return nil, errors.New("connection refused")
	}
	if path == "/ping" {
		return []byte("OK"), nil
	}
	if path != "/api/rawdata" {
		return nil, fmt.Errorf("GET %s: HTTP 404", path)
	}
	var files []traefik.File
	for p, b := range e.files {
		if strings.Contains(p, "/dynamic/") {
			files = append(files, traefik.File{Path: p, Kind: traefik.KindDynamic, Data: b})
		}
	}
	names, err := traefik.Expected(files)
	if err != nil {
		return nil, err
	}
	type obj struct {
		Status string `json:"status"`
	}
	// Expected's kinds as /api/rawdata keys (testdata/contracts/traefik/streams for the tcp/udp ones).
	keys := map[string]string{"router": "routers", "middleware": "middlewares", "service": "services",
		"tcp-router": "tcpRouters", "tcp-service": "tcpServices", "udp-router": "udpRouters", "udp-service": "udpServices"}
	rd := map[string]map[string]obj{}
	for _, n := range names {
		kind, name, _ := strings.Cut(n, " ")
		if rd[keys[kind]] == nil {
			rd[keys[kind]] = map[string]obj{}
		}
		rd[keys[kind]][name] = obj{Status: "enabled"}
	}
	return json.Marshal(rd)
}
