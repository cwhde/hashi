package firewall

import (
	"context"
	"database/sql"
	"fmt"
	"net/netip"
	"strings"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// ---- bindings and host access ----

// Bindings are the global allow and block set lists (05 §4.1).
type Bindings struct{ Allow, Block []string }

// GetBindings reads the global bindings.
func (m *Manager) GetBindings(ctx context.Context) (Bindings, error) {
	rows, err := m.st.R.ListFirewallBindings(ctx)
	if err != nil {
		return Bindings{}, err
	}
	b := Bindings{Allow: []string{}, Block: []string{}}
	for _, r := range rows {
		if r.Position == "allow" {
			b.Allow = append(b.Allow, r.IpSetID)
		} else {
			b.Block = append(b.Block, r.IpSetID)
		}
	}
	return b, nil
}

// PutBindings replaces the global bindings; every managed host is planned afterwards by the caller.
func (m *Manager) PutBindings(ctx context.Context, actor, ip string, b Bindings) error {
	err := m.st.Tx(ctx, func(q *db.Queries) error {
		if err := q.DeleteFirewallBindings(ctx); err != nil {
			return err
		}
		now := m.now().UnixMilli()
		for pos, ids := range map[string][]string{"allow": b.Allow, "block": b.Block} {
			for i, id := range ids {
				if _, err := q.GetIPSet(ctx, id); err != nil {
					return fmt.Errorf("%w: IP set %s does not exist", ErrInvalid, id)
				}
				if err := q.InsertFirewallBinding(ctx, db.InsertFirewallBindingParams{Position: pos, IpSetID: id, Ord: int64(i),
					CreatedAt: now}); err != nil {
					if strings.Contains(err.Error(), "UNIQUE") {
						return fmt.Errorf("%w: a set is bound twice to %s", ErrInvalid, pos)
					}
					return err
				}
			}
		}
		return m.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "firewall.bindings.updated", IP: ip,
			Target: "firewall_bindings", Outcome: audit.Success, Detail: map[string]any{"allow": b.Allow, "block": b.Block}})
	})
	return err
}

// GetHostAccess lists a host's access sets.
func (m *Manager) GetHostAccess(ctx context.Context, hostID string) ([]string, error) {
	rows, err := m.st.R.ListFirewallHostAccess(ctx, hostID)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, r := range rows {
		out = append(out, r.IpSetID)
	}
	return out, nil
}

// PutHostAccess replaces a host's access sets.
func (m *Manager) PutHostAccess(ctx context.Context, actor, ip, hostID string, sets []string) error {
	return m.st.Tx(ctx, func(q *db.Queries) error {
		if _, err := q.GetFirewallHost(ctx, hostID); err != nil {
			return ErrNotFound
		}
		if err := q.DeleteFirewallHostAccess(ctx, hostID); err != nil {
			return err
		}
		now := m.now().UnixMilli()
		for _, id := range sets {
			if _, err := q.GetIPSet(ctx, id); err != nil {
				return fmt.Errorf("%w: IP set %s does not exist", ErrInvalid, id)
			}
			if err := q.InsertFirewallHostAccess(ctx, db.InsertFirewallHostAccessParams{HostID: hostID, IpSetID: id, CreatedAt: now}); err != nil {
				return err
			}
		}
		return m.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "firewall.host_access.updated", IP: ip,
			Target: "firewall_host:" + hostID, Outcome: audit.Success, Detail: map[string]any{"sets": sets}})
	})
}

// Folded compiles the bindings (plus hashi-hosts on the allow side, 05 §4.1) into the kernel partition, and returns
// the compiled inputs for explanations.
func (m *Manager) Folded(ctx context.Context, q *db.Queries) (Folded, []netip.Prefix, []netip.Prefix, error) {
	g, err := ipsets.LoadGraph(ctx, q)
	if err != nil {
		return Folded{}, nil, nil, err
	}
	b, err := q.ListFirewallBindings(ctx)
	if err != nil {
		return Folded{}, nil, nil, err
	}
	hh, err := q.GetIPSetBySystemKey(ctx, sql.NullString{String: ipsets.SystemHashiHosts, Valid: true})
	if err != nil {
		return Folded{}, nil, nil, err
	}
	r, now := m.sets.Resolver(ctx), m.now()
	allowIDs, blockIDs := []string{hh.ID}, []string{}
	for _, x := range b {
		if x.Position == "allow" {
			allowIDs = append(allowIDs, x.IpSetID)
		} else {
			blockIDs = append(blockIDs, x.IpSetID)
		}
	}
	union := func(ids []string) []netip.Prefix {
		var out []netip.Prefix
		for _, id := range ids {
			out = append(out, g.Compile(id, r, now).Leaves...) // unmerged: longest prefix per side (05 §4.2)
		}
		return out
	}
	allow, block := union(allowIDs), union(blockIDs)
	return Fold(allow, block), allow, block, nil
}

// OwnAddresses are the admin's own addresses: every firewall host's host-access sets (05 §4.3). The abuse engine
// never challenges or bans them, and the kernel never meters them (06 §3, invariant 20).
func (m *Manager) OwnAddresses(ctx context.Context, q *db.Queries) ([]netip.Prefix, error) {
	rows, err := q.ListAllFirewallHostAccess(ctx)
	if err != nil {
		return nil, err
	}
	g, err := ipsets.LoadGraph(ctx, q)
	if err != nil {
		return nil, err
	}
	r, now := m.sets.Resolver(ctx), m.now()
	seen := map[string]bool{}
	var out []netip.Prefix
	for _, a := range rows {
		if seen[a.IpSetID] {
			continue
		}
		seen[a.IpSetID] = true
		out = append(out, g.Compile(a.IpSetID, r, now).Leaves...)
	}
	return out, nil
}
