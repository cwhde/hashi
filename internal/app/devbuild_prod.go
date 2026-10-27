//go:build !dev

package app

// devBuild is false in every image and release binary. `-tags dev` (make dev, make test-e2e) flips it; see devbuild_dev.go.
const devBuild = false
