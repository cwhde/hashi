package monitor

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Resolutions of the rollups, with their retention (01 §4.3).
var resolutions = []struct {
	Seconds int64
	Keep    time.Duration
}{
	{60, 90 * 24 * time.Hour},
	{300, 180 * 24 * time.Hour},
	{3600, 2 * 365 * 24 * time.Hour},
}

const sampleRetention = 30 * 24 * time.Hour

// Rollups computes rollups from samples for the closed buckets in [from, closed): a bucket counts as closed only when
// its whole span lies before closed, so its numbers never change afterwards (07 §4). Pure, for tests.
func Rollups(samples []db.MonitorSample, resolution, closed int64) []db.UpsertMonitorRollupParams {
	width := resolution * 1000
	type key struct {
		endpoint, check string
		bucket          int64
	}
	groups := map[key][]db.MonitorSample{}
	var order []key
	for _, s := range samples {
		b := s.At - s.At%width
		if b+width > closed {
			continue
		}
		k := key{s.EndpointID, s.CheckID, b}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], s)
	}
	out := make([]db.UpsertMonitorRollupParams, 0, len(order))
	for _, k := range order {
		g := groups[k]
		lat := make([]int64, 0, len(g))
		var sum, ok int64
		for _, s := range g {
			lat = append(lat, s.LatencyMs)
			sum += s.LatencyMs
			ok += s.Ok
		}
		slices.Sort(lat)
		n := int64(len(g))
		out = append(out, db.UpsertMonitorRollupParams{EndpointID: k.endpoint, CheckID: k.check, Resolution: resolution, Bucket: k.bucket,
			Count: n, OkCount: ok, MinMs: lat[0], MaxMs: lat[n-1], AvgMs: sum / n, P95Ms: lat[(n*95-1)/100]})
	}
	return out
}

// rollup writes every closed bucket since the last one written, per resolution.
func (s *Service) rollup(ctx context.Context) (int, error) {
	now := s.now().UnixMilli()
	total := 0
	for _, r := range resolutions {
		last, err := s.st.R.LastRollupBucket(ctx, r.Seconds)
		if err != nil {
			return total, err
		}
		width := r.Seconds * 1000
		from := last + width
		if last == 0 {
			from = now - int64(sampleRetention/time.Millisecond)
		}
		samples, err := s.st.R.ListMonitorSamplesBetween(ctx, db.ListMonitorSamplesBetweenParams{At: from, At_2: now - now%width})
		if err != nil {
			return total, err
		}
		rows := Rollups(samples, r.Seconds, now)
		if len(rows) == 0 {
			continue
		}
		if err := s.st.Tx(ctx, func(q *db.Queries) error {
			for _, row := range rows {
				if err := q.UpsertMonitorRollup(ctx, row); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return total, err
		}
		total += len(rows)
	}
	return total, nil
}

// retain deletes what is past its retention (01 §4.3), an hour of samples per transaction.
func (s *Service) retain(ctx context.Context) (int64, error) {
	now := s.now()
	var removed int64
	// Every delete stops short of the batch's last row, so one transaction removes fewer than deleteBatch rows.
	for cutoff := now.Add(-sampleRetention).UnixMilli(); ; {
		end, err := s.st.R.SampleBatchEnd(ctx, cutoff)
		if errors.Is(err, sql.ErrNoRows) {
			end = cutoff
		} else if err != nil {
			return removed, err
		}
		n, err := s.st.Q.DeleteOldMonitorSamples(ctx, end)
		if err != nil {
			return removed, err
		}
		removed += n
		if end == cutoff || n == 0 {
			break
		}
	}
	for _, r := range resolutions {
		for cutoff := now.Add(-r.Keep).UnixMilli(); ; {
			end, err := s.st.R.RollupBatchEnd(ctx, db.RollupBatchEndParams{Resolution: r.Seconds, Bucket: cutoff})
			if errors.Is(err, sql.ErrNoRows) {
				end = cutoff
			} else if err != nil {
				return removed, err
			}
			n, err := s.st.Q.DeleteOldMonitorRollups(ctx, db.DeleteOldMonitorRollupsParams{Resolution: r.Seconds, Bucket: end})
			if err != nil {
				return removed, err
			}
			removed += n
			if end == cutoff || n == 0 {
				break
			}
		}
	}
	// The check job runs every few seconds; its run history is only useful for a while.
	for _, kind := range []string{JobChecks, JobRollup, "notify.deliver"} {
		n, err := s.st.Q.DeleteOldJobRuns(ctx, db.DeleteOldJobRunsParams{Kind: kind, StartedAt: now.Add(-7 * 24 * time.Hour).UnixMilli()})
		if err != nil {
			return removed, err
		}
		removed += n
	}
	return removed, nil
}
