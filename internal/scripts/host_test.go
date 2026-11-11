package scripts

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// install writes Render's files for ds into a temporary root and returns the wrapper's environment.
func install(t *testing.T, ds []Desired) (dir, spool string) {
	t.Helper()
	root := t.TempDir()
	dir, spool = filepath.Join(root, "scripts"), filepath.Join(root, "spool")
	files, _, err := Render(ds)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if !strings.HasPrefix(f.Path, Dir+"/") {
			continue // units
		}
		if err := os.WriteFile(filepath.Join(dir, strings.TrimPrefix(f.Path, Dir+"/")), f.Data, f.Mode); err != nil {
			t.Fatal(err)
		}
	}
	return dir, spool
}

func wrap(t *testing.T, dir, spool string, args ...string) (string, int) {
	t.Helper()
	if _, err := exec.LookPath("timeout"); err != nil {
		t.Skip("timeout(1) not installed")
	}
	cmd := exec.Command("/bin/sh", append([]string{filepath.Join(dir, "run.sh")}, args...)...)
	cmd.Env = append(os.Environ(), "HASHI_SCRIPT_DIR="+dir, "HASHI_SCRIPT_SPOOL="+spool)
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return string(out), ee.ExitCode()
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(out), 0
}

// The wrapper hands every value over unchanged and never evaluates it: quotes, $(...), backticks and spaces stay text.
func TestWrapperEnvIsData(t *testing.T) {
	tricky := `it's "$(touch /tmp/hashi-pwned)" ` + "`id`" + ` $HOME; rm -rf /`
	dir, spool := install(t, []Desired{{Slug: "env", Timeout: 5, Body: "#!/bin/sh\nprintf '%s|%s' \"$KEEP\" \"$TRICKY\"\n",
		Env: map[string]string{"KEEP": "14", "TRICKY": tricky}}})
	out, rc := wrap(t, dir, spool, "env", "5", "--direct")
	if rc != 0 || out != "14|"+tricky {
		t.Fatalf("rc %d out %q", rc, out)
	}
	if _, err := os.Stat("/tmp/hashi-pwned"); err == nil {
		t.Fatal("a value was executed")
	}
}

// A scheduled run leaves output and its outcome in the spool; a run past its timeout exits 124 (09 §1).
func TestWrapperSpoolAndTimeout(t *testing.T) {
	dir, spool := install(t, []Desired{
		{Slug: "ok", Timeout: 5, Body: "#!/bin/sh\necho out; echo err >&2; exit 3\n"},
		{Slug: "slow", Timeout: 1, Body: "#!/bin/sh\nsleep 20\n"},
	})
	if _, rc := wrap(t, dir, spool, "ok", "5"); rc != 3 {
		t.Fatalf("exit %d, want the script's 3", rc)
	}
	done, _ := filepath.Glob(filepath.Join(spool, "ok.*.done"))
	if len(done) != 1 {
		t.Fatalf("spool %v", done)
	}
	meta, _ := os.ReadFile(done[0])
	out, _ := os.ReadFile(strings.TrimSuffix(done[0], ".done") + ".out")
	if !strings.Contains(string(meta), "exit=3\n") || string(out) != "out\nerr\n" {
		t.Fatalf("meta %q out %q", meta, out)
	}
	if _, rc := wrap(t, dir, spool, "slow", "1", "--direct"); rc != TimedOut {
		t.Fatalf("timeout exit %d, want %d", rc, TimedOut)
	}
	for _, args := range [][]string{{"../x", "5"}, {"ok", "5s"}, {"missing", "5"}} {
		if _, rc := wrap(t, dir, spool, args...); rc != 2 {
			t.Errorf("%v: exit %d, want 2", args, rc)
		}
	}
}

func TestRenderUnitsAndManifest(t *testing.T) {
	s, _ := ParseCron("0 12 1 * 1")
	files, man, err := Render([]Desired{{Slug: "backup", Body: "#!/bin/sh\n", Timeout: 60, Calendar: s.OnCalendar()},
		{Slug: "adhoc", Body: "#!/bin/sh\n", Timeout: 60}})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]File{}
	for _, f := range files {
		got[f.Path] = f
	}
	timer := string(got[UnitDir+"/hashi-script-backup.timer"].Data)
	if !strings.Contains(timer, "OnCalendar=Mon *-*-* 12:00:00\nOnCalendar=*-*-01 12:00:00\n") {
		t.Fatalf("timer %s", timer)
	}
	svc := string(got[UnitDir+"/hashi-script-backup.service"].Data)
	if !strings.Contains(svc, "ExecStart="+Wrapper+" backup 60\n") {
		t.Fatalf("service %s", svc)
	}
	if _, ok := got[UnitDir+"/hashi-script-adhoc.timer"]; ok {
		t.Fatal("a manual-only script got a timer")
	}
	if got[Dir+"/backup.env"].Mode != 0o600 || got[Dir+"/backup.sh"].Mode != 0o750 {
		t.Fatal("modes")
	}
	if len(man.Files) != len(files) || len(man.Timers) != 1 || man.Scripts[0] != "adhoc" {
		t.Fatalf("manifest %+v", man)
	}
	for _, bad := range []Desired{{Slug: "../x", Timeout: 1}, {Slug: "ok", Timeout: 0}, {Slug: "ok", Timeout: 1, Env: map[string]string{"A-B": "x"}}} {
		if _, _, err := Render([]Desired{bad}); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}

// Review of S11: a variable named like one of the runner's own (dir, limit, mode, spool, slug) must not change what the
// runner runs, its timeout, or where scheduled output goes.
func TestWrapperVariablesCannotOverrideTheRunner(t *testing.T) {
	env := map[string]string{"dir": "/nonexistent", "limit": "30", "mode": "--direct", "spool": "/nonexistent", "slug": "other"}
	dir, spool := install(t, []Desired{{Slug: "slow", Timeout: 1, Body: "#!/bin/sh\necho \"dir=$dir\"\nsleep 5\necho finished\n", Env: env}})
	out, rc := wrap(t, dir, spool, "slow", "1")
	if rc != TimedOut || strings.Contains(out, "finished") {
		t.Fatalf("timeout not kept: rc %d out %q", rc, out)
	}
	done, _ := filepath.Glob(filepath.Join(spool, "slow.*.done"))
	if len(done) != 1 {
		t.Fatalf("scheduled output left the spool: %v", done)
	}
	b, _ := os.ReadFile(strings.TrimSuffix(done[0], ".done") + ".out")
	if !strings.Contains(string(b), "dir=/nonexistent") { // the script itself still sees its variable
		t.Fatalf("script output %q", b)
	}
}

// Review of S11: no script may take the runner's file name.
func TestRenderRefusesTheRunnerName(t *testing.T) {
	if _, _, err := Render([]Desired{{Slug: "run", Body: "#!/bin/sh\n", Timeout: 5}}); err == nil {
		t.Fatal("a script named run would replace run.sh")
	}
}
