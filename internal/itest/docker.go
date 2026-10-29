// Package itest starts real software (sshd, Traefik, Pebble) in Docker containers for `make test-integration`
// (11-delivery.md §4). It uses the docker CLI only.
//
// On the laptop a container publishes its port on 127.0.0.1. On the CI runner the job itself runs in a container with
// the host's Docker socket, so a published port would be on the wrong host; there the test container joins the job
// container's network namespace instead and is reached on 127.0.0.1 directly. Tests therefore give every container a
// port no other test container uses.
package itest

import (
	"context"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Container is a running test container.
type Container struct {
	ID   string
	Addr string // host:port reachable from the test process
}

// Exec runs a command inside the container and returns combined output.
func (c Container) Exec(t testing.TB, cmd ...string) string {
	t.Helper()
	out, err := docker(append([]string{"exec", c.ID}, cmd...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker exec %v: %v\n%s", cmd, err, out)
	}
	return string(out)
}

// docker builds a docker CLI command; every argument is chosen by the test or returned by docker itself.
func docker(args ...string) *exec.Cmd {
	return exec.CommandContext(context.Background(), "docker", args...) //nolint:gosec // see above
}

func inContainer() bool {
	if v := os.Getenv("HASHI_ITEST_SHARE_NETNS"); v != "" {
		return v == "1"
	}
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

// Run starts image with the given docker run flags and command, publishes port, and removes the container when the
// test ends.
func Run(t testing.TB, port int, flags []string, image string, cmd ...string) Container {
	t.Helper()
	return RunPorts(t, []int{port}, flags, image, cmd...)
}

// RunPorts is Run with more published TCP ports; ports[0] becomes Addr, the others are reached through Port.
func RunPorts(t testing.TB, ports []int, flags []string, image string, cmd ...string) Container {
	t.Helper()
	args := []string{"run", "-d", "--rm"}
	if inContainer() {
		self, err := os.Hostname()
		if err != nil {
			t.Fatal(err)
		}
		args = append(args, "--network", "container:"+self)
	} else {
		for _, p := range ports {
			args = append(args, "-p", "127.0.0.1::"+strconv.Itoa(p))
		}
	}
	c := start(t, append(args, flags...), image, cmd...)
	c.Addr = c.Port(t, ports[0])
	return c
}

// Join starts image in c's network namespace, so the two reach each other on 127.0.0.1 and c's published ports
// carry both (Pebble and its DNS server).
func Join(t testing.TB, c Container, flags []string, image string, cmd ...string) Container {
	t.Helper()
	netns := c.ID
	if inContainer() {
		self, err := os.Hostname()
		if err != nil {
			t.Fatal(err)
		}
		netns = self
	}
	j := start(t, append([]string{"run", "-d", "--rm", "--network", "container:" + netns}, flags...), image, cmd...)
	j.Addr = c.Addr
	return j
}

func start(t testing.TB, args []string, image string, cmd ...string) Container {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	args = append(args, image)
	args = append(args, cmd...)
	out, err := docker(args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker run %s: %v\n%s", image, err, out)
	}
	id := strings.TrimSpace(string(out))
	t.Cleanup(func() {
		if t.Failed() {
			logs, _ := docker("logs", "--tail", "50", id).CombinedOutput()
			t.Logf("container %s logs:\n%s", image, logs)
		}
		_ = docker("rm", "-f", id).Run()
	})
	return Container{ID: id}
}

// Port is the address the test process reaches the container's TCP port on.
func (c Container) Port(t testing.TB, port int) string {
	t.Helper()
	if inContainer() {
		return net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	}
	out, err := docker("port", c.ID, strconv.Itoa(port)+"/tcp").Output()
	if err != nil {
		t.Fatalf("docker port %d: %v", port, err)
	}
	return strings.TrimSpace(strings.Split(string(out), "\n")[0])
}

// WaitFor polls fn until it returns true or the timeout passes.
func WaitFor(t testing.TB, timeout time.Duration, what string, fn func() bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		if fn() {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("timed out after %s waiting for %s", timeout, what)
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// SSHBanner reports whether addr answers with an SSH identification line.
func SSHBanner(addr string) bool {
	d := net.Dialer{Timeout: time.Second}
	c, err := d.DialContext(context.Background(), "tcp", addr)
	if err != nil {
		return false
	}
	defer func() { _ = c.Close() }()
	_ = c.SetReadDeadline(time.Now().Add(time.Second))
	b := make([]byte, 4)
	n, _ := c.Read(b)
	return n == 4 && string(b) == "SSH-"
}

// Build builds the Dockerfile in dir as tag (layers are cached between runs) and skips the test without docker.
func Build(t testing.TB, tag, dir string) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	if out, err := docker("build", "-q", "-t", tag, dir).CombinedOutput(); err != nil {
		t.Fatalf("docker build %s: %v\n%s", dir, err, out)
	}
}

// CopyTo copies a local file into the container.
func (c Container) CopyTo(t testing.TB, src, dst string) {
	t.Helper()
	if out, err := docker("cp", src, c.ID+":"+dst).CombinedOutput(); err != nil {
		t.Fatalf("docker cp %s: %v\n%s", src, err, out)
	}
}

// Try runs a command inside the container and returns its output and error without failing the test.
func (c Container) Try(cmd ...string) (string, error) {
	out, err := docker(append([]string{"exec", c.ID}, cmd...)...).CombinedOutput()
	return string(out), err
}

// Arch is the Docker server's architecture as GOARCH (amd64, arm64).
func Arch(t testing.TB) string {
	t.Helper()
	out, err := docker("info", "-f", "{{.Architecture}}").Output()
	if err != nil {
		t.Fatalf("docker info: %v", err)
	}
	switch a := strings.TrimSpace(string(out)); a {
	case "x86_64":
		return "amd64"
	case "aarch64":
		return "arm64"
	default:
		return a
	}
}

// RunIsolated starts image in its own network namespace on Docker's default bridge, never the test runner's: firewall
// tests change the namespace's netfilter state.
func RunIsolated(t testing.TB, flags []string, image string, cmd ...string) Container {
	t.Helper()
	return start(t, append([]string{"run", "-d", "--rm", "--network", "bridge"}, flags...), image, cmd...)
}

// Network creates a Docker bridge network with subnet and removes it when the test ends.
func Network(t testing.TB, name, subnet string) {
	t.Helper()
	_ = docker("network", "rm", name).Run()
	if out, err := docker("network", "create", "--subnet", subnet, name).CombinedOutput(); err != nil {
		t.Fatalf("docker network create %s: %v\n%s", name, err, out)
	}
	t.Cleanup(func() { _ = docker("network", "rm", name).Run() })
}

// Connect attaches a running container to a network with a fixed address.
func (c Container) Connect(t testing.TB, network, ip string) {
	t.Helper()
	if out, err := docker("network", "connect", "--ip", ip, network, c.ID).CombinedOutput(); err != nil {
		t.Fatalf("docker network connect %s: %v\n%s", network, err, out)
	}
}

// RunOn starts image on a network created with Network, at a fixed address.
func RunOn(t testing.TB, network, ip string, flags []string, image string, cmd ...string) Container {
	t.Helper()
	return start(t, append([]string{"run", "-d", "--rm", "--network", network, "--ip", ip}, flags...), image, cmd...)
}
