package services

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"git.juzo.io/juzo/hashi/internal/usermw"
)

// Host header modes (04 §1.1a).
const (
	HostPublic   = ""         // the public name (Traefik's default)
	HostUpstream = "upstream" // the upstream address as written in the target, with its port (passHostHeader: false)
	HostCustom   = "custom"   // HostValue
)

// Limits of the traffic options (04 §1.1a).
const (
	MaxHeaders         = 20
	MaxMiddlewares     = 20
	maxDialSeconds     = 300
	maxResponseSeconds = 3600
	maxIdleSeconds     = 3600
)

// Header is one extra header; an empty Value removes the header (captured: testdata/contracts/traefik/traffic/).
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Traffic is what an HTTP service's upstream receives (04 §1.1a). The zero value is Traefik's behaviour.
type Traffic struct {
	Host      string `json:"host,omitempty"`
	HostValue string `json:"host_value,omitempty"`
	// DropForwardedFor and DropRealIP remove the client address headers Traefik adds.
	DropForwardedFor bool     `json:"drop_forwarded_for,omitempty"`
	DropRealIP       bool     `json:"drop_real_ip,omitempty"`
	RequestHeaders   []Header `json:"request_headers,omitempty"`
	ResponseHeaders  []Header `json:"response_headers,omitempty"`
	// Timeouts in seconds; 0 keeps Traefik's default.
	DialTimeout           int `json:"dial_timeout,omitempty"`
	ResponseHeaderTimeout int `json:"response_header_timeout,omitempty"`
	IdleTimeout           int `json:"idle_timeout,omitempty"`
}

// Headers reports whether the options need the service's headers middleware.
func (t Traffic) Headers() bool {
	return t.Host == HostCustom || t.DropForwardedFor || t.DropRealIP || len(t.RequestHeaders) > 0 || len(t.ResponseHeaders) > 0
}

// Timeouts reports whether any timeout is set.
func (t Traffic) Timeouts() bool {
	return t.DialTimeout > 0 || t.ResponseHeaderTimeout > 0 || t.IdleTimeout > 0
}

// Zero reports whether nothing is set.
func (t Traffic) Zero() bool {
	return t.Host == "" && t.HostValue == "" && !t.Headers() && !t.Timeouts()
}

// token is RFC 9110's header name characters.
var token = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]{1,64}$")

func validHeaders(hs []Header, field string, request bool) error {
	if len(hs) > MaxHeaders {
		return invalid(field, "at most %d headers", MaxHeaders)
	}
	seen := map[string]bool{}
	for i, h := range hs {
		f := fmt.Sprintf("%s[%d]", field, i)
		if !token.MatchString(h.Name) {
			return invalid(f+".name", "%q is not a header name (letters, digits and - only, at most 64)", h.Name)
		}
		low := strings.ToLower(h.Name)
		if seen[low] {
			return invalid(f+".name", "%s is listed twice", h.Name)
		}
		seen[low] = true
		if strings.HasPrefix(low, "x-hashi-") {
			return invalid(f+".name", "X-Hashi-* headers belong to Hashi's forward auth")
		}
		if request {
			switch low {
			case "host":
				return invalid(f+".name", "set the Host header with the Host option")
			case "x-forwarded-for", "x-real-ip":
				return invalid(f+".name", "%s has its own option", h.Name)
			}
		}
		if len(h.Value) > 1024 || strings.ContainsAny(h.Value, "\r\n\x00") {
			return invalid(f+".value", "a header value is one line of at most 1024 characters")
		}
	}
	return nil
}

func validTimeout(n, maxS int, field string) error {
	if n < 0 || n > maxS {
		return invalid(field, "between 1 and %d seconds, or empty for Traefik's default", maxS)
	}
	return nil
}

