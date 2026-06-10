# H-003: Legacy Dockerfile Missing apk Cache Cleanup

**Priority:** High
**Conflict Type:** bad_implementation
**Spec Reference:** Addendum §17.5; Main Spec §30 (Deployment)

**Status:** Fixed
**Branch:** h/docker-builds

## Description

The legacy Dockerfile's builder stage runs `apk add --no-cache python3 make g++` which correctly uses `--no-cache`. However, the overall image optimization could be improved. The `--no-cache` flag is used correctly in the builder stage, so the apk index is not cached. This is actually properly implemented.

However, upon closer inspection, the `apk add --no-cache` is correct. The real issue is that the runtime stage `node:20-alpine` itself contains package manager caches and Alpine base system files that could be further minimized.

## Evidence

```dockerfile
# hashi.old/docker/Dockerfile line 15
RUN apk add --no-cache python3 make g++
```

The `--no-cache` flag is correctly used, preventing apk index caching. This finding is a false positive for the apk cache specifically. The broader issue (H-002) about base image size remains valid.

## Expected Outcome

- No apk cache in builder or runtime stages
- Minimal Alpine base in runtime

## Fix Guidance

This specific finding is a false positive - the `--no-cache` flag is correctly used. The broader recommendation from H-002 about image size optimization still applies.

## Acceptance Criteria

- [x] `apk add` uses `--no-cache` flag (already implemented)
- [ ] Runtime image is as small as possible
