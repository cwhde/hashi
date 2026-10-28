package traefik

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// Subsystem is the sync subsystem (sync_plans.subsystem): install, files and certificate distribution per Traefik
// connection; the plan target is the connection ID.
const Subsystem = "traefik"

// Vault purposes.
const (
	IdentityPurpose   = "ssh.identity"           // Hashi's own SSH key
	CredentialPurpose = "traefik.ssh.credential" //nolint:gosec // a vault purpose name, not a credential
	PassphrasePurpose = "traefik.ssh.passphrase" //nolint:gosec // a vault purpose name, not a credential
)

// Conn is an open SSH connection to a host.
type Conn interface {
	Host
	Close() error
}

// Connector opens an SSH connection (sshx.Dial in production, a fake in tests).
type Connector func(ctx context.Context, c sshx.Config) (Conn, error)

// BinarySource returns the verified Traefik binary for an arch (Download in production).
type BinarySource func(ctx context.Context, arch string) ([]byte, error)

// Errors the API maps.
var (
	ErrNotReady = errors.New("the Traefik connection is not ready to plan")
)

// Discovery is what validation found on the host before Hashi took over (traefik_connections.discovery_json).
type Discovery struct {
	ExistingBinary  string   `json:"existing_binary,omitempty"`
	ExistingConfigs []string `json:"existing_configs,omitempty"`
	// BackedUp are the ExistingConfigs already copied to StateDir/backup (04 §2 step 5: once, at takeover).
	BackedUp []string `json:"backed_up,omitempty"`
}

// pendingBackup is what still needs a backup before Hashi takes over.
func (d Discovery) pendingBackup() []string {
	var out []string
	for _, c := range d.ExistingConfigs {
		if !slices.Contains(d.BackedUp, c) {
			out = append(out, c)
		}
	}
	return out
}

// Manager plans and applies Traefik hosts through the sync engine (invariant 7: it writes only from an approved plan).
type Manager struct {
	st      *store.Store
	vault   *vault.Vault
	audit   *audit.Log
	connect Connector
	binary  BinarySource
	now     func() time.Time
	// pingTimeout overrides PingTimeout (tests).
	pingTimeout time.Duration
	// tlsDial reaches an edge's websecure port for the certificate check; tlsTimeout bounds the wait for Traefik to
	// serve a new certificate.
	tlsDial    func(ctx context.Context, addr string) (net.Conn, error)
	tlsTimeout time.Duration
	// scan reads a host's SSH key before it is pinned (sshx.Scan).
	scan func(ctx context.Context, addr string, timeout time.Duration) (sshx.HostKey, error)
}

// UseFakes replaces SSH, the release download, host-key scanning and the TLS check (the dev build's in-memory edge;
// never in production builds).
func (m *Manager) UseFakes(connect Connector, binary BinarySource,
	scan func(ctx context.Context, addr string, timeout time.Duration) (sshx.HostKey, error),
	tlsDial func(ctx context.Context, addr string) (net.Conn, error)) {
	m.connect, m.binary, m.scan, m.tlsDial = connect, binary, scan, tlsDial
}

// ScanHostKey reads the host's SSH key for the admin to compare with the host before Hashi pins it.
func (m *Manager) ScanHostKey(ctx context.Context, host string, port int) (sshx.HostKey, error) {
	return m.scan(ctx, sshx.Addr(host, port), 10*time.Second)
}

// NewManager returns the Traefik manager.
func NewManager(st *store.Store, v *vault.Vault, a *audit.Log, connect Connector, binary BinarySource) *Manager {
	return &Manager{st: st, vault: v, audit: a, connect: connect, binary: binary, now: time.Now, tlsTimeout: 30 * time.Second, scan: sshx.Scan,
		tlsDial: func(ctx context.Context, addr string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "tcp", addr)
		}}
}

// Subsystems returns the sync subsystems to register with the engine.
func (m *Manager) Subsystems() []hsync.Subsystem { return []hsync.Subsystem{traefikSub{m}} }

type traefikSub struct{ m *Manager }

