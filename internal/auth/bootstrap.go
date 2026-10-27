package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

const base58 = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// argon2id parameters (RFC 9106 §4, second recommended option).
const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // KiB
	argonThreads = 4
	argonKeyLen  = 32
)

// verifySlots bounds concurrent argon2id verifications (64 MiB each) so login attempts cannot exhaust memory.
var verifySlots = make(chan struct{}, 2)

func randomString(n int) (string, error) {
	var b strings.Builder
	max := big.NewInt(int64(len(base58)))
	for range n {
		i, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b.WriteByte(base58[i.Int64()])
	}
	return b.String(), nil
}

func hashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(pw), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonTime, argonThreads,
		enc.EncodeToString(salt), enc.EncodeToString(key)), nil
}

func checkPassword(ctx context.Context, encoded, pw string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("unknown password hash format")
	}
	var version int
	var m uint32
	var t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errors.New("unsupported argon2 version")
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false, errors.New("bad argon2 parameters")
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[4])
	if err != nil {
		return false, err
	}
	want, err := enc.DecodeString(parts[5])
	if err != nil {
		return false, err
	}
	select {
	case verifySlots <- struct{}{}:
		defer func() { <-verifySlots }()
	case <-ctx.Done():
		return false, ctx.Err()
	}
	//nolint:gosec // want is at most a few dozen bytes, from our own encoder
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// BootstrapCredentials are shown once; only their hash is stored.
type BootstrapCredentials struct {
	Username string
	Password secrets.Value
}

// NewBootstrapCredentials generates credentials and stores their hash with q, replacing any previous ones.
func NewBootstrapCredentials(ctx context.Context, q *db.Queries, now time.Time) (BootstrapCredentials, error) {
	user, err := randomString(8)
	if err != nil {
		return BootstrapCredentials{}, err
	}
	pw, err := randomString(24)
	if err != nil {
		return BootstrapCredentials{}, err
	}
	h, err := hashPassword(pw)
	if err != nil {
		return BootstrapCredentials{}, err
	}
	if err := q.UpsertBootstrapCredentials(ctx, db.UpsertBootstrapCredentialsParams{
		Username: user, PasswordHash: h, CreatedAt: now.UnixMilli(),
	}); err != nil {
		return BootstrapCredentials{}, fmt.Errorf("store bootstrap credentials: %w", err)
	}
	return BootstrapCredentials{Username: user, Password: secrets.New(pw)}, nil // hashi:allow-secret-pattern (typed secret, not a literal)
}

// PrintBootstrap writes the one-time credential block to w (container stdout, not the structured log).
func PrintBootstrap(w io.Writer, c BootstrapCredentials) {
	line := strings.Repeat("=", 64)
	_, _ = fmt.Fprintf(w, "\n%s\nHASHI BOOTSTRAP LOGIN (shown once; valid until setup completes)\n  username: %s\n  password: %s\n"+
		"Sign in from a private network. Lost it? Run `hashi bootstrap reset`.\n%s\n\n",
		line, c.Username, c.Password.Reveal(), line)
}

// ErrBadBootstrapLogin covers a wrong username, a wrong password and absent credentials alike.
var ErrBadBootstrapLogin = errors.New("wrong bootstrap username or password")

// CheckBootstrap verifies a bootstrap login attempt.
func CheckBootstrap(ctx context.Context, q *db.Queries, username, password string) error {
	row, err := q.GetBootstrapCredentials(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrBadBootstrapLogin
	}
	if err != nil {
		return fmt.Errorf("read bootstrap credentials: %w", err)
	}
	ok, err := checkPassword(ctx, row.PasswordHash, password)
	if err != nil {
		return err
	}
	userOK := subtle.ConstantTimeCompare([]byte(row.Username), []byte(username)) == 1
	if !ok || !userOK {
		return ErrBadBootstrapLogin
	}
	return nil
}
