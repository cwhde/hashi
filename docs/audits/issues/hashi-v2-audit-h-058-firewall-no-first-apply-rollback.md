# H-058: Firewall First Apply Has No Rollback Path — Host Becomes Unreachable If SSH Lost

**Priority:** Critical
**Conflict Type:** missing_implementation
**Spec Reference:** Main Spec §14.5 (Hashi schedules a rollback if SSH connectivity is lost during first apply); §14.4 (Hashi should install rollback protection for first firewall apply)

**Status:** Fixed
**Branch:** h/security-2

## Description

The firewall rollback mechanism requires a previous script to exist. On the very first apply to a new host, `host.RollbackScript` is empty and no `hashi-firewall.rollback.sh` is written. If SSH is lost during the very first firewall apply, the host becomes unreachable with no recovery path. The spec explicitly requires rollback protection for first apply because losing NetBird access can also mean losing the rescue path.

## Evidence

FirewallApplyService applies the script and starts a rollback timer, but on first apply there is no previous script to roll back to. The rollback timer fires but does nothing because there is no rollback script to execute.

## Expected Outcome

On first apply, Hashi should snapshot the existing iptables state or generate a "reset" rollback script that flushes only Hashi chains and removes jump rules, providing a recovery path even for brand-new hosts.

## Fix Guidance

Before the first firewall apply: (1) Capture the current iptables state via `iptables-save`, (2) Generate a minimal rollback script that flushes only HASHI_* chains and removes jump rules from global chains, (3) Write this rollback script to the remote host before applying the new firewall script.

## Acceptance Criteria

- [ ] First apply on a new host writes a rollback script before applying
- [ ] If SSH is lost during first apply, the rollback timer restores network access
- [ ] Rollback script only flushes Hashi-owned chains, not global or NetBird rules
