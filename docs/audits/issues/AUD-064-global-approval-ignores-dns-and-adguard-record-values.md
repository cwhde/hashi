# AUD-064: Global approval ignores DNS and AdGuard record values

Priority: P1

Category: Sync / approval integrity

Status: open; confirmed by manual source tracing.

## Finding

Global diffs reduce DNS changes to name/type/kind/risk reason and AdGuard changes to domain/kind/fixed summary. PlansMatch compares only those fields and risk level. Apply regenerates plans, so a changed desired destination or TXT token/TTL with the same change shape can pass comparison and be applied under approval of a different value. Per-provider deterministic plan hashes do include more state but are not bound to the approved global plan.

## Evidence

src/Hashi.Infrastructure/Sync/SyncOrchestratorService.cs:647-659,898-902; src/Hashi.Infrastructure/Platform/AdGuardSyncService.cs:258-262,287-292

## Expected outcome

Bind global approval to canonical exact provider plans, including before/after values, TTL, IDs, target connection and deletions.

## Acceptance criteria

Change only a DNS value/TTL or AdGuard answer between global preview and apply; apply rejects stale approval and requires a new preview.

No live provider state was modified to establish this finding.
