package scripts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Run statuses.
const (
	RunRunning   = "running"
	RunSucceeded = "succeeded"
	RunFailed    = "failed"
	RunTimedOut  = "timed_out"
	RunError     = "error" // Hashi could not run it or lost it: no exit code
)

// Run triggers.
const (
	TriggerManual    = "manual"
	TriggerScheduled = "scheduled"
)

// grace is how long Hashi waits past a script's timeout before it gives up on the SSH session: the wrapper's
// timeout(1) stops the script first, and -k 10 kills it 10 s later.
const grace = 30 * time.Second

// statusOf maps an exit code to a run status.
func statusOf(code int) string {
	switch code {
	case 0:
		return RunSucceeded
	case TimedOut:
		return RunTimedOut
	}
	return RunFailed
}

// redact replaces every secret value of the host's scripts in out (09 §1) and keeps the text valid UTF-8.
func redact(out string, secretVals []string) string {
	vals := slices.Clone(secretVals)
	slices.SortFunc(vals, func(a, b string) int { return len(b) - len(a) }) // longest first: a value inside another
	for _, v := range vals {
		if len(v) >= 3 {
			out = strings.ReplaceAll(out, v, "[secret]")
		}
	}
	return strings.ToValidUTF8(out, "�")
}

// Start runs a script on one host now, in the background, and returns the run's id at once (09 §1, manual trigger).
// The run's own context lives past the request, bounded by the script's timeout.
func (s *Service) Start(ctx context.Context, actor, ip, id, hostID string) (string, error) {
	sc, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if sc.Enabled == 0 {
		return "", fmt.Errorf("%w: the script is turned off", ErrInvalid)
	}
	targets, err := s.Targets(ctx, sc)
	if err != nil {
		return "", err
	}
	if !slices.Contains(targets, hostID) {
		return "", fmt.Errorf("%w: the script does not run on that host", ErrInvalid)
	}
	// Opening the variables now also refuses a run while the vault is locked.
	ds, secretVals, err := s.desired(ctx, hostID)
	if err != nil {
		return "", err
	}
	runID, now := ulid.Make().String(), s.now()
	err = s.st.Tx(ctx, func(q *db.Queries) error {
		if err := q.InsertScriptRun(ctx, db.InsertScriptRunParams{ID: runID, ScriptID: id, HostID: hostID, Trigger: TriggerManual,
			Status: RunRunning, StartedAt: now.UnixMilli(), Actor: actor}); err != nil {
			return err
		}
		return s.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "script.run", Target: "script:" + id, IP: ip,
			Outcome: audit.Success, Detail: map[string]any{"host": hostID, "run": runID}})
	})
	if err != nil {
		return "", err
	}
	want := ""
	for _, d := range ds {
		if d.Slug == sc.Slug {
			files, _, err := Render([]Desired{d})
			if err != nil {
				return "", err
			}
			want = groupHash(filesOf(files)[d.Slug])
		}
	}
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Duration(sc.TimeoutSeconds)*time.Second+grace)
	go func() {
		defer cancel()
		s.finish(rctx, runID, s.runOn(rctx, hostID, sc, want), secretVals)
	}()
	return runID, nil
}

type outcome struct {
	code      int
	output    string
	truncated bool
	err       error
}

// runOn runs the installed copy over SSH. It refuses when the host's copy is not the saved one: the run must be the
// script the admin sees, so a pending change is applied first.
func (s *Service) runOn(ctx context.Context, hostID string, sc Script, want string) outcome {
	h, err := s.dial(ctx, hostID)
	if err != nil {
		return outcome{err: err}
	}
	defer func() { _ = h.Close() }()
	o, err := readObserved(h, nil)
	if err != nil {
		return outcome{err: err}
	}
	var have []File
	for p, b := range o.files {
		if slugOfPath(p) == sc.Slug {
			have = append(have, File{Path: p, Data: b})
		}
	}
	if len(have) == 0 || groupHash(have) != want {
		return outcome{err: fmt.Errorf("%w: apply the pending change for this host first", ErrNotReady)}
	}
	// A fixed command: the slug is checked ([a-z0-9-]) and the timeout is a number.
	cmd := Wrapper + " " + sc.Slug + " " + strconv.FormatInt(sc.TimeoutSeconds, 10) + " --direct 2>&1"
	r, err := h.Run(ctx, cmd, nil)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return outcome{code: TimedOut, output: string(r.Stdout), truncated: r.Truncated}
		}
		return outcome{err: err}
	}
	return outcome{code: r.ExitCode, output: string(r.Stdout), truncated: r.Truncated}
}

func (s *Service) finish(ctx context.Context, runID string, o outcome, secretVals []string) {
	p := db.FinishScriptRunParams{ID: runID, FinishedAt: sql.NullInt64{Int64: s.now().UnixMilli(), Valid: true},
		Output: redact(o.output, secretVals), Truncated: b2i(o.truncated)}
	if o.err != nil {
		p.Status, p.Error = RunError, redact(o.err.Error(), secretVals)
	} else {
		p.Status, p.ExitCode = statusOf(o.code), sql.NullInt64{Int64: int64(o.code), Valid: true}
	}
	// The request that started the run is gone; this write must not die with it.
	wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	err := s.st.Tx(wctx, func(q *db.Queries) error {
		if err := q.FinishScriptRun(wctx, p); err != nil {
			return err
		}
		r, err := q.GetScriptRun(wctx, runID)
		if err != nil {
			return err
		}
		return q.PruneScriptRuns(wctx, r.ScriptID)
	})
	if err != nil {
		s.log.Error("script run: store result", "run", runID, "err", err)
	}
}

