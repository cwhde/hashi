//go:build integration

package traefik_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/itest"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/traefik"
	"git.juzo.io/juzo/hashi/internal/usermw"
)

// TestIntegrationTraffic is 04 §9's traffic test: through the pinned Traefik, the upstream receives each Host option,
// loses each removed client address header and gets the extra headers; the client loses the removed response header;
// a slow upstream gets 504 after its timeout; a user middleware takes effect on a service, on a route and on every
// service (Traefik settings), and the system service gets none of the user's.
func TestIntegrationTraffic(t *testing.T) {
	h := edgeHost(t)
	ctx := context.Background()
	target := func(port int) services.Target { return services.Target{Address: "127.0.0.1", Port: port} }
	user, err := usermw.Parse(`http:
  middlewares:
    office-only:
      ipAllowList:
        sourceRange: [192.0.2.0/24]
    no-frames:
      headers:
        customResponseHeaders:
          X-Frame-Options: DENY
`)
	if err != nil {
		t.Fatal(err)
	}
	crt, key := selfSigned(t, "example.test")
	opts := traefik.Options{
		Paths:   traefik.Paths{Binary: traefik.DefaultBinary, ConfigDir: traefik.DefaultConfigDir},
		APIPort: apiPort, ForwardAuthPort: faPort, HashiURL: "http://127.0.0.1:9000",
		Certs:     []traefik.Cert{{Name: "example.test", CertPEM: crt, KeyPEM: secrets.New(key)}},
		EdgeToken: secrets.New(edgeToken), UserMiddlewares: user, EveryService: []string{"no-frames"},
		Services: []services.Service{
			{Name: "Hashi", Slug: "hashi-admin", Kind: services.KindHTTP, HostName: "dev.hashi.example.test", Enabled: true,
				Origin: services.OriginSystem, OwnerFeature: services.OwnerSetup, Target: target(9000)},
			{Name: "Keep", Slug: "keep", Kind: services.KindHTTP, HostName: "keep.example.test", Enabled: true,
				Origin: services.OriginUser, Target: target(9001)},
			{Name: "Upstream host", Slug: "uphost", Kind: services.KindHTTP, HostName: "uphost.example.test", Enabled: true,
				Origin: services.OriginUser, Target: target(9001), Traffic: services.Traffic{Host: services.HostUpstream}},
			{Name: "Custom", Slug: "custom", Kind: services.KindHTTP, HostName: "custom.example.test", Enabled: true,
				Origin: services.OriginUser, Target: target(9001), Traffic: services.Traffic{
					Host: services.HostCustom, HostValue: "app.internal.example", DropForwardedFor: true, DropRealIP: true,
					RequestHeaders:  []services.Header{{Name: "X-Extra", Value: "one"}, {Name: "X-Remove", Value: ""}},
					ResponseHeaders: []services.Header{{Name: "X-Served-By", Value: "edge"}, {Name: "X-Upstream-Secret", Value: ""}}}},
			{Name: "Slow", Slug: "slow", Kind: services.KindHTTP, HostName: "slow.example.test", Enabled: true,
				Origin: services.OriginUser, Target: target(9001), Traffic: services.Traffic{ResponseHeaderTimeout: 1, DialTimeout: 5}},
			{Name: "Guarded", Slug: "guarded", Kind: services.KindHTTP, HostName: "guarded.example.test", Enabled: true,
				Origin: services.OriginUser, Target: target(9001), Middlewares: []string{"office-only"}},
			{Name: "Routed", Slug: "routed", Kind: services.KindHTTP, HostName: "routed.example.test", Enabled: true,
				Origin: services.OriginUser, Target: target(9001), AdvancedRoutes: true, Routes: []services.Route{
					{ID: "01JR0000000000000000000021", Enabled: true, Targets: []services.Target{target(9001)},
						PathMatch: services.MatchPrefix, PathValue: "/private", Priority: 10, RewriteMode: services.RewriteNone,
						Middlewares: []string{"office-only"}},
				}},
		},
	}
	files, err := traefik.Render(opts)
	if err != nil {
		t.Fatal(err)
	}
	d := traefik.Desired{
		Paths: opts.Paths, APIPort: apiPort, Files: files, PingTimeout: 15 * time.Second, PackageManager: "apk", Init: traefik.InitOpenRC,
		Service: traefik.Service{Path: "/etc/hashi-traefik-run", Data: []byte(runScript), Mode: 0o755,
			Enable: "/etc/hashi-traefik-run", Restart: "/etc/hashi-traefik-run"},
		Binary: func(context.Context) ([]byte, error) {
			return nil, errors.New("the image already has the pinned binary")
		},
	}
	diff, err := traefik.Observe(ctx, h, d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := traefik.Apply(ctx, h, d, diff, time.Now()); err != nil {
		t.Fatal(err)
	}
	// Every router, the user's middlewares and the transports load without an error.
	if err := traefik.Verify(ctx, h, apiPort, files); err != nil {
		t.Fatal(err)
	}

	get := func(host, path string, headers ...string) string {
		cmd := "/usr/local/bin/upstream get " + host + " '" + path + "'"
		for _, hd := range headers {
			cmd += " '" + hd + "'"
		}
		r, err := h.Run(ctx, cmd, nil)
		if err != nil {
			t.Fatalf("%s%s: %v", host, path, err)
		}
		return string(r.Stdout) + string(r.Stderr)
	}
	itest.WaitFor(t, 20*time.Second, "services routed", func() bool {
		return strings.HasPrefix(get("keep.example.test", "/"), "status=200")
	})
	check := func(name, got string, want, absent []string) {
		t.Helper()
		for _, w := range want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: missing %q in\n%s", name, w, got)
			}
		}
		for _, a := range absent {
			if strings.Contains(got, a) {
				t.Errorf("%s: %q should be gone in\n%s", name, a, got)
			}
		}
	}
	client := []string{"X-Forwarded-For=198.51.100.7", "X-Remove=client"}
	check("default", get("keep.example.test", "/headers", client...),
		[]string{"status=200 ", "host=keep.example.test\n", "req X-Forwarded-For=", "req X-Real-Ip=",
			"req X-Forwarded-Host=keep.example.test", "req X-Remove=client", "resp X-Frame-Options=DENY", "resp X-Upstream-Secret=leaks"},
		[]string{"198.51.100.7"})
	check("upstream host", get("uphost.example.test", "/headers"),
		[]string{"host=127.0.0.1:9001\n", "req X-Forwarded-Host=uphost.example.test"}, nil)
	check("custom", get("custom.example.test", "/headers", client...),
		[]string{"host=app.internal.example\n", "req X-Extra=one", "req X-Forwarded-Host=custom.example.test",
			"resp X-Served-By=edge", "resp X-Frame-Options=DENY"},
		[]string{"req X-Forwarded-For=", "req X-Real-Ip=", "req X-Remove=", "resp X-Upstream-Secret"})
	if got := get("slow.example.test", "/slow"); !strings.HasPrefix(got, "status=504") {
		t.Errorf("slow upstream past the timeout: %q", got)
	}
	if got := get("slow.example.test", "/headers"); !strings.HasPrefix(got, "status=200") {
		t.Errorf("fast request through the same transport: %q", got)
	}
	// office-only allows 192.0.2.0/24; the test client is 127.0.0.1.
	if got := get("guarded.example.test", "/"); !strings.HasPrefix(got, "status=403") {
		t.Errorf("service middleware: %q", got)
	}
	if got := get("routed.example.test", "/private/x"); !strings.HasPrefix(got, "status=403") {
		t.Errorf("route middleware: %q", got)
	}
	if got := get("routed.example.test", "/open"); !strings.HasPrefix(got, "status=200") {
		t.Errorf("outside the route: %q", got)
	}
	// The system service gets none of the user's middlewares.
	if got := get("dev.hashi.example.test", "/headers"); strings.Contains(got, "X-Frame-Options=DENY") {
		t.Errorf("the admin service got an every-service middleware:\n%s", got)
	}
}
