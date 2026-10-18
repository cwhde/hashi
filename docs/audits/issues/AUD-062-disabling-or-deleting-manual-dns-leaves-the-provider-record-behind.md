# AUD-062: Disabling or deleting manual DNS leaves the provider record behind

Priority: P1

Category: DNS lifecycle / deletion

Status: open; confirmed by manual source tracing.

## Finding

DeleteManualAsync immediately removes both DnsRecords and DnsRecordOwnership. Disabling excludes the row from desired state AND the provider ownership query. Hetzner lists all records as unowned. DnsPlanner therefore cannot schedule an owned deletion after either mutation; the public DNS record remains live. Preserve a tombstone until a confirmed remote deletion succeeds.

## Evidence

src/Hashi.Infrastructure/Dns/DnsRecordService.cs:101-107,146-160; src/Hashi.Infrastructure/Dns/DnsConnectionService.cs:415-425; src/Hashi.Infrastructure/Dns/DnsDesiredStateBuilder.cs:20-22

## Expected outcome

Keep last applied ownership/identity until remote removal is confirmed.

## Acceptance criteria

Disable/delete a previously applied record; preview includes that exact provider ID and confirmed apply removes it. Retry after provider failure retains ownership and is safe.

No live provider state was modified to establish this finding.
