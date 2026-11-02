package traefik

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"go.yaml.in/yaml/v3"

	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
)

// wafServices: a user service with WAF paths and exclusions next to advanced routes, a service with the WAF off, and
// the system admin service (default mode, uploads on).
func wafServices() []services.Service {
	all := testServices()
	files := services.Service{Name: "Files", Slug: "files", Kind: services.KindHTTP, HostName: "files.example.com",
		Enabled: true, Origin: services.OriginUser, Target: services.Target{Address: "192.0.2.70", Port: 8080},
		AdvancedRoutes: true,
		Routes: []services.Route{{ID: "01JR0000000000000000000010", Enabled: true,
			Targets: []services.Target{{Address: "192.0.2.71", Port: 8080}}, PathMatch: services.MatchPrefix,
			PathValue: "/api", Priority: 5, RewriteMode: services.RewriteNone}},
		WAF: services.WAF{Exclusions: []int{942100}, Paths: []services.WAFPath{
			{ID: "01JW0000000000000000000002", Match: services.MatchExact, Path: "/api/raw", Mode: settings.WAFDisabled},
			{ID: "01JW0000000000000000000001", Match: services.MatchPrefix, Path: "/remote.php", Uploads: true,
				Exclusions: []int{920230}},
		}}}
	off := services.Service{Name: "Legacy", Slug: "legacy", Kind: services.KindHTTP, HostName: "legacy.example.com",
		Enabled: true, Origin: services.OriginUser, Target: services.Target{Address: "192.0.2.72", Port: 80},
		WAF: services.WAF{Mode: settings.WAFDisabled}}
	return append([]services.Service{all[0], all[1], files, off}, all[5:7]...)
}

func renderWAF(t *testing.T) map[string]File {
	t.Helper()
	o := testOptions()
	o.Services = wafServices()
	o.WAFDefault = settings.WAFHeadersOnly
	files, err := Render(o)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]File{}
	for _, f := range files {
		out[filepath.Base(f.Path)] = f
	}
	return out
}

func TestRenderWAFGolden(t *testing.T) {
	files := renderWAF(t)
	golden(t, "waf-services.yml", files["10-hashi-http-services.yml"].Data)
	golden(t, "waf-security.yml", files["40-hashi-security.yml"].Data)
	golden(t, "traefik-waf.yaml", files["traefik.yaml"].Data)
	for _, name := range []string{".traefik.yml", "coraza-http-wasm.wasm"} {
		f, ok := files[name]
		if !ok || f.Kind != KindPlugin || f.Data != nil || f.Path != PluginDir+"/"+name {
			t.Fatalf("plugin file %s: %+v", name, f)
		}
		if sum, _ := PinnedPluginSum(f.Path); f.Hash() != sum {
			t.Errorf("%s hash %s, pinned %s", name, f.Hash(), sum)
		}
	}
}

func TestNoWAFRendersNoPlugin(t *testing.T) {
	o := testOptions()
	o.Services = wafServices()
	o.WAFDefault = settings.WAFDisabled
	o.Services[2].WAF = services.WAF{} // the WAF paths alone would turn it on for their paths
	files, err := Render(o)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Kind == KindPlugin || bytes.Contains(f.Data, []byte("localPlugins")) || bytes.Contains(f.Data, []byte(WAFMiddlewarePrefix)) {
			t.Fatalf("%s has WAF content with the WAF off everywhere", f.Path)
		}
	}
}

// The WAF paths of a router sit right above it and below the next router, the more specific path higher, and each
// router ends with exactly the WAF of its own resolution.
func TestWAFPathRouters(t *testing.T) {
	files := renderWAF(t)
	var c dynamicConfig
	if err := yaml.Unmarshal(files["10-hashi-http-services.yml"].Data, &c); err != nil {
		t.Fatal(err)
	}
	var sec dynamicConfig
	if err := yaml.Unmarshal(files["40-hashi-security.yml"].Data, &sec); err != nil {
		t.Fatal(err)
	}
	r := c.HTTP.Routers
	main, route := r["svc-files"], r["svc-files-r-01jr0000000000000000000010"]
	upload, raw := r["svc-files-w-01jw0000000000000000000001"], r["svc-files-w-01jw0000000000000000000002"]
	rUpload := r["svc-files-r-01jr0000000000000000000010-w-01jw0000000000000000000001"]
	ordered := main.Priority < upload.Priority && upload.Priority < raw.Priority && raw.Priority < route.Priority &&
		route.Priority < rUpload.Priority && rUpload.Priority < route.Priority+wafSlots
	if !ordered {
		t.Errorf("priorities main %d, /remote.php %d, /api/raw %d, route %d, route+/remote.php %d",
			main.Priority, upload.Priority, raw.Priority, route.Priority, rUpload.Priority)
	}
	if upload.Rule != "Host(`files.example.com`) && PathPrefix(`/remote.php`)" ||
		rUpload.Rule != "Host(`files.example.com`) && PathPrefix(`/api`) && PathPrefix(`/remote.php`)" {
		t.Errorf("rules %q / %q", upload.Rule, rUpload.Rule)
	}
	waf := func(rt router) []services.EffectiveWAF {
		var out []services.EffectiveWAF
		for _, m := range rt.Middlewares {
			if !strings.HasPrefix(m, WAFMiddlewarePrefix) {
				continue
			}
			d := strings.Join(sec.HTTP.Middlewares[m].Plugin[PluginName].Directives, "\n")
			out = append(out, services.EffectiveWAF{Uploads: strings.Contains(d, "PROPFIND"),
				Exclusions: removed(d)})
		}
		return out
	}
	if w := waf(main); len(w) != 1 || w[0].Uploads || !slices.Equal(w[0].Exclusions, []int{942100}) {
		t.Errorf("main router WAF %+v", w)
	}
	if w := waf(upload); len(w) != 1 || !w[0].Uploads || !slices.Equal(w[0].Exclusions, []int{920230, 920420, 942100}) {
		t.Errorf("/remote.php WAF %+v", w)
	}
	if w := waf(raw); len(w) != 0 {
		t.Errorf("/api/raw has the WAF off but got %+v", w)
	}
	if w := waf(r["svc-legacy"]); len(w) != 0 {
		t.Errorf("legacy has the WAF off but got %+v", w)
	}
	if w := waf(r["svc-hashi-admin"]); len(w) != 1 || !w[0].Uploads {
		t.Errorf("the admin service needs PUT and DELETE: %+v", w)
	}
	// Forward auth runs before the WAF: Stage A rejects blocked addresses before any rule is evaluated.
	if m := upload.Middlewares; m[len(m)-2] != MiddlewareAuthOpen {
		t.Errorf("chain %v", m)
	}
}

