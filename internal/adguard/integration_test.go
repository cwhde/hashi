//go:build integration

package adguard_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/adguard"
	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/itest"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// The whole path against a real AdGuard Home of a version running in production: manual rewrites survive everything.
func TestRealAdGuardPlanApplyVerify(t *testing.T) {
	c := itest.Run(t, 3000, nil, "adguard/adguardhome:v0.107.79")
	addr := c.Port(t, 3000)
	itest.WaitFor(t, time.Minute, "AdGuard install API", func() bool {
		r, err := http.Get("http://" + addr + "/control/install/get_addresses")
		if err != nil {
			return false
		}
		_ = r.Body.Close()
		return r.StatusCode == 200
	})
	body, _ := json.Marshal(map[string]any{"web": map[string]any{"ip": "0.0.0.0", "port": 3000}, "dns": map[string]any{"ip": "0.0.0.0", "port": 53}, "username": "admin", "password": "test-password-1"})
	r, err := http.Post("http://"+addr+"/control/install/configure", "application/json", strings.NewReader(string(body)))
	if err != nil || r.StatusCode != 200 {
		t.Fatalf("install: %v %v", r, err)
	}
	_ = r.Body.Close()
	ctx := t.Context()
	raw := adguard.Client{Base: "http://" + addr, Username: "admin", Password: secrets.New("test-password-1")}
	itest.WaitFor(t, 30*time.Second, "AdGuard API", func() bool { _, err := raw.Status(ctx); return err == nil })
	manual := adguard.Rewrite{Domain: "manual.example.com", Answer: "192.0.2.50"}
	if err := raw.Add(ctx, manual); err != nil {
		t.Fatal(err)
	}

	st := storetest.New(t)
	v, err := vault.New([]byte("0123456789abcdef0123456789abcdef"), 1)
	if err != nil {
		t.Fatal(err)
	}
	host, port, _ := strings.Cut(addr, ":")
	ip, err := netip.ParseAddr(strings.Replace(host, "localhost", "127.0.0.1", 1))
	if err != nil {
		t.Fatal(err)
	}
	var portN int
	_, _ = fmt.Sscan(port, &portN)
	resolve := func(context.Context, string, string) (ipsets.Target, error) {
		return ipsets.Target{Addresses: []netip.Addr{ip}}, nil
	}
	desired := []adguard.Desired{{Domain: "svc.example.com", Answer: "192.0.2.10", Origin: "service s1"}}
	s := adguard.New(st, v, resolve, func(context.Context) ([]adguard.Desired, error) { return desired, nil })
	eng := hsync.New(st, audit.New([]byte("salt")), s.Subsystems()...)

	if err := st.Q.InsertIPSet(ctx, db.InsertIPSetParams{ID: "t", Slug: "t", Name: "t", Kind: "user", Origin: "user"}); err != nil {
		t.Fatal(err)
	}
	conn, err := s.Create(ctx, adguard.Input{Name: "real", TargetSetID: "t", Port: portN, Scheme: "http", Username: "admin", Password: secrets.New("test-password-1"), TLSVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	if conn.Mode != adguard.ModeReadOnly {
		t.Fatalf("mode %s", conn.Mode)
	}
	// Read-only: nothing is written, though the preview shows what would be.
	if p, err := eng.Auto(ctx, adguard.Subsystem, conn.ID, hsync.TriggerManual, "t"); err != nil || p != nil {
		t.Fatalf("read-only plan %+v %v", p, err)
	}
	view, err := s.Rewrites(ctx, conn.ID)
	if err != nil || len(view.Planned) != 1 || len(view.Rewrites) != 1 || view.Rewrites[0].Owned {
		t.Fatalf("view %+v %v", view, err)
	}
	if err := setMode(ctx, s, conn, adguard.ModeActive, portN); err != nil {
		t.Fatal(err)
	}
	if p, err := eng.Auto(ctx, adguard.Subsystem, conn.ID, hsync.TriggerManual, "t"); err != nil || p == nil || p.Status != hsync.StatusApplied {
		t.Fatalf("apply %+v %v", p, err)
	}
	expect(t, raw, map[string]int{"manual.example.com=192.0.2.50": 1, "svc.example.com=192.0.2.10": 1})

	// A new address updates in place, the second run is quiet, and a stray duplicate of an owned rewrite goes.
	desired[0].Answer = "192.0.2.11"
	if p, err := eng.Auto(ctx, adguard.Subsystem, conn.ID, hsync.TriggerManual, "t"); err != nil || p.Status != hsync.StatusApplied {
		t.Fatalf("update %+v %v", p, err)
	}
	expect(t, raw, map[string]int{"manual.example.com=192.0.2.50": 1, "svc.example.com=192.0.2.11": 1})
	if p, err := eng.Auto(ctx, adguard.Subsystem, conn.ID, hsync.TriggerManual, "t"); err != nil || p != nil {
		t.Fatalf("not quiet: %+v %v", p, err)
	}

	// Deleting waits for approval, then removes only Hashi's rewrite, stray copies included.
	if err := raw.Add(ctx, adguard.Rewrite{Domain: "svc.example.com", Answer: "192.0.2.11"}); err != nil {
		t.Fatal(err)
	}
	desired = nil
	p, err := eng.Auto(ctx, adguard.Subsystem, conn.ID, hsync.TriggerManual, "t")
	if err != nil || p.Status != hsync.StatusPending {
		t.Fatalf("delete plan %+v %v", p, err)
	}
	expect(t, raw, map[string]int{"manual.example.com=192.0.2.50": 1, "svc.example.com=192.0.2.11": 2})
	if err := eng.Approve(ctx, p.ID, "t", ""); err != nil {
		t.Fatal(err)
	}
	if p, err = eng.Apply(ctx, p.ID, "t", ""); err != nil || p.Status != hsync.StatusApplied {
		t.Fatalf("delete %+v %v", p, err)
	}
	expect(t, raw, map[string]int{"manual.example.com=192.0.2.50": 1})
}

func setMode(ctx context.Context, s *adguard.Service, c db.AdguardConnection, mode string, port int) error {
	_, err := s.Update(ctx, c.ID, adguard.Input{Name: c.Name, TargetSetID: c.TargetSetID, Port: port, Scheme: c.Scheme, Username: c.Username, TLSVerify: true, Mode: mode, SaveInactive: false})
	return err
}

func expect(t *testing.T, c adguard.Client, want map[string]int) {
	t.Helper()
	l, err := c.List(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, r := range l {
		got[r.Domain+"="+r.Answer]++
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}
