package scripts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"git.juzo.io/juzo/hashi/internal/sshx"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
)

// Subsystem is the sync engine's name for scripts; a plan's target is a firewall host id.
const Subsystem = "scripts"

// Subsystems returns the sync subsystem to register with the engine.
func (s *Service) Subsystems() []hsync.Subsystem { return []hsync.Subsystem{sub{s}} }

type sub struct{ s *Service }

func (sub) Name() string { return Subsystem }

// LockKey shares the firewall's key: one SSH writer per host at a time.
func (sub) LockKey(target string) string { return "firewall:" + target }

// itemState is one script in a plan item: what the admin needs to judge the change. Variable values stay out (secrets).
type itemState struct {
	Body     string   `json:"body,omitempty"`
	Schedule []string `json:"schedule,omitempty"` // OnCalendar lines
	Timeout  int      `json:"timeout_seconds,omitempty"`
	Env      []string `json:"env,omitempty"` // variable names
	Hash     string   `json:"hash"`          // over all of the script's files
}

// observed is what the host holds now: its manifest and the actual content of each Hashi file.
type observed struct {
	man   ManifestDoc
	files map[string][]byte // path → content; missing files absent
}

func readObserved(h Host, paths []string) (observed, error) {
	o := observed{files: map[string][]byte{}}
	b, err := h.ReadFile(Manifest)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		o.man = ManifestDoc{Version: 1, Files: map[string]string{}}
	case err != nil:
		return o, err
	default:
		if err := json.Unmarshal(b, &o.man); err != nil {
			return o, fmt.Errorf("%s is not Hashi's manifest: %w", Manifest, err)
		}
		if o.man.Files == nil {
			o.man.Files = map[string]string{}
		}
	}
	for p := range o.man.Files {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	for _, p := range slices.Compact(paths) {
		b, err := h.ReadFile(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return o, err
		}
		o.files[p] = b
	}
	return o, nil
}

// filesOf groups rendered files by script slug; the wrapper is "".
func filesOf(files []File) map[string][]File {
	out := map[string][]File{}
	for _, f := range files {
		out[slugOfPath(f.Path)] = append(out[slugOfPath(f.Path)], f)
	}
	return out
}

// slugOfPath maps a Hashi path to its script; the wrapper and unknown paths are "".
func slugOfPath(p string) string {
	switch {
	case p == Wrapper:
		return ""
	case strings.HasPrefix(p, UnitDir+"/"+UnitPrefix):
		n := strings.TrimPrefix(p, UnitDir+"/"+UnitPrefix)
		return strings.TrimSuffix(strings.TrimSuffix(n, ".service"), ".timer")
	case strings.HasPrefix(p, Dir+"/"):
		n := strings.TrimPrefix(p, Dir+"/")
		return strings.TrimSuffix(strings.TrimSuffix(n, ".sh"), ".env")
	}
	return ""
}

func groupHash(fs []File) string {
	parts := make([]string, 0, len(fs))
	for _, f := range fs {
		parts = append(parts, f.Path+"="+f.Hash())
	}
	slices.Sort(parts)
	return sshx.Hash([]byte(strings.Join(parts, "\n")))
}

func raw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// Plan compares what the host should hold with what it holds: one item per script whose files differ, one for the
// runner. Every item is high risk: a script runs as root (09 §1, "shows target hosts and diff before applying").
func (x sub) Plan(ctx context.Context, hostID string) (hsync.Draft, error) {
	ds, _, err := x.s.desired(ctx, hostID)
	if err != nil {
		return hsync.Draft{}, err
	}
	files, _, err := Render(ds)
	if err != nil {
		return hsync.Draft{}, err
	}
	h, err := x.s.dial(ctx, hostID)
	if err != nil {
		return hsync.Draft{}, err
	}
	defer func() { _ = h.Close() }()
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	o, err := readObserved(h, paths)
	if err != nil {
		return hsync.Draft{}, err
	}
	return planItems(ds, files, o), nil
}

