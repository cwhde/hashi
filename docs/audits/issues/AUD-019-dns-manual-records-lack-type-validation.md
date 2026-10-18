# AUD-019: Manual DNS records are not validated by type or DNS constraints

Priority: P2 — DNS feature correctness

Category: Missing validation / invalid desired state

Spec reference: implementation spec §§8.1 and 24 require safe manual DNS CRUD and provider-aware DNS planning.

## Finding

`DnsRecordService.ValidateAsync` checks that the zone exists, the type is one of five supported values, the name and value are nonempty, TTL is positive, and exact local duplicates are absent. It does not validate record values by type, ensure the owner name is valid/in-scope for the selected zone, or enforce DNS constraints such as a CNAME not coexisting with other data at the same owner name. Invalid values and conflicting records are therefore persisted as desired state and fail later at provider sync, or can generate an invalid zone state.

## Evidence

- `src/Hashi.Infrastructure/Dns/DnsRecordService.cs`, `ValidateAsync`: record values are passed through `NormalizeRequired` without A/AAAA IP parsing, MX/TXT/CNAME validation, zone-name validation, or CNAME conflict checks.
- The duplicate query only checks same name/type (or exact same value for MX/TXT); it does not detect cross-type conflicts at a CNAME owner name.
- `DnsDesiredStateBuilder` combines manually configured and generated records before planning, so local and generated records can also form a conflicting owner-name set.
- `src/Hashi.Core/Dns/DnsRecordGenerator.cs`: generated host A records use `PublicIp` without parsing it; `IsPublicIp` filters selected private ranges but accepts other non-global special addresses.

## Expected outcome

Validate DNS syntax and cross-record constraints at the desired-state boundary. Validate generated records too, and surface conflicts before provider apply with the conflicting local source and record details.

## Acceptance criteria

- A records accept only IPv4 addresses and AAAA records only IPv6 addresses.
- Generated A/AAAA records are valid addresses and reject non-global/special-purpose ranges as well as private ranges.
- CNAME targets and owner names are normalized and validated; CNAME exclusivity is checked across all desired records for the owner name.
- MX preference/exchange syntax and TXT representation follow the provider's accepted model.
- Record owner names are confirmed to belong to the selected zone, with a documented representation for apex records.
- Manual/generated conflicts are shown in the plan and cannot become silent provider failures.
