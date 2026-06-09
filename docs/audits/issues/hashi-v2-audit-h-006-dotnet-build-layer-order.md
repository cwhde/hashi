# H-006: Main Dockerfile dotnet-build Stage Copies Full Source Before Restore

**Priority:** Medium
**Conflict Type:** bad_implementation
**Spec Reference:** Addendum §17.3

## Description

In `deploy/docker/Dockerfile`, the `dotnet-build` stage correctly separates csproj copying for restore from source copying for build. The restore step copies only csproj files, runs `dotnet restore`, then copies the full source. This is the standard pattern for Docker layer caching with .NET.

However, there's a subtlety: the restore step includes `COPY Directory.Build.props Hashi.slnx ./` before the csproj files. If `Directory.Build.props` changes (e.g., target framework update), the restore cache is invalidated even if dependencies haven't changed. This is correct behavior but worth noting.

The actual implementation is correct and follows the standard .NET Docker caching pattern. This finding is a minor observation.

## Evidence

```dockerfile
# deploy/docker/Dockerfile lines 13-33
FROM --platform=$BUILDPLATFORM mcr.microsoft.com/dotnet/sdk:10.0 AS dotnet-build
WORKDIR /src
ARG TARGETARCH
COPY Directory.Build.props Hashi.slnx ./
COPY src/Hashi.Api/Hashi.Api.csproj ./src/Hashi.Api/
COPY src/Hashi.Core/Hashi.Core.csproj ./src/Hashi.Core/
COPY src/Hashi.Contracts/Hashi.Contracts.csproj ./src/Hashi.Contracts/
COPY src/Hashi.Infrastructure/Hashi.Infrastructure.csproj ./src/Hashi.Infrastructure/
RUN --mount=type=cache,id=hashi-nuget,target=/root/.nuget/packages,sharing=locked \
  ...
  dotnet restore src/Hashi.Api/Hashi.Api.csproj
COPY src/ ./src/
COPY agents/ ./agents/
COPY --from=web-build /src/web/build ./web/build
RUN --mount=type=cache,id=hashi-nuget,target=/root/.nuget/packages,sharing=locked \
  ...
  dotnet publish src/Hashi.Api/Hashi.Api.csproj ...
```

The pattern is correct:
1. Copy build config + csproj files (stable layer)
2. Restore dependencies (cached)
3. Copy full source (invalidates on source change)
4. Publish (rebuilds on source change)

## Expected Outcome

- NuGet restore is cached when only source files change
- Source changes only trigger the publish step, not a full restore

## Fix Guidance

This is already correctly implemented. The standard .NET Docker caching pattern is followed. No changes needed.

## Acceptance Criteria

- [x] csproj files are copied before restore (implemented)
- [x] Full source is copied after restore (implemented)
- [x] NuGet packages are cached via BuildKit mount (implemented)
