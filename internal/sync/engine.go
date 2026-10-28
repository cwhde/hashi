// Package sync is the plan → approve → apply → verify engine every external state change flows through
// (01-platform.md §5, invariant 7). Subsystems (DNS records, DNS prune, the DNS write probe; Traefik and firewall later)
// compute drafts and apply exactly the items they drafted; the engine persists plans and items, decides approval by
// risk, serialises applies with the lock table, re-plans before applying, and records runs and audit entries.
package sync

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Risk of an item or plan (01 §5.2). A plan is high-risk when any item is.
type Risk string

// Risk levels.
const (
	Low  Risk = "low"
	High Risk = "high"
)

// Item actions. Conflict items are shown, never applied (03 §3: a conflict is never an overwrite).
const (
	ActionCreate   = "create"
	ActionUpdate   = "update"
	ActionDelete   = "delete"
	ActionLabel    = "label"
	ActionConflict = "conflict"
)

// Plan statuses.
const (
	StatusPending    = "pending"
	StatusApproved   = "approved"
	StatusApplying   = "applying"
	StatusApplied    = "applied"
	StatusFailed     = "failed"
	StatusDiscarded  = "discarded"
	StatusSuperseded = "superseded"
)

// Triggers.
const (
	TriggerSave      = "save"
	TriggerReconcile = "reconcile"
	TriggerManual    = "manual"
	TriggerProbe     = "probe"
	TriggerPrune     = "prune"
)

// AutoApprover is the approved_by of a low-risk plan.
const AutoApprover = "auto"

// Errors the API maps.
var (
	ErrNotFound  = errors.New("no such plan")
	ErrState     = errors.New("the plan is not in a state that allows this")
	ErrLocked    = errors.New("another apply for this connection is running")
	ErrReplanned = errors.New("the provider changed since the plan was made; a new plan needs approval")
	ErrUnknown   = errors.New("unknown subsystem")
)

// Item is one change with its exact before/after values (subsystem JSON; nil when absent).
type Item struct {
	Action string          `json:"action"`
	Object string          `json:"object"`
	Before json.RawMessage `json:"before,omitempty"`
	After  json.RawMessage `json:"after,omitempty"`
	Risk   Risk            `json:"risk"`
	Note   string          `json:"note,omitempty"`
}

// Draft is what a subsystem proposes: a title and the items.
type Draft struct {
	Title string
	Items []Item
}

// actionable reports whether the draft changes anything (conflicts alone change nothing).
func (d Draft) actionable() bool {
	return slices.ContainsFunc(d.Items, func(i Item) bool { return i.Action != ActionConflict })
}

// Subsystem computes and applies drafts for one kind of external state.
type Subsystem interface {
	// Name is the plan's subsystem (sync_plans.subsystem).
	Name() string
	// LockKey names the connection the target belongs to; every apply on it holds this key.
	LockKey(target string) string
	// Plan reads the observed state fresh and returns the draft for target. No item may touch what Hashi must never
	// change; subsystems leave it out or emit a conflict.
	Plan(ctx context.Context, target string) (Draft, error)
	// Apply applies exactly items (conflicts skipped), verifies by reading back, and returns a summary.
	Apply(ctx context.Context, target string, items []Item) (string, error)
}

// Plan is a stored plan with its items.
type Plan struct {
	ID, Subsystem, Target, Title, Trigger, Status, Hash, CreatedBy, ApprovedBy, Result, Error string
	Risk                                                                                      Risk
	CreatedAt                                                                                 time.Time
	ApprovedAt, FinishedAt                                                                    *time.Time
	Items                                                                                     []Item
}

// Engine runs plans for registered subsystems.
type Engine struct {
	st    *store.Store
	audit *audit.Log
	subs  map[string]Subsystem
	now   func() time.Time
}

// New returns an engine for subs.
func New(st *store.Store, a *audit.Log, subs ...Subsystem) *Engine {
	e := &Engine{st: st, audit: a, subs: map[string]Subsystem{}, now: time.Now}
	for _, s := range subs {
		e.subs[s.Name()] = s
	}
	return e
}

// Start clears what a previous process left behind: locks and plans stuck in "applying" (they failed; the next plan
// re-reads the provider).
func (e *Engine) Start(ctx context.Context) error {
	return e.st.Tx(ctx, func(q *db.Queries) error {
		if err := q.ClearSyncLocks(ctx); err != nil {
			return err
		}
		_, err := q.ResetInterruptedSyncPlans(ctx, sql.NullInt64{Int64: e.now().UnixMilli(), Valid: true})
		return err
	})
}

