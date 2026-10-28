package jobs

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

func newRunner(t *testing.T) (*Runner, *store.Store) {
	t.Helper()
	s, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "hashi.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatal(err)
	}
	r := New(s.Q, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r.tick = 10 * time.Millisecond
	return r, s
}

func runFor(t *testing.T, r *Runner, d time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	if err := r.Run(ctx); err != nil {
		t.Fatal(err)
	}
}

func lastRuns(t *testing.T, s *store.Store) map[string]db.ListJobsWithLastRunRow {
	t.Helper()
	rows, err := s.Q.ListJobsWithLastRun(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]db.ListJobsWithLastRunRow{}
	for _, r := range rows {
		m[r.Kind] = r
	}
	return m
}

func TestInvariant_15_JobRunsRecordOutcome(t *testing.T) {
	r, s := newRunner(t)
	ctx := context.Background()
	must(t, r.Register(ctx, Job{Kind: "noop", Interval: 5 * time.Minute, Run: func(context.Context) (string, error) { return "ok", nil }}))
	must(t, r.Register(ctx, Job{Kind: "fails", Interval: time.Hour, Run: func(context.Context) (string, error) {
		return "", errors.New("boom")
	}}))
	must(t, r.Register(ctx, Job{Kind: "panics", Interval: time.Hour, Run: func(context.Context) (string, error) {
		panic("bad")
	}}))
	runFor(t, r, 150*time.Millisecond)

	runs := lastRuns(t, s)
	check := func(kind, status, errText string) {
		t.Helper()
		row := runs[kind]
		if row.Status.String != status || row.Error.String != errText || !row.FinishedAt.Valid || !row.DurationMs.Valid {
			t.Fatalf("%s: %+v", kind, row)
		}
		if row.NextRunAt <= row.StartedAt.Int64 {
			t.Fatalf("%s: next run %d not after start %d", kind, row.NextRunAt, row.StartedAt.Int64)
		}
	}
	check("noop", "succeeded", "")
	check("fails", "failed", "boom")
	check("panics", "failed", "panic: bad")
	if runs["noop"].Summary.String != "ok" {
		t.Fatalf("summary: %q", runs["noop"].Summary.String)
	}
}

func TestKindNeverOverlaps(t *testing.T) {
	r, _ := newRunner(t)
	var active, maxActive atomic.Int32
	must(t, r.Register(context.Background(), Job{Kind: "slow", Interval: time.Second, Run: func(ctx context.Context) (string, error) {
		n := active.Add(1)
		defer active.Add(-1)
		if n > maxActive.Load() {
			maxActive.Store(n)
		}
		<-ctx.Done()
		return "", nil
	}}))
	// Force the row to stay due so every tick would start it again if overlap were allowed.
	r.now = func() time.Time { return time.Now().Add(time.Hour) }
	runFor(t, r, 100*time.Millisecond)
	if maxActive.Load() != 1 {
		t.Fatalf("max concurrent runs = %d", maxActive.Load())
	}
}

func TestOrphanedRunsMarkedInterrupted(t *testing.T) {
	r, s := newRunner(t)
	ctx := context.Background()
	must(t, r.Register(ctx, Job{Kind: "x", Interval: time.Hour, Run: func(context.Context) (string, error) { return "", nil }}))
	must(t, s.Q.StartJobRun(ctx, db.StartJobRunParams{ID: "01ORPHAN", Kind: "x", StartedAt: 1}))
	must(t, s.Q.SetJobNextRun(ctx, db.SetJobNextRunParams{Kind: "x", NextRunAt: time.Now().Add(time.Hour).UnixMilli()}))
	runFor(t, r, 30*time.Millisecond)
	if row := lastRuns(t, s)["x"]; row.Status.String != "interrupted" {
		t.Fatalf("orphan: %+v", row)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestSoonRunsBeforeTheInterval(t *testing.T) {
	r, _ := newRunner(t)
	var runs atomic.Int32
	must(t, r.Register(context.Background(), Job{Kind: "hourly", Interval: time.Hour, Run: func(context.Context) (string, error) {
		runs.Add(1)
		return "", nil
	}}))
	runFor(t, r, 50*time.Millisecond) // the first run of a new kind
	if runs.Load() != 1 {
		t.Fatalf("runs after start = %d", runs.Load())
	}
	must(t, r.Soon(context.Background(), "hourly"))
	runFor(t, r, 50*time.Millisecond)
	if runs.Load() != 2 {
		t.Fatalf("runs after Soon = %d", runs.Load())
	}
	if err := r.Soon(context.Background(), "nope"); err == nil {
		t.Error("unknown kind accepted")
	}
}
