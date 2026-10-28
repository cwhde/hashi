//go:build integration

package traefik_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/itest"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/traefik"
)

// The official Traefik image is Alpine-based and carries the pinned binary at the default path, so the install step
// finds it and the test exercises files, restart, Verify and the forward-auth fallback against the real binary.
const (
	traefikImage = "traefik:" + traefik.Version
	sshPort      = 22024
	apiPort      = 28180
	faPort       = 28181
	stubPort     = 28190 // stands in for Hashi: /readyz and /api/edge-auth/forward answer 200
)

// runScript replaces the init system (the image has none): it (re)starts Traefik detached from the SSH session.
// Real OpenRC and systemd are exercised on the dev targets (S4 gate).
const runScript = "#!/bin/sh\npkill -f '^/usr/local/bin/traefik --configFile' ; sleep 1\n" +
	"setsid /usr/local/bin/traefik --configFile=/etc/hashi/traefik/traefik.yaml </dev/null >/var/log/traefik.out 2>&1 &\n"

func selfSigned(t *testing.T, name string) (certPEM []byte, keyPEM string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: name}, DNSNames: []string{name, "*." + name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb}))
}

// TestIntegrationApplyVerifyFallback is 04 §9's integration test: the pinned Traefik loads Hashi's files over SSH, its
// API reports no errors, nothing is rewritten on the second run, fail-open/closed behave as captured with "Hashi"
// stopped, and a static change that keeps Traefik from starting is restored.
func TestIntegrationApplyVerifyFallback(t *testing.T) {
	priv, line, err := sshx.GenerateKey("hashi@itest")
	if err != nil {
		t.Fatal(err)
	}
	script := "apk add -q openssh-server openssh-sftp-server busybox-extras >/dev/null && ssh-keygen -A >/dev/null && " +
		"mkdir -p /root/.ssh /www/api/edge-auth && printf '%s\\n' \"$AUTHORIZED\" > /root/.ssh/authorized_keys && " +
		"chmod 700 /root/.ssh && chmod 600 /root/.ssh/authorized_keys && echo ok > /www/readyz && echo ok > /www/api/edge-auth/forward && " +
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

	crt, key := selfSigned(t, "example.test")
	opts := traefik.Options{
		Paths:   traefik.Paths{Binary: traefik.DefaultBinary, ConfigDir: traefik.DefaultConfigDir},
		APIPort: apiPort, ForwardAuthPort: faPort, HashiURL: "http://127.0.0.1:" + strconv.Itoa(stubPort),
		Certs: []traefik.Cert{{Name: "example.test", CertPEM: crt, KeyPEM: secrets.New(key)}},
	}
	files, err := traefik.Render(opts)
	if err != nil {
		t.Fatal(err)
	}
	d := traefik.Desired{
		Paths: opts.Paths, APIPort: apiPort, Files: files, PingTimeout: 15 * time.Second,
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
	if diff.InstallBinary || diff.InstalledVersion != traefik.Version {
		t.Fatalf("binary: install=%v version=%q", diff.InstallBinary, diff.InstalledVersion)
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

	// 03 §5.3 Verify: the edge serves exactly the distributed certificate, and a renewed one after the next apply
	// (only the certificate files and hashi-certs.yaml change; Traefik reloads them without a restart).
	tlsAddr := c.Port(t, traefik.TLSPort)
	dial := func(ctx context.Context) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", tlsAddr)
	}
	for _, chk := range traefik.VerifyServed(ctx, dial, opts.Certs, 20*time.Second) {
		if chk.Err != nil {
			t.Fatalf("served certificate: %v", chk.Err)
		}
	}
	crt2, key2 := selfSigned(t, "example.test")
	opts.Certs = []traefik.Cert{{Name: "example.test", CertPEM: crt2, KeyPEM: secrets.New(key2)}}
	if files, err = traefik.Render(opts); err != nil {
		t.Fatal(err)
	}
	d.Files = files
	diff, err = traefik.Observe(ctx, h, d)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := traefik.Apply(ctx, h, d, diff, time.Now()); err != nil || out.Restarted || len(out.Written) != 3 {
		t.Fatalf("renewed certificate: written %d restarted %v err %v", len(out.Written), out.Restarted, err)
	}
	for _, chk := range traefik.VerifyServed(ctx, dial, opts.Certs, 20*time.Second) {
		if chk.Err != nil {
			t.Fatalf("renewed certificate not served: %v", chk.Err)
		}
		t.Logf("served %s: %.16s…", chk.Name, chk.Got)
	}

	status := func(path string) string {
		r, err := h.Run(ctx, "wget -qSO- --no-check-certificate -T 5 https://127.0.0.1"+path+" 2>&1 | grep -m1 'HTTP/'", nil)
		if err != nil {
			t.Fatal(err)
		}
		return strings.TrimSpace(string(r.Stdout))
	}
	open, closed := traefik.HealthPath+"/open", traefik.HealthPath+"/closed"
	itest.WaitFor(t, 20*time.Second, "health routes 200 with Hashi up", func() bool {
		return strings.Contains(status(open), "200") && strings.Contains(status(closed), "200")
	})

	// "Hashi" stops: fail-open falls back to ping@internal once the health check notices, fail-closed answers 500.
	c.Exec(t, "pkill", "httpd")
	itest.WaitFor(t, 30*time.Second, "fallback with Hashi down", func() bool {
		return strings.Contains(status(open), "200") && strings.Contains(status(closed), "500")
	})
	t.Logf("Hashi down: open %q, closed %q", status(open), status(closed))

	// A static change Traefik cannot start with (the API port is taken) is put back.
	c.Exec(t, "httpd", "-p", "127.0.0.1:28280", "-h", "/www")
	opts.APIPort = 28280
	bad, err := traefik.Render(opts)
	if err != nil {
		t.Fatal(err)
	}
	d2 := d
	d2.Files, d2.APIPort = bad, 28280
	diff, err = traefik.Observe(ctx, h, d2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := traefik.Apply(ctx, h, d2, diff, time.Now()); !errors.Is(err, traefik.ErrRestored) {
		t.Fatalf("broken static change: err = %v, want ErrRestored", err)
	}
	if err := traefik.Verify(ctx, h, apiPort, files); err != nil {
		t.Fatalf("after restore: %v", err)
	}
}
