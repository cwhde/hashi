# AUD-063: DNS rename, type change and zone move reuse the old provider identity

Priority: P1

Category: DNS lifecycle / mutation

Status: open; confirmed by manual source tracing.

## Finding

UpdateManualAsync changes ZoneId, Name and Type without resetting or retiring ProviderRecordId. Apply update resolves by that ID and calls an adapter method that only changes value and TTL. Rename/type edits can mutate the old record instead of creating the new one and then fail verification; zone moves can leave the old record orphaned.

## Evidence

src/Hashi.Infrastructure/Dns/DnsRecordService.cs:101-112; src/Hashi.Infrastructure/Dns/DnsConnectionService.cs:305-316,429-443; src/Hashi.Infrastructure/Providers/Dns/HetznerDnsProvider.cs:63-80

## Expected outcome

Model identity-changing edits as an explicit create/delete migration with retained old ownership and an exact preview.

## Acceptance criteria

Rename, record-type change and cross-zone move produce the correct remote identity and remove old owned records only after confirmation.

No live provider state was modified to establish this finding.
