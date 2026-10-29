package firewall

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/netip"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/sshx"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
)

// fakeHost keeps files in memory and answers commands from a function.
type fakeHost struct {
	files map[string][]byte
	modes map[string]fs.FileMode
	ran   []string
	run   func(cmd string) sshx.Result
}

func newFake() *fakeHost {
	return &fakeHost{files: map[string][]byte{}, modes: map[string]fs.FileMode{}, run: func(string) sshx.Result { return sshx.Result{} }}
}

func (f *fakeHost) Run(_ context.Context, cmd string, _ io.Reader) (sshx.Result, error) {
	f.ran = append(f.ran, cmd)
	return f.run(cmd), nil
}

func (f *fakeHost) ReadFile(p string) ([]byte, error) {
	b, ok := f.files[p]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return b, nil
}

func (f *fakeHost) WriteFileAtomic(p string, data []byte, mode fs.FileMode) (bool, error) {
	f.files[p], f.modes[p] = data, mode
	return true, nil
}

func (f *fakeHost) MkdirAll(string, fs.FileMode) error { return nil }

func desired(t *testing.T, s Spec) Desired {
	t.Helper()
	b, err := Render(s)
	if err != nil {
		t.Fatal(err)
	}
	return Desired{Spec: s, Table: b, NftPath: "/usr/sbin/nft"}
}

func TestFirstApplyIsHighAndArmsTheTimer(t *testing.T) {
	h := newFake()
	h.run = func(cmd string) sshx.Result {
		switch {
		case cmd == "nft list tables":
			return sshx.Result{Stdout: []byte("table inet foreign\n")}
		case strings.HasPrefix(cmd, "systemctl is-enabled"):
			return sshx.Result{Stdout: []byte("disabled\n"), ExitCode: 1}
		case strings.Contains(cmd, "list table inet hashi"):
			if !strings.Contains(strings.Join(h.ran, "\n"), "-j -f "+TableFile) {
				return sshx.Result{ExitCode: 1, Stderr: []byte("No such file or directory")}
			}
			return sshx.Result{Stdout: h.files[TableFile]} // the rendered batch reads like a listing for Verify
		}
		return sshx.Result{}
	}
	d := desired(t, shapes()["netbird_routing.json"])
	diff, err := Observe(context.Background(), h, d)
	if err != nil {
		t.Fatal(err)
	}
	if !diff.First || diff.Risk != hsync.High || !diff.UnitDisabled || len(diff.Files) != 3 {
		t.Fatalf("%+v", diff)
	}
	for _, it := range diff.Items() {
		if it.Risk != hsync.High {
			t.Errorf("first apply item %s is %s", it.Object, it.Risk)
		}
	}
	rescued := false
	err = Apply(context.Background(), h, d, diff, func(context.Context) error { rescued = true; return nil })
	if err != nil {
		t.Fatal(err)
	}
	ran := strings.Join(h.ran, "\n")
	armAt := strings.Index(ran, "systemd-run --on-active=180 --timer-property=AccuracySec=1s --unit=hashi-fw-rescue "+RollbackFile)
	loadAt := strings.Index(ran, "/usr/sbin/nft -j -f "+TableFile)
	disarmAt := strings.Index(ran, "systemctl stop hashi-fw-rescue.timer\n")
	if armAt < 0 || loadAt < armAt || !rescued || !strings.HasSuffix(ran, "systemctl stop hashi-fw-rescue.timer") || disarmAt > loadAt {
		t.Fatalf("order arm → load → rescue → disarm broken:\n%s", ran)
	}
	if h.modes[RollbackFile] != 0o700 || !strings.Contains(string(h.files[RollbackFile]), "delete table inet hashi") ||
		strings.Contains(string(h.files[RollbackFile]), "flush ruleset") {
		t.Fatalf("rollback.sh %o:\n%s", h.modes[RollbackFile], h.files[RollbackFile])
	}
	if !strings.Contains(ran, "systemctl enable "+UnitName) || strings.Contains(ran, "nftables.service") {
		t.Fatalf("unit handling:\n%s", ran)
	}
}

func TestFailedRescueLeavesTheTimerArmed(t *testing.T) {
	h := newFake()
	h.run = func(cmd string) sshx.Result {
		return liveHost(h, cmd)
	}
	d := desired(t, shapes()["netbird_routing.json"])
	diff, _ := Observe(context.Background(), h, d)
	err := Apply(context.Background(), h, d, diff, func(context.Context) error { return errors.New("ssh: connection refused") })
	if !errors.Is(err, ErrRescuePending) {
		t.Fatalf("err %v", err)
	}
	if strings.HasSuffix(strings.Join(h.ran, "\n"), "systemctl stop hashi-fw-rescue.timer") {
		t.Fatal("the timer was disarmed although the host was not confirmed")
	}
}

