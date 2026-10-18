# AUD-131: Configured root domains bypass default TTL fallback for manual records

Priority: P2

Category: DNS / default TTL semantics

Status: open; confirmed by manual source trace.

## Finding

BuildAsync initially maps null manual TTL to the zone default. When RootDomain is configured it discards that list and calls MergeRecords on entities, where TTL is copied unchanged as null. Provider create defaults null to3600, while VerifyRemoteApply requires exact equality with the plan null. An otherwise successful write is then reported failed, and the zone's configured default may be ignored. The no-root-domain path behaves differently.

## Evidence

src/Hashi.Infrastructure/Dns/DnsDesiredStateBuilder.cs:23-37,112-130; Providers/Dns/HetznerDnsProvider.cs:45-60; DnsConnectionService.cs:595-602.

## Expected outcome and acceptance criteria

Normalize effective TTL once for all desired records and compare the effective provider semantics. A null-TTL manual record must use its zone default consistently with and without a configured root domain and verify successfully.
