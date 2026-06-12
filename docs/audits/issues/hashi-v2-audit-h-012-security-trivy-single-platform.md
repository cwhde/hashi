# H-012: Security Workflow Trivy Container Scan Image Build Not Cross-Platform

**Priority:** Low
**Conflict Type:** bad_implementation
**Spec Reference:** Addendum §17.7

**Status:** Fixed
**Branch:** h/ci-cd

## Description

The security workflow builds a local Docker image for Trivy scanning when no published image exists. The local build uses `load: true` which builds for a single platform (the runner's architecture, typically amd64). This is correct for vulnerability scanning since the same vulnerabilities exist across architectures.

However, the spec requires "linux/amd64 and linux/arm64 multi-arch images" for the main image. The security scan only checks one architecture, which is acceptable since vulnerability scanning is architecture-agnostic for most packages.

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

The `load: true` flag builds a single-platform image for local use. This is correct for Trivy scanning since:
1. Most vulnerabilities are in shared libraries/packages
2. Building both platforms doubles scan time
3. The published multi-arch image is scanned separately when available

## Expected Outcome

- Security scans cover all relevant vulnerabilities
- Scan builds are fast and efficient
- Published images are scanned when available

## Fix Guidance

This is correctly implemented. Single-platform scanning is sufficient for vulnerability detection. No changes needed.

## Acceptance Criteria

- [x] Local builds are single-platform for efficiency (implemented)
- [x] Published images are scanned when available (implemented)
- [x] Vulnerability coverage is sufficient (correct behavior)
