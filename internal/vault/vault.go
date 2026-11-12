// Package vault seals secrets with envelope encryption under a deployment master key (02-setup-auth.md §5).
//
// Master key (32 bytes, file) → per-class KEK via HKDF-SHA256 → per-secret random DEK (AES-256-GCM) → payload.
// Only service mode exists before S12; strict mode (master key not on disk) is added there.
package vault

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// KeySize is the length of the master key, KEKs and DEKs.
const KeySize = 32

// Class decides which KEK wraps a secret's DEK (02 §5.1).
type Class string

const (
	Service Class = "service" // decryptable unattended
	Admin   Class = "admin"   // reserved; needs an unlocked admin session (not used before S12)
)

// keyCheckPurpose names the canary secret that proves the master key matches this database.
const keyCheckPurpose = "vault.keycheck"

// ErrWrongMasterKey means the configured master key cannot open this database's secrets.
var ErrWrongMasterKey = errors.New("the master key does not match this database (wrong HASHI_MASTER_KEY_FILE?)")

// Vault seals and opens secret records. It never logs or returns key material.
type Vault struct {
	mu      sync.RWMutex
	master  []byte // nil while locked (strict mode, 02 §5.4)
	version int64
	now     func() time.Time
	ops     map[string][]byte           // operational keys kept from an earlier master key or for strict mode
	kept    map[string]secrets.Value    // strict mode: stored secrets readable while locked (KeptPurposes)
	unlock  []func(ctx context.Context) // run after Unlock
}

// OperationalNames lists every operational key. They are stable for the life of an installation: the first rotation
// stores them sealed (purpose "vault.op.<name>"), so a new master key keeps Pulse tokens, CSRF tokens, CAPTCHA
// clearances and audit correlation working.
var OperationalNames = []string{"audit-correlation", "csrf", "pulse-token-pepper", "captcha-clearance"}

const opPurpose = "vault.op."

// New returns a vault for a 32-byte master key at the given key version (1 until `hashi vault rotate`).
func New(master []byte, version int64) (*Vault, error) {
	if len(master) != KeySize {
		return nil, fmt.Errorf("master key must be %d bytes, got %d", KeySize, len(master))
	}
	return &Vault{master: append([]byte(nil), master...), version: version, now: time.Now}, nil
}

// Keygen returns a fresh master key in the file format LoadMasterKey reads.
func Keygen() (string, error) {
	k := make([]byte, KeySize)
	if _, err := rand.Read(k); err != nil {
		return "", err
	}
	return EncodeMasterKey(k), nil
}

// LoadMasterKey reads a base64 master key file (as printed by `hashi keygen`).
func LoadMasterKey(path string) ([]byte, error) {
	if path == "" {
		return nil, errors.New("no master key: set HASHI_MASTER_KEY_FILE (create one with `hashi keygen`)")
	}
	raw, err := os.ReadFile(path) //nolint:gosec // operator-configured path
	if err != nil {
		return nil, fmt.Errorf("read master key: %w", err)
	}
	k, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		return nil, errors.New("master key file is not base64 (create one with `hashi keygen`)")
	}
	if len(k) != KeySize {
		return nil, fmt.Errorf("master key must decode to %d bytes, got %d", KeySize, len(k))
	}
	return k, nil
}

// masterKey returns the master key, or ErrLocked.
func (v *Vault) masterKey() ([]byte, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	if v.master == nil {
		return nil, ErrLocked
	}
	return v.master, nil
}

// derive returns HKDF-SHA256(master, info) — the only way keys are derived from the master key.
func (v *Vault) derive(info string) ([]byte, error) {
	m, err := v.masterKey()
	if err != nil {
		return nil, err
	}
	k, err := hkdf.Key(sha256.New, m, nil, info, KeySize)
	if err != nil {
		panic(err) // only fails for lengths > 255*32
	}
	return k, nil
}

func (v *Vault) kek(c Class, version int64) ([]byte, error) {
	return v.derive("hashi/kek/" + string(c) + "/v" + strconv.FormatInt(version, 10))
}