func TestElementOnlyChangeIsLowWithoutTimer(t *testing.T) {
	h := newFake()
	s := shapes()["netbird_routing.json"]
	old := desired(t, s)
	h.files[TableFile], h.files[SysctlFile], h.files[UnitFile] = old.Table, Sysctl(false), Unit("/usr/sbin/nft")
	h.run = func(cmd string) sshx.Result {
		return liveHost(h, cmd)
	}
	s.Folded = Fold(pfx("198.51.100.7/32", "198.51.100.8/32"), pfx("203.0.113.0/24"))
	d := desired(t, s)
	diff, err := Observe(context.Background(), h, d)
	if err != nil || diff.Risk != hsync.Low || len(diff.Files) != 1 {
		t.Fatalf("%+v %v", diff, err)
	}
	if err := Apply(context.Background(), h, d, diff, nil); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(h.ran, "\n"), "systemd-run") {
		t.Fatal("a low-risk apply armed the timer")
	}
	// Nothing changed → no items.
	diff, _ = Observe(context.Background(), h, d)
	if len(diff.Items()) != 0 {
		t.Fatalf("idempotent plan has items: %+v", diff.Items())
	}
}

func TestTableRisk(t *testing.T) {
	base := shapes()["netbird_routing.json"]
	render := func(mut func(*Spec)) []byte {
		s := base
		s.Ports = append([]Port(nil), base.Ports...)
		mut(&s)
		b, err := Render(s)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	old := render(func(*Spec) {})
	g := Guard{AdminIPs: []netip.Addr{netip.MustParseAddr("198.51.100.7")}, RescueIP: netip.MustParseAddr("198.51.100.3")}
	cases := map[string]struct {
		mut  func(*Spec)
		want string
	}{
		"same":                  {func(*Spec) {}, ""},
		"other allow added":     {func(s *Spec) { s.Folded = Fold(pfx("198.51.100.7/32", "192.0.2.9/32"), pfx("203.0.113.0/24")) }, ""},
		"admin allow removed":   {func(s *Spec) { s.Folded = Fold(nil, pfx("203.0.113.0/24")) }, "allow4 no longer contains 198.51.100.7"},
		"rescue access removed": {func(s *Spec) { s.HostAccess = nil }, "host_access4 no longer contains 198.51.100.3"},
		"small block":           {func(s *Spec) { s.Folded = Fold(pfx("198.51.100.7/32"), pfx("203.0.113.0/24", "192.0.2.0/24")) }, ""},
		"block a /16":           {func(s *Spec) { s.Folded = Fold(pfx("198.51.100.7/32"), pfx("203.0.113.0/24", "10.99.0.0/16")) }, "block4 newly blocks 10.99.0.0/16"},
		"port opened":           {func(s *Spec) { s.Ports = append(s.Ports, Port{Proto: "tcp", Port: 25565, TargetPort: 25565}) }, "published ports change"},
		"meters on":             {func(s *Spec) { s.Meters.Enabled = true }, "chains or sets change"},
		"inbound drop on":       {func(s *Spec) { s.DropOtherInbound = true }, "chains or sets change"},
	}
	for name, c := range cases {
		got := strings.Join(TableRisk(old, render(c.mut), g), "; ")
		if !strings.Contains(got, c.want) || (c.want == "" && got != "") {
			t.Errorf("%s: %q, want %q", name, got, c.want)
		}
	}
}

func TestVerifyComparesCoverageNotText(t *testing.T) {
	d := desired(t, shapes()["netbird_routing.json"])
	// nft lists adjacent prefixes merged into a range: same coverage, different text.
	live := strings.Replace(string(d.Table), `{"prefix": {"addr": "203.0.113.0", "len": 28}}`, `{"range": ["203.0.113.0", "203.0.113.15"]}`, 1)
	h := newFake()
	h.run = func(string) sshx.Result { return sshx.Result{Stdout: []byte(live)} }
	if err := Verify(context.Background(), h, d); err != nil {
		t.Fatal(err)
	}
	h.run = func(string) sshx.Result {
		return sshx.Result{Stdout: []byte(strings.Replace(string(d.Table), `"198.51.100.7"`, `"198.51.100.9"`, 1))}
	}
	if err := Verify(context.Background(), h, d); err == nil {
		t.Fatal("a wrong allow element verified")
	}
}

// liveHost answers like a host where Hashi's table and unit are in place.
func liveHost(h *fakeHost, cmd string) sshx.Result {
	switch {
	case strings.Contains(cmd, "list table inet hashi"):
		if h.files[TableFile] == nil {
			return sshx.Result{ExitCode: 1, Stderr: []byte("Error: No such file or directory")}
		}
		return sshx.Result{Stdout: h.files[TableFile]}
	case cmd == "nft list tables":
		return sshx.Result{Stdout: []byte("table inet hashi\n")}
	case strings.HasPrefix(cmd, "systemctl is-enabled"):
		return sshx.Result{Stdout: []byte("enabled\n")}
	}
	return sshx.Result{}
}