func planItems(ds []Desired, files []File, o observed) hsync.Draft {
	want := filesOf(files)
	byslug := map[string]Desired{}
	for _, d := range ds {
		byslug[d.Slug] = d
	}
	have := map[string][]File{}
	for p, b := range o.files {
		have[slugOfPath(p)] = append(have[slugOfPath(p)], File{Path: p, Data: b})
	}
	var items []hsync.Item
	differs := func(slug string) bool {
		w := want[slug]
		if len(w) != len(have[slug]) {
			return true
		}
		for _, f := range w {
			if b, ok := o.files[f.Path]; !ok || sshx.Hash(b) != f.Hash() {
				return true
			}
		}
		return false
	}
	if differs("") {
		items = append(items, hsync.Item{Action: actionFor(o.files[Wrapper] != nil), Object: "runner " + Wrapper,
			After: raw(itemState{Hash: groupHash(want[""])}), Risk: hsync.High})
	}
	slugs := slices.Sorted(func(yield func(string) bool) {
		for k := range want {
			if k != "" && !yield(k) {
				return
			}
		}
	})
	for _, slug := range slugs {
		// A file at one of the script's paths that the manifest does not list is not Hashi's (invariant 1, owned
		// objects only): never overwritten, shown as a conflict.
		if foreign := slices.IndexFunc(want[slug], func(f File) bool {
			_, there := o.files[f.Path]
			_, ours := o.man.Files[f.Path]
			return there && !ours
		}); foreign >= 0 {
			items = append(items, hsync.Item{Action: hsync.ActionConflict, Object: "script " + slug, Risk: hsync.High,
				Note: want[slug][foreign].Path + " exists on the host and was not written by Hashi; rename the script or remove that file"})
			continue
		}
		if !differs(slug) {
			continue
		}
		d := byslug[slug]
		after := itemState{Body: d.Body, Schedule: d.Calendar, Timeout: d.Timeout, Env: slices.Sorted(mapKeys(d.Env)), Hash: groupHash(want[slug])}
		it := hsync.Item{Action: hsync.ActionCreate, Object: "script " + slug, After: raw(after), Risk: hsync.High}
		if len(have[slug]) > 0 {
			it.Action, it.Before = hsync.ActionUpdate, raw(itemState{Body: string(o.files[Dir+"/"+slug+".sh"]), Hash: groupHash(have[slug])})
		}
		items = append(items, it)
	}
	// Scripts the manifest lists but the host should no longer hold. Only manifest-listed files are Hashi's.
	for _, slug := range o.man.Scripts {
		if _, ok := want[slug]; ok || !slugRe.MatchString(slug) {
			continue
		}
		items = append(items, hsync.Item{Action: hsync.ActionDelete, Object: "script " + slug,
			Before: raw(itemState{Body: string(o.files[Dir+"/"+slug+".sh"]), Hash: groupHash(have[slug])}), Risk: hsync.High})
	}
	return hsync.Draft{Title: fmt.Sprintf("Scripts: %d change(s)", len(items)), Items: items}
}

func actionFor(exists bool) string {
	if exists {
		return hsync.ActionUpdate
	}
	return hsync.ActionCreate
}

func mapKeys(m map[string]string) func(func(string) bool) {
	return func(yield func(string) bool) {
		for k := range m {
			if !yield(k) {
				return
			}
		}
	}
}

