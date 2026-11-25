package app

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"git.juzo.io/juzo/hashi/internal/assets"
	"git.juzo.io/juzo/hashi/internal/httpapi"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/monitor"
	"git.juzo.io/juzo/hashi/internal/pulse"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

// Review of S7, 07 §7: a public page's failure shows visitors nothing of the error.
func TestPublicMuxHidesErrors(t *testing.T) {
	st := storetest.New(t)
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	var on atomic.Bool
	on.Store(true)
	h := publicMux(&httpapi.Server{Store: st, Log: slog.New(slog.DiscardHandler)}, "status", &on)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/public/status", nil))
	if rec.Code != 500 || strings.Contains(rec.Body.String(), "sql") || strings.Contains(rec.Body.String(), "database") {
		t.Fatalf("%d %q", rec.Code, rec.Body)
	}
	on.Store(false)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/public/status", nil))
	if rec.Code != 404 {
		t.Fatalf("disabled %d", rec.Code)
	}
}

// A host gets the install script from Hashi without a session, and it names the release to download from (08 §1).
func TestAdminMuxServesPulseInstall(t *testing.T) {
	in := start(t, newConfig(t))
	type answer struct {
		Code int
		Body string
	}
	get := func(path string) answer {
		t.Helper()
		res, err := http.Get("http://" + in.addr + path) //nolint:noctx // a test against localhost
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = res.Body.Close() }()
		b, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		return answer{res.StatusCode, string(b)}
	}
	for _, p := range []string{"/api/pulse/install", "/api/pulse/install.sh"} {
		r := get(p)
		if r.Code != 200 || !strings.HasPrefix(r.Body, "#!/bin/sh") || !strings.Contains(r.Body, pulse.ReleaseBase) || strings.Contains(r.Body, "@@") {
			t.Fatalf("script %s: %d %q", p, r.Code, r.Body)
		}
	}
	if r := get("/api/pulse/download/amd64"); r.Code == 200 && strings.HasPrefix(r.Body, "\x7fELF") {
		t.Fatal("the Hashi image no longer carries the agent")
	}
	// A heartbeat reaches its handler without a session, and a wrong token gets no further.
	res, err := http.Post("http://"+in.addr+"/api/pulse/nobody/heartbeat", "application/json", strings.NewReader(`{"version":"1","hostname":"h","candidates":[]}`)) //nolint:noctx // a test against localhost
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest { // no Authorization header at all
		t.Fatalf("heartbeat without a token: %d", res.StatusCode)
	}
	// A huge body is refused before it is read into memory.
	req, err := http.NewRequest(http.MethodPost, "http://"+in.addr+"/api/pulse/nobody/heartbeat", strings.NewReader(`{"version":"`+strings.Repeat("x", 1<<20)+`","hostname":"h","candidates":[]}`)) //nolint:noctx // a test against localhost
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer x")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversized heartbeat: %d", res.StatusCode)
	}
}

// The public page switches (Settings → Appearance) turn each page on and off on its own listener (07 §7, §8).
func TestPublicPageSettings(t *testing.T) {
	p := newPublicToggles()
	p.update(settings.Appearance{Theme: "graphite", PublicDashboardEnabled: false, PublicStatusEnabled: true})
	srv := &httpapi.Server{Store: storetest.New(t), Log: slog.New(slog.DiscardHandler)}
	for page, off := range map[string]bool{"apps": true, "status": false} {
		enabled := &p.apps
		if page == "status" {
			enabled = &p.status
		}
		rec := httptest.NewRecorder()
		publicMux(srv, page, enabled).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		// Off is 404; on serves the page (503 where the web UI is not built, as in CI's check job).
		if got := rec.Code == http.StatusNotFound; got != off {
			t.Errorf("%s: %d, switched off %v", page, rec.Code, off)
		}
	}
}

