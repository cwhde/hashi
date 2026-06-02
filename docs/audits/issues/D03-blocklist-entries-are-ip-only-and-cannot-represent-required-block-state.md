# D03 - Blocklist entries are IP-only and cannot represent required block state

Priority: High

Spec conflicts: section 13.4 requires IP, ASN, and country/region block types. Block entries must include scope, reason, source, created by, created at, expiry, last hit, and applied-to host list. Section 19 requires top blocked IPs with count, last seen, country, ASN, reason, and expiry.

## Problem

The active abuse blocklist is modeled as permanent exact-IP rows. It cannot represent ASN blocks, country/region blocks, scoped blocks, expiry, source, creator, last-hit time, or per-host application state. Edge forward auth therefore enforces only exact IP blocks, and firewall sync has only a global `SyncedToFirewall` boolean rather than an applied-to host list.

The security dashboard contract has the same limitation: top blocked IPs are just strings, so the UI cannot show the count, last seen, country, ASN, reason, or expiry required by the spec.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:648-657` requires IP, ASN, and country/region block types.
- `docs/implementation-spec/hashi-v2-implementation-spec.md:659-668` lists the required block-entry fields.
- `docs/implementation-spec/hashi-v2-implementation-spec.md:1087-1098` requires security dashboard widgets including top blocked IPs with count, last seen, country, ASN, reason, and expiry.
- `src/Hashi.Infrastructure/Persistence/Entities/ExtendedPlatformEntities.cs:462-473` defines `BlocklistEntryEntity` with only `ClientIp`, `Reason`, `SyncedToFirewall`, and `CreatedAtUtc`.
- `src/Hashi.Infrastructure/Platform/EdgeAuthService.cs:23-27` denies only when `BlocklistEntries.ClientIp` exactly matches the evaluated IP.
- `src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:115-127` creates blocklist rows only from `request.ClientIp` with reason `abuse_score_threshold`.
- `src/Hashi.Infrastructure/Platform/FirewallApplyService.cs:67-70` builds firewall blocked subjects by selecting `ClientIp` from blocklist entries.
- `src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:394-447` syncs all pending blocklist rows to all firewall hosts and then flips a single `SyncedToFirewall` flag; it does not track which hosts applied each entry.
- `src/Hashi.Contracts/Api/PlatformContracts.cs:541-556` exposes `TopBlockedIps` as `IReadOnlyList<string>`.
- `src/Hashi.Infrastructure/Platform/SecurityIngestionService.cs:295-303` calculates top blocked IPs with counts internally but returns only the IP string.
- `web/src/routes/(admin)/security/+page.svelte:138-145` renders each top blocked IP with only the value `blocked`.

## Expected outcome

Blocklist entries should model the full scope and lifecycle required by the spec, and forward auth/firewall/dashboard paths should consume that model.

## Fix guidance

Replace or extend `BlocklistEntryEntity` with block scope/type/value, source, creator, expiry, last-hit timestamp, and a child table for applied host state. Teach Edge auth to enforce IP, ASN, and country/region entries using the same GeoIP context it already receives for resource rules. Keep firewall application limited to IP blocks unless explicitly expanded. Return structured dashboard DTOs for top blocked IPs.

## Acceptance criteria

- Block entries can represent IP, ASN, country, and region scopes.
- Block entries include reason, source, creator, created-at, expiry, last-hit, and per-host applied status.
- Expired blocks are not enforced by forward auth or firewall rendering.
- Forward auth enforces ASN and country/region blocks without expanding them to firewall ranges.
- Firewall sync records which hosts applied each relevant block.
- Security dashboard top blocked IPs include count, last seen, country, ASN, reason, and expiry.
- Tests cover IP, ASN, country, region, expiry, last-hit updates, and per-host sync state.
