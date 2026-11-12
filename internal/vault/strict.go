package vault

import (
	"context"
	"crypto/hkdf"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Strict mode (02 §5.4): the master key is not on disk. After a start the vault is locked until the admin unlocks it
// with a PRF passkey or the recovery key. While locked, secrets can't be opened or sealed; the operational keys and
// the KeptPurposes secrets stay readable from vault_kept, so Pulse, sign-ins, CAPTCHA clearances and edge sessions
// keep working (owner, 2026-11-11: no automation may break).

// ErrLocked means the vault is locked (strict mode, not unlocked since the start).
var ErrLocked = errors.New("the vault is locked: unlock it with a passkey or the recovery key")

// ErrNoPRF means the browser returned no PRF output: this passkey can't unlock the vault.
var ErrNoPRF = errors.New("this passkey gave no PRF output, so it can't unlock the vault; use another passkey or the recovery key")

// Mode is the vault mode.
type Mode string

const (
	ModeService Mode = "service"
	ModeStrict  Mode = "strict"
)

// KeptPurposes are stored secrets whose value strict mode keeps readable while locked: the edge session cookie key
// (edge.keyPurpose), so signed-in users of protected apps stay signed in, and the edge token
// (traefik.EdgeTokenPurpose), so forwarded client addresses from Hashi's Traefik stay trusted.
var KeptPurposes = []string{"edge:session-key", "traefik.edge.token"}

const (
	keptOp     = "op:"
	keptSecret = "secret:"
)

// ReadMode returns the stored mode; no row means service mode.
func ReadMode(ctx context.Context, q *db.Queries) (Mode, error) {
	m, err := q.GetVaultMode(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return ModeService, nil
	}
	if err != nil {
		return "", fmt.Errorf("read vault mode: %w", err)
	}
	return Mode(m), nil
}

// LoadStrict returns a locked vault for a strict-mode database, carrying the kept keys.
func LoadStrict(ctx context.Context, q *db.Queries) (*Vault, error) {
	version := int64(1)
	if r, err := q.GetSecretByPurpose(ctx, keyCheckPurpose); err == nil {
		version = r.KeyVersion
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("read key check: %w", err)
	}
	rows, err := q.ListVaultKept(ctx)
	if err != nil {
		return nil, fmt.Errorf("read kept keys: %w", err)
	}
	v := &Vault{version: version, now: time.Now, ops: map[string][]byte{}, kept: map[string]secrets.Value{}}
	for _, r := range rows {
		switch {
		case len(r.Name) > len(keptOp) && r.Name[:len(keptOp)] == keptOp:
			v.ops[r.Name[len(keptOp):]] = r.Value
		case len(r.Name) > len(keptSecret) && r.Name[:len(keptSecret)] == keptSecret:
			v.kept[r.Name[len(keptSecret):]] = secrets.New(string(r.Value))
		}
	}
	for _, n := range OperationalNames {
		if len(v.ops[n]) != KeySize {
			return nil, fmt.Errorf("strict mode: operational key %s is missing; unlock is impossible without it", n)
		}
	}
	return v, nil
}

// Locked reports whether the vault is locked.
func (v *Vault) Locked() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.master == nil
}

// OnUnlock registers f to run after each successful Unlock (paused jobs start again).
func (v *Vault) OnUnlock(f func(ctx context.Context)) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.unlock = append(v.unlock, f)
}

// Unlock installs master if it opens this database's key check, then refreshes the kept keys with q (a writer).
func (v *Vault) Unlock(ctx context.Context, q *db.Queries, master []byte) error {
	probe, err := New(master, v.version)
	if err != nil {
		return err
	}
	if err := probe.CheckKey(ctx, q); err != nil {
		return err
	}
	v.mu.Lock()
	if v.master == nil {
		v.master = append([]byte(nil), master...)
	}
	hooks := append([]func(context.Context){}, v.unlock...)
	v.mu.Unlock()
	if err := v.RefreshKept(ctx, q); err != nil {
		return err
	}
	for _, f := range hooks {
		f(ctx)
	}
	return nil
}

