package firewall

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// ---- host model ----

// HostInput is what the admin enters for a firewall host (05 §2). HostKey is the scanned key the admin confirmed.
// Empty Credential/Passphrase on an update keep the stored ones.
type HostInput struct {
	Name, EdgeMode      string
	Host                string
	Port                int
	User, Auth          string
	Credential          secrets.Value
	Passphrase          secrets.Value
	HostKey             []byte
	TraefikConnectionID string
	InternalTraefikIP   string
	RouteViaHostID      string
	PublicIPOverride    string
	PublicIP6Override   string
	WANOverride         string
	Subnets             []string
	NetBirdMode         string
	NetBirdIface        string
	NetBirdPort         int
	DropInbound         bool
	// AddressAgentID: a host behind a router takes its public and internal addresses from this Pulse agent (bug 23).
	AddressAgentID string
}

var labelRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ReservedNames cannot be host names: they are the structural labels of host records (03 §5.1).
var ReservedNames = []string{"on", "via", "ovr"}

func parseSubnets(ss []string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, s := range ss {
		p, err := netip.ParsePrefix(strings.TrimSpace(s))
		if err != nil {
			return nil, fmt.Errorf("%w: %q is not a subnet (e.g. 10.40.4.0/24)", ErrInvalid, s)
		}
		p = p.Masked()
		for _, o := range out {
			if o.Overlaps(p) {
				return nil, fmt.Errorf("%w: subnets %s and %s overlap", ErrInvalid, o, p)
			}
		}
		out = append(out, p)
	}
	return out, nil
}

func optAddr(s, what string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return "", fmt.Errorf("%w: %s %q is not an address", ErrInvalid, what, s)
	}
	return a.Unmap().String(), nil
}

