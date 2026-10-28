//go:build integration

package certs

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/itest"
	"git.juzo.io/juzo/hashi/internal/secrets"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
)

// Pebble 2.10.1 with EAB required (the image's own test config) validates DNS-01 against pebble-challtestsrv, which
// shares its network namespace. Ports are unique among the integration tests (itest package doc).
const (
	pebbleImage = "ghcr.io/letsencrypt/pebble:2.10.1"
	challImage  = "ghcr.io/letsencrypt/pebble-challtestsrv:2.10.1"
	acmePort    = 14000
	mgmtPort    = 28055
	dnsPort     = 28053
	// Pebble's published test EAB key "kid-1" (test/config/pebble-config-external-account-bindings.json in the image).
	pebbleKID  = "kid-1"
	pebbleHMAC = "zWNDZM6eQGHWpSRTPal5eIUYFTu7EajVIoguysqZ9wG44nMEtx3MUAsUDkMTQ12W" // hashi:allow-secret-pattern
)

// challSolver publishes challenge values in challtestsrv (set-txt appends, clear-txt removes all values of a name,
// probed 2026-10-28), keeping the values of other names and other writers.
type challSolver struct {
	mgmt string
	mu   sync.Mutex
	vals map[string][]string
	log  []string
}

func (s *challSolver) post(ctx context.Context, path string, body map[string]string) error {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+s.mgmt+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	_ = resp.Body.Close()
	return nil
}

func (s *challSolver) Present(ctx context.Context, fqdn, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vals[fqdn] = append(s.vals[fqdn], value)
	s.log = append(s.log, "present "+fqdn)
	return s.post(ctx, "/set-txt", map[string]string{"host": fqdn + ".", "value": value})
}

func (s *challSolver) CleanUp(ctx context.Context, fqdn, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.log = append(s.log, "cleanup "+fqdn)
	s.vals[fqdn] = slices.DeleteFunc(s.vals[fqdn], func(v string) bool { return v == value })
	if err := s.post(ctx, "/clear-txt", map[string]string{"host": fqdn + "."}); err != nil {
		return err
	}
	for _, v := range s.vals[fqdn] {
		if err := s.post(ctx, "/set-txt", map[string]string{"host": fqdn + ".", "value": v}); err != nil {
			return err
		}
	}
	return nil
}

