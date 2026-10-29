// Package ipsets owns IP sets (05a): the item variants, normalization, the cycle check and the only compiler. Consumers
// receive compiled results and never parse items themselves (05a §7).
package ipsets

import (
	"errors"
	"fmt"
	"net/netip"
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
	TypeIP   = "ip"
	TypeCIDR = "cidr"
	TypeHost = "host"
	TypeDNS  = "dns"
	TypeSet  = "set"
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

// DNS is a name's A/AAAA answers.
type DNS struct{ FQDN string }

// SetRef includes another IP set.
type SetRef struct{ SetID string }

func (IP) Type() string     { return TypeIP }
func (CIDR) Type() string   { return TypeCIDR }
func (Host) Type() string   { return TypeHost }
func (DNS) Type() string    { return TypeDNS }
func (SetRef) Type() string { return TypeSet }

func (IP) isItem()     {}
func (CIDR) isItem()   {}
func (Host) isItem()   {}
func (DNS) isItem()    {}
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
	case DNS:
		n, err := NormalizeFQDN(v.FQDN)
		if err != nil {
			return nil, err
		}
		return DNS{FQDN: n}, nil
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
	case IP, Host, DNS:
		return true
	}
	return false
}
