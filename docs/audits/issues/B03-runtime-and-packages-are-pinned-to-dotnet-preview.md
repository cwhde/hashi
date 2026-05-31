# B03 - Runtime and packages are pinned to .NET preview builds

Priority: High

Spec conflicts: section 4 requires `.NET 10 LTS` and C# 14, and section 27 expects deployable release images.

## Problem

The project still uses preview .NET images, preview package versions, and `LangVersion` set to `preview`. This is not the stable LTS runtime/language posture required by the spec and makes releases depend on preview package feeds and runtime behavior.

## Evidence

- `Directory.Build.props:3-4` targets `net10.0` but sets `<LangVersion>preview</LangVersion>`.
- `deploy/docker/Dockerfile:11` uses `mcr.microsoft.com/dotnet/sdk:10.0-preview`.
- `deploy/docker/Dockerfile:20` uses `mcr.microsoft.com/dotnet/aspnet:10.0-preview`.
- `src/Hashi.Api/Hashi.Api.csproj:9-10` references `10.0.0-preview.2.25163.8` packages.
- `src/Hashi.Infrastructure/Hashi.Infrastructure.csproj:23-31` references EF Core, hosting, logging, and Npgsql preview packages.
- `src/Hashi.Core/Hashi.Core.csproj:14-15` references preview Microsoft extension packages.

## Expected outcome

The build, package references, Dockerfile, and language version should use the stable .NET 10 LTS/C# 14 toolchain and matching provider packages.

## Fix guidance

Move Docker images and Microsoft package references to stable .NET 10 versions. Replace `LangVersion=preview` with C# 14 or remove the override if the SDK default is correct. Regenerate EF migrations only if required by package updates.

## Acceptance criteria

- No production Docker stage uses a `preview` .NET image.
- No non-migration package reference uses `10.0.0-preview.*`.
- The repository builds with the stable .NET 10 SDK.
- `dotnet test Hashi.slnx /p:SkipFrontendBuild=true` passes after the update.
