package dns

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/store/db"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
)

// Subsystem names (sync_plans.subsystem).
const (
	// SubsystemRecords creates, changes and labels Hashi's entries (low risk, applies automatically).
	SubsystemRecords = "dns-records"
	// SubsystemRecordDeletes deletes RRSets Hashi no longer wants (high risk). It is its own plan so a delete waiting
	// for approval never holds back safe changes (invariant 16).
	SubsystemRecordDeletes = "dns-record-deletes"
	SubsystemPrune         = "dns-prune"
	SubsystemProbe         = "dns-probe"
)

// EntrySubsystems are the subsystems a saved entry or the background reconcile plans.
var EntrySubsystems = []string{SubsystemRecords, SubsystemRecordDeletes}

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
	ttl    int // the provider connection's TTL for RRSets Hashi creates (setup step 3)
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
	return conn{zoneID: zoneID, zone: Zone{ID: zrow.ProviderZoneID, Name: zrow.Name, TTL: int(zrow.ZoneTtl)}, p: p, caps: caps,
		ttl: int(prow.DefaultTtl)}, nil
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
	return []hsync.Subsystem{recordsSub{s: s}, recordsSub{s: s, deletes: true}, pruneSub{s}, probeSub{s}}
}

func lockKey(zoneID string) string { return "dns:" + zoneID }

// recordsSub plans desired entries (dns_records) against the observed RRSets Hashi owns. With deletes it keeps only
// the delete items, without only the rest (and the conflicts).
type recordsSub struct {
	s       *Service
	deletes bool
}

func (r recordsSub) Name() string {
	if r.deletes {
		return SubsystemRecordDeletes
	}
	return SubsystemRecords
}

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
	if err := s.ensureImportedEntries(ctx, zoneID, obs); err != nil {
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
	gens, err := s.generated(ctx, c.zone.Name)
	if err != nil {
		return hsync.Draft{}, err
	}
	wanted := map[rrsetKey]bool{}
	var desired, observedOwned []RRSet
	var conflicts []hsync.Item
	conflict := func(r RRSet, note string) {
		conflicts = append(conflicts, hsync.Item{Action: hsync.ActionConflict, Object: r.Name + " " + r.Type, After: toItemJSON(&r),
			Risk: hsync.Low, Note: note})
	}
	// A manual entry at a generated name wins (the admin decided); an imported one is taken over by the generated
	// record (a migrated service record, 11-delivery §7).
	userAt, genAt := map[string]bool{}, map[string][]string{} // name → generated types
	for _, rec := range recs {
		if rec.Origin == OriginUser {
			userAt[rec.RRSet.Name] = true
		}
	}
	var wants []RRSet
	for _, g := range gens {
		if userAt[g.RRSet.Name] {
			conflict(g.RRSet, "a manual DNS entry has this name; "+g.Source+" gets its record once that entry is deleted")
			continue
		}
		genAt[g.RRSet.Name] = append(genAt[g.RRSet.Name], g.RRSet.Type)
		// A record taken over keeps its TTL (a migration changes nothing but ownership); a new one gets the
		// connection's TTL.
		d := g.RRSet
		if o, ok := byKey[rrsetKey{d.Name, d.Type}]; ok {
			d.TTL = o.TTL
		} else {
			d.TTL = &c.ttl
		}
		wants = append(wants, d)
	}
	for _, rec := range recs {
		if rec.Origin == OriginImported && supersedes(genAt[rec.RRSet.Name], rec.RRSet.Type) {
			continue
		}
		d, ok, err := s.entryValues(ctx, rec, byKey)
		if err != nil {
			return hsync.Draft{}, err
		}
		if ok {
			wants = append(wants, d)
		} else {
			wanted[rrsetKey{d.Name, d.Type}] = true // unresolved and not at the provider yet: nothing to keep or delete
		}
	}
	for _, d := range wants {
		wanted[rrsetKey{d.Name, d.Type}] = true
	}
	for _, d := range wants {
		d.Owner = inst
		k := rrsetKey{d.Name, d.Type}
		o, exists := byKey[k]
		switch {
		case Protected(d.Type):
			conflict(d, d.Type+" records are never changed by Hashi")
		case exists && !owned[k].adopted():
			conflict(d, "exists at the provider and is not Hashi's; import it on the Import tab to manage it")
		case exists && o.Protected:
			conflict(d, "locked at the provider; unlock it in the provider console")
		case cnameClash(d, obs):
			conflict(d, "a CNAME cannot share its name with other record types (waits until the other RRSet is gone)")
		default:
			desired = append(desired, d)
		}
	}
	if err := s.ownGenerated(ctx, zoneID, gens, desired, owned, byKey); err != nil {
		return hsync.Draft{}, err
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
		name, typ := ch.Key()
		// A routing override record (03 §5.1) leaves in the same plan as the service record change that dropped it.
		ovr := ch.Op == OpDelete && overrideName(name) && owned[rrsetKey{name, typ}].origin == OriginManaged
		if (ch.Op == OpDelete && !ovr) != r.deletes {
			continue
		}
		note := ""
		switch {
		case ovr:
			note = "routing override record no service uses any more"
		case ch.Op == OpDelete:
			note = "no longer a Hashi entry"
		case ch.Op == OpLabel:
			note = "marks the record as Hashi's in the provider console"
		}
		it := changeItem(ch, note)
		if ovr {
			it.Risk = hsync.Low // a generated indirection: nothing points at it any more
		}
		items = append(items, it)
	}
	if r.deletes {
		return hsync.Draft{Title: "Delete DNS records in " + c.zone.Name, Items: items}, nil
	}
	items = append(items, conflicts...)
	return hsync.Draft{Title: "DNS records in " + c.zone.Name, Items: items}, nil
}

