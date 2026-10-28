package traefik

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/sshx"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
)

// fakeHost is an in-memory host: files, directories, a command log, and a Traefik that answers ping and /api/rawdata
// with whatever the test sets.
type fakeHost struct {
	mu      sync.Mutex
	files   map[string][]byte
	modes   map[string]fs.FileMode
	dirs    map[string]bool
	ran     []string
	writes  []string
	version string // traefik version output; "" = no binary
	up      bool   // ping answers
	// startOK decides whether a start/restart command brings Traefik up (given the static file content).
	startOK func(static []byte) bool
	rawdata []byte
	probe   string // what ProbeCommand prints
	// noLogrotate makes `command -v logrotate` fail.
	noLogrotate bool
}

func newFakeHost() *fakeHost {
	return &fakeHost{files: map[string][]byte{}, modes: map[string]fs.FileMode{}, dirs: map[string]bool{},
		startOK: func([]byte) bool { return true }}
}

func (h *fakeHost) Run(_ context.Context, cmd string, _ io.Reader) (sshx.Result, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ran = append(h.ran, cmd)
	switch {
	case cmd == ProbeCommand:
		return sshx.Result{Stdout: []byte(h.probe)}, nil
	case cmd == "command -v logrotate" && h.noLogrotate:
		return sshx.Result{ExitCode: 1}, nil
	case strings.HasPrefix(cmd, "cp -p "):
		args := strings.Fields(strings.TrimPrefix(cmd, "cp -p "))
		from, to := unquote(args[0]), unquote(args[1])
		b, ok := h.files[from]
		if !ok {
			return sshx.Result{ExitCode: 1, Stderr: []byte("no such file")}, nil
		}
		h.files[to] = append([]byte(nil), b...)
		return sshx.Result{}, nil
	case strings.HasSuffix(cmd, " version"):
		if h.version == "" {
			return sshx.Result{ExitCode: 127, Stderr: []byte("not found")}, nil
		}
		return sshx.Result{Stdout: []byte("Version:      " + h.version + "\nCodename:     x\n")}, nil
	case strings.HasPrefix(cmd, "test -d "):
		if h.dirs[unquote(strings.TrimPrefix(cmd, "test -d "))] {
			return sshx.Result{}, nil
		}
		return sshx.Result{ExitCode: 1}, nil
	case cmd == "fake-start" || cmd == "fake-restart" || strings.Contains(cmd, "rc-service "+ServiceName) || strings.HasSuffix(cmd, " "+ServiceName) && strings.HasPrefix(cmd, "systemctl"):
		h.up = h.startOK(h.files[DefaultConfigDir+"/traefik.yaml"])
		return sshx.Result{}, nil
	}
	return sshx.Result{}, nil
}

func (h *fakeHost) ReadFile(p string) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	b, ok := h.files[p]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), b...), nil
}

func (h *fakeHost) WriteFileAtomic(p string, data []byte, mode fs.FileMode) (bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if old, ok := h.files[p]; ok && sshx.Hash(old) == sshx.Hash(data) {
		return false, nil
	}
	h.files[p], h.modes[p] = append([]byte(nil), data...), mode
	h.writes = append(h.writes, p)
	return true, nil
}

func (h *fakeHost) MkdirAll(p string, _ fs.FileMode) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dirs[p] = true
	return nil
}

func (h *fakeHost) Fetch(_ context.Context, _ int, path string) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.up {
		return nil, errors.New("connection refused")
	}
	if path == "/ping" {
		return []byte("OK"), nil
	}
	if h.rawdata != nil {
		return h.rawdata, nil
	}
	// Without a set answer the fake Traefik reports everything in its dynamic files as loaded.
	var files []File
	for p, b := range h.files {
		if strings.HasPrefix(p, DefaultConfigDir+"/dynamic/") {
			files = append(files, File{Path: p, Kind: KindDynamic, Data: b})
		}
	}
	names, err := Expected(files)
	if err != nil {
		return nil, err
	}
	rd := rawData{Routers: map[string]rawObject{}, Middlewares: map[string]rawObject{}, Services: map[string]rawObject{}}
	for _, n := range names {
		kind, name, _ := strings.Cut(n, " ")
		map[string]map[string]rawObject{"router": rd.Routers, "middleware": rd.Middlewares, "service": rd.Services}[kind][name] = rawObject{Status: "enabled"}
	}
	return json.Marshal(rd)
}

