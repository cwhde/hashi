# LEG-003: Manual and scheduled runs can stop or duplicate automatic synchronization

Priority: P2

Category: Legacy scheduler reliability

Status: open; manual source review. Applies to the separately deployed legacy artifact, not the active V2 process.

## Finding

If the scheduled timeout fires while a manual run is active, runSync returns immediately without rescheduling or clearing timer. The manual run later sees a nonnull timer handle and skips scheduling its successor, stopping automatic sync. Restart/stop while a run is active does not cancel that run and can allow multiple future timers to be installed. Unbounded configured intervals also exceed native timeout limits.

## Evidence

hashi.old/src/services/scheduler.js:23-55,63-67,106-109,132-139.

## Expected outcome and acceptance criteria

Use an explicit scheduler state machine with one owned next timer, clear fired handles, coordinate manual/running work and clamp intervals. A timer firing during a manual run must leave exactly one future scheduled run.
