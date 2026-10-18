# AUD-068: CAPTCHA outages count as user failures and trigger blocks

Priority: P2

Category: Security / CAPTCHA outage handling

Status: open; confirmed by manual source tracing.

## Finding

VerifyChallengeAsync routes every unsuccessful CapVerifyResult, including unavailable (timeouts, server errors and missing service key), through RecordFailedSolveAsync. That increments FailedChallengeCount and TotalOffenseCount and soft-blocks after the failure threshold. An unavailable verifier thus penalizes legitimate users for infrastructure failure.

## Evidence

src/Hashi.Infrastructure/Platform/CaptchaChallengeService.cs:254-268,394-405,491-525

## Expected outcome

Record verifier outages separately without incrementing user failure/offense counters; apply the configured availability policy explicitly.

## Acceptance criteria

Repeated verifier timeouts/5xx do not escalate a user ban. Invalid proof still increments appropriate failure counters. Hard blocks remain enforced.

No live provider state was modified to establish this finding.