// validateTraffic checks a service's traffic options and extra middleware names (their existence in the user file is
// checked by references).
func validateTraffic(s Service) error {
	t := s.Traffic
	if s.Stream() || s.System() {
		if !t.Zero() || len(s.Middlewares) > 0 {
			return invalid("traffic", "TCP, UDP and system services take no traffic options or middlewares")
		}
	}
	switch t.Host {
	case HostPublic, HostUpstream:
		if t.HostValue != "" {
			return invalid("traffic.host_value", "a Host name is set only with the custom Host option")
		}
	case HostCustom:
		if err := validHost(t.HostValue); err != nil {
			return invalid("traffic.host_value", "%s", err)
		}
	default:
		return invalid("traffic.host", "the Host option is empty (public name), upstream or custom")
	}
	if err := validHeaders(t.RequestHeaders, "traffic.request_headers", true); err != nil {
		return err
	}
	if err := validHeaders(t.ResponseHeaders, "traffic.response_headers", false); err != nil {
		return err
	}
	if err := validTimeout(t.DialTimeout, maxDialSeconds, "traffic.dial_timeout"); err != nil {
		return err
	}
	if err := validTimeout(t.ResponseHeaderTimeout, maxResponseSeconds, "traffic.response_header_timeout"); err != nil {
		return err
	}
	if err := validTimeout(t.IdleTimeout, maxIdleSeconds, "traffic.idle_timeout"); err != nil {
		return err
	}
	if err := validNames(s.Middlewares, "middlewares"); err != nil {
		return err
	}
	for i, r := range s.Routes {
		if err := validNames(r.Middlewares, fmt.Sprintf("routes[%d].middlewares", i)); err != nil {
			return err
		}
	}
	return nil
}

// validHost accepts a host name or address, optionally with a port, as an upstream may expect it in Host.
func validHost(h string) error {
	if h == "" || len(h) > 253 || strings.ContainsAny(h, " \t\r\n/\\@?#`\"'") {
		return fmt.Errorf("%q is not a host name", h)
	}
	return nil
}

func validNames(ns []string, field string) error {
	if len(ns) > MaxMiddlewares {
		return invalid(field, "at most %d middlewares", MaxMiddlewares)
	}
	for i, n := range ns {
		if err := usermw.CheckName(n); err != nil {
			return invalid(fmt.Sprintf("%s[%d]", field, i), "%s", err)
		}
		if slices.Index(ns, n) != i {
			return invalid(fmt.Sprintf("%s[%d]", field, i), "%s is listed twice", n)
		}
	}
	return nil
}

// MiddlewareUse is one place that names a user middleware.
type MiddlewareUse struct {
	ServiceID, ServiceName string
	RouteID                string // empty: the service itself
	RoutePath              string
}

// MiddlewareUses lists where each user middleware name is used, for the editor and the save check (04 §4).
func MiddlewareUses(svcs []Service) map[string][]MiddlewareUse {
	out := map[string][]MiddlewareUse{}
	for _, s := range svcs {
		for _, n := range s.Middlewares {
			out[n] = append(out[n], MiddlewareUse{ServiceID: s.ID, ServiceName: s.Name})
		}
		for _, r := range s.Routes {
			for _, n := range r.Middlewares {
				out[n] = append(out[n], MiddlewareUse{ServiceID: s.ID, ServiceName: s.Name, RouteID: r.ID, RoutePath: r.PathValue})
			}
		}
	}
	return out
}

// CheckMiddlewares marks every renderable HTTP service invalid that names a middleware the user file does not define
// (04 §1.1a): Traefik would disable its routers ("middleware … does not exist"). every is the Traefik setting's list,
// which applies to every user HTTP service.
func CheckMiddlewares(svcs []Service, f usermw.File, every []string) {
	for i, s := range svcs {
		if s.Stream() || s.InvalidReason != "" {
			continue
		}
		var names []string
		if !s.System() {
			names = append(slices.Clone(every), s.Middlewares...)
		}
		for _, r := range s.Routes {
			if s.AdvancedRoutes && r.Enabled { // only rendered routes reference anything
				names = append(names, r.Middlewares...)
			}
		}
		for _, n := range names {
			if !f.Has(n) {
				svcs[i].InvalidReason = fmt.Sprintf("the middleware %s is not in %s", n, usermw.FileName)
				break
			}
		}
	}
}
