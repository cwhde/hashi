package dns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/store/db"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
)

// Subsystem names (sync_plans.subsystem).
const (
	SubsystemRecords = "dns-records"
	SubsystemPrune   = "dns-prune"
	SubsystemProbe   = "dns-probe"
)

// ProbeName is the probe RRSet's relative name (03 §2 point 3): a leading "_" label is not a host name, and
// "_hashi-test" is Hashi's own, so no real record can match it.
const ProbeName = "_hashi-test"

// itemRRSet is the before/after JSON of a DNS plan item: exactly what is sent and verified.
type itemRRSet struct {
	Name   string   `json:"name"`
	Type   string   `json:"type"`
	TTL    *int     `json:"ttl"`
	Values []string `json:"values"`
	Owner  string   `json:"owner,omitempty"`
}

func toItemJSON(r *RRSet) json.RawMessage {
	if r == nil {
		return nil
	}
	b, _ := json.Marshal(itemRRSet{Name: r.Name, Type: r.Type, TTL: r.TTL, Values: r.Values, Owner: r.Owner})
	return b
}

func fromItemJSON(raw json.RawMessage) (*RRSet, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var it itemRRSet
	if err := json.Unmarshal(raw, &it); err != nil {
		return nil, err
	}
	r := Normalize(RRSet{Name: it.Name, Type: it.Type, TTL: it.TTL, Values: it.Values, Owner: it.Owner})
	return &r, nil
}

// changeItem turns a provider change into a plan item. Deletes are high risk (01 §5.2).
func changeItem(c Change, note string) hsync.Item {
	name, typ := c.Key()
	risk := hsync.Low
	if c.Op == OpDelete {
		risk = hsync.High
	}
	return hsync.Item{Action: string(c.Op), Object: name + " " + typ, Before: toItemJSON(c.Before), After: toItemJSON(c.After), Risk: risk, Note: note}
}

// itemChanges turns stored items back into provider changes, skipping conflicts.
func itemChanges(items []hsync.Item) ([]Change, error) {
	var out []Change
	for _, it := range items {
		if it.Action == hsync.ActionConflict {
			continue
		}
		before, err := fromItemJSON(it.Before)
		if err != nil {
			return nil, fmt.Errorf("item %s: %w", it.Object, err)
		}
		after, err := fromItemJSON(it.After)
		if err != nil {
			return nil, fmt.Errorf("item %s: %w", it.Object, err)
		}
		c := Change{Op: Op(it.Action), Before: before, After: after}
		if (c.Op == OpCreate) != (before == nil) || (c.Op == OpDelete) != (after == nil) {
			return nil, fmt.Errorf("item %s: %s with before=%t after=%t", it.Object, it.Action, before != nil, after != nil)
		}
		out = append(out, c)
	}
	return out, nil
}

// conn is an open provider connection for one managed zone.
type conn struct {
	zoneID string
	zone   Zone // the provider's zone
	p      Provider
	caps   Capabilities
}

// connect opens the zone's provider. It reads nothing.
func (s *Service) connect(ctx context.Context, zoneID string) (conn, error) {
	zrow, err := s.st.R.GetDNSZone(ctx, zoneID)
	if err != nil {
		return conn{}, fmt.Errorf("%w: %w", ErrNoZone, err)
	}
	prow, err := s.st.R.GetDNSProvider(ctx, zrow.ProviderID)
	if err != nil {
		return conn{}, err
	}
	token, err := s.token(ctx, providerInfo(prow))
	if err != nil {
		return conn{}, err
	}
	p, err := s.open(prow.Type, token)
	if err != nil {
		return conn{}, err
	}
	caps, err := p.Capabilities(ctx)
	if err != nil {
		return conn{}, err
	}
	return conn{zoneID: zoneID, zone: Zone{ID: zrow.ProviderZoneID, Name: zrow.Name, TTL: int(zrow.ZoneTtl)}, p: p, caps: caps}, nil
}

// fresh refreshes the snapshot from the provider (a plan always starts from fresh observed state, 01 §5.1) and returns
// it. A failed read is a failed plan: Hashi never writes when reading fails (invariant 3).
func (s *Service) fresh(ctx context.Context, zoneID string) ([]Observed, error) {
	if _, err := s.Refresh(ctx, zoneID); err != nil {
		return nil, fmt.Errorf("%w: reading the zone failed: %w", ErrValidation, err)
	}
	return s.observed(ctx, s.st.R, zoneID)
}

// Subsystems returns the DNS sync subsystems for the engine.
func (s *Service) Subsystems() []hsync.Subsystem {
	return []hsync.Subsystem{recordsSub{s}, pruneSub{s}, probeSub{s}}
}

func lockKey(zoneID string) string { return "dns:" + zoneID }

