# AUD-061: Generated DNS records lose ownership after creation

Priority: P1

Category: DNS lifecycle / ownership

Status: open; confirmed by manual source tracing.

## Finding

Generated resource and host records have no corresponding DnsRecordEntity. PersistCreatedProviderRecordAsync returns without persisting their provider IDs. Hetzner ListRecordsAsync always returns IsManagedByHashi=false; BuildCurrentSnapshot only restores ownership from enabled DnsRecords. On the next reconcile, these generated records become unowned. Their subsequent updates/deletions cannot be managed safely. The in-memory provider retains true on listed records and masks this production difference.

## Evidence

src/Hashi.Infrastructure/Dns/DnsConnectionService.cs:408-426,446-470; src/Hashi.Infrastructure/Dns/DnsDesiredStateBuilder.cs:40-112; src/Hashi.Infrastructure/Providers/Dns/HetznerDnsProvider.cs:23-37; src/Hashi.Infrastructure/Providers/Dns/InMemoryDnsProvider.cs:23-41

## Expected outcome

Persist workflow ownership and provider identity independently of manual desired records, and retain it across list/restart.

## Acceptance criteria

After creating a generated record with a provider that never tags ownership, a fresh read can update and later delete exactly that record; unrelated records remain untouched.

No live provider state was modified to establish this finding.
