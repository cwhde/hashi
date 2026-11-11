package traefik

import (
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"git.juzo.io/juzo/hashi/internal/services"
)

// Names in 10-hashi-http-services.yml. Every object of a service starts with "svc-<slug>"; a route adds its ULID, so
// renaming or reordering routes never changes another route's objects (04 §1.1: identity is the ULID).
const (
	// MiddlewareEdge sets the edge token on requests Hashi receives (01 §8).
	MiddlewareEdge = "hashi-edge"
	// EdgeHeader carries the edge token.
	EdgeHeader = "X-Hashi-Edge"

	tlsOptions12 = "hashi-tls12"
	tlsOptions13 = "hashi-tls13"

	// A service's main router sits below its routes; routes keep their own order above it and stay below the health
	// routers (healthPriority).
	mainPriority  = 10
	routePriority = 1000
)

// httpServices renders the routers, middlewares, services and transports of every renderable service (04 §4,
// 10-hashi-http-services.yml). Disabled and invalid services are absent (04 §7). every are the middlewares on every
// user service (Traefik settings); the caller has checked that the user file defines every name (CheckMiddlewares).
func httpServices(svcs []services.Service, edge bool, every []string) dynamicConfig {
	h := &httpConfig{Routers: map[string]router{}, Middlewares: map[string]middleware{}, Services: map[string]service{},
		ServersTransports: map[string]serversTransport{}}
	tlsOpts := map[string]tlsOption{}
	sorted := append([]services.Service(nil), svcs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Slug < sorted[j].Slug })
	for _, s := range sorted {
		if !s.Renderable() || s.Stream() {
			continue
		}
		base := "svc-" + s.Slug
		rt := &routerTLS{}
		switch s.TLSMinVersion {
		case services.TLS12:
			rt.Options, tlsOpts[tlsOptions12] = tlsOptions12, tlsOption{MinVersion: services.TLS12}
		case services.TLS13:
			rt.Options, tlsOpts[tlsOptions13] = tlsOptions13, tlsOption{MinVersion: services.TLS13}
		}
		transport := ""
		if st, ok := transportFor(s); ok {
			transport = base
			h.ServersTransports[transport] = st
		}
		// Hashi's defaults, the traffic headers, then the user's middlewares (04 §1.1a): every user middleware runs
		// after forward auth.
		svcChain := chain(s, edge)
		if mw, ok := trafficHeaders(s.Traffic); ok {
			h.Middlewares[base+"-traffic"] = mw
			svcChain = append(svcChain, base+"-traffic")
		}
		if !s.System() {
			svcChain = appendNew(appendNew(svcChain, every...), s.Middlewares...)
		}
		pass := passHost(s.Traffic)
		host := "Host(`" + s.HostName + "`)"
		add := func(name string, r router) { h.Routers[name] = r }
		add(base, router{Rule: host, EntryPoints: []string{"websecure"}, Middlewares: svcChain, Service: base,
			Priority: mainPriority, TLS: rt})
		targets := s.Targets
		if targets == nil {
			targets = []services.Target{s.Target}
		}
		h.Services[base] = service{LoadBalancer: balancer(s.Kind, targets, transport, pass)}
		if !s.AdvancedRoutes {
			continue
		}
		for _, r := range s.Routes {
			if !r.Enabled {
				continue
			}
			name := base + "-r-" + strings.ToLower(r.ID)
			mws := appendNew(slices.Clone(svcChain), r.Middlewares...)
			if edge && s.IsChallenge() && r.PathValue == services.ChallengePath+"/" {
				// Hashi's half of the challenge service needs the client address, so it gets the edge token; Cap's half
				// (the main router) never sees the token.
				mws = append([]string{MiddlewareEdge}, mws...)
			}
			if mw, ok := rewrite(r); ok {
				h.Middlewares[name] = mw
				mws = append(mws, name)
			}
			add(name, router{Rule: host + " && " + pathRule(r), EntryPoints: []string{"websecure"}, Middlewares: mws,
				Service: name, Priority: routePriority + r.Priority, TLS: rt})
			kind := s.Kind
			if r.Kind != "" {
				kind = r.Kind // one name can mix them: gRPC needs h2c next to plain HTTP (04 §1.1)
			}
			h.Services[name] = service{LoadBalancer: balancer(kind, r.Targets, transport, pass)}
		}
	}
	c := dynamicConfig{HTTP: h}
	if len(tlsOpts) > 0 {
		c.TLS = &tlsConfig{Options: tlsOpts}
	}
	return c
}

// chain is a service's default middleware chain (04 §4.1): security headers, compression, forward auth, failing open or
// closed by the content of its rules (06 §2.1). Requests to Hashi itself carry the edge token.
func chain(s services.Service, edge bool) []string {
	var out []string
	if edge && s.System() && s.OwnerFeature == services.OwnerSetup {
		out = append(out, MiddlewareEdge)
	}
	auth := MiddlewareAuthOpen
	if s.FailClosed {
		auth = MiddlewareAuthClosed
	}
	return append(out, MiddlewareHeaders, MiddlewareCompress, auth)
}

