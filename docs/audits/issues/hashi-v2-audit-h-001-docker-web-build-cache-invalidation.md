# H-001: Docker Build Layer Cache Invalidation in web-build Stage

**Priority:** Critical
**Conflict Type:** bad_implementation
**Spec Reference:** Addendum §17.3, §17.5, §17.8; Main Spec §30

## Description

In `deploy/docker/Dockerfile`, the `web-build` stage copies `web/package.json`, `web/pnpm-lock.yaml`, and `web/pnpm-workspace.yaml` for dependency installation, then copies the entire `web/` directory. The `COPY web/ ./` instruction after the install step invalidates the pnpm store cache layer whenever any source file under `web/` changes, even though the pnpm store is separately cached via BuildKit.

The real problem is that the stage does not separate dependency installation from source compilation. The `pnpm run build` step runs in the same layer as the source copy, meaning any source change forces a full rebuild from `COPY web/ ./` onward, including `pnpm run build`.

## Evidence

```dockerfile
# deploy/docker/Dockerfile lines 3-11
FROM --platform=$BUILDPLATFORM node:24-bookworm AS web-build
WORKDIR /src/web
RUN corepack enable && corepack prepare pnpm@9.15.9 --activate
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN --mount=type=cache,id=hashi-pnpm-store,target=/pnpm/store \
  pnpm config set store-dir /pnpm/store \
  && (pnpm install --frozen-lockfile || pnpm install --no-frozen-lockfile)
COPY web/ ./
RUN pnpm run build
```

The `COPY web/ ./` line copies ALL files under `web/` including source code, tests, config files. Any change to any file invalidates this layer and everything after it.

## Expected Outcome

- Only dependency installation layers are cached when source changes
- `pnpm run build` is a separate layer that only rebuilds when source changes
- Source-only changes don't trigger a full pnpm reinstall

## Fix Guidance

Restructure the web-build stage to:
1. Copy `package.json`, `pnpm-lock.yaml`, `pnpm-workspace.yaml` only
2. Run `pnpm install` (cached layer)
3. Copy source files separately
4. Run `pnpm run build` as the final layer

This ensures that when only source files change, the install layer is reused and only the build layer rebuilds.

## Acceptance Criteria

- [ ] `COPY web/ ./` is separated from `pnpm install` by a distinct layer boundary
- [ ] Changing a `.svelte` or `.ts` source file does not trigger a full `pnpm install`
- [ ] The `pnpm run build` layer rebuilds only when source files change
- [ ] The `validate-ci-optimization.sh` script still passes
