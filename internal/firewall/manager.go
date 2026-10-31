package firewall

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// Subsystem is the sync subsystem (sync_plans.subsystem); the plan target is the host ID.
const Subsystem = "firewall"

// Vault purposes. Hashi's own SSH key is the one the Traefik hosts use (traefik.IdentityPurpose).
const (
	IdentityPurpose   = "ssh.identity"
	CredentialPurpose = "firewall.ssh.credential" //nolint:gosec // a vault purpose name, not a credential
	PassphrasePurpose = "firewall.ssh.passphrase" //nolint:gosec // a vault purpose name, not a credential
)

// Edge modes (05 §1.1).
const (
	ModeManaged     = "managed"
	ModeExternalNAT = "external_nat"
)

// Errors the API maps.
var (
	ErrInvalid  = errors.New("invalid firewall host")
	ErrNotReady = errors.New("the firewall host is not ready to plan")
	ErrNotFound = errors.New("firewall host not found")
	ErrInUse    = errors.New("still in use")
)

// Conn is an open SSH connection to a host.
type Conn interface {
	Host
	Close() error
}

// Connector opens an SSH connection (sshx.Dial in production, a fake in tests and the dev build).
type Connector func(ctx context.Context, c sshx.Config) (Conn, error)

// Manager plans and applies firewall hosts through the sync engine (invariant 7).
type Manager struct {
	st      *store.Store
	vault   *vault.Vault
	audit   *audit.Log
	sets    *ipsets.Service
	connect Connector
	scan    func(ctx context.Context, addr string, timeout time.Duration) (sshx.HostKey, error)
	now     func() time.Time
}

// NewManager returns the manager and wires host items of IP sets to it (05a §2 `host`).
func NewManager(st *store.Store, v *vault.Vault, a *audit.Log, sets *ipsets.Service, connect Connector) *Manager {
	m := &Manager{st: st, vault: v, audit: a, sets: sets, connect: connect, scan: sshx.Scan, now: time.Now}
	sets.SetHosts(m.hostAddrs, func(ctx context.Context, q *db.Queries, id string) (string, error) {
		h, err := q.GetFirewallHost(ctx, id)
		return h.Name, err
	})
	sets.AddConsumers(m.consumers)
	return m
}

// InstallKey adds Hashi's key to a host with a credential given once (never stored), at the admin's request.
func (m *Manager) InstallKey(ctx context.Context, actor, ip string, c sshx.Config, line string) error {
	rec, err := m.st.R.GetSecretByPurpose(ctx, IdentityPurpose)
	if err != nil {
		return fmt.Errorf("the Hashi SSH key: %w", err)
	}
	own, err := m.vault.Open(rec)
	if err != nil {
		return err
	}
	err = sshx.InstallKey(ctx, func(ctx context.Context, c sshx.Config) (sshx.Runner, error) { return m.connect(ctx, c) }, c, line,
		sshx.Auth{User: c.Auth.User, PrivateKey: own})
	if err != nil && !errors.Is(err, sshx.ErrKeyAdded) {
		return err
	}
	return errors.Join(err, m.audit.Record(ctx, m.st.Q, audit.Event{Actor: actor, Action: "ssh.key.installed", IP: ip, Target: "host:" + c.Addr,
		Outcome: audit.Success, Detail: map[string]any{"user": c.Auth.User, "host_key": c.Pinned.Fingerprint, "login_proved": err == nil}}))
}

// UseFakes replaces SSH and host-key scanning (tests and the dev build).
func (m *Manager) UseFakes(connect Connector, scan func(ctx context.Context, addr string, timeout time.Duration) (sshx.HostKey, error)) {
	m.connect, m.scan = connect, scan
}

// ScanHostKey reads a host's SSH key for the admin to compare before Hashi pins it.
func (m *Manager) ScanHostKey(ctx context.Context, host string, port int) (sshx.HostKey, error) {
	return m.scan(ctx, sshx.Addr(host, port), 10*time.Second)
}

// Subsystems returns the sync subsystem to register with the engine.
func (m *Manager) Subsystems() []hsync.Subsystem { return []hsync.Subsystem{fwSub{m}} }

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
}

var labelRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// ReservedNames cannot be host names: they are the structural labels of host records (03 §5.1).
var ReservedNames = []string{"on", "via", "ovr"}

