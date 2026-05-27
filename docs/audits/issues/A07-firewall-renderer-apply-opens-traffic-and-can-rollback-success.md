# A07 - Firewall renderer/apply can open all public traffic and rollback successful applies

Priority: Critical

Spec conflicts: non-negotiable rules 7, 18, 19, and 20; firewall sections 12 and 24.

## Problem

The generated firewall script schedules rollback but never cancels or disarms it after a successful apply. If a rollback script exists, a successful apply can be reverted later by the timer.

The script also accepts all inbound traffic to the public IP before the final drop rule. This defeats the intended port-based exposure model. NetBird MSS clamping appends directly to the global `FORWARD` chain and can duplicate on every apply. The NAT rules include a catch-all masquerade in Hashi's postrouting chain, affecting unrelated forwarded traffic.

The API exposes direct firewall apply instead of plan/preview/apply/result/audit. Validation of remote package availability is incomplete compared with the spec's `iptables`, `ipset`, `ip`, `sysctl`, and persistence requirements.

## Evidence

- `src/Hashi.Core/Firewall/FirewallScriptRenderer.cs:53-65` defines rollback and starts the timer.
- `src/Hashi.Core/Firewall/FirewallScriptRenderer.cs:65` starts `( sleep "$ROLLBACK_TIMER" && rollback ) &` with no later cancellation.
- `src/Hashi.Core/Firewall/FirewallScriptRenderer.cs:93-101` accepts trusted traffic, then all traffic to `$PUBLIC_IP`, then drops everything else.
- `src/Hashi.Core/Firewall/FirewallScriptRenderer.cs:111-114` adds broad masquerade/SNAT rules.
- `src/Hashi.Core/Firewall/FirewallScriptRenderer.cs:182-184` appends TCPMSS directly to global `FORWARD`.
- `src/Hashi.Api/Features/Platform/PlatformEndpoints.cs:195-229` exposes render/apply/rollback endpoints without a sync plan.
- `src/Hashi.Infrastructure/Platform/FirewallApplyService.cs:128-164` renders and writes scripts directly.

## Expected outcome

Hashi-generated firewall changes must live in Hashi-specific chains/sets and must not flush or mutate unrelated global state beyond one stable jump into Hashi chains. Only configured ports should be opened. Rollback protection must be disarmed after health/SSH verification succeeds. Apply must be plan-driven and audited.

## Fix guidance

Remove the public-IP allow-all rule. Model public ports explicitly in input and forward chains. Put MSS clamping into managed chains or ensure idempotent global rule handling with exact deletes. Disarm rollback after validation. Add a preflight package/capability check. Convert apply to plan/apply or integrate with global sync orchestration.

## Acceptance criteria

- Rendered rules do not allow arbitrary inbound traffic to the public IP.
- Repeated applies do not duplicate global `FORWARD` rules.
- Successful apply cancels rollback after SSH/connectivity checks pass.
- Firewall apply produces a diff preview and auditable result before remote mutation.
