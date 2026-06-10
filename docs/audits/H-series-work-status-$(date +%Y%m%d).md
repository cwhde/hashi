# Audit Series H — Work Status Document

**Last Updated:** 2026-06-10 (session 2)
**Branch:** audit-series-h (from main e93b64f)
**Active Subagent quota exhausted (429 rate limit) — session stopped mid-work**

---

## ⚠️ CURRENT STATE — READ FIRST

The session was interrupted by quota limits. Work was **in progress** on this branch.
The following changes are **staged locally but NOT yet committed** and must be committed on resume.

### Uncommitted Changes on `audit-series-h` (as of 2026-06-10 ~09:30 CEST)

All these files are modified but uncommitted:

#### Issue Fixes (WIP/Done — need commit):
| File | What Changed | Issue |
|------|-------------|-------|
| `docs/audits/issues/hashi-v2-audit-h-039-*.md` | Status updated to Fixed | H-039 |
| `docs/audits/issues/hashi-v2-audit-h-049-*.md` | Status updated to Fixed | H-049 |
| `src/Hashi.Infrastructure/Auth/ServiceSyncVaultBootstrapper.cs` | Read key from env var only, warn if in appsettings.json | H-039 |
| `src/Hashi.Infrastructure/Platform/SecurityAddendumJobWorker.cs` | N+1 fix: single batch query + in-memory lookup (ToUpperInvariant for case-insensitive tuple match) | H-049 |

#### H-068: Default Per-Resource OIDC Provider (partially done — need commit + migration):
| File | What Changed |
|------|-------------|
| `src/Hashi.Infrastructure/Persistence/Entities/ExtendedPlatformEntities.cs` | Added `IsDefault bool` to `OidcProviderEntity` |
| `src/Hashi.Infrastructure/Persistence/Entities/PlatformEntities.cs` | Added `OidcProviderId Guid?` + `OidcProvider` nav prop to `ResourceEntity`, also `ErrorHandlingEnabled bool = true` |
| `src/Hashi.Infrastructure/Persistence/HashiDbContext.cs` | FK config for `OidcProvider` on `ResourceEntity` with `OnDelete.SetNull` |
| `src/Hashi.Infrastructure/Platform/OidcProviderAdminService.cs` | `IsDefault` handling: `ClearOtherDefaultsAsync` on create/update |
| `src/Hashi.Infrastructure/Platform/PlatformServices.cs` | Maps `OidcProviderId`, `ErrorHandlingEnabled` to response/definition; UpdateAsync handles `ClearOidcProviderId` + `ErrorHandlingEnabled` |
| `src/Hashi.Contracts/Api/PlatformContracts.cs` | `ResourceResponse`, `CreateResourceRequest`, `UpdateResourceRequest` all updated with `OidcProviderId` and `ErrorHandlingEnabled` |
| `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs` | Updated to handle OIDC default provider resolution |
| `src/Hashi.Api/Features/Platform/ErrorEndpoints.cs` | **NEW FILE** — `/api/error/{status}` endpoint serving styled 5xx HTML error pages |
| `src/Hashi.Api/Hosting/AdminApiAuthMiddleware.cs` | Whitelisted `/api/error/` prefix from auth |
| `src/Hashi.Api/Program.cs` | Registered `MapErrorEndpoints()` |
| `src/Hashi.Core/Resources/ResourceModels.cs` | Added `ErrorHandlingEnabled` to `ResourceDefinition` |
| `src/Hashi.Core/Traefik/TraefikConfigRenderer.cs` | Added `hashi-errors` middleware + service in `RenderCoreMiddlewares` (conditional on `ErrorHandlingEnabled`); added `HashiErrorUrl`, `ErrorHandlingEnabled` to `TraefikRenderOptions` |
| `src/Hashi.Infrastructure/Persistence/Entities/CoreEntities.cs` | Added `ErrorHandlingEnabled bool = true` to `AppSettingsEntity` |

