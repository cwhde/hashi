package firewall

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/netip"
	"slices"
	"strings"
	"time"

	"go4.org/netipx"

	"git.juzo.io/juzo/hashi/internal/sshx"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
)

// Files on the host (05 §3.1).
const (
	Dir           = "/opt/hashi/firewall"
	TableFile     = Dir + "/hashi.nft.json"
	PreviousFile  = Dir + "/previous.nft.json"       // the live table before a risky apply (nft listing, no handles)
	PrevTableFile = Dir + "/previous.hashi.nft.json" // the rendered file before it, restored by the rollback
	RollbackFile  = Dir + "/rollback.sh"
	SysctlFile    = "/etc/sysctl.d/90-hashi.conf"
	UnitFile      = "/etc/systemd/system/hashi-firewall.service"
	UnitName      = "hashi-firewall.service"
	RescueUnit    = "hashi-fw-rescue"
)

// RescueWindow is how long a risky apply has to prove the host is still reachable (05 §7).
var RescueWindow = 180 * time.Second

// Host is an SSH connection to a firewall host (sshx.Client in production).
type Host interface {
	Run(ctx context.Context, cmd string, stdin io.Reader) (sshx.Result, error)
	ReadFile(p string) ([]byte, error)
	WriteFileAtomic(p string, data []byte, mode fs.FileMode) (bool, error)
	MkdirAll(p string, mode fs.FileMode) error
}

// Guard names the addresses a change must never lock out (05 §4.4).
type Guard struct {
	AdminIPs []netip.Addr // addresses of the active admin sessions
	RescueIP netip.Addr   // the address the host sees Hashi's SSH from
	// Blocks are the block side's prefixes as bound, before folding carves allowed addresses out of them: a /16
	// block stays a /16 for the risk rule even when an allowed address sits inside it.
	Blocks []netip.Prefix
}

// Desired is everything Hashi wants on one managed host.
type Desired struct {
	Spec    Spec
	Table   []byte // Render(Spec)
	NftPath string // from the probe's tools; the boot unit runs it
	// InstallNft is the package command when nft is missing ("" when present).
	InstallNft string
	IPv6       bool
	Guard      Guard
	// Takeover names the old firewall script's objects (05 §3.4); removed under the rescue timer while they exist.
	Takeover Takeover
}

// FileChange is one file that differs on the host.
type FileChange struct {
	Path     string
	Mode     fs.FileMode
	Data     []byte
	HostData []byte // nil when absent
}

// Diff is what Observe found.
type Diff struct {
	InstallNft   string
	Files        []FileChange
	TableMissing bool // the file matches but the kernel has no table inet hashi (reboot without the unit, flushed)
	UnitDisabled bool
	First        bool // Hashi never applied here: no table file on the host
	Risk         hsync.Risk
	Reasons      []string
	Takeover     *TakeoverPlan // nil or empty: nothing of the old script is left
}

