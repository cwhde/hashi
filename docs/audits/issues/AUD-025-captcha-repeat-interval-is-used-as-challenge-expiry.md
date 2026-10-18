# AUD-025: CAPTCHA repeat interval is unused for repeats and repurposed as expiry

Priority: P2 — security policy behavior mismatch

Category: Wrong setting implementation / challenge lifecycle

Spec reference: implementation addendum §§13.5 and 14; the challenge cleanup job may reset challenge-required state after a configured maximum duration, while repeat-challenge policy must remain distinct.

## Finding

The `MinimumRepeatChallengeSeconds` setting is only read by `SecurityMaintenanceService.CleanupStaleChallengesAsync`, where it becomes the lifetime after which an active challenge is cleared (`max(3600, configured value)`). `CaptchaChallengeService.MarkChallengeRequiredAsync` never checks this setting or `LastChallengeSolvedAtUtc`, so a subject can be challenged again immediately after solving even when the configured minimum repeat interval is longer. Conversely, changing the repeat interval changes how long the current challenge remains required, up to 24 hours, despite that field not being a maximum challenge duration.

## Evidence

- `src/Hashi.Infrastructure/Platform/SecurityAddendumJobWorker.cs`: `CleanupStaleChallengesAsync` calculates `staleSeconds` from `MinimumRepeatChallengeSeconds` and clears challenge state after that duration.
- `src/Hashi.Infrastructure/Platform/CaptchaChallengeService.cs`: `MarkChallengeRequiredAsync` sets challenge state without consulting the repeat interval or last solve timestamp.
- Repository search finds no other runtime use of `MinimumRepeatChallengeSeconds`.
- The implementation addendum's Challenge Cleanup Job requirement distinguishes a configured maximum challenge duration from repeat challenge behavior.

## Expected outcome

Enforce the minimum repeat interval when deciding whether to challenge a recently verified subject. Use a separate, explicitly named maximum challenge duration for stale-state cleanup, and only auto-clear when that policy is configured.

## Acceptance criteria

- A successful solve prevents another challenge for the configured minimum interval unless a stronger block/security policy applies.
- The minimum repeat interval does not control expiry of an already active challenge.
- Challenge-required state is only auto-cleared after a separately configured maximum duration; absent that setting, cleanup follows an explicit safe policy.
- User-visible settings and API descriptions match the implemented behavior.