#### Other Infrastructure Fixes (uncommitted):
| File | What Changed | Issue |
|------|-------------|-------|
| `src/Hashi.Core/Dns/DnsModels.cs` | Various DNS model fixes | Multiple |
| `src/Hashi.Core/Firewall/FirewallScriptRenderer.cs` | Firewall script renderer changes | Multiple |
| `src/Hashi.Infrastructure/Connections/SshConnectionService.cs` | SSH connection fixes | Multiple |
| `src/Hashi.Infrastructure/Crypto/KeyDerivation.cs` | Key derivation changes | Multiple |
| `src/Hashi.Infrastructure/Dns/DnsDesiredStateBuilder.cs` | DNS desired state builder | Multiple |
| `src/Hashi.Infrastructure/Platform/CaptchaChallengeService.cs` | Captcha challenge fixes | Multiple |
| `src/Hashi.Infrastructure/Platform/ConnectionTargetResolver.cs` | Connection resolver fixes | Multiple |
| `src/Hashi.Infrastructure/Platform/MonitoringService.cs` | Monitoring service updates | Multiple |
| `src/Hashi.Infrastructure/Platform/SecurityDecisionService.cs` | Security decision fixes (also has merge conflict markers partially cleaned) | Multiple |
| `tests/Hashi.IntegrationTests/AdGuardIntegrationTests.cs` | AdGuard integration test fixes | Multiple |
| `tests/Hashi.IntegrationTests/SmtpFakeServerTests.cs` | SMTP test fixes | Multiple |
| `tests/Hashi.IntegrationTests/TraefikConfigValidationTests.cs` | Traefik config validation tests | Multiple |
| `tests/Hashi.UnitTests/ConnectionTargetResolverTests.cs` | Connection resolver unit tests | Multiple |
| `tests/Hashi.UnitTests/HighRiskSyncPlanApprovalTests.cs` | Sync plan approval tests | Multiple |
| `tests/Hashi.UnitTests/NetBirdPreservationTests.cs` | NetBird preservation tests | Multiple |
| `tests/Hashi.UnitTests/ResourceRuleEvaluationTests.cs` | Resource rule tests | Multiple |
| `tests/Hashi.UnitTests/StatusRollupTests.cs` | Status rollup tests | Multiple |
| `tests/Hashi.UnitTests/TraefikRenderOutputTests.cs` | Traefik render tests | Multiple |

#### Untracked New Files (need `git add`):
| File | Description |
|------|-------------|
| `dotnet-tools.json` | EF tools manifest |
| `src/Hashi.Api/Features/Platform/ErrorEndpoints.cs` | New error page endpoint |
| `src/Hashi.Infrastructure/Persistence/Migrations/20260610065242_FixPendingModelChanges.*` | EF migration |
| `src/Hashi.Infrastructure/Persistence/Migrations/20260610071427_AddResourceDefaultOidcProvider.*` | EF migration |
| `src/Hashi.Infrastructure/Persistence/Migrations/20260610072248_AddErrorHandlingEnabled.*` | EF migration |

---

## ⚠️ KNOWN TEST FAILURE (must fix before committing)

**Failing test:** `Hashi.UnitTests.TraefikConfigRendererTests.Render_uses_configured_internal_urls_for_hashi_middlewares_and_health_service`

**Reason:** The new `hashi-errors` service section in `RenderCoreMiddlewares` contains a URL derived from `HashiHealthUrl` (e.g. `http://127.0.0.1:8080`). The test asserts `DoesNotContain("127.0.0.1:8080")` across the whole core YAML — but the test is checking that the health URL was replaced with a custom URL. The hashi-errors service also includes the default URL.

**Fix needed:** The test at `tests/Hashi.UnitTests/PlatformTests.cs:124` needs to be updated to also pass a custom `HashiErrorUrl` in the test options, OR the `hashi-errors` service URL should only appear when `ErrorHandlingEnabled = true` AND a non-default URL is configured. The simplest fix is to update the test to also set `HashiErrorUrl` in the `TraefikRenderOptions` when setting custom URLs.

**File:** `tests/Hashi.UnitTests/PlatformTests.cs` around line 112-130

---

## ⚠️ REMAINING ISSUES — H-071 is partially done

### H-071: Missing Traefik Error Handling Middleware
- **Status:** Partially implemented
- **What's done:**
  - `hashi-errors` middleware added to `RenderCoreMiddlewares` (conditional)
  - `hashi-errors` service added pointing to internal Hashi error URL
  - `ErrorHandlingEnabled` flag added to `ResourceEntity`, `AppSettingsEntity`, `ResourceDefinition`, `TraefikRenderOptions`
  - `/api/error/{status}` endpoint created in `ErrorEndpoints.cs` serving styled HTML
  - Auth bypass added for `/api/error/` prefix
  - EF migration `AddErrorHandlingEnabled` generated
