package firewall

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

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

// Dial opens SSH to a firewall host with its stored credential and pinned host key, for other subsystems that work on
// the same host (scripts, 09 §1). A host without SSH yet is ErrNotReady.
func (m *Manager) Dial(ctx context.Context, id string) (Conn, error) {
	row, err := m.st.R.GetFirewallHost(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if row.SshHost == "" || len(row.HostKey) == 0 {
		return nil, ErrNotReady
	}
	return m.dial(ctx, row)
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
	var last error
	for _, addr := range m.sshAddrs(ctx, row) {
		c, err := m.connect(ctx, sshx.Config{Addr: addr, Auth: a, Pinned: hk, Timeout: 15 * time.Second})
		if err == nil {
			return c, nil
		}
		last = fmt.Errorf("connect to %s (%s): %w", row.Name, addr, err)
	}
	return nil, last
}
