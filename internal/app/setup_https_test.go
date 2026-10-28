package app

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/setup"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	"git.juzo.io/juzo/hashi/internal/traefik"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// Step 8 over HTTP (02 §1 step 8): the proof counts only through Hashi-managed Traefik (edge token), for the admin
// domain, over HTTPS, from a bootstrap network, with the latest unused nonce; it then signs the browser in on the
// HTTPS origin with a Secure cookie bound to its address there.
func TestSetupStep8HTTPSProof(t *testing.T) {
	cfg := newConfig(t)
	ctx := context.Background()
	st := storetest.Open(t, cfg.DBPath())
	master, err := vault.LoadMasterKey(cfg.MasterKeyFile)
	if err != nil {
		t.Fatal(err)
	}
	v, err := vault.New(master, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Put(ctx, st.Q, settings.General{RootDomain: "example.com", AdminDomain: "dev.hashi.example.com",
		InternalURL: "http://192.0.2.10:3340", SyncIntervalSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	edge, err := traefik.EnsureEdgeToken(ctx, st, v)
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Close()

	in := start(t, cfg)
	c := in.client()
	c.bootstrapLogin()
	c.mustStatus(c.do(http.MethodPost, "/api/setup/https/start", nil), 409) // step 7 is not done

	st = storetest.Open(t, cfg.DBPath())
	if err := setup.New(st, v, audit.New([]byte("k"))).MarkDone(ctx, st.Q, setup.StepSystemSvc, nil); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
	r := c.mustStatus(c.do(http.MethodPost, "/api/setup/https/start", nil), 200)
	link := r.body["url"].(string)
	prefix := "https://dev.hashi.example.com/setup#https="
	if !strings.HasPrefix(link, prefix) {
		t.Fatalf("link %q", link)
	}
	nonce := strings.TrimPrefix(link, prefix)

	browser := func(h map[string]string) *client {
		b := in.client()
		for k, v := range h {
			b.header.Set(k, v)
		}
		return b
	}
	ok := map[string]string{"Host": "dev.hashi.example.com", "X-Forwarded-Proto": "https", "X-Forwarded-For": "192.168.50.1",
		"X-Hashi-Edge": edge.Reveal()}
	without := func(k string) map[string]string {
		m := map[string]string{}
		for kk, vv := range ok {
			if kk != k {
				m[kk] = vv
			}
		}
		return m
	}
	with := func(k, v string) map[string]string {
		m := without(k)
		m[k] = v
		return m
	}
	for name, h := range map[string]map[string]string{
		"no edge token":  without("X-Hashi-Edge"),
		"wrong token":    with("X-Hashi-Edge", "forged"),
		"other host":     with("Host", "hashi.example.com"),
		"plain http":     without("X-Forwarded-Proto"),
		"public address": with("X-Forwarded-For", "198.51.100.7"),
	} {
		b := browser(h)
		if r := b.do(http.MethodPost, "/api/setup/https/proof", map[string]string{"nonce": nonce}); r.status != 403 || b.cookie != nil {
			t.Errorf("%s: %d %s", name, r.status, r.raw)
		}
	}
	b := browser(ok)
	b.mustStatus(b.do(http.MethodPost, "/api/setup/https/proof", map[string]string{"nonce": "wrong"}), 403)
	r = b.mustStatus(b.do(http.MethodPost, "/api/setup/https/proof", map[string]string{"nonce": nonce}), 200)
	if b.cookie == nil || b.cookie.Name != "__Host-hashi.session" || !b.cookie.Secure || !b.cookie.HttpOnly {
		t.Fatalf("cookie %+v", b.cookie)
	}
	if r.body["bound_ip"] != "192.168.50.1" || r.body["method"] != "bootstrap" {
		t.Errorf("session: %s", r.raw)
	}
	r = b.mustStatus(b.do(http.MethodGet, "/api/setup", nil), 200)
	if s := r.body["steps"].([]any)[setup.StepHTTPS-1].(map[string]any); s["status"] != "done" {
		t.Errorf("step 8: %v", s)
	}
	again := browser(ok)
	again.mustStatus(again.do(http.MethodPost, "/api/setup/https/proof", map[string]string{"nonce": nonce}), 403) // used

	// S5 gate: every request on the admin route also passes forward auth, which the edge sends through its loopback
	// hashi-fa hop: same cookie, edge token, and 127.0.0.1 appended to X-Forwarded-For (contract finding 3). That call
	// must neither be mistaken for a request from 127.0.0.1 nor end the session (it did: ip_mismatch, bounce to login).
	fa := browser(map[string]string{"X-Hashi-Edge": edge.Reveal(), "X-Forwarded-For": "192.168.50.1, 127.0.0.1",
		"X-Forwarded-Proto": "https", "X-Forwarded-Host": "dev.hashi.example.com", "Host": "127.0.0.1:28181"})
	fa.cookie = b.cookie
	fa.mustStatus(fa.do(http.MethodGet, "/api/edge-auth/forward", nil), 204)
	r = b.mustStatus(b.do(http.MethodGet, "/api/auth/session", nil), 200)
	if r.body["authenticated"] != true || r.body["bound_ip"] != "192.168.50.1" {
		t.Fatalf("session after a forward-auth call: %s", r.raw)
	}
}
