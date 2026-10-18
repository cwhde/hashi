# AUD-086: Pulse cannot discover the configured NetBird overlay addresses

Priority: P1

Category: Pulse / NetBird discovery

Status: open; confirmed by manual source tracing.

## Finding

The shipped firewall defaults use100.110.0.0/16, but the Go agent accepts IPv4 candidates only when net.IP.IsPrivate is true, which excludes shared100.64.0.0/10. The server repeats an RFC1918-only check. Selecting wt0 on the default NetBird range therefore produces no selected candidate, breaking private agent-bound SSH/DNS targets or selecting another interface unexpectedly.

## Evidence

agents/pulse/cmd/hashi-pulse/main.go:183-193; src/Hashi.Infrastructure/Platform/PulseAgentService.cs:237-276,282-299; src/Hashi.Infrastructure/Persistence/Entities/ExtendedPlatformEntities.cs:64-70

## Expected outcome

Support explicitly configured overlay/private target ranges consistently on agent and server without treating arbitrary public addresses as private.

## Acceptance criteria

An agent on100.110.x.x reports/selects wt0 and resolves agent-bound private SSH/internal DNS correctly; public/private modes remain distinct.

No live credentials, host scripts or services were changed for this review.
