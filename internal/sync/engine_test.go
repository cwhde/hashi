package sync_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
)

// fake is a subsystem whose next draft the test sets; Apply records the items it was given.
type fake struct {
	draft   hsync.Draft
	applied [][]hsync.Item
	err     error
	// onPlan runs at the start of Plan.
	onPlan func()
}

func (f *fake) Name() string            { return "dns-records" }
func (f *fake) LockKey(t string) string { return "dns:" + t }
func (f *fake) Plan(context.Context, string) (hsync.Draft, error) {
	if f.onPlan != nil {
		f.onPlan()
	}
	return f.draft, nil
}

func (f *fake) Apply(_ context.Context, _ string, items []hsync.Item) (string, error) {
	f.applied = append(f.applied, items)
	return "ok", f.err
}

func item(action string, risk hsync.Risk, obj string) hsync.Item {
	return hsync.Item{Action: action, Object: obj, After: json.RawMessage(`{"v":"` + obj + `"}`), Risk: risk}
}

func newEngine(t *testing.T, f *fake) *hsync.Engine {
	t.Helper()
	e := hsync.New(storetest.New(t), audit.New([]byte("k")), f)
	if err := e.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestLowRiskAppliesAutomatically(t *testing.T) {
	f := &fake{draft: hsync.Draft{Title: "t", Items: []hsync.Item{item(hsync.ActionCreate, hsync.Low, "a")}}}
	e := newEngine(t, f)
	p, err := e.Auto(t.Context(), "dns-records", "z1", hsync.TriggerSave, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != hsync.StatusApplied || p.ApprovedBy != hsync.AutoApprover || len(f.applied) != 1 {
		t.Fatalf("plan %+v applied %d", p, len(f.applied))
	}
	// Nothing to change → no plan.
	f.draft = hsync.Draft{}
	if p, err := e.Auto(t.Context(), "dns-records", "z1", hsync.TriggerReconcile, "system"); p != nil || err != nil {
		t.Fatalf("empty draft gave %+v %v", p, err)
	}
	// Conflicts alone change nothing → no plan either.
	f.draft = hsync.Draft{Items: []hsync.Item{item(hsync.ActionConflict, hsync.Low, "c")}}
	if p, err := e.Auto(t.Context(), "dns-records", "z1", hsync.TriggerReconcile, "system"); p != nil || err != nil {
		t.Fatalf("conflict-only draft gave %+v %v", p, err)
	}
}

// Invariant 7: a high-risk change is never applied before an admin approves it, and what runs is what was approved.
func TestInvariant_7_HighRiskWaitsForApproval(t *testing.T) {
	f := &fake{draft: hsync.Draft{Items: []hsync.Item{item(hsync.ActionCreate, hsync.Low, "a"), item(hsync.ActionDelete, hsync.High, "b")}}}
	e := newEngine(t, f)
	ctx := t.Context()
	p, err := e.Auto(ctx, "dns-records", "z1", hsync.TriggerSave, "admin")
	if err != nil || p.Status != hsync.StatusPending || p.Risk != hsync.High {
		t.Fatalf("plan %+v err %v", p, err)
	}
	if _, err := e.Apply(ctx, p.ID, "admin", ""); !errors.Is(err, hsync.ErrState) || len(f.applied) != 0 {
		t.Fatalf("apply before approval: %v, applied %d", err, len(f.applied))
	}
	if err := e.Approve(ctx, p.ID, "admin", "192.0.2.1"); err != nil {
		t.Fatal(err)
	}
	got, err := e.Apply(ctx, p.ID, "admin", "192.0.2.1")
	if err != nil || got.ID != p.ID || got.Status != hsync.StatusApplied {
		t.Fatalf("apply: %+v %v", got, err)
	}
	if hsync.Hash(f.applied[0]) != p.Hash {
		t.Error("applied items differ from the approved plan")
	}
	if err := e.Approve(ctx, p.ID, "admin", ""); !errors.Is(err, hsync.ErrState) {
		t.Errorf("approve twice: %v", err)
	}
}

// If the provider changed between approval and apply, a new high-risk item sends the plan back to approval; a change
// that only touches low-risk items applies the fresh plan.
func TestReplanBeforeApply(t *testing.T) {
	high := item(hsync.ActionDelete, hsync.High, "b")
	f := &fake{draft: hsync.Draft{Items: []hsync.Item{high}}}
	e := newEngine(t, f)
	ctx := t.Context()
	p, _ := e.Propose(ctx, "dns-records", "z1", hsync.TriggerSave, "admin")
	_ = e.Approve(ctx, p.ID, "admin", "")

	f.draft = hsync.Draft{Items: []hsync.Item{high, item(hsync.ActionCreate, hsync.Low, "new-low")}}
	got, err := e.Apply(ctx, p.ID, "admin", "")
	if err != nil || got.ID == p.ID || got.Status != hsync.StatusApplied || len(f.applied[0]) != 2 {
		t.Fatalf("low-risk drift: %+v %v", got, err)
	}
	old, _ := e.Get(ctx, p.ID)
	if old.Status != hsync.StatusSuperseded {
		t.Errorf("old plan %s", old.Status)
	}

	f.draft = hsync.Draft{Items: []hsync.Item{high}}
	p, _ = e.Propose(ctx, "dns-records", "z1", hsync.TriggerSave, "admin")
	_ = e.Approve(ctx, p.ID, "admin", "")
	f.draft = hsync.Draft{Items: []hsync.Item{high, item(hsync.ActionDelete, hsync.High, "c")}}
	got, err = e.Apply(ctx, p.ID, "admin", "")
	if !errors.Is(err, hsync.ErrReplanned) || got.Status != hsync.StatusPending || len(f.applied) != 1 {
		t.Fatalf("new high-risk item: %+v %v applied %d", got, err, len(f.applied))
	}
}

func TestOneOpenPlanPerTarget(t *testing.T) {
	f := &fake{draft: hsync.Draft{Items: []hsync.Item{item(hsync.ActionDelete, hsync.High, "a")}}}
	e := newEngine(t, f)
	ctx := t.Context()
	a, _ := e.Propose(ctx, "dns-records", "z1", hsync.TriggerSave, "admin")
	same, _ := e.Propose(ctx, "dns-records", "z1", hsync.TriggerReconcile, "system")
	if same.ID != a.ID {
		t.Error("same draft created a second plan")
	}
	f.draft = hsync.Draft{Items: []hsync.Item{item(hsync.ActionDelete, hsync.High, "b")}}
	b, _ := e.Propose(ctx, "dns-records", "z1", hsync.TriggerSave, "admin")
	if old, _ := e.Get(ctx, a.ID); old.Status != hsync.StatusSuperseded || b.Status != hsync.StatusPending {
		t.Errorf("old %s new %s", old.Status, b.Status)
	}
	open, _ := e.Open(ctx)
	if len(open) != 1 || open[0].ID != b.ID {
		t.Errorf("open = %d", len(open))
	}
	if err := e.Discard(ctx, b.ID, "admin", ""); err != nil {
		t.Fatal(err)
	}
	if open, _ := e.Open(ctx); len(open) != 0 {
		t.Error("discarded plan still open")
	}
}

func TestApplyFailureIsRecorded(t *testing.T) {
	f := &fake{draft: hsync.Draft{Items: []hsync.Item{item(hsync.ActionCreate, hsync.Low, "a")}}, err: errors.New("provider said no")}
	e := newEngine(t, f)
	_, err := e.Auto(t.Context(), "dns-records", "z1", hsync.TriggerSave, "admin")
	if err == nil {
		t.Fatal("no error")
	}
	recent, _ := e.Recent(t.Context(), 10)
	if len(recent) != 1 || recent[0].Status != hsync.StatusFailed || recent[0].Error == "" {
		t.Fatalf("recent %+v", recent)
	}
}

// The lock table serialises applies on one connection.
func TestApplyHoldsTheLock(t *testing.T) {
	f := &fake{draft: hsync.Draft{Items: []hsync.Item{item(hsync.ActionCreate, hsync.Low, "a")}}}
	st := storetest.New(t)
	e := hsync.New(st, audit.New([]byte("k")), f)
	p, _ := e.Propose(t.Context(), "dns-records", "z1", hsync.TriggerSave, "admin")
	if _, err := st.Q.AcquireSyncLock(t.Context(), dbLock("dns:z1")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Apply(t.Context(), p.ID, "admin", ""); !errors.Is(err, hsync.ErrLocked) {
		t.Fatalf("err = %v", err)
	}
	if err := e.Start(t.Context()); err != nil { // a restart clears stale locks
		t.Fatal(err)
	}
	if _, err := e.Apply(t.Context(), p.ID, "admin", ""); err != nil {
		t.Fatal(err)
	}
}

func dbLock(key string) db.AcquireSyncLockParams {
	return db.AcquireSyncLockParams{Key: key, Holder: "someone", AcquiredAt: 1}
}