// Hash is the identity of a list of items: what is approved is exactly what is applied (01 §5.1 step 2).
func Hash(items []Item) string {
	b, _ := json.Marshal(items) // Items marshal without error
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// RiskOf is the plan risk of items.
func RiskOf(items []Item) Risk {
	if slices.ContainsFunc(items, func(i Item) bool { return i.Risk == High }) {
		return High
	}
	return Low
}

// Propose asks the subsystem for a draft and stores it as the target's open plan, superseding an older open plan.
// It returns nil when nothing would change. An open plan with the same items is returned as it is. Low-risk plans
// are approved automatically; high-risk ones wait for an admin (01 §5.1 step 2).
func (e *Engine) Propose(ctx context.Context, subsystem, target, trigger, actor string) (*Plan, error) {
	sub, ok := e.subs[subsystem]
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrUnknown, subsystem)
	}
	d, err := sub.Plan(ctx, target)
	if err != nil {
		return nil, err
	}
	var id string
	err = e.st.Tx(ctx, func(q *db.Queries) error {
		id, err = e.store(ctx, q, sub, target, trigger, actor, d, nil)
		return err
	})
	if err != nil || id == "" {
		return nil, err
	}
	return e.Get(ctx, id)
}

// store persists d as the open plan of (sub, target) unless an open plan with the same items exists. approvedHigh,
// when set, lists the high-risk items an admin already approved: a draft with no other high-risk items is approved.
func (e *Engine) store(ctx context.Context, q *db.Queries, sub Subsystem, target, trigger, actor string, d Draft, approvedHigh map[string]bool) (string, error) {
	open, err := q.GetOpenSyncPlan(ctx, db.GetOpenSyncPlanParams{Subsystem: sub.Name(), Target: target})
	hasOpen := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if !d.actionable() {
		if hasOpen && open.Status != StatusApplying {
			if err := e.finish(ctx, q, open.ID, StatusSuperseded, "nothing to change any more", ""); err != nil {
				return "", err
			}
		}
		return "", nil
	}
	h := Hash(d.Items)
	if hasOpen {
		if open.PlanHash == h && approvedHigh == nil {
			return open.ID, nil
		}
		if open.Status == StatusApplying && approvedHigh == nil {
			return "", ErrLocked
		}
		if err := e.finish(ctx, q, open.ID, StatusSuperseded, "replaced by a newer plan", ""); err != nil {
			return "", err
		}
	}
	risk := RiskOf(d.Items)
	now := e.now().UnixMilli()
	status, approvedBy, approvedAt := StatusPending, "", sql.NullInt64{}
	if risk == Low || coveredBy(d.Items, approvedHigh) {
		status, approvedAt = StatusApproved, sql.NullInt64{Int64: now, Valid: true}
		approvedBy = AutoApprover
		if approvedHigh != nil && risk == High {
			approvedBy = actor
		}
	}
	id := ulid.Make().String()
	if err := q.InsertSyncPlan(ctx, db.InsertSyncPlanParams{ID: id, Subsystem: sub.Name(), Target: target, Title: d.Title, Trigger: trigger,
		Risk: string(risk), Status: status, PlanHash: h, CreatedBy: actor, CreatedAt: now, ApprovedBy: approvedBy, ApprovedAt: approvedAt}); err != nil {
		return "", err
	}
	for i, it := range d.Items {
		if err := q.InsertSyncPlanItem(ctx, db.InsertSyncPlanItemParams{PlanID: id, Seq: int64(i), Action: it.Action, Object: it.Object,
			BeforeJson: string(it.Before), AfterJson: string(it.After), Risk: string(it.Risk), Note: it.Note}); err != nil {
			return "", err
		}
	}
	return id, e.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "sync.plan.created", Target: "sync_plan:" + id,
		Detail: map[string]any{"subsystem": sub.Name(), "target": target, "risk": risk, "items": len(d.Items), "status": status}})
}

// coveredBy reports whether every high-risk item of items is in approved (by item hash).
func coveredBy(items []Item, approved map[string]bool) bool {
	if approved == nil {
		return false
	}
	for _, it := range items {
		if it.Risk == High && !approved[Hash([]Item{it})] {
			return false
		}
	}
	return true
}

// Approve records an admin's approval of a pending plan (the API requires sync.apply and a fresh reauth).
func (e *Engine) Approve(ctx context.Context, id, actor, ip string) error {
	return e.st.Tx(ctx, func(q *db.Queries) error {
		n, err := q.ApproveSyncPlan(ctx, db.ApproveSyncPlanParams{ApprovedBy: actor, ApprovedAt: sql.NullInt64{Int64: e.now().UnixMilli(), Valid: true}, ID: id})
		if err != nil {
			return err
		}
		if n == 0 {
			return e.stateErr(ctx, q, id)
		}
		return e.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "sync.plan.approved", Target: "sync_plan:" + id, IP: ip})
	})
}

