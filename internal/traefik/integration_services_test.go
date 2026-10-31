//go:build integration

package traefik_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/itest"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/traefik"
)

const edgeToken = "itest-edge-token"

// buildUpstream cross-compiles the stub upstream (testdata/upstream) for the Docker host's architecture.
func buildUpstream(t *testing.T) []byte {
	t.Helper()
	out := filepath.Join(t.TempDir(), "upstream")
	cmd := exec.Command("go", "build", "-o", out, "./testdata/upstream")
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH="+runtime.GOARCH, "CGO_ENABLED=0")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build upstream: %v\n%s", err, b)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestIntegrationServices is 04 §9's service integration test: the pinned Traefik loads a rendered services file with
// every HTTP kind and rewrite mode, its API reports every object enabled, and requests through it reach stub HTTP,
// H2C and HTTPS upstreams with the rewritten path; TCP (with and without PROXY protocol) and UDP streams reach their
// echo upstreams through their own entry points. Requests to Hashi and forward-auth calls carry the edge token
// (a client cannot forge it); other upstreams never see it.
func TestIntegrationServices(t *testing.T) {
	traefik.VerifyWait = 20 * time.Second
	bin := buildUpstream(t)
	priv, line, err := sshx.GenerateKey("hashi@itest")
	if err != nil {
		t.Fatal(err)
	}
	script := "apk add -q openssh-server openssh-sftp-server >/dev/null && ssh-keygen -A >/dev/null && " +
		"mkdir -p /root/.ssh && printf '%s\\n' \"$AUTHORIZED\" > /root/.ssh/authorized_keys && " +
		"chmod 700 /root/.ssh && chmod 600 /root/.ssh/authorized_keys && " +
		"exec /usr/sbin/sshd -D -e -p " + strconv.Itoa(sshPort) + " -o PermitRootLogin=prohibit-password"
	c := itest.RunPorts(t, []int{sshPort}, []string{"-e", "AUTHORIZED=" + line, "--entrypoint", "sh"}, traefikImage, "-c", script)
	itest.WaitFor(t, 120*time.Second, "sshd", func() bool { return itest.SSHBanner(c.Addr) })

	ctx := context.Background()
	hk, err := sshx.Scan(ctx, c.Addr, 10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	h, err := sshx.Dial(ctx, sshx.Config{Addr: c.Addr, Pinned: hk, Auth: sshx.Auth{User: "root", PrivateKey: priv}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = h.Close() }()
	if _, err := h.WriteFileAtomic("/usr/local/bin/upstream", bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Run(ctx, "setsid /usr/local/bin/upstream </dev/null >/var/log/upstream.out 2>&1 &", nil); err != nil {
		t.Fatal(err)
	}

	target := func(port int) services.Target { return services.Target{Address: "127.0.0.1", Port: port} }
	route := func(id string, prio int, match, value, mode, pattern, repl string) services.Route {
		return services.Route{ID: id, Enabled: true, Targets: []services.Target{target(9001)}, PathMatch: match, PathValue: value,
			Priority: prio, RewriteMode: mode, RewritePattern: pattern, RewriteValue: repl}
	}
	crt, key := selfSigned(t, "example.test")
	opts := traefik.Options{
		Paths:   traefik.Paths{Binary: traefik.DefaultBinary, ConfigDir: traefik.DefaultConfigDir},
		APIPort: apiPort, ForwardAuthPort: faPort, HashiURL: "http://127.0.0.1:9000",
		Certs:     []traefik.Cert{{Name: "example.test", CertPEM: crt, KeyPEM: secrets.New(key)}},
		EdgeToken: secrets.New(edgeToken),
		Services: []services.Service{
			{Name: "Hashi", Slug: "hashi-admin", Kind: services.KindHTTP, HostName: "dev.hashi.example.test", Enabled: true,
				Origin: services.OriginSystem, OwnerFeature: services.OwnerSetup, Target: target(9000)},
			{Name: "Search", Slug: "search", Kind: services.KindHTTP, HostName: "search.example.test", Enabled: true,
				Origin: services.OriginUser, Target: target(9001), AdvancedRoutes: true, Routes: []services.Route{
					route("01JR0000000000000000000001", 40, services.MatchPrefix, "/api", services.RewriteStripPrefix, "", ""),
					route("01JR0000000000000000000002", 30, services.MatchPrefix, "/old", services.RewriteReplacePrefix, "", "/new"),
					route("01JR0000000000000000000003", 20, services.MatchExact, "/health", services.RewriteExact, "", "/status"),
					route("01JR0000000000000000000004", 10, services.MatchRegex, `^/u/[0-9]+`, services.RewriteRegex,
						`^/u/([0-9]+)`, "/user/$1"),
				}},
			{Name: "Grpc", Slug: "grpc", Kind: services.KindH2C, HostName: "grpc.example.test", Enabled: true,
				Origin: services.OriginUser, Target: target(9002), TLSMinVersion: services.TLS12},
			{Name: "Secure", Slug: "secure", Kind: services.KindHTTPS, HostName: "secure.example.test", Enabled: true,
				Origin: services.OriginUser, Target: target(9003), TLSSkipVerify: true},
			{Name: "Off", Slug: "off", Kind: services.KindHTTP, HostName: "off.example.test", Enabled: false,
				Origin: services.OriginUser, Target: target(9001)},
			// Streams (04 §1.2): one entry point per public port, PROXY protocol v2 on one TCP service.
			{Name: "Db", Slug: "db", Kind: services.KindTCP, HostName: "db.example.test", Enabled: true, Origin: services.OriginUser,
				Target: target(9010), PublicPort: 15432, ProxyProtocol: true},
			{Name: "Plain", Slug: "plain", Kind: services.KindTCP, HostName: "plain.example.test", Enabled: true,
				Origin: services.OriginUser, Target: target(9010), PublicPort: 15434},
			{Name: "Game", Slug: "game", Kind: services.KindUDP, HostName: "game.example.test", Enabled: true,
				Origin: services.OriginUser, Target: target(9012), PublicPort: 15433},
		},
	}
	files, err := traefik.Render(opts)
	if err != nil {
		t.Fatal(err)
	}
	d := traefik.Desired{
		Paths: opts.Paths, APIPort: apiPort, Files: files, PingTimeout: 15 * time.Second, PackageManager: "apk", Init: traefik.InitOpenRC,
		Service: traefik.Service{Path: "/etc/hashi-traefik-run", Data: []byte(runScript), Mode: 0o755,
			Enable: "/etc/hashi-traefik-run", Restart: "/etc/hashi-traefik-run"},
		Binary: func(context.Context) ([]byte, error) {
			return nil, errors.New("the image already has the pinned binary")
		},
	}
	diff, err := traefik.Observe(ctx, h, d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := traefik.Apply(ctx, h, d, diff, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := traefik.Verify(ctx, h, apiPort, files); err != nil {
		t.Fatal(err)
	}
	if again, err := traefik.Observe(ctx, h, d); err != nil || !again.Empty() {
		t.Fatalf("second observe: %+v %v", again.Items(), err)
	}

	// A dynamic-only change (no restart): Verify right after the write must wait until Traefik's file watcher loaded it
	// (S5 gate: it read the API before the reload and reported the new router missing).
	late := opts
	late.Services = append(append([]services.Service(nil), opts.Services...), services.Service{Name: "Late", Slug: "late",
		Kind: services.KindHTTP, HostName: "late.example.test", Enabled: true, Origin: services.OriginUser, Target: target(9001)})
	lateFiles, err := traefik.Render(late)
	if err != nil {
		t.Fatal(err)
	}
	d.Files = lateFiles
	diff, err = traefik.Observe(ctx, h, d)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := traefik.Apply(ctx, h, d, diff, time.Now()); err != nil || out.Restarted {
		t.Fatalf("dynamic change: restarted %v err %v", out.Restarted, err)
	}
	if err := traefik.Verify(ctx, h, apiPort, lateFiles); err != nil {
		t.Fatalf("verify right after a dynamic change: %v", err)
	}

	get := func(host, path string, headers ...string) string {
		cmd := "/usr/local/bin/upstream get " + host + " '" + path + "'"
		for _, hd := range headers {
			cmd += " '" + hd + "'"
		}
		r, err := h.Run(ctx, cmd, nil)
		if err != nil {
			t.Fatalf("%s%s: %v", host, path, err)
		}
		return strings.TrimSpace(string(r.Stdout) + string(r.Stderr))
	}
	itest.WaitFor(t, 20*time.Second, "services routed", func() bool {
		return strings.HasPrefix(get("search.example.test", "/"), "status=200")
	})
	cases := []struct{ host, path, want string }{
		{"dev.hashi.example.test", "/setup", "port=9000 proto=HTTP/1.1 path=/setup edge=" + edgeToken},
		{"search.example.test", "/", "port=9001 proto=HTTP/1.1 path=/ edge="},
		{"search.example.test", "/api/v1/q", "port=9001 proto=HTTP/1.1 path=/v1/q edge="},
		{"search.example.test", "/old/a/b", "port=9001 proto=HTTP/1.1 path=/new/a/b edge="},
		{"search.example.test", "/health", "port=9001 proto=HTTP/1.1 path=/status edge="},
		{"search.example.test", "/u/42", "port=9001 proto=HTTP/1.1 path=/user/42 edge="},
		{"grpc.example.test", "/x", "port=9002 proto=HTTP/2.0 path=/x edge="},
		{"secure.example.test", "/s", "port=9003"},
	}
	for _, tc := range cases {
		if got := get(tc.host, tc.path); !strings.HasPrefix(got, "status=200 ") || !strings.Contains(got, tc.want) {
			t.Errorf("%s%s: got %q, want %q", tc.host, tc.path, got, tc.want)
		}
	}
	if got := get("secure.example.test", "/s"); !strings.Contains(got, "path=/s edge=") {
		t.Errorf("https upstream: %q", got)
	}
	stream := func(network, port, msg string) string {
		r, err := h.Run(ctx, "/usr/local/bin/upstream "+network+" "+port+" "+msg, nil)
		if err != nil {
			t.Fatalf("%s %s: %v", network, port, err)
		}
		return strings.TrimSpace(string(r.Stdout) + string(r.Stderr))
	}
	for _, tc := range []struct{ network, port, want string }{
		{"tcp", "15432", "tcp port=9010 proxy=v2 src=127.0.0.1 got=hello"},
		{"tcp", "15434", "tcp port=9010 proxy=none got=hello"},
		{"udp", "15433", "udp got=hello"},
	} {
		if got := stream(tc.network, tc.port, "hello"); got != tc.want {
			t.Errorf("%s %s through Traefik: got %q, want %q", tc.network, tc.port, got, tc.want)
		}
	}
	if got := get("off.example.test", "/"); !strings.HasPrefix(got, "status=404") {
		t.Errorf("disabled service answered: %q", got)
	}
	// A client cannot forge the token toward Hashi: the edge overwrites it.
	if got := get("dev.hashi.example.test", "/", "X-Hashi-Edge=forged"); !strings.Contains(got, "edge="+edgeToken+"\n") &&
		!strings.HasSuffix(got, "edge="+edgeToken) {
		t.Errorf("forged edge header: %q", got)
	}
	// Forward auth for a user service reached "Hashi" with the token, while the service itself saw none.
	get("search.example.test", "/fa-check", "X-Hashi-Edge=forged")
	r, err := h.Run(ctx, "wget -qO- -T 5 http://127.0.0.1:9000/last-fa", nil)
	if err != nil || strings.TrimSpace(string(r.Stdout)) != "fa-edge="+edgeToken {
		t.Errorf("forward auth edge header: %q %v", r.Stdout, err)
	}
}
