# Hashi V2 Spec Compliance Audit - 2026-06-09 (H-Series)

**Audit Date:** 2026-06-09
**Auditor:** AI Agent (Pass 1 of 3 + Pass 2 of 3 + Pass 3 of 3)
**Scope:** Full codebase comparison against `hashi-v2-implementation-spec.md` and `hashi-v2-implementation-spec-addendum.md` (excluding `hashi.old/`)
**Methodology:** Read implementation specs, explore all source code, compare spec requirements against actual implementation, identify gaps, misimplementations, and quality issues.

## Summary (Combined: Pass 1 [H-001–H-028] + Pass 2 [H-029–H-055] + Pass 3 [H-056–H-100])

| Category | Critical | High | Medium | Low | Total |
|----------|----------|------|--------|-----|-------|
| Docker Builds | 2 | 4 | 3 | 1 | 10 |
| CI/CD | 1 | 2 | 1 | 1 | 5 |
| Spec Compliance | 2 | 8 | 8 | 2 | 20 |
| Test Coverage | 0 | 2 | 3 | 0 | 5 |
| Code Quality | 0 | 1 | 5 | 6 | 12 |
| Security | 3 | 6 | 5 | 0 | 14 |
| Sync Engine | 1 | 4 | 5 | 1 | 11 |
| Architecture | 1 | 0 | 3 | 1 | 5 |
| UI/Visual | 0 | 2 | 1 | 3 | 6 |
| Resource Model | 0 | 0 | 2 | 2 | 4 |
| Monitoring | 0 | 0 | 0 | 3 | 3 |
| DNS/Firewall | 1 | 1 | 3 | 1 | 6 |
| **Total** | **11** | **30** | **39** | **20** | **100** |

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

### Pass 3 (H-056–H-100)