// 07 §9: each public listener serves the asset slots (its page needs them), and nothing when the page is off.
func TestPublicMuxServesAssets(t *testing.T) {
	st := storetest.New(t)
	store, err := assets.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89")
	ct, etag, err := store.Put(assets.Icon, png)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Put(context.Background(), st.Q, settings.PublicAssets{Slots: map[string]settings.AssetSlot{
		"icon": {Origin: settings.AssetUploaded, ContentType: ct, ETag: etag}}}); err != nil {
		t.Fatal(err)
	}
	api := &httpapi.Server{Store: st, Assets: store, Log: slog.New(slog.DiscardHandler)}
	for _, page := range []string{"apps", "status"} {
		var on atomic.Bool
		on.Store(true)
		h := publicMux(api, page, &on)
		for path, want := range map[string]int{"/assets/icon": 200, "/assets/logo_dark": 404, "/assets/hashi.db": 404} {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != want {
				t.Errorf("%s %s: %d", page, path, rec.Code)
			}
		}
		on.Store(false)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/icon", nil))
		if rec.Code != 404 {
			t.Errorf("%s off: %d", page, rec.Code)
		}
	}
}

// 07 §7.2, §7.3: the status listener serves each entry's service page and badges; an entry with Status off, an
// unknown kind and a kind the entry lacks answer 404, and the apps listener serves neither.
func TestStatusListenerServesServicePagesAndBadges(t *testing.T) {
	st := storetest.New(t)
	ctx := t.Context()
	mon := monitor.New(st, func(context.Context) error { return nil }, nil, nil)
	id, err := mon.CreateUser(ctx, "Lamp", "Home", monitor.CheckSpec{Type: monitor.TypeTCP, Target: "127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	public := func(on bool) {
		t.Helper()
		if err := mon.Update(ctx, id, monitor.Settings{Group: "Home", IntervalSeconds: 60, TimeoutSeconds: 5, Public: on}); err != nil {
			t.Fatal(err)
		}
	}
	public(true)
	api := &httpapi.Server{Store: st, Monitor: mon, Log: slog.New(slog.DiscardHandler)}
	var on atomic.Bool
	on.Store(true)
	status, apps := publicMux(api, "status", &on), publicMux(api, "apps", &on)
	get := func(h http.Handler, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec
	}
	if r := get(status, "/api/public/status/lamp?range=7d"); r.Code != 200 || !strings.Contains(r.Body.String(), `"slug":"lamp"`) {
		t.Fatalf("service page: %d %s", r.Code, r.Body)
	}
	if r := get(status, "/api/public/status/lamp?range=1y"); r.Code != 200 {
		t.Fatalf("an unknown range shows the 24 h page: %d", r.Code)
	}
	r := get(status, "/badge/lamp/status.svg")
	if r.Code != 200 || !strings.HasPrefix(r.Body.String(), "<svg") {
		t.Fatalf("badge: %d %s", r.Code, r.Body)
	}
	h := r.Header()
	if h.Get("Content-Type") != "image/svg+xml" || h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Cache-Control") != "public, max-age=60" ||
		h.Get("Content-Security-Policy") != "default-src 'none'; style-src 'unsafe-inline'" {
		t.Fatalf("badge headers %v", h)
	}
	for _, p := range []string{"/badge/lamp/uptime-24h.svg", "/badge/lamp/response-24h.svg"} {
		if r := get(status, p); r.Code != 200 {
			t.Errorf("%s: %d", p, r.Code)
		}
	}
	for _, p := range []string{"/badge/lamp/certificate.svg", "/badge/lamp/nope.svg", "/badge/lamp/status.png", "/badge/gone/status.svg", "/api/public/status/gone"} {
		if r := get(status, p); r.Code != 404 {
			t.Errorf("%s: %d, want 404", p, r.Code)
		}
	}
	// The HTML names the page as its script does, for link previews and bookmarks.
	if err := settings.Put(ctx, st.Q, settings.General{RootDomain: "example.com", AdminDomain: "hashi.example.com",
		InternalURL: "http://192.0.2.10:3340", SyncIntervalSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		page, path, want string
		missing          bool
	}{
		{"status", "/", "Status · example.com", false},
		{"status", "/s/lamp", "Lamp · Status · example.com", false},
		{"status", "/s/gone", "Status · example.com", true}, // answered 404 (bug 172)
		{"apps", "/", "Apps · example.com", false},
	} {
		if got, missing := api.PublicTitle(httptest.NewRequest(http.MethodGet, c.path, nil), c.page); got != c.want || missing != c.missing {
			t.Errorf("%s %s: title %q missing %v, want %q %v", c.page, c.path, got, missing, c.want, c.missing)
		}
	}
	// The page itself: served (503 where the web UI is not built, as in CI's check job), never on the apps listener.
	if r := get(status, "/s/lamp"); r.Code == 404 {
		t.Error("/s/lamp: 404 on the status listener")
	}
	for _, p := range []string{"/s/lamp", "/badge/lamp/status.svg", "/api/public/status/lamp"} {
		if r := get(apps, p); r.Code != 404 {
			t.Errorf("apps listener %s: %d", p, r.Code)
		}
	}
	public(false)
	for _, p := range []string{"/badge/lamp/status.svg", "/api/public/status/lamp"} {
		if r := get(status, p); r.Code != 404 {
			t.Errorf("Status off %s: %d, want 404", p, r.Code)
		}
	}
}

// The pages' title (07 §8.1, bug 173): set in Settings → Public pages, it replaces the root domain in both pages' own
// title and in their tab titles; empty is the domain.
func TestPublicPageTitleSetting(t *testing.T) {
	st := storetest.New(t)
	ctx := context.Background()
	if err := settings.Put(ctx, st.Q, settings.General{RootDomain: "example.com", AdminDomain: "hashi.example.com",
		InternalURL: "http://192.0.2.10:3340", SyncIntervalSeconds: 3600}); err != nil {
		t.Fatal(err)
	}
	api := &httpapi.Server{Store: st, Monitor: monitor.New(st, func(context.Context) error { return nil }, nil, nil),
		Log: slog.New(slog.DiscardHandler)}
	tab := func() string {
		got, _ := api.PublicTitle(httptest.NewRequest(http.MethodGet, "/", nil), "apps")
		return got
	}
	own := func() string {
		r, err := api.GetPublicApps(ctx, gen.GetPublicAppsRequestObject{})
		if err != nil {
			t.Fatal(err)
		}
		return r.(gen.GetPublicApps200JSONResponse).Title
	}
	if tab() != "Apps · example.com" || own() != "example.com" {
		t.Fatalf("without a title: %q, %q", tab(), own())
	}
	if err := settings.Put(ctx, st.Q, settings.PublicAssets{Title: "Juzo's lab"}); err != nil {
		t.Fatal(err)
	}
	if tab() != "Apps · Juzo's lab" || own() != "Juzo's lab" {
		t.Fatalf("with a title: %q, %q", tab(), own())
	}
}

// The public pages start in the configured theme (bug 178); a custom one brings its mode, canvas and text.
func TestPublicPageStartsInItsTheme(t *testing.T) {
	st := storetest.New(t)
	ctx := context.Background()
	api := &httpapi.Server{Store: st, Log: slog.New(slog.DiscardHandler)}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if err := settings.Put(ctx, st.Q, settings.Appearance{Theme: "linen", PublicDashboardEnabled: true, PublicStatusEnabled: true}); err != nil {
		t.Fatal(err)
	}
	if th, mode, _, _ := api.PublicTheme(req); th != "linen" || mode != "light" {
		t.Fatalf("linen: %q %q", th, mode)
	}
	custom := &settings.CustomTheme{Canvas: "#f8f8f8", Surface: "#ffffff", Text: "#111111", Muted: "#555555", Border: "#cccccc",
		Accent: "#2255aa", Roundness: "default"}
	if err := settings.Put(ctx, st.Q, settings.Appearance{Theme: "custom", CustomTheme: custom, PublicDashboardEnabled: true, PublicStatusEnabled: true}); err != nil {
		t.Fatal(err)
	}
	if th, mode, canvas, text := api.PublicTheme(req); th != "custom" || mode != "light" || canvas != "#f8f8f8" || text != "#111111" {
		t.Fatalf("custom: %q %q %q %q", th, mode, canvas, text)
	}
}
