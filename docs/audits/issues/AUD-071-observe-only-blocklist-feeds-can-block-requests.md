# AUD-071: Observe-only blocklist feeds can block requests

Priority: P1

Category: Security / blocklist enforcement modes

Status: open; confirmed by manual source tracing.

## Finding

ToEntity sets SyncedToFirewall=true for any source that does not require firewall sync, including observe and middleware feeds. The first blocklist decision query treats SyncedToFirewall OR enforcement=firewall as a hard-block match, before public CAPTCHA bypass. Thus observe entries become denies instead of telemetry. The separate explicit SyncBlocklistToAllFirewallsAsync also selects all enabled IP/CIDR entries without filtering enforcement mode and marks them synced, compounding inaccurate enforcement.

## Evidence

Paths below are relative to src/Hashi.Infrastructure/:

Platform/BlocklistSourceManagementService.cs:330-348,364-373; Platform/SecurityDecisionService.cs:140-159,614-640; Platform/SecurityIngestionService.cs:788-831

## Expected outcome

Represent not-applicable separately from applied and enforce only explicitly configured modes throughout render, sync, middleware and reporting.

## Acceptance criteria

Observe feeds never deny or enter firewall rules; middleware feeds do not become hard firewall blocks; switching modes reconciles prior enforcement correctly.

No live service configuration or firewall was changed to establish this finding.
