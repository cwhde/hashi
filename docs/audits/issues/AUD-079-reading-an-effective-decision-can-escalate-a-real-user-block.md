# AUD-079: Reading an effective decision can escalate a real user block

Priority: P2

Category: Security / preview side effects

Status: open; confirmed by manual source tracing.

## Finding

GetEffectiveDecisionAsync invokes the live DecideForwardAuthAsync for a synthetic host. For challenged subjects that method increments RequestsWhileChallenged, writes security events and may escalate soft/firewall blocks via CountProtectedHitWhileChallengedAsync. Repeated admin GETs/refreshes therefore mutate the inspected subject and can block them without protected traffic. This also exposes state-changing behavior through a read endpoint.

## Evidence

Paths below are relative to src/Hashi.Infrastructure/:

Platform/SecuritySubjectOperationsService.cs:150-186; Platform/SecurityDecisionService.cs:221-246; Platform/CaptchaChallengeService.cs:309-356

## Expected outcome

Separate pure decision explanation from recording/enforcement side effects and provide real selectable resource context.

## Acceptance criteria

Repeated effective-decision GETs leave every counter, block duration, event and session unchanged while matching actual decision semantics.

No live service configuration or firewall was changed to establish this finding.
