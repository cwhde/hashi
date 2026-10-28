package store

import (
	"context"
	"testing"

	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Migration 00005 rebuilds the sync tables to widen the subsystem CHECK. Existing plans, items and runs must survive,
// deleting a plan must still cascade, the open-plan index must still hold, and the down migration must work.
func TestMigration5KeepsSyncHistory(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	p, err := s.provider()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 4); err != nil {
		t.Fatal(err)
	}
	q := s.Q
	plan := func(id, subsystem, target, status string) error {
		return q.InsertSyncPlan(ctx, db.InsertSyncPlanParams{ID: id, Subsystem: subsystem, Target: target, Title: id,
			Trigger: "manual", Risk: "low", Status: status, PlanHash: "h", CreatedBy: "t", CreatedAt: 1})
	}
	for _, id := range []string{"p1", "p2"} {
		if err := plan(id, "dns-records", "z1", "applied"); err != nil {
			t.Fatal(err)
		}
		if err := q.InsertSyncPlanItem(ctx, db.InsertSyncPlanItemParams{PlanID: id, Seq: 1, Action: "create", Object: "a", Risk: "low"}); err != nil {
			t.Fatal(err)
		}
		if err := q.InsertSyncRun(ctx, db.InsertSyncRunParams{ID: "r" + id, PlanID: id, StartedAt: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if err := plan("old", "traefik", "c1", "pending"); err == nil {
		t.Fatal("schema 4 accepted subsystem traefik")
	}

	if _, err := p.UpTo(ctx, 5); err != nil {
		t.Fatal(err)
	}
	count := func(table string) int {
		var n int
		if err := s.Writer.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&n); err != nil { //nolint:gosec // fixed table names
			t.Fatal(err)
		}
		return n
	}
	for _, tbl := range []string{"sync_plans", "sync_plan_items", "sync_runs"} {
		if n := count(tbl); n != 2 {
			t.Errorf("%s has %d rows after migration, want 2", tbl, n)
		}
	}
	var violations int
	if err := s.Writer.QueryRowContext(ctx, "SELECT count(*) FROM pragma_foreign_key_check").Scan(&violations); err != nil {
		t.Fatal(err)
	}
	if violations != 0 {
		t.Errorf("foreign_key_check reports %d violations", violations)
	}
	if _, err := s.Writer.ExecContext(ctx, "DELETE FROM sync_plans WHERE id = 'p1'"); err != nil {
		t.Fatal(err)
	}
	if n := count("sync_plan_items"); n != 1 {
		t.Errorf("deleting a plan left %d items, want 1 (cascade)", n)
	}
	if n := count("sync_runs"); n != 1 {
		t.Errorf("deleting a plan left %d runs, want 1 (cascade)", n)
	}
	for _, sub := range []string{"traefik", "certificates"} {
		if err := plan("new-"+sub, sub, "c1", "pending"); err != nil {
			t.Errorf("subsystem %s rejected: %v", sub, err)
		}
	}
	if err := plan("dup", "traefik", "c1", "pending"); err == nil {
		t.Error("a second open plan for the same subsystem and target was accepted")
	}

	if _, err := p.DownTo(ctx, 4); err != nil {
		t.Fatal(err)
	}
	if n := count("sync_plans"); n != 1 {
		t.Errorf("after down: %d plans, want 1 (the DNS plan)", n)
	}
}

// Rows the rest of Hashi relies on can't be stored half-formed: a GTS account needs EAB, a connection needs a
// credential unless it uses Hashi's own key, and an issued certificate needs its key.
func TestTraefikSchemaRejectsIncompleteRows(t *testing.T) {
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
	acme := db.UpsertACMEAccountParams{ID: "a1", Directory: "gts", DirectoryUrl: "https://ca.invalid/dir", Email: "a@example.com",
		DnsProviderID: "p1", PropagationDelayS: 30, ResolversJson: "[]", Health: "unknown"}
	if err := q.UpsertACMEAccount(ctx, acme); err == nil {
		t.Error("GTS account without EAB accepted")
	}
	acme.Directory = "letsencrypt"
	if err := q.UpsertACMEAccount(ctx, acme); err != nil {
		t.Errorf("Let's Encrypt account without EAB rejected: %v", err)
	}

	conn := db.InsertTraefikConnectionParams{ID: "c1", Name: "edge", SshHost: "192.0.2.10", SshPort: 22, SshUser: "root",
		Auth: "password", HostKeyType: "ssh-ed25519", HostKey: []byte{1}, HostKeyFingerprint: "SHA256:x"}
	if err := q.InsertTraefikConnection(ctx, conn); err == nil {
		t.Error("password connection without a credential accepted")
	}
	conn.Auth = "hashi_key"
	if err := q.InsertTraefikConnection(ctx, conn); err != nil {
		t.Errorf("connection with Hashi's key rejected: %v", err)
	}

	if err := q.InsertCertificate(ctx, db.InsertCertificateParams{ID: "k1", Name: "example.com", NamesJson: `["example.com","*.example.com"]`}); err != nil {
		t.Fatal(err)
	}
	if err := q.SetCertificateIssued(ctx, db.SetCertificateIssuedParams{ID: "k1", NamesJson: "[]", CertPem: "-----BEGIN CERTIFICATE-----",
		LastRenewalResult: "issued"}); err == nil {
		t.Error("issued certificate without a key accepted")
	}
}
