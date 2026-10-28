package services

import (
	"fmt"
	"net/netip"
	"regexp"
	"strings"
)

// InvalidError says which field of a service is wrong and why (04 §1.5). The API returns it as a 422.
type InvalidError struct {
	Field string
	Msg   string
}

func (e *InvalidError) Error() string { return e.Field + ": " + e.Msg }

func invalid(field, format string, a ...any) error {
	return &InvalidError{Field: field, Msg: fmt.Sprintf(format, a...)}
}

var (
	slugRe  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
	labelRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
)

// Names validates Hashi needs from settings and the DNS step to check a service's host name.
type Names struct {
	Root string // root domain (setup step 2)
	Zone string // the managed zone (setup step 3); every served name must lie inside it (03 §5.3)
}

// Validate checks a service before it is saved or rendered (04 §1.5). Values are never interpolated into config, but the
// renderer still relies on these checks: host names are LDH-only and paths contain no backtick, so a Traefik rule built
// from them cannot be broken out of.
func Validate(s Service, n Names) error {
	if strings.TrimSpace(s.Name) == "" || len(s.Name) > 64 {
		return invalid("name", "is required and at most 64 characters")
	}
	if !slugRe.MatchString(s.Slug) {
		return invalid("slug", "must be lowercase letters, digits and dashes (at most 63)")
	}
	switch s.Kind {
	case KindHTTP, KindHTTPS, KindH2C:
	default:
		return invalid("kind", "must be http, https or h2c")
	}
	if err := validateHost(s, n); err != nil {
		return err
	}
	if err := validateTarget("target", s.Target); err != nil {
		return err
	}
	if s.Kind != KindHTTPS && (s.TLSSkipVerify || s.TLSServerName != "") {
		return invalid("tls_server_name", "upstream TLS options apply to https services only")
	}
	if s.TLSServerName != "" && !isHostName(s.TLSServerName) {
		return invalid("tls_server_name", "%q is not a host name", s.TLSServerName)
	}
	switch s.TLSMinVersion {
	case "", TLS12, TLS13:
	default:
		return invalid("tls_min_version", "must be empty, %s or %s", TLS12, TLS13)
	}
	if !s.AdvancedRoutes {
		return nil
	}
	seen := map[int]bool{}
	for i, r := range s.Routes {
		if err := validateRoute(fmt.Sprintf("routes[%d]", i), r); err != nil {
			return err
		}
		if seen[r.Priority] {
			return invalid(fmt.Sprintf("routes[%d].priority", i), "priority %d is used twice", r.Priority)
		}
		seen[r.Priority] = true
	}
	return nil
}

func validateHost(s Service, n Names) error {
	h := s.HostName
	if !isHostName(h) {
		return invalid("host_name", "%q is not a valid domain name", h)
	}
	if n.Zone == "" || n.Root == "" {
		return invalid("host_name", "set the root domain and the DNS zone first (setup steps 2 and 3)")
	}
	if !within(h, n.Zone) {
		return invalid("host_name", "%s is not inside the managed zone %s", h, n.Zone)
	}
	switch s.DomainMode {
	case ModeRoot:
		if h != n.Root {
			return invalid("host_name", "the root domain mode serves %s itself", n.Root)
		}
	case ModeSubdomain:
		if !strings.HasSuffix(h, "."+n.Root) {
			return invalid("host_name", "%s is not a subdomain of %s", h, n.Root)
		}
	case ModeCustom:
	default:
		return invalid("domain_mode", "must be root, subdomain or custom")
	}
	// Reserved for generated host records (03 §5.1): nothing may lie under on.*, via.* or *.ovr.* of the root.
	if rel, ok := strings.CutSuffix(h, "."+n.Root); ok {
		labels := strings.Split(rel, ".")
		if last := labels[len(labels)-1]; last == "on" || last == "via" {
			return invalid("host_name", "names under %s.%s are reserved for host records", last, n.Root)
		}
		for _, l := range labels {
			if l == "ovr" {
				return invalid("host_name", "names with an ovr label are reserved for routing overrides")
			}
		}
	}
	return nil
}

