// Package sshxtest runs an in-process SSH server for tests: password or key login, `exec` through a caller-supplied
// handler, the SFTP subsystem on the real file system and `direct-tcpip` tunnels. The real-sshd check is the
// integration test in package sshx.
package sshxtest

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// Exec answers one `exec` request: it gets the command line and stdin and returns output and exit code.
type Exec func(cmd string, stdin io.Reader, stdout, stderr io.Writer) int

// Server is a running test server.
type Server struct {
	Addr    string
	HostKey ssh.PublicKey
	User    string
	// Password and AuthorizedKey are the accepted credentials (either may be empty).
	Password      string
	AuthorizedKey ssh.PublicKey
	// Tunnels maps a requested "host:port" to the local address actually dialled; unknown targets are refused.
	Tunnels map[string]string
	// NoForwarding refuses every tunnel like sshd with `AllowTcpForwarding no`.
	NoForwarding bool

	mu       sync.Mutex
	exec     Exec
	Commands []string
}

// Start listens on 127.0.0.1 and serves until the test ends.
func Start(t testing.TB, user, password string, authorized ssh.PublicKey, exec Exec) *Server {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	var lc net.ListenConfig
	l, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Addr: l.Addr().String(), HostKey: signer.PublicKey(), User: user, Password: password,
		AuthorizedKey: authorized, Tunnels: map[string]string{}, exec: exec}
	cfg := &ssh.ServerConfig{
		PasswordCallback: func(m ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			if m.User() == s.User && s.Password != "" && string(pw) == s.Password {
				return nil, nil
			}
			return nil, errDenied
		},
		PublicKeyCallback: func(m ssh.ConnMetadata, k ssh.PublicKey) (*ssh.Permissions, error) {
			if m.User() == s.User && s.AuthorizedKey != nil && string(k.Marshal()) == string(s.AuthorizedKey.Marshal()) {
				return nil, nil
			}
			return nil, errDenied
		},
	}
	cfg.AddHostKey(signer)
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go s.serve(c, cfg)
		}
	}()
	return s
}

type denied struct{}

func (denied) Error() string { return "denied" }

var errDenied = denied{}

func (s *Server) serve(c net.Conn, cfg *ssh.ServerConfig) {
	defer func() { _ = c.Close() }()
	_, chans, reqs, err := ssh.NewServerConn(c, cfg)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		switch nc.ChannelType() {
		case "session":
			ch, creqs, err := nc.Accept()
			if err != nil {
				continue
			}
			go s.session(ch, creqs)
		case "direct-tcpip":
			var p struct {
				Host       string
				Port       uint32
				OriginHost string
				OriginPort uint32
			}
			if err := ssh.Unmarshal(nc.ExtraData(), &p); err != nil {
				_ = nc.Reject(ssh.ConnectionFailed, "bad request")
				continue
			}
			if s.NoForwarding {
				_ = nc.Reject(ssh.Prohibited, "open failed")
				continue
			}
			target, ok := s.Tunnels[net.JoinHostPort(p.Host, strconv.FormatUint(uint64(p.Port), 10))]
			if !ok {
				_ = nc.Reject(ssh.ConnectionFailed, "no such tunnel")
				continue
			}
			var d net.Dialer
			up, err := d.DialContext(context.Background(), "tcp", target)
			if err != nil {
				_ = nc.Reject(ssh.ConnectionFailed, err.Error())
				continue
			}
			ch, creqs, err := nc.Accept()
			if err != nil {
				_ = up.Close()
				continue
			}
			go ssh.DiscardRequests(creqs)
			go func() { _, _ = io.Copy(ch, up); _ = ch.Close() }()
			go func() { _, _ = io.Copy(up, ch); _ = up.Close() }()
		default:
			_ = nc.Reject(ssh.UnknownChannelType, "unsupported")
		}
	}
}

func (s *Server) session(ch ssh.Channel, reqs <-chan *ssh.Request) {
	defer func() { _ = ch.Close() }()
	for r := range reqs {
		switch r.Type {
		case "exec":
			if len(r.Payload) < 4 {
				_ = r.Reply(false, nil)
				return
			}
			cmd := string(r.Payload[4:])
			_ = r.Reply(true, nil)
			s.mu.Lock()
			s.Commands = append(s.Commands, cmd)
			s.mu.Unlock()
			code := 127
			if s.exec != nil {
				code = s.exec(cmd, ch, ch, ch.Stderr())
			}
			status := make([]byte, 4)
			binary.BigEndian.PutUint32(status, uint32(code))
			_, _ = ch.SendRequest("exit-status", false, status)
			return
		case "subsystem":
			if len(r.Payload) < 4 || string(r.Payload[4:]) != "sftp" {
				_ = r.Reply(false, nil)
				return
			}
			_ = r.Reply(true, nil)
			srv, err := sftp.NewServer(ch)
			if err != nil {
				return
			}
			_ = srv.Serve()
			return
		default:
			_ = r.Reply(false, nil)
		}
	}
}

// Ran returns the commands executed so far.
func (s *Server) Ran() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.Commands...)
}
