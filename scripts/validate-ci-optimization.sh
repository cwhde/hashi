#!/usr/bin/env bash
set -euo pipefail

fail() {
  printf 'CI optimization validation failed: %s\n' "$1" >&2
  exit 1
}

require_pattern() {
  local file="$1"
  local pattern="$2"
  local description="$3"

  if ! grep -Eq -- "$pattern" "$file"; then
    fail "${description} (${file})"
  fi
}

require_literal() {
  local file="$1"
  local needle="$2"
  local description="$3"

  if ! grep -Fq -- "$needle" "$file"; then
    fail "${description} (${file})"
  fi
}

main_dockerfile="deploy/docker/Dockerfile"
pulse_dockerfile="agents/pulse/Dockerfile"
legacy_dockerfile="hashi.old/docker/Dockerfile"

require_literal "$main_dockerfile" 'FROM --platform=$BUILDPLATFORM node:' 'main Dockerfile must pin Node build stage to BUILDPLATFORM'
require_literal "$main_dockerfile" 'FROM --platform=$BUILDPLATFORM mcr.microsoft.com/dotnet/sdk:' 'main Dockerfile must pin .NET SDK stage to BUILDPLATFORM'
require_literal "$main_dockerfile" 'ARG TARGETARCH' 'main Dockerfile must declare TARGETARCH'
require_literal "$main_dockerfile" '--mount=type=cache' 'main Dockerfile must use BuildKit cache mounts'
require_literal "$main_dockerfile" 'hashi-pnpm-store' 'main Dockerfile must cache pnpm store'
require_literal "$main_dockerfile" 'hashi-nuget' 'main Dockerfile must cache NuGet packages'
require_literal "$main_dockerfile" 'linux-arm64' 'main Dockerfile must map arm64 to a .NET runtime identifier'
require_literal "$main_dockerfile" 'linux-x64' 'main Dockerfile must map amd64 to a .NET runtime identifier'

require_literal "$pulse_dockerfile" 'FROM --platform=$BUILDPLATFORM golang:' 'Pulse Dockerfile must pin Go build stage to BUILDPLATFORM'
require_literal "$pulse_dockerfile" 'ARG TARGETOS' 'Pulse Dockerfile must declare TARGETOS'
require_literal "$pulse_dockerfile" 'ARG TARGETARCH' 'Pulse Dockerfile must declare TARGETARCH'
require_literal "$pulse_dockerfile" 'GOOS="$target_os"' 'Pulse Dockerfile must build with target OS'
require_literal "$pulse_dockerfile" 'GOARCH="$target_arch"' 'Pulse Dockerfile must build with target arch'
require_literal "$pulse_dockerfile" '--mount=type=cache,target=/go/pkg/mod' 'Pulse Dockerfile must cache Go modules'
require_literal "$pulse_dockerfile" '--mount=type=cache,target=/root/.cache/go-build' 'Pulse Dockerfile must cache Go build data'

require_literal "$legacy_dockerfile" '--mount=type=cache,target=/root/.npm' 'legacy Dockerfile must cache npm downloads'

require_literal '.gitea/workflows/ci.yml' 'actions/cache@v3' 'CI workflow must include dependency/tool cache steps'
require_literal '.gitea/workflows/ci.yml' '~/.nuget/packages' 'CI workflow must cache NuGet packages'
require_literal '.gitea/workflows/ci.yml' 'pnpm store path' 'CI workflow must cache pnpm store'
require_literal '.gitea/workflows/ci.yml' '~/.cache/go-build' 'CI workflow must cache Go build data'
require_literal '.gitea/workflows/ci.yml' '~/.cache/ms-playwright' 'CI workflow must cache Playwright browsers'
require_literal '.gitea/workflows/ci.yml' 'shellcheck-' 'CI workflow must cache ShellCheck'
require_literal '.gitea/workflows/security.yml' 'actions/cache@v3' 'security workflow must include dependency/tool cache steps'
require_literal '.gitea/workflows/security.yml' '~/.cache/trivy' 'security workflow must cache Trivy data'
require_literal '.gitea/workflows/security.yml' 'published image digest' 'security workflow must prefer published image digest when available'

require_literal '.gitea/workflows/docker-build.yml' 'platforms: linux/amd64,linux/arm64' 'main image build must remain multi-arch'
require_literal '.gitea/workflows/docker-build-pulse.yml' 'platforms: linux/amd64,linux/arm64' 'Pulse image build must remain multi-arch'
require_literal '.gitea/workflows/docker-build-old.yml' 'platforms: linux/amd64,linux/arm64' 'legacy image build must remain multi-arch'
require_literal '.gitea/workflows/docker-build.yml' 'cache-from: type=registry' 'main image build must keep registry cache'
require_literal '.gitea/workflows/docker-build-pulse.yml' 'cache-from: type=registry' 'Pulse image build must keep registry cache'
require_literal '.gitea/workflows/docker-build-old.yml' 'cache-from: type=registry' 'legacy image build must keep registry cache'

printf 'CI optimization invariants validated.\n'