// recordsSub plans desired entries (dns_records) against the observed RRSets Hashi owns.
type recordsSub struct{ s *Service }

func (recordsSub) Name() string               { return SubsystemRecords }
func (recordsSub) LockKey(zone string) string { return lockKey(zone) }

func (r recordsSub) Plan(ctx context.Context, zoneID string) (hsync.Draft, error) {
	s := r.s
	c, err := s.connect(ctx, zoneID)
	if err != nil {
		return hsync.Draft{}, err
	}
	obs, err := s.fresh(ctx, zoneID)
	if err != nil {
		return hsync.Draft{}, err
	}
	owned, err := ownership(ctx, s.st.R, zoneID)
	if err != nil {
		return hsync.Draft{}, err
	}
	recs, err := s.Records(ctx, s.st.R, zoneID)
	if err != nil {
		return hsync.Draft{}, err
	}
	inst := ""
	if c.caps.OwnershipMarker {
		if inst, err = s.InstanceID(ctx); err != nil {
			return hsync.Draft{}, err
		}
	}
	byKey := make(map[rrsetKey]Observed, len(obs))
	for _, o := range obs {
		byKey[rrsetKey{o.Name, o.Type}] = o
	}
	wanted := map[rrsetKey]bool{}
	var desired, observedOwned []RRSet
	var conflicts []hsync.Item
	conflict := func(r RRSet, note string) {
		conflicts = append(conflicts, hsync.Item{Action: hsync.ActionConflict, Object: r.Name + " " + r.Type, After: toItemJSON(&r),
			Risk: hsync.Low, Note: note})
	}
	for _, rec := range recs {
		d := rec.RRSet
		d.Owner = inst
		k := rrsetKey{d.Name, d.Type}
		wanted[k] = true
		o, exists := byKey[k]
		switch {
		case Protected(d.Type):
			conflict(d, d.Type+" records are never changed by Hashi")
		case exists && !owned[k].adopted():
			conflict(d, "exists at the provider and is not Hashi's; import it on the Import tab to manage it")
		case exists && o.Protected:
			conflict(d, "locked at the provider; unlock it in the provider console")
		case cnameClash(d, obs, owned):
			conflict(d, "a CNAME cannot share its name with other record types")
		default:
			desired = append(desired, d)
		}
	}
	for _, o := range obs {
		k := rrsetKey{o.Name, o.Type}
		if !owned[k].adopted() || Protected(o.Type) {
			continue
		}
		if o.Protected {
			if !wanted[k] {
				conflict(o.RRSet, "Hashi no longer wants it, but it is locked at the provider")
			}
			continue
		}
		observedOwned = append(observedOwned, o.RRSet)
	}
	plan, err := c.p.Plan(ctx, c.zone, desired, observedOwned)
	if err != nil {
		return hsync.Draft{}, err
	}
	items := make([]hsync.Item, 0, len(plan.Changes)+len(conflicts))
	for _, ch := range plan.Changes {
		note := ""
		switch ch.Op {
		case OpDelete:
			note = "no longer a Hashi entry"
		case OpLabel:
			note = "marks the record as Hashi's in the provider console"
		}
		items = append(items, changeItem(ch, note))
	}
	items = append(items, conflicts...)
	return hsync.Draft{Title: "DNS records in " + c.zone.Name, Items: items}, nil
}

// cnameClash reports whether d would share its name with a CNAME (or be a CNAME beside other types) that stays.
func cnameClash(d RRSet, obs []Observed, owned map[rrsetKey]owner) bool {
	for _, o := range obs {
		if o.Name != d.Name || o.Type == d.Type || (d.Type != "CNAME" && o.Type != "CNAME") {
			continue
		}
		if !owned[rrsetKey{o.Name, o.Type}].adopted() {
			return true // an RRSet Hashi does not own stays
		}
	}
	return false
}

func (r recordsSub) Apply(ctx context.Context, zoneID string, items []hsync.Item) (string, error) {
	s := r.s
	changes, err := itemChanges(items)
	if err != nil {
		return "", err
	}
	owned, err := ownership(ctx, s.st.R, zoneID)
	if err != nil {
		return "", err
	}
	for _, ch := range changes { // invariant 1 backstop: only RRSets Hashi owns
		name, typ := ch.Key()
		o := owned[rrsetKey{name, typ}]
		if !o.present || (ch.Op != OpCreate && !o.adopted()) {
			return "", fmt.Errorf("%w: %s %s is not Hashi's", ErrForbiddenChange, name, typ)
		}
	}
	return s.applyChanges(ctx, zoneID, changes, func(q *db.Queries, now int64) error {
		for _, ch := range changes {
			name, typ := ch.Key()
			if ch.Op == OpDelete {
				if err := q.DeleteDNSOwnership(ctx, db.DeleteDNSOwnershipParams{ZoneID: zoneID, Name: name, Type: typ}); err != nil {
					return err
				}
				continue
			}
			if err := q.SetDNSOwnershipApplied(ctx, db.SetDNSOwnershipAppliedParams{LastAppliedHash: ch.After.Hash(), UpdatedAt: now,
				ZoneID: zoneID, Name: name, Type: typ}); err != nil {
				return err
			}
		}
		return nil
	})
}

