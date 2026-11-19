package httpapi

import (
	"context"
	"errors"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/firewall"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/rules"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/traefik"
	"git.juzo.io/juzo/hashi/internal/usermw"
)

func serviceError(err error) error {
	var ie *services.InvalidError
	switch {
	case errors.As(err, &ie):
		return invalid(ie.Error())
	case errors.Is(err, services.ErrNotFound):
		return notFound("No such service.")
	case errors.Is(err, services.ErrSystem), errors.Is(err, services.ErrTaken):
		return conflict(err.Error())
	}
	return traefikError(err)
}

func serviceBody(s services.Service) gen.Service {
	target := func(t services.Target) gen.ServiceTarget {
		out := gen.ServiceTarget{Address: t.Address, SetId: strPtr(t.SetID), Port: t.Port}
		if t.AgentID != "" { // the agent stands for its hidden set (bug 17)
			out.SetId, out.AgentId = nil, &t.AgentID
		}
		if t.Weight > 0 {
			out.Weight = ptr(t.Weight)
		}
		return out
	}
	routes := make([]gen.ServiceRoute, 0, len(s.Routes))
	for _, r := range s.Routes {
		ts := make([]gen.ServiceTarget, 0, len(r.Targets))
		for _, t := range r.Targets {
			ts = append(ts, target(t))
		}
		routes = append(routes, gen.ServiceRoute{Id: ptr(r.ID), Enabled: r.Enabled, Targets: ts,
			Kind: routeKindBody(r.Kind), PathMatch: gen.ServiceRoutePathMatch(r.PathMatch), PathValue: r.PathValue, Priority: r.Priority,
			RewriteMode: gen.ServiceRouteRewriteMode(r.RewriteMode), RewritePattern: r.RewritePattern, RewriteValue: r.RewriteValue,
			Middlewares: ptr(names(r.Middlewares))})
	}
	return gen.Service{Id: s.ID, Name: s.Name, Slug: s.Slug, Kind: gen.ServiceKind(s.Kind), DomainMode: gen.ServiceDomainMode(s.DomainMode),
		HostName: s.HostName, Enabled: s.Enabled, Origin: gen.ServiceOrigin(s.Origin), OwnerFeature: s.OwnerFeature,
		Target: target(s.Target), TlsSkipVerify: s.TLSSkipVerify, TlsServerName: s.TLSServerName,
		TlsMinVersion: gen.ServiceTlsMinVersion(s.TLSMinVersion), AdvancedRoutes: s.AdvancedRoutes, Routes: routes,
		DashboardVisible: s.DashboardVisible, MonitoringVisible: s.MonitoringVisible, AdguardRewrite: s.AdGuardRewrite,
		EntryHostId: strPtr(s.EntryHostID), PublicPort: intPtr(s.PublicPort), ProxyProtocol: boolPtr(s.ProxyProtocol),
		InvalidReason: s.InvalidReason, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
		Traffic: trafficBody(s.Traffic), Middlewares: names(s.Middlewares)}
}

func names(ns []string) []string {
	if ns == nil {
		return []string{}
	}
	return ns
}

func headersBody(hs []services.Header) *[]gen.ServiceHeader {
	if len(hs) == 0 {
		return nil
	}
	out := make([]gen.ServiceHeader, 0, len(hs))
	for _, h := range hs {
		out = append(out, gen.ServiceHeader{Name: h.Name, Value: h.Value})
	}
	return &out
}

func trafficBody(t services.Traffic) gen.ServiceTraffic {
	host := gen.ServiceTrafficHost(t.Host)
	return gen.ServiceTraffic{Host: &host, HostValue: strPtr(t.HostValue), DropForwardedFor: boolPtr(t.DropForwardedFor),
		DropRealIp: boolPtr(t.DropRealIP), RequestHeaders: headersBody(t.RequestHeaders), ResponseHeaders: headersBody(t.ResponseHeaders),
		DialTimeout: intPtr(t.DialTimeout), ResponseHeaderTimeout: intPtr(t.ResponseHeaderTimeout), IdleTimeout: intPtr(t.IdleTimeout)}
}

