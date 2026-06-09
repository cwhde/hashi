# H-060: CAPTCHA Solve Erases Offense History — Repeat Offenders Can Clear Track Record

**Priority:** High
**Conflict Type:** wrong_implementation
**Spec Reference:** Addendum §6.2 (Do not reset offense history after a successful CAPTCHA solve); §7.4 (Hashi does not clear offense history; Hashi does not bypass SSO)

## Description

`CaptchaChallengeService.ClearChallengeAfterSolveAsync()` resets `RequestsWhileChallenged = 0` and `FailedChallengeCount = 0`, and optionally fully resets or decays bucket challenge counters. The spec explicitly requires that CAPTCHA solve does NOT erase offense history, including soft block count, firewall block count, manual block history, security event history, and repeat-offender counters. Resetting these metrics means repeat offenders can clear their track record with each CAPTCHA solve, defeating the escalation system.

## Evidence

CaptchaChallengeService.ClearChallengeAfterSolveAsync() — `state.RequestsWhileChallenged = 0; state.FailedChallengeCount = 0;` and bucket reset/decay logic. These are offense history metrics that should be preserved per spec.

## Expected Outcome

CAPTCHA solve clears only the active challenge state. RequestsWhileChallenged, FailedChallengeCount, and offense counters are preserved. Only the triggering rate buckets may be reset/decayed.

## Fix Guidance

(1) Remove the resets of RequestsWhileChallenged and FailedChallengeCount from ClearChallengeAfterSolveAsync. (2) Add separate fields for "current challenge" metrics vs "lifetime offense" metrics. (3) Only reset/decay the rate buckets that triggered the challenge, not the cumulative offense counters.

## Acceptance Criteria

- [ ] After CAPTCHA solve, challenge_required is cleared but offense counters are preserved
- [ ] Repeat offenders accumulate offense history across multiple CAPTCHA solves
- [ ] Ban duration policy correctly uses cumulative offense count
