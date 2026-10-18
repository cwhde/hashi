# AUD-080: Systemd schedule conversion changes valid cron timing

Priority: P2

Category: Scripts / scheduled feature correctness

Status: open; confirmed by manual source tracing.

## Finding

ConvertCronToOnCalendar converts stepped day/month fields with MapCronField, which turns */2 into 2. A cron schedule intended for alternating months therefore runs in February only; stepped day-of-month schedules likewise become a single day. When both day-of-month and day-of-week are restricted, cron OR semantics become a systemd calendar AND. Unsupported weekday step expressions return null and the scheduler silently omits/removes the timer instead of falling back or rejecting the schedule. ScriptCronSchedule uses Cronos but is not used by this deployment conversion.

## Evidence

src/Hashi.Infrastructure/Platform/ScriptExecutionService.cs:381-410,927-1016.

## Expected outcome

Preserve standard cron timing through a supported converter or an actual cron scheduler; reject unsupported schedules with an actionable message.

## Acceptance criteria

Month/day steps, weekday steps and combined DOM/DOW schedules retain their documented occurrence times, and no accepted schedule is silently dropped.

## Review correction

The initial candidate about an omitted target list running on all hosts was withdrawn after cross-checking base specification section21: default all Linux firewall hosts is explicitly required. No claim that this default itself is a bug is retained. Explicitly disabled target fallback remains a separate edge-case candidate in coverage notes.