func parseSubnets(ss []string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, s := range ss {
		p, err := netip.ParsePrefix(strings.TrimSpace(s))
		if err != nil {
			return nil, fmt.Errorf("%w: %q is not a subnet (e.g. 10.0.4.0/24)", ErrInvalid, s)
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
	// The login is part of a host's identity here: the update has no columns for it, and saving must not pretend.
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
			if dropAt.Valid {
				if err := q.UpdateFirewallHostSettings(ctx, settingsParams(id, in, traefikIP, pub, pub6, subnetsJSON, dropAt, now)); err != nil {
					return err
				}
			}
		} else {
			if err := q.UpdateFirewallHostSettings(ctx, settingsParams(id, in, traefikIP, pub, pub6, subnetsJSON, dropAt, now)); err != nil {
				return err
			}
			if moved {
				return fmt.Errorf("%w: the SSH address, login or host key changed: delete the host and add it again", ErrInvalid)
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
		return m.ensureHashiHosts(ctx, q)
	})
	if err != nil {
		return db.FirewallHost{}, err
	}
	m.sets.Changed(ctx)
	return m.st.R.GetFirewallHost(ctx, id)
}

func settingsParams(id string, in HostInput, traefikIP, pub, pub6 string, subnets []byte, dropAt sql.NullInt64, now int64) db.UpdateFirewallHostSettingsParams {
	nullStr := func(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }
	return db.UpdateFirewallHostSettingsParams{Name: in.Name, TraefikConnectionID: nullStr(in.TraefikConnectionID),
		InternalTraefikIp: traefikIP, RouteViaHostID: nullStr(in.RouteViaHostID), PublicIpOverride: pub,
		PublicIp6Override: pub6, WanIfaceOverride: strings.TrimSpace(in.WANOverride), SubnetsJson: string(subnets),
		NetbirdMode: in.NetBirdMode, NetbirdIface: in.NetBirdIface, NetbirdPort: int64(in.NetBirdPort),
		DropInboundAcceptedAt: dropAt, UpdatedAt: now, ID: id}
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

// ---- SSH ----

func (m *Manager) secret(ctx context.Context, id string) (secrets.Value, error) {
	rec, err := m.st.R.GetSecret(ctx, id)
	if err != nil {
		return secrets.Value{}, err
	}
	return m.vault.Open(rec)
}

func (m *Manager) auth(ctx context.Context, row db.FirewallHost) (sshx.Auth, error) {
	a := sshx.Auth{User: row.SshUser}
	switch row.Auth {
	case "hashi_key":
		rec, err := m.st.R.GetSecretByPurpose(ctx, IdentityPurpose)
		if err != nil {
			return a, fmt.Errorf("the Hashi SSH key (create it on the Traefik setup step): %w", err)
		}
		k, err := m.vault.Open(rec)
		if err != nil {
			return a, err
		}
		a.PrivateKey = k
	case "private_key", "password":
		v, err := m.secret(ctx, row.CredentialSecretID.String)
		if err != nil {
			return a, fmt.Errorf("SSH credential: %w", err)
		}
		if row.Auth == "password" {
			a.Password = v
			break
		}
		a.PrivateKey = v
		if row.PassphraseSecretID.Valid {
			if a.Passphrase, err = m.secret(ctx, row.PassphraseSecretID.String); err != nil {
				return a, fmt.Errorf("SSH key passphrase: %w", err)
			}
		}
	default:
		return a, fmt.Errorf("unknown auth %q", row.Auth)
	}
	return a, nil
}

func (m *Manager) dial(ctx context.Context, row db.FirewallHost) (Conn, error) {
	a, err := m.auth(ctx, row)
	if err != nil {
		return nil, err
	}
	hk, err := sshx.ParseHostKey(row.HostKey)
	if err != nil {
		return nil, err
	}
	addr := sshx.Addr(row.SshHost, int(row.SshPort))
	c, err := m.connect(ctx, sshx.Config{Addr: addr, Auth: a, Pinned: hk, Timeout: 15 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("connect to %s (%s): %w", row.Name, addr, err)
	}
	return c, nil
}

// ---- plan / apply ----

// desired builds what the host should hold.
func (m *Manager) desired(ctx context.Context, id string) (db.FirewallHost, Desired, error) {
	return m.build(ctx, id, true)
}

// build is desired; preview skips the ownership confirmation (the rendered tab before the first apply).
func (m *Manager) build(ctx context.Context, id string, owned bool) (db.FirewallHost, Desired, error) {
	row, err := m.st.R.GetFirewallHost(ctx, id)
	if err != nil {
		return row, Desired{}, ErrNotFound
	}
	if row.EdgeMode != ModeManaged {
		return row, Desired{}, fmt.Errorf("%w: %s is an external_nat host: Hashi writes no firewall there", ErrNotReady, row.Name)
	}
	if owned && !row.OwnershipConfirmedAt.Valid {
		return row, Desired{}, fmt.Errorf("%w: confirm that Hashi manages %s's firewall first", ErrNotReady, row.Name)
	}
	e := EffectiveOf(row)
	if e.Facts.NftPath == "" {
		return row, Desired{}, fmt.Errorf("%w: validate %s first", ErrNotReady, row.Name)
	}
	folded, _, blocks, err := m.Folded(ctx, m.st.R)
	if err != nil {
		return row, Desired{}, err
	}
	g, err := ipsets.LoadGraph(ctx, m.st.R)
	if err != nil {
		return row, Desired{}, err
	}
	access, err := m.st.R.ListFirewallHostAccess(ctx, id)
	if err != nil {
		return row, Desired{}, err
	}
	var hostAccess []netip.Prefix
	for _, a := range access {
		res := g.Compile(a.IpSetID, m.sets.Resolver(ctx), m.now())
		hostAccess = append(append(hostAccess, res.V4...), res.V6...)
	}
	ports, err := m.ports(ctx)
	if err != nil {
		return row, Desired{}, err
	}
	// A stream saved before this host's NetBird port was known must not stop the host's plans: only that port waits.
	ports = slices.DeleteFunc(ports, func(p Port) bool {
		return row.NetbirdMode != NetBirdDisabled && p.Proto == "udp" && p.Port == int(row.NetbirdPort)
	})
	traefikIP, _ := netip.ParseAddr(row.InternalTraefikIp)
	spec := Spec{WAN: e.WAN, PublicIP: e.PublicIP, Subnets: e.Subnets, TraefikIP: traefikIP, Ports: ports, Folded: folded,
		HostAccess: hostAccess, NetBird: NetBird{Mode: row.NetbirdMode, Iface: row.NetbirdIface, Port: int(row.NetbirdPort)},
		DropOtherInbound: row.DropInboundAcceptedAt.Valid, Meters: DefaultMeters}
	table, err := Render(spec)
	if err != nil {
		return row, Desired{}, fmt.Errorf("%w: %w", ErrNotReady, err)
	}
	guard := Guard{RescueIP: e.Facts.SSHFrom, Blocks: blocks}
	sessions, err := m.st.R.ListActiveSessions(ctx, m.now().UnixMilli())
	if err != nil {
		return row, Desired{}, err
	}
	for _, s := range sessions {
		if a, err := netip.ParseAddr(s.BoundIp); err == nil {
			guard.AdminIPs = append(guard.AdminIPs, a)
		}
	}
	takeover, err := ParseTakeover(row.TakeoverJson)
	if err != nil {
		return row, Desired{}, fmt.Errorf("stored takeover of %s: %w", row.Name, err)
	}
	return row, Desired{Spec: spec, Table: table, NftPath: e.Facts.NftPath, IPv6: false, Guard: guard, Takeover: takeover}, nil
}

// ports are the published ports: 80 and 443 to Traefik's web/websecure entry points while any HTTP-family service is
// served (05 §8). TCP/UDP stream ports join with stream services.
func (m *Manager) ports(ctx context.Context) ([]Port, error) {
	all, err := services.List(ctx, m.st.R)
	if err != nil {
		return nil, err
	}
	// Same view as Traefik's render: a stream that never resolved has no entry point, so no port either.
	if all, err = services.Resolve(ctx, all, m.sets.ResolveTarget); err != nil {
		return nil, err
	}
	var out []Port
	if slices.ContainsFunc(all, func(s services.Service) bool { return s.Renderable() && !s.Stream() }) {
		out = append(out, Port{Proto: "tcp", Port: 80, TargetPort: 80}, Port{Proto: "tcp", Port: 443, TargetPort: 443})
	}
	// Streams: the public port DNATs to the same port on Traefik, its tcp-<port>/udp-<port> entry point (05 §3.2,
	// 04 §3); UDP stream ports are what the UDP meters watch.
	for _, s := range all {
		if s.Stream() && s.Renderable() {
			out = append(out, Port{Proto: s.Kind, Port: s.PublicPort, TargetPort: s.PublicPort, Metered: s.Kind == services.KindUDP})
		}
	}
	return out, nil
}

// Rescue returns the post-apply check: a new SSH connection to the host runs a command, and NetBird (when enabled)
// is still connected.
func (m *Manager) rescue(row db.FirewallHost) Rescue {
	return func(ctx context.Context) error {
		c, err := m.dial(ctx, row)
		if err != nil {
			return err
		}
		defer func() { _ = c.Close() }()
		r, err := c.Run(ctx, "true", nil)
		if err != nil {
			return fmt.Errorf("a command over the new connection: %w", err)
		}
		if !r.OK() {
			return fmt.Errorf("a command over the new connection exited %d", r.ExitCode)
		}
		if row.NetbirdMode == NetBirdDisabled {
			return nil
		}
		r, err = c.Run(ctx, "netbird status --json", nil)
		if err != nil {
			return err
		}
		nb, err := parseNetBird(string(r.Stdout), "", nil)
		if err != nil {
			return err
		}
		if !nb.Connected {
			return errors.New("NetBird is no longer connected")
		}
		return nil
	}
}

type fwSub struct{ m *Manager }

func (fwSub) Name() string                 { return Subsystem }
func (fwSub) LockKey(target string) string { return "firewall:" + target }

func (s fwSub) Plan(ctx context.Context, id string) (hsync.Draft, error) {
	row, d, err := s.m.desired(ctx, id)
	if err != nil {
		return hsync.Draft{}, err
	}
	c, err := s.m.dial(ctx, row)
	if err != nil {
		return hsync.Draft{}, err
	}
	defer func() { _ = c.Close() }()
	diff, err := Observe(ctx, c, d)
	if err != nil {
		return hsync.Draft{}, err
	}
	return hsync.Draft{Title: "Firewall on " + row.Name, Items: diff.Items()}, nil
}

func (s fwSub) Apply(ctx context.Context, id string, items []hsync.Item) (string, error) {
	m := s.m
	row, d, err := m.desired(ctx, id)
	if err != nil {
		return "", err
	}
	c, err := m.dial(ctx, row)
	if err != nil {
		return "", err
	}
	defer func() { _ = c.Close() }()
	diff, err := Observe(ctx, c, d)
	if err != nil {
		return "", err
	}
	if hsync.Hash(diff.Items()) != hsync.Hash(items) {
		return "", hsync.ErrReplanned
	}
	if err := Apply(ctx, c, d, diff, m.rescue(row)); err != nil {
		_ = m.st.Q.SetFirewallHostHealth(ctx, db.SetFirewallHostHealthParams{Health: "degraded", LastValidationError: err.Error(),
			UpdatedAt: m.now().UnixMilli(), ID: id})
		return "", err
	}
	now := m.now().UnixMilli()
	if err := m.st.Q.SetFirewallHostApplied(ctx, db.SetFirewallHostAppliedParams{LastAppliedHash: sha(d.Table),
		LastAppliedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: id}); err != nil {
		return "", err
	}
	summary := fmt.Sprintf("%d change(s) applied on %s; table verified", len(items), row.Name)
	if diff.Risk == hsync.High {
		summary += "; confirmed over a new SSH connection, rescue timer disarmed"
	}
	return summary, nil
}

// PlanAll plans every managed host whose ownership is confirmed (after a binding, set or service change); low-risk
// plans apply, high-risk ones wait for approval.
func (m *Manager) PlanAll(ctx context.Context, eng *hsync.Engine, actor string) ([]*hsync.Plan, error) {
	hosts, err := m.st.R.ListFirewallHosts(ctx)
	if err != nil {
		return nil, err
	}
	var out []*hsync.Plan
	var errs []error
	for _, h := range hosts {
		if h.EdgeMode != ModeManaged || !h.OwnershipConfirmedAt.Valid {
			continue
		}
		p, err := eng.Auto(ctx, Subsystem, h.ID, hsync.TriggerReconcile, actor)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", h.Name, err))
			continue
		}
		if p != nil {
			out = append(out, p)
		}
	}
	return out, errors.Join(errs...)
}

// Rendered is the table Hashi would load on the host now.
func (m *Manager) Rendered(ctx context.Context, id string) (Desired, error) {
	_, d, err := m.build(ctx, id, false)
	return d, err
}

// Ruleset reads everything on the host now (all nft tables, iptables in every backend, ipsets); read-only.
func (m *Manager) Ruleset(ctx context.Context, id string) (now, atSetup string, err error) {
	row, err := m.st.R.GetFirewallHost(ctx, id)
	if err != nil {
		return "", "", ErrNotFound
	}
	if row.EdgeMode != ModeManaged {
		return "", row.SetupRuleset, nil
	}
	c, err := m.dial(ctx, row)
	if err != nil {
		return "", row.SetupRuleset, err
	}
	defer func() { _ = c.Close() }()
	r, err := c.Run(ctx, SetupRulesetCommand, nil)
	if err != nil {
		return "", row.SetupRuleset, err
	}
	return string(r.Stdout), row.SetupRuleset, nil
}
