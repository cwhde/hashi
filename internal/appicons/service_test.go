package appicons

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// library serves the captured index subset and one icon with its white variant.
func library(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	index, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "selfhst", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var indexHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/index.json":
			indexHits.Add(1)
			_, _ = w.Write(index)
		case "/svg/gitea.svg":
			_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0"/></svg>`))
		case "/svg/gitea-light.svg":
			_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><path fill="#fff" d="M0 0"/></svg>`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &indexHits
}

func TestLibrarySearchRanksLikePocketID(t *testing.T) {
	srv, hits := library(t)
	l := NewLibrary(srv.URL, srv.Client())
	ctx := context.Background()
	got, err := l.Search(ctx, "gitea")
	if err != nil || len(got) == 0 || got[0].Reference != "gitea" || got[0].Format != "svg" || !got[0].Light {
		t.Fatalf("exact %+v %v", got, err)
	}
	got, _ = l.Search(ctx, "home assistant")
	if len(got) == 0 || got[0].Reference != "home-assistant" {
		t.Fatalf("spaces vs dashes %+v", got)
	}
	got, _ = l.Search(ctx, "git") // the prefix gitea before tag-only matches
	if len(got) < 2 || got[0].Reference != "gitea" {
		t.Fatalf("prefix then tags %+v", got)
	}
	if all, _ := l.Search(ctx, ""); len(all) == 0 || len(all) > maxResults {
		t.Fatalf("empty query %d", len(all))
	}
	if _, err := l.Lookup(ctx, "../etc/passwd"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad reference %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("index fetched %d times within a day", hits.Load())
	}
	l.now = func() time.Time { return time.Now().Add(25 * time.Hour) }
	srv.Close() // a stale index still answers while the CDN is away
	if got, err := l.Search(ctx, "gitea"); err != nil || len(got) == 0 {
		t.Fatalf("stale %v", err)
	}
}

// to sends every request, whatever its address, to srv: the apps' public names and upstreams stay in the test.
func to(srv *httptest.Server) *http.Client {
	tr := srv.Client().Transport.(*http.Transport).Clone()
	tr.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, srv.Listener.Addr().String())
	}
	tr.DialTLSContext = nil
	return &http.Client{Transport: tr, Timeout: 5 * time.Second}
}

func appFixture(t *testing.T, libURL string, c *http.Client) (*Service, string) {
	t.Helper()
	ctx := context.Background()
	st := storetest.New(t)
	if err := settings.Put(ctx, st.Q, settings.General{RootDomain: "example.com", AdminDomain: "hashi.example.com",
		InternalURL: "http://192.0.2.5:3340", SyncIntervalSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	v, _ := vault.New(bytes.Repeat([]byte{7}, vault.KeySize), 1)
	rec, _ := v.Seal(vault.Service, "dns.token", secrets.New("t"))
	if err := st.Q.InsertSecret(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertDNSProvider(ctx, db.InsertDNSProviderParams{ID: "p1", Name: "hetzner", Type: "hetzner", DefaultTtl: 3600,
		TokenSecretID: rec.ID, Health: "healthy", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := st.Q.InsertDNSZone(ctx, db.InsertDNSZoneParams{ID: "z1", ProviderID: "p1", ProviderZoneID: "1", Name: "example.com", ZoneTtl: 3600, CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	svc, err := services.Create(ctx, st.Q, services.Service{Name: "git", Slug: "git", Kind: services.KindHTTP, DomainMode: services.ModeSubdomain,
		HostName: "git.example.com", Enabled: true, DashboardVisible: true, Origin: services.OriginUser,
		Target: services.Target{Address: "192.0.2.20", Port: 3000}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	s, err := New(st, t.TempDir(), Finder{Client: c, Upstream: c}, NewLibrary(libURL, c), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return s, Key("service", svc.ID)
}

// 07 §9.1: an upload, a selfh.st icon (with its white variant) and the automatic one each fill the app's icon; files
// are served only for what is stored; an app without any icon has none (the letter shows).
func TestAppIconModes(t *testing.T) {
	ctx := context.Background()
	srv, _ := library(t)
	s, key := appFixture(t, srv.URL, srv.Client())
	apps, err := s.Apps(ctx)
	if err != nil || len(apps) != 1 || apps[0].Upstream != "http://192.0.2.20:3000/" || apps[0].Public != "https://git.example.com/" {
		t.Fatalf("apps %+v %v", apps, err)
	}

	if _, err := s.Upload(ctx, key, []byte("<html></html>")); !errors.Is(err, ErrInvalid) {
		t.Fatalf("html %v", err)
	}
	if _, err := s.Upload(ctx, "service-nope", png); !errors.Is(err, ErrNoApp) {
		t.Fatalf("unknown app %v", err)
	}
	r, err := s.Upload(ctx, key, png)
	if icon, light := URLs(r); err != nil || r.Mode != ModeUpload || !strings.HasPrefix(icon, "/assets/apps/"+key+"?v=") || light != "" {
		t.Fatalf("upload %+v %v", r, err)
	}

	r, err = s.UseLibrary(ctx, key, "gitea")
	if icon, light := URLs(r); err != nil || r.Mode != ModeLibrary || r.LibraryRef != "gitea" || icon == "" || light == "" {
		t.Fatalf("library %+v %v", r, err)
	}
	f, ct, _, err := s.Open(ctx, key+"-light")
	if err != nil || ct != "image/svg+xml" {
		t.Fatalf("open light %q %v", ct, err)
	}
	_ = f.Close()

	// Automatic: the app answers nothing, so the letter shows and the white variant is gone.
	nothing := httptest.NewServer(http.NotFoundHandler())
	defer nothing.Close()
	s.Find = Finder{Client: to(nothing), Upstream: to(nothing)}
	r, err = s.Auto(ctx, key)
	if icon, light := URLs(r); err != nil || r.Mode != ModeAuto || icon != "" || light != "" {
		t.Fatalf("auto %+v %v", r, err)
	}
	if _, _, _, err := s.Open(ctx, key); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("open after auto %v", err)
	}
	if _, _, _, err := s.Open(ctx, "../hashi.db"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("open outside %v", err)
	}
}

// Refresh looks at an automatic icon once a day, never at a chosen one, and removes files of apps that are gone.
func TestRefreshOncePerDay(t *testing.T) {
	ctx := context.Background()
	var asked atomic.Int32
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		http.NotFound(w, r)
	}))
	defer app.Close()
	srv, _ := library(t)
	s, key := appFixture(t, srv.URL, srv.Client())
	s.Find = Finder{Client: to(app), Upstream: to(app)}
	if err := s.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(s.dir, "service-gone"), png, 0o600)
	before := asked.Load()
	if err := s.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if asked.Load() != before {
		t.Fatal("asked again within a day")
	}
	if _, err := os.Stat(filepath.Join(s.dir, "service-gone")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a gone app's file stayed")
	}
	if _, err := s.UseLibrary(ctx, key, "gitea"); err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Now().Add(48 * time.Hour) }
	if err := s.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if r, _ := s.Icons(ctx); r[key].Mode != ModeLibrary {
		t.Fatalf("a chosen icon was replaced: %+v", r[key])
	}
}
