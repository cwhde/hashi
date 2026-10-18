# AUD-070: Any edge SSO provider session satisfies every resource

Priority: P1

Category: Security / SSO provider isolation

Status: open; confirmed by manual source tracing.

## Finding

BuildContextAsync validates the edge session without passing the required resource provider. ValidateSessionAsync checks only token existence and time, never OidcProviderId or whether that provider remains enabled. Both Sso and DefaultResourceDecision allow immediately on this boolean. A login to provider A can satisfy a resource assigned to provider B, and disabling A does not invalidate its existing sessions.

## Evidence

Paths below are relative to src/Hashi.Infrastructure/:

Platform/SecurityDecisionService.cs:296-313,432-435,455-458; Platform/OidcEdgeAuthService.cs:125-139,142-167

## Expected outcome

Bind validation to the resolved resource/default provider and current enabled state; define and enforce intentional cross-provider trust explicitly.

## Acceptance criteria

An A session cannot authorize B-only resources. Disabled/revoked provider sessions stop authorizing resources immediately.

No live service configuration or firewall was changed to establish this finding.
