# AUD-030: Forward-auth materializes all active security records per request

Priority: P3 — hot-path database and allocation cost grows with total configured state

Category: Inefficiency / scalability

Spec reference: implementation addendum §5 requires the effective decision to remain suitable for incident-response and runtime use; implementation spec §§11 and 12 require request-time forward-auth and policy evaluation.

## Finding

For each forward-auth request, `SecurityDecisionService` loads every enabled resource to identify the request host, every active manual security entry, every enabled firewall subject block, and then all applicable active blocklist entries into application memory before checking whether they match this one request. This makes request cost and transferred row volume grow with the full configured security dataset, even when almost all records concern unrelated subjects or resources. Under traffic, this repeats the same broad reads for every request and can dominate database work as entries grow.

## Evidence

- `src/Hashi.Infrastructure/Platform/SecurityDecisionService.cs`: `BuildContextAsync` calls `ToListAsync` for all enabled resources and performs host matching in memory.
- `LoadMatchingManualEntriesAsync` filters only by enabled and expiry in SQL, then materializes all entries and applies scope/subject matching in memory.
- `LoadMatchingFirewallSubjectBlockAsync` materializes all enabled firewall subject blocks before matching.
- `LoadMatchingBlocklistEntryAsync` filters source/enforcement/expiry in SQL but materializes all remaining active entries before matching by IP/CIDR/ASN/country/region.

## Expected outcome

Reduce repeated full-table reads on the request path. Use normalized keys, indexed lookup tables, provider-side predicates for straightforward match types, and/or a bounded cache with explicit invalidation for policy data. Keep matching semantics and deterministic precedence identical.

## Acceptance criteria

- A request for one subject does not transfer all unrelated manual and blocklist entries from the database.
- Host-to-resource resolution is indexed or otherwise bounded without loading every enabled resource per request.
- Query plans use appropriate indexes for common IP and resource lookups, and broad match types have documented bounded behavior.
- Create/update/delete/expiry of policy records becomes visible within a defined short interval; stale allow/block decisions cannot persist beyond that interval.
- Decision precedence and matching outcomes remain consistent across IP, CIDR, ASN, country, region, global, root-domain, and resource-scoped rules.