// Save creates (id "") or updates a host. A changed address or host key clears what validation and the ownership
// confirmation found: they belong to the old machine.
func (m *Manager) Save(ctx context.Context, actor, ip, id string, in HostInput) (db.FirewallHost, error) {
	in.Name, in.Host, in.User = strings.ToLower(strings.TrimSpace(in.Name)), strings.TrimSpace(in.Host), strings.TrimSpace(in.User)
	if in.Auth == "" {
		in.Auth = "hashi_key"
	}
	if in.Port == 0 {
		in.Port = 22
	}
	if in.NetBirdMode == "" {
		in.NetBirdMode = NetBirdDisabled
	}
	switch {
	case !labelRe.MatchString(in.Name) || slices.Contains(ReservedNames, in.Name):
		return db.FirewallHost{}, fmt.Errorf("%w: the name becomes <name>.<root>: lowercase letters, digits and dashes, not on/via/ovr", ErrInvalid)
	case in.EdgeMode != ModeManaged && in.EdgeMode != ModeExternalNAT:
		return db.FirewallHost{}, fmt.Errorf("%w: edge mode must be managed or external_nat", ErrInvalid)
	case in.NetBirdMode != NetBirdDisabled && in.NetBirdMode != NetBirdPeer && in.NetBirdMode != NetBirdRouting:
		return db.FirewallHost{}, fmt.Errorf("%w: NetBird mode must be disabled, peer or routing", ErrInvalid)
	case in.Auth != "hashi_key" && in.Auth != "private_key" && in.Auth != "password":
		return db.FirewallHost{}, fmt.Errorf("%w: unknown login method %q", ErrInvalid, in.Auth)
	}
	subnets, err := parseSubnets(in.Subnets)
	if err != nil {
		return db.FirewallHost{}, err
	}
	traefikIP, err := optAddr(in.InternalTraefikIP, "internal Traefik address")
	if err != nil {
		return db.FirewallHost{}, err
	}
	pub, err := optAddr(in.PublicIPOverride, "public IPv4")
	if err != nil {
		return db.FirewallHost{}, err
	}
	pub6, err := optAddr(in.PublicIP6Override, "public IPv6")
	if err != nil {
		return db.FirewallHost{}, err
	}
	var hk sshx.HostKey
	if in.EdgeMode == ModeManaged {
		switch {
		case in.Host == "" || in.User == "":
			return db.FirewallHost{}, fmt.Errorf("%w: a managed host needs its SSH address and user", ErrInvalid)
		case in.Port < 1 || in.Port > 65535:
			return db.FirewallHost{}, fmt.Errorf("%w: SSH port must be between 1 and 65535", ErrInvalid)
		case len(subnets) == 0:
			return db.FirewallHost{}, fmt.Errorf("%w: a managed host needs at least one managed subnet", ErrInvalid)
		case in.TraefikConnectionID == "" || traefikIP == "":
			return db.FirewallHost{}, fmt.Errorf("%w: link a Traefik connection and its internal address (the DNAT target)", ErrInvalid)
		case in.NetBirdMode != NetBirdDisabled && (in.NetBirdIface == "" || in.NetBirdPort < 1 || in.NetBirdPort > 65535):
			return db.FirewallHost{}, fmt.Errorf("%w: NetBird needs its interface and WireGuard port (validation detects both)", ErrInvalid)
		}
		if !slices.ContainsFunc(subnets, func(p netip.Prefix) bool { return p.Contains(netip.MustParseAddr(traefikIP)) }) {
			return db.FirewallHost{}, fmt.Errorf("%w: the internal Traefik address %s is in none of the managed subnets", ErrInvalid, traefikIP)
		}
	}
	if in.AddressAgentID != "" {
		if in.EdgeMode != ModeExternalNAT {
			return db.FirewallHost{}, fmt.Errorf("%w: only a host behind a router follows a Pulse agent; a managed host detects its addresses itself", ErrInvalid)
		}
		if _, err := m.st.R.GetPulseAgent(ctx, in.AddressAgentID); err != nil {
			return db.FirewallHost{}, fmt.Errorf("%w: no Pulse agent %s", ErrInvalid, in.AddressAgentID)
		}
	}
	var cur db.FirewallHost
	if id != "" {
		if cur, err = m.st.R.GetFirewallHost(ctx, id); err != nil {
			return cur, ErrNotFound
		}
	}
	if in.EdgeMode == ModeManaged {
		// Editing a host keeps its pinned key; only a new one needs the key the admin confirmed.
		key := in.HostKey
		if len(key) == 0 && id != "" {
			key = cur.HostKey
		}
		if hk, err = sshx.ParseHostKey(key); err != nil {
			return db.FirewallHost{}, fmt.Errorf("%w: scan and confirm the host key first: %w", ErrInvalid, err)
		}
	}
	if in.RouteViaHostID != "" {
		if in.RouteViaHostID == id {
			return cur, fmt.Errorf("%w: a host cannot route through itself", ErrInvalid)
		}
		if _, err := m.st.R.GetFirewallHost(ctx, in.RouteViaHostID); err != nil {
			return cur, fmt.Errorf("%w: the entry host does not exist", ErrInvalid)
		}
	}
	needsCred := in.EdgeMode == ModeManaged && in.Auth != "hashi_key" && (id == "" || cur.Auth != in.Auth || !cur.CredentialSecretID.Valid)
	if needsCred && in.Credential.IsZero() {
		return cur, fmt.Errorf("%w: the %s is required", ErrInvalid, strings.ReplaceAll(in.Auth, "_", " "))
	}
	var recs []db.InsertSecretParams
	seal := func(purpose string, v secrets.Value, keep sql.NullString) (sql.NullString, error) {
		if v.IsZero() {
			return keep, nil
		}
		rec, err := m.vault.Seal(vault.Service, purpose, v)
		if err != nil {
			return sql.NullString{}, err
		}
		recs = append(recs, rec)
		return sql.NullString{String: rec.ID, Valid: true}, nil
	}
	cred, pass := sql.NullString{}, sql.NullString{}
	if in.EdgeMode == ModeManaged && in.Auth != "hashi_key" {
		if cred, err = seal(CredentialPurpose, in.Credential, cur.CredentialSecretID); err != nil {
			return cur, err
		}
		if in.Auth == "private_key" {
			if pass, err = seal(PassphrasePurpose, in.Passphrase, cur.PassphraseSecretID); err != nil {
				return cur, err
			}
		}
	}
	subnetsJSON, _ := json.Marshal(subnets)
	now := m.now().UnixMilli()
	isNew := id == ""
	if isNew {
		id = ulid.Make().String()
	}
	nullStr := func(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
	var dropAt sql.NullInt64
	if in.DropInbound {
		dropAt = sql.NullInt64{Int64: now, Valid: true}
		if cur.DropInboundAcceptedAt.Valid {
			dropAt = cur.DropInboundAcceptedAt
		}
	}
	// A new address, login or key is another way in (bug 24): the host is validated and confirmed again.
	moved := !isNew && (cur.SshHost != in.Host || cur.SshPort != int64(in.Port) || string(cur.HostKey) != string(hk.Key) ||
		cur.SshUser != in.User || cur.Auth != in.Auth || !in.Credential.IsZero() || !in.Passphrase.IsZero())
	err = m.st.Tx(ctx, func(q *db.Queries) error {
		for _, r := range recs {
			if err := q.InsertSecret(ctx, r); err != nil {
				return err
			}
		}
		if isNew {
			if err := q.InsertFirewallHost(ctx, db.InsertFirewallHostParams{ID: id, Name: in.Name, EdgeMode: in.EdgeMode,
				SshHost: in.Host, SshPort: int64(in.Port), SshUser: in.User, Auth: in.Auth, CredentialSecretID: cred,
				PassphraseSecretID: pass, HostKeyType: hk.Type, HostKey: hk.Key, HostKeyFingerprint: hk.Fingerprint,
				TraefikConnectionID: nullStr(in.TraefikConnectionID), InternalTraefikIp: traefikIP,
				RouteViaHostID: nullStr(in.RouteViaHostID), PublicIpOverride: pub, PublicIp6Override: pub6,
				WanIfaceOverride: strings.TrimSpace(in.WANOverride), SubnetsJson: string(subnetsJSON), NetbirdMode: in.NetBirdMode,
				NetbirdIface: in.NetBirdIface, NetbirdPort: int64(in.NetBirdPort), CreatedAt: now, UpdatedAt: now}); err != nil {
				if strings.Contains(err.Error(), "UNIQUE") {
					return fmt.Errorf("%w: another host is named %s", ErrInvalid, in.Name)
				}
				return err
			}
			if dropAt.Valid || in.AddressAgentID != "" { // the insert has no columns for these
				if err := q.UpdateFirewallHostSettings(ctx, settingsParams(id, in, traefikIP, pub, pub6, subnetsJSON, dropAt, now)); err != nil {
					return err
				}
			}
		} else {
			if err := q.UpdateFirewallHostSettings(ctx, settingsParams(id, in, traefikIP, pub, pub6, subnetsJSON, dropAt, now)); err != nil {
				return err
			}
			if moved {
				if in.EdgeMode != ModeManaged {
					return fmt.Errorf("%w: a host behind a router has no SSH login", ErrInvalid)
				}
				if err := q.UpdateFirewallHostAccess(ctx, db.UpdateFirewallHostAccessParams{SshHost: in.Host, SshPort: int64(in.Port),
					SshUser: in.User, Auth: in.Auth, CredentialSecretID: cred, PassphraseSecretID: pass, HostKeyType: hk.Type,
					HostKey: hk.Key, HostKeyFingerprint: hk.Fingerprint, UpdatedAt: now, ID: id}); err != nil {
					return err
				}
				if cur.HostKeyFingerprint != hk.Fingerprint { // another machine: its ruleset is read again
					if err := q.ClearFirewallHostSetupRuleset(ctx, id); err != nil {
						return err
					}
				}
				for _, old := range []sql.NullString{cur.CredentialSecretID, cur.PassphraseSecretID} {
					if old.Valid && old != cred && old != pass {
						if err := q.DeleteSecret(ctx, old.String); err != nil {
							return err
						}
					}
				}
			}
		}
		action := "firewall.host.updated"
		if isNew {
			action = "firewall.host.created"
		}
		if err := m.audit.Record(ctx, q, audit.Event{Actor: actor, Action: action, IP: ip, Target: "firewall_host:" + id,
			Outcome: audit.Success, Detail: map[string]any{"name": in.Name, "edge_mode": in.EdgeMode, "host": in.Host,
				"host_key": hk.Fingerprint, "subnets": subnets, "netbird": in.NetBirdMode, "drop_inbound": in.DropInbound}}); err != nil {
			return err
		}
		if err := followAgent(ctx, q, id, now); err != nil {
			return err
		}
		return m.ensureHashiHosts(ctx, q)
	})
	if err != nil {
		return db.FirewallHost{}, err
	}
	m.sets.Changed(ctx)
	return m.st.R.GetFirewallHost(ctx, id)
}

// FollowAgents copies each following host's Pulse agent addresses into its detected ones (bug 23, 05 §1.1): the
// public address the agent reached Hashi from, and its private addresses as the host's internal ones. It runs before
// the IP sets recompile after an agent's address change, so host items and host records follow.
func (m *Manager) FollowAgents(ctx context.Context) error {
	return m.st.Tx(ctx, func(q *db.Queries) error {
		rows, err := q.ListFirewallHostsFollowingAgents(ctx)
		if err != nil {
			return err
		}
		for _, r := range rows {
			if err := followAgent(ctx, q, r.ID, m.now().UnixMilli()); err != nil {
				return err
			}
		}
		return nil
	})
}

func followAgent(ctx context.Context, q *db.Queries, id string, now int64) error {
	h, err := q.GetFirewallHost(ctx, id)
	if err != nil || !h.AddressAgentID.Valid {
		return err
	}
	a, err := q.GetPulseAgent(ctx, h.AddressAgentID.String)
	if err != nil {
		return err
	}
	pub4, pub6 := "", ""
	if ip, err := netip.ParseAddr(a.PublicIp); err == nil {
		if ip.Unmap().Is4() {
			pub4 = ip.Unmap().String()
		} else {
			pub6 = ip.String()
		}
	}
	var cands []struct {
		Address string `json:"address"`
	}
	_ = json.Unmarshal([]byte(a.CandidatesJson), &cands) // written by the Pulse heartbeat
	internal := []string{}
	if a.SelectedIp != "" {
		internal = append(internal, a.SelectedIp)
	}
	for _, c := range cands {
		if c.Address != a.SelectedIp {
			internal = append(internal, c.Address)
		}
	}
	raw, _ := json.Marshal(internal)
	if h.PublicIp == pub4 && h.PublicIp6 == pub6 && h.InternalIpsJson == string(raw) {
		return nil
	}
	return q.SetFirewallHostAgentAddresses(ctx, db.SetFirewallHostAgentAddressesParams{PublicIp: pub4, PublicIp6: pub6,
		InternalIpsJson: string(raw), UpdatedAt: now, ID: id})
}

func settingsParams(id string, in HostInput, traefikIP, pub, pub6 string, subnets []byte, dropAt sql.NullInt64, now int64) db.UpdateFirewallHostSettingsParams {
	nullStr := func(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
	return db.UpdateFirewallHostSettingsParams{Name: in.Name, TraefikConnectionID: nullStr(in.TraefikConnectionID),
		InternalTraefikIp: traefikIP, RouteViaHostID: nullStr(in.RouteViaHostID), PublicIpOverride: pub,
		PublicIp6Override: pub6, WanIfaceOverride: strings.TrimSpace(in.WANOverride), SubnetsJson: string(subnets),
		NetbirdMode: in.NetBirdMode, NetbirdIface: in.NetBirdIface, NetbirdPort: int64(in.NetBirdPort),
		DropInboundAcceptedAt: dropAt, AddressAgentID: nullStr(in.AddressAgentID), UpdatedAt: now, ID: id}
}

// Delete removes a host that no IP set item references and no other host routes through (05a §2).
func (m *Manager) Delete(ctx context.Context, actor, ip, id string) error {
	err := m.st.Tx(ctx, func(q *db.Queries) error {
		h, err := q.GetFirewallHost(ctx, id)
		if err != nil {
			return ErrNotFound
		}
		hosts, err := q.ListFirewallHosts(ctx)
		if err != nil {
			return err
		}
		if len(hosts) == 1 {
			steps, err := q.ListSetupSteps(ctx)
			if err != nil {
				return err
			}
			for _, st := range steps {
				if st.Step == 11 && st.Status == "done" {
					return fmt.Errorf("%w: %s is the last firewall host; Hashi needs at least one (invariant 11): add another first", ErrInUse, h.Name)
				}
			}
		}
		for _, o := range hosts {
			if o.RouteViaHostID.String == id {
				return fmt.Errorf("%w: %s routes through %s", ErrInUse, o.Name, h.Name)
			}
		}
		if svcs, err := q.ListServicesByEntryHost(ctx, sql.NullString{String: id, Valid: true}); err != nil || len(svcs) > 0 {
			if err == nil {
				err = fmt.Errorf("%w: service %s enters through %s (routing override)", ErrInUse, svcs[0].Name, h.Name)
			}
			return err
		}
		// The hashi-hosts element is Hashi's own; every other reference blocks the delete.
		if err := q.DeleteFirewallHost(ctx, id); err != nil {
			return err
		}
		if err := m.ensureHashiHosts(ctx, q); err != nil {
			return err
		}
		if n, err := q.CountIPSetElementsForHost(ctx, sql.NullString{String: id, Valid: true}); err != nil || n > 0 {
			if err == nil {
				err = fmt.Errorf("%w: IP set items reference %s", ErrInUse, h.Name)
			}
			return err
		}
		return m.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "firewall.host.deleted", IP: ip,
			Target: "firewall_host:" + id, Outcome: audit.Success, Detail: map[string]any{"name": h.Name}})
	})
	if err == nil {
		m.sets.Changed(ctx)
	}
	return err
}

