# H-008: Docker Build Multi-Stage Inefficiency - Final Image Size

**Priority:** Medium
**Conflict Type:** bad_implementation
**Spec Reference:** Addendum §17.3, §17.5

**Status:** Fixed
**Branch:** h/docker-builds

## Description

The main Dockerfile's final stage uses `mcr.microsoft.com/dotnet/aspnet:10.0` which is the full Debian-based ASP.NET runtime image. This includes the entire .NET runtime, ASP.NET libraries, and Debian base system. For a production deployment, this is appropriate but could be optimized.

The `curl` installation for healthcheck adds approximately 20-30MB to the final image. The `apt-get upgrade -y` step (noted in H-004) also increases image size unnecessarily.

For the legacy image, `node:20-alpine` is used which is smaller than the Debian-based .NET image but still includes the full Node.js runtime and npm.

## Evidence

```dockerfile
# Main Dockerfile final stage
FROM mcr.microsoft.com/dotnet/aspnet:10.0 AS final
RUN apt-get update && apt-get upgrade -y && apt-get install -y --no-install-recommends curl && rm -rf /var/lib/apt/lists/*

# Legacy Dockerfile final stage
FROM node:20-alpine
```

The .NET aspnet image is approximately 220MB compressed. The node:20-alpine image is approximately 130MB compressed. Both could potentially be smaller.

## Expected Outcome

- Final images are as small as possible
- No unnecessary packages in runtime images
- Healthcheck uses minimal tools

## Fix Guidance

1. Remove `apt-get upgrade -y` from main Dockerfile (H-004)
2. Consider using `wget` instead of `curl` if available in base image
3. For legacy, consider `node:20-alpine` with additional cleanup
4. Both images are functional but could be 10-30% smaller

## Acceptance Criteria

- [x] Final image size is documented
- [x] No unnecessary packages are installed
- [x] Healthcheck works with minimal tools
- [x] Images are reproducible (no random package upgrades)

## Verification - 2026-06-12

The amd64 main image is 112,134,813 bytes (106 MiB). Its final stage contains the ASP.NET 10.0 runtime and `curl` required by the declared healthcheck; apt lists are empty after installation and no random package upgrade is performed. The legacy amd64 runtime is 52 MiB. Fresh main and legacy builds completed successfully, including multi-architecture builds for `linux/amd64` and `linux/arm64`.
