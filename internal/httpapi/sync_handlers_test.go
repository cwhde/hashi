package httpapi

import (
	"context"
	"encoding/json"
	"testing"

	"git.juzo.io/juzo/hashi/internal/adguard"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
)

// Bug 74: an AdGuard plan went out as a DNS RRSet with no values, and the Pending changes page threw on it, hiding
// every other plan. AdGuard items carry a domain and an answer; they must arrive as text, never as an RRSet.
func TestPlanBodyAdGuardItemsAreText(t *testing.T) {
	s := &Server{Store: storetest.New(t)}
	before, _ := json.Marshal(map[string]any{"domain": "app.example.com", "answer": "192.0.2.10"})
	p := &hsync.Plan{ID: "p1", Subsystem: adguard.Subsystem, Target: "missing-connection", Status: hsync.StatusPending,
		Items: []hsync.Item{{Action: hsync.ActionDelete, Object: "app.example.com", Risk: hsync.High, Before: before}}}

	out := s.planBody(context.Background(), p)

	if string(out.Subsystem) != adguard.Subsystem {
		t.Fatalf("subsystem = %q", out.Subsystem)
	}
	it := out.Items[0]
	if it.Before != nil || it.After != nil {
		t.Fatalf("AdGuard item sent as an RRSet: before=%+v after=%+v", it.Before, it.After)
	}
	if it.BeforeText == nil || *it.BeforeText != "app.example.com → 192.0.2.10" {
		t.Fatalf("before_text = %v", it.BeforeText)
	}
	if it.AfterText != nil {
		t.Fatalf("after_text = %q, want none for a delete", *it.AfterText)
	}
}
