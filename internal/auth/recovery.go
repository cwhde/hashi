package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"git.juzo.io/juzo/hashi/internal/store/db"
)

// RecoveryLinkTTL is how long a `hashi admin recover` link works (02 §2).
const RecoveryLinkTTL = 15 * time.Minute

// ErrRecoveryLink covers unknown, used and expired recovery links alike.
var ErrRecoveryLink = errors.New("this recovery link is invalid, used or expired; run `hashi admin recover` again")

// NewRecoveryLink stores the hash of a fresh one-time token with q and returns the token.
func NewRecoveryLink(ctx context.Context, q *db.Queries, now time.Time) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	if err := q.InsertRecoveryLink(ctx, db.InsertRecoveryLinkParams{
		TokenHash: hashToken(raw), CreatedAt: now.UnixMilli(), ExpiresAt: now.Add(RecoveryLinkTTL).UnixMilli(),
	}); err != nil {
		return "", fmt.Errorf("store recovery link: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// UseRecoveryLink consumes a token with q. It succeeds once, within the TTL.
func UseRecoveryLink(ctx context.Context, q *db.Queries, token string, now time.Time) error {
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(raw) != 32 {
		return ErrRecoveryLink
	}
	n, err := q.UseRecoveryLink(ctx, db.UseRecoveryLinkParams{Now: now.UnixMilli(), TokenHash: hashToken(raw)})
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrRecoveryLink
	}
	return nil
}