// appendNew appends the names not listed yet: a middleware named on every service and on the service runs once.
func appendNew(list []string, names ...string) []string {
	for _, n := range names {
		if !slices.Contains(list, n) {
			list = append(list, n)
		}
	}
	return list
}

// transportFor is a service's serversTransport: upstream TLS options (https only) and forwarding timeouts.
func transportFor(s services.Service) (serversTransport, bool) {
	var st serversTransport
	if s.Kind == services.KindHTTPS {
		st.ServerName, st.InsecureSkipVerify = s.TLSServerName, s.TLSSkipVerify
	}
	if t := s.Traffic; t.Timeouts() {
		sec := func(n int) string {
			if n == 0 {
				return ""
			}
			return strconv.Itoa(n) + "s"
		}
		st.ForwardingTimeouts = &forwardingTimeouts{DialTimeout: sec(t.DialTimeout),
			ResponseHeaderTimeout: sec(t.ResponseHeaderTimeout), IdleConnTimeout: sec(t.IdleTimeout)}
	}
	return st, st != (serversTransport{})
}

// trafficHeaders is the service's headers middleware: Host, the client address headers removed, extra headers.
// Traefik's own canonical names are used for the address headers (traffic/requests.txt).
func trafficHeaders(t services.Traffic) (middleware, bool) {
	if !t.Headers() {
		return middleware{}, false
	}
	hd := &headers{}
	req := func(k, v string) {
		if hd.CustomRequestHeaders == nil {
			hd.CustomRequestHeaders = map[string]string{}
		}
		hd.CustomRequestHeaders[k] = v
	}
	if t.Host == services.HostCustom {
		req("Host", t.HostValue)
	}
	if t.DropForwardedFor {
		req("X-Forwarded-For", "")
	}
	if t.DropRealIP {
		req("X-Real-Ip", "")
	}
	for _, x := range t.RequestHeaders {
		req(x.Name, x.Value)
	}
	for _, x := range t.ResponseHeaders {
		if hd.CustomResponseHeaders == nil {
			hd.CustomResponseHeaders = map[string]string{}
		}
		hd.CustomResponseHeaders[x.Name] = x.Value
	}
	return middleware{Headers: hd}, true
}

// passHost is nil (Traefik's default: the public name) unless the upstream address is sent as Host.
func passHost(t services.Traffic) *bool {
	if t.Host != services.HostUpstream {
		return nil
	}
	f := false
	return &f
}

func balancer(kind string, targets []services.Target, transport string, pass *bool) *loadBalancer {
	scheme := map[string]string{services.KindHTTP: "http", services.KindHTTPS: "https", services.KindH2C: "h2c"}[kind]
	weighted := false
	for _, t := range targets {
		weighted = weighted || t.Weight > 0
	}
	lb := &loadBalancer{ServersTransport: transport, PassHostHeader: pass}
	for _, t := range targets {
		srv := server{URL: scheme + "://" + t.HostPort()}
		if weighted {
			w := max(t.Weight, 1)
			srv.Weight = &w
		}
		lb.Servers = append(lb.Servers, srv)
	}
	return lb
}

func pathRule(r services.Route) string {
	switch r.PathMatch {
	case services.MatchExact:
		return "Path(`" + r.PathValue + "`)"
	case services.MatchRegex:
		return "PathRegexp(`" + r.PathValue + "`)"
	default:
		return "PathPrefix(`" + r.PathValue + "`)"
	}
}

// rewrite maps a route's rewrite mode to a Traefik middleware (04 §1.1 rewrite modes).
func rewrite(r services.Route) (middleware, bool) {
	switch r.RewriteMode {
	case services.RewriteStripPrefix:
		return middleware{StripPrefix: &stripPrefix{Prefixes: []string{r.PathValue}}}, true
	case services.RewriteReplacePrefix:
		// "$" in the new prefix is literal: Go's replacement syntax needs it doubled.
		return middleware{ReplacePathRegex: &replacePathRegex{Regex: "^" + regexp.QuoteMeta(r.PathValue) + "(.*)",
			Replacement: strings.ReplaceAll(r.RewriteValue, "$", "$$") + "$1"}}, true
	case services.RewriteExact:
		return middleware{ReplacePath: &replacePath{Path: r.RewriteValue}}, true
	case services.RewriteRegex:
		return middleware{ReplacePathRegex: &replacePathRegex{Regex: r.RewritePattern, Replacement: r.RewriteValue}}, true
	}
	return middleware{}, false
}

// edgeMiddleware sets the edge token, overwriting any value a client sent.
func edgeMiddleware(token string) middleware {
	return middleware{Headers: &headers{CustomRequestHeaders: map[string]string{EdgeHeader: token}}}
}

// EffectiveYAML is the part of 10-hashi-http-services.yml (20-hashi-stream-services.yml for a stream) one service produces (rendered config preview, 04 §8), as
// it would render while enabled, with every the middlewares on every service. The edge token itself lives in the
// security file and never appears here.
func EffectiveYAML(s services.Service, every []string) ([]byte, error) {
	s.Enabled, s.InvalidReason = true, ""
	if s.Stream() {
		return marshal(streamServices([]services.Service{s}))
	}
	return marshal(httpServices([]services.Service{s}, true, every))
}
