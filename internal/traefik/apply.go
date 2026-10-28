package traefik

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"

	"git.juzo.io/juzo/hashi/internal/sshx"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
)

// Host is what Apply needs from an SSH connection (*sshx.Client implements it).
type Host interface {
	Run(ctx context.Context, cmd string, stdin io.Reader) (sshx.Result, error)
	ReadFile(p string) ([]byte, error)
	WriteFileAtomic(p string, data []byte, mode fs.FileMode) (bool, error)
	MkdirAll(p string, mode fs.FileMode) error
	Fetch(ctx context.Context, port int, urlPath string) ([]byte, error)
}

// Desired is everything Hashi wants on one host.
type Desired struct {
	Paths   Paths
	APIPort int
	Files   []File // from Render
	Service Service
	// Backup lists existing config directories to copy to StateDir/backup/<time>/ before Hashi takes over
	// (04 §2 step 5). The caller includes them only after the user confirmed ownership.
	Backup []string
	// Binary returns the verified Traefik binary (Download + Binary); called only when the host needs it.
	Binary func(ctx context.Context) ([]byte, error)
	// PingTimeout overrides the default wait after a restart (tests).
	PingTimeout time.Duration
	// Certs are the certificates in Files; the manager checks each by TLS handshake after Verify (03 §5.3).
	Certs []Cert
}

// Diff is what differs between Desired and the host, read fresh from the host (04 §6 step 1).
type Diff struct {
	InstalledVersion string // "" when no traefik at the binary path
	InstallBinary    bool
	MissingDirs      []Directory
	Backup           []string
	Service          FileChange
	Files            []FileChange
}

// FileChange is one file whose host content differs from the render. HostHash "" = absent on the host.
type FileChange struct {
	File     File
	HostHash string
}

func (c FileChange) changed() bool { return c.File.Path != "" && c.HostHash != c.File.Hash() }

var versionLine = regexp.MustCompile(`(?m)^Version:\s+v?(\S+)\s*$`)

// Observe reads the host and returns the diff. A failed read fails the plan: nothing is written when reading fails
// (invariant 3).
func Observe(ctx context.Context, h Host, d Desired) (Diff, error) {
	var diff Diff
	res, err := h.Run(ctx, sshx.Quote(d.Paths.Binary)+" version", nil)
	if err != nil {
		return diff, fmt.Errorf("check traefik binary: %w", err)
	}
	if m := versionLine.FindStringSubmatch(string(res.Stdout)); res.OK() && m != nil {
		diff.InstalledVersion = "v" + m[1]
	}
	diff.InstallBinary = diff.InstalledVersion != Version

	for _, dir := range d.Paths.Directories() {
		res, err := h.Run(ctx, "test -d "+sshx.Quote(dir.Path), nil)
		if err != nil {
			return diff, fmt.Errorf("check %s: %w", dir.Path, err)
		}
		if !res.OK() {
			diff.MissingDirs = append(diff.MissingDirs, dir)
		}
	}
	for _, b := range d.Backup {
		if !safePath.MatchString(b) {
			return diff, fmt.Errorf("backup path %q is not a plain absolute path", b)
		}
	}
	diff.Backup = d.Backup

	hostHash := func(p string) (string, error) {
		b, err := h.ReadFile(p)
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		if err != nil {
			return "", fmt.Errorf("read %s: %w", p, err)
		}
		return sshx.Hash(b), nil
	}
	svc := File{Path: d.Service.Path, Kind: "service", Mode: d.Service.Mode, Data: d.Service.Data}
	if diff.Service.HostHash, err = hostHash(svc.Path); err != nil {
		return diff, err
	}
	diff.Service.File = svc
	for _, f := range d.Files {
		hh, err := hostHash(f.Path)
		if err != nil {
			return diff, err
		}
		if hh != f.Hash() {
			diff.Files = append(diff.Files, FileChange{File: f, HostHash: hh})
		}
	}
	return diff, nil
}

// itemFile is a file item's before/after JSON: path and hash, never content (keys must not reach the database).
type itemFile struct {
	Path string `json:"path"`
	Kind string `json:"kind,omitempty"`
	Mode string `json:"mode,omitempty"`
	Hash string `json:"sha256"`
}

func raw(v any) json.RawMessage {
	b, _ := json.Marshal(v) // plain structs
	return b
}