// Discard drops a pending or approved plan.
func (e *Engine) Discard(ctx context.Context, id, actor, ip string) error {
	return e.st.Tx(ctx, func(q *db.Queries) error {
		p, err := q.GetSyncPlan(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if p.Status != StatusPending && p.Status != StatusApproved {
			return fmt.Errorf("%w: it is %s", ErrState, p.Status)
		}
		if err := e.finish(ctx, q, id, StatusDiscarded, "discarded", ""); err != nil {
			return err
		}
		return e.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "sync.plan.discarded", Target: "sync_plan:" + id, IP: ip})
	})
}

// Apply runs an approved plan: lock the connection, re-plan from fresh observed state, apply, verify (01 §5.1 step 3–4).
// If the fresh draft differs, it replaces the plan: when its high-risk items were all approved already it is applied,
// otherwise it waits for approval and Apply returns ErrReplanned with that plan. The returned plan is the one that ran.
func (e *Engine) Apply(ctx context.Context, id, actor, ip string) (*Plan, error) {
	row, err := e.st.R.GetSyncPlan(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if row.Status != StatusApproved {
		return nil, fmt.Errorf("%w: it is %s, not approved", ErrState, row.Status)
	}
	sub, ok := e.subs[row.Subsystem]
	if !ok {
		return nil, fmt.Errorf("%w %q", ErrUnknown, row.Subsystem)
	}
	key, holder := sub.LockKey(row.Target), ulid.Make().String()
	n, err := e.st.Q.AcquireSyncLock(ctx, db.AcquireSyncLockParams{Key: key, Holder: holder, AcquiredAt: e.now().UnixMilli()})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrLocked
	}
	defer func() {
		_ = e.st.Q.ReleaseSyncLock(context.WithoutCancel(ctx), db.ReleaseSyncLockParams{Key: key, Holder: holder})
	}()

	old, err := e.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	fresh, err := sub.Plan(ctx, row.Target)
	if err != nil {
		return nil, e.fail(ctx, old, actor, ip, "", fmt.Errorf("re-plan: %w", err))
	}
	if Hash(fresh.Items) != old.Hash {
		approved := map[string]bool{}
		for _, it := range old.Items {
			if it.Risk == High {
				approved[Hash([]Item{it})] = true
			}
		}
		var newID string
		if err := e.st.Tx(ctx, func(q *db.Queries) error {
			newID, err = e.store(ctx, q, sub, row.Target, row.Trigger, actor, fresh, approved)
			return err
		}); err != nil {
			return nil, err
		}
		if newID == "" {
			return old, nil // nothing to change any more; the old plan was superseded
		}
		next, err := e.Get(ctx, newID)
		if err != nil {
			return nil, err
		}
		if next.Status != StatusApproved {
			return next, ErrReplanned
		}
		old = next
	}
	return e.run(ctx, sub, old, actor, ip)
}

// run applies p's items and records the outcome. The lock is held by the caller.
func (e *Engine) run(ctx context.Context, sub Subsystem, p *Plan, actor, ip string) (*Plan, error) {
	n, err := e.st.Q.SetSyncPlanStatus(ctx, db.SetSyncPlanStatusParams{Status: StatusApplying, ID: p.ID, FromStatus: StatusApproved})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, e.stateErr(ctx, e.st.R, p.ID)
	}
	runID := ulid.Make().String()
	start := e.now()
	if err := e.st.Q.InsertSyncRun(ctx, db.InsertSyncRunParams{ID: runID, PlanID: p.ID, StartedAt: start.UnixMilli()}); err != nil {
		return nil, err
	}
	summary, aerr := sub.Apply(ctx, p.Target, p.Items)
	if aerr != nil {
		return nil, e.fail(ctx, p, actor, ip, runID, aerr)
	}
	ctx = context.WithoutCancel(ctx) // the provider changed: record it even if the request went away
	err = e.st.Tx(ctx, func(q *db.Queries) error {
		if err := e.finish(ctx, q, p.ID, StatusApplied, summary, ""); err != nil {
			return err
		}
		if err := q.FinishSyncRun(ctx, db.FinishSyncRunParams{FinishedAt: sql.NullInt64{Int64: e.now().UnixMilli(), Valid: true},
			Outcome: "applied", Summary: summary, ID: runID}); err != nil {
			return err
		}
		return e.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "sync.plan.applied", Target: "sync_plan:" + p.ID, IP: ip,
			Detail: map[string]any{"subsystem": p.Subsystem, "target": p.Target, "items": len(p.Items), "summary": summary,
				"duration_ms": e.now().Sub(start).Milliseconds()}})
	})
	if err != nil {
		return nil, err
	}
	return e.Get(ctx, p.ID)
}