// ensureImportedEntries gives every imported RRSet without an entry one, copied from the observed RRSet (imports
// saved before entries existed, S2). Without it the planner would see an owned RRSet nobody wants and delete it.
func (s *Service) ensureImportedEntries(ctx context.Context, zoneID string, obs []Observed) error {
	return s.st.Tx(ctx, func(q *db.Queries) error {
		owned, err := ownership(ctx, q, zoneID)
		if err != nil {
			return err
		}
		recs, err := q.ListDNSRecords(ctx, zoneID)
		if err != nil {
			return err
		}
		have := make(map[rrsetKey]bool, len(recs))
		for _, r := range recs {
			have[rrsetKey{r.Name, r.Type}] = true
		}
		now := s.now().UnixMilli()
		for _, o := range obs {
			k := rrsetKey{o.Name, o.Type}
			if owned[k].origin != OriginImported || have[k] {
				continue
			}
			if err := q.InsertDNSRecord(ctx, recordParams(ulid.Make().String(), zoneID, o.RRSet, OriginImported, RecordInput{}, now)); err != nil {
				return err
			}
		}
		return nil
	})
}

// cnameClash reports whether d would share its name with a CNAME (or be a CNAME beside other types) that is still at
// the provider, whoever owns it: one Hashi wants gone leaves through the delete plan first, then d is planned.
func cnameClash(d RRSet, obs []Observed) bool {
	return slices.ContainsFunc(obs, func(o Observed) bool {
		return o.Name == d.Name && o.Type != d.Type && (d.Type == "CNAME" || o.Type == "CNAME")
	})
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
	gen, err := s.generatedKeys(ctx, zoneID)
	if err != nil {
		return "", err
	}
	return s.applyChanges(ctx, zoneID, changes, func(q *db.Queries, now int64, done []Change) error {
		for _, ch := range done {
			name, typ := ch.Key()
			k := db.DeleteImportedDNSRecordParams{ZoneID: zoneID, Name: name, Type: typ}
			if ch.Op == OpDelete {
				if err := q.DeleteDNSOwnership(ctx, db.DeleteDNSOwnershipParams{ZoneID: zoneID, Name: name, Type: typ}); err != nil {
					return err
				}
				// An imported entry is deleted only when a generated record took its name over (it is never
				// unwanted otherwise): its row goes with it.
				if err := q.DeleteImportedDNSRecord(ctx, k); err != nil {
					return err
				}
				continue
			}
			if gen[rrsetKey{name, typ}] {
				if err := q.SetDNSOwnershipManaged(ctx, db.SetDNSOwnershipManagedParams{UpdatedAt: now, ZoneID: zoneID, Name: name, Type: typ}); err != nil {
					return err
				}
				if err := q.DeleteImportedDNSRecord(ctx, k); err != nil {
					return err
				}
			}
			if err := q.SetDNSOwnershipApplied(ctx, db.SetDNSOwnershipAppliedParams{LastAppliedHash: ch.After.Hash(), UpdatedAt: now,
				ZoneID: zoneID, Name: name, Type: typ}); err != nil {
				return err
			}
		}
		return nil
	})
}

