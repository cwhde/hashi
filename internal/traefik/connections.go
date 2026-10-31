package traefik

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// ErrInvalid means the connection input is not acceptable; the message says what to change.
var ErrInvalid = errors.New("invalid Traefik connection")

// Auth methods (traefik_connections.auth).
const (
	AuthHashiKey   = "hashi_key"
	AuthPrivateKey = "private_key"
	AuthPassword   = "password"
)

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

// Identity returns Hashi's own SSH public key as an authorized_keys line, creating the key on first use (04 §5: the
// admin installs it on the host instead of handing Hashi a password).
func (m *Manager) Identity(ctx context.Context, actor, ip string) (string, error) {
	rec, err := m.st.R.GetSecretByPurpose(ctx, IdentityPurpose)
	if errors.Is(err, sql.ErrNoRows) {
		priv, _, gerr := sshx.GenerateKey("hashi")
		if gerr != nil {
			return "", gerr
		}
		ins, serr := m.vault.Seal(vault.Service, IdentityPurpose, priv)
		if serr != nil {
			return "", serr
		}
		if err := m.st.Tx(ctx, func(q *db.Queries) error {
			if err := q.InsertSecret(ctx, ins); err != nil {
				return err
			}
			return m.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "traefik.identity.created", IP: ip, Target: "secret:" + ins.ID})
		}); err != nil {
			return "", err
		}
		return sshx.PublicLine(priv)
	}
	if err != nil {
		return "", err
	}
	priv, err := m.vault.Open(rec)
	if err != nil {
		return "", err
	}
	return sshx.PublicLine(priv)
}

// ConnectionInput is what the admin enters for a Traefik host (04 §5). HostKey is the key the admin confirmed after a
// scan (wire format); Hashi pins exactly that. Empty Credential/Passphrase on an update keep the stored ones.
type ConnectionInput struct {
	Name, Host string
	Port       int
	User, Auth string
	Credential secrets.Value
	Passphrase secrets.Value
	HostKey    []byte
	InternalIP string
	Binary     string
	ConfigDir  string
	APIPort    int
}

