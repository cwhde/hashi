package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"git.juzo.io/juzo/hashi/internal/captcha"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
)

// The challenge flow (06 §3.3, §11): Hashi's page and API on the challenge service's name. Cap's own public paths sit
// next to them on the same name, so the page loads the widget and its WASM from its own origin.
const (
	challengeStartPath  = services.ChallengePath + "/start"
	challengeVerifyPath = services.ChallengePath + "/verify"
	challengeStatusPath = services.ChallengePath + "/status"
	scopeGlobal         = "global"
	scopeService        = "service"
	// CaptchaSecretPurpose is the Cap secret key's vault purpose.
	CaptchaSecretPurpose = "captcha:secret"
	clearancePurpose     = "captcha-clearance"
)

// CaptchaCache keeps the CAPTCHA settings forward auth reads on every challenged request.
type CaptchaCache struct {
	v atomic.Pointer[settings.Captcha]
}

// Set replaces the cached settings.
func (c *CaptchaCache) Set(v settings.Captcha) { c.v.Store(&v) }

func (s *Server) captchaSettings(ctx context.Context) settings.Captcha {
	if s.Captcha != nil {
		if v := s.Captcha.v.Load(); v != nil {
			return *v
		}
	}
	v, err := settings.Get[settings.Captcha](ctx, s.Store.R)
	if err != nil {
		s.Log.Warn("CAPTCHA settings", "err", err)
		return settings.Captcha{}
	}
	return v
}

var capPublicPath = regexp.MustCompile(`^/[0-9a-f]{10}/(challenge|redeem)$`)

// challengeFlowPath reports whether p belongs to the challenge flow: Hashi's page and API, Cap's assets and its
// challenge and redeem calls. Everything else on the challenge service's name is refused; the dashboard never shows there.
func challengeFlowPath(p string) bool {
	return strings.HasPrefix(p, services.ChallengePath+"/") || strings.HasPrefix(p, "/assets/") || capPublicPath.MatchString(p)
}

func (s *Server) challengeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+challengeStartPath, s.challengeStart)
	mux.HandleFunc("POST "+challengeVerifyPath, s.challengeVerify)
	mux.HandleFunc("GET "+challengeStatusPath, s.challengeStatus)
	mux.HandleFunc("GET "+challengeFontPath+"{name}", challengeFont)
}

// challengeURL is where a browser goes to solve: the challenge page with the scope and the original address.
func challengeURL(c settings.Captcha, scope, serviceID, rd string) string {
	q := url.Values{"scope": {scope}, "rd": {rd}}
	if serviceID != "" {
		q.Set("service", serviceID)
	}
	return "https://" + c.ChallengeDomain + challengeStartPath + "?" + q.Encode()
}

func nonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawStdEncoding.EncodeToString(b)
}

// challengeTarget checks a start or verify request's scope, service and return address. For scope service, rd must be
// on that service's own name, so a clearance is never handed to another service.
func (s *Server) challengeTarget(ctx context.Context, scope, serviceID, rd string) (string, bool) {
	g, err := settings.Get[settings.General](ctx, s.Store.R)
	if err != nil || !s.edgeRedirectAllowed(ctx, rd, g) {
		return "", false
	}
	u, _ := url.Parse(rd) // parsed by edgeRedirectAllowed
	switch scope {
	case scopeGlobal:
		return hostOnly(u.Host), serviceID == ""
	case scopeService:
		sv, err := services.Get(ctx, s.Store.R, serviceID)
		return sv.HostName, err == nil && sv.HostName == hostOnly(u.Host)
	}
	return "", false
}