// Items turns a diff into plan items. A first install, the static file, the service definition and the binary are high
// risk (they restart Traefik, 01 §5.2); dynamic and certificate files are low risk (the file provider reloads).
func (d Diff) Items() []hsync.Item {
	var items []hsync.Item
	if d.InstallBinary {
		it := hsync.Item{Action: hsync.ActionCreate, Object: "traefik binary", Risk: hsync.High,
			After: raw(map[string]string{"version": Version}), Note: "download, verify the release checksum, upload"}
		if d.InstalledVersion != "" {
			it.Action, it.Before = hsync.ActionUpdate, raw(map[string]string{"version": d.InstalledVersion})
		}
		items = append(items, it)
	}
	if len(d.MissingDirs) > 0 {
		var dirs []string
		for _, m := range d.MissingDirs {
			dirs = append(dirs, fmt.Sprintf("%s (%o)", m.Path, m.Mode))
		}
		items = append(items, hsync.Item{Action: hsync.ActionCreate, Object: "directories", Risk: hsync.Low,
			After: raw(map[string][]string{"paths": dirs})})
	}
	for _, b := range d.Backup {
		items = append(items, hsync.Item{Action: hsync.ActionCreate, Object: "backup " + b, Risk: hsync.High,
			After: raw(map[string]string{"to": StateDir + "/backup/<time>/"}), Note: "existing configuration, kept before Hashi takes over"})
	}
	file := func(c FileChange, risk hsync.Risk) hsync.Item {
		it := hsync.Item{Action: hsync.ActionCreate, Object: c.File.Path, Risk: risk,
			After: raw(itemFile{Path: c.File.Path, Kind: c.File.Kind, Mode: fmt.Sprintf("%o", c.File.Mode), Hash: c.File.Hash()})}
		if c.HostHash != "" {
			it.Action, it.Before = hsync.ActionUpdate, raw(itemFile{Path: c.File.Path, Hash: c.HostHash})
		}
		return it
	}
	if d.Service.changed() {
		items = append(items, file(d.Service, hsync.High))
	}
	for _, c := range d.Files {
		risk := hsync.Low
		if c.File.Kind == KindStatic {
			risk = hsync.High
		}
		items = append(items, file(c, risk))
	}
	return items
}

// Empty reports whether the host already matches.
func (d Diff) Empty() bool { return len(d.Items()) == 0 }

// PingTimeout is how long Apply waits for Traefik's ping after a restart (04 §6 step 3), unless Desired says otherwise.
const PingTimeout = 30 * time.Second

// ErrRestored means a restart failed and what was there before (binary, static config, service) was put back
// (04 §6 step 3, 04 §2.1).
var ErrRestored = errors.New("traefik did not come back after the change; the previous version was restored")

// Applied lists the files written, for last_applied_hash (04 §6 step 5).
type Applied struct {
	Written   []File
	Restarted bool
}

