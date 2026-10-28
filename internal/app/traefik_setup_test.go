package app

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/dns/hetzner"
	"git.juzo.io/juzo/hashi/internal/dns/hetzner/hetznertest"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

// Setup steps 4 and 5 over HTTP (02 §1.4–1.5) up to what needs a real CA or SSH host (those run against Pebble and
// sshd in the integration tests and on the dev targets at the gate).
func TestSetupSteps4And5API(t *testing.T) {
	_, srv := hetznertest.NewSimServer(t)
	cfg := newConfig(t)
	cfg.dnsProviders = func(_ string, token secrets.Value) (dns.Provider, error) {
		return hetzner.New(token, hetzner.WithBaseURL(srv.URL+"/v1")), nil
	}
	in := start(t, cfg)
	c := in.client()
	c.bootstrapLogin()
	const hmac = "c2VjcmV0LWVhYi1obWFjLWtleS1mb3ItdGVzdHM" // hashi:allow-secret-pattern
	ca := map[string]any{"directory": "custom", "directory_url": "https://127.0.0.1:1/dir", "email": "admin@example.com",
		"eab_kid": "kid-1", "eab_hmac": hmac}

	c.mustStatus(c.do(http.MethodPut, "/api/setup/certificates", ca), 422) // needs the DNS provider (step 3)
	c.mustStatus(c.do(http.MethodPut, "/api/setup/base-settings", map[string]any{"root_domain": "example.com",
		"admin_domain": "hashi.example.com", "internal_url": "http://192.0.2.10:3340", "sync_interval_seconds": 3600,
		"theme": "graphite", "public_dashboard_enabled": true, "public_status_enabled": true}), 200)
	c.mustStatus(c.do(http.MethodPut, "/api/setup/dns/provider", map[string]any{"type": "hetzner", "token": hetznertest.Token,
		"default_ttl": 3600}), 200)

	c.mustStatus(c.do(http.MethodPut, "/api/setup/certificates", map[string]any{"directory": "gts", "email": "admin@example.com"}), 422)
	c.mustStatus(c.do(http.MethodPut, "/api/setup/certificates", map[string]any{"directory": "letsencrypt", "email": "nope"}), 422)

	// The CA cannot be reached: the provider is stored, the step stays open with the reason.
	r := c.mustStatus(c.do(http.MethodPut, "/api/setup/certificates", ca), 200)
	if r.body["health"] != "failed" || r.body["account_registered"] != false || r.body["has_eab_hmac"] != true ||
		!strings.Contains(r.body["last_error"].(string), "register the ACME account") {
		t.Fatalf("unreachable CA: %s", r.raw)
	}
	r = c.mustStatus(c.do(http.MethodGet, "/api/setup", nil), 200)
	if s := r.body["steps"].([]any)[3].(map[string]any); s["status"] != "failed" || r.body["certificates"] == nil {
		t.Fatalf("step 4: %s", r.raw)
	}
	// Issuing runs as a plan; the unreachable CA fails the plan, not the request.
	if r := c.mustStatus(c.do(http.MethodPost, "/api/setup/certificates/issue", nil), 200); r.body["plan"].(map[string]any)["status"] != "failed" {
		t.Fatalf("issue: %s", r.raw)
	}

	// Step 5: Hashi's key, a scan of a closed port, input checks, nothing to confirm or plan yet.
	r = c.mustStatus(c.do(http.MethodPost, "/api/setup/traefik/identity", nil), 200)
	if !strings.HasPrefix(r.body["public_key"].(string), "ssh-ed25519 ") {
		t.Fatalf("identity: %s", r.raw)
	}
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	c.mustStatus(c.do(http.MethodPost, "/api/setup/traefik/scan", map[string]any{"host": "127.0.0.1", "port": port}), 502)
	c.mustStatus(c.do(http.MethodPut, "/api/setup/traefik/connection", map[string]any{"name": "edge", "host": "192.0.2.10",
		"port": 22, "user": "root", "auth": "hashi_key", "host_key": ""}), 422)
	c.mustStatus(c.do(http.MethodPost, "/api/setup/traefik/confirm", nil), 409)
	c.mustStatus(c.do(http.MethodPost, "/api/setup/traefik/plan", nil), 409)

	// The admin endpoints need a passkey session's scopes.
	for _, p := range []string{"/api/traefik/connections", "/api/certificates"} {
		c.mustStatus(c.do(http.MethodGet, p, nil), 403)
	}

	// Invariant 5: the EAB key is neither returned, logged nor stored in plain text.
	if strings.Contains(r.raw, hmac) || strings.Contains(in.logs.String(), hmac) {
		t.Fatal("EAB key returned or logged")
	}
	in.stop()
	dump := filepath.Join(t.TempDir(), "dump.db")
	st := storetest.Open(t, filepath.Join(in.dataDir, "hashi.db"))
	if err := st.BackupTo(context.Background(), dump); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dump) //nolint:gosec // test file
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), hmac) {
		t.Fatal("EAB key in the database")
	}
}