func sha(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// Unit renders the boot unit: load the table once the network is up; never flush, never delete on stop (05 §3.1).
func Unit(nftPath string) []byte {
	return []byte(`# Managed by Hashi. Loads Hashi's nftables table (table inet hashi) at boot. It never flushes the ruleset and
# leaves the table in place when stopped.
[Unit]
Description=Hashi firewall table
Wants=network-online.target
After=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=` + nftPath + ` -j -f ` + TableFile + `
ExecReload=` + nftPath + ` -j -f ` + TableFile + `

[Install]
WantedBy=multi-user.target
`)
}

// Sysctl renders the forwarding settings.
func Sysctl(ipv6 bool) []byte {
	s := "# Managed by Hashi: forwarding for published ports and managed subnets.\nnet.ipv4.ip_forward=1\n"
	if ipv6 {
		s += "net.ipv6.conf.all.forwarding=1\n"
	}
	return []byte(s)
}

// Rollback renders rollback.sh: delete Hashi's table, load the previous one and put the previous rendered file back;
// on a first apply (no previous) the boot unit is disabled again. It touches nothing but Hashi's own objects.
func Rollback(nftPath string) []byte {
	return []byte(`#!/bin/sh
# Hashi rescue rollback (05 §7). Runs from the hashi-fw-rescue timer when an apply was not confirmed in time.
` + nftPath + ` delete table inet ` + TableName + ` 2>/dev/null
if [ -s ` + PreviousFile + ` ]; then
	` + nftPath + ` -j -f ` + PreviousFile + `
fi
if [ -s ` + PrevTableFile + ` ]; then
	cp ` + PrevTableFile + ` ` + TableFile + `
else
	rm -f ` + TableFile + `
	systemctl disable ` + UnitName + ` 2>/dev/null
fi
if [ -x ` + takeoverRestore + ` ]; then
	` + takeoverRestore + `
	rm -rf ` + TakeoverDir + `.rolled-back
	mv ` + TakeoverDir + ` ` + TakeoverDir + `.rolled-back
fi
`)
}

// Observe reads the host fresh and returns what differs from d.
func Observe(ctx context.Context, h Host, d Desired) (Diff, error) {
	diff := Diff{InstallNft: d.InstallNft, Risk: hsync.Low}
	want := []FileChange{{Path: TableFile, Mode: 0o600, Data: d.Table}, {Path: SysctlFile, Mode: 0o644, Data: Sysctl(d.IPv6)},
		{Path: UnitFile, Mode: 0o644, Data: Unit(d.NftPath)}}
	for _, f := range want {
		cur, err := h.ReadFile(f.Path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return diff, fmt.Errorf("read %s: %w", f.Path, err)
		}
		if f.Path == TableFile && cur == nil {
			diff.First = true
		}
		if !bytes.Equal(cur, f.Data) {
			f.HostData = cur
			diff.Files = append(diff.Files, f)
		}
	}
	if d.InstallNft == "" {
		r, err := h.Run(ctx, "nft list tables", nil)
		if err != nil {
			return diff, err
		}
		diff.TableMissing = !strings.Contains(string(r.Stdout), "table inet "+TableName)
	} else {
		diff.TableMissing = true
	}
	r, err := h.Run(ctx, "systemctl is-enabled "+UnitName, nil)
	if err != nil {
		return diff, err
	}
	diff.UnitDisabled = strings.TrimSpace(string(r.Stdout)) != "enabled"

	high := func(why string) { diff.Risk, diff.Reasons = hsync.High, append(diff.Reasons, why) }
	if diff.First {
		high("first apply: Hashi's table is created under the rescue timer")
	}
	if diff.InstallNft != "" {
		high("nftables is installed")
	}
	for _, f := range diff.Files {
		switch f.Path {
		case TableFile:
			if !diff.First {
				for _, why := range TableRisk(f.HostData, f.Data, d.Guard) {
					high(why)
				}
			}
		case SysctlFile, UnitFile:
			high(f.Path + " changes")
		}
	}
	if diff.TableMissing && !diff.First {
		high("the kernel has no Hashi table: it is loaded again")
	}
	if !d.Takeover.Empty() {
		var err error
		if diff.Takeover, err = ObserveTakeover(ctx, h, d.Takeover); err != nil {
			return diff, fmt.Errorf("read the old firewall script's objects: %w", err)
		}
		if !diff.Takeover.Empty() {
			high("the old firewall script is removed")
		}
	}
	return diff, nil
}

// Items are the plan items; paths and hashes only.
func (d Diff) Items() []hsync.Item {
	var items []hsync.Item
	note := strings.Join(d.Reasons, "; ")
	if d.InstallNft != "" {
		items = append(items, hsync.Item{Action: hsync.ActionCreate, Object: "package nftables", Risk: hsync.High,
			After: raw(map[string]string{"command": d.InstallNft}), Note: "the nftables.service unit stays disabled"})
	}
	for _, f := range d.Files {
		it := hsync.Item{Action: hsync.ActionCreate, Object: f.Path, Risk: hsync.Low,
			After: raw(map[string]string{"sha256": sha(f.Data), "mode": fmt.Sprintf("%o", f.Mode)})}
		if f.HostData != nil {
			it.Action, it.Before = hsync.ActionUpdate, raw(map[string]string{"sha256": sha(f.HostData)})
		}
		if d.Risk == hsync.High {
			it.Risk, it.Note = hsync.High, note
		}
		items = append(items, it)
	}
	if d.TableMissing || len(d.Files) > 0 {
		items = append(items, hsync.Item{Action: hsync.ActionUpdate, Object: "table inet " + TableName, Risk: d.Risk,
			After: raw(map[string]string{"sha256": sha(fileData(d))}), Note: note})
	}
	if d.UnitDisabled {
		items = append(items, hsync.Item{Action: hsync.ActionCreate, Object: "enable " + UnitName, Risk: hsync.High,
			Note: "loads the table at boot"})
	}
	if !d.Takeover.Empty() {
		items = append(items, d.Takeover.items...)
	}
	return items
}

func fileData(d Diff) []byte {
	for _, f := range d.Files {
		if f.Path == TableFile {
			return f.Data
		}
	}
	return nil
}

func raw(v any) json.RawMessage {
	b, _ := json.Marshal(v) // plain maps marshal without error
	return b
}

// ErrRescuePending means the apply could not be confirmed: the rescue timer will restore the previous table.
var ErrRescuePending = errors.New("the host could not be confirmed after the apply; the rescue timer restores the previous table")

// Rescue proves the host is still reachable after a risky apply: a new SSH connection over the rescue path that runs a
// command, and NetBird still connected when it is enabled (05 §7 step 5).
type Rescue func(ctx context.Context) error

// Apply writes the changed files and loads the table. A high-risk diff runs under the rescue timer: it is armed
// before anything changes and disarmed only after rescue succeeded.
func Apply(ctx context.Context, h Host, d Desired, diff Diff, rescue Rescue) error {
	run := func(cmd string) error {
		r, err := h.Run(ctx, cmd, nil)
		if err != nil {
			return fmt.Errorf("%s: %w", cmd, err)
		}
		if !r.OK() {
			return fmt.Errorf("%s: exit %d: %s", cmd, r.ExitCode, strings.TrimSpace(string(r.Stderr)+string(r.Stdout)))
		}
		return nil
	}
	if err := h.MkdirAll(Dir, 0o700); err != nil {
		return err
	}
	armed := diff.Risk == hsync.High
	takeover := !diff.Takeover.Empty()
	if takeover { // saved before the timer is armed, so a rollback always finds them
		if err := diff.Takeover.save(h); err != nil {
			return fmt.Errorf("save the old firewall script's objects (nothing changed): %w", err)
		}
	}
	if armed && !takeover {
		if err := run(staleTakeoverCleanup); err != nil {
			return fmt.Errorf("clear old takeover files (nothing changed): %w", err)
		}
	}
	if armed {
		if err := arm(ctx, h, d, run); err != nil {
			return fmt.Errorf("arm the rescue timer (nothing changed): %w", err)
		}
	}
	if diff.InstallNft != "" {
		if err := run(diff.InstallNft); err != nil {
			return err
		}
	}
	for _, f := range diff.Files {
		if _, err := h.WriteFileAtomic(f.Path, f.Data, f.Mode); err != nil {
			return fmt.Errorf("write %s: %w", f.Path, err)
		}
	}
	// Containers and some LXCs cannot write sysctls; the value already in effect is good enough.
	if err := run("sysctl -p " + SysctlFile); err != nil {
		keys, want := "net.ipv4.ip_forward", "1"
		if d.IPv6 {
			keys, want = keys+" net.ipv6.conf.all.forwarding", "1 1"
		}
		r, rerr := h.Run(ctx, "sysctl -n "+keys, nil)
		if rerr != nil || strings.Join(strings.Fields(string(r.Stdout)), " ") != want {
			return err
		}
	}
	steps := []string{d.NftPath + " -j -f " + TableFile}
	if takeover {
		steps = append(append(diff.Takeover.cronSteps(), steps...), diff.Takeover.removeSteps()...)
	}
	if diff.UnitDisabled || slices.ContainsFunc(diff.Files, func(f FileChange) bool { return f.Path == UnitFile }) {
		steps = append(steps, "systemctl daemon-reload", "systemctl enable "+UnitName)
	}
	for _, s := range steps {
		if err := run(s); err != nil {
			if armed {
				return fmt.Errorf("%w (%w)", ErrRescuePending, err)
			}
			return err
		}
	}
	if err := Verify(ctx, h, d); err != nil {
		if armed {
			return fmt.Errorf("%w (%w)", ErrRescuePending, err)
		}
		return err
	}
	if takeover {
		left, err := ObserveTakeover(ctx, h, d.Takeover)
		if err == nil && !left.Empty() {
			err = fmt.Errorf("%d object(s) of the old firewall script are still there", len(left.items))
		}
		if err != nil {
			return fmt.Errorf("%w (%w)", ErrRescuePending, err)
		}
	}
	if !armed {
		return nil
	}
	rctx, cancel := context.WithTimeout(ctx, RescueWindow/2)
	defer cancel()
	if err := rescue(rctx); err != nil {
		return fmt.Errorf("%w (%w)", ErrRescuePending, err)
	}
	if err := run("systemctl stop " + RescueUnit + ".timer"); err != nil {
		return err
	}
	if takeover { // confirmed: later rollbacks must not bring the script back
		return run("rm -rf " + TakeoverDoneDir + " && mv " + TakeoverDir + " " + TakeoverDoneDir)
	}
	return nil
}

// arm saves the live table and the current file, writes rollback.sh (0700) and starts the timer (05 §7 steps 1–3).
func arm(ctx context.Context, h Host, d Desired, run func(string) error) error {
	prev := []byte{}
	r, err := h.Run(ctx, d.NftPath+" -j list table inet "+TableName, nil)
	if err == nil && r.OK() && len(bytes.TrimSpace(r.Stdout)) > 0 {
		if prev, err = stripHandles(r.Stdout); err != nil {
			return fmt.Errorf("previous table: %w", err)
		}
	}
	if _, err := h.WriteFileAtomic(PreviousFile, prev, 0o600); err != nil {
		return err
	}
	cur, err := h.ReadFile(TableFile)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if _, err := h.WriteFileAtomic(PrevTableFile, cur, 0o600); err != nil {
		return err
	}
	if _, err := h.WriteFileAtomic(RollbackFile, Rollback(d.NftPath), 0o700); err != nil {
		return err
	}
	// A leftover or failed unit of the same name blocks arming (testdata/contracts/firewall/README.md).
	_, _ = h.Run(ctx, "systemctl stop "+RescueUnit+".timer; systemctl reset-failed "+RescueUnit+".service", nil)
	return run(fmt.Sprintf("systemd-run --on-active=%d --timer-property=AccuracySec=1s --unit=%s %s",
		int(RescueWindow.Seconds()), RescueUnit, RollbackFile))
}

// stripHandles turns `nft -j list table` output into loadable input (testdata/contracts/nft finding 1).
func stripHandles(b []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	var walk func(any) any
	walk = func(x any) any {
		switch t := x.(type) {
		case map[string]any:
			delete(t, "handle")
			for k, e := range t {
				t[k] = walk(e)
			}
		case []any:
			for i, e := range t {
				t[i] = walk(e)
			}
		}
		return x
	}
	return json.Marshal(walk(v))
}

// Verify reads the live table back: every chain present and every plan-owned set covering exactly the rendered
// addresses and ports (nft may merge adjacent prefixes into ranges, so coverage is compared, not text).
func Verify(ctx context.Context, h Host, d Desired) error {
	r, err := h.Run(ctx, d.NftPath+" -j list table inet "+TableName, nil)
	if err != nil {
		return err
	}
	if !r.OK() {
		return fmt.Errorf("table inet %s is not loaded: %s", TableName, strings.TrimSpace(string(r.Stderr)))
	}
	live, err := listed(r.Stdout)
	if err != nil {
		return err
	}
	want, err := listed(d.Table)
	if err != nil {
		return err
	}
	for name := range want.chains {
		if !live.chains[name] {
			return fmt.Errorf("chain %s missing", name)
		}
	}
	for _, name := range planSets {
		if !live.sets[name].Equal(want.sets[name]) || !slices.Equal(live.ports[name], want.ports[name]) {
			return fmt.Errorf("set %s does not hold the rendered elements", name)
		}
	}
	return nil
}

type contents struct {
	chains map[string]bool
	sets   map[string]*netipx.IPSet
	ports  map[string][]int
}

// listed reads chains and set elements from either a rendered batch or an nft -j listing.
func listed(b []byte) (contents, error) {
	var doc struct {
		Nftables []map[string]json.RawMessage `json:"nftables"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		return contents{}, err
	}
	c := contents{chains: map[string]bool{}, sets: map[string]*netipx.IPSet{}, ports: map[string][]int{}}
	builders := map[string]*netipx.IPSetBuilder{}
	addElems := func(name string, elems []json.RawMessage) {
		if builders[name] == nil {
			builders[name] = &netipx.IPSetBuilder{}
		}
		for _, e := range elems {
			addElem(builders[name], c.ports, name, e)
		}
	}
	for _, o := range doc.Nftables {
		var inner map[string]json.RawMessage
		for k, v := range o {
			switch k {
			case "add":
				_ = json.Unmarshal(v, &inner)
			case "chain", "set":
				inner = map[string]json.RawMessage{k: v}
			}
		}
		if ch, ok := inner["chain"]; ok {
			var x struct{ Name string }
			_ = json.Unmarshal(ch, &x)
			c.chains[x.Name] = true
		}
		var el struct {
			Name string            `json:"name"`
			Elem []json.RawMessage `json:"elem"`
		}
		if s, ok := inner["set"]; ok {
			_ = json.Unmarshal(s, &el)
			addElems(el.Name, el.Elem)
		}
		if s, ok := inner["element"]; ok {
			_ = json.Unmarshal(s, &el)
			addElems(el.Name, el.Elem)
		}
	}
	for name, bl := range builders {
		s, err := bl.IPSet()
		if err != nil {
			return c, err
		}
		c.sets[name] = s
		slices.Sort(c.ports[name])
	}
	for _, name := range planSets {
		if c.sets[name] == nil {
			c.sets[name], _ = (&netipx.IPSetBuilder{}).IPSet()
		}
	}
	return c, nil
}

// addElem reads one element: "addr", 443, {"prefix":…}, {"range":[a,b]} or {"elem":{"val":…}}.
func addElem(b *netipx.IPSetBuilder, ports map[string][]int, set string, e json.RawMessage) {
	var n int
	if json.Unmarshal(e, &n) == nil {
		ports[set] = append(ports[set], n)
		return
	}
	var s string
	if json.Unmarshal(e, &s) == nil {
		if a, err := netip.ParseAddr(s); err == nil {
			b.Add(a)
		}
		return
	}
	var o struct {
		Prefix *struct {
			Addr string `json:"addr"`
			Len  int    `json:"len"`
		} `json:"prefix"`
		Range []string `json:"range"`
		Elem  *struct {
			Val json.RawMessage `json:"val"`
		} `json:"elem"`
	}
	if json.Unmarshal(e, &o) != nil {
		return
	}
	switch {
	case o.Prefix != nil:
		if a, err := netip.ParseAddr(o.Prefix.Addr); err == nil {
			b.AddPrefix(netip.PrefixFrom(a, o.Prefix.Len))
		}
	case len(o.Range) == 2:
		lo, err1 := netip.ParseAddr(o.Range[0])
		hi, err2 := netip.ParseAddr(o.Range[1])
		if err1 == nil && err2 == nil {
			b.AddRange(netipx.IPRangeFrom(lo, hi))
		}
	case o.Elem != nil:
		addElem(b, ports, set, o.Elem.Val)
	}
}

// TableRisk lists why a table change is high risk (05 §4.4); none means low. Structure, chains and published ports
// are high; allow and host-access removals that cover the admin's or the rescue address are high; a newly blocked
// range of /16 or larger (IPv6 /32) is high.
func TableRisk(old, new []byte, g Guard) []string {
	o, err1 := listed(old)
	n, err2 := listed(new)
	if err1 != nil || err2 != nil {
		return []string{"the previous table cannot be read"}
	}
	var why []string
	if structure(old) != structure(new) {
		why = append(why, "chains or sets change")
	}
	for _, p := range []string{"pub_tcp", "pub_udp", "metered_udp"} {
		if !slices.Equal(o.ports[p], n.ports[p]) {
			why = append(why, "published ports change")
			break
		}
	}
	for _, set := range []string{"allow", "host_access"} {
		for _, a := range append(slices.Clone(g.AdminIPs), g.RescueIP) {
			if !a.IsValid() {
				continue
			}
			name := set + suffix(a.Is4())
			if o.sets[name].Contains(a) && !n.sets[name].Contains(a) {
				why = append(why, fmt.Sprintf("%s no longer contains %s", name, a))
			}
		}
	}
	for _, name := range []string{"block4", "block6"} {
		var added netipx.IPSetBuilder
		added.AddSet(n.sets[name])
		added.RemoveSet(o.sets[name])
		s, _ := added.IPSet()
		limit := 16
		if name == "block6" {
			limit = 32
		}
		wide := ""
		for _, p := range s.Prefixes() {
			if p.Bits() <= limit {
				wide = p.String()
				break
			}
		}
		for _, p := range g.Blocks {
			if wide == "" && p.Addr().Is4() == (name == "block4") && p.Bits() <= limit && s.OverlapsPrefix(p) {
				wide = p.String()
			}
		}
		if wide != "" {
			why = append(why, fmt.Sprintf("%s newly blocks %s", name, wide))
		}
	}
	return why
}

// structure is the rendered batch without the elements of the allow, block and host-access sets.
func structure(b []byte) string {
	var doc struct {
		Nftables []map[string]map[string]json.RawMessage `json:"nftables"`
	}
	if json.Unmarshal(b, &doc) != nil {
		return ""
	}
	var out []string
	for _, c := range doc.Nftables {
		if el, ok := c["add"]["element"]; ok {
			var x struct{ Name string }
			_ = json.Unmarshal(el, &x)
			if !strings.HasPrefix(x.Name, "pub_") && x.Name != "metered_udp" {
				continue
			}
		}
		line, _ := json.Marshal(c)
		out = append(out, string(line))
	}
	return strings.Join(out, "\n")
}
