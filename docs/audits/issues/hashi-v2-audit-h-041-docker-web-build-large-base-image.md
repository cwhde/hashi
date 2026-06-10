# H-041: Main Dockerfile Uses Large node:24-bookworm for web-build Stage

**Priority:** Medium
**Conflict Type:** bad_implementation
**Spec Reference:** Addendum §17.3, §17.5

**Status:** Fixed
**Branch:** h/backend-quality

## Description

The main Dockerfile's `web-build` stage uses `node:24-bookworm` as the base image:

```dockerfile
FROM --platform=$BUILDPLATFORM node:24-bookworm AS web-build
```

`node:24-bookworm` is a full Debian-based image (~1 GB) containing a complete OS userland. For a build-only stage that only needs Node.js + pnpm to compile a SvelteKit frontend, this is unnecessarily large and pulls in hundreds of packages that are never used.

The `node:24-alpine` or `node:24-slim` variants would provide the same Node.js/npm/pnpm functionality at a fraction of the size (~120 MB for alpine, ~250 MB for slim), significantly reducing:
- Image pull time during CI/CD
- Build layer size
- Registry storage consumption

The Pulse Dockerfile already correctly uses `golang:1.22-alpine` for its build stage.

Additionally, the addendum §17.5 (legacy Dockerfile requirements) specifies: "Split dependency install from source copy where possible. Use BuildKit cache mounts for package manager caches. Avoid unnecessary native rebuilds." While this is for the legacy image, the same efficiency principles apply.

## Evidence

```dockerfile
# deploy/docker/Dockerfile:3
FROM --platform=$BUILDPLATFORM node:24-bookworm AS web-build
```

Compare to the efficient Pulse agent Dockerfile:
```dockerfile
# agents/pulse/Dockerfile:3
FROM --platform=$BUILDPLATFORM golang:1.22-alpine AS build
```

## Expected Outcome

The `web-build` stage should use `node:24-alpine` or `node:24-slim` as the base image. Alpine is preferred since:
1. `pnpm` and `corepack` are available on Alpine Node images
2. The SvelteKit build process only needs Node.js runtime, not a full Debian system
3. No native compilation (C++ addons) is required for the frontend build process

## Fix Guidance

1. Change `FROM --platform=$BUILDPLATFORM node:24-bookworm AS web-build` to `FROM --platform=$BUILDPLATFORM node:24-alpine AS web-build`.
2. Verify that `corepack enable` and `pnpm` work correctly on Alpine (they should, as these are pure JavaScript tools).
3. Verify the SvelteKit build completes successfully on Alpine.

## Acceptance Criteria

- [ ] web-build stage uses `node:24-alpine` or `node:24-slim`
- [ ] Frontend build completes successfully
- [ ] pnpm install and build work without additional packages
- [ ] Image pull time for web-build stage is measurably reduced
- [ ] Final multi-arch image builds for both linux/amd64 and linux/arm64