func fakeService() Service {
	return Service{Path: "/etc/init.d/" + ServiceName, Data: []byte("#!/sbin/openrc-run\n"), Mode: 0o755,
		Enable: "fake-start", Restart: "fake-restart"}
}

func desired(t *testing.T) Desired {
	t.Helper()
	files, err := Render(testOptions())
	if err != nil {
		t.Fatal(err)
	}
	return Desired{Paths: testOptions().Paths, APIPort: 8080, Files: files, Service: fakeService(),
		Binary: func(context.Context) ([]byte, error) { return []byte("ELF"), nil }, PingTimeout: 1500 * time.Millisecond}
}

func TestFirstInstallThenNothingToDo(t *testing.T) {
	ctx := context.Background()
	h := newFakeHost()
	d := desired(t)
	diff, err := Observe(ctx, h, d)
	if err != nil {
		t.Fatal(err)
	}
	items := diff.Items()
	if hsync.RiskOf(items) != hsync.High || items[0].Object != "traefik binary" || items[1].Object != "directories" {
		t.Fatalf("items: %+v", items)
	}
	for _, it := range items {
		if strings.Contains(string(it.After), "PRIVATE KEY") {
			t.Fatalf("item %s carries key material", it.Object)
		}
	}
	out, err := Apply(ctx, h, d, diff, time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !out.Restarted || string(h.files[DefaultBinary]) != "ELF" || h.modes[DefaultConfigDir+"/certs/example.com.key"] != 0o600 {
		t.Errorf("restarted=%v binary=%q", out.Restarted, h.files[DefaultBinary])
	}
	if !slices.Contains(h.ran, "fake-start") {
		t.Errorf("new service was not enabled: %v", h.ran)
	}
	// Certificates are written before the YAML that lists them, the static file last.
	certs, list, static := index(h.writes, ".crt"), index(h.writes, "hashi-certs.yaml"), index(h.writes, "traefik.yaml")
	if certs > list || list > static {
		t.Errorf("write order: %v", h.writes)
	}

	h.version = Version
	again, err := Observe(ctx, h, d)
	if err != nil {
		t.Fatal(err)
	}
	if !again.Empty() {
		t.Errorf("second plan not empty: %+v", again.Items())
	}
}

// Invariant 8: a host that already matches gets no writes, and a dynamic-only change writes only that file without a
// restart.
func TestInvariant_8_OnlyChangedFilesAreWritten(t *testing.T) {
	ctx := context.Background()
	h := newFakeHost()
	h.version, h.up = Version, true
	d := desired(t)
	for _, dir := range d.Paths.Directories() {
		h.dirs[dir.Path] = true
	}
	h.files[d.Service.Path] = d.Service.Data
	for _, f := range d.Files {
		h.files[f.Path] = f.Data
	}
	diff, _ := Observe(ctx, h, d)
	if out, err := Apply(ctx, h, d, diff, time.Now()); err != nil || len(h.writes) != 0 || out.Restarted {
		t.Fatalf("matching host: writes %v restarted %v err %v", h.writes, out.Restarted, err)
	}

	o := testOptions()
	o.HashiURL = "http://192.0.2.6:3340"
	d.Files, _ = Render(o)
	diff, _ = Observe(ctx, h, d)
	items := diff.Items()
	if len(items) != 1 || !strings.HasSuffix(items[0].Object, "40-hashi-security.yml") || items[0].Risk != hsync.Low {
		t.Fatalf("items: %+v", items)
	}
	out, err := Apply(ctx, h, d, diff, time.Now())
	if err != nil || out.Restarted || len(h.writes) != 1 {
		t.Errorf("dynamic change: writes %v restarted %v err %v", h.writes, out.Restarted, err)
	}
}

// 04 §6 step 3 / §9: a static change that keeps Traefik from coming back is undone.
func TestStaticRestartFailureRestores(t *testing.T) {
	ctx := context.Background()
	h := newFakeHost()
	h.version, h.up = Version, true
	d := desired(t)
	for _, dir := range d.Paths.Directories() {
		h.dirs[dir.Path] = true
	}
	h.files[d.Service.Path] = d.Service.Data
	for _, f := range d.Files {
		h.files[f.Path] = f.Data
	}
	good := h.files[d.Paths.Static()]
	h.startOK = func(static []byte) bool { return string(static) == string(good) }

	o := testOptions()
	o.APIPort = 9080 // a static change
	d.Files, _ = Render(o)
	diff, _ := Observe(ctx, h, d)
	if hsync.RiskOf(diff.Items()) != hsync.High {
		t.Fatalf("static change is not high risk: %+v", diff.Items())
	}
	out, err := Apply(ctx, h, d, diff, time.Now())
	if !errors.Is(err, ErrRestored) {
		t.Fatalf("err = %v, want ErrRestored", err)
	}
	if string(h.files[d.Paths.Static()]) != string(good) || !h.up {
		t.Error("previous static config not restored or Traefik not back up")
	}
	for _, f := range out.Written {
		if f.Kind == KindStatic {
			t.Error("restored static file reported as applied")
		}
	}
}

func TestBackupOfExistingConfig(t *testing.T) {
	ctx := context.Background()
	h := newFakeHost()
	d := desired(t)
	d.Backup = []string{"/etc/traefik"}
	diff, err := Observe(ctx, h, d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(ctx, h, d, diff, time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(h.ran, "mkdir -p '/var/lib/hashi/traefik/backup/20261028T120000Z' && cp -a '/etc/traefik' '/var/lib/hashi/traefik/backup/20261028T120000Z/'") {
		t.Errorf("no backup command: %v", h.ran)
	}
	d.Backup = []string{"/etc/traefik; rm -rf /"}
	if _, err := Observe(ctx, h, d); err == nil {
		t.Error("unsafe backup path accepted")
	}
}

func TestVerifyAgainstCapturedRawdata(t *testing.T) {
	ctx := context.Background()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "traefik", "api_rawdata.json"))
	if err != nil {
		t.Fatal(err)
	}
	h := newFakeHost()
	h.up, h.rawdata = true, b
	// The captured file defines app-open/app-closed/fa-local/broken; a render naming "broken" must fail with its error.
	files := []File{{Path: "/d/x.yml", Kind: KindDynamic, Data: []byte("http:\n  routers:\n    app-open: {rule: x, entryPoints: [a], service: s}\n")}}
	if err := Verify(ctx, h, 8080, files); err != nil {
		t.Errorf("enabled router: %v", err)
	}
	files[0].Data = []byte("http:\n  routers:\n    broken: {rule: x, entryPoints: [a], service: s}\n    gone: {rule: x, entryPoints: [a], service: s}\n")
	err = Verify(ctx, h, 8080, files)
	if !errors.Is(err, ErrVerify) || !strings.Contains(err.Error(), `does not exist`) || !strings.Contains(err.Error(), "router gone@file is missing") {
		t.Errorf("err = %v", err)
	}
}

func index(list []string, suffix string) int {
	for i, x := range list {
		if strings.HasSuffix(x, suffix) {
			return i
		}
	}
	return -1
}

// 04 §3: a host without logrotate gets it with its package manager (high risk: a package install), then Hashi's config.
func TestLogrotateInstalledWhenMissing(t *testing.T) {
	ctx := context.Background()
	h := newFakeHost()
	h.noLogrotate = true
	d := desired(t)
	d.PackageManager, d.Init = "dnf", InitSystemd
	diff, err := Observe(ctx, h, d)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(diff.Items(), func(i hsync.Item) bool { return i.Object == "package logrotate" && i.Risk == hsync.High }) {
		t.Fatalf("items: %+v", diff.Items())
	}
	if _, err := Apply(ctx, h, d, diff, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(h.ran, "dnf install -y logrotate && systemctl enable --now logrotate.timer") || h.files[LogrotatePath] == nil {
		t.Errorf("ran %v", h.ran)
	}
	d.PackageManager = "pacman"
	if _, err := Observe(ctx, h, d); !errors.Is(err, ErrUnsupported) {
		t.Errorf("unknown package manager: %v", err)
	}
}
