package dns

import (
	"context"
	"fmt"
	"strings"
)

// ChallengeLabel is the DNS-01 label (RFC 8555 §8.4): the CA reads TXT at _acme-challenge.<name>.
const ChallengeLabel = "_acme-challenge"

// ChallengeTTL is the TTL of a challenge RRSet Hashi creates; resolvers should not cache a value that is gone in minutes.
const ChallengeTTL = 60

// IsChallenge reports whether a zone-relative name is a DNS-01 challenge name. These names are reserved (03 §5.3):
// Hashi adds and removes single values there while it issues certificates, and never imports, prunes or plans them as
// entries, so another ACME client's value at the same name is left alone.
func IsChallenge(name string) bool {
	return name == ChallengeLabel || strings.HasPrefix(name, ChallengeLabel+".")
}

// Challenger writes DNS-01 values into the managed zone through its provider (origin system: no entry, no ownership
// row; the values live only while an order is validated).
type Challenger struct {
	zone Zone
	vw   ValueWriter
	ns   []string
}

// Challenger opens the zone's provider for DNS-01 values. It fails when the provider cannot add single values.
func (s *Service) Challenger(ctx context.Context, zoneID string) (*Challenger, error) {
	c, err := s.connect(ctx, zoneID)
	if err != nil {
		return nil, err
	}
	vw, ok := c.p.(ValueWriter)
	if !ok {
		return nil, fmt.Errorf("the %s DNS provider cannot write single challenge values", c.zone.Name)
	}
	obs, err := s.observed(ctx, s.st.R, zoneID)
	if err != nil {
		return nil, err
	}
	var ns []string
	for _, o := range obs {
		if o.Name == Apex && o.Type == "NS" {
			for _, v := range o.Values {
				ns = append(ns, CanonicalName(v))
			}
		}
	}
	return &Challenger{zone: c.zone, vw: vw, ns: ns}, nil
}

// Zone is the managed zone's name.
func (c *Challenger) Zone() string { return CanonicalName(c.zone.Name) }

// Nameservers are the zone's authoritative servers from the last snapshot (apex NS), the default place to look for a
// challenge value before asking the CA to check it.
func (c *Challenger) Nameservers() []string { return c.ns }

func (c *Challenger) name(fqdn string) (string, error) {
	rel, err := relativeName(fqdn, c.zone.Name)
	if err != nil {
		return "", err
	}
	if FQDN(rel, c.zone.Name) != CanonicalName(fqdn) {
		return "", fmt.Errorf("%w: %s is outside the managed zone %s; Hashi can only prove it owns names in that zone, so "+
			"certificates need a root domain inside it", ErrForbiddenChange, strings.TrimPrefix(fqdn, ChallengeLabel+"."), c.zone.Name)
	}
	if !IsChallenge(rel) {
		return "", fmt.Errorf("%w: %s is not a challenge name in %s", ErrForbiddenChange, fqdn, c.zone.Name)
	}
	return rel, nil
}

// Present adds value to the TXT RRSet at fqdn (an _acme-challenge name inside the zone), keeping other values.
func (c *Challenger) Present(ctx context.Context, fqdn, value string) error {
	rel, err := c.name(fqdn)
	if err != nil {
		return err
	}
	return c.vw.AddValue(ctx, c.zone, rel, "TXT", ChallengeTTL, value)
}

// CleanUp removes value from the TXT RRSet at fqdn; other values stay.
func (c *Challenger) CleanUp(ctx context.Context, fqdn, value string) error {
	rel, err := c.name(fqdn)
	if err != nil {
		return err
	}
	return c.vw.RemoveValue(ctx, c.zone, rel, "TXT", value)
}
