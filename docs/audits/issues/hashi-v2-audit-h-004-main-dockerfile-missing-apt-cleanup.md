# H-004: Main Dockerfile Runtime Stage Missing apt Cache Cleanup

**Priority:** High
**Conflict Type:** bad_implementation
**Spec Reference:** Addendum §17.3; Main Spec §30 (Deployment)

**Status:** Not Started
**Branch:** 

## Description

The main Dockerfile's `final` stage installs `curl` for healthcheck purposes but does not clean up the apt cache after installation. This leaves the `/var/lib/apt/lists/` directory in the final image, adding unnecessary size. While the Dockerfile does run `rm -rf /var/lib/apt/lists/*` after the install, the `apt-get upgrade -y` step downloads and installs package upgrades that increase the image size beyond what's necessary.

The `apt-get upgrade -y` step is problematic because:
1. It upgrades ALL installed packages in the base image, not just the ones being installed
2. This makes the image non-reproducible - different builds may get different package versions
3. It increases build time and image size unnecessarily
4. The base image (`mcr.microsoft.com/dotnet/aspnet:10.0`) should already be up-to-date

## Evidence

```dockerfile
# deploy/docker/Dockerfile lines 51-56
FROM mcr.microsoft.com/dotnet/aspnet:10.0 AS final
WORKDIR /app
RUN apt-get update \
  && apt-get upgrade -y \
  && apt-get install -y --no-install-recommends curl \
  && rm -rf /var/lib/apt/lists/*
```

The `apt-get upgrade -y` line upgrades all packages in the base image. This is:
- Unnecessary if the base image is already up-to-date
- Non-reproducible across builds
- Adding significant build time
- The `rm -rf /var/lib/apt/lists/*` correctly cleans up apt cache, but the upgrade step is the issue

## Expected Outcome

- No unnecessary package upgrades in the final image
- Reproducible builds
- Minimal final image size
- Only curl is installed for healthcheck

## Fix Guidance

1. Remove `apt-get upgrade -y` - rely on the base image being current
2. Keep `apt-get install -y --no-install-recommends curl` for healthcheck
3. Keep `rm -rf /var/lib/apt/lists/*` for apt cache cleanup
4. Alternatively, use `wget` instead of `curl` if it's already available in the base image

The `aspnet:10.0` base image should be kept updated via base image updates, not via `apt-get upgrade` in the Dockerfile.

## Acceptance Criteria

- [ ] `apt-get upgrade -y` is removed from the final stage
- [ ] Only `curl` (or equivalent) is installed for healthcheck
- [ ] apt cache is cleaned up after install
- [ ] The healthcheck still functions correctly
- [ ] Build is more reproducible (no random package version changes)