func (s *Server) challengeStart(w http.ResponseWriter, r *http.Request) {
	c := s.captchaSettings(r.Context())
	q := r.URL.Query()
	host, ok := s.challengeTarget(r.Context(), q.Get("scope"), q.Get("service"), q.Get("rd"))
	if !c.Enabled || !ok {
		edgePage(w, http.StatusNotFound, "Not found.")
		return
	}
	n := nonce()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	// The widget runs its work in blob: workers and compiles WASM; everything else comes from this origin (Cap finding 4, 8).
	// Cap's anti-bot check ("block headless browsers") runs obfuscated code built at run time in a sandboxed srcdoc frame,
	// which inherits this policy: without 'unsafe-eval' it never answers and Cap refuses the solve (seen 2026-11-03). The
	// page holds nothing a visitor controls (escaped parameters only), so run-time code adds no injection path.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'nonce-"+n+"' 'unsafe-eval' 'wasm-unsafe-eval'; "+
		"style-src 'nonce-"+n+"'; font-src 'self'; connect-src 'self'; worker-src blob:; img-src 'self' data:; base-uri 'none'; "+
		"form-action 'none'; frame-ancestors 'none'")
	_ = challengePage.Execute(w, s.challengePageData(r.Context(), c, n, host, q.Get("scope"), q.Get("service"), q.Get("rd"), info(r.Context()).ip))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) captchaSecret(ctx context.Context) (secrets.Value, error) {
	rec, err := s.Store.R.GetSecretByPurpose(ctx, CaptchaSecretPurpose)
	if err != nil {
		return secrets.Value{}, err
	}
	return s.Vault.Open(rec)
}

// challengeVerify checks the Cap token server-side (06 §11). Global: the address leaves dynamic-challenge, its ban
// counter stays. Service: a clearance cookie for that service on the root domain. Failures say nothing more than
// "not solved" or "unavailable".
func (s *Server) challengeVerify(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ri := info(ctx)
	c := s.captchaSettings(ctx)
	var in struct {
		Token   string `json:"token"`
		Scope   string `json:"scope"`
		Service string `json:"service"`
		RD      string `json:"rd"`
	}
	if !c.Enabled || !ri.ip.IsValid() || json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in) != nil || in.Token == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not_solved"})
		return
	}
	if _, ok := s.challengeTarget(ctx, in.Scope, in.Service, in.RD); !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not_solved"})
		return
	}
	secret, err := s.captchaSecret(ctx)
	if err != nil {
		s.Log.Error("CAPTCHA secret", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	}
	vctx, cancel := context.WithTimeout(ctx, time.Duration(c.TimeoutSeconds)*time.Second)
	defer cancel()
	res, err := captcha.Verify(vctx, http.DefaultClient, c.VerifyURL, c.SiteKey, secret, in.Token)
	switch res {
	case captcha.Unavailable:
		s.Log.Warn("CAPTCHA verify", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
		return
	case captcha.NotSolved:
		if err != nil && strings.Contains(err.Error(), "Invalid site key or secret") {
			s.Log.Error("CAPTCHA verify: Cap refuses Hashi's site key or secret", "err", err)
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "not_solved"})
		return
	}
	if in.Scope == scopeGlobal {
		if s.Security != nil {
			if _, err := s.Security.Solve(ctx, ri.ip); err != nil {
				s.Log.Error("CAPTCHA solve", "err", err)
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
				return
			}
		}
	} else {
		g, err := settings.Get[settings.General](ctx, s.Store.R)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "unavailable"})
			return
		}
		hours := 24
		if s.Security != nil {
			hours = s.Security.Settings().ClearanceHours
		}
		until := s.now().Add(time.Duration(hours) * time.Hour)
		http.SetCookie(w, &http.Cookie{Name: captcha.CookieName(in.Service), Value: captcha.Clearance(s.clearanceKey(), in.Service, until),
			Domain: g.RootDomain, Path: "/", Expires: until, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	}
	writeJSON(w, http.StatusOK, map[string]string{"redirect": in.RD})
}

func (s *Server) clearanceKey() []byte { return s.Vault.Operational(clearancePurpose) }

// cleared reports whether r carries a valid clearance for serviceID.
func (s *Server) cleared(r *http.Request, serviceID string) bool {
	c, err := r.Cookie(captcha.CookieName(serviceID))
	return err == nil && captcha.Cleared(s.clearanceKey(), serviceID, c.Value, s.now())
}

func (s *Server) challengeStatus(w http.ResponseWriter, r *http.Request) {
	ri := info(r.Context())
	challenged := s.Security != nil && ri.ip.IsValid() && s.Security.IsChallenged(ri.ip)
	writeJSON(w, http.StatusOK, map[string]bool{"challenged": challenged, "captcha": s.captchaSettings(r.Context()).Enabled})
}