// TestIntegrationIssueAndRenewWithPebble is 03 §5.3 against a real ACME server: the account registers with EAB, the
// certificates plan issues root + wildcard through DNS-01 (values checked on the DNS server before the CA is asked,
// removed afterwards), the job plans nothing while the certificate is valid, and "Renew now" replaces certificate and
// key.
func TestIntegrationIssueAndRenewWithPebble(t *testing.T) {
	chall := itest.RunPorts(t, []int{acmePort, mgmtPort, dnsPort}, nil, challImage,
		"-dnsserver", ":"+itoa(dnsPort), "-management", ":"+itoa(mgmtPort), "-http01", "", "-https01", "", "-tlsalpn01", "", "-doh", "")
	itest.Join(t, chall, []string{"-e", "PEBBLE_VA_NOSLEEP=1", "-e", "PEBBLE_WFE_NONCEREJECT=0"}, pebbleImage,
		"-config", "test/config/pebble-config-external-account-bindings.json", "-dnsserver", "127.0.0.1:"+itoa(dnsPort))
	insecure := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}} //nolint:gosec // Pebble's test CA
	dir := "https://" + chall.Addr + "/dir"
	itest.WaitFor(t, 60*time.Second, "pebble directory", func() bool {
		resp, err := insecure.Get(dir)
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	})

	ctx := context.Background()
	f := newFixture(t)
	solver := &challSolver{mgmt: chall.Port(t, mgmtPort), vals: map[string][]string{}}
	m := NewManager(f.st, f.vault, f.audit, func(context.Context) (Solver, []string, error) { return solver, nil, nil }, insecure, slog.Default())
	var waited []string
	m.wait = func(ctx context.Context, servers []string, fqdn, value string) error {
		waited = append(waited, fqdn+" @ "+strings.Join(servers, ","))
		return WaitTXT(ctx, servers, fqdn, value, 200*time.Millisecond)
	}
	eng := hsync.New(f.st, f.audit, m.Subsystems()...)

	// Setup step 4: the provider is stored and the account registers with EAB.
	a, err := m.Configure(ctx, "setup", "", ProviderInput{Directory: "custom", DirectoryURL: dir, Email: "admin@example.com",
		EABKID: pebbleKID, EABHMAC: secrets.New(pebbleHMAC), Resolvers: []string{chall.Port(t, dnsPort)}})
	if err != nil || a.AccountUri == "" || a.Health != "healthy" {
		t.Fatalf("configure: %+v %v", a, err)
	}
	if _, err := m.Configure(ctx, "setup", "", ProviderInput{Directory: "custom", DirectoryURL: dir, Email: "admin@example.com",
		EABKID: "kid-unknown", EABHMAC: secrets.New(pebbleHMAC)}); err == nil {
		t.Error("an unknown EAB key ID was accepted")
	}
	if a, err = m.Configure(ctx, "setup", "", ProviderInput{Directory: "custom", DirectoryURL: dir, Email: "admin@example.com",
		EABKID: pebbleKID, EABHMAC: secrets.New(pebbleHMAC), Resolvers: []string{chall.Port(t, dnsPort)}}); err != nil || a.Health != "healthy" {
		t.Fatalf("back to the valid key: %+v %v", a, err)
	}

	sum, err := m.Renew(eng, nil)(ctx)
	if err != nil {
		t.Fatalf("renew job: %v", err)
	}
	rows, _ := f.st.R.ListCertificates(ctx)
	if len(rows) != 1 || rows[0].LastRenewalResult != "issued" || rows[0].CertPem == "" || !rows[0].KeySecretID.Valid {
		t.Fatalf("after %q: %+v", sum, rows)
	}
	first := rows[0]
	b, _ := pem.Decode([]byte(first.CertPem))
	leaf, err := x509.ParseCertificate(b.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	if !SameNames(leaf.DNSNames, []string{"example.com", "*.example.com"}) || Fingerprint(b.Bytes) != first.Fingerprint {
		t.Errorf("leaf names %v", leaf.DNSNames)
	}
	if len(waited) != 2 || !strings.HasPrefix(waited[0], "_acme-challenge.example.com @ ") {
		t.Errorf("propagation checks: %v", waited)
	}
	if vals, _ := LookupTXT(ctx, chall.Port(t, dnsPort), "_acme-challenge.example.com"); len(vals) != 0 {
		t.Errorf("challenge values left behind: %v", vals)
	}
	acct, _ := f.st.R.GetACMEAccount(ctx)
	if acct.AccountUri == "" || !acct.AccountKeySecretID.Valid || acct.Health != "healthy" {
		t.Errorf("account: uri %q health %s", acct.AccountUri, acct.Health)
	}
	t.Logf("issued by %q until %s; job: %s", first.Issuer, time.UnixMilli(first.NotAfter.Int64).UTC(), sum)

	if sum, err := m.Renew(eng, nil)(ctx); err != nil || !strings.Contains(sum, "certificates current") {
		t.Errorf("second run: %q %v", sum, err)
	}

	p, err := eng.Auto(ctx, Subsystem, first.ID, hsync.TriggerManual, audit.System)
	if err != nil || p.Status != hsync.StatusApplied {
		t.Fatalf("renew now: %+v %v", p, err)
	}
	renewed, _ := f.st.R.GetCertificate(ctx, first.ID)
	if renewed.Fingerprint == first.Fingerprint || renewed.KeySecretID == first.KeySecretID || renewed.LastRenewalResult != "renewed" {
		t.Errorf("renewed: %+v", renewed)
	}
	if _, err := f.st.R.GetSecret(ctx, first.KeySecretID.String); err == nil {
		t.Error("the replaced key is still in the vault")
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
