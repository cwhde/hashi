// Package ipsets owns IP sets (05a): the item variants, normalization, the cycle check and the only compiler. Consumers
// receive compiled results and never parse items themselves (05a §7).
package ipsets

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Set kinds (05a §3).
const (
	KindUser    = "user"
	KindSystem  = "system"
	KindFeed    = "feed"
	KindDynamic = "dynamic"
)

// System set keys.
const (
	SystemAll             = "all"
	SystemHashiHosts      = "hashi_hosts"
	SystemPrivateNetworks = "private_networks"
)

// Item types.
const (
	TypeIP    = "ip"
	TypeCIDR  = "cidr"
	TypeHost  = "host"
	TypePulse = "pulse"
	TypeDNS   = "dns"
	TypeGeo   = "geo"
	TypeSet   = "set"
)

// Scopes of host (and later Pulse) items.
const (
	ScopePublic   = "public"
	ScopeInternal = "internal"
	ScopeAll      = "all"
)

// Item is one element's content: an IP item or a reference to another set. The variants are the types below; the
// unexported method keeps the list closed.
type Item interface {
	Type() string
	isItem()
}

// IP is one address (/32 or /128).
type IP struct{ Addr netip.Addr }

// CIDR is one prefix with host bits zero, never a single address (those are IP).
type CIDR struct{ Prefix netip.Prefix }

// Host is a firewall or edge host's addresses in a scope.
type Host struct {
	HostID string
	Scope  string
}

// Pulse is a Pulse agent's addresses in a scope; Selector narrows internal candidates to an interface or a prefix (05a §2).
type Pulse struct {
	AgentID  string
	Scope    string
	Selector string
	// Name is the agent's name, filled when a graph loads, for display only.
	Name string
}

// DNS is a name's A/AAAA answers.
type DNS struct{ FQDN string }

// Geo kinds (05a §2).
const (
	GeoCountry = "country"
	GeoRegion  = "region"
	GeoASN     = "asn"
)

// Geo is a country, region or network: a lookup at L7 and the database's prefixes at the firewall (05a §4.3).
type Geo struct {
	Kind  string
	Value string
}

// Matches reports whether a lookup's answer is this predicate.
func (g Geo) Matches(country, region string, asn uint) bool {
	switch g.Kind {
	case GeoCountry:
		return country != "" && g.Value == country
	case GeoRegion:
		return region != "" && g.Value == region
	case GeoASN:
		return asn != 0 && g.Value == strconv.FormatUint(uint64(asn), 10)
	}
	return false
}

// SetRef includes another IP set.
type SetRef struct{ SetID string }

func (IP) Type() string     { return TypeIP }
func (CIDR) Type() string   { return TypeCIDR }
func (Host) Type() string   { return TypeHost }
func (Pulse) Type() string  { return TypePulse }
func (DNS) Type() string    { return TypeDNS }
func (Geo) Type() string    { return TypeGeo }
func (SetRef) Type() string { return TypeSet }

func (IP) isItem()     {}
func (CIDR) isItem()   {}
func (Host) isItem()   {}
func (Pulse) isItem()  {}
func (DNS) isItem()    {}
func (Geo) isItem()    {}
func (SetRef) isItem() {}

// Element is an item at a position inside a set.
type Element struct {
	ID        string
	SetID     string
	Position  int
	Origin    string
	Locked    bool
	Item      Item
	Note      string
	ExpiresAt *time.Time
	CreatedBy string
	CreatedAt time.Time
}

// Expired reports whether the element has dropped out of its set at now (05a §2).
func (e Element) Expired(now time.Time) bool { return e.ExpiresAt != nil && !e.ExpiresAt.After(now) }

// ErrInvalid wraps every item validation error; the API answers 400 with its text.
var ErrInvalid = errors.New("invalid item")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// ParseAddress normalizes an address: IPv4-mapped IPv6 becomes IPv4, zones are refused.
func ParseAddress(s string) (netip.Addr, error) {
	a, err := netip.ParseAddr(strings.TrimSpace(s))
	if err != nil {
		return netip.Addr{}, invalid("%q is not an IP address", s)
	}
	if a.Zone() != "" {
		return netip.Addr{}, invalid("%q has a zone; zones are not allowed", s)
	}
	return a.Unmap(), nil
}

