package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/edge"
	"git.juzo.io/juzo/hashi/internal/rules"
	"git.juzo.io/juzo/hashi/internal/security"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/traefik"
	"git.juzo.io/juzo/hashi/internal/waf"
)

const (
	edgeLoginPath    = "/api/edge-auth/login"
	edgeCallbackPath = "/api/edge-auth/callback"
	edgeLogoutPath   = "/api/edge-auth/logout"
)

// edgeRoutes are the plain handlers Traefik and browsers call (06 §2, §7).
func (s *Server) edgeRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET "+forwardAuthPath, s.edgeForward)
	mux.HandleFunc("GET "+edgeLoginPath, s.edgeLogin)
	mux.HandleFunc("GET "+edgeCallbackPath, s.edgeCallback)
	mux.HandleFunc("GET "+edgeLogoutPath, s.edgeLogout)
}

func edgePage(w http.ResponseWriter, status int, text string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(text + "\n"))
}

// edgeForward decides one request (06 §2): 204 allow, 302 to sign-in for browsers, 401 for API clients, 403 denied.
func (s *Server) edgeForward(w http.ResponseWriter, r *http.Request) {
	ri := info(r.Context())
	if !ri.ip.IsValid() { // fail closed (01 §8)
		edgePage(w, http.StatusForbidden, "Access denied.")
		return
	}
	host := strings.TrimSuffix(hostOnly(r.Header.Get("X-Forwarded-Host")), ".")
	uri := r.Header.Get("X-Forwarded-Uri")
	// The health routers answer on every host name with ping@internal, nothing behind them (06 §2.1).
	if p := edge.NormalizePath(uri); p == traefik.HealthPath+"/open" || p == traefik.HealthPath+"/closed" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	d := s.Edge.DecideHost(host, ri.ip, uri)
	// The challenge service passes only the challenge flow's paths: Cap's dashboard is never reachable there (06 §11).
	if d.Challenge && !challengeFlowPath(edge.NormalizePath(uri)) {
		edgePage(w, http.StatusNotFound, "Not found.")
		return
	}

	var sess *edge.Session
	if c, err := r.Cookie(edge.CookieSession); err == nil && c.Value != "" {
		got, err := s.SSO.Validate(r.Context(), c.Value)
		switch {
		case err == nil:
			sess = &got
		case !errors.Is(err, edge.ErrNoSession):
			s.Log.Warn("edge session", "err", err)
		}
	}

	// Every decision is counted per minute for the dashboard (06 §6.1).
	decision := security.DecisionDeny
	if s.Security != nil {
		defer func() {
			s.Security.Record(security.Hit{At: time.Now(), Addr: ri.ip, ServiceID: d.ServiceID, Decision: decision,
				Path: edge.NormalizePath(uri)})
		}()
	}

	// Stage A runs before the service's rules, for every service (06 §3).
	if s.Security != nil {
		// The challenge flow gets A1 only: a challenged address must always reach it, and it never challenges (06 §11).
		o, err := s.Security.StageA(r.Context(), security.Request{Addr: ri.ip, SSO: sess != nil, ChallengeFlow: d.Challenge,
			StaticBlock: s.Edge.StaticBlocked(ri.ip), ServiceID: d.ServiceID})
		if err != nil { // the state change was not stored: answer as if it had been, and say so in the log
			s.Log.Error("abuse engine", "err", err)
		}
		switch o.Step {
		case security.Blocked, security.BannedNow:
			decision = security.DecisionBlocked
			edgePage(w, http.StatusForbidden, "Access denied.")
			return
		case security.Challenged:
			decision = security.DecisionChallenge
			s.challenge(w, r, "https://"+host+uri, scopeGlobal, "")
			return
		}
	}
	if d.Challenge {
		decision = security.DecisionAllow
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// The WAF (06 §9) sees what forward auth sees: method, URI and headers, never the body. A refused request counts
	// towards the address's WAF trigger; an allowed address is refused too but never challenged for it.
	if s.WAF != nil && s.Security != nil {
		eff := s.wafFor(d.WAF, d.System, uri)
		if eff.Mode == settings.WAFHeadersOnly {
			v, err := s.WAF.Check(waf.Config{Exclusions: eff.Exclusions}, wafRequest(r, ri.ip, host, uri))
			switch {
			case err != nil: // a WAF that cannot run never takes the service down
				s.Log.Warn("WAF", "service", d.ServiceID, "err", err)
			case v.Blocked:
				decision = security.DecisionDeny
				if err := s.Security.WAFMatch(r.Context(), ri.ip, d.ServiceID, security.WAFDetail{Method: r.Header.Get("X-Forwarded-Method"), Path: pathOnly(uri), Rules: v.Rules}); err != nil {
					s.Log.Error("WAF match", "err", err)
				}
				edgePage(w, http.StatusForbidden, "Request refused.")
				return
			}
		}
	}
	allow := func() {
		decision = security.DecisionAllow
		if sess != nil {
			w.Header().Set(traefik.HeaderUser, sess.Username)
		}
		w.WriteHeader(http.StatusNoContent)
	}
	switch d.Action {
	case rules.ActionNone:
		allow()
	case rules.ActionCaptcha:
		switch {
		case sess != nil: // a valid edge SSO session satisfies it: a signed-in user is never asked (06 §4.1)
			allow()
		case !s.captchaSettings(r.Context()).Enabled: // nothing to solve: the item stays shut (06 §11)
			s.Log.Warn("a rule asks for a CAPTCHA, but CAPTCHA is not set up", "service", d.ServiceID)
			edgePage(w, http.StatusForbidden, "Access denied.")
		case s.cleared(r, d.ServiceID):
			allow()
		default:
			decision = security.DecisionChallenge
			s.challenge(w, r, "https://"+host+uri, scopeService, d.ServiceID)
		}
	case rules.ActionAuth:
		provider := d.Item.Entry.Provider
		want := provider
		if want == "" {
			want = s.SSO.DefaultProviderID(r.Context())
		}
		switch {
		case want == "": // no provider set up: a rule that asks for sign-in stays shut
			s.Log.Warn("a rule asks for sign-in, but no provider is set up", "service", d.ServiceID)
			edgePage(w, http.StatusForbidden, "Access denied.")
		case sess != nil && sess.ProviderID == want && sess.HasAnyGroup(d.Item.Entry.Groups):
			allow()
		case sess != nil && sess.ProviderID == want:
			edgePage(w, http.StatusForbidden, "Your account is not allowed here.")
		case browserRequest(r):
			decision = security.DecisionAuth
			s.redirectToLogin(w, r, host, uri, provider)
		default:
			decision = security.DecisionAuth
			edgePage(w, http.StatusUnauthorized, "Authentication required.")
		}
	default: // deny, and anything this build does not know
		edgePage(w, http.StatusForbidden, "Access denied.")
	}
}

// wafRequest is the original request as forward auth received it: Traefik copies the client's headers and adds the
// X-Forwarded-* ones, which are Hashi's and Traefik's, not the client's.
func wafRequest(r *http.Request, client netip.Addr, host, uri string) waf.Request {
	h := map[string][]string{}
	for k, v := range r.Header {
		lk := strings.ToLower(k)
		if strings.HasPrefix(lk, "x-forwarded-") || lk == "x-real-ip" || lk == strings.ToLower(traefik.EdgeHeader) {
			continue
		}
		h[k] = v
	}
	method := r.Header.Get("X-Forwarded-Method")
	if method == "" {
		method = http.MethodGet
	}
	return waf.Request{Client: client, Method: method, URI: uri, Host: host, Header: h}
}

// challenge answers a request that has to solve a CAPTCHA first (06 §3.3): a browser goes to the challenge page, an API
// client gets 429. Until a CAPTCHA is set up there is nothing to solve, so browsers get 429 too; unsolved requests
// still count.
func (s *Server) challenge(w http.ResponseWriter, r *http.Request, rd, scope, serviceID string) {
	if c := s.captchaSettings(r.Context()); c.Enabled && browserRequest(r) {
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, challengeURL(c, scope, serviceID, rd), http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = w.Write([]byte(`{"error":"challenge_required"}` + "\n"))
}

// browserRequest is 06 §3.4's test: Accept names HTML and the original method is safe.
func browserRequest(r *http.Request) bool {
	m := r.Header.Get("X-Forwarded-Method")
	return strings.Contains(r.Header.Get("Accept"), "text/html") && (m == http.MethodGet || m == http.MethodHead)
}

func (s *Server) redirectToLogin(w http.ResponseWriter, r *http.Request, host, uri, provider string) {
	g, err := settings.Get[settings.General](r.Context(), s.Store.R)
	if err != nil || g.AdminDomain == "" {
		s.Log.Warn("sign-in redirect without an admin domain", "err", err)
		edgePage(w, http.StatusForbidden, "Access denied.")
		return
	}
	q := url.Values{"rd": {"https://" + host + uri}}
	if provider != "" {
		q.Set("provider", provider)
	}
	http.Redirect(w, r, "https://"+g.AdminDomain+edgeLoginPath+"?"+q.Encode(), http.StatusFound)
}

// edgeRedirectAllowed is the open-redirect check (06 §7): https, on the root domain or a service's own name.
func (s *Server) edgeRedirectAllowed(ctx context.Context, rd string, g settings.General) bool {
	u, err := url.Parse(rd)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Host == "" {
		return false
	}
	host := hostOnly(u.Host)
	if g.RootDomain != "" && (host == g.RootDomain || strings.HasSuffix(host, "."+g.RootDomain)) {
		return true
	}
	svcs, err := services.List(ctx, s.Store.R)
	if err != nil {
		return false
	}
	for _, sv := range svcs {
		if sv.HostName == host {
			return true
		}
	}
	return false
}

func (s *Server) edgeLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, err := settings.Get[settings.General](ctx, s.Store.R)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	rd := r.URL.Query().Get("rd")
	if !s.edgeRedirectAllowed(ctx, rd, g) {
		edgePage(w, http.StatusBadRequest, "That sign-in link does not point at a service here.")
		return
	}
	authURL, state, err := s.SSO.Begin(ctx, r.URL.Query().Get("provider"), rd, "https://"+g.AdminDomain+edgeCallbackPath)
	if err != nil {
		s.Log.Warn("edge sign-in", "err", err)
		edgePage(w, http.StatusBadGateway, "Sign-in is not available right now.")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: edge.CookieState, Value: state, Path: "/", MaxAge: 600, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, authURL, http.StatusFound) //nolint:gosec // the configured provider's endpoint, not user input
}

func (s *Server) edgeCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ri := info(ctx)
	g, err := settings.Get[settings.General](ctx, s.Store.R)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: edge.CookieState, Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		s.Log.Info("the sign-in provider answered with an error", "error", e)
		edgePage(w, http.StatusForbidden, "The sign-in was not completed.")
		return
	}
	stateCookie := ""
	if c, err := r.Cookie(edge.CookieState); err == nil {
		stateCookie = c.Value
	}
	cookie, rd, expires, err := s.SSO.Finish(ctx, q.Get("code"), q.Get("state"), stateCookie, "https://"+g.AdminDomain+edgeCallbackPath, ri.ipString(), ri.ua)
	if err != nil {
		s.Log.Warn("edge sign-in failed", "err", err)
		_ = s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: audit.System, Action: "edge.login", IP: ri.ipString(), Target: "edge", Outcome: audit.Failure})
		edgePage(w, http.StatusForbidden, "The sign-in could not be completed. Go back to the page and try again.")
		return
	}
	cfg, err := settings.Get[settings.EdgeSSO](ctx, s.Store.R)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	http.SetCookie(w, edgeSessionCookie(g.RootDomain, cookie, expires, cfg.RememberDevice))
	_ = s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: audit.System, Action: "edge.login", IP: ri.ipString(), Target: "edge", Outcome: audit.Success})
	http.Redirect(w, r, rd, http.StatusFound) //nolint:gosec // checked by edgeRedirectAllowed and kept server-side since login
}

