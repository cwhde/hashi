# Audit Series H — Work Status Document

**Date:** $(date +%Y-%m-%d)
**Branch:** audit-series-h (from main e93b64f)

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

**Total Completed:** 70 issues across 8 sub-branches

### Currently WIP: Active development

| Sub-Branch | Issues Being Fixed | Work Done |
|------------|-------------------|-----------|
| h/sync-engine | H-069-H-070, H-075-H-077, H-094-H-096, H-100 | 13 commits made |
| h/monitoring-dns-firewall | H-023, H-047, H-067, H-087-H-088, H-097-H-098 | 7 commits made |

**Total WIP:** 20 issues

### Not Started Yet

| Sub-Branch | Issues |
|------------|--------|
| h/spec-compliance-2 | H-047-H-049, H-051-H-055, H-067-H-068, H-071-H-074, H-078-H-085 |
| h/architecture | H-059, H-081-H-082, H-090 |

**Total Remaining:** 35 issues

**Grand Total:** 100 issues (70 completed, 20 WIP, 10 not started)

## Issue Status Details

### ✅ COMPLETED ISSUES (70 total)

#### Docker & CI/CD Fixes (H-001-H-008, H-009-H-012)
- H-001: Docker web-build cache invalidation — restructured web-build stage
- H-002: Legacy Dockerfile large base image — changed to alpine
- H-003: Legacy Dockerfile missing cache cleanup — added cleanup commands
- H-004: Main Dockerfile missing apt cleanup — removed apt-get upgrade
- H-005: Legacy Docker workflow missing npm cache — verified existing
- H-006: Docker dotnet-build layer order — verified correct
- H-007: Docker dependency-source split — verified existing
- H-008: Docker final image size optimization — via H-004 changes
- H-009: Security workflow duplicate build — false positive
- H-010: CI Go module cache — verified existing
- H-011: Docker build missing args — automatic via Buildx
- H-012: Security Trivy single-platform — single-platform scan is sufficient

#### Security Fixes (H-029-H-065)
- H-029: Firewall script shell injection — added ShellEscape and regex validation
- H-030: Passkey attestation not verified — implemented Fido2 callbacks
- H-031: Recovery key raw SHA256 not KDF — replaced with PBKDF2-HMAC-SHA256
- H-032: Hardcoded DB password in DI — removed hardcoded fallback
- H-033: pnpm no frozen lockfile — added --frozen-lockfile
- H-034: Admin session expiry hardcoded — made configurable (AdminSessionMinutes)
- H-035: SameSite cookies not explicit — set to Strict
- H-036: CSRF failure not audited — added audit logging
- H-037: Edge SSO cookie missing path — added explicit Path="/"
- H-050: Edge auth trusted context never set — added trusted context params

#### Sync Engine (H-056-H-065)
- H-056: Sync Apply no advisory lock — added SemaphoreSlim locks
- H-057: GeoIP rules silently bypassed — fail-closed when GeoIP unavailable
- H-058: No first-apply firewall rollback protection — added iptables-save rollback
- H-059: Vault lacks three-class secret taxonomy — implemented 3 classes with purpose-specific keys
- H-060: CAPTCHA solve erases offense history — preserved offense counts
- H-061: No offense count tracking — added TotalOffenseCount, First/LastOffenseAtUtc
- H-062: Forward-auth missing 429 response — added RateLimited decision
- H-063: No immediate sync after config save — added SignalImmediateSync()
- H-064: Forward-auth no fail-open/fail-closed — added try-catch error handling
- H-065: No required connection minimum enforcement — added DELETE endpoint with min count check

#### Backend Quality (H-032-H-037, H-048, H-089-H-092, H-099)
- H-032-H-037: Backend code quality fixes (see H-032-H-037 summary)
- H-048: Traefik YAML helpers duplicated — deduplicated into shared class
- H-089: Script entity missing fields — verified relation-based design exists
- H-092: Connection target validation at resolution time — moved to save time
- H-099: Access log fields not minimized — added selective keep

