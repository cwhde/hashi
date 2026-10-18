# AUD-136: Agent DNS stale retention options never inspect agent age

Priority: P2

Category: Wrong implementation / stale agent DNS

Status: open; confirmed by manual source trace.

## Finding

SyncInternalAgentDnsRewritesAsync checks KeepLastRewriteWhenAgentStale only when the selected answer is empty. An offline agent normally retains LastPrivateIp/LastPublicIp, so the code continues to desire its old rewrite regardless of its heartbeat age or a false keep-last setting. The option does not implement its advertised stale-agent behavior.

## Evidence

src/Hashi.Infrastructure/Platform/AdGuardSyncService.cs:492-548,596-607. Addendum10.6 requires explicit stale behavior rather than silent answer replacement/deletion.

## Expected outcome and acceptance criteria

Define staleness from the configured heartbeat threshold, evaluate it separately from missing address data, and retain/remove only according to both global and per-agent settings with normal destructive planning. Test stale-but-addressed and fresh-but-unresolved agents.