func removed(directives string) []int {
	var out []int
	for _, l := range strings.Split(directives, "\n") {
		var id int
		if n, _ := fmt.Sscanf(l, "SecRuleRemoveById %d", &id); n == 1 {
			out = append(out, id)
		}
	}
	return out
}

func zipOf(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for n, c := range files {
		f, err := w.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = f.Write([]byte(c))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestPluginArchiveChecksums(t *testing.T) {
	a := zipOf(t, map[string]string{".traefik.yml": "x", "coraza-http-wasm.wasm": "y"})
	if _, err := PluginArchive(a); !errors.Is(err, ErrChecksum) {
		t.Fatalf("an archive that is not the pinned one: %v", err)
	}
	sum := sha256.Sum256(a)
	if _, err := pluginArchive(a, hex.EncodeToString(sum[:])); !errors.Is(err, ErrChecksum) {
		t.Fatalf("files that are not the pinned ones: %v", err)
	}
	b := zipOf(t, map[string]string{"coraza-http-wasm.wasm": "y"})
	sum = sha256.Sum256(b)
	if _, err := pluginArchive(b, hex.EncodeToString(sum[:])); err == nil || errors.Is(err, ErrChecksum) {
		t.Fatalf("missing member: %v", err)
	}
}

// The plugin is fetched only when a host needs it, writing it restarts Traefik, and a host that has it plans nothing.
func TestApplyWAFPlugin(t *testing.T) {
	ctx := context.Background()
	h := newFakeHost()
	h.pinnedPlugin = true
	o := testOptions()
	o.Services = wafServices()
	o.WAFDefault = settings.WAFHeadersOnly
	files, err := Render(o)
	if err != nil {
		t.Fatal(err)
	}
	fetched := 0
	d := Desired{Paths: o.Paths, APIPort: 8080, Files: files, Service: fakeService(), PingTimeout: 1500 * time.Millisecond,
		Binary: func(context.Context) ([]byte, error) { return []byte("ELF"), nil },
		Plugin: func(context.Context) (map[string][]byte, error) {
			fetched++
			return map[string][]byte{PluginDir + "/.traefik.yml": []byte("m"), PluginDir + "/coraza-http-wasm.wasm": []byte("w")}, nil
		}}
	diff, err := Observe(ctx, h, d)
	if err != nil {
		t.Fatal(err)
	}
	plugin := 0
	for _, it := range diff.Items() {
		if strings.HasPrefix(it.Object, PluginDir) {
			plugin++
			if it.Risk != hsync.High || !strings.Contains(it.Note, PluginVersion) {
				t.Errorf("plugin item %+v", it)
			}
		}
	}
	if plugin != 2 || fetched != 0 {
		t.Fatalf("plugin items %d, fetched during the plan %d", plugin, fetched)
	}
	out, err := Apply(ctx, h, d, diff, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !out.Restarted || fetched != 1 || string(h.files[PluginDir+"/coraza-http-wasm.wasm"]) != "w" {
		t.Fatalf("restarted %v, fetched %d", out.Restarted, fetched)
	}
	if index(h.writes, "coraza-http-wasm.wasm") > index(h.writes, "traefik.yaml") {
		t.Errorf("the plugin must be in place before the static file that loads it: %v", h.writes)
	}
	h.version = Version
	again, err := Observe(ctx, h, d)
	if err != nil || !again.Empty() {
		t.Fatalf("second plan: %+v %v", again.Items(), err)
	}
	if fetched != 1 {
		t.Errorf("fetched again: %d", fetched)
	}
}
