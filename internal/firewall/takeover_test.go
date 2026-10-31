package firewall

import (
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/sshx"
)

// captureHost answers the takeover reads from the capture of the first real host.
type captureHost struct{ sections map[string]string }

func (h captureHost) Run(_ context.Context, cmd string, _ io.Reader) (sshx.Result, error) {
	return sshx.Result{Stdout: []byte(h.sections[cmd])}, nil
}
func (captureHost) ReadFile(string) ([]byte, error)                           { return nil, fs.ErrNotExist }
func (captureHost) WriteFileAtomic(string, []byte, fs.FileMode) (bool, error) { return true, nil }
func (captureHost) MkdirAll(string, fs.FileMode) error                        { return nil }

func capturedTakeover(t *testing.T) captureHost {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "firewall", "takeover_iptables_legacy.txt"))
	if err != nil {
		t.Fatal(err)
	}
	h := captureHost{sections: map[string]string{}}
	for _, sec := range strings.Split("\n"+string(b), "\n### ")[1:] {
		cmd, body, _ := strings.Cut(sec, "\n")
		h.sections[cmd] = body
	}
	return h
}

var realTakeover = Takeover{Chains: []string{"CUSTOM_DNAT", "CUSTOM_FWD", "CUSTOM_INPUT"},
	Rules: []string{"mangle -A FORWARD -p tcp -m tcp --tcp-flags SYN,RST SYN -j TCPMSS --clamp-mss-to-pmtu",
		"nat -A POSTROUTING -s 10.0.0.0/8 -d 10.30.0.1/32 -j MASQUERADE", "nat -A POSTROUTING -s 10.0.0.0/8 -o vmbr0 -j MASQUERADE",
		"nat -A POSTROUTING -s 10.30.0.0/24 -d 10.0.0.0/8 -o wt0 -j MASQUERADE",
		"filter -A FORWARD -i wt0 -o vmbr1 -j ACCEPT", "filter -A FORWARD -i vmbr1 -o wt0 -j ACCEPT"},
	IPSets: []string{"trusted_hosts"}, CronMatch: []string{"/scripts/firewall.sh", "/scripts/sync_and_apply.sh"}}

// The first real host's capture (05 §3.4): every duplicate goes, NetBird's rules and sets and the unrelated cron jobs
// stay, and the restore input holds exactly what is removed.
func TestTakeoverOfCapturedScript(t *testing.T) {
	if err := realTakeover.Validate(); err != nil {
		t.Fatal(err)
	}
	p, err := ObserveTakeover(t.Context(), capturedTakeover(t), realTakeover)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(p.Remove4, "-D FORWARD -i wt0 -o vmbr1 -j ACCEPT\n"); n != 561 {
		t.Errorf("copies removed: %d", n)
	}
	for _, want := range []string{"-D INPUT -j CUSTOM_INPUT", "-D PREROUTING -j CUSTOM_DNAT", "-D OUTPUT -j CUSTOM_DNAT", "-D FORWARD -j CUSTOM_FWD",
		"-D FORWARD -m set --match-set trusted_hosts src -j ACCEPT", "-X CUSTOM_DNAT", "-D POSTROUTING -s 10.0.0.0/8 -o vmbr0 -j MASQUERADE"} {
		if !strings.Contains(p.Remove4, want+"\n") {
			t.Errorf("remove lacks %q", want)
		}
	}
	for _, in := range []string{p.Remove4, p.Remove6, p.Restore4, p.Restore6, p.IPSetSave} {
		if strings.Contains(in, "NETBIRD") || strings.Contains(in, "nb-") || strings.Contains(in, "0x1bd") {
			t.Errorf("a NetBird object is in the takeover:\n%.400s", in)
		}
	}
	if !strings.Contains(p.Restore4, "-A CUSTOM_INPUT -j DROP\n") || !strings.Contains(p.Restore4, ":CUSTOM_INPUT - [0:0]\n") {
		t.Error("restore lacks the script's chain")
	}
	if strings.Join(p.IPSets, ",") != "trusted_hosts" || strings.Count(p.IPSetSave, "add trusted_hosts") != 2 {
		t.Errorf("ipsets %v\n%s", p.IPSets, p.IPSetSave)
	}
	if len(p.CronRemoved) != 4 || !strings.Contains(p.CrontabNew, "shrink_logs") || !strings.Contains(p.CrontabNew, "mount_rclone") ||
		strings.Contains(p.CrontabNew, "firewall.sh") {
		t.Errorf("cron removed %v, new:\n%s", p.CronRemoved, p.CrontabNew)
	}
	if p.Empty() {
		t.Error("empty plan")
	}
}

// Names Hashi must never touch are refused whatever is asked (05 §5).
func TestTakeoverRefusesForeignObjects(t *testing.T) {
	for _, tk := range []Takeover{{Chains: []string{"NETBIRD-ACL-INPUT"}}, {Chains: []string{"FORWARD"}}, {Chains: []string{"DOCKER-USER"}},
		{IPSets: []string{"nb-5223c4bd"}}, {IPSets: []string{"PVEFW-1"}}, {Rules: []string{"filter -A FORWARD -j NETBIRD-RT-FWD-IN"}},
		{Rules: []string{"-A FORWARD -j ACCEPT"}}, {CronMatch: []string{"a"}}, {CronFiles: []string{"/etc/cron; rm -rf"}}} {
		if err := tk.Validate(); err == nil {
			t.Errorf("accepted %+v", tk)
		}
	}
}
