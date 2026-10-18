# AUD-077: Scoped manual blocks leak into global subject state

Priority: P1

Category: Security / scope isolation

Status: open; confirmed by manual source tracing.

## Finding

CreateManualEntryAsync accepts resource/root-domain/connection/host scope but always calls UpsertSubjectStateAsync without scope. A timed block writes global SoftBlockedUntilUtc. DecideForwardAuthAsync checks scoped entries first, then applies that global soft-block state even when the current resource is outside the entry scope. RootDomain scope matching additionally accepts scopeId==configured RootDomain without checking the request host. Connection/host scope strings are accepted for storage but not implemented by MatchesScope.

## Evidence

Paths below are relative to src/Hashi.Infrastructure/:

Platform/SecuritySubjectOperationsService.cs:189-215,559-586,827-835; Platform/SecurityDecisionService.cs:178-188,678-700

## Expected outcome

Keep scoped enforcement in scoped state/evaluation; implement and validate each advertised scope and bind root-domain matches to the actual request host.

## Acceptance criteria

A block scoped to A does not block B; configured root scope does not affect unrelated custom domains; connection/host scopes work predictably.

No live service configuration or firewall was changed to establish this finding.
