package firewall

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"slices"

	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/words"
)

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
	engine, err := settings.Get[settings.Engine](ctx, m.st.R)
	if err != nil {
		return row, Desired{}, err
	}
	fwDefaults, err := settings.Get[settings.FirewallDefaults](ctx, m.st.R)
	if err != nil {
		return row, Desired{}, err
	}
	traefikIP, _ := netip.ParseAddr(row.InternalTraefikIp)
	spec := Spec{WAN: e.WAN, PublicIP: e.PublicIP, Subnets: e.Subnets, TraefikIP: traefikIP, Ports: ports, Folded: folded,
		HostAccess: hostAccess, NetBird: NetBird{Mode: row.NetbirdMode, Iface: row.NetbirdIface, Port: int(row.NetbirdPort)},
		DropOtherInbound: row.DropInboundAcceptedAt.Valid, Meters: MetersFrom(engine), NoMSSClamp: !fwDefaults.MSSClamp,
		Narrow: narrowIfaces(e.Facts, e.WAN, e.Subnets)}
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
		return checkManagement(ctx, c)
	}
}

var managementURL = regexp.MustCompile(`(?m)^Management: Connected to https?://([A-Za-z0-9.-]+):(\d+)\s*$`)

// checkManagement proves the host can still open a connection to NetBird's management server. "Connected" alone only
// says the running daemon has not noticed yet: a firewall change that cuts the path shows at its next restart (found on
// the first real host, whose management server sits behind its own public address).
func checkManagement(ctx context.Context, c Conn) error {
	r, err := c.Run(ctx, "netbird status -d", nil)
	if err != nil {
		return err
	}
	m := managementURL.FindStringSubmatch(string(r.Stdout))
	if m == nil {
		return nil // an older or differently worded status: nothing to test
	}
	r, err = c.Run(ctx, `timeout 5 bash -c 'exec 3<>/dev/tcp/`+m[1]+`/`+m[2]+`'`, nil)
	if err != nil {
		return err
	}
	if !r.OK() {
		return fmt.Errorf("this host can no longer reach NetBird's management server %s:%s", m[1], m[2])
	}
	return nil
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
	summary := fmt.Sprintf("%s applied on %s; table verified", words.Count(len(items), "change", "changes"), row.Name)
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

// sshConsumers lists the hosts reached through set id.
func sshConsumers(ctx context.Context, q *db.Queries, id string) ([]ipsets.Consumer, error) {
	rows, err := q.ListFirewallHostsBySSHTarget(ctx, sql.NullString{String: id, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]ipsets.Consumer, 0, len(rows))
	for _, r := range rows {
		out = append(out, ipsets.Consumer{Type: "firewall_ssh", ID: r.ID, Name: "SSH to " + r.Name, Position: ipsets.PositionTarget})
	}
	return out, nil
}

// narrowIfaces are the interfaces holding a managed subnet with an MTU below the WAN interface's (bug 29).
func narrowIfaces(f Facts, wan string, subnets []netip.Prefix) []IfaceMTU {
	wanMTU := 1500
	for _, i := range f.Ifaces {
		if i.Name == wan && i.MTU > 0 {
			wanMTU = i.MTU
		}
	}
	var out []IfaceMTU
	for _, i := range f.Ifaces {
		if i.Name == wan || i.MTU < 1280 || i.MTU >= wanMTU {
			continue
		}
		if slices.ContainsFunc(i.Addrs, func(a netip.Prefix) bool {
			return slices.ContainsFunc(subnets, func(n netip.Prefix) bool { return n.Contains(a.Addr()) })
		}) {
			out = append(out, IfaceMTU{Name: i.Name, MTU: i.MTU})
		}
	}
	return out
}
