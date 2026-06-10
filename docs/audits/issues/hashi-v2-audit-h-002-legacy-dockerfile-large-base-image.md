# H-002: Legacy Dockerfile Uses Unnecessarily Large Base Image

**Priority:** Critical
**Conflict Type:** bad_implementation
**Spec Reference:** Addendum §17.5; Main Spec §30 (Deployment)

**Status:** Fixed
**Branch:** h/docker-builds

## Description

The legacy Dockerfile at `hashi.old/docker/Dockerfile` uses `node:20-alpine` for the runtime stage (stage 2). This image includes the full Node.js runtime, npm, and Alpine Linux base, resulting in an unnecessarily large final image. For a production Node.js application, a slimmer base image like `node:20-alpine` with `--omit=dev` is acceptable, but the current implementation does not properly minimize the image.

Additionally, the builder stage installs `python3 make g++` for native module compilation (bcrypt), which are heavy build dependencies. These are not needed in the final image, but the multi-stage build correctly isolates them. The issue is specifically with the runtime base image size.

## Evidence

```dockerfile
# hashi.old/docker/Dockerfile lines 10-24
FROM node:20-alpine AS builder
WORKDIR /app
RUN apk add --no-cache python3 make g++
COPY package.json package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
  npm ci --omit=dev

FROM node:20-alpine
```

The runtime stage uses `node:20-alpine` which is approximately 130MB+ compressed. The spec's Addendum §17.5 requires: "Optimize Dockerfile caching" and "Avoid unnecessary native rebuilds."

## Expected Outcome

- Runtime image uses a minimal base (Alpine with only Node.js runtime)
- No development/build tools in the final image
- Image size is minimized

## Fix Guidance

The current multi-stage build correctly separates build and runtime. The main optimization would be:
1. Ensure `npm ci --omit=dev` is used (it is)
2. Consider using `node:20-alpine` slim variant or distroless Node.js if available
3. Verify that no dev dependencies leak into the final image
4. Add labels and metadata properly (already present)

The image is functional but could be smaller. The spec's requirement to "avoid unnecessary native rebuilds" is met by the multi-stage separation.

## Acceptance Criteria

- [ ] Runtime stage does not contain python3, make, or g++
- [ ] Only production dependencies are installed in the final image
- [ ] Image size is documented and reasonable for a Node.js application
