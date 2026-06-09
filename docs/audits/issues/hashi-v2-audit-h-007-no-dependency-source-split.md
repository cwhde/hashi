# H-007: Dockerfiles Do Not Split Dependency Install From Source Copy

**Priority:** Medium
**Conflict Type:** bad_implementation
**Spec Reference:** Addendum §17.3, §17.5

## Description

Both the main Dockerfile and legacy Dockerfile follow the correct pattern of copying dependency manifests before installing, then copying source. However, the user noted that "we have bad reusability" and "we have issues where we use unnecessarily large base images."

The main issues identified across both Dockerfiles:

1. **Main Dockerfile**: The `web-build` stage copies `web/` after install, which is correct but the layer ordering could be optimized
2. **Legacy Dockerfile**: Uses `node:20-alpine` which is larger than needed
3. **Both**: The multi-stage build pattern is correct but the runtime images could be smaller

The core pattern (copy manifests → install → copy source → build) is correctly implemented in both Dockerfiles. The "bad reusability" issue is primarily about layer invalidation when source changes, which is addressed in H-001.

## Evidence

Main Dockerfile follows the pattern:
```dockerfile
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install
COPY web/ ./
RUN pnpm run build
```

Legacy Dockerfile follows the pattern:
```dockerfile
COPY package.json package-lock.json ./
RUN npm ci --omit=dev
COPY ...
```

Both correctly separate dependency installation from source copying. The issue is that the `COPY web/ ./` in the main Dockerfile includes ALL web files, which invalidates subsequent layers.

## Expected Outcome

- Dependency installation layers are reused when only source changes
- Source changes only trigger build steps, not reinstall steps
- Base images are minimal

## Fix Guidance

The pattern is already correctly implemented. The specific improvements needed are:
1. H-001: Ensure web-build stage properly separates install from build
2. H-002: Use smaller base images for legacy
3. H-004: Remove unnecessary apt-get upgrade

## Acceptance Criteria

- [x] Dependency manifests are copied before install (implemented in both)
- [x] Source is copied after install (implemented in both)
- [ ] Layer cache hit rate is high when only source changes
- [ ] Base images are minimal
