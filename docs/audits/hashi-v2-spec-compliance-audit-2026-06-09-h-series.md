# Hashi V2 Spec Compliance Audit - 2026-06-09 (H-Series)

**Audit Date:** 2026-06-09
**Auditor:** AI Agent (Pass 1 of 3 + Pass 2 of 3)
**Scope:** Full codebase comparison against `hashi-v2-implementation-spec.md` and `hashi-v2-implementation-spec-addendum.md` (excluding `hashi.old/`)
**Methodology:** Read implementation specs, explore all source code, compare spec requirements against actual implementation, identify gaps, misimplementations, and quality issues.

## Summary (Combined: Pass 1 [H-001–H-028] + Pass 2 [H-029–H-055])

| Category | Critical | High | Medium | Low | Total |
|----------|----------|------|--------|-----|-------|
| Docker Builds | 2 | 4 | 3 | 1 | 10 |
| CI/CD | 1 | 2 | 1 | 1 | 5 |
| Spec Compliance | 1 | 3 | 4 | 1 | 9 |
| Test Coverage | 0 | 2 | 3 | 0 | 5 |
| Code Quality | 0 | 1 | 3 | 4 | 8 |
| Security | 2 | 4 | 3 | 0 | 9 |
| Architecture | 1 | 0 | 3 | 1 | 5 |
| **Total** | **7** | **16** | **20** | **8** | **55** |

## Findings Index

- [H-001: Docker Build Layer Cache Invalidation in web-build Stage](./hashi-v2-audit-h-001-docker-web-build-cache-invalidation.md)
- [H-002: Legacy Dockerfile Uses Unnecessarily Large Base Image](./hashi-v2-audit-h-002-legacy-dockerfile-large-base-image.md)
- [H-003: Legacy Dockerfile Missing apt/apk Cache Cleanup](./hashi-v2-audit-h-003-legacy-dockerfile-missing-cache-cleanup.md)
- [H-004: Main Dockerfile Runtime Stage Missing apt Cache Cleanup](./hashi-v2-audit-h-004-main-dockerfile-missing-apt-cleanup.md)
- [H-005: Legacy Docker Workflow Missing npm Cache Mount](./hashi-v2-audit-h-005-legacy-docker-workflow-missing-npm-cache.md)
- [H-006: Main Dockerfile dotnet-build Stage Copies Full Source Before Restore](./hashi-v2-audit-h-006-dotnet-build-layer-order.md)
- [H-007: Dockerfiles Do Not Split Dependency Install From Source Copy](./hashi-v2-audit-h-007-no-dependency-source-split.md)
- [H-008: Docker Build Multi-Stage Inefficiency - Final Image Size](./hashi-v2-audit-h-008-docker-final-image-size.md)
- [H-009: Security Workflow Rebuilds Image When Published Digest Exists](./hashi-v2-audit-h-009-security-workflow-duplicate-build.md)
- [H-010: CI Workflow Missing Go Module Cache Specificity](./hashi-v2-audit-h-010-ci-go-module-cache.md)
- [H-011: Docker Build Workflow Does Not Pass Build Args](./hashi-v2-audit-h-011-docker-build-missing-args.md)
- [H-012: Security Workflow Trivy Container Scan Image Build Not Cross-Platform](./hashi-v2-audit-h-012-security-trivy-single-platform.md)
- [H-013: Frontend Framework Deviation from Spec](./hashi-v2-audit-h-013-frontend-framework-deviation.md)
- [H-014: Traefik Dynamic Config File Naming Deviation](./hashi-v2-audit-h-014-traefik-dynamic-config-naming.md)
- [H-015: Missing E2E Tests for Core User Flows](./hashi-v2-audit-h-015-missing-e2e-tests.md)
- [H-016: Missing E2E Tests for Security and CAPTCHA Flows](./hashi-v2-audit-h-016-missing-security-e2e-tests.md)
- [H-017: Missing Unit Tests for Several Core Areas](./hashi-v2-audit-h-017-missing-unit-tests.md)
- [H-018: Missing Integration Tests for Key Flows](./hashi-v2-audit-h-018-missing-integration-tests.md)
- [H-019: Incomplete Test Coverage for Safety Requirements](./hashi-v2-audit-h-019-incomplete-safety-tests.md)
- [H-020: ConnectionTargetEntity Missing path_prefix Field](./hashi-v2-audit-h-020-connection-target-missing-path-prefix.md)
- [H-021: ResourceRuleEntity Missing Match Fields for Country/Region/ASN](./hashi-v2-audit-h-021-resource-rule-missing-match-fields.md)
- [H-022: BlocklistEntryEntity Missing first_seen_at_utc](./hashi-v2-audit-h-022-blocklist-entry-missing-fields.md)
- [H-023: MonitorEndpointEntity Missing Group/CheckType Fields](./hashi-v2-audit-h-023-monitor-endpoint-missing-fields.md)
- [H-024: NotificationProviderEntity Missing Type-Specific Config Fields](./hashi-v2-audit-h-024-notification-provider-missing-fields.md)
- [H-025: ScriptEntity Missing target_hosts and environment_vars Fields](./hashi-v2-audit-h-025-script-entity-missing-fields.md)
- [H-026: Missing docs/operations/backup-restore.md Verification](./hashi-v2-audit-h-026-backup-restore-docs-not-verified.md)
- [H-027: web/src/lib/api/types.ts May Be Stale](./hashi-v2-audit-h-027-api-types-may-be-stale.md)
- [H-028: Docker Compose Missing Environment Variable Validation](./hashi-v2-audit-h-028-docker-compose-env-validation.md)