// applyChanges sends changes, verifies them, records the result with after(q, now, done) and refreshes the snapshot.
// When the apply or the read-back fails, the zone is read again and after gets the changes that did take effect, so
// an RRSet Hashi created stays Hashi's (01 §5.1 step 4); the error is returned.
func (s *Service) applyChanges(ctx context.Context, zoneID string, changes []Change, after func(q *db.Queries, now int64, done []Change) error) (string, error) {
	c, err := s.connect(ctx, zoneID)
	if err != nil {
		return "", err
	}
	plan := ProviderPlan{Changes: changes}
	err = c.p.Apply(ctx, c.zone, plan)
	if err == nil {
		err = c.p.Verify(ctx, c.zone, plan)
	}
	ctx = context.WithoutCancel(ctx)
	done := changes
	if err != nil {
		done = s.tookEffect(ctx, c, changes)
	}
	if len(done) > 0 {
		if terr := s.st.Tx(ctx, func(q *db.Queries) error { return after(q, s.now().UnixMilli(), done) }); terr != nil {
			return "", errors.Join(err, terr)
		}
	}
	if _, rerr := s.Refresh(ctx, zoneID); rerr != nil && err == nil {
		return "", fmt.Errorf("applied and verified, but refreshing the snapshot failed: %w", rerr)
	}
	if err != nil {
		return "", err
	}
	return summarize(changes) + ", verified", nil
}

