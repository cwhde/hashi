package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/certs"
	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/firewall"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/traefik"
)

// syncError maps engine and DNS write errors to problems.
func syncError(err error) error {
	switch {
	case errors.Is(err, hsync.ErrNotFound):
		return notFound("No plan with that ID.")
	case errors.Is(err, hsync.ErrState), errors.Is(err, hsync.ErrLocked), errors.Is(err, dns.ErrConflict):
		return conflict(err.Error())
	case errors.Is(err, dns.ErrForbiddenChange):
		return &Problem{Type: "/problems/forbidden-change", Title: "Hashi does not make this change", Status: http.StatusConflict, Detail: err.Error()}
	case errors.Is(err, dns.ErrNoRecord):
		return notFound("No DNS entry with that ID.")
	case errors.Is(err, dns.ErrVerify):
		return providerFailed(err.Error())
	}
	return dnsError(err)
}

func (s *Server) planBody(ctx context.Context, p *hsync.Plan) gen.SyncPlan {
	name := p.Target
	switch p.Subsystem {
	case traefik.Subsystem:
		if c, err := s.Store.R.GetTraefikConnection(ctx, p.Target); err == nil {
			name = c.Name
		}
	case firewall.Subsystem:
		if h, err := s.Store.R.GetFirewallHost(ctx, p.Target); err == nil {
			name = h.Name
		}
	case certs.Subsystem:
		if c, err := s.Store.R.GetCertificate(ctx, p.Target); err == nil {
			name = c.Name
		} else {
			name = "certificates"
		}
	default:
		if z, err := s.DNS.Zone(ctx, p.Target); err == nil {
			name = z.Name
		}
	}
	out := gen.SyncPlan{Id: p.ID, Subsystem: gen.SyncPlanSubsystem(p.Subsystem), Target: p.Target, TargetName: name, Title: p.Title,
		Trigger: gen.SyncPlanTrigger(p.Trigger), Risk: gen.SyncPlanRisk(p.Risk), Status: gen.SyncPlanStatus(p.Status),
		CreatedBy: p.CreatedBy, CreatedAt: p.CreatedAt, ApprovedBy: p.ApprovedBy, ApprovedAt: p.ApprovedAt, FinishedAt: p.FinishedAt,
		Result: p.Result, Error: p.Error, Items: make([]gen.SyncPlanItem, len(p.Items))}
	dnsItems := p.Subsystem != traefik.Subsystem && p.Subsystem != certs.Subsystem && p.Subsystem != firewall.Subsystem
	for i, it := range p.Items {
		out.Items[i] = gen.SyncPlanItem{Action: gen.SyncPlanItemAction(it.Action), Object: it.Object, Risk: gen.SyncPlanItemRisk(it.Risk),
			Note: it.Note}
		if dnsItems {
			out.Items[i].Before, out.Items[i].After = itemValue(it.Before), itemValue(it.After)
		} else {
			out.Items[i].BeforeText, out.Items[i].AfterText = itemText(it.Before), itemText(it.After)
		}
	}
	return out
}

// itemText summarizes a Traefik or certificate item's JSON (path, hash, mode, version, names, expiry) in one line;
// hashes are shortened, key material never appears in items.
func itemText(raw json.RawMessage) *string {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	var parts []string
	for _, k := range []string{"package", "version", "names", "paths", "to", "mode", "sha256", "hash", "fingerprint", "not_after"} {
		v, ok := m[k]
		if !ok || v == "" || v == nil {
			continue
		}
		switch x := v.(type) {
		case []any:
			var ss []string
			for _, e := range x {
				ss = append(ss, fmt.Sprint(e))
			}
			parts = append(parts, strings.Join(ss, ", "))
		case string:
			switch k {
			case "sha256", "hash", "fingerprint":
				if len(x) > 12 {
					x = x[:12] + "…"
				}
				parts = append(parts, "sha256 "+x)
			case "not_after":
				if t, err := time.Parse(time.RFC3339, x); err == nil {
					x = "valid until " + t.UTC().Format(time.DateOnly)
				}
				parts = append(parts, x)
			case "mode":
				parts = append(parts, "mode "+x)
			case "to":
				parts = append(parts, "to "+x)
			default:
				parts = append(parts, x)
			}
		}
	}
	if len(parts) == 0 {
		return nil
	}
	t := strings.Join(parts, " · ")
	return &t
}

