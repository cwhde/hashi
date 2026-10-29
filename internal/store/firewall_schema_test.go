package store

import (
	"context"
	"testing"

	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Migration 00008 rebuilds the sync tables again to add the firewall subsystem: existing plans and items survive, a
// firewall plan is accepted, and the down migration removes it and the firewall tables.
func TestMigration8FirewallAndSyncHistory(t *testing.T) {
	ctx := context.Background()
	s := open(t)
	p, err := s.provider()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.UpTo(ctx, 7); err != nil {
		t.Fatal(err)
	}
	plan := func(id, subsystem string) error {
		return s.Q.InsertSyncPlan(ctx, db.InsertSyncPlanParams{ID: id, Subsystem: subsystem, Target: "t-" + id, Title: id,
			Trigger: "manual", Risk: "high", Status: "applied", PlanHash: "h", CreatedBy: "t", CreatedAt: 1})
	}
	if err := plan("p1", "traefik"); err != nil {
		t.Fatal(err)
	}
	if err := plan("p0", "firewall"); err == nil {
		t.Fatal("firewall plan accepted before migration 8")
	}
	if _, err := p.UpTo(ctx, 8); err != nil {
		t.Fatal(err)
	}
	if err := plan("p2", "firewall"); err != nil {
		t.Fatal(err)
	}
	plans, err := s.Q.ListRecentSyncPlans(ctx, 10)
	if err != nil || len(plans) != 2 {
		t.Fatalf("plans after up: %d %v", len(plans), err)
	}
	if _, err := s.Writer.ExecContext(ctx, `INSERT INTO firewall_hosts (id, name, edge_mode, created_at, updated_at)
		VALUES ('h', 'edge-a', 'managed', 1, 1)`); err == nil {
		t.Fatal("managed host without SSH target accepted")
	}
	if _, err := s.Writer.ExecContext(ctx, `INSERT INTO firewall_hosts (id, name, edge_mode, created_at, updated_at)
		VALUES ('h', 'edge-b', 'external_nat', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.DownTo(ctx, 7); err != nil {
		t.Fatal(err)
	}
	plans, err = s.Q.ListRecentSyncPlans(ctx, 10)
	if err != nil || len(plans) != 1 || plans[0].ID != "p1" {
		t.Fatalf("plans after down: %+v %v", plans, err)
	}
}
