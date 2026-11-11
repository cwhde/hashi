# Scripts contracts (09 §1)

| File | What |
| --- | --- |
| `probe.sh` | Probe run over SSH on a dev target (writes units and files, removes them) |
| `probe_debian13.txt` | Its output on the Debian 13 dev target, 2026-11-11 ("From now" lines dropped: they count from the real clock, not `--base-time`) |

Findings that shape the code:

1. `systemd-analyze calendar --base-time=… --iterations=N` gives exact firing times: the round-trip test for the
   cron → `OnCalendar` conversion compares against them.
2. A weekday and a day of the month together are **AND** in systemd (`Mon *-*-01` = Mondays that are the 1st) but **OR**
   in cron. The conversion emits two `OnCalendar=` lines for that case (a timer fires on the union of its lines).
3. Times are host time (the dev target runs UTC). The UI says "host time".
4. `systemctl list-timers --output=json`: `next`/`last` are µs since the epoch, `last` 0 = never ran. `left` held the same
   value as `next` here, so it is not used.
5. A failed oneshot: `systemctl start` exits 1, `show` gives `Result=exit-code`, `ExecMainStatus` = the script's exit code.
6. `timeout` exits 124 when it kills the script (with and without `-k`).
7. `install -d -m 0750` and `umask 077` give 750 / 600 root:root as 09 §1 asks.
