package firewall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"slices"
	"strings"

	hsync "git.juzo.io/juzo/hashi/internal/sync"
)

// Takeover names what belongs to a host's old firewall script (05 §3.4). Nothing is guessed: only what is listed here
// is removed, and names of NetBird, Docker or Proxmox are refused. Formats: testdata/contracts/firewall (iptables-legacy).
type Takeover struct {
	// Chains are the script's own chains: flushed and deleted, with every rule elsewhere that jumps to them.
	Chains []string `json:"chains,omitempty"`
	// Rules are its rules in built-in chains, "<table> -A <chain> <spec>" as iptables-legacy-save prints them. Every
	// copy of a rule is removed.
	Rules []string `json:"rules,omitempty"`
	// IPSets are destroyed, with every rule elsewhere that matches them.
	IPSets []string `json:"ipsets,omitempty"`
	// CronMatch removes the lines holding one of these strings from root's crontab and from CronFiles.
	CronMatch []string `json:"cron_match,omitempty"`
	// CronFiles are files the script installs the crontab from.
	CronFiles []string `json:"cron_files,omitempty"`
}

// Empty reports no takeover.
func (t Takeover) Empty() bool {
	return len(t.Chains)+len(t.Rules)+len(t.IPSets)+len(t.CronMatch)+len(t.CronFiles) == 0
}

