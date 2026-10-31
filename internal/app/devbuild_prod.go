//go:build !dev

package app

import (
	"git.juzo.io/juzo/hashi/internal/certs"
	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/firewall"
	"git.juzo.io/juzo/hashi/internal/traefik"
)

// devBuild is false in every image and release binary. `-tags dev` (make dev, make test-e2e) flips it; see devbuild_dev.go.
const devBuild = false

// devDNSProviders is never set outside dev builds.
func devDNSProviders(Config) (dns.Factory, error) { return nil, nil }

// devEdge is never set outside dev builds.
func devEdge(Config, *traefik.Manager, *certs.Manager) error { return nil }

// devFirewall is never set outside dev builds.
func devFirewall(Config, *firewall.Manager) error { return nil }

// devEdgeToken is never set outside dev builds.
func devEdgeToken(Config) string { return "" }
