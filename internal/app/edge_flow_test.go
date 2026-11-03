package app

import (
	"database/sql"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/dns/hetzner"
	"git.juzo.io/juzo/hashi/internal/dns/hetzner/hetznertest"
	"git.juzo.io/juzo/hashi/internal/secrets"
)

// Forward auth over HTTP (06 §2, §4, §17): a shared rule set decides on two services, the fallback closes a service, a
// path behaves like its normalized form, the trace names the deciding rule, and an unknown host passes (health routers).
func TestForwardAuthRules(t *testing.T) {
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
	c.mustStatus(c.do(http.MethodPut, "/api/setup/dns/provider", map[string]any{"type": "hetzner", "token": hetznertest.Token, "default_ttl": 3600}), 200)
	// Rule sets need security.manage, which a setup session doesn't hold; widening this test's session is simpler than the
	// whole setup path the others walk.
	raw, err := sql.Open("sqlite", filepath.Join(in.dataDir, "hashi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = raw.Close() }()
	if _, err := raw.Exec(`UPDATE admin_sessions SET scopes_json = json_insert(scopes_json, '$[#]', 'security.manage', '$[#]', 'admin.read')`); err != nil {
		t.Fatal(err)
	}

	service := func(slug string) string {
		r := c.mustStatus(c.do(http.MethodPost, "/api/services", map[string]any{"name": slug, "slug": slug, "kind": "http", "domain_mode": "subdomain",
			"host_name": slug + ".example.com", "target": map[string]any{"address": "192.0.2.20", "port": 8080}}), 200)
		return r.body["service"].(map[string]any)["id"].(string)
	}
	a, b := service("aaa"), service("bbb")

	// forward asks as Traefik does; the test client's address is the loopback one.
	forward := func(host, uri string) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, "http://"+in.addr+"/api/edge-auth/forward", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Forwarded-Host", host)
		req.Header.Set("X-Forwarded-Uri", uri)
		req.Header.Set("X-Forwarded-Method", "GET")
		resp, err := http.DefaultTransport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}
	if got := forward("aaa.example.com", "/"); got != 204 {
		t.Fatalf("open service: %d", got)
	}
	// A name no service has is shut, however it is spelled; the health routers answer on any name (06 §2.1).
	if got := forward("nothing.example.com", "/"); got != 403 {
		t.Fatalf("unknown host: %d", got)
	}
	if got := forward("nothing.example.com", "/.well-known/hashi/health/open"); got != 204 {
		t.Fatalf("health: %d", got)
	}

	// An IP set with our address; a shared rule set denies /admin for it and is attached to both services.
	set := c.mustStatus(c.do(http.MethodPost, "/api/ip-sets", map[string]any{"slug": "me", "name": "Me"}), 200).body["id"].(string)
	c.mustStatus(c.do(http.MethodPost, "/api/ip-sets/"+set+"/elements", map[string]any{"item": map[string]any{"type": "ip", "address": "127.0.0.1"}}), 200)
	rs := c.mustStatus(c.do(http.MethodPost, "/api/rule-sets", map[string]any{"slug": "shared", "name": "Shared"}), 200).body["id"].(string)
	c.mustStatus(c.do(http.MethodPut, "/api/rule-sets/"+rs+"/entries", map[string]any{"entries": []map[string]any{
		{"type": "item", "applies_to": set, "match_type": "prefix", "pattern": "/admin", "action": "deny", "enabled": true}}}), 200)
	for _, id := range []string{a, b} {
		c.mustStatus(c.do(http.MethodPut, "/api/services/"+id+"/access", map[string]any{"entries": []map[string]any{{"type": "set", "child_id": rs}}}), 200)
	}
	for _, host := range []string{"aaa.example.com", "bbb.example.com"} {
		if got := forward(host, "/admin/x"); got != 403 {
			t.Errorf("%s /admin/x: %d", host, got)
		}
		if got := forward(host, "/%61dmin/../admin"); got != 403 {
			t.Errorf("%s encoded path: %d", host, got)
		}
		if got := forward(host, "/open"); got != 204 {
			t.Errorf("%s /open: %d", host, got)
		}
	}
	tr := c.mustStatus(c.do(http.MethodPost, "/api/addresses/127.0.0.1/trace", map[string]any{"service_id": a, "path": "/admin"}), 200)
	if tr.body["action"] != "deny" || tr.body["decided"].(float64) != 0 {
		t.Fatalf("trace: %s", tr.raw)
	}
	path := tr.body["flattened"].([]any)[0].(map[string]any)["path"].([]any)
	if len(path) != 2 || path[0] != "service root" || path[1] != "shared" {
		t.Fatalf("trace path: %v", path)
	}

	// Deny fallback: closed, and the outage behaviour says so (06 §2.1).
	r := c.mustStatus(c.do(http.MethodPut, "/api/services/"+b+"/access", map[string]any{"entries": []map[string]any{}, "fallback": "deny"}), 200)
	if r.body["access"].(map[string]any)["fail_closed"] != true {
		t.Fatalf("access: %s", r.raw)
	}
	for _, host := range []string{"bbb.example.com", "bbb.example.com.", "BBB.example.com:443"} {
		if got := forward(host, "/open"); got != 403 {
			t.Errorf("closed service as %q: %d", host, got)
		}
	}
	// A renamed service's old name is shut at once, though Traefik may route it until its plan applies.
	c.mustStatus(c.do(http.MethodPut, "/api/services/"+b+"/access", map[string]any{"entries": []map[string]any{}, "fallback": "none"}), 200)
	if got := forward("bbb.example.com.", "/open"); got != 204 {
		t.Errorf("open service with a trailing dot: %d", got)
	}
	c.mustStatus(c.do(http.MethodPut, "/api/services/"+b+"/access", map[string]any{"entries": []map[string]any{}, "fallback": "deny"}), 200)

	// Attached set can't be deleted; a cycle is refused; Pass CAPTCHA without CAPTCHA set up and sign-in without a
	// provider stay shut.
	c.mustStatus(c.do(http.MethodDelete, "/api/rule-sets/"+rs, nil), 409)
	c.mustStatus(c.do(http.MethodPut, "/api/rule-sets/"+rs+"/entries", map[string]any{"entries": []map[string]any{{"type": "set", "child_id": rs}}}), 400)
	c.mustStatus(c.do(http.MethodPut, "/api/rule-sets/"+rs+"/entries", map[string]any{"entries": []map[string]any{
		{"type": "item", "applies_to": set, "action": "pass_captcha", "enabled": true}}}), 200)
	if got := forward("aaa.example.com", "/"); got != 403 {
		t.Errorf("Pass CAPTCHA without CAPTCHA set up: %d", got)
	}
	c.mustStatus(c.do(http.MethodPut, "/api/services/"+a+"/access", map[string]any{"entries": []map[string]any{
		{"type": "item", "applies_to": set, "action": "pass_authentication", "enabled": true}}}), 200)
	if got := forward("aaa.example.com", "/"); got != 403 {
		t.Errorf("sign-in rule without a provider: %d", got)
	}
}

// Stage A over HTTP (06 §3): a flood is challenged before the service's rules, ignoring the challenge ends in a ban,
// and the ban is enforced by A1 from then on. Without a CAPTCHA the challenge answers 429.
func TestForwardAuthStageA(t *testing.T) {
	_, srv := hetznertest.NewSimServer(t)
	cfg := newConfig(t)
	cfg.dnsProviders = func(_ string, token secrets.Value) (dns.Provider, error) {
		return hetzner.New(token, hetzner.WithBaseURL(srv.URL+"/v1")), nil
	}
	// One frozen instant: the 318 requests fall in one 10 s window however slow the runner is (CI took 9.6 s).
	at := time.Now()
	cfg.abuseNow = func() time.Time { return at }
	in := start(t, cfg)
	c := in.client()
	c.bootstrapLogin()
	c.mustStatus(c.do(http.MethodPut, "/api/setup/base-settings", map[string]any{"root_domain": "example.com",
		"admin_domain": "dev.hashi.example.com", "internal_url": "http://192.0.2.10:3340", "sync_interval_seconds": 3600,
		"theme": "graphite", "public_dashboard_enabled": true, "public_status_enabled": true}), 200)
	c.mustStatus(c.do(http.MethodPut, "/api/setup/dns/provider", map[string]any{"type": "hetzner", "token": hetznertest.Token, "default_ttl": 3600}), 200)
	c.mustStatus(c.do(http.MethodPost, "/api/services", map[string]any{"name": "aaa", "slug": "aaa", "kind": "http", "domain_mode": "subdomain",
		"host_name": "aaa.example.com", "target": map[string]any{"address": "192.0.2.20", "port": 8080}}), 200)

	forward := func() int {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, "http://"+in.addr+"/api/edge-auth/forward", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Forwarded-Host", "aaa.example.com")
		req.Header.Set("X-Forwarded-Uri", "/")
		req.Header.Set("X-Forwarded-Method", "GET")
		resp, err := http.DefaultTransport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}
	seen := map[int]int{}
	first429 := -1
	for i := range 300 + 15 + 3 {
		code := forward()
		seen[code]++
		if code == 429 && first429 < 0 {
			first429 = i
		}
	}
	// 300 within 10 s pass, the 301st is challenged, 14 more are challenged and the 15th unsolved request bans.
	if seen[204] != 300 || first429 != 300 || seen[429] != 15 || seen[403] != 3 {
		t.Fatalf("answers %v, first 429 at %d", seen, first429)
	}
}

// The WAF in forward auth (06 §9) with the default headers-only mode: plain requests, WebDAV, uploads and git pass;
// SQL injection, path traversal and an unknown method get 403; a path with the WAF off passes everything; five blocks
// within a minute challenge the address (429 for an API client).
func TestForwardAuthWAF(t *testing.T) {
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
	c.mustStatus(c.do(http.MethodPut, "/api/setup/dns/provider", map[string]any{"type": "hetzner", "token": hetznertest.Token, "default_ttl": 3600}), 200)
	r := c.mustStatus(c.do(http.MethodPost, "/api/services", map[string]any{"name": "files", "slug": "files", "kind": "http",
		"domain_mode": "subdomain", "host_name": "files.example.com", "target": map[string]any{"address": "192.0.2.20", "port": 8080}}), 200)
	id := r.body["service"].(map[string]any)["id"].(string)
	c.mustStatus(c.do(http.MethodPut, "/api/services/"+id+"/waf", map[string]any{"paths": []map[string]any{
		{"match": "prefix", "path": "/raw", "mode": "disabled"}}}), 200)

	forward := func(method, uri, contentType string) int {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, "http://"+in.addr+"/api/edge-auth/forward", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Forwarded-Host", "files.example.com")
		req.Header.Set("X-Forwarded-Uri", uri)
		req.Header.Set("X-Forwarded-Method", method)
		req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0")
		req.Header.Set("Accept", "*/*")
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		resp, err := http.DefaultTransport.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode
	}
	for _, x := range []struct {
		method, uri, ct string
		want            int
	}{
		{"GET", "/index.html", "", 204},
		{"POST", "/api/items", "application/json", 204},
		{"PROPFIND", "/dav/", "", 204},
		{"PUT", "/dav/file", "application/octet-stream", 204},
		{"POST", "/owner/repo.git/git-receive-pack", "application/x-git-receive-pack-request", 204},
		{"GET", "/.git/config", "", 403},
		{"GET", "/raw?id=1%27%20OR%201=1--", "", 204},
		{"GET", "/?id=1%27%20OR%201=1--", "", 403},
		{"GET", "/files?name=../../etc/passwd", "", 403},
		{"TRACE", "/", "", 403},
	} {
		if got := forward(x.method, x.uri, x.ct); got != x.want {
			t.Errorf("%s %s: %d, want %d", x.method, x.uri, got, x.want)
		}
	}
	// Three blocks so far; two more within the minute reach the WAF trigger (5): the address is challenged.
	forward("GET", "/?q=%3Cscript%3Ealert(1)%3C/script%3E", "")
	forward("GET", "/?id=1%27%20OR%202=2--", "")
	if got := forward("GET", "/index.html", ""); got != 429 {
		t.Errorf("after five WAF blocks: %d, want 429 (challenged)", got)
	}
}
