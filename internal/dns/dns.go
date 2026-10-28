// Package dns is Hashi's provider-neutral DNS model (03-dns.md §1): zones, RRSets, the provider interface, and the
// service that keeps the observed snapshot, ownership and import decisions of the managed zone. Provider modules
// (internal/dns/hetzner, …) implement Reader; provider-specific fields never leave the adapter.
package dns

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
)

// Apex is the relative name of the zone apex.
const Apex = "@"

// Zone is a provider zone. ID is the provider's identifier as text; TTL is the zone default that RRSets without a TTL
// of their own inherit.
type Zone struct {
	ID   string
	Name string
	TTL  int
}

// RRSet is the unit of ownership and change (03 §1): name + type → TTL + values.
//
// Name is relative to the zone, lowercase, Apex for the apex. TTL nil means "zone default" and is kept that way
// (03 §3). Values are sorted (order inside an RRSet carries no meaning); TXT values are the decoded text, so one value
// is one logical string however the provider chunks it.
type RRSet struct {
	Name      string
	Type      string
	TTL       *int
	Values    []string
	Labels    map[string]string
	Protected bool // the provider forbids changes; read-only to Hashi (03 §3)
}

// Capabilities describe what a provider supports (03 §1).
type Capabilities struct {
	// ManagedTypes can be created and changed by Hashi; ReadOnlyTypes are only ever read (NS, SOA: invariant 2).
	ManagedTypes  []string
	ReadOnlyTypes []string
	// MultiValue: one RRSet holds several values. ZoneDefaultTTL: an RRSet can inherit the zone TTL.
	MultiValue     bool
	ZoneDefaultTTL bool
	// OwnershipMarker: the provider can mark an RRSet as Hashi's (never authoritative, 03 §3).
	OwnershipMarker bool
	// ChangeProtection: the provider reports RRSets that must not be changed.
	ChangeProtection bool
}

// Reader is the read part of the provider interface (03 §1). Plan/Apply/Verify arrive with the write side (S3).
type Reader interface {
	Capabilities(ctx context.Context) (Capabilities, error)
	ListZones(ctx context.Context) ([]Zone, error)
	// ResolveZone returns the zone with the longest name that fqdn lies in.
	ResolveZone(ctx context.Context, fqdn string) (Zone, error)
	ListRRSets(ctx context.Context, zone Zone) ([]RRSet, error)
}

// Errors every provider maps its failures to.
var (
	// ErrUnauthorized: the token was rejected. Adapters wrap it with an actionable message.
	ErrUnauthorized = errors.New("the DNS provider rejected the API token")
	ErrZoneNotFound = errors.New("no zone found")
	ErrRateLimited  = errors.New("the DNS provider's rate limit is exhausted")
)

// Protected reports whether Hashi may never own, change or delete RRSets of this type (invariant 2).
func Protected(typ string) bool {
	switch strings.ToUpper(typ) {
	case "NS", "SOA":
		return true
	}
	return false
}

// CanonicalName lowercases a DNS name and drops the trailing dot.
func CanonicalName(n string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(n)), ".")
}

// FQDN returns the absolute name (without trailing dot) of a relative name in zone.
func FQDN(name, zone string) string {
	zone = CanonicalName(zone)
	if name == Apex || name == "" {
		return zone
	}
	return CanonicalName(name) + "." + zone
}

// ResolveFromList picks the zone with the longest name that fqdn equals or lies under.
func ResolveFromList(zones []Zone, fqdn string) (Zone, error) {
	fqdn = CanonicalName(fqdn)
	best, found := Zone{}, false
	for _, z := range zones {
		n := CanonicalName(z.Name)
		if (fqdn == n || strings.HasSuffix(fqdn, "."+n)) && len(n) > len(CanonicalName(best.Name)) {
			best, found = z, true
		}
	}
	if !found {
		return Zone{}, ErrZoneNotFound
	}
	return best, nil
}

// Normalize returns r in the canonical form described on RRSet. Adapters call it on everything they return.
func Normalize(r RRSet) RRSet {
	out := RRSet{Name: strings.ToLower(strings.TrimSpace(r.Name)), Type: strings.ToUpper(strings.TrimSpace(r.Type)),
		Labels: r.Labels, Protected: r.Protected}
	if out.Name == "" {
		out.Name = Apex
	}
	if r.TTL != nil {
		ttl := *r.TTL
		out.TTL = &ttl
	}
	if out.Labels == nil {
		out.Labels = map[string]string{}
	}
	out.Values = make([]string, len(r.Values))
	for i, v := range r.Values {
		v = strings.TrimSpace(v)
		if out.Type == "TXT" {
			if t, ok := DecodeTXT(v); ok {
				v = t
			}
		}
		out.Values[i] = v
	}
	slices.Sort(out.Values)
	return out
}

// EffectiveTTL is the TTL resolvers see: the RRSet's own or the zone default.
func (r RRSet) EffectiveTTL(zoneTTL int) int {
	if r.TTL != nil {
		return *r.TTL
	}
	return zoneTTL
}

// Hash identifies the DNS content of a normalized RRSet (name, type, TTL, values; labels and protection excluded).
func (r RRSet) Hash() string {
	ttl := ""
	if r.TTL != nil {
		ttl = strconv.Itoa(*r.TTL)
	}
	b, _ := json.Marshal([]any{r.Name, r.Type, ttl, r.Values}) // cannot fail for strings
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// DecodeTXT turns a TXT value in zone-file presentation form — one or more quoted character strings, e.g.
// `"v=DKIM1; k=rsa; p=MIIB" "IjANBg"` — into the text it encodes (the strings concatenated, RFC 1035 §5.1 escapes
// `\"`, `\\` and `\DDD` resolved). ok is false when v is not in that form; callers then keep v unchanged.
func DecodeTXT(v string) (text string, ok bool) {
	var b strings.Builder
	i, n := 0, len(v)
	found := false
	for {
		for i < n && (v[i] == ' ' || v[i] == '\t') {
			i++
		}
		if i == n {
			return b.String(), found
		}
		if v[i] != '"' {
			return "", false
		}
		i++
		closed := false
		for i < n {
			c := v[i]
			switch {
			case c == '"':
				closed = true
				i++
			case c == '\\' && i+3 < n && isDigit(v[i+1]) && isDigit(v[i+2]) && isDigit(v[i+3]):
				d, err := strconv.ParseUint(v[i+1:i+4], 10, 8) // \DDD is one octet; 256–999 fail here
				if err != nil {
					return "", false
				}
				b.WriteByte(byte(d))
				i += 4
				continue
			case c == '\\' && i+1 < n:
				b.WriteByte(v[i+1])
				i += 2
				continue
			case c == '\\':
				return "", false
			default:
				b.WriteByte(c)
				i++
				continue
			}
			break
		}
		if !closed {
			return "", false
		}
		found = true
	}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
