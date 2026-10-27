package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/logring"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// syncBuffer is a goroutine-safe bytes.Buffer (Run writes to it while the test reads).
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// instance is one running Hashi process (in-process Run) with captured stdout and logs.
type instance struct {
	t       *testing.T
	addr    string
	dataDir string
	stdout  *syncBuffer
	logs    *syncBuffer
	stop    func()
}

func writeMasterKey(t *testing.T, dir string) string {
	t.Helper()
	k, err := vault.Keygen()
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "master.key")
	if err := os.WriteFile(p, []byte(k), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func start(t *testing.T, cfg Config) *instance {
	t.Helper()
	in := &instance{t: t, dataDir: cfg.DataDir, stdout: &syncBuffer{}, logs: &syncBuffer{}}
	cfg.Stdout = in.stdout
	cfg.Logs = logring.New(100)
	if cfg.AdminAddr == "" {
		cfg.AdminAddr = "127.0.0.1:0"
	}
	ctx, cancel := context.WithCancel(context.Background())
	addrCh := make(chan string, 1)
	done := make(chan error, 1)
	log := NewLogger(io.MultiWriter(in.logs, cfg.Logs), slog.LevelDebug)
	go func() { done <- Run(ctx, cfg, log, func(a string) { addrCh <- a }) }()
	select {
	case in.addr = <-addrCh:
	case err := <-done:
		cancel()
		t.Fatalf("Run exited early: %v", err)
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("not ready in time")
	}
	var once sync.Once
	in.stop = func() {
		once.Do(func() {
			cancel()
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("Run returned %v after cancel", err)
				}
			case <-time.After(10 * time.Second):
				t.Error("shutdown hung")
			}
		})
	}
	t.Cleanup(in.stop)
	return in
}

var bootstrapRe = regexp.MustCompile(`username: (\S+)\n\s+password: (\S+)`)

func (in *instance) bootstrapCredentials() (string, string) {
	m := bootstrapRe.FindStringSubmatch(in.stdout.String())
	if m == nil {
		in.t.Fatalf("no bootstrap block in stdout:\n%s", in.stdout.String())
	}
	return m[1], m[2]
}

// client is a browser stand-in: one cookie, the CSRF token, and optional forwarding headers.
type client struct {
	in     *instance
	cookie *http.Cookie
	csrf   string
	header http.Header
}

func (in *instance) client() *client { return &client{in: in, header: http.Header{}} }

type response struct {
	status int
	body   map[string]any
	raw    string
	header http.Header
}

func (c *client) do(method, path string, body any) response {
	c.in.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, "http://"+c.in.addr+path, rd)
	if err != nil {
		c.in.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range c.header {
		req.Header[k] = v
	}
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}
	if c.csrf != "" {
		req.Header.Set("X-Hashi-CSRF", c.csrf)
	}
	hc := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	res, err := hc.Do(req)
	if err != nil {
		c.in.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(res.Body)
	out := response{status: res.StatusCode, raw: string(raw), header: res.Header}
	_ = json.Unmarshal(raw, &out.body)
	for _, ck := range res.Cookies() {
		if strings.HasPrefix(ck.Name, "__Host-hashi") || ck.Name == "hashi.bootstrap" {
			if ck.MaxAge < 0 {
				c.cookie = nil
			} else {
				c.cookie = ck
			}
		}
	}
	if tok, ok := out.body["csrf_token"].(string); ok {
		c.csrf = tok
	}
	return out
}

func (c *client) mustStatus(r response, want int) response {
	c.in.t.Helper()
	if r.status != want {
		c.in.t.Fatalf("status %d, want %d: %s", r.status, want, r.raw)
	}
	return r
}

func (c *client) bootstrapLogin() response {
	c.in.t.Helper()
	u, p := c.in.bootstrapCredentials()
	return c.mustStatus(c.do(http.MethodPost, "/api/auth/bootstrap/login", map[string]string{"username": u, "password": p}), 200)
}