func validateTarget(field string, t Target) error {
	if t.Port < 1 || t.Port > 65535 {
		return invalid(field+".port", "must be 1–65535")
	}
	if t.Weight < 0 || t.Weight > 1000 {
		return invalid(field+".weight", "must be 0–1000")
	}
	if _, err := netip.ParseAddr(t.Address); err == nil {
		if strings.Contains(t.Address, "%") {
			return invalid(field+".address", "zoned IPv6 addresses are not supported")
		}
		return nil
	}
	if !isHostName(t.Address) {
		return invalid(field+".address", "%q is neither an IP address nor a host name", t.Address)
	}
	return nil
}

func validateRoute(field string, r Route) error {
	if r.Priority < 1 || r.Priority > 9999 {
		return invalid(field+".priority", "must be 1–9999")
	}
	if len(r.Targets) == 0 {
		return invalid(field+".targets", "need at least one target")
	}
	for i, t := range r.Targets {
		if err := validateTarget(fmt.Sprintf("%s.targets[%d]", field, i), t); err != nil {
			return err
		}
	}
	if strings.ContainsAny(r.PathValue, "`\n") || strings.ContainsAny(r.RewritePattern, "`\n") ||
		strings.ContainsAny(r.RewriteValue, "`\n") {
		return invalid(field, "paths and patterns cannot contain backticks or line breaks")
	}
	switch r.PathMatch {
	case MatchPrefix, MatchExact:
		if !strings.HasPrefix(r.PathValue, "/") {
			return invalid(field+".path_value", "must start with /")
		}
	case MatchRegex:
		if _, err := regexp.Compile(r.PathValue); err != nil {
			return invalid(field+".path_value", "regular expression: %v", err)
		}
	default:
		return invalid(field+".path_match", "must be prefix, exact or regex")
	}
	switch r.RewriteMode {
	case RewriteNone:
		if r.RewritePattern != "" || r.RewriteValue != "" {
			return invalid(field+".rewrite_value", "set a rewrite mode or leave the rewrite empty")
		}
	case RewriteStripPrefix:
		if r.PathMatch != MatchPrefix {
			return invalid(field+".rewrite_mode", "strip prefix needs a prefix match")
		}
	case RewriteReplacePrefix:
		if r.PathMatch != MatchPrefix {
			return invalid(field+".rewrite_mode", "replace prefix needs a prefix match")
		}
		if !strings.HasPrefix(r.RewriteValue, "/") {
			return invalid(field+".rewrite_value", "the new prefix must start with /")
		}
	case RewriteExact:
		if !strings.HasPrefix(r.RewriteValue, "/") {
			return invalid(field+".rewrite_value", "the new path must start with /")
		}
	case RewriteRegex:
		if _, err := regexp.Compile(r.RewritePattern); err != nil || r.RewritePattern == "" {
			return invalid(field+".rewrite_pattern", "a valid regular expression is required")
		}
		if r.RewriteValue == "" {
			return invalid(field+".rewrite_value", "a replacement is required")
		}
	default:
		return invalid(field+".rewrite_mode", "unknown rewrite mode %q", r.RewriteMode)
	}
	return nil
}

// isHostName reports whether h is a lowercase LDH domain name with at least two labels.
func isHostName(h string) bool {
	if len(h) > 253 || h != strings.ToLower(h) {
		return false
	}
	labels := strings.Split(h, ".")
	if len(labels) < 2 {
		return false
	}
	for _, l := range labels {
		if !labelRe.MatchString(l) {
			return false
		}
	}
	return true
}

// within reports whether name is zone or lies under it.
func within(name, zone string) bool { return name == zone || strings.HasSuffix(name, "."+zone) }
