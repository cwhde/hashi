# AUD-122: Successful CAPTCHA solves can immediately challenge the same user again

Priority: P1

Category: Wrong implementation / CAPTCHA recovery

Status: open; confirmed by manual source trace.

## Finding

ClearChallengeAfterSolveAsync clears the normalized subject flag but never resets/decays the legacy AbuseBucket score or State. SecurityDecisionService still evaluates that legacy state and IsChallengeRequired, so an abuse-score-triggered challenge can recur immediately after a valid solve. A require_challenge resource rule also unconditionally calls MarkChallengeRequiredAsync on the next matching request without consulting LastChallengeSolvedAtUtc or the configured repeat interval. The reset/decay implementation instead rewrites historical SecurityRequestBucket challenge counts, corrupting analytics while leaving the actual legacy trigger intact.

## Evidence

src/Hashi.Infrastructure/Platform/CaptchaChallengeService.cs:271-307,425-488; SecurityDecisionService.cs:201-225,348-363; SecurityIngestionService.cs:61-78. AUD-025 separately covers using the repeat interval as an expiry lifetime; AUD-072 covers lifetime benign-traffic abuse scoring.

## Expected outcome and acceptance criteria

Unify active enforcement state and score reset/decay semantics, honor repeat-challenge cooldown and preserve historical event/bucket counts. After an independent successful solve, the same protected request must pass during its intended cooldown; cumulative offense history must remain intact.