// ensureHashiHosts keeps the system set hashi-hosts at one host element (scope public) per registered host (05a §3).
func (m *Manager) ensureHashiHosts(ctx context.Context, q *db.Queries) error {
	set, err := q.GetIPSetBySystemKey(ctx, sql.NullString{String: ipsets.SystemHashiHosts, Valid: true})
	if err != nil {
		return err
	}
	hosts, err := q.ListFirewallHosts(ctx)
	if err != nil {
		return err
	}
	els, err := q.ListIPSetElements(ctx, set.ID)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, e := range els {
		if !slices.ContainsFunc(hosts, func(h db.FirewallHost) bool { return h.ID == e.HostID.String }) {
			if err := q.DeleteIPSetElement(ctx, e.ID); err != nil {
				return err
			}
			continue
		}
		have[e.HostID.String] = true
	}
	now := m.now().UnixMilli()
	for _, h := range hosts {
		if have[h.ID] {
			continue
		}
		pos, err := q.NextIPSetElementPosition(ctx, set.ID)
		if err != nil {
			return err
		}
		if err := q.InsertIPSetElement(ctx, db.InsertIPSetElementParams{ID: ulid.Make().String(), IpSetID: set.ID,
			Position: pos, Type: ipsets.TypeHost, Origin: "system", Locked: 1,
			HostID: sql.NullString{String: h.ID, Valid: true}, Scope: sql.NullString{String: ipsets.ScopePublic, Valid: true},
			Note: h.Name, CreatedBy: audit.System, CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
	}
	return nil
}

// Effective is a host's settings with overrides applied.
type Effective struct {
	Row       db.FirewallHost
	Facts     Facts
	WAN       string
	PublicIP  netip.Addr
	PublicIP6 netip.Addr
	Subnets   []netip.Prefix
	Internal  []netip.Addr
}

// Effective reads one host with its overrides applied.
func EffectiveOf(row db.FirewallHost) Effective {
	e := Effective{Row: row, WAN: row.WanIface}
	_ = json.Unmarshal([]byte(row.FactsJson), &e.Facts)
	_ = json.Unmarshal([]byte(row.SubnetsJson), &e.Subnets)
	var internal []string
	_ = json.Unmarshal([]byte(row.InternalIpsJson), &internal)
	for _, s := range internal {
		if a, err := netip.ParseAddr(s); err == nil {
			e.Internal = append(e.Internal, a)
		}
	}
	if row.WanIfaceOverride != "" {
		e.WAN = row.WanIfaceOverride
	}
	pick := func(override, detected string) netip.Addr {
		if override != "" {
			detected = override
		}
		a, _ := netip.ParseAddr(detected)
		return a
	}
	e.PublicIP, e.PublicIP6 = pick(row.PublicIpOverride, row.PublicIp), pick(row.PublicIp6Override, row.PublicIp6)
	return e
}

// hostAddrs answers host items of IP sets (05a §2): public = the public addresses, internal = NetBird and bridge
// addresses detected at validation.
func (m *Manager) hostAddrs(ctx context.Context, id, scope string) ([]netip.Addr, error) {
	row, err := m.st.R.GetFirewallHost(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("host %s: %w", id, ErrNotFound)
	}
	e := EffectiveOf(row)
	var out []netip.Addr
	if scope == ipsets.ScopePublic || scope == ipsets.ScopeAll {
		for _, a := range []netip.Addr{e.PublicIP, e.PublicIP6} {
			if a.IsValid() {
				out = append(out, a)
			}
		}
	}
	if scope == ipsets.ScopeInternal || scope == ipsets.ScopeAll {
		out = append(out, e.Internal...)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s has no %s address yet (validate it or set an override)", row.Name, scope)
	}
	return out, nil
}

// consumers lists the firewall positions that use a set (05a §5.1).
func (m *Manager) consumers(ctx context.Context, q *db.Queries, setID string) ([]ipsets.Consumer, error) {
	var out []ipsets.Consumer
	bs, err := q.ListFirewallBindings(ctx)
	if err != nil {
		return nil, err
	}
	for _, b := range bs {
		if b.IpSetID == setID {
			out = append(out, ipsets.Consumer{Type: "firewall_binding", ID: b.Position, Name: "Global firewall " + b.Position,
				Position: ipsets.PositionMatch})
		}
	}
	access, err := q.ListAllFirewallHostAccess(ctx)
	if err != nil {
		return nil, err
	}
	for _, a := range access {
		if a.IpSetID == setID {
			name := a.HostID
			if h, err := q.GetFirewallHost(ctx, a.HostID); err == nil {
				name = h.Name
			}
			out = append(out, ipsets.Consumer{Type: "firewall_host_access", ID: a.HostID, Name: "Host access on " + name,
				Position: ipsets.PositionMatch})
		}
	}
	return out, nil
}

// PutSetupAccess replaces the host's setup access set ("<name>-access") with addrs and binds it to the host (setup step 6).
func (m *Manager) PutSetupAccess(ctx context.Context, actor, ip, id, name string, addrs []string) error {
	items := make([]ipsets.Item, 0, len(addrs))
	for _, a := range addrs {
		it, err := ipsets.ParseIPOrCIDR(strings.TrimSpace(a))
		if err != nil {
			return fmt.Errorf("%w: %q is not an address or range", ErrInvalid, a)
		}
		items = append(items, it)
	}
	slug, now := name+"-access", m.now()
	err := m.st.Tx(ctx, func(q *db.Queries) error {
		g, err := ipsets.LoadGraph(ctx, q)
		if err != nil {
			return err
		}
		ed := ipsets.Editor{Sources: m.sets.Sources()}
		setID := ""
		for sid, set := range g {
			if set.Slug != slug {
				continue
			}
			setID = sid
			for _, e := range set.Elements {
				if _, err := ed.DeleteElement(ctx, q, sid, e.ID, now); err != nil {
					return err
				}
			}
		}
		if setID == "" {
			set, err := ipsets.CreateSet(ctx, q, ipsets.SetInput{Slug: slug, Name: "Access to " + name}, now)
			if err != nil {
				return err
			}
			setID = set.ID
		}
		for _, it := range items {
			if _, err := ed.AddElement(ctx, q, setID, ipsets.ElementInput{Item: it}, actor, now); err != nil {
				return err
			}
		}
		if err := q.DeleteFirewallHostAccess(ctx, id); err != nil {
			return err
		}
		if err := q.InsertFirewallHostAccess(ctx, db.InsertFirewallHostAccessParams{HostID: id, IpSetID: setID, CreatedAt: now.UnixMilli()}); err != nil {
			return err
		}
		return m.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "firewall.host_access.updated", IP: ip, Target: "firewall_host:" + id,
			Outcome: audit.Success, Detail: map[string]any{"addresses": addrs}})
	})
	if err == nil {
		m.sets.Changed(ctx)
	}
	return err
}