// tookEffect reads the zone and returns the changes whose result is there: a delete whose RRSet is gone, anything
// else whose RRSet has exactly the planned content. A failed read returns none.
func (s *Service) tookEffect(ctx context.Context, c conn, changes []Change) []Change {
	rrsets, err := c.p.ListRRSets(ctx, c.zone)
	if err != nil {
		return nil
	}
	got := make(map[rrsetKey]RRSet, len(rrsets))
	for _, r := range rrsets {
		got[rrsetKey{r.Name, r.Type}] = r
	}
	var done []Change
	for _, ch := range changes {
		name, typ := ch.Key()
		r, ok := got[rrsetKey{name, typ}]
		if ch.Op == OpDelete && !ok || ch.Op != OpDelete && ok && r.Hash() == ch.After.Hash() {
			done = append(done, ch)
		}
	}
	return done
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

// prunable: never NS/SOA (invariant 2) or a locked RRSet, never a challenge name, never one Hashi owns other than
// through an import.
func prunable(o Observed) bool {
	return !Protected(o.Type) && !o.Protected && !IsChallenge(o.Name) && (o.Origin == "" || o.Origin == OriginImported)
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
	return s.applyChanges(ctx, zoneID, changes, func(q *db.Queries, _ int64, done []Change) error {
		for _, ch := range done {
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
	cerr := c.p.Apply(ctx, c.zone, create)
	if cerr == nil {
		cerr = c.p.Verify(ctx, c.zone, create)
	}
	// Delete whatever happened to the create: a failed create may still have created the probe (the provider
	// accepted it and only a later step failed), and deleting a missing RRSet is success. Leave nothing behind.
	ctx = context.WithoutCancel(ctx)
	derr := c.p.Apply(ctx, c.zone, del)
	if derr == nil {
		derr = c.p.Verify(ctx, c.zone, del)
	}
	if err := errors.Join(cerr, derr); err != nil {
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
			var parts []string
			for _, sub := range EntrySubsystems {
				p, err := eng.Auto(ctx, sub, z.ID, hsync.TriggerReconcile, audit.System)
				switch {
				case err != nil && !errors.Is(err, hsync.ErrReplanned):
					errs = append(errs, fmt.Errorf("%s %s: %w", z.Name, sub, err))
				case p != nil:
					parts = append(parts, fmt.Sprintf("%s %s (%d items)", sub, p.Status, len(p.Items)))
				}
			}
			if len(parts) == 0 {
				parts = []string{"in sync"}
			}
			sums = append(sums, z.Name+": "+strings.Join(parts, ", "))
		}
		return strings.Join(sums, "; "), errors.Join(errs...)
	}
}

// overrideName reports a service routing override record name, <entry>.ovr.on.<home> (03 §5.1).
func overrideName(name string) bool { return strings.Contains("."+name+".", ".ovr.on.") }

// supersedes reports whether generated types at a name take over an imported entry of type typ: the same type, or
// every type when either side is a CNAME (it cannot share its name, 03 §3). Other types at the name stay.
func supersedes(gen []string, typ string) bool {
	return slices.Contains(gen, typ) || len(gen) > 0 && (typ == "CNAME" || slices.Contains(gen, "CNAME"))
}

// genRecord is a generated record in zone-relative form.
type genRecord struct {
	RRSet  RRSet
	Source string
}

// generated returns the generator's records inside zone, normalized; nil without a generator.
func (s *Service) generated(ctx context.Context, zone string) ([]genRecord, error) {
	if s.generate == nil {
		return nil, nil
	}
	gs, err := s.generate(ctx, zone)
	if err != nil {
		return nil, fmt.Errorf("generated records: %w", err)
	}
	out := make([]genRecord, 0, len(gs))
	seen := map[rrsetKey]bool{}
	for _, g := range gs {
		name, err := relativeName(g.Name, zone)
		if err != nil {
			continue // outside the managed zone
		}
		vals := make([]string, 0, len(g.Values))
		for _, v := range g.Values {
			nv, err := normalizeValue(g.Type, name, v)
			if err != nil {
				return nil, fmt.Errorf("generated %s %s: %w", g.Name, g.Type, err)
			}
			vals = append(vals, nv)
		}
		r := Normalize(RRSet{Name: name, Type: g.Type, Values: vals})
		if k := (rrsetKey{r.Name, r.Type}); !seen[k] {
			seen[k] = true
			out = append(out, genRecord{RRSet: r, Source: g.Source})
		}
	}
	return out, nil
}

// generatedKeys is the set of generated name/type pairs of a zone, for Apply.
func (s *Service) generatedKeys(ctx context.Context, zoneID string) (map[rrsetKey]bool, error) {
	z, err := s.st.R.GetDNSZone(ctx, zoneID)
	if err != nil {
		return nil, err
	}
	gens, err := s.generated(ctx, z.Name)
	if err != nil {
		return nil, err
	}
	out := make(map[rrsetKey]bool, len(gens))
	for _, g := range gens {
		out[rrsetKey{g.RRSet.Name, g.RRSet.Type}] = true
	}
	return out, nil
}

// entryValues returns the RRSet an entry wants. An address-set entry takes the set's addresses of its family; when
// there are none (invariant 21) it keeps what the provider holds, and ok is false when the provider holds nothing.
func (s *Service) entryValues(ctx context.Context, rec Record, byKey map[rrsetKey]Observed) (RRSet, bool, error) {
	d := rec.RRSet
	if rec.ValueSetID == "" {
		return d, true, nil
	}
	if s.targets == nil {
		return d, false, errors.New("address-set values are not wired")
	}
	t, err := s.targets(ctx, "dns_record:"+rec.ID, rec.ValueSetID)
	if err != nil {
		return d, false, fmt.Errorf("%s %s: %w", d.Name, d.Type, err)
	}
	d.Values = nil
	for _, a := range t.Addresses {
		if a.Is4() == (d.Type == "A") {
			d.Values = append(d.Values, a.Unmap().String())
		}
	}
	if len(d.Values) > 0 {
		return Normalize(d), true, nil
	}
	o, ok := byKey[rrsetKey{d.Name, d.Type}]
	if !ok {
		return d, false, nil
	}
	d.Values = o.Values
	return Normalize(d), true, nil
}

// ownGenerated records ownership (origin managed) for generated RRSets about to be created, so Apply's backstop
// accepts them, and drops managed rows that were never applied and are no longer wanted. An RRSet that exists at the
// provider is never claimed here: only an apply makes it Hashi's (invariant 1).
func (s *Service) ownGenerated(ctx context.Context, zoneID string, gens []genRecord, desired []RRSet, owned map[rrsetKey]owner, byKey map[rrsetKey]Observed) error {
	want := map[rrsetKey]bool{}
	for _, d := range desired {
		want[rrsetKey{d.Name, d.Type}] = true
	}
	return s.st.Tx(ctx, func(q *db.Queries) error {
		now := s.now().UnixMilli()
		for _, g := range gens {
			k := rrsetKey{g.RRSet.Name, g.RRSet.Type}
			if _, exists := byKey[k]; !want[k] || exists || owned[k].present {
				continue
			}
			if err := q.InsertManagedDNSOwnership(ctx, db.InsertManagedDNSOwnershipParams{ZoneID: zoneID, Name: k.name, Type: k.typ,
				CreatedAt: now, UpdatedAt: now}); err != nil {
				return err
			}
		}
		for k, o := range owned {
			if o.origin == OriginManaged && o.appliedHash == "" && !want[k] {
				if err := q.DeleteStaleManagedOwnership(ctx, db.DeleteStaleManagedOwnershipParams{ZoneID: zoneID, Name: k.name, Type: k.typ}); err != nil {
					return err
				}
			}
		}
		return nil
	})
}
