# AUD-151: Internal DNS settings can save before collision checks or validate stale enabled rows

Priority: P2

Category: Persistence / internal DNS validation

Status: open; confirmed by manual source trace.

## Finding

UpdateAsync modifies the tracked global settings then calls EnsureAgentSettingsAsync, which invokes SaveChanges when any agent lacks its settings row. Those global changes can commit before collision validation throws. EnsureNoCollisionsAsync also filters Enabled in the database before inspecting tracked per-agent changes: newly enabled colliding rows may be absent from its query and newly disabled rows may still be included. The endpoint can reject after partial persistence or accept a collision that later breaks synchronization.

## Evidence

src/Hashi.Infrastructure/Platform/InternalAgentDnsSettingsService.cs:22-76,138-185.

## Expected outcome and acceptance criteria

Validate the complete proposed in-memory state before committing and save it transactionally, with a database invariant where possible. A failed update must leave old settings unchanged; enable/disable/name changes in one request must use the proposed state.
