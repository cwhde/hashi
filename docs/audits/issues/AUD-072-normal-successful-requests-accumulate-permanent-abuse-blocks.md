# AUD-072: Normal successful requests accumulate permanent abuse blocks

Priority: P1

Category: Security / adaptive abuse detection

Status: open; confirmed by manual source tracing.

## Finding

IngestAccessLogAsync adds one score for every successful request and two for every4xx/5xx. The per-IP score is lifetime accumulated; no window or decay is applied here. At10 points the user is challenged,15 soft blocked,20 a global automatic blocklist entry is created without expiry and a firewall sync attempted. Ordinary page assets or long-term legitimate use therefore eventually trigger indefinite blocks. Configured ban/window policies are not used in this path.

## Evidence

Paths below are relative to src/Hashi.Infrastructure/:

Platform/SecurityIngestionService.cs:37-79,171-215,1084-1103

## Expected outcome

Base abuse decisions on validated suspicious signals and configured windows/decay/escalation durations; preserve ordinary successful traffic and historical counters independently.

## Acceptance criteria

Normal successful browsing over many requests/days never creates an abuse ban. Suspicious traffic escalates within configured windows and expires according to policy.

No live service configuration or firewall was changed to establish this finding.
