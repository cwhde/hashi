package ipsets

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Target is the resolution of one target position (05a §5.2): the addresses to use, and whether they are a fresh
// resolution or the last good one kept because the set resolves to nothing now (invariant 21).
type Target struct {
	Addresses []netip.Addr
	// Unresolved is the reason the set resolves to nothing now ("" when Addresses are fresh). Addresses then hold the
	// last good resolution, or nothing when the position never resolved.
	Unresolved string
}

// ErrNotAddressSet: a target position names a set that holds more than single addresses.
var ErrNotAddressSet = fmt.Errorf("%w: targets take only address sets (ip, host and dns items)", ErrInvalid)

// CheckTarget refuses a set that cannot stand in a target position (05a §5.2).
func CheckTarget(ctx context.Context, q *db.Queries, id string) error {
	g, err := LoadGraph(ctx, q)
	if err != nil {
		return err
	}
	s, ok := g[id]
	if !ok {
		return ErrNotFound
	}
	if !g.Compile(id, nil, time.Time{}).AddressSet {
		return fmt.Errorf("%w (%s)", ErrNotAddressSet, s.Name)
	}
	return nil
}

// ResolveTarget resolves set id for consumer, a stable key of the position ("service:<id>", "dns_record:<id>", …).
// A non-empty resolution is stored as the position's last good one; an empty one returns the stored addresses with
// the reason and records it for Overview → needs attention. Nothing is ever emptied because a resolution failed.
func (s *Service) ResolveTarget(ctx context.Context, consumer, id string) (Target, error) {
	res, err := s.Compile(ctx, s.st.R, id)
	if err != nil {
		return Target{}, err
	}
	if !res.AddressSet {
		return Target{}, ErrNotAddressSet
	}
	now := s.now().UnixMilli()
	if len(res.Addresses) > 0 {
		b, err := json.Marshal(res.Addresses)
		if err != nil {
			return Target{}, err
		}
		cur, err := s.st.R.GetIPSetTarget(ctx, db.GetIPSetTargetParams{Consumer: consumer, IpSetID: id})
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return Target{}, err
		}
		if err != nil || cur.AddressesJson != string(b) || cur.UnresolvedReason != "" {
			if err := s.st.Q.PutIPSetTargetResolved(ctx, db.PutIPSetTargetResolvedParams{Consumer: consumer, IpSetID: id,
				AddressesJson: string(b), ResolvedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now}); err != nil {
				return Target{}, err
			}
		}
		return Target{Addresses: res.Addresses}, nil
	}
	reason := unresolvedReason(res)
	t := Target{Unresolved: reason}
	cur, err := s.st.R.GetIPSetTarget(ctx, db.GetIPSetTargetParams{Consumer: consumer, IpSetID: id})
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return Target{}, err
	default:
		if err := json.Unmarshal([]byte(cur.AddressesJson), &t.Addresses); err != nil {
			return Target{}, fmt.Errorf("stored target %s: %w", consumer, err)
		}
		if cur.UnresolvedReason == reason {
			return t, nil
		}
	}
	if err := s.st.Q.PutIPSetTargetUnresolved(ctx, db.PutIPSetTargetUnresolvedParams{Consumer: consumer, IpSetID: id,
		UnresolvedReason: reason, UpdatedAt: now}); err != nil {
		return Target{}, err
	}
	return t, nil
}

func unresolvedReason(r Result) string {
	if len(r.Problems) == 0 {
		return "the set has no addresses"
	}
	msgs := make([]string, 0, len(r.Problems))
	for _, p := range r.Problems {
		msgs = append(msgs, p.Message)
	}
	return "target unresolved: " + strings.Join(msgs, "; ")
}