// Operational returns an operational key (cookie/CSRF HMAC, audit salt, …) (02 §5.1): the stored one after a rotation,
// otherwise derived from the master key. A name missing from OperationalNames is a programming error.
func (v *Vault) Operational(name string) []byte {
	if !slices.Contains(OperationalNames, name) {
		panic("vault: operational key " + name + " is not in OperationalNames")
	}
	v.mu.RLock()
	k, ok := v.ops[name]
	v.mu.RUnlock()
	if ok {
		return append([]byte(nil), k...)
	}
	k, err := v.derive("hashi/op/" + name + "/v1")
	if err != nil {
		// A locked vault always carries every operational key (LoadStrict refuses to start otherwise).
		panic("vault: operational key " + name + " missing while locked")
	}
	return k
}

func aad(id string, c Class, version int64) []byte {
	return []byte(id + "|" + string(c) + "|" + strconv.FormatInt(version, 10))
}

func seal(key, plaintext, ad []byte) (nonce, ct []byte, err error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce = make([]byte, g.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return nonce, g.Seal(nil, nonce, plaintext, ad), nil
}

func open(key, nonce, ct, ad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return g.Open(nil, nonce, ct, ad)
}

// Seal encrypts value into a new record with the given purpose. The caller inserts it.
func (v *Vault) Seal(c Class, purpose string, value secrets.Value) (db.InsertSecretParams, error) {
	return v.sealID(ulid.Make().String(), c, purpose, value)
}

func (v *Vault) sealID(id string, c Class, purpose string, value secrets.Value) (db.InsertSecretParams, error) {
	ad := aad(id, c, v.version)
	dek := make([]byte, KeySize)
	if _, err := rand.Read(dek); err != nil {
		return db.InsertSecretParams{}, err
	}
	nonce, ct, err := seal(dek, []byte(value.Reveal()), ad)
	if err != nil {
		return db.InsertSecretParams{}, fmt.Errorf("seal secret: %w", err)
	}
	kek, err := v.kek(c, v.version)
	if err != nil {
		return db.InsertSecretParams{}, err
	}
	dekNonce, wrapped, err := seal(kek, dek, ad)
	if err != nil {
		return db.InsertSecretParams{}, fmt.Errorf("wrap key: %w", err)
	}
	now := v.now().UnixMilli()
	return db.InsertSecretParams{
		ID: id, Class: string(c), Purpose: purpose, KeyVersion: v.version, Nonce: nonce, Ciphertext: ct,
		DekNonce: dekNonce, WrappedDek: wrapped, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Open decrypts a record. A tampered record, a moved ciphertext or a wrong master key all fail.
func (v *Vault) Open(r db.SecretRecord) (secrets.Value, error) {
	c := Class(r.Class)
	ad := aad(r.ID, c, r.KeyVersion)
	kek, err := v.kek(c, r.KeyVersion)
	if errors.Is(err, ErrLocked) {
		v.mu.RLock()
		val, ok := v.kept[r.Purpose]
		v.mu.RUnlock()
		if ok {
			return val, nil
		}
	}
	if err != nil {
		return secrets.Value{}, err
	}
	dek, err := open(kek, r.DekNonce, r.WrappedDek, ad)
	if err != nil {
		return secrets.Value{}, fmt.Errorf("unwrap key for secret %s: %w", r.ID, err)
	}
	pt, err := open(dek, r.Nonce, r.Ciphertext, ad)
	if err != nil {
		return secrets.Value{}, fmt.Errorf("open secret %s: %w", r.ID, err)
	}
	return secrets.New(string(pt)), nil
}

// CheckKey proves the master key matches the database: it opens the canary secret, creating it on first start.
func (v *Vault) CheckKey(ctx context.Context, q *db.Queries) error {
	r, err := q.GetSecretByPurpose(ctx, keyCheckPurpose)
	if errors.Is(err, sql.ErrNoRows) {
		canary := make([]byte, 16)
		if _, err := rand.Read(canary); err != nil {
			return err
		}
		p, err := v.Seal(Service, keyCheckPurpose, secrets.New(base64.StdEncoding.EncodeToString(canary)))
		if err != nil {
			return err
		}
		return q.InsertSecret(ctx, p)
	}
	if err != nil {
		return fmt.Errorf("read key check: %w", err)
	}
	if _, err := v.Open(r); err != nil {
		return ErrWrongMasterKey
	}
	return nil
}

// KeyVersion is the master key version new secrets are sealed under.
func (v *Vault) KeyVersion() int64 { return v.version }

// EncodeMasterKey renders a master key in the file format LoadMasterKey reads.
func EncodeMasterKey(k []byte) string { return base64.StdEncoding.EncodeToString(k) }
