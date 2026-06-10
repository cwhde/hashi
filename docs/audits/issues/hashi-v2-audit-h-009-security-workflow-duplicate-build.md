# H-009: Security Workflow Rebuilds Image When Published Digest Exists

**Priority:** High
**Conflict Type:** bad_implementation
**Spec Reference:** Addendum §17.7, §17.9

**Status:** Fixed
**Branch:** h/ci-cd

## Description

The security workflow (`security.yml`) builds a local Docker image for vulnerability scanning when no published image digest is available. This creates a duplicate build that may overlap with the `docker-build.yml` workflow.

Per Addendum §17.9: "Security should scan release image digest when available instead of building the same image again." The implementation correctly attempts to use the published digest first (lines 206-220), but falls back to building a local image (lines 222-231). This fallback is correct for PRs/branches where no published image exists, but could be optimized.

The workflow correctly prefers published digests for main/tag builds:
```yaml
- name: Resolve published image digest
  id: published-image
  if: github.event_name == 'push' && (github.ref == 'refs/heads/main' || startsWith(github.ref, 'refs/tags/'))
```

## Evidence

```yaml
# .gitea/workflows/security.yml lines 222-231
- name: Build image for vulnerability scan
  if: steps.published-image.outputs.ref == ''
  uses: docker/build-push-action@v6
  with:
    context: .
    file: ./deploy/docker/Dockerfile
    load: true
    push: false
    tags: hashi:security-scan
    cache-from: type=registry,ref=${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:buildcache
```

The fallback build uses `load: true` (single-platform) which is correct for local scanning. However, this duplicates work if `docker-build.yml` also runs on the same commit.

## Expected Outcome

- Security scans use published images when available
- Local builds only happen when no published image exists
- No duplicate expensive builds

## Fix Guidance

The implementation is correct per the spec. The optimization would be:
1. For main/tag builds: always use published digest (already implemented)
2. For PR/branch builds: build locally (necessary, already implemented)
3. Consider using `workflow_run` to trigger security scans after docker-build completes

The current implementation is acceptable. The spec says "PR/local security may still build a local single-platform image if needed."

## Acceptance Criteria

- [x] Published image digest is preferred for main/tag builds (implemented)
- [x] Local fallback exists for PR/branch builds (implemented)
- [x] Cache is used for local builds (implemented)
