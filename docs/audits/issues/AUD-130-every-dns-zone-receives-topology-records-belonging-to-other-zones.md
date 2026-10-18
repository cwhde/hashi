# AUD-130: Every DNS zone receives topology records belonging to other zones

Priority: P1

Category: DNS / zone isolation

Status: open; confirmed by manual source trace.

## Finding

DnsDesiredStateBuilder filters manual rows by zoneId but builds generated host/resource records for the entire installation without looking up the requested zone name or filtering generated FQDNs to that zone. Global sync invokes it for each enabled zone. A multi-zone account therefore plans/writes root-domain and custom-domain records into unrelated zones; a strict provider rejects the apply, while relative-name adapters can create unintended names. A resource in another domain can prevent otherwise unrelated zone synchronization.

## Evidence

src/Hashi.Infrastructure/Dns/DnsDesiredStateBuilder.cs:20-112; DnsConnectionService.cs:259; global DNS zone loop in SyncOrchestratorService. Main specification15 supports zones and explicit resource domains.

## Expected outcome and acceptance criteria

Resolve each generated FQDN to its configured authoritative managed zone with label-boundary handling, and produce only that zone's desired records. Two managed zones plus a custom-domain resource must yield disjoint correct plans.
