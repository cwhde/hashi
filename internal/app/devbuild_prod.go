//go:build !dev

package app

import "git.juzo.io/juzo/hashi/internal/dns"

// devBuild is false in every image and release binary. `-tags dev` (make dev, make test-e2e) flips it; see devbuild_dev.go.
const devBuild = false

// devDNSProviders is never set outside dev builds.
func devDNSProviders(Config) (dns.Factory, error) { return nil, nil }
