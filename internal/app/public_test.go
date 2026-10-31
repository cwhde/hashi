package app

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"git.juzo.io/juzo/hashi/internal/httpapi"
	"git.juzo.io/juzo/hashi/internal/pulse"
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
	r := get("/api/pulse/install.sh")
	if r.Code != 200 || !strings.HasPrefix(r.Body, "#!/bin/sh") || !strings.Contains(r.Body, pulse.ReleaseBase) || strings.Contains(r.Body, "@@") {
		t.Fatalf("script: %d %q", r.Code, r.Body)
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
