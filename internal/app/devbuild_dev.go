//go:build dev

package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"git.juzo.io/juzo/hashi/internal/certs"
	"git.juzo.io/juzo/hashi/internal/certs/certstest"
	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/dns/hetzner"
	"git.juzo.io/juzo/hashi/internal/dns/hetzner/hetznertest"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/traefik"
	"git.juzo.io/juzo/hashi/internal/traefik/traefiktest"
)

// devBuild enables what local development and the E2E suite need and production must never have:
//   - HASHI_DEV_WEBAUTHN_ORIGIN (e.g. http://localhost:5173) binds passkeys to RP ID "localhost" at that origin,
//     because Hashi has no HTTPS domain before S5 (02-setup-auth.md §1);
//   - setup steps from later slices count as done, so the wizard can complete;
//   - HASHI_DEV_DNS_FIXTURES points the Hetzner adapter at a fake that starts from the recorded zone (devDNSProviders)
//     and replaces the CA and Traefik hosts with in-process fakes (devEdge).
const devBuild = true

// devDNSProviders runs the stateful Hetzner fake (hetznertest.Sim) on the recorded zone in HASHI_DEV_DNS_FIXTURES
// in-process (token hetznertest.Token), so setup step 3, /dns and DNS plans work in make dev and E2E without a real
// provider. Writes live until the process ends.
func devDNSProviders(cfg Config) (dns.Factory, error) {
	if cfg.DevDNSFixtures == "" {
		return nil, nil
	}
	h, err := hetznertest.NewSim(cfg.DevDNSFixtures)
	if err != nil {
		return nil, fmt.Errorf("HASHI_DEV_DNS_FIXTURES: %w", err)
	}
	client := &http.Client{Transport: handlerTransport{h}}
	return func(typ string, token secrets.Value) (dns.Provider, error) {
		if typ != "hetzner" {
			return nil, fmt.Errorf("no fixtures for DNS provider type %q", typ)
		}
		return hetzner.New(token, hetzner.WithBaseURL("http://hetzner.fixtures/v1"), hetzner.WithHTTPClient(client)), nil
	}, nil
}

// handlerTransport answers requests with an in-process handler.
type handlerTransport struct{ h http.Handler }

func (t handlerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	t.h.ServeHTTP(rec, r)
	return rec.Result(), nil
}

// devEdge replaces the CA and the Traefik host with in-process fakes when HASHI_DEV_DNS_FIXTURES is set (make dev,
// E2E): certstest signs certificates after publishing the DNS-01 values through the fake zone, and traefiktest is
// one in-memory Debian 13 host that every SSH address reaches.
func devEdge(cfg Config, tm *traefik.Manager, cm *certs.Manager) error {
	if cfg.DevDNSFixtures == "" {
		return nil
	}
	probe, err := os.ReadFile(filepath.Join(cfg.DevDNSFixtures, "..", "os", "os_debian13.txt"))
	if err != nil {
		return fmt.Errorf("dev edge probe fixture: %w", err)
	}
	edge, err := traefiktest.NewEdge(string(probe))
	if err != nil {
		return err
	}
	tm.UseFakes(edge.Connect, edge.Binary, edge.Scan, edge.DialTLS)
	ca, err := certstest.NewCA()
	if err != nil {
		return err
	}
	cm.UseClients(ca.Clients)
	return nil
}

// DevEdgeToken is the edge token dev builds with HASHI_DEV_DNS_FIXTURES trust instead of the vault's, so the E2E suite
// can send the setup step 8 proof as Hashi's Traefik would (there is no real Traefik in front of the dev server).
const DevEdgeToken = "dev-edge-token"

func devEdgeToken(cfg Config) string {
	if cfg.DevDNSFixtures == "" {
		return ""
	}
	return DevEdgeToken
}