func (traefikSub) Name() string                 { return Subsystem }
func (traefikSub) LockKey(target string) string { return "traefik:" + target }

func (s traefikSub) Plan(ctx context.Context, connID string) (hsync.Draft, error) {
	row, d, err := s.m.desired(ctx, connID)
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
	return hsync.Draft{Title: "Traefik on " + row.Name, Items: diff.Items()}, nil
}

func (s traefikSub) Apply(ctx context.Context, connID string, items []hsync.Item) (string, error) {
	m := s.m
	row, d, err := m.desired(ctx, connID)
	if err != nil {
		return "", err
	}
	c, err := m.dial(ctx, row)
	if err != nil {
		return "", m.health(ctx, connID, err)
	}
	defer func() { _ = c.Close() }()
	diff, err := Observe(ctx, c, d)
	if err != nil {
		return "", m.health(ctx, connID, err)
	}
	if hsync.Hash(diff.Items()) != hsync.Hash(items) {
		return "", hsync.ErrReplanned
	}
	out, err := Apply(ctx, c, d, diff, m.now())
	if err == nil {
		err = Verify(ctx, c, d.APIPort, d.Files)
	}
	filesOK := err == nil
	var checks []CertCheck
	if filesOK && len(d.Certs) > 0 {
		checks = VerifyServed(ctx, m.dialer(row), d.Certs, m.tlsTimeout)
		for _, chk := range checks {
			err = errors.Join(err, chk.Err)
		}
	}
	now := m.now().UnixMilli()
	// Files that were written before a failure are on the host: record them so the next plan's "before" is right.
	if serr := m.st.Tx(ctx, func(q *db.Queries) error {
		record := out.Written
		if filesOK {
			record = d.Files // every desired file now matches the host
		}
		for _, f := range record {
			if f.Kind == KindCert && f.Secret {
				continue // key hashes are not needed to plan and stay out of the database
			}
			if e := q.UpsertTraefikFile(ctx, db.UpsertTraefikFileParams{ConnectionID: connID, Path: f.Path,
				LastAppliedHash: f.Hash(), AppliedAt: now}); e != nil {
				return e
			}
		}
		if !filesOK {
			return nil
		}
		// The host now runs the pinned version, whether this apply uploaded it or found it there.
		if e := q.SetTraefikInstalledVersion(ctx, db.SetTraefikInstalledVersionParams{InstalledVersion: Version, UpdatedAt: now, ID: connID}); e != nil {
			return e
		}
		if len(diff.Backup) > 0 {
			var disc Discovery
			_ = json.Unmarshal([]byte(row.DiscoveryJson), &disc)
			disc.BackedUp = append(disc.BackedUp, diff.Backup...)
			b, _ := json.Marshal(disc)
			if e := q.SetTraefikConnectionDetected(ctx, db.SetTraefikConnectionDetectedParams{OsFamily: row.OsFamily,
				PackageManager: row.PackageManager, InitSystem: row.InitSystem, OsProbe: row.OsProbe, DiscoveryJson: string(b),
				InstalledVersion: Version, UpdatedAt: now, ID: connID}); e != nil {
				return e
			}
		}
		return m.recordDistribution(ctx, q, connID, checks, now)
	}); serr != nil {
		return "", errors.Join(err, serr)
	}
	if err != nil {
		return "", m.health(ctx, connID, err)
	}
	_ = m.health(ctx, connID, nil)
	sum := fmt.Sprintf("%d change(s) applied on %s and verified through the Traefik API", len(items), row.Name)
	if len(checks) > 0 {
		sum += fmt.Sprintf("; %d certificate(s) served with the expected fingerprint", len(checks))
	}
	if out.Restarted {
		sum += "; Traefik restarted"
	}
	return sum, nil
}