- **Still needed:**
  - Fix the failing unit test (see above)
  - Update issue status markdown to "Fixed"
  - Commit everything

---

## Summary of Work Status

### Completed: Merged into audit-series-h

| Sub-Branch | Issues Fixed | Status |
|------------|--------------|--------|
| h/docker-builds | H-001-H-008 | ✅ Merged |
| h/ci-cd | H-009-H-012 | ✅ Merged |
| h/security-1 | H-029-H-039, H-050 | ✅ Merged |
| h/security-2 | H-056-H-065 | ✅ Merged |
| h/backend-quality | H-032-H-037, H-048, H-089-H-092, H-099 | ✅ Merged |
| h/tests | H-015-H-019 | ✅ Merged |
| h/frontend-ui | H-013, H-066, H-080, H-086, H-089, H-093 | ✅ Merged |
| h/spec-compliance-1 | H-014, H-020-H-028, H-038, H-040-H-046 | ✅ Merged |
| h/sync-engine | H-069, H-075, H-077, H-087, H-088, H-094, H-097 | ✅ Merged |

**Total from branches:** ~80 issues

### Uncommitted (need commit on resume):
- H-039: ServiceSyncVaultBootstrapper reads key from env var ✅ implemented, needs commit
- H-049: SecurityAddendumJobWorker ExpireBlocksAsync N+1 fix ✅ implemented, needs commit
- H-068: Default OIDC Provider per resource ✅ implemented (entity+API+migrations), needs commit
- H-071: Traefik error handling middleware ✅ largely implemented, **test failure needs fix first**

### ❌ Not Started (not touched this session):
| Issue | Description |
|-------|-------------|
| H-072 | Confirmation before entrypoint removal |
| H-073 | DNS provider capability discovery |
| H-074 | Warning before default deny firewall |
| H-078 | Guard against Traefik routers for `.home.arpa` |
| H-079 | Missing dashboard API endpoint |
| H-081 | Vault setup tradeoff not explicit (subagent was working on this — quota exceeded) |
| H-082 | Service-sync vault unavailable does not pause gracefully (subagent was working) |
| H-083 | Pulse allowed scopes never enforced (subagent was working) |
| H-084 | Pulse no reachability checks (subagent was working) |
| H-085 | Resource missing fields (subagent was working) |
| H-086 | Monitor paused state unreachable (subagent was working — may have partial work) |
| H-090 | Platform endpoints splitting (mono-file refactor) |
| H-091 | Systemd timer configuration for scripts |
| H-092 | Connection target validation at resolution |

> **Note on H-081 through H-086:** The subagent working on these hit a quota limit (429) and was killed mid-task. Its worktree is at `/home/juzo/.gemini/antigravity-cli/brain/6b5e8646-3a8d-43b6-a36d-e57c17109250/.system_generated/worktrees/subagent-Security-and-Spec-Compliance-Developer-self-a66d9eba`. The branch `subagent-Security-and-Spec-Compliance-Developer-self-a66d9eba` exists and is currently checked out at the same commit as `audit-series-h` (no commits made by subagent). Its worktree had an uncommitted diff to `SecurityDecisionService.cs` (had merge conflict markers from h/backend-quality — may need manual cleanup).

---

## How To Continue

### Step 1: Fix the test failure
```bash
# Edit tests/Hashi.UnitTests/PlatformTests.cs around line 112-130
# Find the test: Render_uses_configured_internal_urls_for_hashi_middlewares_and_health_service
# Add: HashiErrorUrl = "http://custom-host:8080" to the TraefikRenderOptions
dotnet test --filter "Render_uses_configured_internal_urls"
```

