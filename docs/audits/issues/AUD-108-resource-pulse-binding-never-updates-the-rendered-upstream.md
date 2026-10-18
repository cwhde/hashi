# AUD-108: Resource Pulse binding never updates the rendered upstream

Priority: P1

Category: Missing feature / Pulse resource targets

Status: open; confirmed by manual source trace.

## Finding

Resource CRUD stores PulseAgentId, but BuildDefinitionsAsync constructs the upstream solely from the persisted TargetHost/TargetPort. Heartbeats refresh ConnectionTargets and DNS, not Resource.TargetHost or a resource-target resolution used by the renderer. A resource bound to an agent therefore continues to proxy to its old literal address after the agent changes IP. DNS rewrites can change while the actual upstream remains stale.

## Evidence

src/Hashi.Infrastructure/Platform/PlatformServices.cs:46-76,803-874; PulseAgentService.cs:155-159; ConnectionTargetResolver.cs:74-93. Main specification17.4 requires agent-backed resource targets to resolve automatically.

## Expected outcome and acceptance criteria

Resolve agent-backed resource upstreams from the configured address selector at render time, record dependent Traefik changes on heartbeat, and keep explicit static overrides intentional. A synthetic address change must update the approved rendered upstream and DNS consistently.
