//go:build integration

package pulse_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/itest"
	"git.juzo.io/juzo/hashi/internal/pulse"
)

// The installer on a real OpenWrt (bug 21, testdata/contracts/openwrt/): procd runs as PID 1 in the container, there is
// no curl and no install(1), and only files on a keep list survive a firmware upgrade. The agent is served by the
// container's own uhttpd, so the download goes through OpenWrt's wget (uclient-fetch).
func TestRealOpenWrtInstall(t *testing.T) {
	arch := itest.Arch(t)
	image := map[string]string{"arm64": "openwrt/rootfs:aarch64_generic-25.12.6", "amd64": "openwrt/rootfs:x86_64-25.12.6"}[arch]
	if image == "" {
		t.Skipf("no OpenWrt image for %s", arch)
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "hashi-pulse-linux-"+arch)
	build := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w -X main.Version=v0.0.1-test", "-o", bin, "../../agents/pulse")
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH="+arch)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build agent: %v\n%s", err, out)
	}
	data, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	if err := os.WriteFile(bin+".sha256", []byte(hex.EncodeToString(sum[:])+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "install.sh")
	if err := os.WriteFile(script, pulse.InstallScript, 0o600); err != nil {
		t.Fatal(err)
	}

	// The images name their platform after OpenWrt's target (linux/aarch64_generic), so --platform would refuse them.
	c := itest.RunIsolated(t, nil, image, "/sbin/init")
	itest.WaitFor(t, time.Minute, "procd and uhttpd", func() bool {
		out, err := c.Try("ubus", "call", "service", "list", `{"name":"uhttpd"}`)
		return err == nil && strings.Contains(out, `"running": true`)
	})
	c.Exec(t, "mkdir", "-p", "/www/rel")
	c.CopyTo(t, bin, "/www/rel/")
	c.CopyTo(t, bin+".sha256", "/www/rel/")
	c.CopyTo(t, script, "/tmp/install.sh")
	c.Exec(t, "chmod", "-R", "a+rX", "/www/rel") // uhttpd serves only files everyone may read
	for _, tool := range []string{"curl", "install"} {
		if out, err := c.Try("sh", "-c", "command -v "+tool); err == nil {
			t.Fatalf("the image has %s (%s): the test would not prove the fallback", tool, out)
		}
	}

	install := []string{"sh", "/tmp/install.sh", "--url", "http://127.0.0.1:9", "--id", "test-agent", "--token", "test-token", "--from", "http://127.0.0.1/rel"}
	if out := c.Exec(t, install...); !strings.Contains(out, "(procd)") {
		t.Fatalf("installer did not take the procd path:\n%s", out)
	}
	running := func() string {
		out, _ := c.Try("ubus", "call", "service", "list", `{"name":"hashi-pulse"}`)
		return out
	}
	itest.WaitFor(t, 30*time.Second, "hashi-pulse under procd", func() bool {
		out := running()
		return strings.Contains(out, `"running": true`) && strings.Contains(out, "/usr/local/bin/hashi-pulse")
	})
	if out := c.Exec(t, "/usr/local/bin/hashi-pulse", "version"); strings.TrimSpace(out) != "v0.0.1-test" {
		t.Fatalf("installed version: %q", out)
	}
	if out := c.Exec(t, "ls", "-l", "/etc/hashi-pulse/config.yaml"); !strings.HasPrefix(out, "-rw-------") {
		t.Fatalf("config mode: %s", out)
	}
	if _, err := c.Try("test", "-L", "/etc/rc.d/S99hashi-pulse"); err != nil {
		t.Fatal("the service is not enabled at boot")
	}

	// sysupgrade -l prints what a firmware upgrade keeps.
	kept := c.Exec(t, "sysupgrade", "-l")
	for _, f := range []string{"/usr/local/bin/hashi-pulse", "/etc/hashi-pulse/config.yaml", "/etc/init.d/hashi-pulse", "/etc/rc.d/S99hashi-pulse", "/lib/upgrade/keep.d/hashi-pulse"} {
		if !strings.Contains(kept, f+"\n") {
			t.Errorf("a firmware upgrade would drop %s; sysupgrade -l:\n%s", f, kept)
		}
	}

	// Installing again over the running agent replaces its file and restarts it.
	if out := c.Exec(t, install...); !strings.Contains(out, "(procd)") {
		t.Fatalf("second install:\n%s", out)
	}
	itest.WaitFor(t, 30*time.Second, "hashi-pulse running after reinstall", func() bool {
		return strings.Contains(running(), `"running": true`)
	})
}
