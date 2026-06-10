# H-044: Docker Build Agents Directory Copied When Not Needed by dotnet-build

**Priority:** Low
**Conflict Type:** bad_implementation
**Spec Reference:** Addendum §17.3, Main Spec §30

**Status:** Fixed
**Branch:** h/backend-quality

## Description

In the main Dockerfile, the `dotnet-build` stage copies the entire `agents/` directory:

```dockerfile
# deploy/docker/Dockerfile:35
COPY agents/ ./agents/
```

The `dotnet-build` stage runs `dotnet restore` and `dotnet publish` for the .NET solution. The Pulse agent (written in Go under `agents/pulse/`) is not needed by any .NET build step. Copying it into the .NET build context:
1. Unnecessarily enlarges the Docker build context
2. Invalidates the NuGet cache layer when any file in `agents/` changes
3. Is inconsistent with the stated goal of build efficiency

The Pulse agent has its own separate Docker build workflow (`docker-build-pulse.yml`) and Dockerfile (`agents/pulse/Dockerfile`). It should not be part of the main Hashi container build.

However, note that the `Hashi.Api.csproj` may copy the pulse install script for inclusion in the runtime container. If so, only the specific needed files (`agents/pulse/install.sh`) should be copied, not the entire Go source and build artifacts.

## Evidence

```dockerfile
# deploy/docker/Dockerfile:34-35
COPY src/ ./src/
COPY agents/ ./agents/   # ← copies all 6 subdirectories/files including .gitignore, Makefile, README.md, cmd/hashi-pulse/main.go, go.mod, Dockerfile
```

## Expected Outcome

1. The `dotnet-build` stage should only copy files needed by .NET build (should not need `agents/` at all).
2. If the `Hashi.Api.csproj` MSBuild targets reference files from `agents/pulse/` for inclusion in published output, copy only those specific files.
3. The build cache should not be invalidated by changes to Go source code.

## Fix Guidance

1. Remove `COPY agents/ ./agents/` from the `dotnet-build` stage.
2. If needed for the runtime container, copy only the required file(s) (e.g., `COPY agents/pulse/install.sh ./`) or reference them from the `web-build` or final stage.
3. Verify that the .NET build and publish complete successfully without the agents directory.

## Acceptance Criteria

- [ ] `dotnet-build` stage does not copy the `agents/` directory
- [ ] Building the Docker image when only Go source changes does not invalidate the .NET build cache
- [ ] All required runtime files are included through explicit COPY of specific files
- [ ] Pulse install script inclusion (if needed) is handled separately from the .NET build context