// Save creates (id "") or updates a connection with sealed credentials and the confirmed host key. A changed address
// or host key clears what validation and the ownership confirmation found: they belong to the old machine.
func (m *Manager) Save(ctx context.Context, actor, ip, id string, in ConnectionInput) (db.TraefikConnection, error) {
	in.Name, in.Host, in.User = strings.TrimSpace(in.Name), strings.TrimSpace(in.Host), strings.TrimSpace(in.User)
	if in.Binary == "" {
		in.Binary = DefaultBinary
	}
	if in.ConfigDir == "" {
		in.ConfigDir = DefaultConfigDir
	}
	if in.APIPort == 0 {
		in.APIPort = 8080
	}
	switch {
	case in.Name == "" || in.Host == "" || in.User == "":
		return db.TraefikConnection{}, fmt.Errorf("%w: name, host and user are required", ErrInvalid)
	case in.Port < 1 || in.Port > 65535:
		return db.TraefikConnection{}, fmt.Errorf("%w: SSH port must be between 1 and 65535", ErrInvalid)
	case in.APIPort < 1024 || in.APIPort > 65534:
		return db.TraefikConnection{}, fmt.Errorf("%w: the Traefik API port must be between 1024 and 65534 (the next port is forward auth)", ErrInvalid)
	case in.Auth != AuthHashiKey && in.Auth != AuthPrivateKey && in.Auth != AuthPassword:
		return db.TraefikConnection{}, fmt.Errorf("%w: unknown login method %q", ErrInvalid, in.Auth)
	}
	if err := (Paths{Binary: in.Binary, ConfigDir: in.ConfigDir}).Validate(); err != nil {
		return db.TraefikConnection{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	hk, err := sshx.ParseHostKey(in.HostKey)
	if err != nil {
		return db.TraefikConnection{}, fmt.Errorf("%w: scan and confirm the host key first: %w", ErrInvalid, err)
	}
	var cur db.TraefikConnection
	if id != "" {
		if cur, err = m.st.R.GetTraefikConnection(ctx, id); err != nil {
			return cur, fmt.Errorf("traefik connection %s: %w", id, err)
		}
	}
	needsCred := in.Auth != AuthHashiKey && (id == "" || cur.Auth != in.Auth || !cur.CredentialSecretID.Valid)
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
	if in.Auth != AuthHashiKey {
		if cred, err = seal(CredentialPurpose, in.Credential, cur.CredentialSecretID); err != nil {
			return cur, err
		}
		if in.Auth == AuthPrivateKey {
			if pass, err = seal(PassphrasePurpose, in.Passphrase, cur.PassphraseSecretID); err != nil {
				return cur, err
			}
		}
	}
	now := m.now().UnixMilli()
	isNew := id == ""
	if isNew {
		id = ulid.Make().String()
	}
	moved := !isNew && (cur.SshHost != in.Host || cur.SshPort != int64(in.Port) || string(cur.HostKey) != string(hk.Key))
	err = m.st.Tx(ctx, func(q *db.Queries) error {
		for _, r := range recs {
			if err := q.InsertSecret(ctx, r); err != nil {
				return err
			}
		}
		if isNew {
			if err := q.InsertTraefikConnection(ctx, db.InsertTraefikConnectionParams{ID: id, Name: in.Name, SshHost: in.Host,
				SshPort: int64(in.Port), SshUser: in.User, Auth: in.Auth, CredentialSecretID: cred, PassphraseSecretID: pass,
				HostKeyType: hk.Type, HostKey: hk.Key, HostKeyFingerprint: hk.Fingerprint, InternalIp: in.InternalIP,
				CreatedAt: now, UpdatedAt: now}); err != nil {
				return err
			}
		} else {
			if err := q.UpdateTraefikConnectionAccess(ctx, db.UpdateTraefikConnectionAccessParams{Name: in.Name, SshHost: in.Host,
				SshPort: int64(in.Port), SshUser: in.User, Auth: in.Auth, CredentialSecretID: cred, PassphraseSecretID: pass,
				HostKeyType: hk.Type, HostKey: hk.Key, HostKeyFingerprint: hk.Fingerprint, InternalIp: in.InternalIP,
				UpdatedAt: now, ID: id}); err != nil {
				return err
			}
			if moved {
				if err := q.SetTraefikConnectionDetected(ctx, db.SetTraefikConnectionDetectedParams{DiscoveryJson: "{}", UpdatedAt: now, ID: id}); err != nil {
					return err
				}
				if err := q.ConfirmTraefikOwnership(ctx, db.ConfirmTraefikOwnershipParams{UpdatedAt: now, ID: id}); err != nil {
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
		if err := q.SetTraefikConnectionPaths(ctx, db.SetTraefikConnectionPathsParams{TraefikBinary: in.Binary, ConfigDir: in.ConfigDir,
			ApiPort: int64(in.APIPort), UpdatedAt: now, ID: id}); err != nil {
			return err
		}
		action := "traefik.connection.updated"
		if isNew {
			action = "traefik.connection.created"
		}
		return m.audit.Record(ctx, q, audit.Event{Actor: actor, Action: action, IP: ip, Target: "traefik_connection:" + id,
			Detail: map[string]any{"name": in.Name, "host": in.Host, "port": in.Port, "user": in.User, "auth": in.Auth,
				"host_key": hk.Fingerprint}})
	})
	if err != nil {
		return db.TraefikConnection{}, err
	}
	return m.st.R.GetTraefikConnection(ctx, id)
}

// Validate logs in with the pinned host key and checks the host (04 §5): OS, package manager and init system, an
// existing Traefik and its config, root, and that Hashi can write in its own state directory (a uniquely named file,
// removed again). The findings are stored; nothing outside /var/lib/hashi is touched.
func (m *Manager) Validate(ctx context.Context, id string) (db.TraefikConnection, Probe, error) {
	row, err := m.st.R.GetTraefikConnection(ctx, id)
	if err != nil {
		return row, Probe{}, fmt.Errorf("traefik connection %s: %w", id, err)
	}
	p, err := m.validate(ctx, row)
	if herr := m.health(ctx, id, err); herr != nil {
		return row, p, herr
	}
	row, err = m.st.R.GetTraefikConnection(ctx, id)
	return row, p, err
}

func (m *Manager) validate(ctx context.Context, row db.TraefikConnection) (Probe, error) {
	c, err := m.dial(ctx, row)
	if err != nil {
		return Probe{}, err
	}
	defer func() { _ = c.Close() }()
	r, err := c.Run(ctx, ProbeCommand, nil)
	if err != nil {
		return Probe{}, fmt.Errorf("detect the operating system: %w", err)
	}
	p, err := ParseProbe(string(r.Stdout))
	if err != nil {
		return p, err
	}
	if !p.Root {
		return p, fmt.Errorf("%w: Hashi installs Traefik as a service and needs to log in as root (user %s is not)", ErrUnsupported, row.SshUser)
	}
	if err := c.MkdirAll(StateDir, 0o700); err != nil {
		return p, fmt.Errorf("write check: %w", err)
	}
	probe := StateDir + "/.hashi-write-check-" + strings.ToLower(ulid.Make().String())
	if _, err := c.WriteFileAtomic(probe, []byte("hashi\n"), 0o600); err != nil {
		return p, fmt.Errorf("write check: %w", err)
	}
	res, err := c.Run(ctx, "rm -f "+sshx.Quote(probe), nil)
	if err != nil {
		return p, fmt.Errorf("write check: remove %s: %w", probe, err)
	}
	if !res.OK() {
		return p, fmt.Errorf("write check: could not remove %s: %s", probe, strings.TrimSpace(string(res.Stderr)))
	}
	var prev Discovery
	_ = json.Unmarshal([]byte(row.DiscoveryJson), &prev)
	disc, _ := json.Marshal(Discovery{ExistingBinary: p.ExistingBinary, ExistingConfigs: p.ExistingConfigs, BackedUp: prev.BackedUp})
	return p, m.st.Q.SetTraefikConnectionDetected(ctx, db.SetTraefikConnectionDetectedParams{OsFamily: p.Family,
		PackageManager: p.PackageManager, InitSystem: p.Init, OsProbe: string(r.Stdout), DiscoveryJson: string(disc),
		InstalledVersion: row.InstalledVersion, UpdatedAt: m.now().UnixMilli(), ID: row.ID})
}

// ConfirmOwnership records that the admin lets Hashi take over the host: existing Traefik config is backed up and Hashi's
// own service replaces it (04 §2 step 5). Only a validated connection can be confirmed.
func (m *Manager) ConfirmOwnership(ctx context.Context, actor, ip, id string) error {
	row, err := m.st.R.GetTraefikConnection(ctx, id)
	if err != nil {
		return fmt.Errorf("traefik connection %s: %w", id, err)
	}
	if row.OsProbe == "" {
		return fmt.Errorf("%w: validate %s first", ErrNotReady, row.Name)
	}
	now := m.now().UnixMilli()
	return m.st.Tx(ctx, func(q *db.Queries) error {
		if err := q.ConfirmTraefikOwnership(ctx, db.ConfirmTraefikOwnershipParams{
			OwnershipConfirmedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: id}); err != nil {
			return err
		}
		return m.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "traefik.ownership.confirmed", IP: ip,
			Target: "traefik_connection:" + id, Detail: map[string]any{"discovery": json.RawMessage(row.DiscoveryJson)}})
	})
}