// applyChanges sends changes, verifies them, records the result with after(q) and refreshes the snapshot.
func (s *Service) applyChanges(ctx context.Context, zoneID string, changes []Change, after func(q *db.Queries, now int64) error) (string, error) {
	c, err := s.connect(ctx, zoneID)
	if err != nil {
		return "", err
	}
	plan := ProviderPlan{Changes: changes}
	if err := c.p.Apply(ctx, c.zone, plan); err != nil {
		return "", err
	}
	if err := c.p.Verify(ctx, c.zone, plan); err != nil {
		return "", err
	}
	ctx = context.WithoutCancel(ctx)
	if err := s.st.Tx(ctx, func(q *db.Queries) error { return after(q, s.now().UnixMilli()) }); err != nil {
		return "", err
	}
	if _, err := s.Refresh(ctx, zoneID); err != nil {
		return "", fmt.Errorf("applied and verified, but refreshing the snapshot failed: %w", err)
	}
	return summarize(changes) + ", verified", nil
}

func summarize(changes []Change) string {
	n := map[Op]int{}
	for _, c := range changes {
		n[c.Op]++
	}
	var parts []string
	for _, op := range []Op{OpCreate, OpUpdate, OpLabel, OpDelete} {
		if n[op] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n[op], map[Op]string{OpCreate: "created", OpUpdate: "changed",
				OpLabel: "labelled", OpDelete: "deleted"}[op]))
		}
	}
	if len(parts) == 0 {
		return "no changes"
	}
	return strings.Join(parts, ", ")
}

// pruneSub deletes the RRSets marked "prune" on the import table, as one high-risk plan (03 §4).
type pruneSub struct{ s *Service }

func (pruneSub) Name() string               { return SubsystemPrune }
func (pruneSub) LockKey(zone string) string { return lockKey(zone) }

func (p pruneSub) Plan(ctx context.Context, zoneID string) (hsync.Draft, error) {
	s := p.s
	c, err := s.connect(ctx, zoneID)
	if err != nil {
		return hsync.Draft{}, err
	}
	obs, err := s.fresh(ctx, zoneID)
	if err != nil {
		return hsync.Draft{}, err
	}
	var targets []RRSet
	for _, o := range obs {
		if o.Decision != nil && *o.Decision == Prune && prunable(o) {
			targets = append(targets, o.RRSet)
		}
	}
	plan, err := c.p.Plan(ctx, c.zone, nil, targets)
	if err != nil {
		return hsync.Draft{}, err
	}
	items := make([]hsync.Item, 0, len(plan.Changes))
	for _, ch := range plan.Changes {
		items = append(items, changeItem(ch, "marked for removal on the import table"))
	}
	return hsync.Draft{Title: fmt.Sprintf("Remove %d records from %s", len(items), c.zone.Name), Items: items}, nil
}

// prunable: never NS/SOA (invariant 2) or a locked RRSet, never one Hashi owns other than through an import.
func prunable(o Observed) bool {
	return !Protected(o.Type) && !o.Protected && (o.Origin == "" || o.Origin == OriginImported)
}

func (p pruneSub) Apply(ctx context.Context, zoneID string, items []hsync.Item) (string, error) {
	s := p.s
	changes, err := itemChanges(items)
	if err != nil {
		return "", err
	}
	obs, err := s.observed(ctx, s.st.R, zoneID)
	if err != nil {
		return "", err
	}
	byKey := make(map[rrsetKey]Observed, len(obs))
	for _, o := range obs {
		byKey[rrsetKey{o.Name, o.Type}] = o
	}
	for _, ch := range changes { // still marked, still prunable
		name, typ := ch.Key()
		o, ok := byKey[rrsetKey{name, typ}]
		if ch.Op != OpDelete || !ok || o.Decision == nil || *o.Decision != Prune || !prunable(o) {
			return "", fmt.Errorf("%w: %s %s is no longer marked for removal", ErrForbiddenChange, name, typ)
		}
	}
	return s.applyChanges(ctx, zoneID, changes, func(q *db.Queries, _ int64) error {
		for _, ch := range changes {
			name, typ := ch.Key()
			if err := q.DeleteImportDecision(ctx, db.DeleteImportDecisionParams{ZoneID: zoneID, Name: name, Type: typ}); err != nil {
				return err
			}
			if err := q.DeleteImportedDNSRecord(ctx, db.DeleteImportedDNSRecordParams{ZoneID: zoneID, Name: name, Type: typ}); err != nil {
				return err
			}
			if _, err := q.DeleteImportedOwnership(ctx, db.DeleteImportedOwnershipParams{ZoneID: zoneID, Name: name, Type: typ}); err != nil {
				return err
			}
		}
		return nil
	})
}

