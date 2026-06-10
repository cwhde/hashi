# H-011: Docker Build Workflow Does Not Pass Build Args

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** Addendum §17.8

**Status:** Not Started
**Branch:** 

## Description

The `docker-build.yml` workflow builds the main Docker image but does not explicitly pass any build arguments. The Dockerfile uses `ARG TARGETARCH` which is automatically set by Buildx for multi-platform builds. However, the workflow does not pass any custom build arguments that might be needed for configuration.

This is actually correct behavior - Buildx automatically sets `TARGETARCH` and `TARGETPLATFORM` for multi-platform builds. The `--platform` flag in the workflow (`platforms: linux/amd64,linux/arm64`) tells Buildx to build for both architectures, and the `ARG TARGETARCH` in the Dockerfile is automatically populated.

## Evidence

```yaml
# .gitea/workflows/docker-build.yml lines 50-59
- uses: docker/build-push-action@v6
  with:
    context: .
    file: ./deploy/docker/Dockerfile
    platforms: linux/amd64,linux/arm64
    push: true
    tags: ${{ steps.meta.outputs.tags }}
    labels: ${{ steps.meta.outputs.labels }}
    cache-from: type=registry,ref=${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:buildcache
    cache-to: type=registry,ref=${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:buildcache,mode=max
```

Buildx automatically handles `TARGETARCH`, `TARGETOS`, and `TARGETPLATFORM` for multi-platform builds. No explicit `build-args` are needed unless custom arguments are defined in the Dockerfile.

## Expected Outcome

- Multi-platform builds work correctly
- TARGETARCH is automatically set by Buildx
- No custom build args are needed

## Fix Guidance

This is correctly implemented. Buildx handles platform args automatically. No changes needed.

## Acceptance Criteria

- [x] Multi-platform builds are configured (implemented)
- [x] TARGETARCH is used in Dockerfile (implemented)
- [x] Buildx handles platform args automatically (correct behavior)
