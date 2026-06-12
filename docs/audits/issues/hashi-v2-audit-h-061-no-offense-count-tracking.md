# H-061: No Offense Count Tracking — Ban Duration Escalation Policies Cannot Function

**Priority:** High
**Conflict Type:** missing_implementation
**Spec Reference:** Addendum §6.2 (Offense count must be tracked separately from active block state)

**Status:** Fixed
**Branch:** h/security-2

## Description

`SecuritySubjectStateEntity` has no `OffenseCount` field. The `BanDurationPolicyEvaluator` accepts `offenseCount` as a parameter but nothing in the infrastructure tracks or persists it. Offense count is never incremented when a ban is applied or a challenge is failed. Without offense count tracking, the ban duration escalation policies (linear, exponential, capped exponential, permanent after N) cannot function correctly — they would always use offenseCount=0 or offenseCount=1.

## Evidence

SecuritySubjectStateEntity has no OffenseCount, TotalChallengesReceived, TotalBlocksReceived, or similar cumulative counter. BanDurationPolicyEvaluator.EvaluateAsync() accepts offenseCount but the callers either pass 0 or 1 without historical accumulation.

## Expected Outcome

Each security subject tracks a cumulative offense count that increments with each new offense (challenge triggered, soft block applied, etc.). This count is tracked separately from the active block state and persists across challenge solves.

## Fix Guidance

Add OffenseCount, FirstOffenseAtUtc, LastOffenseAtUtc, and TotalBlockCount fields to SecuritySubjectStateEntity. Increment these in the escalation paths of SecurityIngestionService and SecurityDecisionService. Pass the persisted offense count to BanDurationPolicyEvaluator.

## Acceptance Criteria

- [ ] SecuritySubjectStateEntity tracks cumulative offense count
- [ ] Offense count increments on each new challenge/block event
- [ ] Ban duration policies use the correct historical offense count
- [ ] Offense count persists across CAPTCHA solves
