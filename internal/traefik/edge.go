package traefik

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// EdgeTokenPurpose is the vault purpose of the edge token (01 §8).
const EdgeTokenPurpose = "traefik.edge.token" //nolint:gosec // a vault purpose name, not a credential

// EnsureEdgeToken returns the edge token every Hashi-managed Traefik sets on requests to Hashi, creating it on first
// use. It is a vault secret: rendered only into the edges' owner-only security file, never logged or returned.
func EnsureEdgeToken(ctx context.Context, st *store.Store, v *vault.Vault) (secrets.Value, error) {
	rec, err := st.R.GetSecretByPurpose(ctx, EdgeTokenPurpose)
	if err == nil {
		return v.Open(rec)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return secrets.Value{}, err
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return secrets.Value{}, err
	}
	tok := secrets.New(base64.RawURLEncoding.EncodeToString(b))
	ins, err := v.Seal(vault.Service, EdgeTokenPurpose, tok)
	if err != nil {
		return secrets.Value{}, err
	}
	if err := st.Tx(ctx, func(q *db.Queries) error { return q.InsertSecret(ctx, ins) }); err != nil {
		return secrets.Value{}, err
	}
	return tok, nil
}
