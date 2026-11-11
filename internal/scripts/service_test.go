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
	"git.juzo.io/juzo/hashi/internal/firewall/firewalltest"
	. "git.juzo.io/juzo/hashi/internal/scripts"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/store"
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
	st   *store.Store
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
	return fixture{st: st, s: s, host: h, sub: s.Subsystems()[0]}
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
	if len(runs) != 2 || runs[0].Trigger != TriggerScheduled || runs[0].Status != RunTimedOut {
		t.Fatalf("runs %+v", runs)
	}
	if r, _ := f.s.GetRun(ctx, runs[0].ID); r.Output != "token is [secret]\n" {
		t.Fatalf("scheduled output %q", r.Output)
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

// The sync engine stores and applies a scripts plan: the plans table accepts the subsystem and a saved script waits as a
// high-risk plan until approved.
func TestEngineRunsScriptsPlans(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	al := audit.New([]byte("k"))
	eng := hsync.New(f.st, al, f.sub)
	if err := eng.Start(ctx); err != nil {
		t.Fatal(err)
	}
	f.s.OnSave(func(ctx context.Context, ids []string, actor string) {
		for _, h := range ids {
			if _, err := eng.Propose(ctx, Subsystem, h, hsync.TriggerSave, actor); err != nil {
				t.Errorf("propose: %v", err)
			}
		}
	})
	if _, err := f.s.Create(ctx, "admin", "", Input{Name: "x", Body: "#!/bin/sh\n", Enabled: true, TimeoutSeconds: 5, AllHosts: true}); err != nil {
		t.Fatal(err)
	}
	open, err := eng.Open(ctx)
	if err != nil || len(open) != 1 || open[0].Status != hsync.StatusPending || open[0].Risk != hsync.High {
		t.Fatalf("open %+v %v", open, err)
	}
	if err := eng.Approve(ctx, open[0].ID, "admin", ""); err != nil {
		t.Fatal(err)
	}
	p, err := eng.Apply(ctx, open[0].ID, "admin", "")
	if err != nil || p.Status != hsync.StatusApplied {
		t.Fatalf("apply %+v %v", p, err)
	}
	if _, ok := f.host.File(Dir + "/x.sh"); !ok {
		t.Fatal("not on the host")
	}
}

// Review of S11: every run is audited (09 §1, invariant 17), scheduled ones too; and the runner's name is refused.
func TestScheduledRunsAreAuditedAndRunIsReserved(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	if _, err := f.s.Create(ctx, "admin", "", Input{Name: "Run", Body: "#!/bin/sh\n", Enabled: true, TimeoutSeconds: 5, AllHosts: true}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("a script named Run: %v", err)
	}
	sc, err := f.s.Create(ctx, "admin", "", Input{Name: "tick", Body: "#!/bin/sh\n", Enabled: true, Schedule: "* * * * *", TimeoutSeconds: 5, AllHosts: true})
	if err != nil {
		t.Fatal(err)
	}
	f.apply(t)
	name := Spool + "/tick.1794454200.7"
	f.host.Put(name+".out", []byte("x\n"))
	f.host.Put(name+".done", []byte("exit=1\nstart=1794454200\nend=1794454201\nsize=2\n"))
	if _, err := f.s.Collect(ctx); err != nil {
		t.Fatal(err)
	}
	evs, err := f.st.R.ListAuditEvents(ctx, db.ListAuditEventsParams{Target: "script:" + sc.ID, Lim: 20})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range evs {
		found = found || (e.Action == "script.run" && strings.Contains(e.DetailJson, `"trigger":"scheduled"`))
	}
	if !found {
		t.Fatalf("no audit entry for the scheduled run: %+v", evs)
	}
}

// Review of S11, invariant 1: a unit file Hashi did not write is never taken over because of its name.
func TestForeignUnitIsAConflict(t *testing.T) {
	ctx := context.Background()
	f := newFixture(t)
	f.host.Put(UnitDir+"/hashi-script-backup.timer", []byte("[Timer]\nOnCalendar=daily\n"))
	if _, err := f.s.Create(ctx, "admin", "", Input{Name: "backup", Body: "#!/bin/sh\n", Enabled: true, Schedule: "0 3 * * *",
		TimeoutSeconds: 5, AllHosts: true}); err != nil {
		t.Fatal(err)
	}
	d := f.plan(t)
	var conflict bool
	for _, it := range d.Items {
		conflict = conflict || (it.Object == "script backup" && it.Action == hsync.ActionConflict)
	}
	if !conflict {
		t.Fatalf("plan %+v", d.Items)
	}
	if _, err := f.sub.Apply(ctx, "h1", d.Items); err != nil {
		t.Fatal(err)
	}
	if b, _ := f.host.File(UnitDir + "/hashi-script-backup.timer"); string(b) != "[Timer]\nOnCalendar=daily\n" {
		t.Fatalf("foreign timer overwritten: %q", b)
	}
}
