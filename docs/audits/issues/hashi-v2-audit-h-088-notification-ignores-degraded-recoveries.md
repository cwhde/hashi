# H-088: Notification Ignores Degraded→Up Recoveries

**Priority:** Low
**Conflict Type:** wrong_implementation
**Spec Reference:** §18.6 (Recovery notifications)

**Status:** Not Started
**Branch:** 

## Description

Notification routing only fires on down→up recovery, not degraded→up. The spec says "recovery notifications" broadly. Degraded→up recoveries are silently ignored.

## Evidence

- Notification logic only triggers on down→up transition
- Degraded→up transition produces no notification

## Expected Outcome

Degraded→up transitions should trigger recovery notifications. Notification routes should be configurable for which transitions trigger notifications.

## Fix Guidance

1. Add degraded→up as a recovery transition in the notification routing logic.
2. Allow configuration of which transitions trigger notifications.

## Acceptance Criteria

- [ ] Degraded→up transitions trigger recovery notifications
- [ ] Notification routes can configure which transitions to notify on