func trafficFromBody(b gen.ServiceTraffic) services.Traffic {
	n := func(p *int) int {
		if p == nil {
			return 0
		}
		return *p
	}
	hs := func(p *[]gen.ServiceHeader) []services.Header {
		if p == nil {
			return nil
		}
		var out []services.Header
		for _, h := range *p {
			out = append(out, services.Header{Name: h.Name, Value: h.Value})
		}
		return out
	}
	t := services.Traffic{HostValue: str(b.HostValue), DropForwardedFor: b.DropForwardedFor != nil && *b.DropForwardedFor,
		DropRealIP: b.DropRealIp != nil && *b.DropRealIp, RequestHeaders: hs(b.RequestHeaders), ResponseHeaders: hs(b.ResponseHeaders),
		DialTimeout: n(b.DialTimeout), ResponseHeaderTimeout: n(b.ResponseHeaderTimeout), IdleTimeout: n(b.IdleTimeout)}
	if b.Host != nil {
		t.Host = string(*b.Host)
	}
	return t
}

func intPtr(n int) *int {
	if n == 0 {
		return nil
	}
	return &n
}

func boolPtr(b bool) *bool {
	if !b {
		return nil
	}
	return &b
}

// serviceFromInput maps the request onto a service; flags absent from the input default to on (dashboard,
// monitoring, AdGuard), like the add form.
func serviceFromInput(in gen.ServiceInput) services.Service {
	b := func(p *bool, def bool) bool {
		if p == nil {
			return def
		}
		return *p
	}
	target := func(t gen.ServiceTarget) services.Target {
		out := services.Target{Address: t.Address, SetID: str(t.SetId), AgentID: str(t.AgentId), Port: t.Port}
		if t.Weight != nil {
			out.Weight = *t.Weight
		}
		return out
	}
	s := services.Service{Name: in.Name, Slug: in.Slug, Kind: string(in.Kind), DomainMode: string(in.DomainMode),
		HostName: in.HostName, Enabled: b(in.Enabled, true), Target: target(in.Target), TLSSkipVerify: b(in.TlsSkipVerify, false),
		TLSServerName: str(in.TlsServerName), AdvancedRoutes: b(in.AdvancedRoutes, false),
		DashboardVisible: b(in.DashboardVisible, true), MonitoringVisible: b(in.MonitoringVisible, true),
		AdGuardRewrite: b(in.AdguardRewrite, true), EntryHostID: str(in.EntryHostId), ProxyProtocol: b(in.ProxyProtocol, false)}
	if in.PublicPort != nil {
		s.PublicPort = *in.PublicPort
	}
	if in.TlsMinVersion != nil {
		s.TLSMinVersion = string(*in.TlsMinVersion)
	}
	if in.Traffic != nil {
		s.Traffic = trafficFromBody(*in.Traffic)
	}
	if in.Middlewares != nil {
		s.Middlewares = *in.Middlewares
	}
	if in.Routes != nil {
		for _, r := range *in.Routes {
			rt := services.Route{ID: str(r.Id), Enabled: r.Enabled, Kind: routeKind(r.Kind), PathMatch: string(r.PathMatch), PathValue: r.PathValue,
				Priority: r.Priority, RewriteMode: string(r.RewriteMode), RewritePattern: r.RewritePattern, RewriteValue: r.RewriteValue}
			if r.Middlewares != nil {
				rt.Middlewares = *r.Middlewares
			}
			for _, t := range r.Targets {
				rt.Targets = append(rt.Targets, target(t))
			}
			s.Routes = append(s.Routes, rt)
		}
	}
	return s
}

// syncServices runs after every service change: each ready Traefik host is planned (every host receives every
// enabled service, 04 §7; low-risk plans apply right away) and, when a served name is not covered by the certificate
// yet, the certificate job runs now instead of at its next hour. The job distributes the new certificate afterwards.
// madeBy says whether a plan belongs in a save's answer: a plan that was already waiting before the save is not this
// save's doing and is not counted again (bug 76).
func madeBy(p *hsync.Plan, saveStarted time.Time) bool {
	return p.Status != hsync.StatusPending || !p.CreatedAt.Before(saveStarted)
}

