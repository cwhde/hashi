package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/auth"
	"git.juzo.io/juzo/hashi/internal/captcha"
	"git.juzo.io/juzo/hashi/internal/clientip"
	"git.juzo.io/juzo/hashi/internal/edge"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/rules"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/security"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// fakeCap answers siteverify like Cap 3.1.15 (testdata/contracts/cap/siteverify.txt): one good token, single use.
type fakeCap struct {
	mu   sync.Mutex
	good map[string]bool
	down bool
}

func (f *fakeCap) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.down {
		hj, _ := w.(http.Hijacker)
		c, _, _ := hj.Hijack()
		_ = c.Close()
		return
	}
	var in map[string]string
	_ = json.NewDecoder(r.Body).Decode(&in)
	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	switch {
	case r.URL.Path != "/0123456789/siteverify" || in["secret"] != "sk-test":
		w.WriteHeader(http.StatusForbidden)
		_, _ = io.WriteString(w, `{"success":false,"error":"Invalid site key or secret"}`)
	case f.good[in["response"]]:
		delete(f.good, in["response"])
		_, _ = io.WriteString(w, `{"success":true}`)
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"success":false,"error":"Token not found"}`)
	}
}

// The CAPTCHA flow (06 §3.3, §11) with a real store, vault, forward-auth and abuse engine, and a stand-in Cap.
func TestChallengeFlow(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	al := audit.New([]byte("k"))
	v, err := vault.New(bytes.Repeat([]byte{7}, vault.KeySize), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Put(ctx, st.Q, settings.General{RootDomain: "example.com", AdminDomain: "hashi.example.com",
		InternalURL: "http://192.0.2.5:3340", SyncIntervalSeconds: 3600, TrustedProxies: []string{}}); err != nil {
		t.Fatal(err)
	}
	rec, _ := v.Seal(vault.Service, "dns.token", secrets.New("t"))
	_ = st.Q.InsertSecret(ctx, rec)
	_ = st.Q.InsertDNSProvider(ctx, db.InsertDNSProviderParams{ID: "p1", Name: "h", Type: "hetzner", DefaultTtl: 3600,
		TokenSecretID: rec.ID, Health: "healthy", CreatedAt: 1, UpdatedAt: 1})
	_ = st.Q.InsertDNSZone(ctx, db.InsertDNSZoneParams{ID: "z1", ProviderID: "p1", ProviderZoneID: "1", Name: "example.com", CreatedAt: 1})
	now := time.Now()
	aaa, err := services.Create(ctx, st.Q, services.Service{Name: "aaa", Slug: "aaa", Kind: services.KindHTTP,
		DomainMode: services.ModeSubdomain, HostName: "aaa.example.com", Enabled: true,
		Target: services.Target{Address: "192.0.2.20", Port: 8080}}, now)
	if err != nil {
		t.Fatal(err)
	}

	fc := &fakeCap{good: map[string]bool{"good-1": true, "good-2": true}}
	capSrv := httptest.NewServer(fc)
	defer capSrv.Close()
	c := settings.Captcha{Enabled: true, ChallengeDomain: "cap.hashi.example.com", CapAddress: "192.0.2.6", CapPort: 3343,
		VerifyURL: capSrv.URL, SiteKey: "0123456789", TimeoutSeconds: 2, AdminServiceID: aaa.ID}
	if err := settings.Put(ctx, st.Q, c); err != nil {
		t.Fatal(err)
	}
	srec, _ := v.Seal(vault.Service, CaptchaSecretPurpose, secrets.New("sk-test"))
	if err := st.Q.InsertSecret(ctx, srec); err != nil {
		t.Fatal(err)
	}
	chal, changed, err := services.EnsureChallenge(ctx, st.Q, now)
	if err != nil || !changed || !chal.IsChallenge() || len(chal.Routes) != 1 || chal.Target.Port != 3343 {
		t.Fatalf("challenge service %+v %v %v", chal, changed, err)
	}
	if _, again, err := services.EnsureChallenge(ctx, st.Q, now); err != nil || again {
		t.Fatalf("second ensure changed %v %v", again, err)
	}
	if err := rules.EnsureRoots(ctx, st.Q, now); err != nil {
		t.Fatal(err)
	}

	// No CAPTCHA in front of CAPTCHA: the Cap dashboard's service refuses Pass CAPTCHA on save.
	g, _ := rules.Load(ctx, st.R)
	var aaaRoot string
	for _, rs := range g {
		if rs.Kind == rules.KindServiceRoot && rs.ServiceID == aaa.ID {
			aaaRoot = rs.ID
		}
	}
	all, _ := st.R.GetIPSetBySystemKey(ctx, sql.NullString{String: ipsets.SystemAll, Valid: true})
	captchaItem := []rules.Entry{{Type: rules.EntryItem, AppliesTo: all.ID, Action: rules.ActionCaptcha, Enabled: true}}
	if err := rules.PutEntries(ctx, st.Q, aaaRoot, captchaItem, rules.ActionNone, false, now); err == nil {
		t.Fatal("Pass CAPTCHA on the Cap dashboard's service was saved")
	}
	c.AdminServiceID = ""
	_ = settings.Put(ctx, st.Q, c)
	if err := rules.PutEntries(ctx, st.Q, aaaRoot, captchaItem, rules.ActionNone, false, now); err != nil {
		t.Fatal(err)
	}

	sets := ipsets.NewService(st, al, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
	eng := edge.NewEngine(st, sets, slog.Default())
	if err := eng.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	sec := security.NewEngine(st, slog.Default(), time.Now)
	engineCfg := settings.DefaultOf[settings.Engine]()
	engineCfg.ClearanceHours = 3 // the clearance cookie follows the CAPTCHA clearance setting (09 §2)
	sec.UseSettings(engineCfg)
	sec.UseAllowed(eng.Allowed)
	sec.OnChange(func() {})
	cache := &CaptchaCache{}
	cache.Set(c)
	sso := edge.NewSSO(st, v, http.DefaultClient, time.Now)
	s := &Server{Store: st, Audit: al, Vault: v, Sessions: auth.NewManager(st, al, []byte("csrf")), Edge: eng, Security: sec,
		Captcha: cache, SSO: sso, IP: clientip.New([]netip.Prefix{netip.MustParsePrefix("127.0.0.0/8")}), Log: slog.Default()}
	srv := httptest.NewServer(s.Handler(http.NewServeMux()))
	defer srv.Close()
	visitor := netip.MustParseAddr("198.51.100.7")

	noRedirect := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	type answer struct {
		StatusCode int
		Header     http.Header
	}
	forward := func(host, uri string, browser bool, cookies ...*http.Cookie) answer {
		t.Helper()
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/edge-auth/forward", nil)
		req.Header.Set("X-Forwarded-For", visitor.String())
		req.Header.Set("X-Forwarded-Host", host)
		req.Header.Set("X-Forwarded-Uri", uri)
		req.Header.Set("X-Forwarded-Method", http.MethodGet)
		if browser {
			req.Header.Set("Accept", "text/html")
		}
		for _, ck := range cookies {
			req.AddCookie(ck)
		}
		resp, err := noRedirect.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		return answer{resp.StatusCode, resp.Header}
	}
	type verified struct {
		StatusCode int
		Cookies    []*http.Cookie
	}
	verify := func(body map[string]string) (verified, map[string]string) {
		t.Helper()
		b, _ := json.Marshal(body)
		req, _ := http.NewRequest(http.MethodPost, srv.URL+challengeVerifyPath, bytes.NewReader(b))
		req.Header.Set("X-Forwarded-For", visitor.String())
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		var out map[string]string
		_ = json.NewDecoder(resp.Body).Decode(&out)
		return verified{resp.StatusCode, resp.Cookies()}, out
	}

	// The challenge service passes only the flow's paths; the dashboard never shows there.
	for uri, want := range map[string]int{"/": 404, "/server/keys": 404, "/auth/login": 404,
		challengeStartPath + "?scope=global": 204, "/assets/widget.js": 204, "/0123456789/challenge": 204, "/0123456789/redeem": 204} {
		if got := forward("cap.hashi.example.com", uri, true).StatusCode; got != want {
			t.Errorf("challenge service %s: %d, want %d", uri, got, want)
		}
	}

	// A challenged browser goes to the challenge page; an API client gets 429; the flow itself stays reachable and its
	// requests are not failures.
	if _, err := sec.Challenge(ctx, visitor, security.Cause{Source: security.SourceRate, Reason: "test"}); err != nil {
		t.Fatal(err)
	}
	resp := forward("aaa.example.com", "/page?x=1", true)
	loc, _ := url.Parse(resp.Header.Get("Location"))
	if resp.StatusCode != http.StatusFound || loc.Host != "cap.hashi.example.com" || loc.Path != challengeStartPath ||
		loc.Query().Get("scope") != "global" || loc.Query().Get("rd") != "https://aaa.example.com/page?x=1" {
		t.Fatalf("challenged browser: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	if got := forward("aaa.example.com", "/api", false).StatusCode; got != http.StatusTooManyRequests {
		t.Errorf("challenged API client: %d", got)
	}
	before := sec.Challenges()[0].Failures
	for range 20 {
		if got := forward("cap.hashi.example.com", "/0123456789/challenge", false).StatusCode; got != 204 {
			t.Fatalf("challenge flow while challenged: %d", got)
		}
	}
	if after := sec.Challenges()[0].Failures; after != before {
		t.Errorf("challenge flow requests counted as failures: %d → %d", before, after)
	}

	// The page: only this origin, the visited name shown; a foreign return address gets nothing.
	get := func(q string) (int, http.Header, []byte) {
		r, err := http.Get(srv.URL + challengeStartPath + "?" + q)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = r.Body.Close() }()
		b, _ := io.ReadAll(r.Body)
		return r.StatusCode, r.Header, b
	}
	pageStatus, pageHeader, body := get(loc.RawQuery)
	csp := pageHeader.Get("Content-Security-Policy")
	if pageStatus != 200 || !strings.Contains(string(body), `data-cap-api-endpoint="/0123456789/"`) ||
		!strings.Contains(string(body), "aaa.example.com") || !strings.Contains(csp, "connect-src 'self'") ||
		!strings.Contains(csp, "'unsafe-eval'") || !strings.Contains(string(body), `addEventListener("error"`) ||
		strings.Contains(string(body), "sk-test") || strings.Contains(string(body), "jsdelivr") {
		t.Fatalf("page %d, CSP %q:\n%s", pageStatus, csp, body)
	}
	for _, q := range []string{"scope=global&rd=https://evil.example.net/", "scope=global&rd=http://aaa.example.com/",
		"scope=service&service=" + aaa.ID + "&rd=https://other.example.com/", "scope=nope&rd=https://aaa.example.com/"} {
		if code, _, _ := get(q); code != http.StatusNotFound {
			t.Errorf("start %s: %d", q, code)
		}
	}

	// Solving: a bad token is "not solved", the good one clears the challenge (the ban counter stays), reuse fails.
	if r, out := verify(map[string]string{"token": "bad", "scope": "global", "rd": "https://aaa.example.com/"}); r.StatusCode != 400 || out["error"] != "not_solved" {
		t.Errorf("bad token: %d %v", r.StatusCode, out)
	}
	if r, out := verify(map[string]string{"token": "good-1", "scope": "global", "rd": "https://aaa.example.com/page"}); r.StatusCode != 200 || out["redirect"] != "https://aaa.example.com/page" {
		t.Fatalf("good token: %d %v", r.StatusCode, out)
	}
	if sec.IsChallenged(visitor) {
		t.Error("still challenged after solving")
	}
	if r, _ := verify(map[string]string{"token": "good-1", "scope": "global", "rd": "https://aaa.example.com/"}); r.StatusCode != 400 {
		t.Errorf("reused token: %d", r.StatusCode)
	}

	// Pass CAPTCHA on a service: a browser goes to the page with scope service, a solve gives a clearance cookie for
	// that service on the root domain, and the cookie lets the next request through.
	if err := eng.Rebuild(ctx); err != nil {
		t.Fatal(err)
	}
	resp = forward("aaa.example.com", "/x", true)
	loc, _ = url.Parse(resp.Header.Get("Location"))
	if resp.StatusCode != http.StatusFound || loc.Query().Get("scope") != "service" || loc.Query().Get("service") != aaa.ID {
		t.Fatalf("pass_captcha browser: %d %s", resp.StatusCode, resp.Header.Get("Location"))
	}
	r, out := verify(map[string]string{"token": "good-2", "scope": "service", "service": aaa.ID, "rd": "https://aaa.example.com/x"})
	var clear *http.Cookie
	for _, ck := range r.Cookies {
		if ck.Name == captcha.CookieName(aaa.ID) {
			clear = ck
		}
	}
	if r.StatusCode != 200 || out["redirect"] != "https://aaa.example.com/x" || clear == nil || clear.Domain != "example.com" ||
		!clear.Secure || !clear.HttpOnly {
		t.Fatalf("service solve: %d %v %+v", r.StatusCode, out, clear)
	}
	if left := time.Until(clear.Expires); left < 179*time.Minute || left > 181*time.Minute {
		t.Errorf("clearance lasts %v, want the configured 3 h", left)
	}
	if got := forward("aaa.example.com", "/x", true, &http.Cookie{Name: clear.Name, Value: clear.Value}).StatusCode; got != 204 {
		t.Errorf("with clearance: %d", got)
	}
	if got := forward("aaa.example.com", "/x", true, &http.Cookie{Name: clear.Name, Value: clear.Value + "x"}).StatusCode; got != http.StatusFound {
		t.Errorf("with a tampered clearance: %d", got)
	}
	// A valid edge SSO session satisfies Pass CAPTCHA: a signed-in user is never asked for one (06 §4.1).
	if err := st.Q.InsertOIDCProvider(ctx, db.InsertOIDCProviderParams{ID: "idp", Name: "IdP", Issuer: "https://id.example.com",
		ClientID: "hashi", SecretID: rec.ID, Scopes: "openid", UsernameClaim: "preferred_username", EmailClaim: "email",
		GroupsClaim: "groups", IsDefault: 1, CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	cookie, _, err := sso.OpenSession(ctx, "idp", edge.Identity{Subject: "u1", Username: "alice"}, "198.51.100.7", "test")
	if err != nil {
		t.Fatal(err)
	}
	if got := forward("aaa.example.com", "/x", true, &http.Cookie{Name: edge.CookieSession, Value: cookie}).StatusCode; got != 204 {
		t.Errorf("signed in, still asked for a CAPTCHA: %d", got)
	}

	// Cap unreachable: "unavailable", and nothing is cleared.
	fc.mu.Lock()
	fc.down = true
	fc.mu.Unlock()
	if r, out := verify(map[string]string{"token": "x", "scope": "global", "rd": "https://aaa.example.com/"}); r.StatusCode != 503 || out["error"] != "unavailable" {
		t.Errorf("Cap down: %d %v", r.StatusCode, out)
	}
}
