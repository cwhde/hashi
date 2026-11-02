//go:build integration

package traefik_test

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/itest"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/traefik"
)

// runFromState starts Traefik in its service's working directory, where the local plugin lives (06 §9).
const runFromState = "#!/bin/sh\npkill -f '^/usr/local/bin/traefik --configFile' ; sleep 1\n" +
	"cd " + traefik.StateDir + " && setsid /usr/local/bin/traefik --configFile=/etc/hashi/traefik/traefik.yaml </dev/null >/var/log/traefik.out 2>&1 &\n"

// The WAF through Hashi's own plan and apply on the real Traefik with the real plugin and CRS (06 §9, acceptance 2):
// the plugin is installed as local files (the edge downloads nothing), a plain request passes, SQL injection in the
// query and a refused method or content type get 403, a path marked uploads lets a 3 MB PUT and WebDAV through, a
// path with the WAF off skips it, and the tail reads exactly one block per refused request from the general log.
func TestIntegrationWAF(t *testing.T) {
	traefik.VerifyWait = 30 * time.Second
	priv, line, err := sshx.GenerateKey("hashi@itest")
	if err != nil {
		t.Fatal(err)
	}
	script := "apk add -q openssh-server openssh-sftp-server busybox-extras >/dev/null && ssh-keygen -A >/dev/null && " +
		"mkdir -p /root/.ssh /www/api/edge-auth /www/up && printf '%s\\n' \"$AUTHORIZED\" > /root/.ssh/authorized_keys && " +
		"chmod 700 /root/.ssh && chmod 600 /root/.ssh/authorized_keys && echo ok > /www/readyz && echo ok > /www/api/edge-auth/forward && " +
		"echo hello > /www/index.html && echo raw > /www/raw && " +
		"httpd -p 127.0.0.1:" + strconv.Itoa(stubPort) + " -h /www && " +
		"exec /usr/sbin/sshd -D -e -p " + strconv.Itoa(sshPort) + " -o PermitRootLogin=prohibit-password"
	c := itest.RunPorts(t, []int{sshPort, traefik.TLSPort}, []string{"-e", "AUTHORIZED=" + line, "--entrypoint", "sh"}, traefikImage, "-c", script)
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

	const name = "files.example.test"
	crt, key := selfSigned(t, name)
	svc := services.Service{Name: "Files", Slug: "files", Kind: services.KindHTTP, HostName: name, Enabled: true,
		Origin: services.OriginUser, Target: services.Target{Address: "127.0.0.1", Port: stubPort},
		WAF: services.WAF{Paths: []services.WAFPath{
			{ID: "01JW0000000000000000000001", Match: services.MatchPrefix, Path: "/up", Uploads: true},
			{ID: "01JW0000000000000000000002", Match: services.MatchExact, Path: "/raw", Mode: settings.WAFDisabled},
		}}}
	opts := traefik.Options{
		Paths:   traefik.Paths{Binary: traefik.DefaultBinary, ConfigDir: traefik.DefaultConfigDir},
		APIPort: apiPort, ForwardAuthPort: faPort, HashiURL: "http://127.0.0.1:" + strconv.Itoa(stubPort),
		Certs:    []traefik.Cert{{Name: name, CertPEM: crt, KeyPEM: secrets.New(key)}},
		Services: []services.Service{svc}, WAFDefault: settings.WAFHeadersOnly,
	}
	files, err := traefik.Render(opts)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	d := traefik.Desired{
		Paths: opts.Paths, APIPort: apiPort, Files: files, PingTimeout: 20 * time.Second, PackageManager: "apk", Init: traefik.InitOpenRC,
		Service: traefik.Service{Path: "/etc/hashi-traefik-run", Data: []byte(runFromState), Mode: 0o755,
			Enable: "/etc/hashi-traefik-run", Restart: "/etc/hashi-traefik-run"},
		Binary: func(context.Context) ([]byte, error) {
			return nil, errors.New("the image already has the pinned binary")
		},
		Plugin: func(ctx context.Context) (map[string][]byte, error) { return traefik.DownloadPlugin(ctx, client, "") },
	}
	diff, err := traefik.Observe(ctx, h, d)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := traefik.Apply(ctx, h, d, diff, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := traefik.Verify(ctx, h, apiPort, files); err != nil {
		t.Log(c.Exec(t, "sh", "-c", "tail -n 20 /var/log/hashi/traefik/traefik.log"))
		t.Fatal(err)
	}
	if again, err := traefik.Observe(ctx, h, d); err != nil || !again.Empty() {
		t.Fatalf("second observe: %+v %v", again.Items(), err)
	}
	if out := c.Exec(t, "sh", "-c", "ls -d /plugins-storage "+traefik.StateDir+"/plugins-storage 2>/dev/null; echo done"); strings.Contains(out, "plugins-storage") {
		t.Errorf("Traefik downloaded the plugin itself: %s", out)
	}

	addr := c.Port(t, traefik.TLSPort)
	web := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{ServerName: name, InsecureSkipVerify: true}}} //nolint:gosec // self-signed test certificate
	do := func(method, path, contentType string, body []byte) int {
		t.Helper()
		req, err := http.NewRequestWithContext(ctx, method, "https://"+addr+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Host = name
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		resp, err := web.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	itest.WaitFor(t, 20*time.Second, "service up", func() bool { return do(http.MethodGet, "/", "", nil) == 200 })
	_, cur, _, err := traefik.Follow(ctx, h, traefik.LogDir, traefik.GeneralLogFile, traefik.Cursor{})
	if err != nil {
		t.Fatal(err)
	}

	big := bytes.Repeat([]byte("x"), 3<<20)
	refused := 0
	for _, c := range []struct {
		method, path, ct string
		body             []byte
		blocked          bool
	}{
		{http.MethodGet, "/?id=1%27%20OR%201=1--", "", nil, true},                // SQL injection in the query
		{http.MethodPut, "/file", "application/octet-stream", []byte("x"), true}, // content type refused (920420)
		{"PROPFIND", "/", "", nil, true},                                         // method refused (911100)
		{http.MethodPut, "/up/file", "application/octet-stream", big, false},     // uploads path
		{"PROPFIND", "/up/", "", nil, false},
		{http.MethodGet, "/raw?id=1%27%20OR%201=1--", "", nil, false}, // WAF off on this path
		{http.MethodGet, "/index.html", "", nil, false},
	} {
		got := do(c.method, c.path, c.ct, c.body)
		if (got == http.StatusForbidden) != c.blocked {
			t.Errorf("%s %s (%d bytes): %d, blocked expected %v", c.method, c.path, len(c.body), got, c.blocked)
		}
		if c.blocked {
			refused++
		}
		t.Logf("%s %s: %d", c.method, c.path, got)
	}

	var blocks []traefik.WAFBlock
	itest.WaitFor(t, 10*time.Second, "WAF blocks in the general log", func() bool {
		b, next, _, err := traefik.Follow(ctx, h, traefik.LogDir, traefik.GeneralLogFile, cur)
		if err != nil {
			t.Fatal(err)
		}
		cur = next
		blocks = append(blocks, traefik.ParseWAFBlocks(b)...)
		return len(blocks) >= refused
	})
	time.Sleep(time.Second)
	b, _, _, _ := traefik.Follow(ctx, h, traefik.LogDir, traefik.GeneralLogFile, cur)
	blocks = append(blocks, traefik.ParseWAFBlocks(b)...)
	if len(blocks) != refused {
		t.Fatalf("%d blocks for %d refused requests: %+v", len(blocks), refused, blocks)
	}
	for _, bl := range blocks {
		if slug, ok := traefik.RouterService(bl.Router, []string{"files"}); !ok || slug != "files" || !bl.Client.IsValid() {
			t.Errorf("block %+v", bl)
		}
	}
}
