package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/auth/authtest"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	"git.juzo.io/juzo/hashi/internal/vault"
)

func newConfig(t *testing.T) Config {
	t.Helper()
	dir := t.TempDir()
	return Config{DataDir: dir, MasterKeyFile: writeMasterKey(t, dir)}
}

// trustLoopbackProxy makes 127.0.0.1 a trusted proxy so tests can choose client IPs with X-Forwarded-For.
func trustLoopbackProxy(t *testing.T, cfg Config) {
	t.Helper()
	st := storetest.Open(t, cfg.DBPath())
	if err := settings.Put(context.Background(), st.Q, settings.General{SyncIntervalSeconds: 3600, TrustedProxies: []string{"127.0.0.1/32"}}); err != nil {
		t.Fatal(err)
	}
	_ = st.Close()
}

func TestBootstrapLoginFromLoopbackUsesBootstrapCookie(t *testing.T) {
	in := start(t, newConfig(t))
	c := in.client()
	if r := c.do(http.MethodGet, "/api/auth/session", nil); r.status != 200 || r.body["authenticated"] != false {
		t.Fatalf("anonymous session: %s", r.raw)
	}
	r := c.bootstrapLogin()
	if c.cookie == nil || c.cookie.Name != "hashi.bootstrap" || c.cookie.Secure || !c.cookie.HttpOnly || c.cookie.SameSite != http.SameSiteStrictMode {
		t.Fatalf("cookie %+v", c.cookie)
	}
	if r.body["bound_ip"] != "127.0.0.1" || r.body["method"] != "bootstrap" {
		t.Fatalf("session %s", r.raw)
	}
	r = c.mustStatus(c.do(http.MethodGet, "/api/setup", nil), 200)
	steps := r.body["steps"].([]any)
	if steps[0].(map[string]any)["status"] != "done" {
		t.Fatalf("step 1 not done: %v", steps[0])
	}
	// Bootstrap sessions carry only setup scopes.
	c.mustStatus(c.do(http.MethodGet, "/api/settings", nil), 403)
	// Wrong password.
	u, _ := in.bootstrapCredentials()
	c2 := in.client()
	c2.mustStatus(c2.do(http.MethodPost, "/api/auth/bootstrap/login", map[string]string{"username": u, "password": "nope"}), 401)
}

// 02 §6.3: untrusted forwarding headers cannot select the bound IP.
func TestUntrustedForwardingHeadersAreIgnored(t *testing.T) {
	in := start(t, newConfig(t))
	c := in.client()
	c.header.Set("X-Forwarded-For", "192.168.7.7")
	r := c.bootstrapLogin()
	if r.body["bound_ip"] != "127.0.0.1" {
		t.Fatalf("bound to %v", r.body["bound_ip"])
	}
	c.header.Set("X-Forwarded-For", "192.168.8.8")
	if r := c.do(http.MethodGet, "/api/auth/session", nil); r.body["authenticated"] != true {
		t.Fatal("header change from an untrusted peer affected the session")
	}
}