// RefreshKept writes the operational keys and the KeptPurposes secrets to vault_kept. The vault must be unlocked.
func (v *Vault) RefreshKept(ctx context.Context, q *db.Queries) error {
	if v.Locked() {
		return ErrLocked
	}
	for _, n := range OperationalNames {
		if err := q.PutVaultKept(ctx, db.PutVaultKeptParams{Name: keptOp + n, Value: v.Operational(n)}); err != nil {
			return err
		}
	}
	for _, p := range KeptPurposes {
		r, err := q.GetSecretByPurpose(ctx, p)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		val, err := v.Open(r)
		if err != nil {
			return err
		}
		if err := q.PutVaultKept(ctx, db.PutVaultKeptParams{Name: keptSecret + p, Value: []byte(val.Reveal())}); err != nil {
			return err
		}
	}
	return nil
}

// EnableStrict switches an unlocked vault to strict mode with q (one transaction): kept keys written, mode stored.
// From the next start the master key file is ignored and the vault starts locked.
func (v *Vault) EnableStrict(ctx context.Context, q *db.Queries) error {
	if err := v.RefreshKept(ctx, q); err != nil {
		return err
	}
	return q.SetVaultMode(ctx, db.SetVaultModeParams{Mode: string(ModeStrict), ChangedAt: v.now().UnixMilli()})
}

// ErrFileKeyMismatch means the configured master key file is missing or holds another key, so service mode could not
// start after the next restart.
var ErrFileKeyMismatch = errors.New("install the master key as HASHI_MASTER_KEY_FILE first: service mode needs it at every start")

// DisableStrict switches back to service mode with q. fileKey is what HASHI_MASTER_KEY_FILE holds now (nil if
// missing); it must be this vault's master key. Kept keys and passkey unlockers are removed.
func (v *Vault) DisableStrict(ctx context.Context, q *db.Queries, fileKey []byte) error {
	if v.Locked() {
		return ErrLocked
	}
	if fileKey == nil || !v.MatchesMaster(fileKey) {
		return ErrFileKeyMismatch
	}
	if err := q.DeleteVaultKept(ctx); err != nil {
		return err
	}
	if err := q.DeleteVaultUnlockers(ctx); err != nil {
		return err
	}
	return q.SetVaultMode(ctx, db.SetVaultModeParams{Mode: string(ModeService), ChangedAt: v.now().UnixMilli()})
}

func prfKey(prf []byte) []byte {
	k, err := hkdf.Key(sha256.New, prf, nil, "hashi/strict/prf/v1", KeySize)
	if err != nil {
		panic(err)
	}
	return k
}

func unlockerAAD(passkeyID string, version int64) []byte {
	return []byte("hashi/strict/unlocker|" + passkeyID + "|" + strconv.FormatInt(version, 10))
}

// NewUnlocker wraps this vault's master key for a passkey, given the passkey's PRF output for salt.
func (v *Vault) NewUnlocker(passkeyID string, salt, prf []byte) (db.InsertVaultUnlockerParams, error) {
	if len(prf) < KeySize {
		return db.InsertVaultUnlockerParams{}, ErrNoPRF
	}
	m, err := v.masterKey()
	if err != nil {
		return db.InsertVaultUnlockerParams{}, err
	}
	nonce, wrapped, err := seal(prfKey(prf), m, unlockerAAD(passkeyID, v.version))
	if err != nil {
		return db.InsertVaultUnlockerParams{}, err
	}
	return db.InsertVaultUnlockerParams{ID: ulid.Make().String(), PasskeyID: passkeyID, Salt: salt, KeyVersion: v.version,
		Nonce: nonce, WrappedMaster: wrapped, CreatedAt: v.now().UnixMilli()}, nil
}

// OpenUnlocker returns the master key wrapped for a passkey, given its PRF output.
func OpenUnlocker(u db.VaultUnlocker, prf []byte) ([]byte, error) {
	if len(prf) < KeySize {
		return nil, ErrNoPRF
	}
	m, err := open(prfKey(prf), u.Nonce, u.WrappedMaster, unlockerAAD(u.PasskeyID, u.KeyVersion))
	if err != nil {
		return nil, errors.New("this passkey's PRF output does not open the vault")
	}
	return m, nil
}
