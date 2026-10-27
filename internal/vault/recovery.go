package vault

import (
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// A recovery key is 256 random bits shown as 13 groups of 4 base32 characters plus one checksum group (02 §5.3):
// XXXX-XXXX-…-XXXX-CCCC. The checksum catches typos; confirming it proves the user kept the whole key.

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

const groupLen = 4

// ErrRecoveryKeyFormat means the entered text is not a well-formed recovery key (typo, wrong length, bad checksum).
var ErrRecoveryKeyFormat = errors.New("not a valid recovery key: check for typos")

// ErrRecoveryKeyMismatch means a well-formed key that does not belong to this installation.
var ErrRecoveryKeyMismatch = errors.New("this recovery key does not belong to this installation")

func checksum(key []byte) string {
	sum := sha256.Sum256(append([]byte("hashi/recovery/checksum|"), key...))
	return b32.EncodeToString(sum[:])[:groupLen]
}

func group(s string) string {
	var parts []string
	for len(s) > groupLen {
		parts = append(parts, s[:groupLen])
		s = s[groupLen:]
	}
	return strings.Join(append(parts, s), "-")
}

// NewRecoveryKey returns a fresh key, its display form and its checksum group (what the user re-enters).
func NewRecoveryKey() (key []byte, display secrets.Value, check string, err error) {
	key = make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, secrets.Value{}, "", err
	}
	check = checksum(key)
	return key, secrets.New(group(b32.EncodeToString(key) + check)), check, nil
}

// ParseRecoveryKey accepts the display form with any spacing, dashes or case and validates the checksum.
func ParseRecoveryKey(s string) ([]byte, error) {
	clean := strings.Map(func(r rune) rune {
		switch {
		case r == '-' || r == ' ' || r == '\t' || r == '\n':
			return -1
		case r >= 'a' && r <= 'z':
			return r - 'a' + 'A'
		}
		return r
	}, s)
	if len(clean) < groupLen {
		return nil, ErrRecoveryKeyFormat
	}
	body, check := clean[:len(clean)-groupLen], clean[len(clean)-groupLen:]
	key, err := b32.DecodeString(body)
	if err != nil || len(key) != KeySize {
		return nil, ErrRecoveryKeyFormat
	}
	if subtle.ConstantTimeCompare([]byte(checksum(key)), []byte(check)) != 1 {
		return nil, ErrRecoveryKeyFormat
	}
	return key, nil
}

func recoveryDerive(key []byte, info string) []byte {
	k, err := hkdf.Key(sha256.New, key, nil, info, KeySize)
	if err != nil {
		panic(err)
	}
	return k
}

func recoveryAAD(version int64) []byte {
	return []byte("hashi/recovery/master|" + strconv.FormatInt(version, 10))
}

// WrapMaster seals a copy of this vault's master key under the recovery key.
func (v *Vault) WrapMaster(recovery []byte) (db.UpsertRecoveryKeyParams, error) {
	nonce, wrapped, err := seal(recoveryDerive(recovery, "hashi/recovery/wrap/v1"), v.master, recoveryAAD(v.version))
	if err != nil {
		return db.UpsertRecoveryKeyParams{}, fmt.Errorf("wrap master key: %w", err)
	}
	return db.UpsertRecoveryKeyParams{
		KeyVersion: v.version, Verifier: recoveryDerive(recovery, "hashi/recovery/verifier/v1"),
		Nonce: nonce, WrappedMaster: wrapped, CreatedAt: v.now().UnixMilli(),
	}, nil
}

// RecoverMaster returns the master key wrapped in r, if recovery is the right key.
func RecoverMaster(r db.RecoveryKey, recovery []byte) ([]byte, error) {
	if subtle.ConstantTimeCompare(recoveryDerive(recovery, "hashi/recovery/verifier/v1"), r.Verifier) != 1 {
		return nil, ErrRecoveryKeyMismatch
	}
	m, err := open(recoveryDerive(recovery, "hashi/recovery/wrap/v1"), r.Nonce, r.WrappedMaster, recoveryAAD(r.KeyVersion))
	if err != nil {
		return nil, fmt.Errorf("unwrap master key: %w", err)
	}
	return m, nil
}

// MatchesMaster reports whether m is this vault's master key (the setup step 9 test unlock).
func (v *Vault) MatchesMaster(m []byte) bool { return subtle.ConstantTimeCompare(m, v.master) == 1 }