// Runs lists a script's newest runs.
func (s *Service) Runs(ctx context.Context, id string) ([]db.ScriptRun, error) {
	if _, err := s.Get(ctx, id); err != nil {
		return nil, err
	}
	return s.st.R.ListScriptRuns(ctx, id)
}

// GetRun loads one run.
func (s *Service) GetRun(ctx context.Context, runID string) (db.ScriptRun, error) {
	r, err := s.st.R.GetScriptRun(ctx, runID)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

// Interrupted marks runs that were running when Hashi stopped (at start, invariant 15).
func (s *Service) Interrupted(ctx context.Context) error {
	return s.st.Tx(ctx, func(q *db.Queries) error { return q.InterruptScriptRuns(ctx, sql.NullInt64{Int64: s.now().UnixMilli(), Valid: true}) })
}

// spoolEntry is one finished scheduled run in a host's spool: <slug>.<start>.<pid>.{out,done}.
type spoolEntry struct {
	name, slug string
	start, end time.Time
	code, size int
}

func parseDone(name string, meta []byte) (spoolEntry, bool) {
	e := spoolEntry{name: name}
	parts := strings.Split(name, ".")
	if len(parts) != 3 || !slugRe.MatchString(parts[0]) {
		return e, false
	}
	e.slug = parts[0]
	got := 0
	for _, l := range strings.Split(string(meta), "\n") {
		k, v, ok := strings.Cut(l, "=")
		n, err := strconv.Atoi(v)
		if !ok || err != nil {
			continue
		}
		switch k {
		case "exit":
			e.code, got = n, got+1
		case "start":
			e.start, got = time.Unix(int64(n), 0), got+1
		case "end":
			e.end, got = time.Unix(int64(n), 0), got+1
		case "size":
			e.size = n
		}
	}
	return e, got == 3
}

// Collect moves finished scheduled runs from every script host's spool into the history (09 §1). A run is stored
// once: the spool name is unique per host, so a collection that died before deleting the files skips them next time.
func (s *Service) Collect(ctx context.Context) (string, error) {
	hosts, err := s.st.R.ListFirewallHosts(ctx)
	if err != nil {
		return "", err
	}
	all, err := s.List(ctx)
	if err != nil {
		return "", err
	}
	if len(all) == 0 {
		return "no scripts", nil
	}
	bySlug := map[string]Script{}
	for _, sc := range all {
		bySlug[sc.Slug] = sc
	}
	total := 0
	var errs []error
	for _, h := range hosts {
		if !Eligible(h) || !slices.ContainsFunc(all, func(sc Script) bool { return slices.Contains(sc.targets(hosts), h.ID) }) {
			continue
		}
		n, err := s.collectHost(ctx, h.ID, bySlug)
		total += n
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", h.Name, err))
		}
	}
	return fmt.Sprintf("%d scheduled run(s) collected", total), errors.Join(errs...)
}

func (s *Service) collectHost(ctx context.Context, hostID string, bySlug map[string]Script) (int, error) {
	_, secretVals, err := s.desired(ctx, hostID)
	if err != nil {
		return 0, err
	}
	h, err := s.dial(ctx, hostID)
	if err != nil {
		return 0, err
	}
	defer func() { _ = h.Close() }()
	r, err := h.Run(ctx, "find "+Spool+" -maxdepth 1 -name '*.done' -type f 2>/dev/null; true", nil)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, p := range strings.Fields(string(r.Stdout)) {
		name := strings.TrimSuffix(strings.TrimPrefix(p, Spool+"/"), ".done")
		meta, err := h.ReadFile(p)
		if err != nil {
			return n, err
		}
		e, ok := parseDone(name, meta)
		sc, known := bySlug[e.slug]
		if ok && known {
			out, err := h.ReadFile(Spool + "/" + name + ".out")
			if err != nil {
				return n, err
			}
			if err := s.storeScheduled(ctx, hostID, sc, e, out, secretVals); err != nil {
				return n, err
			}
			n++
		}
		// Stored, unknown (the script was deleted) or unreadable: either way it leaves the spool.
		base := Spool + "/" + name
		if rr, err := h.Run(ctx, "rm -f "+sshx.Quote(base+".done")+" "+sshx.Quote(base+".out"), nil); err != nil || !rr.OK() {
			return n, cmdErr("rm spool "+name, rr, err)
		}
	}
	return n, nil
}

func (s *Service) storeScheduled(ctx context.Context, hostID string, sc Script, e spoolEntry, out []byte, secretVals []string) error {
	text := string(out)
	if !utf8.ValidString(text) {
		text = strings.ToValidUTF8(text, "�")
	}
	return s.st.Tx(ctx, func(q *db.Queries) error {
		n, err := q.CountScriptSpool(ctx, db.CountScriptSpoolParams{HostID: hostID, SpoolName: e.name})
		if err != nil || n > 0 {
			return err
		}
		if err := q.InsertScriptRun(ctx, db.InsertScriptRunParams{ID: ulid.Make().String(), ScriptID: sc.ID, HostID: hostID,
			Trigger: TriggerScheduled, Status: statusOf(e.code), ExitCode: sql.NullInt64{Int64: int64(e.code), Valid: true},
			StartedAt: e.start.UnixMilli(), FinishedAt: sql.NullInt64{Int64: e.end.UnixMilli(), Valid: true},
			Output: redact(text, secretVals), Truncated: b2i(e.size > len(out)), SpoolName: e.name}); err != nil {
			return err
		}
		return q.PruneScriptRuns(ctx, sc.ID)
	})
}
