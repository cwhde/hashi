package firewall

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// ---- validation ----

// Validate logs in with the pinned host key, reads the host (05 §2) and stores the facts. The first validation also
// keeps the complete existing ruleset, shown before the first plan.
func (m *Manager) Validate(ctx context.Context, id string) (Facts, error) {
	row, err := m.st.R.GetFirewallHost(ctx, id)
	if err != nil {
		return Facts{}, ErrNotFound
	}
	if row.EdgeMode != ModeManaged {
		return Facts{}, fmt.Errorf("%w: %s is behind a router Hashi does not manage (external_nat): nothing to validate", ErrInvalid, row.Name)
	}
	f, err := m.validate(ctx, row)
	now := m.now().UnixMilli()
	health, msg := "healthy", ""
	if err != nil {
		health, msg = "failed", err.Error()
	}
	factsJSON, _ := json.Marshal(f)
	var internal []string
	for _, i := range f.Ifaces {
		if i.Name == "lo" || i.Name == f.WAN {
			continue
		}
		for _, a := range i.Addrs {
			if a.Addr().Is4() && !a.Addr().IsLinkLocalUnicast() {
				internal = append(internal, a.Addr().String())
			}
		}
	}
	internalJSON, _ := json.Marshal(internal)
	addr := func(a netip.Addr) string {
		if a.IsValid() {
			return a.String()
		}
		return ""
	}
	if serr := m.st.Q.SetFirewallHostDetected(ctx, db.SetFirewallHostDetectedParams{PublicIp: addr(f.PublicIP),
		PublicIp6: addr(f.PublicIP6), WanIface: f.WAN, InternalIpsJson: string(internalJSON), FactsJson: string(factsJSON),
		Health: health, LastValidatedAt: sql.NullInt64{Int64: now, Valid: true}, LastValidationError: msg, UpdatedAt: now, ID: id}); serr != nil {
		return f, serr
	}
	m.sets.Changed(ctx)
	return f, err
}

// SetupRulesetCommand reads everything that exists before Hashi touches the host (05 §2: shown, never changed).
const SetupRulesetCommand = `echo "### nft -j list ruleset"; nft -j list ruleset 2>&1; ` +
	`for c in iptables-legacy-save ip6tables-legacy-save iptables-nft-save ip6tables-nft-save "ipset save"; do ` +
	`echo "### $c"; $c 2>&1; done`

func (m *Manager) validate(ctx context.Context, row db.FirewallHost) (Facts, error) {
	c, err := m.dial(ctx, row)
	if err != nil {
		return Facts{}, err
	}
	defer func() { _ = c.Close() }()
	r, err := c.Run(ctx, ProbeCommand, nil)
	if err != nil {
		return Facts{}, fmt.Errorf("read the host: %w", err)
	}
	f, perr := ParseProbe(string(r.Stdout))
	if sr, err := c.Run(ctx, `echo "$SSH_CONNECTION"`, nil); err == nil {
		if fs := strings.Fields(string(sr.Stdout)); len(fs) > 0 {
			f.SSHFrom, _ = netip.ParseAddr(fs[0])
		}
	}
	if perr != nil {
		return f, perr
	}
	if row.SetupRuleset == "" {
		if rr, err := c.Run(ctx, SetupRulesetCommand, nil); err == nil {
			_ = m.st.Q.SetFirewallHostSetupRuleset(ctx, db.SetFirewallHostSetupRulesetParams{SetupRuleset: string(rr.Stdout),
				UpdatedAt: m.now().UnixMilli(), ID: row.ID})
		}
	}
	if !f.Systemd {
		return f, fmt.Errorf("%w: %s", ErrInvalid, strings.Join(f.Warnings(), "; "))
	}
	if w := f.Warnings(); len(w) > 0 {
		return f, fmt.Errorf("check before the first apply: %s", strings.Join(w, "; "))
	}
	return f, nil
}

// ConfirmOwnership records that the admin lets Hashi manage the host's firewall (after the ruleset was shown).
func (m *Manager) ConfirmOwnership(ctx context.Context, actor, ip, id string) error {
	row, err := m.st.R.GetFirewallHost(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	if !row.LastValidatedAt.Valid || row.Health == "failed" {
		return fmt.Errorf("%w: validate %s first", ErrNotReady, row.Name)
	}
	now := m.now().UnixMilli()
	return m.st.Tx(ctx, func(q *db.Queries) error {
		if err := q.ConfirmFirewallHostOwnership(ctx, db.ConfirmFirewallHostOwnershipParams{
			OwnershipConfirmedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: id}); err != nil {
			return err
		}
		return m.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "firewall.ownership.confirmed", IP: ip,
			Target: "firewall_host:" + id, Outcome: audit.Success, Detail: map[string]any{"name": row.Name}})
	})
}
