package httpapi

import (
	"context"
	"errors"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/traefik"
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
		out := gen.ServiceTarget{Address: t.Address, Port: t.Port}
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
			PathMatch: gen.ServiceRoutePathMatch(r.PathMatch), PathValue: r.PathValue, Priority: r.Priority,
			RewriteMode: gen.ServiceRouteRewriteMode(r.RewriteMode), RewritePattern: r.RewritePattern, RewriteValue: r.RewriteValue})
	}
	return gen.Service{Id: s.ID, Name: s.Name, Slug: s.Slug, Kind: gen.ServiceKind(s.Kind), DomainMode: gen.ServiceDomainMode(s.DomainMode),
		HostName: s.HostName, Enabled: s.Enabled, Origin: gen.ServiceOrigin(s.Origin), OwnerFeature: s.OwnerFeature,
		Target: target(s.Target), TlsSkipVerify: s.TLSSkipVerify, TlsServerName: s.TLSServerName,
		TlsMinVersion: gen.ServiceTlsMinVersion(s.TLSMinVersion), AdvancedRoutes: s.AdvancedRoutes, Routes: routes,
		DashboardVisible: s.DashboardVisible, MonitoringVisible: s.MonitoringVisible, AdguardRewrite: s.AdGuardRewrite,
		InvalidReason: s.InvalidReason, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt}
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
		out := services.Target{Address: t.Address, Port: t.Port}
		if t.Weight != nil {
			out.Weight = *t.Weight
		}
		return out
	}
	s := services.Service{Name: in.Name, Slug: in.Slug, Kind: string(in.Kind), DomainMode: string(in.DomainMode),
		HostName: in.HostName, Enabled: b(in.Enabled, true), Target: target(in.Target), TLSSkipVerify: b(in.TlsSkipVerify, false),
		TLSServerName: str(in.TlsServerName), AdvancedRoutes: b(in.AdvancedRoutes, false),
		DashboardVisible: b(in.DashboardVisible, true), MonitoringVisible: b(in.MonitoringVisible, true),
		AdGuardRewrite: b(in.AdguardRewrite, true)}
	if in.TlsMinVersion != nil {
		s.TLSMinVersion = string(*in.TlsMinVersion)
	}
	if in.Routes != nil {
		for _, r := range *in.Routes {
			rt := services.Route{ID: str(r.Id), Enabled: r.Enabled, PathMatch: string(r.PathMatch), PathValue: r.PathValue,
				Priority: r.Priority, RewriteMode: string(r.RewriteMode), RewritePattern: r.RewritePattern, RewriteValue: r.RewriteValue}
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
func (s *Server) syncServices(ctx context.Context) ([]gen.SyncPlan, bool, error) {
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
		if p != nil {
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
			plans = append(plans, s.planBody(ctx, p))
		}
	}
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
	plans, pending, err := s.syncServices(ctx)
	return svc, plans, pending, err
}

func (s *Server) ListServices(ctx context.Context, _ gen.ListServicesRequestObject) (gen.ListServicesResponseObject, error) {
	all, err := services.List(ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	out := gen.ServiceList{Services: make([]gen.Service, 0, len(all))}
	for _, sv := range all {
		out.Services = append(out.Services, serviceBody(sv))
	}
	return gen.ListServices200JSONResponse(out), nil
}

func (s *Server) GetService(ctx context.Context, req gen.GetServiceRequestObject) (gen.GetServiceResponseObject, error) {
	sv, err := services.Get(ctx, s.Store.R, req.Id)
	if err != nil {
		return nil, serviceError(err)
	}
	return gen.GetService200JSONResponse(serviceBody(sv)), nil
}

func (s *Server) CreateService(ctx context.Context, req gen.CreateServiceRequestObject) (gen.CreateServiceResponseObject, error) {
	in := serviceFromInput(*req.Body)
	sv, plans, pending, err := s.changeService(ctx, "service.created", func(q *db.Queries) (services.Service, error) {
		created, err := services.Create(ctx, q, in, s.now())
		if err != nil {
			return created, err
		}
		return services.Get(ctx, q, created.ID)
	})
	if err != nil {
		return nil, err
	}
	return gen.CreateService200JSONResponse{Service: serviceBody(sv), Plans: plans, CertificatePending: pending}, nil
}

func (s *Server) UpdateService(ctx context.Context, req gen.UpdateServiceRequestObject) (gen.UpdateServiceResponseObject, error) {
	in := serviceFromInput(*req.Body)
	in.ID = req.Id
	sv, plans, pending, err := s.changeService(ctx, "service.updated", func(q *db.Queries) (services.Service, error) {
		if _, err := services.Update(ctx, q, in, s.now()); err != nil {
			return services.Service{}, err
		}
		return services.Get(ctx, q, req.Id)
	})
	if err != nil {
		return nil, err
	}
	return gen.UpdateService200JSONResponse{Service: serviceBody(sv), Plans: plans, CertificatePending: pending}, nil
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
	return gen.ServiceSaveResult{Service: serviceBody(sv), Plans: plans, CertificatePending: pending}, err
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
	b, err := traefik.EffectiveYAML(sv)
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