// 02 §6.2 / acceptance 2, 3, 11: exact-IP binding through a trusted proxy; a second IP gets its own session; the same
// canonical IP is used at issuance and validation.
func TestSessionBoundToClientIPThroughTrustedProxy(t *testing.T) {
	cfg := newConfig(t)
	trustLoopbackProxy(t, cfg)
	in := start(t, cfg)

	a := in.client()
	a.header.Set("X-Forwarded-For", "192.168.1.5")
	if r := a.bootstrapLogin(); r.body["bound_ip"] != "192.168.1.5" {
		t.Fatalf("bound to %v", r.body["bound_ip"])
	}
	b := in.client()
	b.header.Set("X-Forwarded-For", "192.168.1.6")
	b.bootstrapLogin()

	// IPv4-mapped spelling of the same address is the same canonical IP.
	a.header.Set("X-Forwarded-For", "::ffff:192.168.1.5")
	if r := a.do(http.MethodGet, "/api/auth/session", nil); r.body["authenticated"] != true {
		t.Fatalf("mapped address rejected: %s", r.raw)
	}
	stolen := *a.cookie
	// A's cookie replayed from B's address: rejected, revoked, cookie cleared.
	thief := in.client()
	thief.cookie = &stolen
	thief.header.Set("X-Forwarded-For", "192.168.1.6")
	r := thief.do(http.MethodGet, "/api/auth/session", nil)
	if r.body["authenticated"] != false || !strings.Contains(strings.Join(r.header.Values("Set-Cookie"), ";"), "Max-Age=0") {
		t.Fatalf("replay not rejected: %s %v", r.raw, r.header.Values("Set-Cookie"))
	}
	a.header.Set("X-Forwarded-For", "192.168.1.5")
	if r := a.do(http.MethodGet, "/api/auth/session", nil); r.body["authenticated"] != false {
		t.Fatal("revoked session still works from its own IP")
	}
	if r := b.do(http.MethodGet, "/api/auth/session", nil); r.body["authenticated"] != true {
		t.Fatal("independent session B was affected")
	}
}

func TestBootstrapOnlyFromPrivateNetworks(t *testing.T) {
	cfg := newConfig(t)
	trustLoopbackProxy(t, cfg)
	in := start(t, cfg)
	c := in.client()
	c.header.Set("X-Forwarded-For", "203.0.113.9")
	u, p := in.bootstrapCredentials()
	c.mustStatus(c.do(http.MethodPost, "/api/auth/bootstrap/login", map[string]string{"username": u, "password": p}), 403)
}

func TestCSRFRequiredOnUnsafeMethods(t *testing.T) {
	in := start(t, newConfig(t))
	c := in.client()
	c.bootstrapLogin()
	token := c.csrf
	c.csrf = ""
	c.mustStatus(c.do(http.MethodPost, "/api/setup/recovery-key", nil), 403)
	c.csrf = "wrong"
	c.mustStatus(c.do(http.MethodPost, "/api/auth/logout", nil), 403)
	c.csrf = token
	c.mustStatus(c.do(http.MethodPost, "/api/auth/logout", nil), 204)
	if c.cookie != nil {
		t.Fatal("logout did not clear the cookie")
	}
}

// 02 §6.2: restart keeps sessions; the master key file is required and must match.
func TestRestartKeepsSessionAndChecksMasterKey(t *testing.T) {
	cfg := newConfig(t)
	in := start(t, cfg)
	c := in.client()
	c.bootstrapLogin()
	first := in.stdout.String()
	in.stop()

	in2 := start(t, cfg)
	c.in = in2
	if r := c.do(http.MethodGet, "/api/auth/session", nil); r.body["authenticated"] != true {
		t.Fatalf("session lost across restart: %s", r.raw)
	}
	if strings.Contains(in2.stdout.String(), "password:") {
		t.Fatal("bootstrap credentials printed again on restart")
	}
	u, p := in.bootstrapCredentials()
	c2 := in2.client()
	c2.mustStatus(c2.do(http.MethodPost, "/api/auth/bootstrap/login", map[string]string{"username": u, "password": p}), 200)
	in2.stop()
	if !strings.Contains(first, "password:") {
		t.Fatal("first start printed no bootstrap block")
	}

	ctx := context.Background()
	missing := cfg
	missing.MasterKeyFile = ""
	if err := Run(ctx, missing, NewLogger(&syncBuffer{}, 0), nil); err == nil || !strings.Contains(err.Error(), "HASHI_MASTER_KEY_FILE") {
		t.Fatalf("no master key: %v", err)
	}
	wrong := cfg
	wrong.MasterKeyFile = writeMasterKey(t, t.TempDir())
	if err := Run(ctx, wrong, NewLogger(&syncBuffer{}, 0), nil); !errors.Is(err, vault.ErrWrongMasterKey) {
		t.Fatalf("wrong master key: %v", err)
	}
}

