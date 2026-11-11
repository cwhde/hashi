package vault

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// ErrRecoveryOtherMaster means the recovery key is valid but wraps a different master key than the one configured.
var ErrRecoveryOtherMaster = errors.New("the recovery key belongs to a different master key than the configured one")

// Load returns the vault for this database: the key version is the one its key check was sealed under (1 before any
// rotation), the master key must open it, and operational keys kept by a rotation are loaded.
func Load(ctx context.Context, q *db.Queries, master []byte) (*Vault, error) {
	version := int64(1)
	switch r, err := q.GetSecretByPurpose(ctx, keyCheckPurpose); {
	case err == nil:
		version = r.KeyVersion
	case !errors.Is(err, sql.ErrNoRows):
		return nil, fmt.Errorf("read key check: %w", err)
	}
	v, err := New(master, version)
	if err != nil {
		return nil, err
	}
	if err := v.CheckKey(ctx, q); err != nil {
		return nil, err
	}
	for _, name := range OperationalNames {
		r, err := q.GetSecretByPurpose(ctx, opPurpose+name)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read operational key %s: %w", name, err)
		}
		val, err := v.Open(r)
		if err != nil {
			return nil, err
		}
		k, err := base64.StdEncoding.DecodeString(val.Reveal())
		if err != nil || len(k) != KeySize {
			return nil, fmt.Errorf("operational key %s is damaged", name)
		}
		if v.ops == nil {
			v.ops = map[string][]byte{}
		}
		v.ops[name] = k
	}
	return v, nil
}

// Rotate replaces the master key (02 §5.2). Run it inside one transaction with the server stopped: every secret is
// re-sealed under a fresh master key at the next version, the operational keys are kept (stored sealed on the first
// rotation), and the recovery key's copy is re-wrapped, so the same recovery key keeps working. recovery must be this
// installation's recovery key and must wrap v's master key. It returns the new vault and its master key.
func (v *Vault) Rotate(ctx context.Context, q *db.Queries, recovery []byte) (*Vault, []byte, error) {
	rec, err := q.GetRecoveryKey(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, errors.New("this database has no recovery key (setup step 9 was never finished)")
	}
	if err != nil {
		return nil, nil, err
	}
	m, err := RecoverMaster(rec, recovery)
	if err != nil {
		return nil, nil, err
	}
	if !v.MatchesMaster(m) {
		return nil, nil, ErrRecoveryOtherMaster
	}
	master := make([]byte, KeySize)
	if _, err := rand.Read(master); err != nil {
		return nil, nil, err
	}
	nv, err := New(master, v.version+1)
	if err != nil {
		return nil, nil, err
	}
	nv.ops = map[string][]byte{}
	for _, name := range OperationalNames {
		k := v.Operational(name)
		nv.ops[name] = k
		if _, err := q.GetSecretByPurpose(ctx, opPurpose+name); errors.Is(err, sql.ErrNoRows) {
			p, err := v.Seal(Service, opPurpose+name, secrets.New(base64.StdEncoding.EncodeToString(k)))
			if err != nil {
				return nil, nil, err
			}
			if err := q.InsertSecret(ctx, p); err != nil {
				return nil, nil, fmt.Errorf("keep operational key %s: %w", name, err)
			}
		} else if err != nil {
			return nil, nil, err
		}
	}
	all, err := q.ListSecrets(ctx)
	if err != nil {
		return nil, nil, err
	}
	for _, r := range all {
		val, err := v.Open(r)
		if err != nil {
			return nil, nil, err
		}
		p, err := nv.sealID(r.ID, Class(r.Class), r.Purpose, val)
		if err != nil {
			return nil, nil, err
		}
		if err := q.ResealSecret(ctx, db.ResealSecretParams{
			KeyVersion: p.KeyVersion, Nonce: p.Nonce, Ciphertext: p.Ciphertext, DekNonce: p.DekNonce,
			WrappedDek: p.WrappedDek, UpdatedAt: p.UpdatedAt, ID: r.ID,
		}); err != nil {
			return nil, nil, fmt.Errorf("re-seal secret %s: %w", r.ID, err)
		}
	}
	w, err := nv.WrapMaster(recovery)
	if err != nil {
		return nil, nil, err
	}
	if err := q.RewrapRecoveryKey(ctx, db.RewrapRecoveryKeyParams{KeyVersion: w.KeyVersion, Nonce: w.Nonce, WrappedMaster: w.WrappedMaster}); err != nil {
		return nil, nil, err
	}
	return nv, master, nil
}
