package firewall

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "firewall", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The fixtures were produced by exactly this command.
func TestProbeCommandIsTheCapturedOne(t *testing.T) {
	if strings.TrimSpace(fixture(t, "probe.sh")) != ProbeCommand {
		t.Fatal("ProbeCommand differs from testdata/contracts/firewall/probe.sh: recapture the fixtures")
	}
}

func TestParseProbeNetBirdHost(t *testing.T) {
	f, err := ParseProbe(fixture(t, "probe_debian13_pve_netbird.txt"))
	if err != nil {
		t.Fatal(err)
	}
	got := fmt.Sprintf("%s %s nft %s json=%v ipt=%s tables=%v wan=%s gw=%s pub=%s unit=%v firewalld=%v fwd=%v/%v systemd=%v missing=%v",
		f.OSID, f.Kernel, f.NftVersion, f.NftJSON, f.IptablesBackend, f.Tables, f.WAN, f.Gateway, f.PublicIP,
		f.NftablesUnitEnabled, f.FirewalldActive, f.IPForward, f.IP6Forward, f.Systemd, f.Missing)
	want := "debian 7.0.2-6-pve nft 1.1.3 json=true ipt=legacy tables=[] wan=vmbr0 gw=198.18.3.2 pub=198.18.3.1 unit=false firewalld=false fwd=true/true systemd=true missing=[rc-service]"
	if got != want {
		t.Fatalf("\n got %s\nwant %s", got, want)
	}
	nb := f.NetBird
	if nb == nil || nb.Version != "0.81.3" || !nb.Connected || nb.Port != 51820 || !nb.Kernel || nb.Iface != "wt0" ||
		nb.Overlay.String() != "10.31.0.1/16" || fmt.Sprint(nb.Routed) != "[10.32.0.0/24]" {
		t.Fatalf("netbird %+v", nb)
	}
	if fmt.Sprint(f.Candidates()) != "[10.32.0.0/24]" {
		t.Fatalf("candidates %v", f.Candidates())
	}
	var wg bool
	for _, l := range f.Listeners {
		wg = wg || (l.Proto == "udp" && l.Port == 51820)
	}
	if !wg || len(f.Listeners) < 10 {
		t.Fatalf("listeners %+v", f.Listeners)
	}
	if f.FilterTable() || f.HashiTable() {
		t.Fatal("no nft tables on this host")
	}
}

func TestParseProbeDebianLXC(t *testing.T) {
	f, err := ParseProbe(fixture(t, "probe_debian13_lxc.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if f.NetBird != nil || !f.NftablesUnitEnabled || !f.FilterTable() || f.WAN != "eth0" || f.IptablesBackend == "legacy" {
		t.Fatalf("%+v", f)
	}
}

func TestParseProbeWithoutNft(t *testing.T) {
	out := strings.Replace(fixture(t, "probe_debian13_lxc.txt"), "nftables v1.1.3 (Commodore Bullmoose #4)", "sh: 1: nft: not found", 1)
	out = strings.Replace(out, "### nft --version\nsh: 1: nft: not found\n### rc=0", "### nft --version\nsh: 1: nft: not found\n### rc=127", 1)
	if _, err := ParseProbe(out); !errors.Is(err, ErrNoNft) {
		t.Fatalf("err %v", err)
	}
}
