package traefik

import (
	"regexp"
	"sort"
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
// 10-hashi-http-services.yml). Disabled and invalid services are absent (04 §7).
func httpServices(svcs []services.Service, edge bool) dynamicConfig {
	h := &httpConfig{Routers: map[string]router{}, Middlewares: map[string]middleware{}, Services: map[string]service{},
		ServersTransports: map[string]serversTransport{}}
	tlsOpts := map[string]tlsOption{}
	sorted := append([]services.Service(nil), svcs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Slug < sorted[j].Slug })
	for _, s := range sorted {
		if !s.Renderable() {
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
		if s.Kind == services.KindHTTPS && (s.TLSSkipVerify || s.TLSServerName != "") {
			transport = base
			h.ServersTransports[transport] = serversTransport{ServerName: s.TLSServerName, InsecureSkipVerify: s.TLSSkipVerify}
		}
		host := "Host(`" + s.HostName + "`)"
		h.Routers[base] = router{Rule: host, EntryPoints: []string{"websecure"}, Middlewares: chain(s, edge), Service: base,
			Priority: mainPriority, TLS: rt}
		targets := s.Targets
		if targets == nil {
			targets = []services.Target{s.Target}
		}
		h.Services[base] = service{LoadBalancer: balancer(s.Kind, targets, transport)}
		if !s.AdvancedRoutes {
			continue
		}
		for _, r := range s.Routes {
			if !r.Enabled {
				continue
			}
			name := base + "-r-" + strings.ToLower(r.ID)
			mws := chain(s, edge)
			if mw, ok := rewrite(r); ok {
				h.Middlewares[name] = mw
				mws = append(mws, name)
			}
			h.Routers[name] = router{Rule: host + " && " + pathRule(r), EntryPoints: []string{"websecure"}, Middlewares: mws,
				Service: name, Priority: routePriority + r.Priority, TLS: rt}
			h.Services[name] = service{LoadBalancer: balancer(s.Kind, r.Targets, transport)}
		}
	}
	c := dynamicConfig{HTTP: h}
	if len(tlsOpts) > 0 {
		c.TLS = &tlsConfig{Options: tlsOpts}
	}
	return c
}

// chain is a service's default middleware chain (04 §4.1): security headers, compression, forward auth. Every service
// is fail-open until rule sets that can deny exist (S9, 06 §2.1). Requests to Hashi itself carry the edge token.
func chain(s services.Service, edge bool) []string {
	var out []string
	if edge && s.System() && s.OwnerFeature == services.OwnerSetup {
		out = append(out, MiddlewareEdge)
	}
	return append(out, MiddlewareHeaders, MiddlewareCompress, MiddlewareAuthOpen)
}

func balancer(kind string, targets []services.Target, transport string) *loadBalancer {
	scheme := map[string]string{services.KindHTTP: "http", services.KindHTTPS: "https", services.KindH2C: "h2c"}[kind]
	weighted := false
	for _, t := range targets {
		weighted = weighted || t.Weight > 0
	}
	lb := &loadBalancer{ServersTransport: transport}
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

// EffectiveYAML is the part of 10-hashi-http-services.yml one service produces (rendered config preview, 04 §8), as
// it would render while enabled. The edge token itself lives in the security file and never appears here.
func EffectiveYAML(s services.Service) ([]byte, error) {
	s.Enabled, s.InvalidReason = true, ""
	return marshal(httpServices([]services.Service{s}, true))
}