- [H-056: Sync Apply Has No Advisory Lock — Concurrent Applies Can Race](./hashi-v2-audit-h-056-sync-apply-no-advisory-lock.md)
- [H-057: GeoIP Rules Silently Bypassed When GeoIP Unavailable](./hashi-v2-audit-h-057-geoip-rules-silently-bypassed.md)
- [H-058: No First-Apply Firewall Rollback Protection](./hashi-v2-audit-h-058-firewall-no-first-apply-rollback.md)
- [H-059: Vault Lacks Three-Class Secret Taxonomy and Purpose-Specific Keys](./hashi-v2-audit-h-059-vault-lacks-secret-classes-and-purpose-keys.md)
- [H-060: CAPTCHA Solve Erases Offense History Counters](./hashi-v2-audit-h-060-captcha-solve-erases-offense-history.md)
- [H-061: No Offense Count Tracking for Ban Duration Policies](./hashi-v2-audit-h-061-no-offense-count-tracking.md)
- [H-062: Forward-Auth Missing 429 Rate-Limit Response](./hashi-v2-audit-h-062-forward-auth-missing-rate-limit-response.md)
- [H-063: No Immediate Sync Trigger After Config Save](./hashi-v2-audit-h-063-no-immediate-sync-after-save.md)
- [H-064: Forward-Auth No Configurable Fail-Open/Fail-Closed Behavior](./hashi-v2-audit-h-064-forward-auth-no-fail-open-fail-closed.md)
- [H-065: No Required Connection Minimum Count Enforcement](./hashi-v2-audit-h-065-no-required-connection-minimum-enforcement.md)
- [H-066: No Light Theme Implementation](./hashi-v2-audit-h-066-no-light-theme.md)
- [H-067: Invalid CNAME for on.* DNS Records When No Linked Host](./hashi-v2-audit-h-067-invalid-cname-on-dns-records.md)
- [H-068: No Default/Per-Resource OIDC Provider Support](./hashi-v2-audit-h-068-no-default-per-resource-oidc-provider.md)
- [H-069: Sync Plan Lacks Validation Step Before Apply](./hashi-v2-audit-h-069-sync-plan-lacks-validation.md)
- [H-070: Atomic Write Missing Validation Between Temp-Write and Move](./hashi-v2-audit-h-070-atomic-write-no-validation-before-move.md)
- [H-071: Missing Traefik Error Handling Middleware](./hashi-v2-audit-h-071-missing-traefik-error-handling-middleware.md)
- [H-072: Entry Point Removal Requires No Confirmation](./hashi-v2-audit-h-072-entry-point-removal-no-confirmation.md)
- [H-073: DNS Provider Capability Discovery Missing](./hashi-v2-audit-h-073-dns-provider-capability-discovery-missing.md)
- [H-074: No Warning Before Default-Deny Firewall Blocks Admin SSH](./hashi-v2-audit-h-074-no-warning-before-default-deny-firewall.md)
- [H-075: Sync Apply Does Not Auto-Apply Low-Risk Separately From High-Risk](./hashi-v2-audit-h-075-sync-apply-no-risk-tiering.md)
- [H-076: Reconcile Missing Verify/Hashes/Audit/Dependent Syncs](./hashi-v2-audit-h-076-reconcile-missing-verify-audit-dependent-syncs.md)
- [H-077: Pulse IP Change Does Not Trigger AdGuard Topology Rewrite Sync](./hashi-v2-audit-h-077-pulse-ip-change-no-adguard-sync.md)
- [H-078: No Guard Preventing Traefik Routers for hashi.home.arpa](./hashi-v2-audit-h-078-no-guard-against-traefik-routers-for-home-arpa.md)
- [H-079: Missing /api/dashboard/* Admin Dashboard Endpoint](./hashi-v2-audit-h-079-missing-dashboard-api-endpoint.md)
- [H-080: Missing Settings UI Panels for 7 Categories](./hashi-v2-audit-h-080-missing-settings-ui-panels.md)
- [H-081: Vault Setup Tradeoff Not Explicit in Setup Flow](./hashi-v2-audit-h-081-vault-setup-tradeoff-not-explicit.md)
- [H-082: Service-Sync Vault Unavailable Does Not Pause Jobs Gracefully](./hashi-v2-audit-h-082-service-sync-vault-unavailable-does-not-pause-gracefully.md)
- [H-083: Pulse AllowedScopesJson Never Enforced on Heartbeat](./hashi-v2-audit-h-083-pulse-allowed-scopes-never-enforced.md)
- [H-084: Pulse No Reachability Checks on Heartbeat IPs](./hashi-v2-audit-h-084-pulse-no-reachability-checks.md)
- [H-085: Resource Missing Fields: AdGuardRewriteVisibility, ExplicitRoutingOverride, SecurityProfile](./hashi-v2-audit-h-085-resource-missing-fields.md)
- [H-086: Monitor Paused State Unreachable in UI](./hashi-v2-audit-h-086-monitor-paused-state-unreachable.md)
- [H-087: No Database Views for Monitor Data Per Spec](./hashi-v2-audit-h-087-no-database-views-for-monitor-data.md)
- [H-088: Notification Routing Ignores Degraded→Up Recoveries](./hashi-v2-audit-h-088-notification-ignores-degraded-recoveries.md)
- [H-089: Script No Diff View or Target Hosts List in UI](./hashi-v2-audit-h-089-script-no-diff-view-or-target-list.md)
- [H-090: PlatformEndpoints.cs Crams 14 Endpoint Groups Into One 1482-Line File](./hashi-v2-audit-h-090-platform-endpoints-mono-file.md)
- [H-091: No Systemd Timer Option for Remote Script Scheduling](./hashi-v2-audit-h-091-no-systemd-timer-for-scripts.md)
- [H-092: Connection Target Validation at Resolution Time Not Save Time](./hashi-v2-audit-h-092-connection-target-validation-at-resolution.md)
- [H-093: Missing static.juzo.io Asset References](./hashi-v2-audit-h-093-missing-static-juzo-io-assets.md)
- [H-094: Firewall Apply No Skip-If-Unchanged](./hashi-v2-audit-h-094-firewall-apply-no-skip-if-unchanged.md)
- [H-095: No Remote Validation for DNS/Firewall/AdGuard Apply](./hashi-v2-audit-h-095-no-remote-validation-for-dns-firewall-adguard.md)
- [H-096: Forward-Auth Decision Flow Missing 429 and Order Deviates From Spec](./hashi-v2-audit-h-096-forward-auth-flow-order-and-429.md)
- [H-097: Resource DetectedFirewallHost Has No Auto-Detection Logic](./hashi-v2-audit-h-097-resource-detected-firewall-host-no-auto-detect.md)
- [H-098: AdGuard Duplicate Rewrite Rows Not Cleaned Up on Sync](./hashi-v2-audit-h-098-adguard-duplicate-rewrite-rows-no-cleanup.md)
- [H-099: Access Log Fields Not Minimized Per Spec](./hashi-v2-audit-h-099-access-log-fields-not-minimized.md)
- [H-100: No Stale Plan Recheck in Sync Apply](./hashi-v2-audit-h-100-no-stale-recheck-in-sync-apply.md)