// probeSub is the write test (02 §1.3, invariant 4): create a TXT probe RRSet, verify it, delete it, verify it is
// gone. It is only ever proposed on request, and its items are high risk, so it never runs without approval.
type probeSub struct{ s *Service }

func (probeSub) Name() string               { return SubsystemProbe }
func (probeSub) LockKey(zone string) string { return lockKey(zone) }

func probeRRSet() RRSet {
	return Normalize(RRSet{Name: ProbeName, Type: "TXT", Values: []string{"hashi write test"}})
}

func (p probeSub) Plan(ctx context.Context, zoneID string) (hsync.Draft, error) {
	s := p.s
	c, err := s.connect(ctx, zoneID)
	if err != nil {
		return hsync.Draft{}, err
	}
	obs, err := s.fresh(ctx, zoneID)
	if err != nil {
		return hsync.Draft{}, err
	}
	if slices.ContainsFunc(obs, func(o Observed) bool { return o.Name == ProbeName && o.Type == "TXT" }) {
		return hsync.Draft{}, fmt.Errorf("%w: a %s TXT record already exists in %s; remove it before testing write access",
			ErrConflict, ProbeName, c.zone.Name)
	}
	r := probeRRSet()
	note := "write test: created, read back, then deleted again"
	return hsync.Draft{Title: "Test write access to " + c.zone.Name, Items: []hsync.Item{
		{Action: hsync.ActionCreate, Object: ProbeName + " TXT", After: toItemJSON(&r), Risk: hsync.High, Note: note},
		{Action: hsync.ActionDelete, Object: ProbeName + " TXT", Before: toItemJSON(&r), Risk: hsync.High, Note: note},
	}}, nil
}

func (p probeSub) Apply(ctx context.Context, zoneID string, items []hsync.Item) (string, error) {
	s := p.s
	changes, err := itemChanges(items)
	if err != nil {
		return "", err
	}
	want := probeRRSet()
	if len(changes) != 2 || changes[0].Op != OpCreate || changes[1].Op != OpDelete ||
		changes[0].After.Hash() != want.Hash() || changes[1].Before.Hash() != want.Hash() {
		return "", fmt.Errorf("%w: not the probe plan", ErrForbiddenChange)
	}
	c, err := s.connect(ctx, zoneID)
	if err != nil {
		return "", err
	}
	create, del := ProviderPlan{Changes: changes[:1]}, ProviderPlan{Changes: changes[1:]}
	if err := c.p.Apply(ctx, c.zone, create); err != nil {
		return "", err
	}
	verr := c.p.Verify(ctx, c.zone, create)
	if err := c.p.Apply(ctx, c.zone, del); err != nil { // delete even when the read-back failed: leave nothing behind
		return "", errors.Join(verr, err)
	}
	if err := errors.Join(verr, c.p.Verify(ctx, c.zone, del)); err != nil {
		return "", err
	}
	if _, err := s.Refresh(context.WithoutCancel(ctx), zoneID); err != nil {
		return "", fmt.Errorf("probe passed, but refreshing the snapshot failed: %w", err)
	}
	return "write access confirmed: " + ProbeName + " TXT created, verified and deleted", nil
}

// Reconcile refreshes every managed zone and plans its entries (the dns.refresh job, invariant 16): low-risk changes
// apply without a browser session, high-risk ones stay pending for approval.
func (s *Service) Reconcile(eng *hsync.Engine) func(ctx context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		rows, err := s.st.R.ListDNSZones(ctx)
		if err != nil {
			return "", err
		}
		if len(rows) == 0 {
			return "no DNS zone configured", nil
		}
		var sums []string
		var errs []error
		for _, z := range rows {
			p, err := eng.Auto(ctx, SubsystemRecords, z.ID, hsync.TriggerReconcile, audit.System)
			switch {
			case err != nil && !errors.Is(err, hsync.ErrReplanned):
				errs = append(errs, fmt.Errorf("%s: %w", z.Name, err))
			case p == nil:
				sums = append(sums, z.Name+": in sync")
			default:
				sums = append(sums, fmt.Sprintf("%s: plan %s (%d items)", z.Name, p.Status, len(p.Items)))
			}
		}
		return strings.Join(sums, "; "), errors.Join(errs...)
	}
}
