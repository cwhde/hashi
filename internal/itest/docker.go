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

// Run starts image with the given docker run flags and command, waits until port accepts TCP, and removes the
// container when the test ends.
func Run(t testing.TB, port int, flags []string, image string, cmd ...string) Container {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not available")
	}
	args := []string{"run", "-d", "--rm"}
	if inContainer() {
		self, err := os.Hostname()
		if err != nil {
			t.Fatal(err)
		}
		args = append(args, "--network", "container:"+self)
	} else {
		args = append(args, "-p", "127.0.0.1::"+strconv.Itoa(port))
	}
	args = append(args, flags...)
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
	c := Container{ID: id, Addr: net.JoinHostPort("127.0.0.1", strconv.Itoa(port))}
	if !inContainer() {
		out, err := docker("port", id, strconv.Itoa(port)).Output()
		if err != nil {
			t.Fatalf("docker port: %v", err)
		}
		c.Addr = strings.TrimSpace(strings.Split(string(out), "\n")[0])
	}
	return c
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
