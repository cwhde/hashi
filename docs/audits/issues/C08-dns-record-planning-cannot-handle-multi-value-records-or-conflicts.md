# C08 - DNS record planning cannot handle multi-value records or conflicts

Priority: High

Spec conflicts: section 15.2 requires manual A, AAAA, CNAME, MX, and TXT records. Section 15 requires ownership-aware DNS management and safe plans that do not silently overwrite or drop user-owned records.

## Problem

DNS planning keys records only by name and type. That prevents common valid DNS shapes such as multiple MX or TXT records at the same name. The manual record validator rejects same-zone/name/type duplicates regardless of value, and `DnsPlanner` uses `ToDictionary` with the same key, so duplicate provider or desired records can throw or collapse before a safe plan can be produced.

The desired-state merge also silently skips generated records whenever any manual/imported record has the same name, even if the type/value differs. That hides resource DNS output instead of surfacing a sync conflict that the user can resolve.

## Evidence

- `docs/implementation-spec/hashi-v2-implementation-spec.md:800-812` defines the DNS provider interface and plan support.
- `docs/implementation-spec/hashi-v2-implementation-spec.md:825-832` requires manual MX and TXT support, which often needs multiple records at the same name.
- `docs/implementation-spec/hashi-v2-implementation-spec.md:847-865` requires ownership-aware resource DNS behavior.
- `src/Hashi.Infrastructure/Dns/DnsRecordService.cs:176-187` rejects any enabled duplicate with the same zone/name/type.
- `src/Hashi.Core/Dns/DnsPlanner.cs:12-13` converts current and desired records to dictionaries keyed by name/type.
- `src/Hashi.Core/Dns/DnsPlanner.cs:105-106` defines that key as only `Name|Type`.
- `src/Hashi.Infrastructure/Dns/DnsDesiredStateBuilder.cs:121-126` skips every generated record whose name appears in preserved manual names.
- `tests/Hashi.UnitTests/DnsRecordServiceTests.cs:87-99` asserts that duplicate same-name/type manual records are rejected, which locks in the bad behavior for MX/TXT.
- `tests/Hashi.UnitTests/DnsRecordServiceTests.cs:56-85` asserts that a generated resource record with the same name as a manual record is omitted rather than planned as a conflict.

## Expected outcome

Hashi should support valid multi-value DNS records and should surface manual/generated conflicts in sync previews rather than silently dropping desired generated records.

## Fix guidance

Use a stable identity that includes provider id or value where DNS types permit multi-value records. Add type-aware validation: CNAME can remain single-record, but MX/TXT and other multi-value-capable records should allow multiple values. Update desired-state merge and planning to emit explicit conflict/no-op changes for same-name collisions instead of hiding generated output.

## Acceptance criteria

- Users can create multiple MX records at the same name with different values.
- Users can create multiple TXT records at the same name with different values.
- Provider records with duplicate name/type do not crash planning.
- Generated resource DNS that conflicts with a manual/imported name appears as a visible plan conflict.
- Tests cover multi-value MX/TXT, duplicate provider records, and manual/generated conflict previews.
