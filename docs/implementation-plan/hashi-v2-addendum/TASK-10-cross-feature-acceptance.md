# TASK-10: Cross-Feature Acceptance and Hardening

## Goal

Run the final integration pass after tasks 01-09 so the addendum is complete as a product feature, not just as isolated backend pieces.

## Spec Context

- Original spec sections: 3, 25, 29, 30, 31, 33.
- Addendum sections: 18 and 20.

## Acceptance Criteria From Addendum

The implementation is complete when:

1. A user can search an IP and see effective decision, history, events, rules, blocks, challenges, and blocklist hits.
2. A user can manually allow/block IPs and ranges.
3. Manual allow prevents Hashi-controlled blocking/escalation but does not bypass SSO or CAPTCHA.
4. Ban duration policy is configurable without unsafe eval.
5. Blocklists are individually selectable in setup and settings.
6. Custom blocklist URL fetch is SSRF-protected.
7. Blocklist entries can be enforced at middleware and firewall level where supported.
8. Cap integration works with an existing Cap instance.
9. CAPTCHA challenge endpoint cannot be deleted while CAPTCHA is enabled.
10. Cap admin dashboard is an optional normal resource, not a required protected system resource.
11. CAPTCHA solve clears current challenge and resets/decays triggering buckets only.
12. Continued spam while challenged escalates to soft/firewall block.
13. AdGuard and other supported connections can target Pulse agents instead of static IPs.
14. Internal agent DNS creates `agent.hashi.home.arpa` rewrites without replacing real-domain rewrites.
15. No reverse-proxy routing is generated for `hashi.home.arpa`.
16. Pulse remains Go.
17. Hashi main, Pulse, and legacy images remain multi-arch.
18. Docker builds use cross-compilation where practical instead of ARM64 emulation.
19. CI uses dependency/tool caches where available.
20. Security checks remain strict and are not removed for speed.

## Cross-Cutting Checks

Security:

- No Cap, OIDC, AdGuard, SSH, DNS, session, or vault secrets in logs, audit metadata, API responses, or frontend state.
- Unsafe admin endpoints require CSRF.
- High-risk operations require recent passkey reauthentication.
- Public challenge endpoints are anonymous only where intended.
- API-like challenged requests do not redirect blindly.
- Browser return URLs are same-origin/resource-safe.

Ownership and sync:

- DNS, Traefik, firewall, and AdGuard changes use plan/apply/result and audit logs.
- Firewall changes only use Hashi-owned chains/sets.
- Manual AdGuard rewrites are never deleted.
- `hashi.home.arpa` never generates Traefik routers.
- CAPTCHA required system resource cannot be deleted while enabled.

Contract:

- `openapi/hashi.json` regenerated.
- `web/src/lib/api/schema.d.ts` regenerated.
- Frontend API helpers updated.
- `git diff --check` clean.

## Required Test Passes

Backend:

- `dotnet format Hashi.slnx --verify-no-changes`
- `dotnet test Hashi.slnx /p:SkipFrontendBuild=true`

Frontend:

- `corepack pnpm install --frozen-lockfile`
- `corepack pnpm run check`
- `corepack pnpm run lint`
- `corepack pnpm run test`
- `corepack pnpm run build`
- E2E coverage for addendum workflows where feasible.

Pulse:

- `make vet`
- `make test`
- `make build`
- Docker build smoke if Docker is available.

Contracts:

- `scripts/export-openapi.sh`
- `scripts/generate-api-client.sh`
- `git diff --exit-code openapi/hashi.json web/src/lib/api/schema.d.ts`

CI/CD:

- Dockerfile validation tests from task 09.
- Workflow path/cache/platform assertions from task 09.

## Deliverables

- Final addendum implementation PR/branch summary.
- Updated docs if any behavior or operations guidance changed.
- Evidence list of commands run and results.
- Remaining risks documented explicitly.
