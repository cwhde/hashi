package app

import (
	"context"
	"io"
	"log/slog"
	"net/netip"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/security"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	"git.juzo.io/juzo/hashi/internal/traefik"
)

// The tail's lines reach the telemetry as response rows of their service; Hashi's own routers and unknown services
// are left out (06 §6.1). The sink was nil from the start, so nothing the tail read was ever counted.
func TestAccessSinkCountsServiceResponses(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	now := time.Now()
	// The sink only maps the router's slug to the service: a service that needs no zone keeps the setup short.
	wiki, err := services.Create(ctx, st.Q, services.Service{Name: "Wiki", Slug: "wiki", Kind: services.KindTCP,
		DomainMode: services.ModeNone, Enabled: true, Origin: services.OriginUser, PublicPort: 2222,
		Target: services.Target{Address: "192.0.2.20", Port: 8080}}, now)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	abuse := security.NewEngine(st, log, func() time.Time { return now })
	client := netip.MustParseAddr("198.51.100.7")
	at := now.Add(-2 * time.Minute)
	accessSink(st, abuse, log)(ctx, "conn", []traefik.AccessLine{
		{Client: client, Router: "svc-wiki@file", Path: "/a", Status: 200, Start: at},
		{Client: client, Router: "svc-wiki@file", Path: "/b", Status: 503, Start: at},
		{Client: client, Router: "hashi-health@file", Path: "/", Status: 200, Start: at},
		{Client: client, Router: "svc-gone@file", Path: "/", Status: 200, Start: at},
	})
	if _, err := abuse.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := st.R.ListSecurityBuckets(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, r := range rows {
		if r.ServiceID != wiki.ID || r.Decision != security.DecisionResponse {
			t.Fatalf("unexpected row %+v", r)
		}
		got[r.StatusClass] += r.RequestCount
	}
	if got["2xx"] != 1 || got["5xx"] != 1 || len(got) != 2 {
		t.Fatalf("response rows %v", got)
	}
}
