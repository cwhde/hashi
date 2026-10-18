# AUD-171: Internal DNS domain validation accepts empty labels and oversized names

Priority: P2

Category: DNS / validation

Status: open; confirmed by manual helper/caller trace.

## Finding and evidence

InternalAgentDnsName.NormalizeDomain splits labels with RemoveEmptyEntries but returns the original normalized string. A leading dot such as `.hashi.home.arpa` passes the nonempty-label checks and is stored unchanged; combining the agent label produces `agent..hashi.home.arpa`. It checks individual labels but not the complete domain plus agent-label DNS length, allowing oversized FQDNs. Accepted settings then fail actual AdGuard/DNS use instead of being rejected during editing.

Evidence: `src/Hashi.Core/Dns/InternalAgentDnsName.cs` NormalizeDomain/BuildFqdn; `src/Hashi.Infrastructure/Platform/InternalAgentDnsSettingsService.cs:29-30`; `AdGuardSyncService.cs:506-507`.

## Expected outcome and acceptance criteria

Reject leading/empty labels, enforce the complete encoded DNS name limit after adding the agent label and use one canonical validation path for settings and generation. Boundary-length valid names must work; invalid inputs must leave saved settings unchanged.
