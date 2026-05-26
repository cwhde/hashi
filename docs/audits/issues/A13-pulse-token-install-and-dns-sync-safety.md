# A13 - Pulse install/token and DNS sync behavior are not safe enough

Priority: Medium

Spec conflicts: non-negotiable rules 5, 20, and 24; Pulse section 16 and DNS sync section 24.

## Problem

Pulse agent tokens are shown once on create/rotate, but the install endpoint also accepts `token` as a query parameter and then embeds it into generated shell/docker commands. Query-string tokens are likely to appear in browser history, proxy logs, access logs, and support screenshots.

Heartbeats carry the token in a JSON body rather than an authorization header. That is workable, but combined with install-query tokens it weakens the "no tokens in logs" rule.

When Pulse detects an IP change, it immediately applies DNS changes through `DnsConnectionService.ApplySafePlanAsync`. That bypasses the endpoint plan preview and does not clearly record high-risk pending plans. The spec says routine sync can continue, but high-risk passive sync plans must wait for approval and every external adapter must support plan/apply/reconcile.

## Evidence

- `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs:531-535` accepts `token` in the install endpoint query.
- `src/Hashi.Infrastructure/Platform/PulseInstallRenderer.cs:7-24` embeds the token into shell and Docker commands.
- `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs:553-566` accepts heartbeat token in request body.
- `src/Hashi.Infrastructure/Platform/PulseAgentService.cs:78-88` applies DNS immediately on IP change.
- `src/Hashi.Infrastructure/Platform/PulseAgentService.cs:150-170` records a sync run summary but does not expose a plan approval path.

## Expected outcome

Pulse tokens should not be passed in URLs. Install flow should use a one-time visible token or copy command generated client-side without sending the token back through query strings. Pulse DNS changes should go through DNS plan/apply/reconcile rules, with high-risk changes recorded as pending for approval.

## Fix guidance

Remove token query support from the install endpoint. Generate install commands from the create/rotate response client-side, or use a short-lived one-time install code. Route Pulse DNS changes through the sync planner and classify safe versus high-risk changes.

## Acceptance criteria

- No Pulse endpoint accepts token in the URL.
- Install command generation does not create token-bearing URLs.
- Pulse IP changes create/apply only safe DNS plans and record high-risk changes as pending.
- Tests verify token redaction and DNS plan behavior.
