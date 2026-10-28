package traefik

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/secrets"
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
	IdentityPurpose   = "ssh.identity"          // Hashi's own SSH key
	CredentialPurpose = "traefik.ssh.credential" //nolint:gosec // a vault purpose name, not a credential
	PassphrasePurpose = "traefik.ssh.passphrase" //nolint:gosec // a vault purpose name, not a credential
	CertKeyPurpose    = "certificate.key"
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
}

// NewManager returns the Traefik manager.
func NewManager(st *store.Store, v *vault.Vault, a *audit.Log, connect Connector, binary BinarySource) *Manager {
	return &Manager{st: st, vault: v, audit: a, connect: connect, binary: binary, now: time.Now}
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
	now := m.now().UnixMilli()
	// Files that were written before a failure are on the host: record them so the next plan's "before" is right.
	if serr := m.st.Tx(ctx, func(q *db.Queries) error {
		record := out.Written
		if err == nil {
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
		if err != nil {
			return nil
		}
		if diff.InstallBinary {
			if e := q.SetTraefikInstalledVersion(ctx, db.SetTraefikInstalledVersionParams{InstalledVersion: Version, UpdatedAt: now, ID: connID}); e != nil {
				return e
			}
		}
		return m.markDistributed(ctx, q, connID, now)
	}); serr != nil {
		return "", errors.Join(err, serr)
	}
	if err != nil {
		return "", m.health(ctx, connID, err)
	}
	_ = m.health(ctx, connID, nil)
	sum := fmt.Sprintf("%d change(s) applied on %s and verified through the Traefik API", len(items), row.Name)
	if out.Restarted {
		sum += "; Traefik restarted"
	}
	return sum, nil
}

// markDistributed records that every issued certificate is on the host (Verify by TLS handshake comes with
// certificates.go).
func (m *Manager) markDistributed(ctx context.Context, q *db.Queries, connID string, now int64) error {
	certs, err := q.ListCertificates(ctx)
	if err != nil {
		return err
	}
	for _, c := range certs {
		if c.CertPem == "" {
			continue
		}
		if err := q.UpsertCertificateDistribution(ctx, db.UpsertCertificateDistributionParams{CertificateID: c.ID,
			ConnectionID: connID, Fingerprint: c.Fingerprint, State: "applied", UpdatedAt: now}); err != nil {
			return err
		}
	}
	return nil
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
	paths := Paths{Binary: row.TraefikBinary, ConfigDir: row.ConfigDir}
	opts := Options{Paths: paths, APIPort: int(row.ApiPort), ForwardAuthPort: int(row.ApiPort) + 1, HashiURL: hashiURL, Certs: certs}
	files, err := Render(opts)
	if err != nil {
		return row, Desired{}, err
	}
	svc, err := ServiceFor(row.InitSystem, paths)
	if err != nil {
		return row, Desired{}, err
	}
	d := Desired{Paths: paths, APIPort: opts.APIPort, Files: files, Service: svc, Backup: disc.ExistingConfigs,
		PingTimeout: m.pingTimeout}
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
			return a, fmt.Errorf("Hashi's SSH key: %w", err)
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