func itemValue(raw json.RawMessage) *gen.SyncPlanItemValue {
	if len(raw) == 0 {
		return nil
	}
	var v gen.SyncPlanItemValue
	if json.Unmarshal(raw, &v) != nil {
		return nil
	}
	return &v
}

func (s *Server) planResult(ctx context.Context, p *hsync.Plan) gen.SyncPlanResult {
	if p == nil {
		return gen.SyncPlanResult{}
	}
	return gen.SyncPlanResult{Plan: ptr(s.planBody(ctx, p))}
}

func (s *Server) ListSyncPlans(ctx context.Context, _ gen.ListSyncPlansRequestObject) (gen.ListSyncPlansResponseObject, error) {
	open, err := s.Sync.Open(ctx)
	if err != nil {
		return nil, err
	}
	recent, err := s.Sync.Recent(ctx, 20)
	if err != nil {
		return nil, err
	}
	out := gen.SyncPlanOverview{Open: make([]gen.SyncPlan, len(open)), Recent: make([]gen.SyncPlan, len(recent))}
	for i, p := range open {
		out.Open[i] = s.planBody(ctx, p)
	}
	for i, p := range recent {
		out.Recent[i] = s.planBody(ctx, p)
	}
	return gen.ListSyncPlans200JSONResponse(out), nil
}

func (s *Server) GetSyncPlan(ctx context.Context, req gen.GetSyncPlanRequestObject) (gen.GetSyncPlanResponseObject, error) {
	p, err := s.Sync.Get(ctx, req.Id)
	if err != nil {
		return nil, syncError(err)
	}
	return gen.GetSyncPlan200JSONResponse(s.planBody(ctx, p)), nil
}

func (s *Server) ApproveSyncPlan(ctx context.Context, req gen.ApproveSyncPlanRequestObject) (gen.ApproveSyncPlanResponseObject, error) {
	ri := info(ctx)
	if err := s.Sync.Approve(ctx, req.Id, s.actor(ri), ri.ipString()); err != nil {
		return nil, syncError(err)
	}
	p, err := s.Sync.Get(ctx, req.Id)
	if err != nil {
		return nil, syncError(err)
	}
	return gen.ApproveSyncPlan200JSONResponse(s.planBody(ctx, p)), nil
}

// ApplySyncPlan returns the plan that ran. A failed apply is answered with the failed plan (its error says why), so the
// page can show the result; re-planning that needs a new approval answers with the new pending plan.
func (s *Server) ApplySyncPlan(ctx context.Context, req gen.ApplySyncPlanRequestObject) (gen.ApplySyncPlanResponseObject, error) {
	ri := info(ctx)
	p, err := s.Sync.Apply(ctx, req.Id, s.actor(ri), ri.ipString())
	switch {
	case err == nil, errors.Is(err, hsync.ErrReplanned):
		return gen.ApplySyncPlan200JSONResponse(s.planBody(ctx, p)), nil
	case errors.Is(err, hsync.ErrNotFound), errors.Is(err, hsync.ErrState), errors.Is(err, hsync.ErrLocked):
		return nil, syncError(err)
	}
	failed, gerr := s.Sync.Get(ctx, req.Id)
	if gerr != nil || failed.Status != hsync.StatusFailed {
		return nil, syncError(err)
	}
	return gen.ApplySyncPlan200JSONResponse(s.planBody(ctx, failed)), nil
}

func (s *Server) DiscardSyncPlan(ctx context.Context, req gen.DiscardSyncPlanRequestObject) (gen.DiscardSyncPlanResponseObject, error) {
	ri := info(ctx)
	if err := s.Sync.Discard(ctx, req.Id, s.actor(ri), ri.ipString()); err != nil {
		return nil, syncError(err)
	}
	p, err := s.Sync.Get(ctx, req.Id)
	if err != nil {
		return nil, syncError(err)
	}
	return gen.DiscardSyncPlan200JSONResponse(s.planBody(ctx, p)), nil
}

// ---- DNS entries and plans (03 §7) ----

func dnsRecordBody(r dns.Record) gen.DNSRecord {
	return gen.DNSRecord{Id: r.ID, ZoneId: r.ZoneID, Name: r.RRSet.Name, Type: r.RRSet.Type, Ttl: r.RRSet.TTL, Values: r.RRSet.Values,
		ValueSetId: strPtr(r.ValueSetID), Origin: gen.DNSOrigin(r.Origin), DisplayName: r.DisplayName, Dashboard: r.Dashboard, Monitoring: r.Monitoring}
}

