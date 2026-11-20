package appicons

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var png = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89")

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "appicons", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func first(t *testing.T, head, manifest string) Candidate {
	t.Helper()
	base, _ := url.Parse("https://app.example.com/")
	c, m := ParseHead(fixture(t, head), base)
	if manifest != "" {
		if m == "" {
			t.Fatalf("%s: no manifest link", head)
		}
		mu, _ := url.Parse(m)
		c = append(c, ParseManifest(fixture(t, manifest), mu)...)
	}
	if len(c) == 0 {
		t.Fatalf("%s: no candidates", head)
	}
	return Order(c)[0]
}

// The captured apps (testdata/contracts/appicons): each gets the icon that fits a card best.
func TestBestIconFromCapturedApps(t *testing.T) {
	for _, c := range []struct{ head, manifest, want string }{
		{"gitea-head.html", "gitea-manifest.json", ".svg"},                            // an SVG (the manifest's logo)
		{"seerr-head.html", "seerr-manifest.json", ""},                                // checked below: a manifest PNG ≥ 96
		{"arcane-head.html", "arcane-manifest.json", ""},                              // idem
		{"proxmox-head.html", "", "https://app.example.com/pve2/images/logo-128.png"}, // the only sized icon
		{"4get-head.html", "", "https://app.example.com/favicon.ico"},                 // a bare favicon
	} {
		got := first(t, c.head, c.manifest)
		switch {
		case c.want != "" && !strings.HasSuffix(got.URL, c.want):
			t.Errorf("%s: %+v, want %s", c.head, got, c.want)
		case c.want == "" && !got.SVG && got.Size < want:
			t.Errorf("%s: %+v is too small", c.head, got)
		}
	}
}

func TestOrderPrefersScalableThenSmallestThatFits(t *testing.T) {
	got := Order([]Candidate{
		{URL: "a16", Size: 16, Kind: 2}, {URL: "a512", Size: 512, Kind: 0}, {URL: "a180", Size: 180, Kind: 1},
		{URL: "a0", Kind: 2}, {URL: "a48", Size: 48, Kind: 2}, {URL: "svg", SVG: true, Kind: 2}, {URL: "a180", Size: 180},
	})
	var urls []string
	for _, c := range got {
		urls = append(urls, c.URL)
	}
	if strings.Join(urls, " ") != "svg a180 a512 a48 a16 a0" {
		t.Fatalf("order %v", urls)
	}
}

func TestParseHeadSkipsMaskAndDataIcons(t *testing.T) {
	base, _ := url.Parse("https://app.example.com/sub/page")
	c, m := ParseHead([]byte(`<link rel="mask-icon" href="m.svg"><link rel=icon href="data:image/png;base64,AA">`+
		`<link href='rel.png' rel='shortcut icon' sizes='16x16 64x64'><link rel="manifest" href="/m.json">`), base)
	if len(c) != 1 || c[0].URL != "https://app.example.com/sub/rel.png" || c[0].Size != 64 || m != "https://app.example.com/m.json" {
		t.Fatalf("%+v %q", c, m)
	}
	mu, _ := url.Parse(m)
	mc := ParseManifest([]byte(`{"icons":[{"src":"mono.png","sizes":"512x512","purpose":"monochrome"},{"src":"i.png","sizes":"192x192","purpose":"any maskable"}]}`), mu)
	if len(mc) != 1 || mc[0].URL != "https://app.example.com/i.png" {
		t.Fatalf("manifest %+v", mc)
	}
}

// A public address behind sign-in (401, as captured) or sending visitors to another host falls back to the upstream,
// asked with the app's own Host header; an app with nothing anywhere has no icon.
func TestFindFallsBackToTheUpstream(t *testing.T) {
	var upstreamHost string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHost = r.Host
		switch r.URL.Path {
		case "/":
			// An absolute link on the app's public name, as many apps write them: asked at the upstream as well.
			_, _ = w.Write([]byte(`<head><link rel="apple-touch-icon" href="http://` + r.Host + `/touch.png"></head>`))
		case "/touch.png":
			_, _ = w.Write(png)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	signin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<link rel="icon" href="/sso.png">`))
	}))
	defer signin.Close()
	public := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/guarded/":
			w.WriteHeader(http.StatusUnauthorized)
		case "/redirected/":
			http.Redirect(w, r, signin.URL+"/login", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer public.Close()
	f := NewFinder()
	f.Client = public.Client()
	f.Client.CheckRedirect = NewFinder().Client.CheckRedirect
	for _, p := range []string{"/guarded/", "/redirected/"} {
		got, err := f.Find(context.Background(), Target{Public: public.URL + p, Upstream: upstream.URL + "/"})
		if err != nil || string(got.Data) != string(png) || got.From != "upstream /touch.png" {
			t.Fatalf("%s: %+v %v", p, got.From, err)
		}
		if pub, _ := url.Parse(public.URL); upstreamHost != pub.Host {
			t.Fatalf("%s: upstream asked with Host %q", p, upstreamHost)
		}
	}
	if _, err := f.Find(context.Background(), Target{Public: public.URL + "/guarded/"}); !errors.Is(err, ErrNone) {
		t.Fatalf("no upstream: %v", err)
	}
}

// An icon link that answers with something other than an image is skipped for the next one.
func TestFindSkipsWhatIsNotAnImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<link rel="icon" sizes="192x192" href="/login-page"><link rel="icon" sizes="32x32" href="/i.png">`))
		case "/login-page":
			_, _ = w.Write([]byte(`<html>sign in</html>`))
		case "/i.png":
			_, _ = w.Write(png)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	f := NewFinder()
	f.Client = srv.Client()
	got, err := f.Find(context.Background(), Target{Public: srv.URL + "/"})
	if err != nil || got.From != "/i.png" || got.ContentType != "image/png" {
		t.Fatalf("%+v %v", got.From, err)
	}
}
