# AUD-020: DNS CRUD and planner collapse multi-value RRsets

Priority: P2 — DNS feature completeness and reconciliation correctness

Category: Missing feature / incorrect record reconciliation

Spec reference: implementation spec §§8.1, 8.3 and 24 require zone record discovery, CRUD, and safe desired-state reconciliation.

## Finding

The DNS model represents provider records individually, but `DnsPlanner` treats only MX and TXT as multi-value types. For A, AAAA, and NS records it indexes only one row per name/type and keys desired values without the value. `DnsRecordService.ValidateAsync` likewise prevents a second A/AAAA/NS value at the same owner name. Providers can hold multiple values in those RRsets, so the UI cannot represent them and planner indexing silently drops all but one current/desired value. The current Hetzner replacement API also operates on RRsets, which makes the one-record-per-owner/type assumption especially incompatible.

## Evidence

- `src/Hashi.Core/Dns/DnsPlanner.cs`: `BuildIndex` stores one record per `Key`; `Key` appends the value only for `IsMultiValue`, which returns true only for MX and TXT.
- `src/Hashi.Infrastructure/Dns/DnsRecordService.cs`: duplicate validation requires unique name/type for A, AAAA, and other non-MX/TXT types.
- Hetzner's current DNS API reference documents RRSet operations, where one RRSet contains multiple records for a name/type.

## Expected outcome

Represent and reconcile the complete value set for each RRSet. Support the provider's multi-value types and preserve single-value constraints (notably CNAME and SOA) explicitly.

## Acceptance criteria

- Multiple A, AAAA, MX, TXT, and NS values at a single owner name can be listed, planned, and safely applied where supported.
- The planner compares full RRSet value sets and TTL, rather than selecting one arbitrary record.
- Duplicate values are deduplicated deterministically; distinct values are preserved.
- CNAME exclusivity and provider limitations are enforced separately from RRSet multi-value behavior.
