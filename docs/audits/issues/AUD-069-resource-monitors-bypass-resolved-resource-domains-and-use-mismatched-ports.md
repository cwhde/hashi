# AUD-069: Resource monitors bypass resolved resource domains and use mismatched ports

Priority: P2

Category: Monitoring / target correctness

Status: open; confirmed by manual source tracing.

## Finding

BuildResourceMonitorUrl uses raw resource.Domain or TargetHost instead of ResourceDomainResolver. Root/subdomain modes can therefore monitor the upstream/default root rather than the generated public route. TCP/UDP URLs pair TargetHost with PublicPort, mixing the upstream host and proxy entry-point port; upstream3000/public8443 probes upstream8443. IPv6 literals are also not bracketed.

## Evidence

src/Hashi.Infrastructure/Platform/MonitoringService.cs:20-32,597-613

## Expected outcome

Choose explicit public-route versus upstream check semantics and consistently resolve the matching host, port, scheme and path.

## Acceptance criteria

Root/subdomain/custom resources monitor their intended endpoint; differing upstream/public ports and IPv6 produce valid correct URLs.

No live provider state was modified to establish this finding.
