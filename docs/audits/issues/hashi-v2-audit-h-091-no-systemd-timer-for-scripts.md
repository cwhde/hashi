# H-091: No Systemd Timer Option for Script Scheduling

**Priority:** Low
**Conflict Type:** missing_implementation
**Spec Reference:** §23 (Cron entries generated in /etc/cron.d/hashi-scripts or systemd timers)

**Status:** Not Started
**Branch:** 

## Description

Script scheduling on remote hosts uses only `/etc/cron.d/hashi-scripts` — no systemd timer option. The spec mentions systemd timers as an alternative.

## Evidence

- Script scheduling code only generates cron entries in `/etc/cron.d/hashi-scripts`
- No systemd timer generation code exists

## Expected Outcome

Scripts should be schedulable via systemd timers on hosts that use systemd. Init system detection during host setup should determine whether to use cron or systemd timers.

## Fix Guidance

1. Add systemd timer generation as an option for script scheduling on hosts that use systemd.
2. Detect init system during host setup and use appropriate method.
3. Ensure both scheduling methods produce correct execution.

## Acceptance Criteria

- [ ] Scripts can be scheduled via systemd timers on systemd hosts
- [ ] Init system detection determines cron vs systemd timer
- [ ] Both scheduling methods produce correct execution