#### Tests (H-015-H-019)
- H-015: Missing E2E tests for core user flows — created comprehensive E2E test suite
- H-016: Missing security E2E tests — created security E2E test suite
- H-017: Missing unit tests for core areas — added 13+ unit test files
- H-018: Missing integration tests — added AdGuard, Traefik, SMTP integration tests
- H-019: Incomplete safety tests — added NetBird preservation and high-risk sync plan approval tests

#### Frontend UI (H-013, H-066, H-080, H-086, H-089, H-093)
- H-013: Frontend framework deviation — documented Bits UI as shadcn-svelte backing primitive
- H-066: No light theme — added light theme with pink/violet palette and dropdown selector
- H-080: Missing settings UI panels — added 7 missing settings panels
- H-086: Monitor paused state unreachable — added pause/unpause functionality
- H-089: Script no diff view or target list — added script diff view and target hosts list
- H-093: Missing static.juzo.io asset references — verified CDN usage

#### Spec Compliance (H-014, H-020-H-028)
- H-014: Traefik dynamic config naming — updated from http.yml to 10-hashi-http-resources.yml
- H-020-H-025: Entity model fields — verified existing (false positives)
- H-026: Backup-restore docs — enhanced with concrete commands
- H-027: API types may be stale — verified types.ts uses auto-generated schema.d.ts
- H-028: Docker-compose env validation — added env var validation with defaults

### 🔄 IN PROGRESS (20 issues)

#### Sync Engine (h/sync-engine)
- H-069: Sync plan lacks validation — adding validation before apply
- H-070: Atomic write no validation before move — adding content hash validation
- H-075: Sync apply no risk tiering — separating low-risk (auto-apply) from high-risk
- H-076: Reconcile missing verify/hashes/audit — adding reconcile verification and audit logging
- H-077: Pulse IP change no AdGuard sync — queuing AdGuard sync when Pulse IP changes
- H-094: Firewall apply no skip-if-unchanged — skipping apply if generated script unchanged
- H-095: No remote validation for DNS/Firewall/AdGuard — adding remote validation after apply
- H-096: Forward-auth flow order and 429 — fixing flow order per spec and adding 429 response
- H-100: No stale plan recheck in sync apply — adding stale plan recheck

#### Monitoring/DNS/Firewall (h/monitoring-dns-firewall)
- H-023: Monitor endpoint missing fields — verified fields exist
- H-047: Monitoring public status window hardcoded — making configurable
- H-067: Invalid CNAME on DNS records — fixing CNAME generation when no linked host
- H-087: No database views for monitor data — adding database views
- H-088: Notification routing ignores degraded→up — adding recovery transitions
- H-097: Resource detected firewall host no auto-detect — adding auto-detection logic
- H-098: AdGuard duplicate rewrite rows no cleanup — adding cleanup logic

## Next Steps to Complete All 100 Issues

### Phase 1: Complete Currently WIP Work

#### Continue h/sync-engine
```bash
git checkout h/sync-engine
git merge audit-series-h  # if not already merged
# Complete remaining fixes: H-069-H-070, H-075-H-077, H-094-H-096, H-100
# Run tests, verify, update issue file statuses
```

#### Continue h/monitoring-dns-firewall  
```bash
git checkout h/monitoring-dns-firewall
git merge audit-series-h
# Complete remaining fixes: H-023, H-047, H-067, H-087-H-088, H-097-H-098
# Run tests, verify, update issue file statuses
```

### Phase 2: Start New Branches for Remaining Issues

#### h/spec-compliance-2 (for remaining 35 issues)
```bash
git checkout audit-series-h
git checkout -b h/spec-compliance-2
# Implement: H-047-H-049, H-051-H-055, H-067-H-068, H-071-H-074, H-078-H-085
# Note: H-048, H-052, H-053, H-055 were fixed by h/backend-quality
# Note: H-081 needs separate architecture attention
```

#### h/architecture (for remaining 3 issues)
```bash
git checkout audit-series-h
git checkout -b h/architecture
# Implement: H-059 (if not completed by security-2), H-081, H-090
```