// fail marks p failed (and its run, if one started) and returns cause.
func (e *Engine) fail(ctx context.Context, p *Plan, actor, ip, runID string, cause error) error {
	ctx = context.WithoutCancel(ctx)
	msg := cause.Error()
	err := e.st.Tx(ctx, func(q *db.Queries) error {
		if err := e.finish(ctx, q, p.ID, StatusFailed, "", msg); err != nil {
			return err
		}
		if runID != "" {
			if err := q.FinishSyncRun(ctx, db.FinishSyncRunParams{FinishedAt: sql.NullInt64{Int64: e.now().UnixMilli(), Valid: true},
				Outcome: "failed", Error: msg, ID: runID}); err != nil {
				return err
			}
		}
		return e.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "sync.plan.failed", Target: "sync_plan:" + p.ID, IP: ip,
			Outcome: audit.Failure, Detail: map[string]any{"subsystem": p.Subsystem, "target": p.Target, "error": msg}})
	})
	return errors.Join(cause, err)
}

func (e *Engine) finish(ctx context.Context, q *db.Queries, id, status, result, errMsg string) error {
	return q.FinishSyncPlan(ctx, db.FinishSyncPlanParams{Status: status, FinishedAt: sql.NullInt64{Int64: e.now().UnixMilli(), Valid: true},
		Result: result, Error: errMsg, ID: id})
}

func (e *Engine) stateErr(ctx context.Context, q *db.Queries, id string) error {
	p, err := q.GetSyncPlan(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("%w: it is %s", ErrState, p.Status)
}

// Auto proposes a plan and applies it right away when it is low-risk (saves and background reconcile, invariant 16).
// A high-risk plan stays pending for approval and is returned as it is.
func (e *Engine) Auto(ctx context.Context, subsystem, target, trigger, actor string) (*Plan, error) {
	p, err := e.Propose(ctx, subsystem, target, trigger, actor)
	if err != nil || p == nil || p.Status != StatusApproved {
		return p, err
	}
	return e.Apply(ctx, p.ID, actor, "")
}

// Get returns a plan with its items.
func (e *Engine) Get(ctx context.Context, id string) (*Plan, error) {
	row, err := e.st.R.GetSyncPlan(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return e.load(ctx, row)
}

func (e *Engine) load(ctx context.Context, row db.SyncPlan) (*Plan, error) {
	items, err := e.st.R.ListSyncPlanItems(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	p := planFromRow(row)
	for _, it := range items {
		p.Items = append(p.Items, Item{Action: it.Action, Object: it.Object, Before: raw(it.BeforeJson), After: raw(it.AfterJson),
			Risk: Risk(it.Risk), Note: it.Note})
	}
	return p, nil
}

// Open lists plans that are pending, approved or applying, newest first.
func (e *Engine) Open(ctx context.Context) ([]*Plan, error) {
	rows, err := e.st.R.ListOpenSyncPlans(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*Plan, 0, len(rows))
	for _, r := range rows {
		p, err := e.load(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// Recent lists finished plans (applied, failed, discarded, superseded), newest first, without items.
func (e *Engine) Recent(ctx context.Context, limit int) ([]*Plan, error) {
	rows, err := e.st.R.ListRecentSyncPlans(ctx, int64(limit))
	if err != nil {
		return nil, err
	}
	out := make([]*Plan, 0, len(rows))
	for _, r := range rows {
		out = append(out, planFromRow(r))
	}
	return out, nil
}

func planFromRow(r db.SyncPlan) *Plan {
	return &Plan{ID: r.ID, Subsystem: r.Subsystem, Target: r.Target, Title: r.Title, Trigger: r.Trigger, Status: r.Status, Hash: r.PlanHash,
		CreatedBy: r.CreatedBy, ApprovedBy: r.ApprovedBy, Result: r.Result, Error: r.Error, Risk: Risk(r.Risk),
		CreatedAt: time.UnixMilli(r.CreatedAt).UTC(), ApprovedAt: msPtr(r.ApprovedAt), FinishedAt: msPtr(r.FinishedAt)}
}

func raw(s string) json.RawMessage {
	if s == "" {
		return nil
	}
	return json.RawMessage(s)
}

func msPtr(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := time.UnixMilli(v.Int64).UTC()
	return &t
}