// saved plans the zone after an entry changed: safe changes apply, a delete waits for approval. The entry stays saved
// when planning fails; the error is reported and the next sync retries. Plan is the plan this save caused: the
// delete plan for a deleted entry, the records plan otherwise (another entry's waiting delete is not this save's).
func (s *Server) saved(ctx context.Context, actor string, r dns.Record, deleted bool) gen.DNSRecordSaved {
	out := gen.DNSRecordSaved{Record: dnsRecordBody(r)}
	mine := dns.SubsystemRecords
	if deleted {
		mine = dns.SubsystemRecordDeletes
	}
	for _, sub := range dns.EntrySubsystems {
		p, err := s.Sync.Auto(ctx, sub, r.ZoneID, hsync.TriggerSave, actor)
		if p != nil && sub == mine {
			out.Plan = ptr(s.planBody(ctx, p))
		}
		if err != nil && !errors.Is(err, hsync.ErrReplanned) {
			out.PlanError = ptr(err.Error())
			break
		}
	}
	return out
}

// syncRecords plans every zone's entries and generated records after something they derive from changed (a firewall
// host, a service): low-risk changes apply, deletes wait for approval. Failures are logged; the next sync retries.
func (s *Server) syncRecords(ctx context.Context) []gen.SyncPlan {
	if s.DNS == nil {
		return nil
	}
	zones, err := s.Store.R.ListDNSZones(ctx)
	if err != nil {
		s.Log.Warn("DNS plans after a change", "err", err)
		return nil
	}
	var out []gen.SyncPlan
	for _, z := range zones {
		for _, sub := range dns.EntrySubsystems {
			p, err := s.Sync.Auto(ctx, sub, z.ID, hsync.TriggerSave, s.actor(info(ctx)))
			if err != nil && !errors.Is(err, hsync.ErrReplanned) {
				s.Log.Warn("DNS plans after a change", "zone", z.Name, "err", err)
				break
			}
			if p != nil {
				out = append(out, s.planBody(ctx, p))
			}
		}
	}
	return out
}

// proposePrune plans the removals marked on the import table (03 §4: a separate high-risk plan). A failure is not the
// save's failure: the decisions are stored and the admin can plan again from the Import tab.
func (s *Server) proposePrune(ctx context.Context, zoneID, actor string) {
	if _, err := s.Sync.Propose(ctx, dns.SubsystemPrune, zoneID, hsync.TriggerPrune, actor); err != nil {
		s.Log.Warn("planning removals failed", "zone", zoneID, "err", err)
	}
}

func recordInput(name, typ string, ttl *int, values []string, set, display *string, dashboard, monitoring *bool) dns.RecordInput {
	in := dns.RecordInput{Name: name, Type: typ, TTL: ttl, Values: values, ValueSetID: str(set)}
	if display != nil {
		in.DisplayName = *display
	}
	if dashboard != nil {
		in.Dashboard = *dashboard
	}
	if monitoring != nil {
		in.Monitoring = *monitoring
	}
	return in
}

func (s *Server) CreateDNSRecord(ctx context.Context, req gen.CreateDNSRecordRequestObject) (gen.CreateDNSRecordResponseObject, error) {
	ri := info(ctx)
	b := req.Body
	r, err := s.DNS.CreateRecord(ctx, s.actor(ri), ri.ipString(), b.ZoneId,
		recordInput(b.Name, string(b.Type), b.Ttl, b.Values, b.ValueSetId, b.DisplayName, b.Dashboard, b.Monitoring))
	if err != nil {
		return nil, syncError(err)
	}
	return gen.CreateDNSRecord200JSONResponse(s.saved(ctx, s.actor(ri), r, false)), nil
}

func (s *Server) UpdateDNSRecord(ctx context.Context, req gen.UpdateDNSRecordRequestObject) (gen.UpdateDNSRecordResponseObject, error) {
	ri := info(ctx)
	b := req.Body
	r, err := s.DNS.UpdateRecord(ctx, s.actor(ri), ri.ipString(), req.Id,
		recordInput("", "", b.Ttl, b.Values, b.ValueSetId, b.DisplayName, b.Dashboard, b.Monitoring))
	if err != nil {
		return nil, syncError(err)
	}
	return gen.UpdateDNSRecord200JSONResponse(s.saved(ctx, s.actor(ri), r, false)), nil
}

