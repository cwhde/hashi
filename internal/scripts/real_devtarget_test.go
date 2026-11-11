//go:build integration

package scripts_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	. "git.juzo.io/juzo/hashi/internal/scripts"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// TestRealScriptsOnDevTarget proves 09 §1 against a real Debian host with systemd: apply writes the runner, script, env
// file and timer; the timer really fires and its run is collected with the secret redacted; a manual run and a timeout;
// delete removes everything. It changes the machine and cleans up after itself.
// HASHI_FW_TARGET=user@addr, HASHI_FW_KEY=<private key file> (a dev target only).
func TestRealScriptsOnDevTarget(t *testing.T) {
	target, keyFile := os.Getenv("HASHI_FW_TARGET"), os.Getenv("HASHI_FW_KEY")
	if target == "" || keyFile == "" {
		t.Skip("set HASHI_FW_TARGET and HASHI_FW_KEY")
	}
	user, addr, _ := strings.Cut(target, "@")
	key, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	port := os.Getenv("HASHI_FW_PORT") // a tunnel, where the test binary cannot reach the network itself
	if port == "" {
		port = "22"
	}
	hk, err := sshx.Scan(ctx, addr+":"+port, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	dial := func(ctx context.Context, _ string) (Host, error) {
		return sshx.Dial(ctx, sshx.Config{Addr: addr + ":" + port, Pinned: hk, Auth: sshx.Auth{User: user, PrivateKey: secrets.New(string(key))}})
	}
	h, err := dial(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.Close() }()
	sh := func(cmd string) string {
		t.Helper()
		r, err := h.Run(ctx, cmd, nil)
		if err != nil || !r.OK() {
			t.Fatalf("%s: %v\n%s%s", cmd, err, r.Stdout, r.Stderr)
		}
		return string(r.Stdout)
	}
	cleanup := func() {
		_, _ = h.Run(ctx, "for t in /etc/systemd/system/hashi-script-*.timer; do [ -e \"$t\" ] && systemctl disable --now \"$(basename \"$t\")\"; done; "+
			"rm -f /etc/systemd/system/hashi-script-*; systemctl daemon-reload; rm -rf /opt/hashi/scripts /var/lib/hashi/scripts; true", nil)
	}
	cleanup()
	defer cleanup()

	st := storetest.New(t)
	if err := st.Q.InsertFirewallHost(ctx, db.InsertFirewallHostParams{ID: "dev", Name: "dev", EdgeMode: "managed", SshHost: addr,
		SshPort: 22, SshUser: user, Auth: "hashi_key", HostKey: []byte{1}, SubnetsJson: "[]", NetbirdMode: "disabled", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	v, err := vault.New(make([]byte, vault.KeySize), 1)
	if err != nil {
		t.Fatal(err)
	}
	al := audit.New([]byte("k"))
	s := New(st, v, al, slog.New(slog.NewTextHandler(io.Discard, nil)), dial)
	eng := hsync.New(st, al, s.Subsystems()...)
	if err := eng.Start(ctx); err != nil {
		t.Fatal(err)
	}
	applyAll := func() {
		t.Helper()
		p, err := eng.Propose(ctx, Subsystem, "dev", hsync.TriggerManual, "admin")
		if err != nil {
			t.Fatal(err)
		}
		if p == nil {
			return
		}
		if err := eng.Approve(ctx, p.ID, "admin", ""); err != nil {
			t.Fatal(err)
		}
		done, err := eng.Apply(ctx, p.ID, "admin", "")
		if err != nil || done.Status != hsync.StatusApplied {
			t.Fatalf("apply %+v %v", done, err)
		}
		if again, err := eng.Propose(ctx, Subsystem, "dev", hsync.TriggerManual, "admin"); err != nil || again != nil {
			t.Fatalf("the host still differs after apply: %+v %v", again, err)
		}
	}

	every, err := s.Create(ctx, "admin", "", Input{Name: "Every minute", Enabled: true, Schedule: "* * * * *", TimeoutSeconds: 30, AllHosts: true,
		Body: "#!/bin/sh\nset -eu\necho \"keep=$KEEP token=$TOKEN\"\ndf -P / | tail -n 1 | awk '{print \"root used \" $5}'\n",
		Env:  []EnvVar{{Name: "KEEP", Value: secrets.New("14")}, {Name: "TOKEN", Value: secrets.New("dev-s3cret-value"), Secret: true}}})
	if err != nil {
		t.Fatal(err)
	}
	slow, err := s.Create(ctx, "admin", "", Input{Name: "Slow", Enabled: true, TimeoutSeconds: 2, AllHosts: true, Body: "#!/bin/sh\necho start\nsleep 30\n"})
	if err != nil {
		t.Fatal(err)
	}
	applyAll()
	t.Logf("files:\n%s", sh("stat -c '%a %U:%G %n' /opt/hashi/scripts /opt/hashi/scripts/* /etc/systemd/system/hashi-script-*"))
	if modes := sh("stat -c '%a %n' /opt/hashi/scripts/every-minute.env /opt/hashi/scripts/every-minute.sh"); !strings.Contains(modes, "600 /opt/hashi/scripts/every-minute.env") ||
		!strings.Contains(modes, "750 /opt/hashi/scripts/every-minute.sh") {
		t.Fatalf("modes %s", modes)
	}
	var timers []struct {
		Next int64  `json:"next"`
		Unit string `json:"unit"`
	}
	if err := json.Unmarshal([]byte(sh("systemctl list-timers --all --output=json hashi-script-every-minute.timer")), &timers); err != nil || len(timers) != 1 || timers[0].Next == 0 {
		t.Fatalf("timer %+v %v", timers, err)
	}

	// A manual run, then a run past its timeout.
	waitRun := func(id string) db.ScriptRun {
		t.Helper()
		for range 120 {
			r, err := s.GetRun(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			if r.Status != RunRunning {
				return r
			}
			time.Sleep(500 * time.Millisecond)
		}
		t.Fatal("run did not finish")
		return db.ScriptRun{}
	}
	id, err := s.Start(ctx, "admin", "", every.ID, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if r := waitRun(id); r.Status != RunSucceeded || !strings.Contains(r.Output, "keep=14 token=[secret]") || strings.Contains(r.Output, "dev-s3cret") {
		t.Fatalf("manual run %+v", r)
	}
	id, err = s.Start(ctx, "admin", "", slow.ID, "dev")
	if err != nil {
		t.Fatal(err)
	}
	if r := waitRun(id); r.Status != RunTimedOut || !strings.Contains(r.Output, "start") {
		t.Fatalf("slow run %+v", r)
	}

	// The timer fires on its own within about a minute; its run lands in the history.
	deadline := time.Now().Add(150 * time.Second)
	for {
		if _, err := s.Collect(ctx); err != nil {
			t.Fatal(err)
		}
		runs, _ := s.Runs(ctx, every.ID)
		if len(runs) > 0 && runs[0].Trigger == TriggerScheduled {
			r, _ := s.GetRun(ctx, runs[0].ID)
			if r.Status != RunSucceeded || !strings.Contains(r.Output, "keep=14 token=[secret]") || !strings.Contains(r.Output, "root used") {
				t.Fatalf("scheduled run %+v", r)
			}
			t.Logf("scheduled run output: %q", r.Output)
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no scheduled run collected; spool: %s", sh("ls -la /var/lib/hashi/scripts/spool 2>&1; systemctl status hashi-script-every-minute.service 2>&1 | tail -5; true"))
		}
		time.Sleep(10 * time.Second)
	}
	if left := sh("ls /var/lib/hashi/scripts/spool | wc -l"); strings.TrimSpace(left) != "0" {
		t.Fatalf("spool not emptied: %s", left)
	}

	// Delete both: timers stopped, files and units gone, manifest lists no script.
	for _, sc := range []Script{every, slow} {
		if err := s.Delete(ctx, "admin", "", sc.ID); err != nil {
			t.Fatal(err)
		}
	}
	applyAll()
	if out := sh("ls /etc/systemd/system | grep -c '^hashi-script-' || true"); strings.TrimSpace(out) != "0" {
		t.Fatalf("units left: %s", out)
	}
	if out := sh("ls /opt/hashi/scripts"); strings.TrimSpace(out) != "manifest.json\nrun.sh" {
		t.Fatalf("files left: %q", out)
	}
	t.Logf("manifest after delete: %s", sh("cat /opt/hashi/scripts/manifest.json"))
}
