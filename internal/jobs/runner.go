// Package jobs is Hashi's in-process job runner (01-platform.md §6).
package jobs

import (
	"context"
	"crypto/rand"
	"database/sql"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Func does one run of a job. The returned summary is shown in Activity → Jobs.
type Func func(ctx context.Context) (summary string, err error)

// Job is a registered kind with a fixed interval.
type Job struct {
	Kind     string
	Interval time.Duration
	Run      Func
}

// Runner polls the jobs table and starts due kinds. A kind never overlaps itself.
type Runner struct {
	q    *db.Queries
	log  *slog.Logger
	now  func() time.Time
	tick time.Duration
	jobs map[string]Job
	mu   sync.Mutex
	busy map[string]bool
	wg   sync.WaitGroup
}

// New returns a runner using q for persistence.
func New(q *db.Queries, log *slog.Logger) *Runner {
	return &Runner{
		q: q, log: log.With("component", "jobs"), now: time.Now, tick: time.Second,
		jobs: map[string]Job{}, busy: map[string]bool{},
	}
}

// Register adds a kind and ensures its row exists. New kinds run immediately.
func (r *Runner) Register(ctx context.Context, j Job) error {
	if j.Interval < time.Second {
		return fmt.Errorf("job %s: interval %s below 1s", j.Kind, j.Interval)
	}
	now := r.now().UnixMilli()
	if err := r.q.UpsertJob(ctx, db.UpsertJobParams{
		Kind: j.Kind, IntervalSeconds: int64(j.Interval / time.Second), NextRunAt: now, CreatedAt: now,
	}); err != nil {
		return fmt.Errorf("register job %s: %w", j.Kind, err)
	}
	r.jobs[j.Kind] = j
	return nil
}

// Run marks runs orphaned by a previous process as interrupted, then schedules until ctx is done.
// It waits for in-flight runs before returning.
func (r *Runner) Run(ctx context.Context) error {
	n, err := r.q.InterruptRunningJobRuns(context.WithoutCancel(ctx), sqlInt(r.now().UnixMilli()))
	if err != nil {
		return fmt.Errorf("mark interrupted runs: %w", err)
	}
	if n > 0 {
		r.log.Warn("previous runs interrupted", "count", n)
	}
	t := time.NewTicker(r.tick)
	defer t.Stop()
	for {
		r.dispatch(ctx)
		select {
		case <-ctx.Done():
			r.wg.Wait()
			return nil
		case <-t.C:
		}
	}
}

func (r *Runner) dispatch(ctx context.Context) {
	due, err := r.q.DueJobs(ctx, r.now().UnixMilli())
	if err != nil {
		if ctx.Err() == nil {
			r.log.Error("list due jobs", "err", err)
		}
		return
	}
	for _, row := range due {
		j, ok := r.jobs[row.Kind]
		if !ok || !r.claim(row.Kind) {
			continue // unknown kind (removed in this version) or still running
		}
		r.wg.Add(1)
		go func() {
			defer r.wg.Done()
			defer r.release(j.Kind)
			r.runOnce(ctx, j)
		}()
	}
}

func (r *Runner) claim(kind string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.busy[kind] {
		return false
	}
	r.busy[kind] = true
	return true
}

func (r *Runner) release(kind string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.busy, kind)
}

func (r *Runner) runOnce(ctx context.Context, j Job) {
	// Bookkeeping must survive shutdown so a run is never left 'running'.
	bg := context.WithoutCancel(ctx)
	start := r.now()
	id := ulid.MustNew(ulid.Timestamp(start), rand.Reader).String()
	if err := r.q.SetJobNextRun(bg, db.SetJobNextRunParams{NextRunAt: start.Add(j.Interval).UnixMilli(), Kind: j.Kind}); err != nil {
		r.log.Error("schedule next run", "job", j.Kind, "err", err)
		return
	}
	if err := r.q.StartJobRun(bg, db.StartJobRunParams{ID: id, Kind: j.Kind, StartedAt: start.UnixMilli()}); err != nil {
		r.log.Error("record run start", "job", j.Kind, "err", err)
		return
	}

	summary, runErr := safeRun(ctx, j.Run)
	end := r.now()
	status, errText := "succeeded", ""
	if runErr != nil {
		status, errText = "failed", runErr.Error()
	}
	if err := r.q.FinishJobRun(bg, db.FinishJobRunParams{
		FinishedAt: sqlInt(end.UnixMilli()), Status: status, DurationMs: sqlInt(end.Sub(start).Milliseconds()),
		Summary: summary, Error: errText, ID: id,
	}); err != nil {
		r.log.Error("record run finish", "job", j.Kind, "err", err)
	}
	lvl := slog.LevelInfo
	if runErr != nil {
		lvl = slog.LevelError
	}
	r.log.Log(bg, lvl, "job finished", "job", j.Kind, "run_id", id, "status", status,
		"duration_ms", end.Sub(start).Milliseconds(), "summary", summary, "err", errText)
}

// safeRun turns a panicking job into a failed run instead of a dead process.
func safeRun(ctx context.Context, f Func) (summary string, err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("panic: %v", p)
		}
	}()
	return f(ctx)
}

func sqlInt(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} }
