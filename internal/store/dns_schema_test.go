package store

import (
	"context"
	"testing"

	"git.juzo.io/juzo/hashi/internal/store/db"
)

// The schema itself refuses NS/SOA ownership and any NS/SOA import decision other than "leave", so no code path can
// make them Hashi's (invariant 2). The import service checks first; this is the backstop.
func TestInvariant_2_SchemaRejectsNSSOAOwnership(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	if err := s.Migrate(ctx, quiet()); err != nil {
		t.Fatal(err)
	}
	q := s.Q
	if err := q.InsertSecret(ctx, db.InsertSecretParams{ID: "s1", Class: "service", Purpose: "dns.token", KeyVersion: 1,
		Nonce: []byte{1}, Ciphertext: []byte{1}, DekNonce: []byte{1}, WrappedDek: []byte{1}}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertDNSProvider(ctx, db.InsertDNSProviderParams{ID: "p1", Name: "Hetzner", Type: "hetzner", DefaultTtl: 3600,
		TokenSecretID: "s1", Health: "healthy"}); err != nil {
		t.Fatal(err)
	}
	if err := q.InsertDNSZone(ctx, db.InsertDNSZoneParams{ID: "z1", ProviderID: "p1", ProviderZoneID: "1", Name: "example.com", ZoneTtl: 3600}); err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{"NS", "SOA"} {
		for _, origin := range []string{"system", "managed", "imported", "user"} {
			if err := q.UpsertDNSOwnership(ctx, db.UpsertDNSOwnershipParams{ZoneID: "z1", Name: "@", Type: typ, Origin: origin}); err == nil {
				t.Errorf("%s ownership with origin %s was accepted", typ, origin)
			}
		}
		for _, d := range []string{"import", "prune"} {
			if err := q.UpsertImportDecision(ctx, db.UpsertImportDecisionParams{ZoneID: "z1", Name: "@", Type: typ, Decision: d}); err == nil {
				t.Errorf("%s decision %q was accepted", typ, d)
			}
		}
		if err := q.UpsertImportDecision(ctx, db.UpsertImportDecisionParams{ZoneID: "z1", Name: "@", Type: typ, Decision: "leave"}); err != nil {
			t.Errorf("%s decision leave: %v", typ, err)
		}
	}
	if err := q.UpsertImportDecision(ctx, db.UpsertImportDecisionParams{ZoneID: "z1", Name: "@", Type: "A", Decision: "prune"}); err != nil {
		t.Errorf("A prune: %v", err)
	}
}
