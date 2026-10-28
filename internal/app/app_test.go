package app

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"
)

func TestRunServesHealthAndShutsDownCleanly(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{DataDir: dir, AdminAddr: "127.0.0.1:0", PublicStatusAddr: "127.0.0.1:0", MasterKeyFile: writeMasterKey(t, dir), Stdout: io.Discard}
	ctx, cancel := context.WithCancel(context.Background())
	addrCh := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), func(a string) { addrCh <- a })
	}()
	var addr string
	select {
	case addr = <-addrCh:
	case err := <-done:
		t.Fatalf("Run exited early: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("not ready in time")
	}

	// No keep-alive: an unused spare connection would hold Shutdown for 5s (net/http treats StateNew as active).
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	for path, want := range map[string]int{"/healthz": 200, "/readyz": 200, "/api/version": 200, "/api/edge-auth/forward": 204, "/api/nope": 404} {
		res, err := client.Get("http://" + addr + path)
		if err != nil {
			t.Fatal(err)
		}
		var body struct {
			Status string            `json:"status"`
			Checks map[string]string `json:"checks"`
		}
		raw, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		_ = json.Unmarshal(raw, &body)
		if res.StatusCode != want {
			t.Fatalf("%s: status %d, want %d", path, res.StatusCode, want)
		}
		if path == "/readyz" && body.Checks["database"] != "ok" {
			t.Fatalf("readyz checks: %+v", body.Checks)
		}
	}

	// Client routes reach the SPA handler: 200 with a built UI, 503 without (make check does not build it).
	res, err := client.Get("http://" + addr + "/some/spa/route")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("/some/spa/route: status %d", res.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v after cancel", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown hung")
	}
}

func TestLoadConfig(t *testing.T) {
	env := map[string]string{"HASHI_ADMIN_ADDR": ":3340", "HASHI_PUBLIC_APPS_ADDR": "off", "HASHI_LOG_LEVEL": "debug"}
	c, err := LoadConfig([]string{"-data-dir", "/tmp/h"}, func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if c.AdminAddr != ":3340" || c.PublicAppsAddr != "" || c.PublicStatusAddr != ":8082" || c.DataDir != "/tmp/h" || c.LogLevel != slog.LevelDebug {
		t.Fatalf("%+v", c)
	}
	if _, err := LoadConfig([]string{"-admin-addr", "off"}, func(string) string { return "" }); err == nil {
		t.Fatal("admin listener must not be disableable")
	}
}
