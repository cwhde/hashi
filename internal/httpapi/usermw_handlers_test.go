package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/auth"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/traefik"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// edgeStandIn is the Traefik subsystem for these tests: it plans the stored user file and, like Traefik with a file
// it cannot load, fails to apply one that names a middleware "refused-…".
type edgeStandIn struct {
	st      *store.Store
	mu      sync.Mutex
	applied string
}

func (*edgeStandIn) Name() string                 { return traefik.Subsystem }
func (*edgeStandIn) LockKey(target string) string { return "traefik:" + target }

func (e *edgeStandIn) stored(ctx context.Context) string {
	row, err := e.st.R.GetUserMiddlewares(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}
	return row.Yaml
}

func (e *edgeStandIn) Plan(ctx context.Context, _ string) (hsync.Draft, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stored(ctx) == e.applied {
		return hsync.Draft{}, nil
	}
	return hsync.Draft{Title: "user middlewares", Items: []hsync.Item{{Action: hsync.ActionUpdate,
		Object: "/etc/hashi/traefik/dynamic/30-user-middlewares.yml", Risk: hsync.Low}}}, nil
}

func (e *edgeStandIn) Apply(ctx context.Context, _ string, _ []hsync.Item) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	text := e.stored(ctx)
	if strings.Contains(text, "refused-") {
		return "", errors.New("traefik did not load the configuration: router svc-a@file is missing")
	}
	e.applied = text
	return "written", nil
}

