# TASK-09: CI/CD Optimization

## Goal

Make Hashi, Hashi Pulse, and legacy image builds faster without weakening required checks or dropping linux/arm64 support.

## Spec Context

- Original spec sections: 28, 29, 30, 31.
- Addendum section: 17 and 18.4, 19 Phase G.
- Existing operations note: `docs/operations/ci-secrets.md` documents Gitea runner cache caveats.
- Research references: `RESEARCH-RESOURCES.md` Gitea and Docker CI/CD section.

## Current Code Anchors

- Main Dockerfile: `deploy/docker/Dockerfile`
- Pulse Dockerfile: `agents/pulse/Dockerfile`
- Legacy Dockerfile: `hashi.old/docker/Dockerfile`
- CI workflow: `.gitea/workflows/ci.yml`
- Security workflow: `.gitea/workflows/security.yml`
- Main build workflow: `.gitea/workflows/docker-build.yml`
- Pulse build workflow: `.gitea/workflows/docker-build-pulse.yml`
- Legacy build workflow: `.gitea/workflows/docker-build-old.yml`

## Main Dockerfile

Make it cross-build aware:

- Use BuildKit syntax with cache mounts.
- `FROM --platform=$BUILDPLATFORM node:... AS web-build`
- `FROM --platform=$BUILDPLATFORM mcr.microsoft.com/dotnet/sdk:... AS dotnet-build`
- Add `ARG TARGETARCH`.
- Map Docker arch to .NET runtime arch when needed.
- Cache pnpm store.
- Cache NuGet packages.
- Avoid Node/.NET SDK stages running under ARM64 emulation on AMD64 runners.
- Keep final aspnet image platform implicit.
- Preserve healthcheck and exposed ports.

## Pulse Dockerfile

Make it cross-build aware:

- `FROM --platform=$BUILDPLATFORM golang:... AS build`
- `ARG TARGETOS`
- `ARG TARGETARCH`
- `GOOS=$TARGETOS GOARCH=$TARGETARCH CGO_ENABLED=0`
- Use Go module/build cache mounts.
- Keep distroless/static final image.

## Legacy Dockerfile

Keep multi-arch:

- Preserve old tag and behavior.
- Split dependency install from source copy where practical.
- Use cache mounts for package manager caches.
- Avoid unnecessary native rebuilds.
- Do not make legacy AMD64-only.

## CI Workflow

Keep path filtering and all current checks, then add practical caches:

- NuGet: `~/.nuget/packages`
- pnpm store path
- Go module cache
- Go build cache
- Playwright browser cache when stable in Gitea

Avoid installing ShellCheck through apt on every run if a cached/action-provided path is available.

Avoid Playwright browser install when frontend/E2E did not change.

Cache misses or cache service failures must warn, not fail builds.

## Security Workflow

Keep:

- Gitleaks.
- .NET vulnerable package check.
- Frontend audit.
- Semgrep.
- Trivy filesystem scan.
- Trivy container image scan.

Optimize:

- Cache scanner/tool downloads where compatible.
- Cache NuGet/pnpm before audits.
- Avoid rebuilding the release image separately when a publish workflow already built it.
- Prefer scanning exact pushed image digest for main/tag builds.
- For PRs/branches, local image build is acceptable if no digest exists.
- Use path filters so docs-only changes do not run heavy scans unless scheduled/manual.

## Docker Build Workflows

Main:

- Keep `linux/amd64,linux/arm64`.
- Keep latest, SHA, semver tags.
- Keep registry BuildKit cache.

Pulse:

- Keep native binary artifacts for amd64 and arm64.
- Keep checksums and tag release attachment.
- Keep Docker multi-arch image.

Legacy:

- Keep `linux/amd64,linux/arm64`.
- Keep `old` tag.
- Keep registry cache.

## Tests

Add CI validation tests or scripts for:

- Main Dockerfile uses `$BUILDPLATFORM` and `TARGETARCH`.
- Pulse Dockerfile uses `$BUILDPLATFORM`, `TARGETOS`, and `TARGETARCH`.
- Dockerfiles contain expected BuildKit cache mounts.
- Workflows include dependency cache steps.
- Workflows preserve `linux/amd64,linux/arm64` for main, pulse, and legacy images.

## Acceptance

- CI remains strict.
- Security coverage remains strict.
- Main, Pulse, and legacy images remain multi-arch.
- ARM64 builds cross-compile where practical instead of running full SDK/toolchain stages under emulation.
- Build time is measurably reduced on warm cache.