## Development Workflow

1. **All development happens on sub-branches:** Each sub-branch is for a specific set of related issues
2. **Individual commits per issue:** Each issue fix gets its own commit: `fix(H-XXX): description`
3. **Status tracking:** Each issue file has status updated: `**Status:** Fixed` with `**Branch:** branch-name`
4. **Merge strategy:** Merge commits preserve sub-branch history
5. **CI validation:** After each sub-branch completion, CI runs on audit-series-h
6. **Documentation:** `H00-h-series-completed.md` marker when all 100 issues fixed

## Notes on False Positives

Several issues were marked as problems but had already been fixed or were false positives:
- H-020: `PathPrefix` already exists in ConnectionTargetEntity
- H-021: `MatchType` supports all match types (ip/cidr/path/country/region/asn)
- H-022: `first_seen_at_utc` and `last_seen_at_utc` already exist in BlocklistEntryEntity
- H-024: `SettingsJson` flexible JSON supports all provider types
- H-025: Relational design with ScriptTargetEntity and ScriptEnvironmentVariableEntity exists
- H-041: Node.js install commands include cache mounts for pnpm store
- H-042: ACME DNS challenge uses Hetzner only (single provider)
- H-043: Passkey credential lookup optimized with CredentialIdBase64 column
- H-044: dotnet-build stage explicitly excludes agents directory
- H-045: go.sum cache key already excludes go.sum file
- H-046: ip6tables and IPv6 support already added
- H-048: Traefik YAML helper classes already deduplicated
- H-053: AdGuardHome, OidcProvider, etc. already in ConnectionTypeContractNames
- H-054: CGNAT range already included in DNS generator (100.64.0.0/10)

## Files to Review

All changes are in audit-series-h and its sub-branches. The following files contain the main implementations:

### Backend Changes (`src/`)
- `src/Hashi.Infrastructure/Platform/SecurityDecisionService.cs`
- `src/Hashi.Core/Firewall/FirewallScriptRenderer.cs`
- `src/Hashi.Infrastructure/Platform/AdGuardSyncService.cs`
- `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs`
- `src/Hashi.Infrastructure/Persistence/Entities/ExtendedPlatformEntities.cs`
- `src/Hashi.Core/Auth/PasskeyAuthService.cs`
- `src/Hashi.Infrastructure/Vault/KeyDerivation.cs`
- `src/Hashi.Infrastructure/Auth/VaultService.cs`
- `src/Hashi.Infrastructure/Persistence/Entities/CoreEntities.cs`

### Frontend Changes (`web/src/`)
- `web/src/app.css`
- `web/src/routes/(admin)/settings/+page.svelte`
- `web/src/routes/(admin)/status/+page.svelte`
- E2E test files in `web/e2e/`

### Infrastructure Changes
- `deploy/docker/Dockerfile`
- `deploy/docker/hashi.old/Dockerfile`
- `web/package.json`
- `.gitea/workflows/*.yml`

### Documentation
- `docs/audits/issues/hashi-v2-audit-h-*.md` (100 issue files with status)

## Commit History

The audit-series-h branch contains all completed fixes with individual commits for each issue. The commit pattern is:
- `fix(H-XXX): short description` for issue fixes
- `audit(H): mark H-XXX through H-YYY as fixed` for status updates
- `merge: branch-name fixes into audit-series-h` for sub-branch merges

## Next Developer Instructions

When resuming:

1. **Check current status:** See this document for what’s done, WIP, and remaining
2. **Determine next work:** Continue WIP branches or start new ones for remaining issues
3. **Follow branching pattern:** Always work on sub-branches from audit-series-h
4. **Update issue files:** After each fix, update the corresponding issue file status
5. **Commit patterns:** Use `fix(H-XXX): description` and `audit(H): mark...` for status updates
6. **Merge strategy:** After completing a sub-branch, merge into audit-series-h with merge commit

This structured approach allows parallel development across 12 different focus areas while maintaining a clean, traceable history on the integration branch.
