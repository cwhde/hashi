package security

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Rollup and retention of the security telemetry (01 §4.3): the minutes are rolled into hours, the 7 and 30 day
// dashboards read the hours (bug 99), and old rows are deleted in small batches.
const (
	TelemetryJobKind     = "security.telemetry"
	TelemetryJobInterval = 5 * time.Minute

	hourMs = int64(time.Hour / time.Millisecond)
	// reroll is how far back each run recomputes hours, so access-log lines read late still count.
	reroll = 2 * hourMs
	// maxHoursPerRun bounds one run; the first run on a full database catches up over several runs.
	maxHoursPerRun = 7 * 24
	// closeMargin: a minute is written once it is closed, so an hour counts as closed this long after its end.
	closeMargin = 2 * time.Minute

	minuteKeep = 30 * 24 * time.Hour
	hourKeep   = 365 * 24 * time.Hour
	eventKeep  = 90 * 24 * time.Hour
)

// Telemetry runs the rollup and the retention of the security tables.
type Telemetry struct {
	st  *store.Store
	now func() time.Time
}

// NewTelemetry returns the job's state; now is the clock (time.Now in production).
func NewTelemetry(st *store.Store, now func() time.Time) *Telemetry {
	if now == nil {
		now = time.Now
	}
	return &Telemetry{st: st, now: now}
}

// Run is the periodic job: roll first, so retention never deletes a minute that no hour holds yet.
func (t *Telemetry) Run(ctx context.Context) (string, error) {
	hours, err := t.Rollup(ctx)
	if err != nil {
		return "", fmt.Errorf("security rollup: %w", err)
	}
	removed, err := t.Retain(ctx)
	if err != nil {
		return "", fmt.Errorf("security retention: %w", err)
	}
	return fmt.Sprintf("%d hours rolled, %d rows removed", hours, removed), nil
}

// RolledUntil is where the hourly tables end: the dashboard reads hours before it and minutes after it.
func RolledUntil(ctx context.Context, q *db.Queries) (int64, error) {
	last, err := q.LastSecurityHour(ctx)
	if err != nil || last == 0 {
		return 0, err
	}
	return last + hourMs, nil
}

// Rollup recomputes the closed hours since shortly before the last rolled one, each in its own transaction.
func (t *Telemetry) Rollup(ctx context.Context) (int, error) {
	closed := t.now().Add(-closeMargin).UnixMilli() / hourMs * hourMs
	last, err := t.st.R.LastSecurityHour(ctx)
	if err != nil {
		return 0, err
	}
	from := max(last-reroll, 0)
	// Start at the first minute there is, so a quiet stretch (or the first run) costs nothing.
	first, err := t.st.R.NextSecurityMinute(ctx, from)
	if err != nil {
		return 0, err
	}
	if first == 0 {
		return 0, nil
	}
	start := first / hourMs * hourMs
	end := min(closed, start+maxHoursPerRun*hourMs)
	n := 0
	for h := start; h < end; h += hourMs {
		span := struct{ from, to int64 }{h, h + hourMs}
		if err := t.st.Tx(ctx, func(q *db.Queries) error {
			if err := q.ClearSecurityHours(ctx, db.ClearSecurityHoursParams{FromAt: span.from, ToAt: span.to}); err != nil {
				return err
			}
			if err := q.RollupSecurityHours(ctx, db.RollupSecurityHoursParams{FromAt: span.from, ToAt: span.to}); err != nil {
				return err
			}
			if err := q.ClearSecurityTopHours(ctx, db.ClearSecurityTopHoursParams{FromAt: span.from, ToAt: span.to}); err != nil {
				return err
			}
			return q.RollupSecurityTopHours(ctx, db.RollupSecurityTopHoursParams{FromAt: span.from, ToAt: span.to})
		}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// Retain deletes what is past its retention. Minutes go only once an hour holds them.
func (t *Telemetry) Retain(ctx context.Context) (int64, error) {
	now := t.now()
	rolled, err := RolledUntil(ctx, t.st.R)
	if err != nil {
		return 0, err
	}
	minutes := min(now.Add(-minuteKeep).UnixMilli(), rolled)
	hours := now.Add(-hourKeep).UnixMilli()
	tables := []struct {
		cutoff int64
		end    func(context.Context, int64) (int64, error)
		del    func(context.Context, int64) (int64, error)
	}{
		{minutes, t.st.R.SecurityMinuteBatchEnd, t.st.Q.DeleteOldSecurityMinutes},
		{minutes, t.st.R.SecurityTopMinuteBatchEnd, t.st.Q.DeleteOldSecurityTopMinutes},
		{hours, t.st.R.SecurityHourBatchEnd, t.st.Q.DeleteOldSecurityHours},
		{hours, t.st.R.SecurityTopHourBatchEnd, t.st.Q.DeleteOldSecurityTopHours},
		{now.Add(-eventKeep).UnixMilli(), t.st.R.SecurityEventBatchEnd, t.st.Q.DeleteOldSecurityEvents},
	}
	var removed int64
	for _, tb := range tables {
		n, err := deleteBefore(ctx, tb.cutoff, tb.end, tb.del)
		removed += n
		if err != nil {
			return removed, err
		}
	}
	// These jobs run every few minutes; their run history is only useful for a while.
	for _, kind := range []string{TelemetryJobKind, SweepJobKind} {
		n, err := t.st.Q.DeleteOldJobRuns(ctx, db.DeleteOldJobRunsParams{Kind: kind, StartedAt: now.Add(-7 * 24 * time.Hour).UnixMilli()})
		if err != nil {
			return removed, err
		}
		removed += n
	}
	return removed, nil
}

// deleteBefore deletes the rows before cutoff in batches: each delete stops short of the batch's 5000th row. When more
// than a batch shares the oldest time, that one time goes at once.
func deleteBefore(ctx context.Context, cutoff int64, batchEnd, del func(context.Context, int64) (int64, error)) (int64, error) {
	var removed int64
	for {
		end, err := batchEnd(ctx, cutoff)
		if errors.Is(err, sql.ErrNoRows) {
			end = cutoff
		} else if err != nil {
			return removed, err
		}
		n, err := del(ctx, end)
		removed += n
		if err != nil {
			return removed, err
		}
		if end == cutoff {
			return removed, nil
		}
		if n == 0 {
			if n, err = del(ctx, end+1); err != nil || n == 0 {
				return removed + n, err
			}
			removed += n
		}
	}
}