// Apply executes diff in a fixed order: binary, directories, backups, certificate and dynamic files, then service and
// static file with a restart. Unchanged files are never touched (invariant 8). A restart that does not bring ping
// back restores the previous binary (kept as traefik.previous), static file and service definition, restarts again
// and fails.
func Apply(ctx context.Context, h Host, d Desired, diff Diff, now time.Time) (Applied, error) {
	var out Applied
	run := func(what, cmd string) error {
		res, err := h.Run(ctx, cmd, nil)
		if err != nil {
			return fmt.Errorf("%s: %w", what, err)
		}
		if !res.OK() {
			return fmt.Errorf("%s: exit %d: %s", what, res.ExitCode, strings.TrimSpace(string(res.Stderr)+" "+string(res.Stdout)))
		}
		return nil
	}
	write := func(f File) error {
		if _, err := h.WriteFileAtomic(f.Path, f.Data, fs.FileMode(f.Mode)); err != nil {
			return fmt.Errorf("write %s: %w", f.Path, err)
		}
		out.Written = append(out.Written, f)
		return nil
	}

	restart, prevBinary := false, false
	if diff.InstallBinary {
		if d.Binary == nil {
			return out, errors.New("traefik binary needed but no download configured")
		}
		bin, err := d.Binary(ctx)
		if err != nil {
			return out, err
		}
		if diff.InstalledVersion != "" {
			// 04 §2.1: the running binary stays as traefik.previous until the new one answers.
			if err := run("keep the previous traefik binary", "cp -p "+sshx.Quote(d.Paths.Binary)+" "+sshx.Quote(d.Paths.Binary+".previous")); err != nil {
				return out, err
			}
			prevBinary = true
		}
		if _, err := h.WriteFileAtomic(d.Paths.Binary, bin, 0o755); err != nil {
			return out, fmt.Errorf("upload traefik binary: %w", err)
		}
		restart = true
	}
	for _, dir := range diff.MissingDirs {
		if err := h.MkdirAll(dir.Path, fs.FileMode(dir.Mode)); err != nil {
			return out, fmt.Errorf("create %s: %w", dir.Path, err)
		}
		if err := run("chmod "+dir.Path, fmt.Sprintf("chmod %o %s", dir.Mode, sshx.Quote(dir.Path))); err != nil {
			return out, err
		}
	}
	if len(diff.Backup) > 0 {
		to := StateDir + "/backup/" + now.UTC().Format("20060102T150405Z")
		cmd := "mkdir -p " + sshx.Quote(to)
		for _, b := range diff.Backup {
			cmd += " && cp -a " + sshx.Quote(b) + " " + sshx.Quote(to+"/")
		}
		if err := run("back up existing configuration", cmd); err != nil {
			return out, err
		}
	}

	// Certificates first, then hashi-certs.yaml (Render's order), then the other dynamic files.
	var static *FileChange
	for i, c := range diff.Files {
		if c.File.Kind == KindStatic {
			static = &diff.Files[i]
			continue
		}
		if err := write(c.File); err != nil {
			return out, err
		}
	}

	newService := diff.Service.changed() && diff.Service.HostHash == ""
	if !diff.Service.changed() && static == nil && !restart {
		return out, nil
	}
	// Keep what is there now so a failed restart can put it back.
	var prevStatic, prevService []byte
	if static != nil && static.HostHash != "" {
		b, err := h.ReadFile(static.File.Path)
		if err != nil {
			return out, fmt.Errorf("keep previous static config: %w", err)
		}
		prevStatic = b
	}
	if diff.Service.changed() && !newService {
		b, err := h.ReadFile(diff.Service.File.Path)
		if err != nil {
			return out, fmt.Errorf("keep previous service definition: %w", err)
		}
		prevService = b
	}
	if static != nil {
		if err := write(static.File); err != nil {
			return out, err
		}
	}
	if diff.Service.changed() {
		if err := write(diff.Service.File); err != nil {
			return out, err
		}
	}
	start := d.Service.Restart
	if newService {
		start = d.Service.Enable
	}
	out.Restarted = true
	err := run("start traefik", start)
	if err == nil {
		err = waitPing(ctx, h, d.APIPort, d.PingTimeout)
	}
	if err == nil {
		return out, nil
	}
	if prevStatic == nil && prevService == nil && !prevBinary {
		return out, fmt.Errorf("traefik did not start: %w", err)
	}
	if prevBinary {
		if rerr := run("restore the previous traefik binary", "cp -p "+sshx.Quote(d.Paths.Binary+".previous")+" "+sshx.Quote(d.Paths.Binary)); rerr != nil {
			return out, fmt.Errorf("%w; restoring the previous binary failed too: %w", err, rerr)
		}
	}
	if prevStatic != nil {
		if _, werr := h.WriteFileAtomic(static.File.Path, prevStatic, fs.FileMode(static.File.Mode)); werr != nil {
			return out, fmt.Errorf("%w; restoring the static config failed too: %w", err, werr)
		}
	}
	if prevService != nil {
		if _, werr := h.WriteFileAtomic(diff.Service.File.Path, prevService, fs.FileMode(diff.Service.File.Mode)); werr != nil {
			return out, fmt.Errorf("%w; restoring the service definition failed too: %w", err, werr)
		}
	}
	out.Written = slices.DeleteFunc(out.Written, func(f File) bool {
		return f.Kind == KindStatic || f.Path == diff.Service.File.Path
	})
	port := d.APIPort
	if prevStatic != nil {
		if p, ok := apiPortOf(prevStatic); ok {
			port = p // the restored file may use another API port
		}
	}
	rerr := run("restart traefik after restore", d.Service.Restart)
	if rerr == nil {
		rerr = waitPing(ctx, h, port, d.PingTimeout)
	}
	if rerr != nil {
		return out, fmt.Errorf("traefik did not come back after the change (%w), and not after restoring the previous configuration either: %w", err, rerr)
	}
	return out, fmt.Errorf("%w: %w", ErrRestored, err)
}

// apiPortOf reads the API/ping port from a static file Hashi rendered.
func apiPortOf(static []byte) (int, bool) {
	var c staticConfig
	if yaml.Unmarshal(static, &c) != nil {
		return 0, false
	}
	_, p, ok := strings.Cut(c.EntryPoints["traefik"].Address, ":")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(p)
	return n, err == nil
}

func waitPing(ctx context.Context, h Host, port int, timeout time.Duration) error {
	if timeout == 0 {
		timeout = PingTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var last error
	for {
		b, err := h.Fetch(ctx, port, "/ping")
		if err == nil && strings.TrimSpace(string(b)) == "OK" {
			return nil
		}
		if errors.Is(err, sshx.ErrNoFetch) {
			return err
		}
		last = err
		select {
		case <-ctx.Done():
			return fmt.Errorf("no ping answer within %s: %w", timeout, last)
		case <-time.After(time.Second):
		}
	}
}