var (
	takeoverName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,28}$`)
	takeoverPath = regexp.MustCompile(`^/[A-Za-z0-9._/-]+$`)
	foreignName  = regexp.MustCompile(`(?i)NETBIRD|DOCKER|PVEFW|PROXMOX|\bnb[-0-9]`)
	builtinChain = []string{"INPUT", "OUTPUT", "FORWARD", "PREROUTING", "POSTROUTING"}
	ruleTables   = []string{"raw", "mangle", "nat", "filter", "security"}
)

// Validate refuses anything that could touch rules Hashi must never touch (05 §5).
func (t Takeover) Validate() error {
	bad := func(format string, a ...any) error {
		return fmt.Errorf("%w: "+format, append([]any{ErrInvalid}, a...)...)
	}
	for _, c := range t.Chains {
		if !takeoverName.MatchString(c) || slices.Contains(builtinChain, c) || foreignName.MatchString(c) {
			return bad("chain %q cannot be taken over", c)
		}
	}
	for _, s := range t.IPSets {
		if !takeoverName.MatchString(s) || foreignName.MatchString(s) {
			return bad("ipset %q cannot be taken over", s)
		}
	}
	for _, r := range t.Rules {
		f := strings.Fields(r)
		if len(f) < 4 || !slices.Contains(ruleTables, f[0]) || f[1] != "-A" || foreignName.MatchString(r) || strings.ContainsAny(r, "\n\r") {
			return bad("rule %q is not \"<table> -A <chain> <spec>\" of the script", r)
		}
	}
	for _, m := range t.CronMatch {
		if len(strings.TrimSpace(m)) < 4 || strings.ContainsAny(m, "\n\r") {
			return bad("cron match %q is too broad", m)
		}
	}
	for _, p := range t.CronFiles {
		if !takeoverPath.MatchString(p) || strings.Contains(p, "..") {
			return bad("cron file %q is not a plain absolute path", p)
		}
	}
	return nil
}

const (
	// TakeoverDir holds the saved objects while an apply is unconfirmed; rollback.sh restores them when it exists. A
	// confirmed apply moves it to TakeoverDoneDir, so later rollbacks never bring the script back (05 §7).
	TakeoverDir      = Dir + "/takeover"
	TakeoverDoneDir  = Dir + "/takeover-done"
	takeoverRestore  = TakeoverDir + "/restore.sh"
	takeoverCrontab  = TakeoverDir + "/crontab.saved"
	takeoverCronNew  = TakeoverDir + "/crontab.new"
	takeoverIPSets   = TakeoverDir + "/ipsets.saved"
	takeoverRestore4 = TakeoverDir + "/restore.iptables"
	takeoverRestore6 = TakeoverDir + "/restore.ip6tables"
	takeoverRemove4  = TakeoverDir + "/remove.iptables"
	takeoverRemove6  = TakeoverDir + "/remove.ip6tables"
)

// takeoverFile is a cron file with and without the script's lines.
type takeoverFile struct {
	Path     string
	Old, New []byte
	Removed  []string
}

// TakeoverPlan is what is on the host now. Restore and remove inputs are iptables-restore --noflush scripts; removing
// through one input keeps thousands of duplicates to one atomic commit per table and needs no shell quoting.
type TakeoverPlan struct {
	Remove4, Remove6, Restore4, Restore6 string
	IPSets                               []string // to destroy
	IPSetSave                            string   // their `ipset save` lines
	Crontab, CrontabNew                  string
	CronRemoved                          []string
	Files                                []takeoverFile
	items                                []hsync.Item
}

// Empty reports nothing left to remove.
func (p *TakeoverPlan) Empty() bool { return p == nil || len(p.items) == 0 }

// sectionRules turns one iptables-save output into the remove and restore inputs.
func sectionRules(save, family string, t Takeover, items *[]hsync.Item) (remove, restore string) {
	var rm, rs strings.Builder
	for _, block := range strings.Split(save, "\nCOMMIT") {
		i := strings.Index(block, "*")
		if i < 0 {
			continue
		}
		lines := strings.Split(block[i:], "\n")
		table := lines[0][1:]
		var del, chains, restoreLines []string
		copies := map[string]int{}
		for _, c := range t.Chains {
			if slices.Contains(lines, ":"+c+" - [0:0]") || slices.ContainsFunc(lines, func(l string) bool { return strings.HasPrefix(l, ":"+c+" ") }) {
				chains = append(chains, c)
				restoreLines = append(restoreLines, ":"+c+" - [0:0]")
			}
		}
		for _, l := range lines[1:] {
			if !strings.HasPrefix(l, "-A ") {
				continue
			}
			f := strings.Fields(l)
			switch {
			case slices.Contains(chains, f[1]):
				restoreLines = append(restoreLines, l)
			case ownsRule(l, table, t):
				restoreLines = append(restoreLines, l)
				del = append(del, "-D"+l[2:])
				copies[l]++
			}
		}
		if len(del)+len(chains) == 0 {
			continue
		}
		rm.WriteString("*" + table + "\n" + strings.Join(del, "\n") + "\n")
		for _, c := range chains {
			rm.WriteString("-F " + c + "\n-X " + c + "\n")
		}
		rm.WriteString("COMMIT\n")
		rs.WriteString("*" + table + "\n" + strings.Join(restoreLines, "\n") + "\nCOMMIT\n")
		keys := make([]string, 0, len(copies))
		for k := range copies {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			*items = append(*items, hsync.Item{Action: hsync.ActionDelete, Object: family + " " + table + ": " + k, Risk: hsync.High,
				Before: raw(map[string]int{"copies": copies[k]}), Note: "old firewall script; restored if the apply is not confirmed"})
		}
		for _, c := range chains {
			*items = append(*items, hsync.Item{Action: hsync.ActionDelete, Object: family + " " + table + ": chain " + c, Risk: hsync.High,
				Note: "old firewall script; restored if the apply is not confirmed"})
		}
	}
	return rm.String(), rs.String()
}

func ownsRule(line, table string, t Takeover) bool {
	f := strings.Fields(line)
	for i, w := range f {
		if w == "-j" && i+1 < len(f) && slices.Contains(t.Chains, f[i+1]) {
			return true
		}
		if w == "--match-set" && i+1 < len(f) && slices.Contains(t.IPSets, f[i+1]) {
			return true
		}
	}
	return slices.Contains(t.Rules, table+" "+line)
}

func cronFilter(text string, match []string) (kept string, removed []string) {
	var keep []string
	for _, l := range strings.SplitAfter(text, "\n") {
		if slices.ContainsFunc(match, func(m string) bool { return strings.Contains(l, m) }) {
			removed = append(removed, strings.TrimSpace(l))
			continue
		}
		keep = append(keep, l)
	}
	return strings.Join(keep, ""), removed
}

// ObserveTakeover reads the host and returns what of t is still there. It only reads.
func ObserveTakeover(ctx context.Context, h Host, t Takeover) (*TakeoverPlan, error) {
	read := func(cmd string) (string, error) {
		r, err := h.Run(ctx, cmd, nil)
		if err != nil {
			return "", fmt.Errorf("%s: %w", cmd, err)
		}
		if !r.OK() {
			return "", fmt.Errorf("%s: exit %d: %s", cmd, r.ExitCode, strings.TrimSpace(string(r.Stderr)))
		}
		return string(r.Stdout), nil
	}
	p := &TakeoverPlan{}
	if len(t.Chains)+len(t.Rules)+len(t.IPSets) > 0 {
		for _, fam := range []struct {
			cmd, name string
			rm, rs    *string
		}{{"iptables-legacy-save", "iptables", &p.Remove4, &p.Restore4}, {"ip6tables-legacy-save", "ip6tables", &p.Remove6, &p.Restore6}} {
			save, err := read(fam.cmd)
			if err != nil {
				return nil, err
			}
			*fam.rm, *fam.rs = sectionRules(save, fam.name, t, &p.items)
		}
	}
	if len(t.IPSets) > 0 {
		save, err := read("ipset save")
		if err != nil {
			return nil, err
		}
		for _, l := range strings.Split(save, "\n") {
			f := strings.Fields(l)
			if len(f) >= 2 && slices.Contains(t.IPSets, f[1]) {
				p.IPSetSave += l + "\n"
				if f[0] == "create" {
					p.IPSets = append(p.IPSets, f[1])
					p.items = append(p.items, hsync.Item{Action: hsync.ActionDelete, Object: "ipset " + f[1], Risk: hsync.High,
						Note: "old firewall script; restored if the apply is not confirmed"})
				}
			}
		}
	}
	if len(t.CronMatch) > 0 {
		// crontab -l exits 1 when root has no crontab.
		r, err := h.Run(ctx, "crontab -l", nil)
		if err != nil {
			return nil, fmt.Errorf("crontab -l: %w", err)
		}
		if r.OK() {
			p.Crontab = string(r.Stdout)
			p.CrontabNew, p.CronRemoved = cronFilter(p.Crontab, t.CronMatch)
			for _, l := range p.CronRemoved {
				p.items = append(p.items, hsync.Item{Action: hsync.ActionDelete, Object: "root crontab: " + l, Risk: hsync.High,
					Note: "old firewall script; restored if the apply is not confirmed"})
			}
		}
		for _, path := range t.CronFiles {
			b, err := h.ReadFile(path)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", path, err)
			}
			kept, removed := cronFilter(string(b), t.CronMatch)
			if len(removed) == 0 {
				continue
			}
			p.Files = append(p.Files, takeoverFile{Path: path, Old: b, New: []byte(kept), Removed: removed})
			for _, l := range removed {
				p.items = append(p.items, hsync.Item{Action: hsync.ActionDelete, Object: path + ": " + l, Risk: hsync.High,
					Note: "old firewall script; restored if the apply is not confirmed"})
			}
		}
	}
	return p, nil
}

// save writes everything the rollback needs (before the timer is armed) and restore.sh, which puts it all back.
func (p *TakeoverPlan) save(h Host) error {
	if err := h.MkdirAll(TakeoverDir, 0o700); err != nil {
		return err
	}
	var sh strings.Builder
	sh.WriteString("#!/bin/sh\n# Restores what Hashi removed from the old firewall script (05 §7).\n")
	write := func(path, data string) error {
		_, err := h.WriteFileAtomic(path, []byte(data), 0o600)
		return err
	}
	type file struct{ path, data string }
	files := []file{{takeoverRemove4, p.Remove4}, {takeoverRemove6, p.Remove6}, {takeoverRestore4, p.Restore4}, {takeoverRestore6, p.Restore6},
		{takeoverIPSets, p.IPSetSave}, {takeoverCrontab, p.Crontab}, {takeoverCronNew, p.CrontabNew}}
	if p.IPSetSave != "" {
		sh.WriteString("ipset restore -exist < " + takeoverIPSets + "\n")
	}
	if p.Restore4 != "" {
		sh.WriteString("iptables-legacy-restore --noflush < " + takeoverRestore4 + "\n")
	}
	if p.Restore6 != "" {
		sh.WriteString("ip6tables-legacy-restore --noflush < " + takeoverRestore6 + "\n")
	}
	if len(p.CronRemoved) > 0 {
		sh.WriteString("crontab " + takeoverCrontab + "\n")
	}
	for i, f := range p.Files {
		saved := fmt.Sprintf("%s/file%d.saved", TakeoverDir, i)
		files = append(files, file{saved, string(f.Old)}, file{fmt.Sprintf("%s/file%d.new", TakeoverDir, i), string(f.New)})
		sh.WriteString("cat " + saved + " > " + f.Path + "\n")
	}
	for _, f := range files {
		if err := write(f.path, f.data); err != nil {
			return err
		}
	}
	_, err := h.WriteFileAtomic(takeoverRestore, []byte(sh.String()), 0o700)
	return err
}

// cronSteps come before the table: the script's 15-minute run must not put anything back while it is being removed.
// The crontab goes after the files it may be re-installed from.
func (p *TakeoverPlan) cronSteps() []string {
	var out []string
	for i, f := range p.Files {
		out = append(out, fmt.Sprintf("cat %s/file%d.new > %s", TakeoverDir, i, f.Path))
	}
	if len(p.CronRemoved) > 0 {
		out = append(out, "crontab "+takeoverCronNew)
	}
	return out
}

// removeSteps come after Hashi's table is loaded.
func (p *TakeoverPlan) removeSteps() []string {
	var out []string
	if p.Remove4 != "" {
		out = append(out, "iptables-legacy-restore --noflush < "+takeoverRemove4)
	}
	if p.Remove6 != "" {
		out = append(out, "ip6tables-legacy-restore --noflush < "+takeoverRemove6)
	}
	for _, s := range p.IPSets {
		out = append(out, "ipset destroy "+s)
	}
	return out
}

// ParseTakeover reads a stored takeover.
func ParseTakeover(s string) (Takeover, error) {
	var t Takeover
	if s == "" {
		return t, nil
	}
	return t, json.Unmarshal([]byte(s), &t)
}