### Step 2: Commit all current work
```bash
cd /home/juzo/git-repos/hashi
git add -A
git commit -m "fix(H-039): read service sync vault key exclusively from HASHI_SERVICE_SYNC_VAULT_KEY env var"
git add src/Hashi.Infrastructure/Platform/SecurityAddendumJobWorker.cs docs/audits/issues/hashi-v2-audit-h-049-*.md
git commit -m "fix(H-049): refactor ExpireBlocksAsync to use batch query + in-memory lookup (eliminates N+1)"
# Then commit H-068 and H-071 together or separately
git commit -m "fix(H-068): add IsDefault to OidcProvider, OidcProviderId to Resource, default OIDC resolution in PlatformEndpoints"
git commit -m "fix(H-071): add hashi-errors middleware to Traefik core config with styled /api/error/{status} endpoint"
```

### Step 3: Fix SecurityDecisionService.cs merge conflict markers
The subagent worktree had conflict markers in `SecurityDecisionService.cs`. Check the main branch's copy:
```bash
grep -n "<<<<<<\|=======\|>>>>>>>" src/Hashi.Infrastructure/Platform/SecurityDecisionService.cs
```
If conflict markers found, resolve them (the `HEAD` version is the correct one from `h/sync-engine` merge).

### Step 4: Implement remaining issues
For each remaining issue, read the markdown file in `docs/audits/issues/` and implement:

- **H-072** (entry point removal confirmation): Add confirmation dialog/flag before removing Traefik entrypoints
- **H-073** (DNS provider capability discovery): Add capability metadata to DNS provider connections
- **H-074** (warning before default deny firewall): Add user warning when applying default-deny firewall rules
- **H-078** (no home.arpa guard): In `TraefikConfigRenderer`, skip resources with `.home.arpa` domains or validate
- **H-079** (missing dashboard API): Add `/api/dashboard` endpoint returning widget data
- **H-081** (vault setup tradeoff docs): Update docs/setup to explain vault encryption tradeoffs
- **H-082** (service sync vault pause): Make sync pause gracefully when vault unavailable
- **H-083** (pulse scope enforcement): Validate `AllowedScopesJson` in `AcceptHeartbeatAsync`
- **H-084** (pulse reachability checks): Add reachability ping before accepting pulse heartbeat
- **H-085** (resource missing fields): Add any missing fields to resource model/API
- **H-090** (platform endpoints refactor): Split `PlatformEndpoints.cs` (68KB mono-file) into multiple files
- **H-091** (systemd timer for scripts): Add systemd timer config generation for script scheduling
- **H-092** (connection target validation): Validate connection targets when resolving

### Step 5: Update H-series status
Update the WIP entries in the `## IN PROGRESS` section of this document.

### Step 6: Push
```bash
git push origin audit-series-h
```

---

## Branch Map

```
main
└── audit-series-h  ← working branch, HEAD at 4b07898
    ├── h/docker-builds   (merged)
    ├── h/ci-cd           (merged)
    ├── h/security-1      (merged)
    ├── h/security-2      (merged)
    ├── h/backend-quality (merged)
    ├── h/tests           (merged)
    ├── h/frontend-ui     (merged)
    ├── h/spec-compliance-1 (merged)
    ├── h/sync-engine     (merged, at c27a709)
    └── h/monitoring-dns-firewall  (local, NOT merged yet)
```

> `h/monitoring-dns-firewall` has commits for H-023, H-047, H-067, H-087, H-088, H-097, H-098 but was **NOT merged into audit-series-h** in this session. These were merged as part of the h/sync-engine merge (4b07898 is actually the sync-engine merge). Verify status by checking git log.

---

## Development Conventions

1. **Branch naming:** `h/descriptive-name` branched from `audit-series-h`
2. **Commit format:** `fix(H-XXX): short description`
3. **Issue status format:** In each issue `.md`, change `**Status:** Not Started` → `**Status:** Fixed` and set `**Branch:** audit-series-h`
4. **After fixing:** Run `dotnet test` to confirm all pass
5. **Migrations:** After entity changes, run `dotnet ef migrations add MigrationName -p src/Hashi.Infrastructure -s src/Hashi.Api`
6. **Merge:** `git checkout audit-series-h && git merge --no-ff h/branch-name`

---

## Test Status (as of 2026-06-10 session stop)

- **Unit tests:** 472 passing, **1 failing** (TraefikConfigRendererTests.Render_uses_configured_internal_urls...)
- **Integration tests:** 37 passing, 0 failing
- **Fix needed:** PlatformTests.cs line ~124 — add `HashiErrorUrl` to custom URL test options
