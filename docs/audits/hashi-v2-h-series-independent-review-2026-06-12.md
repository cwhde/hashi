# Series H Independent Review - 2026-06-12

## Scope

This review treated the H-series issue files and commit messages as untrusted leads. The implementation specification, addendum, current source, migrations, generated contracts, and executable tests were used as the authority.

All 100 H-series findings were reviewed. Ninety-nine applicable findings are source-, test-, and where required runtime-verified. H-087 is a false positive caused by reading product "views" as PostgreSQL views.

## Independent Fixes

The earlier implementation was not accepted unchanged. This review found and corrected additional defects including:

- Shared undocumented API responses being consumed twice, breaking passkey and CAPTCHA flows.
- Missing security and setup E2E acceptance coverage.
- Reconcile failures not propagating correctly.
- IPv6 values being inserted into IPv4 firewall sets and IPv6 forwarding/NAT gaps.
- Public status ranges ignoring configured retention.
- Invalid resource names producing an empty slug.
- Missing production environment template.
- Firewall apply not verifying the deployed script hash.
- Duplicate remote AdGuard rewrites not being repaired.
- Detected firewall routing being calculated on an untracked entity, ignored by DNS generation, and duplicated by two migrations.
- Discord being implemented as webhook-only despite the bot/manual-ID/pairing specification.
- Endpoint modules still exceeding the H-090 300-line limit.
- The H-044 optimization removing the whole `agents/` copy also removed the Pulse installer required by publish, causing the production Docker build to fail.

## Disposition Matrix

| Issues | Result | Review evidence |
| --- | --- | --- |
| H-001 | Pass | Web dependency cache keys follow lockfile/package boundaries. |
| H-002 | Pass | Legacy amd64 runtime is 52 MiB, production-only, non-root, and contains no build tools; amd64/arm64 builds pass. |
| H-003-H-007 | Pass | Docker cleanup, dependency layering, and source-copy boundaries verified in Dockerfiles/workflows. |
| H-008 | Pass | Main amd64 runtime is 106 MiB; apt cleanup, healthcheck tooling, reproducible package steps, and multi-arch builds verified. |
| H-009-H-014 | Pass | Security build reuse, Pulse cache keys, build args, multi-platform scanning, frontend convention, and Traefik naming verified. H-010 corrected to hash the existing `go.mod` only. |
| H-015-H-019 | Pass | Frontend E2E, security E2E, unit, integration, and safety suites exist and pass. |
| H-020-H-023 | Pass | Connection target, rule, blocklist, and monitor endpoint contracts include required fields and persistence. |
| H-024 | Pass after independent fix | Telegram discovery, Discord bot manual IDs/pairing, SMTP test path, and vault-backed secrets verified. |
| H-025-H-028 | Pass | Script contract, operations docs, generated API contract, and Compose environment validation verified. |
| H-029-H-031 | Pass | Firewall shell escaping, passkey verification, and recovery-key KDF behavior verified by tests. |
| H-032-H-040 | Pass | Credential/configuration, cookie/session/CSRF, service-sync source, rule priority, and target resolution behavior verified. |
| H-041 | Pass | `node:24-alpine` frontend build and complete amd64/arm64 main image builds verified. |
| H-042-H-043 | Pass | Certificate resolver configurability and passkey lookup verified. |
| H-044 | Pass after independent fix | Docker publish now receives only the required Pulse installer instead of the whole agents tree; amd64/arm64 builds pass. |
| H-045 | Pass | Runner-compatible Pulse cache keys verified. |
| H-046 | Pass after independent fix | IPv4/IPv6 sets, forwarding, NAT, NetBird rules, and empty-family edge cases covered. |
| H-047-H-050 | Pass | Public status ranges, shared YAML helpers, batched expiry, and trusted forwarded context verified. |
| H-051 | Pass after independent fix | Empty/all-invalid input, separator collapse, trimming, and 63-character limit tested. |
| H-052-H-054 | Pass | Configurable internal URL, provider naming model, CGNAT/link-local DNS handling verified. |
| H-055 | Pass after independent fix | Required production DB password, closed dev DB port, and `.env.example` verified. |
| H-056-H-065 | Pass | Apply coordination, GeoIP fail-closed behavior, rollback, vault classes, offense history, 429 handling, immediate sync, failure policy, and deletion minimums verified. |
| H-066-H-074 | Pass | Theme, CNAME safety, OIDC defaults, plan validation, atomic validation, error middleware, removal confirmation, DNS capabilities, and firewall warnings verified. |
| H-075-H-086 | Pass | Risk-tiered apply, reconcile verification/audit/dependencies, Pulse-to-AdGuard sync, reserved domains, dashboard/settings, vault pause UX, scope/reachability enforcement, resource fields, and paused monitoring verified. |
| H-087 | False positive | Spec context describes user-facing time-range views. APIs use partitioned samples and retained 1m/5m/1h rollups for those views; SQL `CREATE VIEW` objects are not required. |
| H-088-H-089 | Pass | Degraded recovery routing and script diff/target/confirmation UI verified. |
| H-090 | Pass after independent fix | Endpoint domains are split and the largest endpoint file is 297 lines. |
| H-091-H-094 | Pass | Systemd/cron scheduling, save-time target validation, static asset caching, and unchanged firewall skip verified. |
| H-095 | Pass after independent fix | DNS and AdGuard read-back plus deployed firewall SHA-256 verification verified. |
| H-096 | Pass | Forward-auth ordering and distinct 429 response verified. |
| H-097 | Pass after independent fix | Detected host persists separately, checks managed/NetBird CIDRs, drives DNS routing, and manual override wins. |
| H-098 | Pass after independent fix | Remote duplicates for Hashi-managed domains are collapsed and audited while preserving the desired answer. |
| H-099-H-100 | Pass | Minimal Traefik access-log field allowlist and stale-plan rejection verified. |

## Verification

Final verification completed on 2026-06-12:

- `dotnet format Hashi.slnx --verify-no-changes --no-restore`
- `dotnet build Hashi.slnx -c Release --no-restore`: 0 warnings, 0 errors
- Unit tests: 501 passed
- Integration tests: 39 passed
- Frontend `check` and `lint`: passed with 0 diagnostics
- Frontend Vitest: 30 passed
- Frontend production build: passed
- Frontend Playwright: 25 passed
- Pulse Go 1.22.12 `go test ./...` and `go vet ./...`: passed
- `bash scripts/validate-ci-optimization.sh`: passed
- Fresh OpenAPI export and TypeScript generation: no committed artifact diff
- Merge simulation into current `origin/main` (`e93b64f`): no conflicts; the resulting tree exactly matched the tested audit branch tree
- Isolated Docker host verification: legacy amd64 image 52 MiB; main amd64 image 106 MiB; complete main and legacy builds passed for `linux/amd64` and `linux/arm64`

## Residual Observation Outside Series H

The deprecated `hashi.old` production lockfile reports 14 npm advisories (5 moderate and 9 high). The direct upgrade paths require major-version updates to bcrypt and Fastify and were not folded into the H-series Docker optimization work without compatibility tests. The current Hashi v2 application and its .NET/Svelte runtime are not affected by that legacy dependency tree.
