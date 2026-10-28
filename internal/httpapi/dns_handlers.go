package httpapi

import (
	"context"
	"errors"
	"net/http"

	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/setup"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// dnsError maps DNS service errors to problems. Provider messages never contain the token (the adapter builds them).
func dnsError(err error) error {
	switch {
	case errors.Is(err, dns.ErrInvalid), errors.Is(err, dns.ErrValidation):
		return invalid(err.Error())
	case errors.Is(err, dns.ErrNotConfigured):
		return conflict(err.Error())
	case errors.Is(err, dns.ErrNoZone):
		return notFound("No managed DNS zone with that ID.")
	case errors.Is(err, dns.ErrUnauthorized), errors.Is(err, dns.ErrRateLimited), errors.Is(err, dns.ErrZoneNotFound):
		return providerFailed(err.Error())
	}
	return err
}

func providerFailed(detail string) *Problem {
	return &Problem{Type: "/problems/provider", Title: "The provider did not answer as expected", Status: http.StatusBadGateway, Detail: detail}
}

func dnsProviderBody(p dns.ProviderInfo) gen.DNSProvider {
	return gen.DNSProvider{Id: p.ID, Type: gen.DNSProviderType(p.Type), DefaultTtl: p.DefaultTTL, Health: gen.DNSProviderHealth(p.Health),
		LastValidatedAt: p.LastValidatedAt, LastError: p.LastError}
}

func dnsZoneBody(z dns.ZoneInfo) gen.DNSZone {
	return gen.DNSZone{Id: z.ID, Name: z.Name, Provider: dnsProviderBody(z.Provider), ZoneTtl: z.ZoneTTL, RrsetCount: z.RRSetCount,
		LastRefreshedAt: z.LastRefreshedAt, LastRefreshError: z.LastRefreshError}
}

func dnsRRSetBody(z dns.ZoneInfo, o dns.Observed) gen.DNSRRSet {
	origin := o.Origin
	if origin == "" {
		origin = dns.Unmanaged
	}
	return gen.DNSRRSet{Name: o.Name, Fqdn: dns.FQDN(o.Name, z.Name), Type: o.Type, Ttl: o.TTL, EffectiveTtl: o.EffectiveTTL(z.ZoneTTL),
		Values: o.Values, Protected: o.Protected, Origin: gen.DNSOrigin(origin), ObservedAt: o.ObservedAt}
}

func dnsImportBody(z dns.ZoneInfo, rows []dns.ImportRow) gen.DNSImportTable {
	out := gen.DNSImportTable{Zone: dnsZoneBody(z), Rows: make([]gen.DNSImportRow, len(rows))}
	for i, o := range rows {
		r := gen.DNSImportRow{Rrset: dnsRRSetBody(z, o.Observed), AllowedDecisions: make([]gen.DNSDecision, len(o.Allowed))}
		for j, a := range o.Allowed {
			r.AllowedDecisions[j] = gen.DNSDecision(a)
		}
		if o.Decision != nil {
			r.Decision = ptr(gen.DNSDecision(*o.Decision))
		}
		out.Rows[i] = r
	}
	return out
}

func decisionInputs(b *gen.DNSImportDecisions) []dns.DecisionInput {
	out := make([]dns.DecisionInput, len(b.Decisions))
	for i, d := range b.Decisions {
		out[i] = dns.DecisionInput{Name: d.Name, Type: d.Type, Decision: dns.Decision(d.Decision)}
	}
	return out
}

func (s *Server) ListDNSZones(ctx context.Context, _ gen.ListDNSZonesRequestObject) (gen.ListDNSZonesResponseObject, error) {
	zones, err := s.DNS.Zones(ctx)
	if err != nil {
		return nil, err
	}
	out := gen.DNSZoneList{Zones: make([]gen.DNSZone, len(zones))}
	for i, z := range zones {
		out.Zones[i] = dnsZoneBody(z)
	}
	return gen.ListDNSZones200JSONResponse(out), nil
}

func (s *Server) RefreshDNSZone(ctx context.Context, req gen.RefreshDNSZoneRequestObject) (gen.RefreshDNSZoneResponseObject, error) {
	if _, err := s.DNS.Refresh(ctx, req.Id); err != nil {
		return nil, dnsError(err)
	}
	z, err := s.DNS.Zone(ctx, req.Id)
	if err != nil {
		return nil, dnsError(err)
	}
	return gen.RefreshDNSZone200JSONResponse(dnsZoneBody(z)), nil
}

func (s *Server) ListDNSRRSets(ctx context.Context, req gen.ListDNSRRSetsRequestObject) (gen.ListDNSRRSetsResponseObject, error) {
	var origin, q string
	if req.Params.Origin != nil {
		origin = string(*req.Params.Origin)
	}
	if req.Params.Q != nil {
		q = *req.Params.Q
	}
	z, err := s.DNS.Zone(ctx, req.Id)
	if err != nil {
		return nil, dnsError(err)
	}
	rows, err := s.DNS.RRSets(ctx, req.Id, origin, q)
	if err != nil {
		return nil, dnsError(err)
	}
	out := gen.DNSRRSetList{Zone: dnsZoneBody(z), Rrsets: make([]gen.DNSRRSet, len(rows))}
	for i, o := range rows {
		out.Rrsets[i] = dnsRRSetBody(z, o)
	}
	return gen.ListDNSRRSets200JSONResponse(out), nil
}

func (s *Server) GetDNSImport(ctx context.Context, req gen.GetDNSImportRequestObject) (gen.GetDNSImportResponseObject, error) {
	z, rows, err := s.DNS.ImportTable(ctx, req.ZoneId)
	if err != nil {
		return nil, dnsError(err)
	}
	return gen.GetDNSImport200JSONResponse(dnsImportBody(z, rows)), nil
}

func (s *Server) PutDNSImportDecisions(ctx context.Context, req gen.PutDNSImportDecisionsRequestObject) (gen.PutDNSImportDecisionsResponseObject, error) {
	ri := info(ctx)
	if err := s.DNS.SaveDecisions(ctx, s.actor(ri), ri.ipString(), req.ZoneId, decisionInputs(req.Body)); err != nil {
		return nil, dnsError(err)
	}
	z, rows, err := s.DNS.ImportTable(ctx, req.ZoneId)
	if err != nil {
		return nil, dnsError(err)
	}
	return gen.PutDNSImportDecisions200JSONResponse(dnsImportBody(z, rows)), nil
}

// ---- setup step 3 (02 §1.3) ----

// requireSetupOpen fails once setup is complete: setup endpoints are single-use.
func (s *Server) requireSetupOpen(ctx context.Context) error {
	done, err := s.Setup.Completed(ctx)
	if err != nil {
		return err
	}
	if done {
		return conflict(setup.ErrCompleted.Error())
	}
	return nil
}

// setupDNSBody is the step 3 state for resuming the wizard (no zone list, no token).
func (s *Server) setupDNSBody(ctx context.Context) (*gen.DNSConnectResult, error) {
	p, z, err := s.DNS.Current(ctx)
	if errors.Is(err, dns.ErrNotConfigured) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := &gen.DNSConnectResult{Provider: ptr(dnsProviderBody(*p)), Zones: []gen.DNSProviderZone{}}
	if z != nil {
		out.Zone = ptr(dnsZoneBody(*z))
	}
	return out, nil
}

func (s *Server) PutSetupDNSProvider(ctx context.Context, req gen.PutSetupDNSProviderRequestObject) (gen.PutSetupDNSProviderResponseObject, error) {
	ri := info(ctx)
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	g, err := settings.Get[settings.General](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	if !g.Configured() {
		return nil, conflict("Save the base settings (step 2) first: the zone is chosen from the root domain.")
	}
	in := dns.ConnectInput{Type: string(req.Body.Type), DefaultTTL: req.Body.DefaultTtl, RootDomain: g.RootDomain}
	if req.Body.Token != nil {
		in.Token = secrets.New(*req.Body.Token)
	}
	if req.Body.ZoneId != nil {
		in.ZoneID = *req.Body.ZoneId
	}
	res, err := s.DNS.Connect(ctx, s.actor(ri), ri.ipString(), in)
	if err != nil {
		if errors.Is(err, dns.ErrInvalid) || errors.Is(err, dns.ErrValidation) {
			// The step does not advance; record why (the message never contains the token).
			_ = s.Setup.MarkFailed(ctx, s.Store.Q, setup.StepDNS, err.Error())
		}
		return nil, dnsError(err)
	}
	out := gen.DNSConnectResult{Zones: make([]gen.DNSProviderZone, len(res.Zones))}
	for i, z := range res.Zones {
		out.Zones[i] = gen.DNSProviderZone{Id: z.ID, Name: z.Name}
	}
	if res.Zone == nil {
		return gen.PutSetupDNSProvider200JSONResponse(out), nil // nothing stored; the admin picks a zone
	}
	out.Provider, out.Zone = ptr(dnsProviderBody(*res.Provider)), ptr(dnsZoneBody(*res.Zone))
	inputs := map[string]any{"type": in.Type, "default_ttl": in.DefaultTTL, "zone": res.Zone.Name}
	if err := s.Setup.MarkInProgress(ctx, s.Store.Q, setup.StepDNS, inputs); err != nil {
		return nil, err
	}
	return gen.PutSetupDNSProvider200JSONResponse(out), nil
}

// setupZone is the zone step 3 works on.
func (s *Server) setupZone(ctx context.Context) (dns.ZoneInfo, error) {
	_, z, err := s.DNS.Current(ctx)
	if err != nil && !errors.Is(err, dns.ErrNotConfigured) {
		return dns.ZoneInfo{}, err
	}
	if z == nil {
		return dns.ZoneInfo{}, conflict("Validate the DNS provider and choose a zone first.")
	}
	return *z, nil
}

func (s *Server) GetSetupDNSImport(ctx context.Context, _ gen.GetSetupDNSImportRequestObject) (gen.GetSetupDNSImportResponseObject, error) {
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	z, err := s.setupZone(ctx)
	if err != nil {
		return nil, err
	}
	_, rows, err := s.DNS.ImportTable(ctx, z.ID)
	if err != nil {
		return nil, dnsError(err)
	}
	return gen.GetSetupDNSImport200JSONResponse(dnsImportBody(z, rows)), nil
}

func (s *Server) PutSetupDNSImport(ctx context.Context, req gen.PutSetupDNSImportRequestObject) (gen.PutSetupDNSImportResponseObject, error) {
	ri := info(ctx)
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	z, err := s.setupZone(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.DNS.SaveDecisions(ctx, s.actor(ri), ri.ipString(), z.ID, decisionInputs(req.Body)); err != nil {
		return nil, dnsError(err)
	}
	// Step 3 completes when the token validated, the zone resolved and the decisions are saved (02 §1 table).
	if err := s.Store.Tx(ctx, func(q *db.Queries) error {
		return s.Setup.MarkDone(ctx, q, setup.StepDNS, map[string]any{"type": z.Provider.Type, "zone": z.Name, "decisions": len(req.Body.Decisions)})
	}); err != nil {
		return nil, err
	}
	body, err := s.setupBody(ctx)
	if err != nil {
		return nil, err
	}
	return gen.PutSetupDNSImport200JSONResponse(body), nil
}