func (s *Server) edgeLogout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	g, err := settings.Get[settings.General](ctx, s.Store.R)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	end := ""
	if c, err := r.Cookie(edge.CookieSession); err == nil {
		if end, err = s.SSO.Logout(ctx, c.Value, ""); err != nil {
			s.Log.Warn("edge sign-out", "err", err)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: edge.CookieSession, Domain: g.RootDomain, Path: "/", MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	if end != "" && r.URL.Query().Get("provider") == "1" { // optional end-session at the provider (06 §7)
		http.Redirect(w, r, end, http.StatusFound) //nolint:gosec // the provider's own end-session endpoint
		return
	}
	edgePage(w, http.StatusOK, "Signed out.")
}

// pathOnly drops the query: it can carry tokens, and events keep requests for weeks.
func pathOnly(uri string) string {
	p, _, _ := strings.Cut(uri, "?")
	return p
}

// edgeSessionCookie is the edge sign-in cookie on the root domain. With remember-device it outlives the browser session
// until the sign-in expires; without, it ends when the browser closes (06 §16, 09 §2).
func edgeSessionCookie(root, value string, expires time.Time, remember bool) *http.Cookie {
	c := &http.Cookie{Name: edge.CookieSession, Value: value, Domain: root, Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode}
	if remember {
		c.MaxAge = int(time.Until(expires).Seconds())
	}
	return c
}

// wafFor is the WAF that applies to a request: the service's paths and mode over the engine settings' default mode
// (06 §9, §10).
func (s *Server) wafFor(w services.WAF, system bool, uri string) services.EffectiveWAF {
	return services.ResolveWAF(w, system, s.Security.Settings().WAFDefaultMode, edge.NormalizePath(uri))
}
