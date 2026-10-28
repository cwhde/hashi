package app

import (
	"context"
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

// Setup step 3 over HTTP against the recorded Hetzner responses (02 §1.3, 03 §4).
func TestSetupStep3DNSProviderAndImport(t *testing.T) {
	sim, srv := hetznertest.NewSimServer(t)
	cfg := newConfig(t)
	cfg.dnsProviders = func(_ string, token secrets.Value) (dns.Provider, error) {
		return hetzner.New(token, hetzner.WithBaseURL(srv.URL+"/v1")), nil
	}
	in := start(t, cfg)
	c := in.client()
	c.bootstrapLogin()
	provider := map[string]any{"type": "hetzner", "token": hetznertest.Token, "default_ttl": 3600}

	c.mustStatus(c.do(http.MethodPut, "/api/setup/dns/provider", provider), 409) // needs the root domain from step 2
	c.mustStatus(c.do(http.MethodPut, "/api/setup/base-settings", map[string]any{"root_domain": "example.com",
		"admin_domain": "hashi.example.com", "internal_url": "http://192.0.2.10:3340", "sync_interval_seconds": 3600,
		"theme": "graphite", "public_dashboard_enabled": true, "public_status_enabled": true}), 200)
	c.mustStatus(c.do(http.MethodGet, "/api/setup/dns/import", nil), 409) // no zone yet

	// A rejected token: actionable 422, step 3 failed, nothing stored (invariant 3).
	bad := map[string]any{"type": "hetzner", "token": "legacy-console-token", "default_ttl": 3600}
	r := c.mustStatus(c.do(http.MethodPut, "/api/setup/dns/provider", bad), 422)
	if !strings.Contains(r.raw, "Cloud project API token") || strings.Contains(r.raw, "legacy-console-token") {
		t.Fatalf("bad token: %s", r.raw)
	}
	r = c.mustStatus(c.do(http.MethodGet, "/api/setup", nil), 200)
	if s := r.body["steps"].([]any)[2].(map[string]any); s["status"] != "failed" || r.body["dns"] != nil {
		t.Fatalf("after failed validation: %s", r.raw)
	}

	r = c.mustStatus(c.do(http.MethodPut, "/api/setup/dns/provider", provider), 200)
	zone := r.body["zone"].(map[string]any)
	if zone["name"] != "example.com" || zone["rrset_count"].(float64) != 58 || len(r.body["zones"].([]any)) != 2 {
		t.Fatalf("connect: %s", r.raw)
	}
	r = c.mustStatus(c.do(http.MethodGet, "/api/setup", nil), 200)
	if s := r.body["steps"].([]any)[2].(map[string]any); s["status"] != "in_progress" || r.body["dns"].(map[string]any)["zone"] == nil {
		t.Fatalf("resume state: %s", r.raw)
	}

	r = c.mustStatus(c.do(http.MethodGet, "/api/setup/dns/import", nil), 200)
	rows := r.body["rows"].([]any)
	if len(rows) != 58 {
		t.Fatalf("%d import rows", len(rows))
	}
	for _, row := range rows {
		m := row.(map[string]any)
		rr := m["rrset"].(map[string]any)
		if dns.Protected(rr["type"].(string)) && len(m["allowed_decisions"].([]any)) != 1 {
			t.Errorf("%v %v selectable: %v", rr["name"], rr["type"], m["allowed_decisions"])
		}
	}

	nsPrune := map[string]any{"decisions": []map[string]string{{"name": "@", "type": "NS", "decision": "prune"}}}
	c.mustStatus(c.do(http.MethodPut, "/api/setup/dns/import", nsPrune), 422) // invariant 2
	ok := map[string]any{"decisions": []map[string]string{
		{"name": "svc1", "type": "A", "decision": "import"}, {"name": "svc2", "type": "CNAME", "decision": "prune"},
	}}
	r = c.mustStatus(c.do(http.MethodPut, "/api/setup/dns/import", ok), 200)
	if s := r.body["steps"].([]any)[2].(map[string]any); s["status"] != "done" {
		t.Fatalf("step 3: %s", r.raw)
	}
	r = c.mustStatus(c.do(http.MethodGet, "/api/setup/dns/import", nil), 200)
	decided := 0
	for _, row := range r.body["rows"].([]any) {
		if row.(map[string]any)["decision"] != nil {
			decided++
		}
	}
	if decided != 2 {
		t.Errorf("%d decided rows after save, want 2", decided)
	}

	// The DNS admin API needs a passkey session's scopes; the bootstrap session has only setup.manage.
	c.mustStatus(c.do(http.MethodGet, "/api/dns/zones", nil), 403)
	c.mustStatus(c.do(http.MethodGet, "/api/sync/plans", nil), 403)

	// The write test (invariant 4): planning it writes nothing; running the shown plan creates and deletes the probe.
	r = c.mustStatus(c.do(http.MethodPost, "/api/setup/dns/probe", nil), 200)
	if r.body["status"] != "pending" || r.body["risk"] != "high" || len(r.body["items"].([]any)) != 2 || len(sim.Writes) != 0 {
		t.Fatalf("probe plan: %s, writes %v", r.raw, sim.Writes)
	}
	c.mustStatus(c.do(http.MethodPost, "/api/sync/plans/"+r.body["id"].(string)+"/approve", nil), 403) // needs sync.apply + reauth
	r = c.mustStatus(c.do(http.MethodPost, "/api/setup/dns/probe/"+r.body["id"].(string)+"/run", nil), 200)
	if r.body["status"] != "applied" || !strings.Contains(r.body["result"].(string), "write access confirmed") || len(sim.Writes) != 2 {
		t.Fatalf("probe run: %s, writes %v", r.raw, sim.Writes)
	}
	if _, _, _, ok := sim.RRSet("100001", dns.ProbeName, "TXT"); ok {
		t.Fatal("probe left behind")
	}

	// Invariant 5: the token is neither logged nor stored in plain text.
	if strings.Contains(in.logs.String(), hetznertest.Token) {
		t.Fatal("token in the log")
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
	if strings.Contains(string(b), hetznertest.Token) {
		t.Fatal("token in the database")
	}
}
