# B14 - Manual DNS record CRUD is missing

Priority: High

Spec conflicts: section 15.2 requires a manual DNS tab with at least A, AAAA, CNAME, MX, and TXT records, and section 15 requires ownership-aware safe DNS management.

## Problem

Hashi has a DNS record table and the desired-state builder reads `DnsRecords` as manual desired state, but the admin API and UI only list those records and import provider records. There is no create, edit, disable, or delete flow for manual DNS entries.

Even for records that do exist, generated DNS records can remove same-name manual records during desired-state merge, which is not safe for a manual DNS feature.

## Evidence

- `src/Hashi.Infrastructure/Dns/DnsDesiredStateBuilder.cs:18-27` reads enabled `db.DnsRecords` as manual desired records.
- `src/Hashi.Api/Features/Dns/DnsEndpoints.cs:163-168` exposes only `GET /api/dns/records` for DNS records.
- `web/src/routes/(admin)/dns/+page.svelte:335-351` renders a "Managed records" inventory without manual create/edit/delete controls.
- `web/src/routes/(admin)/dns/+page.svelte:118-181` implements import and prune flows, but not manual record CRUD.
- `src/Hashi.Infrastructure/Dns/DnsDesiredStateBuilder.cs:91-101` removes existing same-name records from the manual set when a generated record is merged.
- `src/Hashi.Infrastructure/Persistence/Entities/DnsEntities.cs:180-186` includes ownership names for `Imported`, `Managed`, `System`, and `User`, but the API does not expose a user-owned manual record mutation path.

## Expected outcome

Users should be able to manage manual A, AAAA, CNAME, MX, and TXT records in Hashi. Manual records must be preserved unless the user explicitly confirms a conflicting generated record should replace them.

## Fix guidance

Add DNS record CRUD endpoints with ownership and validation. Add UI controls for manual records. Update merge behavior so generated records do not silently remove user/manual records with the same name unless a sync plan marks the conflict and the user confirms replacement.

## Acceptance criteria

- Users can create, update, disable, and delete manual A, AAAA, CNAME, MX, and TXT records.
- Manual DNS changes appear in sync preview and apply flows.
- Generated DNS records do not silently overwrite or remove user-owned manual records.
- Tests cover CRUD, ownership, validation, conflict preview, and confirmed replacement behavior.
