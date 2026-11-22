package httpapi

import (
	"context"
	"net/http"
	"testing"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/monitor"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

// The editor's save (07 §7, §8.1): it moves the entry and its switch in one go and writes one audit event; a save from
// an old page is 409, a bad group name 422, and the public status page follows at once.
func TestPublicLayoutSave(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	mon := monitor.New(st, func(context.Context) error { return nil }, nil, nil)
	if _, err := mon.CreateUser(ctx, "Lamp", "Home", monitor.CheckSpec{Type: monitor.TypeTCP, Target: "127.0.0.1:1"}); err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: st, Monitor: mon, Audit: audit.New([]byte("k"))}
	r, err := s.GetPublicLayout(ctx, gen.GetPublicLayoutRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	l := gen.PublicLayout(r.(gen.GetPublicLayout200JSONResponse))
	if len(l.Groups) != 2 || l.Groups[0].Name != "Home" || l.Groups[1].Id != nil || len(l.Groups[0].Entries) != 1 {
		t.Fatalf("layout %+v", l)
	}
	lamp := l.Groups[0].Entries[0]
	if lamp.Status || !lamp.CanStatus || lamp.CanApps || lamp.Slug != "lamp" {
		t.Fatalf("Lamp %+v", lamp)
	}
	yes := true
	save := func(version int64, name string) error {
		_, err := s.PutPublicLayout(ctx, gen.PutPublicLayoutRequestObject{Body: &gen.PublicLayoutSave{Version: version, Other: []string{},
			Groups: []struct {
				Entries []string `json:"entries"`
				Id      *string  `json:"id,omitempty"`
				Name    string   `json:"name"`
			}{{Name: name, Entries: []string{lamp.Key}}},
			Switches: &[]struct {
				Apps   *bool  `json:"apps,omitempty"`
				Entry  string `json:"entry"`
				Status *bool  `json:"status,omitempty"`
			}{{Entry: lamp.Key, Status: &yes}}}})
		return err
	}
	if err := save(l.Version, "Other"); status(err) != http.StatusUnprocessableEntity {
		t.Fatalf("a group called Other: %v", err)
	}
	if err := save(l.Version, "Lights"); err != nil {
		t.Fatal(err)
	}
	if err := save(l.Version, "Lights"); status(err) != http.StatusConflict {
		t.Fatalf("the same save again is stale: %v", err)
	}
	ps, err := s.GetPublicStatus(ctx, gen.GetPublicStatusRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	if g := ps.(gen.GetPublicStatus200JSONResponse).Groups; len(g) != 1 || g[0].Name != "Lights" || g[0].Entries[0].Slug != "lamp" {
		t.Fatalf("status page %+v", g)
	}
	events, err := st.R.ListAuditEvents(ctx, db.ListAuditEventsParams{Lim: 10})
	if err != nil {
		t.Fatal(err)
	}
	saved := 0
	for _, e := range events {
		if e.Action == "public_layout.saved" {
			saved++
		}
	}
	if saved != 1 {
		t.Fatalf("%d audit events for one save", saved)
	}
}
