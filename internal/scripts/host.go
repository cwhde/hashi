package scripts

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Paths on a firewall host (09 §1). Everything under Dir and the units named UnitPrefix* belong to Hashi.
const (
	Dir        = "/opt/hashi/scripts"
	Spool      = "/var/lib/hashi/scripts/spool"
	UnitDir    = "/etc/systemd/system"
	UnitPrefix = "hashi-script-"
	Wrapper    = Dir + "/run.sh"
	Manifest   = Dir + "/manifest.json"
)

var (
	slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)
	envRe  = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// wrapperBody runs one script with its environment and timeout. Scheduled runs (systemd) write their output to the
// spool, which Hashi collects into the run history; --direct prints it for manual runs over SSH. It takes no input but
// its arguments, which it checks itself. Values in the env file are base64, so no value is ever read as shell.
const wrapperBody = `#!/bin/sh
# Written by Hashi (09 §1); changes here are replaced. Usage: run.sh <script> <timeout-seconds> [--direct]
set -u
dir=${HASHI_SCRIPT_DIR:-/opt/hashi/scripts}
spool=${HASHI_SCRIPT_SPOOL:-/var/lib/hashi/scripts/spool}
max=1048576
slug=${1:-}
limit=${2:-}
mode=${3:-}
case $slug in '' | *[!a-z0-9-]*) echo "run.sh: bad script name" >&2; exit 2 ;; esac
case $limit in '' | *[!0-9]*) echo "run.sh: bad timeout" >&2; exit 2 ;; esac
[ -x "$dir/$slug.sh" ] || { echo "run.sh: $slug is not installed" >&2; exit 2; }
if [ -f "$dir/$slug.env" ]; then
	while IFS= read -r line; do
		k=${line%%=*}
		v=${line#*=}
		case $k in '' | *[!A-Za-z0-9_]*) continue ;; esac
		val=$(printf '%s' "$v" | base64 -d) || exit 2
		export "$k=$val"
	done <"$dir/$slug.env"
fi
if [ "$mode" = --direct ]; then
	exec timeout -k 10 "$limit" "$dir/$slug.sh" </dev/null
fi
umask 077
mkdir -p "$spool"
start=$(date +%s)
base="$spool/$slug.$start.$$"
timeout -k 10 "$limit" "$dir/$slug.sh" >"$base.tmp" 2>&1 </dev/null
rc=$?
end=$(date +%s)
size=$(wc -c <"$base.tmp")
head -c "$max" "$base.tmp" >"$base.out"
rm -f "$base.tmp"
printf 'exit=%s\nstart=%s\nend=%s\nsize=%s\n' "$rc" "$start" "$end" "$size" >"$base.part"
mv "$base.part" "$base.done"
exit "$rc"
`

// TimedOut is the wrapper's exit code when it stopped a script (timeout(1), testdata/contracts/scripts finding 6).
const TimedOut = 124

// Desired is one script as a host should hold it.
type Desired struct {
	Slug, Body string
	Calendar   []string // OnCalendar lines; empty = manual only
	Timeout    int      // seconds
	Env        map[string]string
}

// File is one file Hashi writes on the host.
type File struct {
	Path string
	Mode fs.FileMode
	Data []byte
}

// Hash is the SHA-256 the manifest records.
func (f File) Hash() string { s := sha256.Sum256(f.Data); return hex.EncodeToString(s[:]) }

// ManifestDoc is /opt/hashi/scripts/manifest.json: what Hashi last wrote, by path. Only paths listed here are Hashi's.
type ManifestDoc struct {
	Version int               `json:"version"`
	Files   map[string]string `json:"files"`   // path → SHA-256
	Scripts []string          `json:"scripts"` // slugs, sorted
	Timers  []string          `json:"timers"`  // timer unit names that are enabled, sorted
}

