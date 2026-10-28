// Package hetznertest serves the recorded Hetzner Cloud responses in testdata/contracts/hetzner over HTTP, so adapter
// and service tests run against the real API's shapes without touching the live API.
package hetznertest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// Token is the bearer token the replay server accepts; anything else gets the recorded 401.
const Token = "replay-token"

// Fixture is one recorded exchange.
type Fixture struct {
	Request string            `json:"request"`
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    json.RawMessage   `json:"body"`
}

// Dir is the fixture directory.
func Dir() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "testdata", "contracts", "hetzner")
}

// Load reads one fixture by its path below Dir (e.g. "zone_get.json", "write/get_a.json").
func Load(t testing.TB, name string) Fixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(Dir(), name)) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatal(err)
	}
	var f Fixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return f
}

// Server replays every fixture. A request matches a fixture on method, path and the page query parameter (per_page
// is ignored: pagination is driven by the recorded next_page). Unmatched requests get the recorded 404 body.
type Server struct {
	*httptest.Server
	mu       sync.Mutex
	byKey    map[string]Fixture
	requests []string
	// Override, when set, answers before the fixtures (rate-limit and error tests).
	Override func(w http.ResponseWriter, r *http.Request) bool
}

// NewServer starts a replay server; its URL + "/v1" is the adapter base URL.
func NewServer(t testing.TB) *Server {
	t.Helper()
	s := &Server{byKey: map[string]Fixture{}}
	entries, err := os.ReadDir(Dir())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		f := Load(t, e.Name())
		if f.Status == http.StatusUnauthorized {
			s.byKey["401"] = f
			continue
		}
		s.byKey[key(f.Request)] = f
	}
	s.Server = httptest.NewServer(http.HandlerFunc(s.serve))
	t.Cleanup(s.Close)
	return s
}

// BaseURL is the adapter base URL for this server.
func (s *Server) BaseURL() string { return s.URL + "/v1" }

// Requests returns the requests seen so far, e.g. "GET /v1/zones?page=1&per_page=25", in order.
func (s *Server) Requests() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.requests...)
}

func key(request string) string {
	method, target, _ := strings.Cut(request, " ")
	path, query, _ := strings.Cut(target, "?")
	page := "1"
	for _, kv := range strings.Split(query, "&") {
		if v, ok := strings.CutPrefix(kv, "page="); ok {
			page = v
		}
	}
	return method + " " + path + " page=" + page
}

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requests = append(s.requests, r.Method+" "+r.URL.RequestURI())
	override := s.Override
	s.mu.Unlock()
	if override != nil && override(w, r) {
		return
	}
	if r.Header.Get("Authorization") != "Bearer "+Token {
		write(w, s.byKey["401"])
		return
	}
	f, ok := s.byKey[key(r.Method+" "+r.URL.RequestURI())]
	if !ok {
		f = s.byKey[key("GET /v1/zones/999999999")]
	}
	write(w, f)
}

func write(w http.ResponseWriter, f Fixture) {
	for k, v := range f.Headers {
		if w.Header().Get(k) == "" { // an Override may have set it
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(f.Status)
	_, _ = w.Write(f.Body)
}
