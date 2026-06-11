# H-074: No Warning Before Default-Deny Firewall

**Priority:** Medium
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §14.5

**Status:** Fixed
**Branch:** audit-series-h

## Description

The generated firewall script ends with `iptables -A HASHI_INPUT -j DROP` which blocks ALL traffic not explicitly allowed. There is no mechanism to check if the user's current SSH source IP would be blocked, or to warn the user before applying this default-deny policy. If the admin's source IP is not in `ManagedSubnets`, `TrustedPublicIps`, or NetBird, their SSH session will be dropped.

## Evidence

- `FirewallScriptRenderer` generates a default-deny INPUT rule
- No UI warning or validation checks whether the admin's current IP would survive the new rules
- `VerifyPostApplyAsync` only checks that jump rules exist, not that SSH survived

## Expected Outcome

Before applying a default-deny firewall policy, the system should warn the user and verify that their current SSH source IP is in the allowed list.

## Fix Guidance

1. In the firewall plan preview, compare the admin's current source IP against the list of allowed sources.
2. If the admin's IP is NOT in the allowed list, show a prominent warning in the UI.
3. Add a confirmation step specifically for default-deny applies that requires the admin to acknowledge the risk.

## Acceptance Criteria

- [x] Firewall plan preview warns if admin's source IP would be blocked
- [x] Applying default-deny requires explicit acknowledgment
- [x] Warning clearly explains the risk of losing SSH access