// Apply writes exactly the planned scripts, removes the deleted ones, reloads systemd, switches timers, writes the
// manifest last and verifies by reading every file back.
func (x sub) Apply(ctx context.Context, hostID string, items []hsync.Item) (string, error) {
	ds, _, err := x.s.desired(ctx, hostID)
	if err != nil {
		return "", err
	}
	files, _, err := Render(ds)
	if err != nil {
		return "", err
	}
	h, err := x.s.dial(ctx, hostID)
	if err != nil {
		return "", err
	}
	defer func() { _ = h.Close() }()
	o, err := readObserved(h, nil)
	if err != nil {
		return "", err
	}
	want := filesOf(files)
	man := o.man
	man.Version = 1
	wroteUnits := false
	var enable, disable []string
	if err := h.MkdirAll(Dir, 0o750); err != nil {
		return "", err
	}
	if _, err := h.Run(ctx, "chmod 0750 "+Dir, nil); err != nil {
		return "", err
	}
	for _, it := range items {
		if it.Action == hsync.ActionConflict { // shown, never applied (03 §3)
			continue
		}
		slug, ok := itemSlug(it.Object)
		if !ok {
			return "", fmt.Errorf("unknown item %q", it.Object)
		}
		timer := UnitPrefix + slug + ".timer"
		// Files of this slug the manifest knows but the new state does not have (a schedule removed, or a delete).
		var stale []string
		for p := range man.Files {
			if slugOfPath(p) == slug && !slices.ContainsFunc(want[slug], func(f File) bool { return f.Path == p }) {
				stale = append(stale, p)
			}
		}
		slices.Sort(stale)
		for _, p := range stale {
			if strings.HasSuffix(p, ".timer") {
				disable = append(disable, timer)
			}
		}
		for _, f := range want[slug] {
			if _, err := h.WriteFileAtomic(f.Path, f.Data, f.Mode); err != nil {
				return "", fmt.Errorf("write %s: %w", f.Path, err)
			}
			man.Files[f.Path] = f.Hash()
			if strings.HasPrefix(f.Path, UnitDir) {
				wroteUnits = true
			}
			if strings.HasSuffix(f.Path, ".timer") {
				enable = append(enable, timer)
			}
		}
		if slug == "" {
			continue
		}
		man.Scripts = slices.DeleteFunc(man.Scripts, func(s string) bool { return s == slug })
		man.Timers = slices.DeleteFunc(man.Timers, func(s string) bool { return s == timer })
		if len(want[slug]) > 0 {
			man.Scripts = append(man.Scripts, slug)
		}
		if slices.Contains(enable, timer) {
			man.Timers = append(man.Timers, timer)
		}
		if err := x.removeAfterDisable(ctx, h, disable, stale, man); err != nil {
			return "", err
		}
		disable = nil
	}
	if wroteUnits {
		if r, err := h.Run(ctx, "systemctl daemon-reload", nil); err != nil || !r.OK() {
			return "", cmdErr("systemctl daemon-reload", r, err)
		}
	}
	for _, t := range enable { // t is UnitPrefix + a checked slug; restart picks up a changed schedule
		for _, cmd := range []string{"systemctl enable " + t, "systemctl restart " + t} {
			if r, err := h.Run(ctx, cmd, nil); err != nil || !r.OK() {
				return "", cmdErr(cmd, r, err)
			}
		}
	}
	slices.Sort(man.Scripts)
	slices.Sort(man.Timers)
	if _, err := h.WriteFileAtomic(Manifest, MarshalManifest(man), 0o640); err != nil {
		return "", err
	}
	if err := verify(ctx, h, man); err != nil {
		return "", err
	}
	return fmt.Sprintf("%d script change(s) applied", len(items)), nil
}

// removeAfterDisable stops and disables the timers first, then removes the files, so a schedule never fires a removed
// script, and drops them from the manifest.
func (sub) removeAfterDisable(ctx context.Context, h Host, timers, paths []string, man ManifestDoc) error {
	for _, t := range timers {
		cmd := "systemctl disable --now " + t
		if r, err := h.Run(ctx, cmd, nil); err != nil || !r.OK() {
			return cmdErr(cmd, r, err)
		}
	}
	for _, p := range paths {
		if r, err := h.Run(ctx, "rm -f "+sshx.Quote(p), nil); err != nil || !r.OK() {
			return cmdErr("rm "+p, r, err)
		}
		delete(man.Files, p)
	}
	if len(timers) > 0 {
		if r, err := h.Run(ctx, "systemctl daemon-reload", nil); err != nil || !r.OK() {
			return cmdErr("systemctl daemon-reload", r, err)
		}
	}
	return nil
}

func itemSlug(object string) (string, bool) {
	if strings.HasPrefix(object, "runner ") {
		return "", true
	}
	s, ok := strings.CutPrefix(object, "script ")
	return s, ok && slugRe.MatchString(s)
}

func cmdErr(cmd string, r sshx.Result, err error) error {
	if err != nil {
		return fmt.Errorf("%s: %w", cmd, err)
	}
	return fmt.Errorf("%s: exit %d: %s", cmd, r.ExitCode, strings.TrimSpace(string(r.Stderr)))
}

// verify reads every manifest file back and checks each listed timer is enabled.
func verify(ctx context.Context, h Host, man ManifestDoc) error {
	for p, want := range man.Files {
		b, err := h.ReadFile(p)
		if err != nil {
			return fmt.Errorf("verify %s: %w", p, err)
		}
		if sshx.Hash(b) != want {
			return fmt.Errorf("verify %s: content differs after writing", p)
		}
	}
	for _, t := range man.Timers {
		for cmd, want := range map[string]string{"systemctl is-enabled " + t: "enabled", "systemctl is-active " + t: "active"} {
			r, err := h.Run(ctx, cmd, nil)
			if err != nil || strings.TrimSpace(string(r.Stdout)) != want {
				return fmt.Errorf("verify: %w", cmdErr(cmd, r, err))
			}
		}
	}
	return nil
}