// Invariant 5 / 02 §6.3: raw tokens and bootstrap secrets never reach the structured log or the audit API.
func TestInvariant_5_RawTokensNotLogged(t *testing.T) {
	in := start(t, newConfig(t))
	c := in.client()
	c.bootstrapLogin()
	_, pw := in.bootstrapCredentials()
	token := c.cookie.Value
	c.do(http.MethodGet, "/api/auth/session", nil)
	c.do(http.MethodPost, "/api/auth/logout", nil)
	logs := in.logs.String()
	if !strings.Contains(logs, `"msg":"listening"`) {
		t.Fatal("no logs captured")
	}
	for name, secret := range map[string]string{"bootstrap password": pw, "session token": token} {
		if strings.Contains(logs, secret) {
			t.Fatalf("%s in structured log", name)
		}
	}
	dump := filepath.Join(t.TempDir(), "dump.db")
	st := storetest.Open(t, filepath.Join(in.dataDir, "hashi.db"))
	if err := st.BackupTo(context.Background(), dump); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dump) //nolint:gosec // test file
	if err != nil {
		t.Fatal(err)
	}
	for name, secret := range map[string]string{"bootstrap password": pw, "session token": token} {
		if strings.Contains(string(b), secret) {
			t.Fatalf("%s in database", name)
		}
	}
}

