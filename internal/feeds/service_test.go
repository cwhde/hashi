package feeds

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

// feedServer serves a body with an ETag, or fails.
type feedServer struct {
	mu   sync.Mutex
	body string
	fail bool
}

func (f *feedServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fail {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	etag := `"` + strings.ReplaceAll(strings.TrimSpace(f.body), "\n", "|") + `"`
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Set("ETag", etag)
	_, _ = io.WriteString(w, f.body)
}

func (f *feedServer) set(body string, fail bool) {
	f.mu.Lock()
	f.body, f.fail = body, fail
	f.mu.Unlock()
}

// The refresh lifecycle (06 §8): first download, unchanged by validator, changed content, a failed refresh keeps the
// last good entries and marks the set partial, bad content is a failure too, and after the stale time the set empties.
func TestRefreshLifecycle(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	al := audit.New([]byte("k"))
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	sets := ipsets.NewService(st, al, log, nil)
	fs := &feedServer{body: "# test\n45.0.0.0/24\n46.1.2.3\n10.0.0.0/8\n"}
	srv := httptest.NewServer(fs)
	defer srv.Close()
	svc := NewService(st, al, sets, &Fetcher{UserAgent: "t", MaxBytes: 1 << 20, Timeout: 5 * time.Second, AllowInternal: true}, log, clock)

	set := Settings{URL: srv.URL + "/list.txt", Format: FormatLines, AllowHTTP: true}
	pv, err := svc.Preview(ctx, set)
	if err != nil || pv.Count != 2 || pv.Rejected != 1 || pv.Rejects[0].Reason != "private range" {
		t.Fatalf("preview %+v %v", pv, err)
	}
	if _, err := svc.Create(ctx, "Test", "Bad Slug", "", "", set, "admin", ""); err == nil {
		t.Error("bad slug accepted")
	}
	if _, err := svc.Create(ctx, "Test", "test", "", "", Settings{URL: srv.URL, Format: FormatLines}, "admin", ""); err == nil {
		t.Error("plain HTTP accepted without allowing it")
	}
	id, err := svc.Create(ctx, "Test feed", "test-feed", "", "", set, "admin", "")
	if err != nil {
		t.Fatal(err)
	}
	compiled := func() ipsets.Result {
		t.Helper()
		r, err := sets.Compile(ctx, st.R, id)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := compiled(); r.Status != ipsets.StatusEmpty || len(r.Problems) != 1 {
		t.Fatalf("before the first download: %s %+v", r.Status, r.Problems)
	}

	run, err := svc.Refresh(ctx, id)
	if err != nil || run.Outcome != OutcomeUpdated || run.Added != 2 || run.Rejected != 1 {
		t.Fatalf("first refresh %+v %v", run, err)
	}
	if r := compiled(); r.Status != ipsets.StatusOK || len(r.V4) != 2 {
		t.Fatalf("after the first download: %s %v %+v", r.Status, r.V4, r.Problems)
	}

	now = now.Add(time.Hour)
	if run, err = svc.Refresh(ctx, id); err != nil || run.Outcome != OutcomeUnchanged || run.Unchanged != 2 {
		t.Fatalf("304 refresh %+v %v", run, err)
	}

	fs.set("45.0.0.0/24\n47.0.0.1\n", false)
	if run, err = svc.Refresh(ctx, id); err != nil || run.Outcome != OutcomeUpdated || run.Added != 1 || run.Removed != 1 || run.Unchanged != 1 {
		t.Fatalf("changed refresh %+v %v", run, err)
	}

	fs.set("", true)
	if run, err = svc.Refresh(ctx, id); err != nil || run.Outcome != OutcomeFailed || !strings.Contains(run.Error, "502") {
		t.Fatalf("failed refresh %+v %v", run, err)
	}
	if r := compiled(); r.Status != ipsets.StatusPartial || len(r.V4) != 2 {
		t.Fatalf("after a failure the last good entries stay, partial: %s %v", r.Status, r.V4)
	}

	// Content in the wrong format fails as a whole: no partial update.
	f2 := Settings{URL: srv.URL + "/list.json", Format: FormatJSONArray, AllowHTTP: true}
	id2, err := svc.Create(ctx, "JSON", "json-feed", "", "", f2, "admin", "")
	if err != nil {
		t.Fatal(err)
	}
	fs.set(`["45.9.9.9"]`, false)
	if run, _ := svc.Refresh(ctx, id2); run.Outcome != OutcomeUpdated {
		t.Fatalf("json %+v", run)
	}
	fs.set(`<html>maintenance</html>`, false)
	if run, _ := svc.Refresh(ctx, id2); run.Outcome != OutcomeFailed {
		t.Fatalf("json garbage %+v", run)
	}
	if r, _ := sets.Compile(ctx, st.R, id2); len(r.V4) != 1 {
		t.Errorf("garbage replaced the entries: %v", r.V4)
	}
	// The same for a line-based list (all recommended feeds are lines, netset or CSV): an error page that answers 200
	// must not replace the entries with nothing. Seen in review: "updated, added 0 removed 2", the set emptied.
	before, _ := sets.Compile(ctx, st.R, id)
	fs.set("<!doctype html>\n<html><body>Down for maintenance</body></html>\n", false)
	if run, _ := svc.Refresh(ctx, id); run.Outcome != OutcomeFailed {
		t.Fatalf("an HTML page on a lines feed: %+v", run)
	}
	if r, _ := sets.Compile(ctx, st.R, id); len(r.V4) != len(before.V4) || len(r.V4) == 0 {
		t.Errorf("an error page replaced the entries: %v, had %v", r.V4, before.V4)
	}
	// A list that is really empty (nothing rejected) is still an update.
	fs.set("# no entries today\n", false)
	if run, _ := svc.Refresh(ctx, id); run.Outcome != OutcomeUpdated {
		t.Fatalf("an empty list: %+v", run)
	}
	fs.set("45.9.9.1\n45.9.9.2\n", false) // entries again for the stale check below
	if run, _ := svc.Refresh(ctx, id); run.Outcome != OutcomeUpdated {
		t.Fatalf("refill: %+v", run)
	}

	// Stale: no success for longer than the stale time empties the set.
	fs.set("", true)
	now = now.Add(80 * time.Hour)
	msg, err := svc.RunDue(ctx, 6*time.Hour, 72*time.Hour)
	if err != nil || !strings.Contains(msg, "2 expired") {
		t.Fatalf("job: %q %v", msg, err)
	}
	if r := compiled(); r.Status != ipsets.StatusEmpty || len(r.V4) != 0 {
		t.Fatalf("stale feed: %s %v", r.Status, r.V4)
	}
	runs, _ := st.R.ListFeedRuns(ctx, id)
	if len(runs) < 5 || runs[0].Outcome != OutcomeStale {
		t.Errorf("runs %+v", runs)
	}
}
