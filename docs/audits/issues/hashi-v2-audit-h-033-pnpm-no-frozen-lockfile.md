# H-033: pnpm install --no-frozen-lockfile Produces Non-Reproducible Builds

**Priority:** High
**Conflict Type:** bad_implementation
**Spec Reference:** Main Spec §30 (CI/CD), Addendum §17.3

## Description

Multiple build targets use `pnpm install` with `--no-frozen-lockfile` or fallback logic that ignores the lockfile, producing non-reproducible builds:

**In `deploy/docker/Dockerfile` (line 9):**
```dockerfile
&& (pnpm install --frozen-lockfile || pnpm install --no-frozen-lockfile)
```

This first tries `--frozen-lockfile` but falls back to `--no-frozen-lockfile` on failure, meaning lockfile inconsistencies are silently ignored and version drift can occur.

**In `src/Hashi.Api/Hashi.Api.csproj`:**
```xml
<Exec Command="pnpm install --no-frozen-lockfile" WorkingDirectory="..\..\web" />
```

The MSBuild target used during .NET build unconditionally uses `--no-frozen-lockfile`, completely ignoring the lockfile. This means every build could potentially install different dependency versions depending on what's in the npm registry at build time.

The spec §30 requires the CI to "Restore pnpm" as a distinct step, and Addendum §17.9 demands: "CI validates source." Non-reproducible builds violate this principle.

## Evidence

1. `deploy/docker/Dockerfile:9` — Fallback from frozen to non-frozen lockfile
2. `src/Hashi.Api/Hashi.Api.csproj` — Unconditional `--no-frozen-lockfile`

## Expected Outcome

All builds (CI, Docker, local) must use `pnpm install --frozen-lockfile` unconditionally. If the lockfile is out of date, the build must fail, and the developer must update the lockfile. Never silently fall back to unfrozen installation.

## Fix Guidance

1. Change `deploy/docker/Dockerfile` line 9 to use only `pnpm install --frozen-lockfile` without fallback.
2. Change `Hashi.Api.csproj` to use `--frozen-lockfile`.
3. Ensure CI workflow also uses `--frozen-lockfile`.
4. Add a pre-commit hook or CI step to verify lockfile is up to date.

## Acceptance Criteria

- [ ] `--no-frozen-lockfile` appears nowhere in Dockerfiles or build targets
- [ ] All `pnpm install` calls use `--frozen-lockfile`
- [ ] CI fails if `pnpm-lock.yaml` is not in sync with `package.json`
- [ ] Docker build fails if lockfile is out of date rather than silently proceeding
