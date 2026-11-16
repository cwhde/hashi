package firewall

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/ipsets"
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
	// sshTargets resolves a host's SSH address set (08 §5); nil uses the literal address.
	sshTargets services.TargetResolver
}

// UseSSHTargets wires SSH address sets.
func (m *Manager) UseSSHTargets(r services.TargetResolver) { m.sshTargets = r }

// SetSSHTarget makes the host reachable through an address set, or back through its literal address with "". A set that
// resolves to nothing or does not answer is refused unless saveInactive says to keep it (08 §5).
func (m *Manager) SetSSHTarget(ctx context.Context, id, setID string, saveInactive bool) error {
	row, err := m.st.R.GetFirewallHost(ctx, id)
	if err != nil {
		return ErrNotFound
	}
	if setID != "" {
		if err := ipsets.CheckTarget(ctx, m.st.R, setID); err != nil {
			return err
		}
		if !saveInactive {
			silent, err := ipsets.SilentAgents(ctx, m.st.R, setID)
			if err != nil {
				return err
			}
			if len(silent) > 0 {
				return fmt.Errorf("%w: Pulse agent %s has not sent a heartbeat yet", sshx.ErrTargetDown, strings.Join(silent, ", "))
			}
		}
		if !saveInactive && m.sshTargets != nil {
			if err := sshx.ProbeTarget(ctx, m.sshTargets, "firewall_ssh:"+id, setID, int(row.SshPort)); err != nil {
				return err
			}
		}
	}
	return m.st.Q.SetFirewallHostSSHTarget(ctx, db.SetFirewallHostSSHTargetParams{SshTargetSetID: sql.NullString{String: setID, Valid: setID != ""}, ID: id})
}

// ReconnectUsing retests the hosts reached through changed sets (08 §4 step 5).
func (m *Manager) ReconnectUsing(ctx context.Context, changed []string) {
	for _, set := range changed {
		rows, err := m.st.R.ListFirewallHostsBySSHTarget(ctx, sql.NullString{String: set, Valid: true})
		if err != nil {
			continue
		}
		for _, r := range rows {
			row, err := m.st.R.GetFirewallHost(ctx, r.ID)
			if err != nil {
				continue
			}
			_ = m.st.Q.SetFirewallHostHealth(ctx, db.SetFirewallHostHealthParams{Health: "validating", LastValidationError: row.LastValidationError, UpdatedAt: m.now().UnixMilli(), ID: r.ID})
			c, err := m.dial(ctx, row)
			if err == nil {
				_ = c.Close()
			}
			health, msg := sshx.AfterReconnect(row.Health, row.LastValidationError, err)
			_ = m.st.Q.SetFirewallHostHealth(ctx, db.SetFirewallHostHealthParams{Health: health, LastValidationError: msg, UpdatedAt: m.now().UnixMilli(), ID: r.ID})
		}
	}
}

// sshAddrs lists where to reach a host, in order: its address set, then its literal address. The pinned host key is checked on each.
func (m *Manager) sshAddrs(ctx context.Context, row db.FirewallHost) []string {
	var out []string
	if row.SshTargetSetID.Valid && m.sshTargets != nil {
		if t, err := m.sshTargets(ctx, "firewall_ssh:"+row.ID, row.SshTargetSetID.String); err == nil {
			for _, a := range t.Addresses {
				out = append(out, sshx.Addr(a.String(), int(row.SshPort)))
			}
		}
	}
	if lit := sshx.Addr(row.SshHost, int(row.SshPort)); !slices.Contains(out, lit) {
		out = append(out, lit)
	}
	return out
}

// NewManager returns the manager and wires host items of IP sets to it (05a §2 `host`).
func NewManager(st *store.Store, v *vault.Vault, a *audit.Log, sets *ipsets.Service, connect Connector) *Manager {
	m := &Manager{st: st, vault: v, audit: a, sets: sets, connect: connect, scan: sshx.Scan, now: time.Now}
	sets.SetHosts(m.hostAddrs, func(ctx context.Context, q *db.Queries, id string) (string, error) {
		h, err := q.GetFirewallHost(ctx, id)
		return h.Name, err
	})
	sets.AddConsumers(m.consumers)
	sets.AddConsumers(sshConsumers)
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