// Setup steps 2 and 9 over HTTP, with a software authenticator; the public status toggle is consumed at once.
func TestSetupBaseSettingsPasskeyAndRecoveryKey(t *testing.T) {
	cfg := newConfig(t)
	cfg.PublicStatusAddr = freeAddr(t)
	in := start(t, cfg)
	c := in.client()
	c.bootstrapLogin()

	c.mustStatus(c.do(http.MethodPost, "/api/setup/passkey/options", nil), 409) // needs step 2 first
	bad := map[string]any{"root_domain": "example.com", "admin_domain": "hashi.example.net", "internal_url": "http://192.0.2.10:3340",
		"sync_interval_seconds": 3600, "theme": "graphite", "public_dashboard_enabled": true, "public_status_enabled": false}
	c.mustStatus(c.do(http.MethodPut, "/api/setup/base-settings", bad), 422)
	good := bad
	good["admin_domain"] = "hashi.example.com"
	r := c.mustStatus(c.do(http.MethodPut, "/api/setup/base-settings", good), 200)
	if r.body["steps"].([]any)[1].(map[string]any)["status"] != "done" {
		t.Fatalf("step 2: %s", r.raw)
	}

	r = c.mustStatus(c.do(http.MethodPost, "/api/setup/passkey/options", nil), 200)
	pub := r.body["options"].(map[string]any)["publicKey"].(map[string]any)
	if pub["rp"].(map[string]any)["id"] != "example.com" {
		t.Fatalf("rp %v", pub["rp"])
	}
	challenge := decodeB64(t, pub["challenge"].(string))
	handle := decodeB64(t, pub["user"].(map[string]any)["id"].(string))
	a := authtest.New(t)
	cred := mustJSONMap(t, a.Register("example.com", "https://hashi.example.com", challenge, handle))
	c.mustStatus(c.do(http.MethodPost, "/api/setup/passkey/verify", map[string]any{
		"ceremony_id": r.body["ceremony_id"], "name": "Laptop", "credential": cred,
	}), 200)

	r = c.mustStatus(c.do(http.MethodPost, "/api/setup/recovery-key", nil), 200)
	key := r.body["recovery_key"].(string)
	groups := strings.Split(key, "-")
	c.mustStatus(c.do(http.MethodPost, "/api/setup/recovery-key/confirm", map[string]string{"checksum": "AAAA"}), 422)
	r = c.mustStatus(c.do(http.MethodPost, "/api/setup/recovery-key/confirm", map[string]string{"checksum": groups[len(groups)-1]}), 200)
	if r.body["steps"].([]any)[8].(map[string]any)["status"] != "done" {
		t.Fatalf("step 9: %s", r.raw)
	}
	// Steps 3–8 belong to later slices: completion is refused in this build.
	r = c.mustStatus(c.do(http.MethodPost, "/api/setup/complete", nil), 409)
	if !strings.Contains(r.raw, "DNS provider") {
		t.Fatalf("complete: %s", r.raw)
	}
	if strings.Contains(in.logs.String(), key) {
		t.Fatal("recovery key in log")
	}
	// Changing the root domain now would strand the passkey (RP ID): it is removed and step 9 starts over.
	moved := good
	moved["root_domain"], moved["admin_domain"] = "example.org", "hashi.example.org"
	r = c.mustStatus(c.do(http.MethodPut, "/api/setup/base-settings", moved), 200)
	if r.body["passkeys"] != float64(0) || r.body["steps"].([]any)[8].(map[string]any)["status"] != "pending" {
		t.Fatalf("after root domain change: passkeys %v, step 9 %v", r.body["passkeys"], r.body["steps"].([]any)[8])
	}
	// Public status was switched off in step 2 and answers 404 now (the setting's consumer).
	res, err := http.Get("http://" + cfg.PublicStatusAddr + "/")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNotFound {
		t.Fatalf("public status: %d", res.StatusCode)
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().String()
}

func mustJSONMap(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func decodeB64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// 02 §2: `hashi admin recover` → one-time link → a session that can only register a passkey, then ends.
func TestAdminRecoverLinkRegistersOnePasskey(t *testing.T) {
	cfg := newConfig(t)
	in := start(t, cfg)
	c := in.client()
	c.bootstrapLogin()
	c.mustStatus(c.do(http.MethodPut, "/api/setup/base-settings", map[string]any{
		"root_domain": "example.com", "admin_domain": "hashi.example.com", "internal_url": "http://192.0.2.10:3340",
		"sync_interval_seconds": 3600, "theme": "graphite", "public_dashboard_enabled": true, "public_status_enabled": true,
	}), 200)

	var out strings.Builder
	if err := AdminRecover(context.Background(), cfg, &out); err != nil {
		t.Fatal(err)
	}
	link := strings.TrimSpace(out.String()[strings.Index(out.String(), "https://"):])
	if !strings.HasPrefix(link, "https://hashi.example.com/login/recover#") {
		t.Fatalf("link %q", link)
	}
	token := link[strings.Index(link, "#")+1:]

	r := in.client()
	got := r.mustStatus(r.do(http.MethodPost, "/api/auth/recover", map[string]string{"token": token}), 200)
	if got.body["method"] != "recovery" || r.cookie == nil || r.cookie.Name != "__Host-hashi.session" {
		t.Fatalf("recovery session %s %+v", got.raw, r.cookie)
	}
	r.mustStatus(r.do(http.MethodGet, "/api/settings", nil), 403)
	r.mustStatus(r.do(http.MethodPost, "/api/setup/recovery-key", nil), 403)
	again := in.client()
	again.mustStatus(again.do(http.MethodPost, "/api/auth/recover", map[string]string{"token": token}), 403)

	opts := r.mustStatus(r.do(http.MethodPost, "/api/auth/recover/passkey/options", nil), 200)
	pub := opts.body["options"].(map[string]any)["publicKey"].(map[string]any)
	a := authtest.New(t)
	cred := mustJSONMap(t, a.Register("example.com", "https://hashi.example.com",
		decodeB64(t, pub["challenge"].(string)), decodeB64(t, pub["user"].(map[string]any)["id"].(string))))
	r.mustStatus(r.do(http.MethodPost, "/api/auth/recover/passkey/verify", map[string]any{
		"ceremony_id": opts.body["ceremony_id"], "name": "Replacement", "credential": cred,
	}), 200)
	if r.cookie != nil {
		t.Fatal("recovery cookie not cleared")
	}
	stale := in.client()
	stale.cookie = got.header2cookie(t)
	if res := stale.do(http.MethodGet, "/api/auth/session", nil); res.body["authenticated"] != false {
		t.Fatal("recovery session survived passkey registration")
	}
}