// ParseIPOrCIDR turns user input into an ip or cidr item: host bits are zeroed, and a /32 or /128 (or a bare
// address) becomes an ip item (05a §2).
func ParseIPOrCIDR(s string) (Item, error) {
	s = strings.TrimSpace(s)
	if !strings.Contains(s, "/") {
		a, err := ParseAddress(s)
		if err != nil {
			return nil, err
		}
		return IP{Addr: a}, nil
	}
	p, err := netip.ParsePrefix(s)
	if err != nil {
		return nil, invalid("%q is not a prefix", s)
	}
	if p.Addr().Zone() != "" {
		return nil, invalid("%q has a zone; zones are not allowed", s)
	}
	if p.Addr().Is4In6() {
		bits := p.Bits() - 96
		if bits < 0 {
			return nil, invalid("%q is an IPv4-mapped prefix shorter than /96", s)
		}
		p = netip.PrefixFrom(p.Addr().Unmap(), bits)
	}
	p = p.Masked()
	if p.IsSingleIP() {
		return IP{Addr: p.Addr()}, nil
	}
	return CIDR{Prefix: p}, nil
}

var (
	countryRe = regexp.MustCompile(`^[A-Z]{2}$`)
	regionRe  = regexp.MustCompile(`^[A-Z]{2}-[A-Z0-9]{1,3}$`)
)

// NormalizeFQDN lowercases a name and drops a trailing dot; it must be a plausible DNS name.
func NormalizeFQDN(s string) (string, error) {
	n := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
	if n == "" || len(n) > 253 || !strings.Contains(n, ".") {
		return "", invalid("%q is not a fully qualified domain name", s)
	}
	for _, label := range strings.Split(n, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return "", invalid("%q is not a valid domain name", s)
		}
		for _, c := range label {
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' && c != '_' {
				return "", invalid("%q is not a valid domain name", s)
			}
		}
	}
	if _, err := netip.ParseAddr(n); err == nil {
		return "", invalid("%q is an address; use an ip item", s)
	}
	return n, nil
}

// Normalize validates an item's own fields and returns its canonical form. References (host, set) are checked
// against the stored graph by the caller.
func Normalize(it Item) (Item, error) {
	switch v := it.(type) {
	case IP:
		if !v.Addr.IsValid() {
			return nil, invalid("missing address")
		}
		return IP{Addr: v.Addr.Unmap().WithZone("")}, nil
	case CIDR:
		if !v.Prefix.IsValid() {
			return nil, invalid("missing prefix")
		}
		return ParseIPOrCIDR(v.Prefix.String())
	case Host:
		if v.HostID == "" {
			return nil, invalid("missing host")
		}
		switch v.Scope {
		case ScopePublic, ScopeInternal, ScopeAll:
		default:
			return nil, invalid("scope must be public, internal or all")
		}
		return v, nil
	case Pulse:
		if v.AgentID == "" {
			return nil, invalid("missing Pulse agent")
		}
		switch v.Scope {
		case ScopePublic, ScopeInternal, ScopeAll:
		default:
			return nil, invalid("scope must be public, internal or all")
		}
		v.Selector = strings.TrimSpace(v.Selector)
		return v, nil
	case DNS:
		n, err := NormalizeFQDN(v.FQDN)
		if err != nil {
			return nil, err
		}
		return DNS{FQDN: n}, nil
	case Geo:
		v.Value = strings.ToUpper(strings.TrimSpace(v.Value))
		switch v.Kind {
		case GeoCountry:
			if !countryRe.MatchString(v.Value) {
				return nil, invalid("a country is a two-letter ISO code, like CH")
			}
		case GeoRegion:
			if !regionRe.MatchString(v.Value) {
				return nil, invalid("a region is an ISO 3166-2 code, like CH-ZH")
			}
		case GeoASN:
			n, err := strconv.ParseUint(strings.TrimPrefix(v.Value, "AS"), 10, 32)
			if err != nil || n == 0 {
				return nil, invalid("a network is an AS number, like 13030")
			}
			v.Value = strconv.FormatUint(n, 10)
		default:
			return nil, invalid("geo kind must be country, region or asn")
		}
		return v, nil
	case SetRef:
		if v.SetID == "" {
			return nil, invalid("missing set")
		}
		return v, nil
	case nil:
		return nil, invalid("missing item")
	}
	return nil, invalid("unknown item type %T", it)
}

// Targetable reports whether the item can sit in a target position (05a §5.2). A set reference is decided by the
// referenced set's own capability.
func Targetable(it Item) bool {
	switch it.(type) {
	case IP, Host, Pulse, DNS:
		return true
	}
	return false
}
