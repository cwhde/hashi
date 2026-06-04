# TASK-05: Blocklist Source Management

## Goal

Implement individually selectable recommended IP/CIDR blocklist sources, custom URL blocklists, SSRF-safe fetching, parsing/preview, fetch runs, effective-decision integration, and firewall sync integration.

## Spec Context

- Original spec sections: 13, 14, 19, 25, 31.
- Addendum sections: 8, 12.4, 13.1, 15, 16.4, 18, 19 Phase D.
- Research references: `RESEARCH-RESOURCES.md` Blocklist Feeds and Blocklist Parser sections.

## Current Code Anchors

- Current entry-only model: `BlocklistEntryEntity` in `ExtendedPlatformEntities.cs`
- Current firewall sync: `SecurityIngestionService.SyncBlocklistToAllFirewallsAsync`
- Firewall renderer: `src/Hashi.Core/Firewall/FirewallScriptRenderer.cs`
- Firewall apply: `src/Hashi.Infrastructure/Platform/FirewallApplyService.cs`
- Security endpoints: `PlatformEndpoints.cs` `MapSecurityEndpoints`
- Security page: `web/src/routes/(admin)/security/+page.svelte`
- Existing tests: `EdgeAuthServiceTests`, `FirewallApplySafetyTests`, `SecurityIngestionServiceTests`

## Recommended Feeds

Seed as disabled recommendations:

1. Feodo Tracker Botnet C2 IP blocklist.
2. Spamhaus DROP.
3. Spamhaus DROPv6.
4. DShield recommended block list.
5. FireHOL Level 1.
6. Custom URL.

No feed may be enabled silently. UI must show false-positive warning and per-feed metadata.

Use `RESEARCH-RESOURCES.md` for the current documentation URLs and direct URL candidates. Re-check feed docs and one small sample response immediately before implementation, then encode the observed format and rate-limit guidance in tests and seed metadata.

## Fetcher Requirements

For custom URLs:

- HTTPS only by default.
- Optional HTTP only with explicit warning.
- Reject local/private/link-local/multicast targets after DNS resolution.
- Reject redirects to private/internal targets.
- Limit redirect count.
- Limit response size.
- Use request timeout.
- Use Hashi user-agent.
- Support ETag and Last-Modified when present.
- Validate TLS certificates.
- Deny metadata service, Docker bridge, loopback, RFC1918, ULA, link-local, multicast targets by default.

Implement URL, DNS-result, redirect-target, and final-connection-IP validation.

## Parser Requirements

Support v1 formats:

- Plain line-based IP/CIDR lists.
- Plain line-based lists with comments.
- FireHOL `.ipset` / `.netset`.
- CSV/TSV with configurable column.
- JSON array of strings.
- JSON object containing a configured array field.

Parser must:

- Normalize IPv4/IPv6 and CIDR.
- Reject invalid entries.
- Deduplicate entries.
- Preserve source attribution.
- Preserve line number where possible.
- Record parse errors and ignored entries.
- Support preview before enabling.
- Never partially apply a failed update without recording status.

## API Deliverables

Add endpoints:

- `GET /api/security/blocklists`
- `POST /api/security/blocklists`
- `GET /api/security/blocklists/{id}`
- `PATCH /api/security/blocklists/{id}`
- `DELETE /api/security/blocklists/{id}`
- `POST /api/security/blocklists/{id}/fetch-preview`
- `POST /api/security/blocklists/{id}/enable`
- `POST /api/security/blocklists/{id}/disable`
- `POST /api/security/blocklists/{id}/refresh`
- `GET /api/security/blocklists/{id}/runs`
- `GET /api/security/blocklists/{id}/entries`

High-risk operations require recent reauthentication:

- Enable with firewall enforcement.
- Delete active source.
- Change source URL/enforcement mode.

## Enforcement

- IP/CIDR blocklists may be enforced in forward-auth and firewall.
- ASN/country/region rules remain forward-auth only by default.
- Firewall changes must produce preview/apply/result and use existing Hashi-owned chains/sets.
- Fetch failure keeps last known good entries and marks source degraded.
- Disabling a source removes its entries from effective decisions and queues firewall sync when needed.

## Frontend Deliverables

- Setup blocklist selection step with individual checkboxes.
- Security settings blocklist management.
- Custom URL add/edit form.
- Fetch preview table with parsed/rejected/ignored counts.
- Source health, last success, last error, entry count, stale status.

## Tests

- Parser formats and deduplication.
- SSRF target validation including redirects.
- Fetch failure preserves last known good entries.
- Enable/disable effective-decision behavior.
- Firewall plan generation from IP/CIDR blocklists.
- Audit events for source create/update/delete/enable/disable.
- E2E: enable recommended feed, add custom URL, preview entries.

## Acceptance

- No blocklist is enabled without explicit selection.
- Custom URL fetch is SSRF-protected.
- Firewall changes are previewed before high-risk applies.
- Last known good blocklist state survives transient fetch failures.
