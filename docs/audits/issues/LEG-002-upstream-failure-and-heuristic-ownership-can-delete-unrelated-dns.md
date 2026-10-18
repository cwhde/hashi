# LEG-002: Upstream failure and heuristic ownership can delete unrelated DNS

Priority: P2

Category: Legacy provider safety

Status: open; manual source review. Applies to the separately deployed legacy artifact, not the active V2 process.

## Finding

Pangolin API errors are converted into empty organization/resource/target lists. Synchronization then constructs an empty desired mapping and deletes current CNAME records whose values start with on. or contain .on., using that pattern as ownership. A temporary upstream failure or legitimate unrelated CNAME can therefore trigger remote deletion without a reviewed plan. Empty/malformed successful topology TXT results also replace the last good cache. Provider mutation false results are ignored and the cycle can still report success.

## Evidence

hashi.old/src/core/pangolin.js catch/empty returns; src/core/sync.js:355-427,runOnce; src/core/topology.js cache replacement; src/core/hetzner.js mutation error handling.

## Expected outcome and acceptance criteria

Keep failures distinct from valid empty inventories, preserve the last good topology, establish explicit ownership and review destructive changes. Propagate remote operation failure and verify the resulting provider state.
