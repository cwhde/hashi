package httpapi

import (
	"context"

	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

func wafFromBody(b gen.ServiceWAF) services.WAF {
	w := services.WAF{Exclusions: deref(b.Exclusions)}
	if b.Mode != nil {
		w.Mode = string(*b.Mode)
	}
	for _, p := range deref(b.Paths) {
		wp := services.WAFPath{ID: deref(p.Id), Match: string(p.Match), Path: p.Path, Exclusions: deref(p.Exclusions)}
		if p.Mode != nil {
			wp.Mode = string(*p.Mode)
		}
		w.Paths = append(w.Paths, wp)
	}
	return w
}

func optInts(v []int) *[]int {
	if len(v) == 0 {
		return nil
	}
	return &v
}

func optMode(m string) *gen.WAFMode {
	if m == "" {
		return nil
	}
	return ptr(gen.WAFMode(m))
}

// wafView shows what applies: a system service shows its fixed WAF (the default mode), not stored fields.
func wafView(sv services.Service, defaultMode string) gen.ServiceWAFView {
	w := sv.WAF
	if sv.System() {
		w = services.WAF{}
	}
	out := gen.ServiceWAFView{DefaultMode: defaultMode, Editable: !sv.System() && !sv.Stream(), Mode: optMode(w.Mode),
		Exclusions: optInts(w.Exclusions)}
	if len(w.Paths) > 0 {
		paths := make([]gen.ServiceWAFPath, 0, len(w.Paths))
		for _, p := range w.Paths {
			gp := gen.ServiceWAFPath{Id: ptr(p.ID), Match: gen.ServiceWAFPathMatch(p.Match), Path: p.Path,
				Mode: optMode(p.Mode), Exclusions: optInts(p.Exclusions)}
			paths = append(paths, gp)
		}
		out.Paths = &paths
	}
	return out
}

func (s *Server) wafDefault(ctx context.Context) (string, error) {
	e, err := settings.Get[settings.Engine](ctx, s.Store.R)
	return e.WAFDefaultMode, err
}

func (s *Server) GetServiceWAF(ctx context.Context, req gen.GetServiceWAFRequestObject) (gen.GetServiceWAFResponseObject, error) {
	sv, err := services.Get(ctx, s.Store.R, req.Id)
	if err != nil {
		return nil, serviceError(err)
	}
	def, err := s.wafDefault(ctx)
	if err != nil {
		return nil, err
	}
	return gen.GetServiceWAF200JSONResponse(wafView(sv, def)), nil
}

func (s *Server) SetServiceWAF(ctx context.Context, req gen.SetServiceWAFRequestObject) (gen.SetServiceWAFResponseObject, error) {
	in := wafFromBody(*req.Body)
	sv, plans, pending, err := s.changeService(ctx, "service.waf_updated", func(q *db.Queries) (services.Service, error) {
		if _, err := services.SetWAF(ctx, q, req.Id, in, s.now()); err != nil {
			return services.Service{}, err
		}
		return services.Get(ctx, q, req.Id)
	})
	if err != nil {
		return nil, err
	}
	def, err := s.wafDefault(ctx)
	if err != nil {
		return nil, err
	}
	return gen.SetServiceWAF200JSONResponse{Waf: wafView(sv, def), Plans: plans, CertificatePending: pending}, nil
}
