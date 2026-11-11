package scripts_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	. "git.juzo.io/juzo/hashi/internal/scripts"
	"git.juzo.io/juzo/hashi/internal/firewall/firewalltest"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// itemState mirrors the plan item JSON.
type itemState struct {
	Body     string   `json:"body"`
	Schedule []string `json:"schedule"`
	Env      []string `json:"env"`
}

type fixture struct {
	s    *Service
	host *firewalltest.Host
	sub  hsync.Subsystem
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	st := storetest.New(t)
	v, err := vault.New(make([]byte, vault.KeySize), 1)
	if err != nil {
		t.Fatal(err)
	}
	h, err := firewalltest.NewHost("")
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"h1", "h2"} {
		p := db.InsertFirewallHostParams{ID: id, Name: "edge-" + id, EdgeMode: "managed", SshPort: 22, SshUser: "root",
			Auth: "hashi_key", SubnetsJson: "[]", NetbirdMode: "disabled", CreatedAt: 1, UpdatedAt: 1}
		if i == 0 {
			p.SshHost, p.HostKey = "192.0.2.1", []byte{1}
		} else { // h2 has no SSH (an external NAT edge), so no script can live there
			p.EdgeMode, p.SshUser = "external_nat", ""
		}
		if err := st.Q.InsertFirewallHost(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	s := New(st, v, audit.New([]byte("k")), slog.New(slog.NewTextHandler(io.Discard, nil)), func(ctx context.Context, id string) (Host, error) {
		if id != "h1" {
			return nil, errors.New("unreachable")
		}
		c, err := h.Connect(ctx, sshx.Config{Pinned: h.Key()})
		return c, err
	})
	return fixture{s: s, host: h, sub: s.Subsystems()[0]}
}

func (f fixture) plan(t *testing.T) hsync.Draft {
	t.Helper()
	d, err := f.sub.Plan(context.Background(), "h1")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (f fixture) apply(t *testing.T) {
	t.Helper()
	d := f.plan(t)
	if _, err := f.sub.Apply(context.Background(), "h1", d.Items); err != nil {
		t.Fatal(err)
	}
	if again := f.plan(t); len(again.Items) != 0 {
		t.Fatalf("after apply the host still differs: %+v", again.Items)
	}
}

func waitRun(t *testing.T, s *Service, id string) db.ScriptRun {
	t.Helper()
	for range 200 {
		r, err := s.GetRun(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if r.Status != RunRunning {
			return r
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the run never finished")
	return db.ScriptRun{}
}

// Save → plan (the diff, high risk) → apply (files, timer, manifest) → manual run → scheduled run collected → delete
// removes everything Hashi wrote (09 §1).
func TestScriptLifecycle(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	var planned []string
	f.s.OnSave(func(_ context.Context, ids []string, _ string) { planned = append(planned, ids...) })
	in := Input{Name: "Prune snapshots", Body: "#!/bin/sh\necho \"$TOKEN\"\n", Enabled: true, Schedule: "30 3 * * 0",
		TimeoutSeconds: 60, AllHosts: true, Env: []EnvVar{{Name: "KEEP", Value: secrets.New("14")},
			{Name: "TOKEN", Value: secrets.New("s3cret-value"), Secret: true}}}
	sc, err := f.s.Create(ctx, "admin", "", in)
	if err != nil {
		t.Fatal(err)
	}
	// Only the host Hashi can reach is a target; the other has no SSH.
	if sc.Slug != "prune-snapshots" || strings.Join(planned, ",") != "h1" {
		t.Fatalf("slug %q planned %v", sc.Slug, planned)
	}
	d := f.plan(t)
	if len(d.Items) != 2 || d.Items[0].Object != "runner "+Wrapper || d.Items[1].Object != "script prune-snapshots" {
		t.Fatalf("plan %+v", d.Items)
	}
	for _, it := range d.Items {
		if it.Risk != hsync.High {
			t.Fatalf("%s is not high risk", it.Object)
		}
	}
	var after itemState
	_ = json.Unmarshal(d.Items[1].After, &after)
	if after.Body != in.Body || after.Schedule[0] != "Sun *-*-* 03:30:00" || strings.Join(after.Env, ",") != "KEEP,TOKEN" {
		t.Fatalf("item %+v", after)
	}
	if strings.Contains(string(d.Items[1].After), "s3cret") {
		t.Fatal("a secret value is in the plan")
	}
	f.apply(t)
	env, _ := f.host.File(Dir + "/prune-snapshots.env")
	if !strings.Contains(string(env), "TOKEN=czNjcmV0LXZhbHVl\n") {
		t.Fatalf("env file %q", env)
	}
	if !strings.Contains(strings.Join(f.host.Ran(), "\n"), "systemctl restart hashi-script-prune-snapshots.timer") {
		t.Fatal("the timer was not started")
	}

	// A manual run returns at once and lands in the history.
	runID, err := f.s.Start(ctx, "admin", "", sc.ID, "h1")
	if err != nil {
		t.Fatal(err)
	}
	if r := waitRun(t, f.s, runID); r.Status != RunSucceeded || r.Output != "fake run of prune-snapshots\n" || r.Trigger != TriggerManual {
		t.Fatalf("run %+v", r)
	}
	if _, err := f.s.Start(ctx, "admin", "", sc.ID, "h2"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a run on a host the script is not on: %v", err)
	}

	// A scheduled run in the spool is collected once, redacted, and removed from the host.
	name := Spool + "/prune-snapshots.1794454200.42"
	f.host.Put(name+".out", []byte("token is s3cret-value\n"))
	f.host.Put(name+".done", []byte("exit=124\nstart=1794454200\nend=1794454260\nsize=22\n"))
	if sum, err := f.s.Collect(ctx); err != nil || sum != "1 scheduled run(s) collected" {
		t.Fatalf("collect %q %v", sum, err)
	}
	runs, _ := f.s.Runs(ctx, sc.ID)
	if len(runs) != 2 || runs[0].Trigger != TriggerScheduled || runs[0].Status != RunTimedOut || runs[0].Output != "token is [secret]\n" {
		t.Fatalf("runs %+v", runs)
	}
	if _, ok := f.host.File(name + ".done"); ok {
		t.Fatal("the spool entry stayed")
	}

	// A changed body makes the next run refuse until the change is applied.
	in.Body = "#!/bin/sh\necho v2\n"
	in.Env[1].Value = secrets.Value{} // keep the stored secret
	if _, err := f.s.Update(ctx, "admin", "", sc.ID, in); err != nil {
		t.Fatal(err)
	}
	runID, _ = f.s.Start(ctx, "admin", "", sc.ID, "h1")
	if r := waitRun(t, f.s, runID); r.Status != RunError || !strings.Contains(r.Error, "apply the pending change") {
		t.Fatalf("stale copy ran: %+v", r)
	}
	d = f.plan(t)
	if len(d.Items) != 1 || d.Items[0].Action != hsync.ActionUpdate {
		t.Fatalf("update plan %+v", d.Items)
	}
	var before itemState
	_ = json.Unmarshal(d.Items[0].Before, &before)
	if before.Body != "#!/bin/sh\necho \"$TOKEN\"\n" {
		t.Fatalf("the diff's before is not what the host holds: %q", before.Body)
	}
	f.apply(t)

	// Delete: the timer goes first, then every file, and the manifest forgets the script.
	if err := f.s.Delete(ctx, "admin", "", sc.ID); err != nil {
		t.Fatal(err)
	}
	f.apply(t)
	for _, p := range []string{Dir + "/prune-snapshots.sh", Dir + "/prune-snapshots.env", UnitDir + "/hashi-script-prune-snapshots.timer"} {
		if _, ok := f.host.File(p); ok {
			t.Errorf("%s left on the host", p)
		}
	}
	ran := strings.Join(f.host.Ran(), "\n")
	if strings.Index(ran, "systemctl disable --now hashi-script-prune-snapshots.timer") > strings.Index(ran, "rm -f '"+UnitDir) {
		t.Fatal("files removed before the timer was stopped")
	}
	man, _ := f.host.File(Manifest)
	if strings.Contains(string(man), "prune-snapshots") {
		t.Fatalf("manifest %s", man)
	}
}

// An explicit host list with no host means no host, never all (09 §1).
func TestEmptyTargetListMeansNone(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	sc, err := f.s.Create(ctx, "admin", "", Input{Name: "x", Body: "#!/bin/sh\n", Enabled: true, TimeoutSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := f.s.Targets(ctx, sc); len(got) != 0 {
		t.Fatalf("targets %v", got)
	}
	if d := f.plan(t); len(d.Items) != 1 || d.Items[0].Object != "runner "+Wrapper {
		t.Fatalf("plan %+v", d.Items)
	}
}

func TestSaveRefuses(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	ok := Input{Name: "ok", Body: "#!/bin/sh\n", TimeoutSeconds: 5}
	for name, in := range map[string]Input{
		"no shebang": {Name: "a", Body: "echo", TimeoutSeconds: 5},
		"bad cron":   {Name: "a", Body: ok.Body, TimeoutSeconds: 5, Schedule: "every day"},
		"timeout":    {Name: "a", Body: ok.Body},
		"env name":   {Name: "a", Body: ok.Body, TimeoutSeconds: 5, Env: []EnvVar{{Name: "1X"}}},
		"no secret":  {Name: "a", Body: ok.Body, TimeoutSeconds: 5, Env: []EnvVar{{Name: "X", Secret: true}}},
		"host":       {Name: "a", Body: ok.Body, TimeoutSeconds: 5, Hosts: []string{"nope"}},
		"name":       {Name: "!!", Body: ok.Body, TimeoutSeconds: 5},
	} {
		if _, err := f.s.Create(ctx, "admin", "", in); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := f.s.Create(ctx, "admin", "", ok); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Create(ctx, "admin", "", ok); !errors.Is(err, ErrInvalid) {
		t.Fatalf("same name twice: %v", err)
	}
}