func userMWServer(t *testing.T) (*Server, *edgeStandIn) {
	t.Helper()
	ctx := context.Background()
	st := storetest.New(t)
	al := audit.New([]byte("k"))
	if err := settings.Put(ctx, st.Q, settings.General{RootDomain: "example.com", AdminDomain: "hashi.example.com",
		InternalURL: "http://192.0.2.5:3340", SyncIntervalSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	v, err := vault.New(bytes.Repeat([]byte{7}, vault.KeySize), 1)
	if err != nil {
		t.Fatal(err)
	}
	rec, _ := v.Seal(vault.Service, "dns.token", secrets.New("t"))
	if err := st.Q.InsertSecret(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertDNSProvider(ctx, db.InsertDNSProviderParams{ID: "p1", Name: "hetzner", Type: "hetzner",
		DefaultTtl: 3600, TokenSecretID: rec.ID, Health: "healthy", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertDNSZone(ctx, db.InsertDNSZoneParams{ID: "z1", ProviderID: "p1", ProviderZoneID: "1",
		Name: "example.com", ZoneTtl: 3600, CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertTraefikConnection(ctx, db.InsertTraefikConnectionParams{ID: "t1", Name: "edge", SshHost: "192.0.2.9",
		SshPort: 22, SshUser: "root", Auth: "hashi_key", HostKeyType: "ssh-ed25519", HostKey: []byte{1}, HostKeyFingerprint: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.ConfirmTraefikOwnership(ctx, db.ConfirmTraefikOwnershipParams{OwnershipConfirmedAt: sql.NullInt64{Int64: 1, Valid: true},
		UpdatedAt: 1, ID: "t1"}); err != nil {
		t.Fatal(err)
	}
	edge := &edgeStandIn{st: st}
	return &Server{Store: st, Audit: al, Vault: v, Sessions: auth.NewManager(st, al, []byte("csrf")),
		Sync: hsync.New(st, al, edge), Log: slog.Default()}, edge
}

const officeFile = `# office
http:
  middlewares:
    office-only:
      ipAllowList:
        sourceRange: [192.0.2.0/24]
    no-frames:
      headers:
        customResponseHeaders:
          X-Frame-Options: DENY
`

func putFile(t *testing.T, s *Server, text string, every ...string) (gen.UserMiddlewaresSaveResult, error) {
	t.Helper()
	if every == nil {
		every = []string{}
	}
	res, err := s.PutUserMiddlewares(context.Background(), gen.PutUserMiddlewaresRequestObject{
		Body: &gen.UserMiddlewaresInput{Yaml: text, EveryService: every}})
	if err != nil {
		return gen.UserMiddlewaresSaveResult{}, err
	}
	return gen.UserMiddlewaresSaveResult(res.(gen.PutUserMiddlewares200JSONResponse)), nil
}

func getFile(t *testing.T, s *Server) gen.UserMiddlewares {
	t.Helper()
	res, err := s.GetUserMiddlewares(context.Background(), gen.GetUserMiddlewaresRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	return gen.UserMiddlewares(res.(gen.GetUserMiddlewares200JSONResponse))
}

func TestUserMiddlewaresSaveAndCheck(t *testing.T) {
	ctx := context.Background()
	s, edge := userMWServer(t)
	if got := getFile(t, s); got.Yaml != "" || len(got.Middlewares) != 0 || len(got.EveryService) != 0 {
		t.Fatalf("fresh %+v", got)
	}
	// The live check: the first problem with its line, nothing stored.
	chk, err := s.CheckUserMiddlewares(ctx, gen.CheckUserMiddlewaresRequestObject{Body: &gen.UserMiddlewaresCheckInput{
		Yaml: "http:\n  middlewares:\n    a:\n      buffering:\n        maxRequestBodyByte: 1\n"}})
	if err != nil {
		t.Fatal(err)
	}
	c := chk.(gen.CheckUserMiddlewares200JSONResponse)
	if c.Ok || c.Line == nil || *c.Line != 5 || !strings.Contains(*c.Error, "maxRequestBodyByte") {
		t.Fatalf("check %+v", c)
	}
	// A file that fails the check is refused and the stored one stays.
	if _, err := putFile(t, s, "http:\n  routers: {}\n"); status(err) != 422 {
		t.Fatalf("routers in the user file: %v", err)
	}
	if _, err := putFile(t, s, officeFile, "missing"); status(err) != 422 {
		t.Fatalf("every service names a middleware the file lacks: %v", err)
	}
	res, err := putFile(t, s, officeFile, "no-frames")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Plans) != 1 || res.Plans[0].Status != "applied" || edge.applied != officeFile {
		t.Fatalf("plans %+v applied %q", res.Plans, edge.applied)
	}
	got := getFile(t, s)
	if got.Yaml != officeFile || len(got.Middlewares) != 2 || got.Middlewares[0].Name != "no-frames" ||
		!got.Middlewares[0].EveryService || got.Middlewares[1].EveryService || got.UpdatedAt == nil {
		t.Fatalf("after save %+v", got)
	}
	// Dropping a name the every-service setting uses is refused.
	if _, err := putFile(t, s, "http:\n  middlewares:\n    office-only:\n      compress: {}\n", "no-frames"); status(err) != 422 {
		t.Fatalf("dropped an every-service name: %v", err)
	}
}

// Traefik refusing the saved file during apply puts the previous file back, on the hosts and in the database (04 §4).
func TestUserMiddlewaresRefusedByTraefikRestores(t *testing.T) {
	s, edge := userMWServer(t)
	if _, err := putFile(t, s, officeFile); err != nil {
		t.Fatal(err)
	}
	broken := officeFile + "    refused-one:\n      compress: {}\n"
	_, err := putFile(t, s, broken)
	var p *Problem
	if !errors.As(err, &p) || p.Status != 422 || !strings.Contains(p.Detail, "previous one was put back") ||
		!strings.Contains(p.Detail, "router svc-a@file is missing") {
		t.Fatalf("refused file: %v", err)
	}
	if got := getFile(t, s); got.Yaml != officeFile {
		t.Fatalf("stored after the refusal:\n%s", got.Yaml)
	}
	if edge.applied != officeFile {
		t.Fatalf("host has %q", edge.applied)
	}
}

func TestServiceTrafficAndMiddlewares(t *testing.T) {
	ctx := context.Background()
	s, _ := userMWServer(t)
	host := gen.ServiceTrafficHostCustom
	in := gen.ServiceInput{Name: "Photos", Slug: "photos", Kind: gen.ServiceInputKindHttp, DomainMode: gen.ServiceInputDomainModeSubdomain,
		HostName: "photos.example.com", Target: gen.ServiceTarget{Address: "192.0.2.30", Port: 2342},
		Traffic: &gen.ServiceTraffic{Host: &host, HostValue: ptr("photos.internal.example"), DropRealIp: ptr(true),
			ResponseHeaderTimeout: ptr(120)},
		Middlewares: &[]string{"office-only"}}
	if _, err := s.CreateService(ctx, gen.CreateServiceRequestObject{Body: &in}); status(err) != 422 {
		t.Fatalf("a middleware the file does not define: %v", err)
	}
	if _, err := putFile(t, s, officeFile); err != nil {
		t.Fatal(err)
	}
	res, err := s.CreateService(ctx, gen.CreateServiceRequestObject{Body: &in})
	if err != nil {
		t.Fatal(err)
	}
	sv := res.(gen.CreateService200JSONResponse).Service
	if *sv.Traffic.Host != host || *sv.Traffic.HostValue != "photos.internal.example" || sv.Traffic.DropRealIp == nil ||
		*sv.Traffic.ResponseHeaderTimeout != 120 || len(sv.Middlewares) != 1 {
		t.Fatalf("created %+v", sv)
	}
	if u := getFile(t, s).Middlewares; u[1].Name != "office-only" || len(u[1].UsedBy) != 1 || u[1].UsedBy[0].ServiceName != "Photos" {
		t.Fatalf("used by %+v", u)
	}
	// The file can no longer drop office-only while Photos uses it.
	if _, err := putFile(t, s, "http:\n  middlewares:\n    no-frames:\n      compress: {}\n"); status(err) != 422 ||
		!strings.Contains(err.Error(), "Photos") {
		t.Fatalf("dropped a used name: %v", err)
	}
	// A save that leaves traffic and middlewares out keeps them.
	keep := in
	keep.Traffic, keep.Middlewares = nil, nil
	keep.Name = "Photos 2"
	up, err := s.UpdateService(ctx, gen.UpdateServiceRequestObject{Id: sv.Id, Body: &keep})
	if err != nil {
		t.Fatal(err)
	}
	if got := up.(gen.UpdateService200JSONResponse).Service; got.Name != "Photos 2" || *got.Traffic.HostValue != "photos.internal.example" ||
		len(got.Middlewares) != 1 {
		t.Fatalf("kept %+v", got)
	}
	// Request headers that have their own option, or that forward auth owns, are refused.
	for _, name := range []string{"Host", "X-Forwarded-For", "x-real-ip", "X-Hashi-User", "bad name"} {
		bad := in
		bad.Traffic = &gen.ServiceTraffic{RequestHeaders: &[]gen.ServiceHeader{{Name: name, Value: "x"}}}
		if _, err := s.UpdateService(ctx, gen.UpdateServiceRequestObject{Id: sv.Id, Body: &bad}); status(err) != 422 {
			t.Errorf("request header %q: %v", name, err)
		}
	}
	// The rendered preview shows the options.
	eff, err := s.GetServiceEffective(ctx, gen.GetServiceEffectiveRequestObject{Id: sv.Id})
	if err != nil {
		t.Fatal(err)
	}
	y := eff.(gen.GetServiceEffective200JSONResponse).Yaml
	for _, want := range []string{"Host: photos.internal.example", "X-Real-Ip: \"\"", "responseHeaderTimeout: 120s", "- office-only"} {
		if !strings.Contains(y, want) {
			t.Errorf("preview lacks %q:\n%s", want, y)
		}
	}
}