// PutTakeover stores what Hashi removes of a host's old firewall script (05 §3.4); the next plan lists it.
func (m *Manager) PutTakeover(ctx context.Context, actor, ip, id string, t Takeover) error {
	if err := t.Validate(); err != nil {
		return err
	}
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	return m.st.Tx(ctx, func(q *db.Queries) error {
		if _, err := q.GetFirewallHost(ctx, id); err != nil {
			return ErrNotFound
		}
		if err := q.SetFirewallHostTakeover(ctx, db.SetFirewallHostTakeoverParams{TakeoverJson: string(b), UpdatedAt: m.now().UnixMilli(), ID: id}); err != nil {
			return err
		}
		return m.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "firewall.takeover.updated", IP: ip,
			Target: "firewall_host:" + id, Outcome: audit.Success, Detail: map[string]any{"takeover": t}})
	})
}

// CheckStreamPort refuses a public port that belongs to something on a firewall host (05 §8): NetBird's WireGuard port,
// Hashi's SSH path or any listener the last validation saw. DNAT would take that traffic away from it.
func (m *Manager) CheckStreamPort(ctx context.Context, proto string, port int) error {
	rows, err := m.st.R.ListFirewallHosts(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.EdgeMode != ModeManaged {
			continue
		}
		if proto == "udp" && r.NetbirdMode != NetBirdDisabled && int(r.NetbirdPort) == port {
			return fmt.Errorf("%w: %d/udp is NetBird's WireGuard port on %s", ErrInvalid, port, r.Name)
		}
		if proto == "tcp" && int(r.SshPort) == port {
			return fmt.Errorf("%w: %d/tcp is the SSH port of %s", ErrInvalid, port, r.Name)
		}
		for _, l := range EffectiveOf(r).Facts.Listeners {
			if l.Proto == proto && l.Port == port && !l.Loopback && l.Process != "traefik" {
				return fmt.Errorf("%w: %s already listens on %d/%s", ErrInvalid, r.Name, port, proto)
			}
		}
	}
	return nil
}
