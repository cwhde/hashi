package app

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/dns/hetzner"
	"git.juzo.io/juzo/hashi/internal/dns/hetzner/hetznertest"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

// The service API over HTTP with a setup session (services.manage, 02 §3.3): CRUD with 04 §1.5 errors as 422,
// enable/disable, the rendered preview; the system service is refused (invariant 10); deleting needs a reauth a
// bootstrap session cannot give; step 7 needs a Traefik connection first.
func TestServicesAPI(t *testing.T) {
	_, srv := hetznertest.NewSimServer(t)
	cfg := newConfig(t)
	cfg.dnsProviders = func(_ string, token secrets.Value) (dns.Provider, error) {
		return hetzner.New(token, hetzner.WithBaseURL(srv.URL+"/v1")), nil
	}
	in := start(t, cfg)
	c := in.client()
	c.bootstrapLogin()
	c.mustStatus(c.do(http.MethodPut, "/api/setup/base-settings", map[string]any{"root_domain": "example.com",
		"admin_domain": "dev.hashi.example.com", "internal_url": "http://192.0.2.10:3340", "sync_interval_seconds": 3600,
		"theme": "graphite", "public_dashboard_enabled": true, "public_status_enabled": true}), 200)
	c.mustStatus(c.do(http.MethodPut, "/api/setup/dns/provider", map[string]any{"type": "hetzner", "token": hetznertest.Token,
		"default_ttl": 3600}), 200)
	c.mustStatus(c.do(http.MethodPost, "/api/setup/system-service", nil), 409) // no Traefik connection yet

	search := map[string]any{"name": "Search", "slug": "search", "kind": "http", "domain_mode": "subdomain",
		"host_name": "search.example.com", "target": map[string]any{"address": "192.0.2.20", "port": 8080},
		"advanced_routes": true, "routes": []map[string]any{{"enabled": true, "path_match": "prefix", "path_value": "/api",
			"priority": 10, "rewrite_mode": "strip_prefix", "rewrite_pattern": "", "rewrite_value": "",
			"targets": []map[string]any{{"address": "192.0.2.21", "port": 9000}}}}}
	r := c.mustStatus(c.do(http.MethodPost, "/api/services", search), 200)
	svc := r.body["service"].(map[string]any)
	id := svc["id"].(string)
	if svc["origin"] != "user" || svc["enabled"] != true || r.body["certificate_pending"] != true || len(svc["routes"].([]any)) != 1 {
		t.Fatalf("create: %s", r.raw)
	}
	// No firewall host covers the upstreams: the service record is an A with their public addresses (03 §5.2),
	// applied through the normal DNS plan.
	plans := r.body["plans"].([]any)
	if len(plans) != 1 || plans[0].(map[string]any)["subsystem"] != "dns-records" || plans[0].(map[string]any)["status"] != "applied" ||
		!strings.Contains(r.raw, `"values":["192.0.2.20","192.0.2.21"]`) {
		t.Fatalf("create plans: %s", r.raw)
	}

	bad := func(mut func(m map[string]any)) map[string]any {
		m := map[string]any{}
		for k, v := range search {
			m[k] = v
		}
		mut(m)
		return m
	}
	c.mustStatus(c.do(http.MethodPost, "/api/services", bad(func(m map[string]any) { m["host_name"] = "x.example.net"; m["slug"] = "x" })), 422)
	c.mustStatus(c.do(http.MethodPost, "/api/services", bad(func(m map[string]any) { m["slug"] = "other" })), 409) // same host name
	c.mustStatus(c.do(http.MethodPost, "/api/services", bad(func(m map[string]any) {
		m["slug"], m["host_name"], m["advanced_routes"] = "off", "off.example.com", false
		m["routes"] = []map[string]any{{"enabled": true, "path_match": "prefix", "path_value": "/a", "priority": 5,
			"rewrite_mode": "none", "rewrite_pattern": "", "rewrite_value": "", "targets": []map[string]any{{"address": "192.0.2.22", "port": 80}}},
			{"enabled": true, "path_match": "prefix", "path_value": "/b", "priority": 5,
				"rewrite_mode": "none", "rewrite_pattern": "", "rewrite_value": "", "targets": []map[string]any{{"address": "192.0.2.22", "port": 80}}}}
	})), 422)

	r = c.mustStatus(c.do(http.MethodGet, "/api/services/"+id+"/effective", nil), 200)
	if y := r.body["yaml"].(string); !strings.Contains(y, "svc-search:") || !strings.Contains(y, "stripPrefix") {
		t.Errorf("effective: %s", y)
	}
	upd := bad(func(m map[string]any) { m["target"] = map[string]any{"address": "192.0.2.20", "port": 8081} })
	r = c.mustStatus(c.do(http.MethodPut, "/api/services/"+id, upd), 200)
	if r.body["service"].(map[string]any)["target"].(map[string]any)["port"] != float64(8081) {
		t.Errorf("update: %s", r.raw)
	}
	r = c.mustStatus(c.do(http.MethodPost, "/api/services/"+id+"/disable", nil), 200)
	if r.body["service"].(map[string]any)["enabled"] != false {
		t.Errorf("disable: %s", r.raw)
	}
	c.mustStatus(c.do(http.MethodPost, "/api/services/"+id+"/enable", nil), 200)
	if r := c.do(http.MethodDelete, "/api/services/"+id, nil); r.status == 200 {
		t.Errorf("delete without reauth: %s", r.raw)
	}
	c.mustStatus(c.do(http.MethodGet, "/api/services/"+id, nil), 200)
	c.mustStatus(c.do(http.MethodGet, "/api/services/nope", nil), 404)

	// The system service exists once step 7 ran; through the API it can be read but not changed (invariant 10).
	st := storetest.Open(t, cfg.DBPath())
	sys, _, err := services.EnsureSystem(context.Background(), st.Q, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	r = c.mustStatus(c.do(http.MethodGet, "/api/services", nil), 200)
	if list := r.body["services"].([]any); len(list) != 2 || list[0].(map[string]any)["origin"] != "system" {
		t.Errorf("list: %s", r.raw)
	}
	c.mustStatus(c.do(http.MethodPut, "/api/services/"+sys.ID, upd), 409)
	c.mustStatus(c.do(http.MethodPost, "/api/services/"+sys.ID+"/disable", nil), 409)
	r = c.mustStatus(c.do(http.MethodGet, "/api/setup", nil), 200)
	if r.body["system_service"].(map[string]any)["host_name"] != "dev.hashi.example.com" {
		t.Errorf("setup state: %s", r.raw)
	}
	// Planning every Traefik host is an admin action, not a setup one.
	c.mustStatus(c.do(http.MethodPost, "/api/traefik/plan", nil), 403)
}
