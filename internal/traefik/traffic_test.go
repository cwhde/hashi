package traefik

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/usermw"
)

const trafficUserFile = `# comments are dropped on the host
http:
  middlewares:
    office-only:
      ipAllowList:
        sourceRange: [192.0.2.0/24]
    big-uploads:
      buffering:
        maxRequestBodyBytes: 2147483648
    no-frames:
      headers:
        customResponseHeaders:
          X-Frame-Options: DENY
`

// trafficServices covers every traffic option and middleware position (04 §1.1a, §9): each Host mode, removed client
// address headers, extra request and response headers (set and removed), each timeout alone and together with https
// TLS options, service, route and every-service middlewares, a route rewrite after them, a route with its own protocol,
// and the system service that takes none of them.
func trafficServices() []services.Service {
	t := func(addr string, port int) services.Target { return services.Target{Address: addr, Port: port} }
	return []services.Service{
		{Name: "Hashi", Slug: "hashi-admin", Kind: services.KindHTTP, HostName: "hashi.example.com", Enabled: true,
			Origin: services.OriginSystem, OwnerFeature: services.OwnerSetup, Target: t("192.0.2.5", 3340)},
		{Name: "Plain", Slug: "plain", Kind: services.KindHTTP, HostName: "plain.example.com", Enabled: true,
			Origin: services.OriginUser, Target: t("192.0.2.10", 80)},
		{Name: "Upstream host", Slug: "uphost", Kind: services.KindHTTP, HostName: "uphost.example.com", Enabled: true,
			Origin: services.OriginUser, Target: t("app.internal.example", 8080),
			Traffic: services.Traffic{Host: services.HostUpstream}},
		{Name: "Custom host", Slug: "custom", Kind: services.KindHTTP, HostName: "custom.example.com", Enabled: true,
			Origin: services.OriginUser, Target: t("192.0.2.11", 8080),
			Traffic: services.Traffic{Host: services.HostCustom, HostValue: "app.internal.example",
				DropForwardedFor: true, DropRealIP: true,
				RequestHeaders:  []services.Header{{Name: "X-Extra", Value: "one"}, {Name: "Cookie-Debug", Value: ""}},
				ResponseHeaders: []services.Header{{Name: "Server", Value: ""}, {Name: "X-Served-By", Value: "edge"}}}},
		{Name: "Slow", Slug: "slow", Kind: services.KindHTTP, HostName: "slow.example.com", Enabled: true,
			Origin: services.OriginUser, Target: t("192.0.2.12", 8080),
			Traffic: services.Traffic{ResponseHeaderTimeout: 120}},
		{Name: "Vault", Slug: "vault", Kind: services.KindHTTPS, HostName: "vault.example.com", Enabled: true,
			Origin: services.OriginUser, Target: t("192.0.2.13", 8443), TLSServerName: "vault.internal.example",
			Traffic: services.Traffic{DialTimeout: 5, ResponseHeaderTimeout: 30, IdleTimeout: 90, DropRealIP: true},
			Middlewares: []string{"big-uploads"}},
		{Name: "Files", Slug: "files", Kind: services.KindHTTP, HostName: "files.example.com", Enabled: true,
			Origin: services.OriginUser, Target: t("192.0.2.14", 8080), Middlewares: []string{"office-only"},
			AdvancedRoutes: true, Routes: []services.Route{
				{ID: "01JR0000000000000000000011", Enabled: true, Targets: []services.Target{t("192.0.2.15", 9000)},
					PathMatch: services.MatchPrefix, PathValue: "/upload", Priority: 20, RewriteMode: services.RewriteStripPrefix,
					Middlewares: []string{"big-uploads"}},
				{ID: "01JR0000000000000000000012", Enabled: true, Targets: []services.Target{t("192.0.2.16", 50051)},
					Kind: services.KindH2C, PathMatch: services.MatchPrefix, PathValue: "/grpc", Priority: 10,
					RewriteMode: services.RewriteNone},
			}},
	}
}

func trafficOptions(t *testing.T, every []string) Options {
	t.Helper()
	f, err := usermw.Parse(trafficUserFile)
	if err != nil {
		t.Fatal(err)
	}
	o := testOptions()
	o.Services = trafficServices()
	o.UserMiddlewares = f
	o.EveryService = every
	o.EdgeToken = secrets.New("edge-token-for-tests")
	return o
}

func fileNamed(t *testing.T, files []File, name string) File {
	t.Helper()
	for _, f := range files {
		if filepath.Base(f.Path) == name {
			return f
		}
	}
	t.Fatalf("no %s rendered", name)
	return File{}
}

// 04 §9: golden files for every traffic option and middleware combination.
func TestRenderTrafficGolden(t *testing.T) {
	files, err := Render(trafficOptions(t, []string{"no-frames"}))
	if err != nil {
		t.Fatal(err)
	}
	golden(t, "traffic-services.yml", fileNamed(t, files, "10-hashi-http-services.yml").Data)
	golden(t, "traffic-user-middlewares.yml", fileNamed(t, files, usermw.FileName).Data)
}