// Render is every file a host should hold for these scripts, the manifest excluded.
func Render(ds []Desired) ([]File, ManifestDoc, error) {
	files := []File{{Path: Wrapper, Mode: 0o750, Data: []byte(wrapperBody)}}
	man := ManifestDoc{Version: 1, Files: map[string]string{}}
	for _, d := range ds {
		if !slugRe.MatchString(d.Slug) {
			return nil, man, fmt.Errorf("script name %q", d.Slug)
		}
		if d.Timeout < 1 {
			return nil, man, fmt.Errorf("%s: timeout must be at least a second", d.Slug)
		}
		files = append(files, File{Path: Dir + "/" + d.Slug + ".sh", Mode: 0o750, Data: []byte(d.Body)})
		env, err := envFile(d.Env)
		if err != nil {
			return nil, man, fmt.Errorf("%s: %w", d.Slug, err)
		}
		files = append(files, File{Path: Dir + "/" + d.Slug + ".env", Mode: 0o600, Data: env})
		man.Scripts = append(man.Scripts, d.Slug)
		if len(d.Calendar) == 0 {
			continue
		}
		unit := UnitPrefix + d.Slug
		files = append(files,
			File{Path: UnitDir + "/" + unit + ".service", Mode: 0o644, Data: marshalUnit([]section{
				{"Unit", [][2]string{{"Description", "Hashi script " + d.Slug}}},
				{"Service", [][2]string{{"Type", "oneshot"}, {"ExecStart", Wrapper + " " + d.Slug + " " + strconv.Itoa(d.Timeout)}}},
			})},
			File{Path: UnitDir + "/" + unit + ".timer", Mode: 0o644, Data: marshalUnit(timerSections(d))})
		man.Timers = append(man.Timers, unit+".timer")
	}
	for _, f := range files {
		man.Files[f.Path] = f.Hash()
	}
	slices.Sort(man.Scripts)
	slices.Sort(man.Timers)
	return files, man, nil
}

func timerSections(d Desired) []section {
	kv := make([][2]string, 0, len(d.Calendar)+1)
	for _, c := range d.Calendar {
		kv = append(kv, [2]string{"OnCalendar", c})
	}
	kv = append(kv, [2]string{"Persistent", "false"})
	return []section{
		{"Unit", [][2]string{{"Description", "Hashi script " + d.Slug + " schedule"}}},
		{"Timer", kv},
		{"Install", [][2]string{{"WantedBy", "timers.target"}}},
	}
}

// envFile is NAME=base64(value) per line, sorted; the wrapper decodes it without evaluating anything.
func envFile(env map[string]string) ([]byte, error) {
	names := make([]string, 0, len(env))
	for k := range env {
		if !envRe.MatchString(k) {
			return nil, fmt.Errorf("variable name %q", k)
		}
		names = append(names, k)
	}
	slices.Sort(names)
	var b strings.Builder
	for _, k := range names {
		b.WriteString(k + "=" + base64.StdEncoding.EncodeToString([]byte(env[k])) + "\n")
	}
	return []byte(b.String()), nil
}

type section struct {
	name string
	kv   [][2]string
}

// marshalUnit writes a systemd unit. Values come from checked script names, parsed schedules and numbers only; a value
// with a line break would end the key, so it is refused by construction (the callers never pass one).
func marshalUnit(ss []section) []byte {
	var b strings.Builder
	b.WriteString("# Written by Hashi (09 §1); changes here are replaced.\n")
	for i, s := range ss {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString("[" + s.name + "]\n")
		for _, kv := range s.kv {
			if strings.ContainsAny(kv[1], "\r\n") {
				panic("unit value with a line break")
			}
			b.WriteString(kv[0] + "=" + kv[1] + "\n")
		}
	}
	return []byte(b.String())
}

// MarshalManifest is the manifest as written.
func MarshalManifest(m ManifestDoc) []byte {
	b, _ := json.MarshalIndent(m, "", "  ")
	return append(b, '\n')
}