// recordDistribution stores what each certificate check found on the host (Traefik → Certificates tab).
func (m *Manager) recordDistribution(ctx context.Context, q *db.Queries, connID string, checks []CertCheck, now int64) error {
	certs, err := q.ListCertificates(ctx)
	if err != nil {
		return err
	}
	byName := map[string]db.Certificate{}
	for _, c := range certs {
		byName[c.Name] = c
	}
	for _, chk := range checks {
		c, ok := byName[chk.Name]
		if !ok {
			continue
		}
		p := db.UpsertCertificateDistributionParams{CertificateID: c.ID, ConnectionID: connID, Fingerprint: chk.Got,
			State: "verified", LastVerifiedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now}
		if chk.Err != nil {
			p.State, p.Error = "failed", chk.Err.Error()
		}
		if err := q.UpsertCertificateDistribution(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

// dialer reaches the host's websecure port directly (the edge's TLS listener faces the network Hashi is in).
func (m *Manager) dialer(row db.TraefikConnection) Dialer {
	addr := sshx.Addr(row.SshHost, TLSPort)
	return func(ctx context.Context) (net.Conn, error) { return m.tlsDial(ctx, addr) }
}

// health stores the connection's health from an apply's outcome and returns err.
func (m *Manager) health(ctx context.Context, connID string, err error) error {
	h, msg := "healthy", ""
	if err != nil {
		h, msg = "failed", err.Error()
	}
	now := m.now().UnixMilli()
	_ = m.st.Q.SetTraefikConnectionHealth(context.WithoutCancel(ctx), db.SetTraefikConnectionHealthParams{
		Health: h, LastValidatedAt: sql.NullInt64{Int64: now, Valid: true}, LastValidationError: msg, UpdatedAt: now, ID: connID})
	return err
}

// desired builds what Hashi wants on the connection's host from the database.
func (m *Manager) desired(ctx context.Context, connID string) (db.TraefikConnection, Desired, error) {
	row, err := m.st.R.GetTraefikConnection(ctx, connID)
	if err != nil {
		return row, Desired{}, fmt.Errorf("traefik connection %s: %w", connID, err)
	}
	if row.OsProbe == "" || row.InitSystem == "" {
		return row, Desired{}, fmt.Errorf("%w: %s has not been validated", ErrNotReady, row.Name)
	}
	if !row.OwnershipConfirmedAt.Valid {
		return row, Desired{}, fmt.Errorf("%w: ownership of %s is not confirmed", ErrNotReady, row.Name)
	}
	probe, err := ParseProbe(row.OsProbe)
	if err != nil {
		return row, Desired{}, err
	}
	var disc Discovery
	if err := json.Unmarshal([]byte(row.DiscoveryJson), &disc); err != nil {
		return row, Desired{}, fmt.Errorf("discovery of %s: %w", row.Name, err)
	}
	general, err := settings.Get[settings.General](ctx, m.st.R)
	if err != nil {
		return row, Desired{}, err
	}
	hashiURL, err := originOf(general.InternalURL)
	if err != nil {
		return row, Desired{}, fmt.Errorf("%w: %w", ErrNotReady, err)
	}
	certs, err := m.issuedCerts(ctx)
	if err != nil {
		return row, Desired{}, err
	}
	svcs, err := services.List(ctx, m.st.R)
	if err != nil {
		return row, Desired{}, err
	}
	edge, err := EnsureEdgeToken(ctx, m.st, m.vault)
	if err != nil {
		return row, Desired{}, fmt.Errorf("edge token: %w", err)
	}
	paths := Paths{Binary: row.TraefikBinary, ConfigDir: row.ConfigDir}
	opts := Options{Paths: paths, APIPort: int(row.ApiPort), ForwardAuthPort: int(row.ApiPort) + 1, HashiURL: hashiURL, Certs: certs,
		Services: svcs, EdgeToken: edge}
	files, err := Render(opts)
	if err != nil {
		return row, Desired{}, err
	}
	svc, err := ServiceFor(row.InitSystem, paths)
	if err != nil {
		return row, Desired{}, err
	}
	d := Desired{Paths: paths, APIPort: opts.APIPort, Files: files, Service: svc, Backup: disc.pendingBackup(),
		PingTimeout: m.pingTimeout, Certs: certs, PackageManager: probe.PackageManager, Init: probe.Init}
	d.Binary = func(ctx context.Context) ([]byte, error) { return m.binary(ctx, probe.Arch) }
	return row, d, nil
}

// originOf reduces the internal Hashi URL to scheme://host[:port], the form edges call forward auth on.
func originOf(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("the internal Hashi URL %q is not set or not an http(s) URL (setup step 2)", raw)
	}
	return u.Scheme + "://" + u.Host, nil
}

func (m *Manager) issuedCerts(ctx context.Context) ([]Cert, error) {
	rows, err := m.st.R.ListCertificates(ctx)
	if err != nil {
		return nil, err
	}
	var out []Cert
	for _, r := range rows {
		if r.CertPem == "" || !r.KeySecretID.Valid {
			continue
		}
		key, err := m.secret(ctx, r.KeySecretID.String)
		if err != nil {
			return nil, fmt.Errorf("certificate %s key: %w", r.Name, err)
		}
		out = append(out, Cert{Name: r.Name, CertPEM: []byte(r.CertPem), KeyPEM: key})
	}
	return out, nil
}

func (m *Manager) secret(ctx context.Context, id string) (secrets.Value, error) {
	rec, err := m.st.R.GetSecret(ctx, id)
	if err != nil {
		return secrets.Value{}, err
	}
	return m.vault.Open(rec)
}

// Auth returns how Hashi logs in to the connection's host.
func (m *Manager) Auth(ctx context.Context, row db.TraefikConnection) (sshx.Auth, error) {
	a := sshx.Auth{User: row.SshUser}
	switch row.Auth {
	case "hashi_key":
		rec, err := m.st.R.GetSecretByPurpose(ctx, IdentityPurpose)
		if err != nil {
			return a, fmt.Errorf("the Hashi SSH key: %w", err)
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

func (m *Manager) dial(ctx context.Context, row db.TraefikConnection) (Conn, error) {
	auth, err := m.Auth(ctx, row)
	if err != nil {
		return nil, err
	}
	hk, err := sshx.ParseHostKey(row.HostKey)
	if err != nil {
		return nil, err
	}
	addr := sshx.Addr(row.SshHost, int(row.SshPort))
	c, err := m.connect(ctx, sshx.Config{Addr: addr, Auth: auth, Pinned: hk, Timeout: 15 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("connect to %s (%s): %w", row.Name, addr, err)
	}
	return c, nil
}

// Distribute plans every installed connection whose certificates lag behind: an issued certificate without a verified
// distribution of its current fingerprint (the certificates.renew job, after issuing). Certificate files are low risk
// and apply right away; a plan that also needs a restart waits for approval.
func (m *Manager) Distribute(eng *hsync.Engine) func(ctx context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		conns, err := m.st.R.ListTraefikConnections(ctx)
		if err != nil {
			return "", err
		}
		certs, err := m.st.R.ListCertificates(ctx)
		if err != nil {
			return "", err
		}
		dists, err := m.st.R.ListCertificateDistributions(ctx)
		if err != nil {
			return "", err
		}
		verified := map[string]bool{} // connection + fingerprint
		for _, d := range dists {
			if d.State == "verified" {
				verified[d.ConnectionID+" "+d.Fingerprint] = true
			}
		}
		var parts []string
		var errs []error
		for _, c := range conns {
			if !c.OwnershipConfirmedAt.Valid || c.InstalledVersion == "" {
				continue
			}
			lag := false
			for _, cert := range certs {
				if cert.CertPem != "" && !verified[c.ID+" "+cert.Fingerprint] {
					lag = true
				}
			}
			if !lag {
				continue
			}
			p, err := eng.Auto(ctx, Subsystem, c.ID, hsync.TriggerReconcile, audit.System)
			switch {
			case err != nil:
				errs = append(errs, fmt.Errorf("%s: %w", c.Name, err))
			case p != nil:
				parts = append(parts, fmt.Sprintf("%s: plan %s", c.Name, p.Status))
			}
		}
		return strings.Join(parts, ", "), errors.Join(errs...)
	}
}