// The user file goes to the host as parsed, before the services that name its middlewares.
func TestRenderUserFileBeforeServices(t *testing.T) {
	files, err := Render(trafficOptions(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	user := slices.IndexFunc(files, func(f File) bool { return strings.HasSuffix(f.Path, usermw.FileName) })
	svcs := slices.IndexFunc(files, func(f File) bool { return strings.HasSuffix(f.Path, "10-hashi-http-services.yml") })
	if user < 0 || svcs < 0 || user > svcs {
		t.Fatalf("user file at %d, services at %d", user, svcs)
	}
	f := files[user]
	if bytes.Contains(f.Data, []byte("comments are dropped")) {
		t.Error("the user's comment reached the host file")
	}
	if !bytes.Contains(f.Data, []byte("maxRequestBodyBytes: 2147483648\n")) {
		t.Errorf("numbers must stay numbers:\n%s", f.Data)
	}
	// Verify expects the user's middlewares like Hashi's own.
	want, err := Expected(files)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"middleware office-only@file", "middleware big-uploads@file", "middleware no-frames@file"} {
		if !slices.Contains(want, n) {
			t.Errorf("Verify does not expect %s", n)
		}
	}
}

// routerMiddlewares returns each router's middleware list from a rendered services file.
func routerMiddlewares(t *testing.T, data []byte) map[string][]string {
	t.Helper()
	var c dynamicConfig
	if err := yaml.Unmarshal(data, &c); err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{}
	for n, r := range c.HTTP.Routers {
		out[n] = r.Middlewares
	}
	return out
}

// 04 §1.1a chain order: Hashi's defaults with forward auth first, then traffic headers, every-service, service, route
// middlewares, then the rewrite. The system service gets none of the user's.
func TestTrafficChainOrder(t *testing.T) {
	files, err := Render(trafficOptions(t, []string{"no-frames"}))
	if err != nil {
		t.Fatal(err)
	}
	got := routerMiddlewares(t, fileNamed(t, files, "10-hashi-http-services.yml").Data)
	defaults := []string{MiddlewareHeaders, MiddlewareCompress, MiddlewareAuthOpen}
	want := map[string][]string{
		"svc-hashi-admin": append([]string{MiddlewareEdge}, defaults...),
		"svc-plain":       append(slices.Clone(defaults), "no-frames"),
		"svc-custom":      append(slices.Clone(defaults), "svc-custom-traffic", "no-frames"),
		"svc-vault":       append(slices.Clone(defaults), "svc-vault-traffic", "no-frames", "big-uploads"),
		"svc-files":       append(slices.Clone(defaults), "no-frames", "office-only"),
		"svc-files-r-01jr0000000000000000000011": append(slices.Clone(defaults), "no-frames", "office-only", "big-uploads",
			"svc-files-r-01jr0000000000000000000011"),
		"svc-files-r-01jr0000000000000000000012": append(slices.Clone(defaults), "no-frames", "office-only"),
	}
	for r, w := range want {
		if !slices.Equal(got[r], w) {
			t.Errorf("%s: %v, want %v", r, got[r], w)
		}
	}
}

// The Traefik setting "middlewares on every service" (09 §2) changes the render: with it every user service's router
// lists the middleware, without it none does, and the system service never does.
func TestEveryServiceSettingChangesRender(t *testing.T) {
	render := func(every []string) map[string][]string {
		files, err := Render(trafficOptions(t, every))
		if err != nil {
			t.Fatal(err)
		}
		return routerMiddlewares(t, fileNamed(t, files, "10-hashi-http-services.yml").Data)
	}
	with, without := render([]string{"office-only"}), render(nil)
	for r, mws := range with {
		user := !strings.HasPrefix(r, "svc-hashi-admin")
		if got := slices.Contains(mws, "office-only"); got != user && r != "svc-files" && !strings.HasPrefix(r, "svc-files-") {
			t.Errorf("%s with the setting: office-only listed = %v", r, got)
		}
		if r == "svc-plain" && slices.Contains(without[r], "office-only") {
			t.Errorf("%s without the setting still lists office-only", r)
		}
	}
	if !slices.Equal(with["svc-files"][3:], []string{"office-only"}) {
		t.Errorf("a name that is both on every service and the service's own is listed once: %v", with["svc-files"])
	}
}

// Rendered traffic files match the pinned Traefik schema (04 §9).
func TestTrafficFilesMatchSchema(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "..", "testdata", "contracts", "traefik", "schema_v3_file_provider.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	doc, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("s.json", doc); err != nil {
		t.Fatal(err)
	}
	schema, err := c.Compile("s.json")
	if err != nil {
		t.Fatal(err)
	}
	files, err := Render(trafficOptions(t, []string{"no-frames"}))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"10-hashi-http-services.yml", usermw.FileName} {
		var v any
		if err := yaml.Unmarshal(fileNamed(t, files, name).Data, &v); err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(v)
		inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(inst); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// Identical options render identical bytes (invariant 8), user file included.
func TestTrafficRenderStable(t *testing.T) {
	a, err := Render(trafficOptions(t, []string{"no-frames"}))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Render(trafficOptions(t, []string{"no-frames"}))
	if err != nil {
		t.Fatal(err)
	}
	for i := range a {
		if !bytes.Equal(a[i].Data, b[i].Data) {
			t.Errorf("%s differs between two renders", a[i].Path)
		}
	}
}
