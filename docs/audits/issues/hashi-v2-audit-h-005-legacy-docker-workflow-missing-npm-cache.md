# H-005: Legacy Docker Workflow Missing npm Cache Mount

**Priority:** High
**Conflict Type:** bad_implementation
**Spec Reference:** Addendum §17.5; validate-ci-optimization.sh

**Status:** Fixed
**Branch:** h/docker-builds

## Description

The `validate-ci-optimization.sh` script checks that the legacy Dockerfile has `--mount=type=cache,target=/root/.npm` for npm cache. This check exists at line 50:

```bash
require_literal "$legacy_dockerfile" '--mount=type=cache,target=/root/.npm' 'legacy Dockerfile must cache npm downloads'
```

The legacy Dockerfile at `hashi.old/docker/Dockerfile` does include this cache mount:

```dockerfile
RUN --mount=type=cache,target=/root/.npm \
  npm ci --omit=dev
```

This is correctly implemented. However, the broader issue is that the legacy Docker workflow (`docker-build-old.yml`) does not pass any cache-related arguments to the build, meaning the BuildKit cache mounts may not persist across CI runs unless the Buildx builder has cache configured.

## Evidence

```yaml
# .gitea/workflows/docker-build-old.yml lines 42-51
- uses: docker/build-push-action@v6
  with:
    context: ./hashi.old
    file: ./hashi.old/docker/Dockerfile
    platforms: linux/amd64,linux/arm64
    push: true
    tags: ${{ steps.meta.outputs.tags }}
    labels: ${{ steps.meta.outputs.labels }}
    cache-from: type=registry,ref=${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:buildcache-old
    cache-to: type=registry,ref=${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:buildcache-old,mode=max
```

The `cache-from` and `cache-to` are configured for registry-based BuildKit cache. However, the BuildKit inline cache mounts (`--mount=type=cache`) for `/root/.npm` are separate from the registry cache and may not persist across CI runners unless the builder is configured with local cache.

## Expected Outcome

- npm downloads are cached across CI builds
- Legacy Docker builds don't re-download all packages every time
- Build time is reduced for subsequent builds

## Fix Guidance

The Dockerfile already has the cache mount correctly. The workflow already has registry cache configured. The actual npm cache mount may or may not persist depending on the Buildx builder configuration. To ensure persistence:

1. Verify that the Buildx builder used in CI has local cache enabled
2. Consider using `--build-arg BUILDKIT_INLINE_CACHE=1` if not already set
3. The registry cache (`cache-from: type=registry`) should handle layer caching across runs

The implementation is partially correct - the Dockerfile has the mount, and the workflow has registry cache. The gap is that inline cache mounts may not persist across different CI runners.

## Acceptance Criteria

- [x] Legacy Dockerfile has `--mount=type=cache,target=/root/.npm` (implemented)
- [x] Legacy Docker workflow has `cache-from` and `cache-to` (implemented)
- [ ] Verify that npm cache persists across CI runs in practice