func (s *Server) syncServices(ctx context.Context) ([]gen.SyncPlan, bool, error) {
	started := time.Now().Truncate(time.Millisecond)
	conns, err := s.Store.R.ListTraefikConnections(ctx)
	if err != nil {
		return nil, false, err
	}
	plans := []gen.SyncPlan{}
	for _, c := range conns {
		if !c.OwnershipConfirmedAt.Valid {
			continue
		}
		p, err := s.autoRun(ctx, traefik.Subsystem, c.ID)
		if errors.Is(err, traefik.ErrNotReady) {
			continue
		}
		if err != nil {
			return nil, false, traefikError(err)
		}
		if p != nil && madeBy(p, started) {
			plans = append(plans, s.planBody(ctx, p))
		}
	}
	// Ports 80/443 follow whether any HTTP service is served (05 §8): firewall hosts are planned too.
	if s.Firewall != nil {
		fw, err := s.Firewall.PlanAll(ctx, s.Sync, s.actor(info(ctx)))
		if err != nil {
			s.Log.Warn("firewall plans after a service change", "err", err)
		}
		for _, p := range fw {
			if madeBy(p, started) {
				plans = append(plans, s.planBody(ctx, p))
			}
		}
	}
	// Service records follow the service's name, targets and routing override (03 §5.2).
	plans = append(plans, s.syncRecords(ctx)...)
	pending := false
	if s.Certs != nil {
		if pending, err = s.Certs.Pending(ctx); err != nil {
			return nil, false, err
		}
		if pending && s.CertsSoon != nil {
			if err := s.CertsSoon(ctx); err != nil {
				s.Log.Warn("start the certificate job", "err", err)
			}
		}
	}
	return plans, pending, nil
}

// changeService runs one service change with its audit entry in a transaction, then syncs the hosts.
func (s *Server) changeService(ctx context.Context, action string, f func(q *db.Queries) (services.Service, error)) (services.Service, []gen.SyncPlan, bool, error) {
	ri := info(ctx)
	var svc services.Service
	err := s.Store.Tx(ctx, func(q *db.Queries) error {
		var err error
		if svc, err = f(q); err != nil {
			return err
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: action, IP: ri.ipString(), Target: "service:" + svc.ID,
			Detail: map[string]any{"name": svc.Name, "host_name": svc.HostName}})
	})
	if err != nil {
		return svc, nil, false, serviceError(err)
	}
	s.edgeChanged(ctx)
	plans, pending, err := s.syncServices(ctx)
	return svc, plans, pending, err
}