func (s *Server) DeleteDNSRecord(ctx context.Context, req gen.DeleteDNSRecordRequestObject) (gen.DeleteDNSRecordResponseObject, error) {
	ri := info(ctx)
	r, err := s.DNS.DeleteRecord(ctx, s.actor(ri), ri.ipString(), req.Id)
	if err != nil {
		return nil, syncError(err)
	}
	return gen.DeleteDNSRecord200JSONResponse(s.saved(ctx, s.actor(ri), r, true)), nil
}

// PlanDNS plans the zone's entries now; the result is the waiting delete plan when there is one, else the plan that ran.
func (s *Server) PlanDNS(ctx context.Context, req gen.PlanDNSRequestObject) (gen.PlanDNSResponseObject, error) {
	var shown *hsync.Plan
	for _, sub := range dns.EntrySubsystems {
		p, err := s.Sync.Auto(ctx, sub, req.Body.ZoneId, hsync.TriggerManual, s.actor(info(ctx)))
		if err != nil && !errors.Is(err, hsync.ErrReplanned) {
			return nil, syncError(err)
		}
		if p != nil && (shown == nil || p.Status == hsync.StatusPending) {
			shown = p
		}
	}
	return gen.PlanDNS200JSONResponse(s.planResult(ctx, shown)), nil
}

func (s *Server) PlanDNSPrune(ctx context.Context, req gen.PlanDNSPruneRequestObject) (gen.PlanDNSPruneResponseObject, error) {
	p, err := s.Sync.Propose(ctx, dns.SubsystemPrune, req.Body.ZoneId, hsync.TriggerPrune, s.actor(info(ctx)))
	if err != nil {
		return nil, syncError(err)
	}
	return gen.PlanDNSPrune200JSONResponse(s.planResult(ctx, p)), nil
}

func (s *Server) PlanDNSProbe(ctx context.Context, req gen.PlanDNSProbeRequestObject) (gen.PlanDNSProbeResponseObject, error) {
	p, err := s.Sync.Propose(ctx, dns.SubsystemProbe, req.Body.ZoneId, hsync.TriggerProbe, s.actor(info(ctx)))
	if err != nil {
		return nil, syncError(err)
	}
	return gen.PlanDNSProbe200JSONResponse(s.planResult(ctx, p)), nil
}

// ---- setup step 3 write test (02 §1.3) ----

func (s *Server) PostSetupDNSProbe(ctx context.Context, _ gen.PostSetupDNSProbeRequestObject) (gen.PostSetupDNSProbeResponseObject, error) {
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	z, err := s.setupZone(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.Sync.Propose(ctx, dns.SubsystemProbe, z.ID, hsync.TriggerProbe, s.actor(info(ctx)))
	if err != nil {
		return nil, syncError(err)
	}
	return gen.PostSetupDNSProbe200JSONResponse(s.planBody(ctx, p)), nil
}

// RunSetupDNSProbe approves and runs the probe plan the wizard showed. During setup there is no passkey yet, so the
// explicit click on the shown plan is the confirmation (invariant 4); only probe plans of the setup zone qualify.
func (s *Server) RunSetupDNSProbe(ctx context.Context, req gen.RunSetupDNSProbeRequestObject) (gen.RunSetupDNSProbeResponseObject, error) {
	ri := info(ctx)
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	z, err := s.setupZone(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.Sync.Get(ctx, req.Id)
	if err != nil {
		return nil, syncError(err)
	}
	if p.Subsystem != dns.SubsystemProbe || p.Target != z.ID {
		return nil, notFound("No write test plan with that ID.")
	}
	if p.Status == hsync.StatusPending {
		if err := s.Sync.Approve(ctx, p.ID, s.actor(ri), ri.ipString()); err != nil {
			return nil, syncError(err)
		}
	}
	ran, err := s.Sync.Apply(ctx, p.ID, s.actor(ri), ri.ipString())
	if err != nil {
		if errors.Is(err, hsync.ErrState) || errors.Is(err, hsync.ErrLocked) {
			return nil, syncError(err)
		}
		if ran, err = s.Sync.Get(ctx, p.ID); err != nil {
			return nil, syncError(err)
		}
	}
	return gen.RunSetupDNSProbe200JSONResponse(s.planBody(ctx, ran)), nil
}
