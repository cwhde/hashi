package httpapi

import (
	"context"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/services"
)

// Bug 64: App display finds the services that publish its pages by asking each port on Hashi's own address.
func TestPublicPages(t *testing.T) {
	ctx := context.Background()
	s, _ := userMWServer(t)
	for slug, target := range map[string]string{"apps": "192.0.2.5", "status": "192.0.2.5", "other": "192.0.2.9"} {
		port := map[string]int{"apps": 3341, "status": 3342, "other": 3341}[slug]
		if _, err := services.Create(ctx, s.Store.Q, services.Service{Name: slug, Slug: slug, Kind: services.KindHTTP, DomainMode: services.ModeSubdomain,
			HostName: slug + ".example.com", Enabled: true, Target: services.Target{Address: target, Port: port}}, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	old := publicProbe
	defer func() { publicProbe = old }()
	publicProbe = func(_ context.Context, addr, path string) bool {
		return addr == "192.0.2.5:3341" && path == "/api/public/apps" || addr == "192.0.2.5:3342" && path == "/api/public/status"
	}
	r, err := s.GetPublicPages(ctx, gen.GetPublicPagesRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	p := r.(gen.GetPublicPages200JSONResponse)
	if len(p.Apps) != 1 || p.Apps[0].Url != "https://apps.example.com/" || len(p.Status) != 1 || p.Status[0].Name != "status" {
		t.Fatalf("pages %+v", p)
	}
}