func (s *Server) ListServices(ctx context.Context, _ gen.ListServicesRequestObject) (gen.ListServicesResponseObject, error) {
	all, err := services.List(ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	waiting, err := s.waiting(ctx)
	if err != nil {
		return nil, err
	}
	out := gen.ServiceList{Services: make([]gen.Service, 0, len(all))}
	for _, sv := range all {
		b := serviceBody(sv)
		b.Waiting = waiting(sv)
		out.Services = append(out.Services, b)
	}
	return gen.ListServices200JSONResponse(out), nil
}

func (s *Server) GetService(ctx context.Context, req gen.GetServiceRequestObject) (gen.GetServiceResponseObject, error) {
	sv, err := services.Get(ctx, s.Store.R, req.Id)
	if err != nil {
		return nil, serviceError(err)
	}
	b, err := s.serviceNow(ctx, sv)
	if err != nil {
		return nil, err
	}
	return gen.GetService200JSONResponse(b), nil
}

// serviceNow is one service's body with its waiting flag.
func (s *Server) serviceNow(ctx context.Context, sv services.Service) (gen.Service, error) {
	waiting, err := s.waiting(ctx)
	if err != nil {
		return gen.Service{}, err
	}
	b := serviceBody(sv)
	b.Waiting = waiting(sv)
	return b, nil
}

// waiting reports, per service, whether an open plan still holds it back from its hosts: any Traefik host's plan
// (every Traefik host renders every service), and for TCP/UDP also the entry firewall host's plan, which opens the
// public port (bug 77). An off or invalid service is never waiting: there is nothing to publish.
func (s *Server) waiting(ctx context.Context) (func(services.Service) bool, error) {
	open, err := s.Sync.Open(ctx)
	if err != nil {
		return nil, err
	}
	return waitingFor(open), nil
}

func waitingFor(open []*hsync.Plan) func(services.Service) bool {
	traefikOpen, firewallOpen := false, map[string]bool{}
	for _, p := range open {
		switch p.Subsystem {
		case traefik.Subsystem:
			traefikOpen = true
		case firewall.Subsystem:
			firewallOpen[p.Target] = true
		}
	}
	return func(sv services.Service) bool {
		if !sv.Enabled || sv.InvalidReason != "" {
			return false
		}
		return traefikOpen || (sv.Stream() && firewallOpen[sv.EntryHostID])
	}
}

// checkStream refuses a stream port that belongs to something on a firewall host (05 §8).
func (s *Server) checkStream(ctx context.Context, in services.Service) error {
	if !in.Stream() || s.Firewall == nil {
		return nil
	}
	if err := s.Firewall.CheckStreamPort(ctx, in.Kind, in.PublicPort); err != nil {
		return firewallError(err)
	}
	return nil
}

func (s *Server) CreateService(ctx context.Context, req gen.CreateServiceRequestObject) (gen.CreateServiceResponseObject, error) {
	in := serviceFromInput(*req.Body)
	if req.Body.DashboardVisible == nil { // a new service without a choice follows the Dashboard settings (09 §2)
		d, err := settings.Get[settings.Dashboard](ctx, s.Store.R)
		if err != nil {
			return nil, err
		}
		in.DashboardVisible = d.NewServicesVisible
	}
	if err := s.checkStream(ctx, in); err != nil {
		return nil, err
	}
	sv, plans, pending, err := s.changeService(ctx, "service.created", func(q *db.Queries) (services.Service, error) {
		created, err := services.Create(ctx, q, in, s.now())
		if err == nil {
			err = rules.EnsureRoots(ctx, q, s.now())
		}
		if err != nil {
			return created, err
		}
		return services.Get(ctx, q, created.ID)
	})
	if err != nil {
		return nil, err
	}
	b, err := s.serviceNow(ctx, sv)
	if err != nil {
		return nil, err
	}
	return gen.CreateService200JSONResponse{Service: b, Plans: plans, CertificatePending: pending}, nil
}

func (s *Server) UpdateService(ctx context.Context, req gen.UpdateServiceRequestObject) (gen.UpdateServiceResponseObject, error) {
	in := serviceFromInput(*req.Body)
	in.ID = req.Id
	if err := s.checkStream(ctx, in); err != nil {
		return nil, err
	}
	sv, plans, pending, err := s.changeService(ctx, "service.updated", func(q *db.Queries) (services.Service, error) {
		if err := keepAbsent(ctx, q, req.Body, &in); err != nil {
			return services.Service{}, err
		}
		if _, err := services.Update(ctx, q, in, s.now()); err != nil {
			return services.Service{}, err
		}
		return services.Get(ctx, q, req.Id)
	})
	if err != nil {
		return nil, err
	}
	b, err := s.serviceNow(ctx, sv)
	if err != nil {
		return nil, err
	}
	return gen.UpdateService200JSONResponse{Service: b, Plans: plans, CertificatePending: pending}, nil
}

func (s *Server) DeleteService(ctx context.Context, req gen.DeleteServiceRequestObject) (gen.DeleteServiceResponseObject, error) {
	_, plans, pending, err := s.changeService(ctx, "service.deleted", func(q *db.Queries) (services.Service, error) {
		return services.Delete(ctx, q, req.Id)
	})
	if err != nil {
		return nil, err
	}
	return gen.DeleteService200JSONResponse{Plans: plans, CertificatePending: pending}, nil
}

func (s *Server) setEnabled(ctx context.Context, id string, on bool) (gen.ServiceSaveResult, error) {
	action := "service.disabled"
	if on {
		action = "service.enabled"
	}
	sv, plans, pending, err := s.changeService(ctx, action, func(q *db.Queries) (services.Service, error) {
		if _, err := services.SetEnabled(ctx, q, id, on, s.now()); err != nil {
			return services.Service{}, err
		}
		return services.Get(ctx, q, id)
	})
	if err != nil {
		return gen.ServiceSaveResult{}, err
	}
	b, err := s.serviceNow(ctx, sv)
	return gen.ServiceSaveResult{Service: b, Plans: plans, CertificatePending: pending}, err
}

func (s *Server) EnableService(ctx context.Context, req gen.EnableServiceRequestObject) (gen.EnableServiceResponseObject, error) {
	r, err := s.setEnabled(ctx, req.Id, true)
	if err != nil {
		return nil, err
	}
	return gen.EnableService200JSONResponse(r), nil
}

func (s *Server) DisableService(ctx context.Context, req gen.DisableServiceRequestObject) (gen.DisableServiceResponseObject, error) {
	r, err := s.setEnabled(ctx, req.Id, false)
	if err != nil {
		return nil, err
	}
	return gen.DisableService200JSONResponse(r), nil
}

func (s *Server) GetServiceEffective(ctx context.Context, req gen.GetServiceEffectiveRequestObject) (gen.GetServiceEffectiveResponseObject, error) {
	sv, err := services.Get(ctx, s.Store.R, req.Id)
	if err != nil {
		return nil, serviceError(err)
	}
	if sv.InvalidReason != "" {
		return nil, conflict("This service is not rendered: " + sv.InvalidReason)
	}
	if s.IPSets != nil {
		r, err := services.Resolve(ctx, []services.Service{sv}, s.IPSets.ResolveTarget)
		if err != nil {
			return nil, err
		}
		if sv = r[0]; sv.InvalidReason != "" {
			return nil, conflict("This service is not rendered: " + sv.InvalidReason)
		}
	}
	one := []services.Service{sv}
	if err := traefik.MarkFailClosed(ctx, s.Store.R, one); err != nil {
		return nil, err
	}
	every, err := settings.Get[settings.TraefikMiddlewares](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	_, userFile, err := usermw.Load(ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	if services.CheckMiddlewares(one, userFile, every.EveryService); one[0].InvalidReason != "" {
		return nil, conflict("This service is not rendered: " + one[0].InvalidReason)
	}
	b, err := traefik.EffectiveYAML(one[0], every.EveryService)
	if err != nil {
		return nil, err
	}
	return gen.GetServiceEffective200JSONResponse{Yaml: string(b)}, nil
}

func (s *Server) PlanTraefik(ctx context.Context, _ gen.PlanTraefikRequestObject) (gen.PlanTraefikResponseObject, error) {
	plans, pending, err := s.syncServices(ctx)
	if err != nil {
		return nil, err
	}
	return gen.PlanTraefik200JSONResponse{Plans: plans, CertificatePending: pending}, nil
}

// edgeChanged brings forward auth up to date with a service change.
func (s *Server) edgeChanged(ctx context.Context) {
	if s.Edge == nil {
		return
	}
	if err := s.Edge.Rebuild(ctx); err != nil {
		s.Log.Warn("forward auth after a service change", "err", err)
	}
}

// keepAbsent keeps the traffic options and middleware lists a save leaves out (older clients, other forms), so a PUT
// that does not know them never clears them.
func keepAbsent(ctx context.Context, q *db.Queries, body *gen.ServiceInput, in *services.Service) error {
	absent := body.Traffic == nil || body.Middlewares == nil
	if body.Routes != nil {
		for _, r := range *body.Routes {
			absent = absent || r.Middlewares == nil
		}
	}
	if !absent {
		return nil
	}
	cur, err := services.Get(ctx, q, in.ID)
	if err != nil {
		return err
	}
	if body.Traffic == nil {
		in.Traffic = cur.Traffic
	}
	if body.Middlewares == nil {
		in.Middlewares = cur.Middlewares
	}
	if body.Routes != nil {
		for i, r := range *body.Routes {
			if r.Middlewares != nil || r.Id == nil {
				continue
			}
			for _, c := range cur.Routes {
				if c.ID == *r.Id {
					in.Routes[i].Middlewares = c.Middlewares
				}
			}
		}
	}
	return nil
}

func routeKindBody(k string) *gen.ServiceRouteKind {
	if k == "" {
		return nil
	}
	v := gen.ServiceRouteKind(k)
	return &v
}

func routeKind(k *gen.ServiceRouteKind) string {
	if k == nil {
		return ""
	}
	return string(*k)
}

// GetServicePublication says where a service is published and why a part is missing (bug 20).
func (s *Server) GetServicePublication(ctx context.Context, req gen.GetServicePublicationRequestObject) (gen.GetServicePublicationResponseObject, error) {
	if s.Firewall == nil || s.IPSets == nil {
		return nil, serviceError(services.ErrNotFound)
	}
	p, err := s.Firewall.Publication(ctx, s.IPSets.ResolveTarget, req.Id)
	if err != nil {
		return nil, serviceError(err)
	}
	out := gen.ServicePublication{HostId: emptyNil(p.HostID), HostName: emptyNil(p.HostName), Rewrite: emptyNil(p.Rewrite),
		Reason: p.Reason, Records: []gen.PublishedRecord{}}
	for _, r := range p.Records {
		out.Records = append(out.Records, gen.PublishedRecord{Name: strings.TrimSuffix(r.Name, "."), Type: r.Type, Values: r.Values})
	}
	return gen.GetServicePublication200JSONResponse(out), nil
}
