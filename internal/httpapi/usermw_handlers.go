package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/traefik"
	"git.juzo.io/juzo/hashi/internal/usermw"
)

// User middleware file (04 §4) and the Traefik setting "middlewares on every service" (09 §2), edited together.

func userMiddlewareBodies(f usermw.File, uses map[string][]services.MiddlewareUse, every []string) []gen.UserMiddleware {
	out := make([]gen.UserMiddleware, 0, len(f.Middlewares))
	for _, m := range f.Middlewares {
		used := []gen.UserMiddlewareUse{}
		for _, u := range uses[m.Name] {
			used = append(used, gen.UserMiddlewareUse{ServiceId: u.ServiceID, ServiceName: u.ServiceName,
				RouteId: strPtr(u.RouteID), RoutePath: strPtr(u.RoutePath)})
		}
		out = append(out, gen.UserMiddleware{Name: m.Name, Type: m.Type, Line: m.Line, UsedBy: used,
			EveryService: slices.Contains(every, m.Name)})
	}
	return out
}

func (s *Server) userMiddlewares(ctx context.Context, q *db.Queries) (gen.UserMiddlewares, error) {
	text, f, err := usermw.Load(ctx, q)
	if err != nil {
		return gen.UserMiddlewares{}, err
	}
	every, err := settings.Get[settings.TraefikMiddlewares](ctx, q)
	if err != nil {
		return gen.UserMiddlewares{}, err
	}
	svcs, err := services.List(ctx, q)
	if err != nil {
		return gen.UserMiddlewares{}, err
	}
	out := gen.UserMiddlewares{Yaml: text, Middlewares: userMiddlewareBodies(f, services.MiddlewareUses(svcs), every.EveryService),
		EveryService: append([]string{}, every.EveryService...)}
	row, err := q.GetUserMiddlewares(ctx)
	if err == nil {
		t := time.UnixMilli(row.UpdatedAt).UTC()
		out.UpdatedAt = &t
	} else if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	return out, nil
}

func (s *Server) GetUserMiddlewares(ctx context.Context, _ gen.GetUserMiddlewaresRequestObject) (gen.GetUserMiddlewaresResponseObject, error) {
	out, err := s.userMiddlewares(ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	return gen.GetUserMiddlewares200JSONResponse(out), nil
}

func (s *Server) CheckUserMiddlewares(ctx context.Context, req gen.CheckUserMiddlewaresRequestObject) (gen.CheckUserMiddlewaresResponseObject, error) {
	f, err := usermw.Parse(req.Body.Yaml)
	var pe *usermw.Error
	if errors.As(err, &pe) {
		out := gen.UserMiddlewaresCheck{Ok: false, Error: &pe.Msg, Middlewares: []gen.UserMiddleware{}}
		if pe.Line > 0 {
			out.Line = &pe.Line
		}
		return gen.CheckUserMiddlewares200JSONResponse(out), nil
	}
	if err != nil {
		return nil, err
	}
	svcs, err := services.List(ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	every, err := settings.Get[settings.TraefikMiddlewares](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	return gen.CheckUserMiddlewares200JSONResponse{Ok: true,
		Middlewares: userMiddlewareBodies(f, services.MiddlewareUses(svcs), every.EveryService)}, nil
}

// stillUsed refuses a file that drops a name a service, a route or the every-service setting still uses (04 §4).
func stillUsed(f usermw.File, uses map[string][]services.MiddlewareUse, every []string) error {
	var missing []string
	for _, n := range every {
		if !f.Has(n) {
			missing = append(missing, n+" is set on every service (below the editor)")
		}
	}
	names := make([]string, 0, len(uses))
	for n := range uses {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		if f.Has(n) {
			continue
		}
		var where []string
		for _, u := range uses[n] {
			w := u.ServiceName
			if u.RouteID != "" {
				w += " (route " + u.RoutePath + ")"
			}
			where = append(where, w)
		}
		missing = append(missing, n+" is used by "+strings.Join(where, ", "))
	}
	if len(missing) > 0 {
		return invalid("The file no longer defines a middleware that is in use: " + strings.Join(missing, "; ") +
			". Remove it there first.")
	}
	return nil
}

func (s *Server) PutUserMiddlewares(ctx context.Context, req gen.PutUserMiddlewaresRequestObject) (gen.PutUserMiddlewaresResponseObject, error) {
	text, everyIn := req.Body.Yaml, req.Body.EveryService
	f, err := usermw.Parse(text)
	if err != nil {
		var pe *usermw.Error
		if errors.As(err, &pe) {
			return nil, invalid("The file was not saved: " + pe.Error())
		}
		return nil, err
	}
	every := settings.TraefikMiddlewares{EveryService: append([]string{}, everyIn...)}
	if err := every.Validate(); err != nil {
		return nil, invalid(strings.TrimPrefix(err.Error(), settings.ErrInvalid.Error()+": "))
	}
	for _, n := range every.EveryService {
		if !f.Has(n) {
			return nil, invalid(fmt.Sprintf("%s is set on every service but the file does not define it.", n))
		}
	}
	prevText, _, err := usermw.Load(ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	prevEvery, err := settings.Get[settings.TraefikMiddlewares](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	if err := s.storeUserMiddlewares(ctx, text, every, f, "traefik.user_middlewares_saved"); err != nil {
		return nil, err
	}
	plans, pending, err := s.syncServices(ctx)
	if err != nil {
		return nil, err
	}
	// Traefik is the authority: when it refused what was written, the previous file goes back on every host
	// (contracts traefik/traffic findings 5 and 9: a refused file takes every service down).
	if failed := failedTraefik(plans); failed != "" && (prevText != text || !slices.Equal(prevEvery.EveryService, every.EveryService)) {
		pf, perr := usermw.Parse(prevText)
		if perr == nil {
			perr = s.storeUserMiddlewares(ctx, prevText, prevEvery, pf, "traefik.user_middlewares_restored")
		}
		if perr != nil {
			return nil, fmt.Errorf("traefik refused the user middleware file (%s), and putting the previous one back failed: %w", failed, perr)
		}
		if _, _, err := s.syncServices(ctx); err != nil {
			return nil, err
		}
		return nil, invalid("Traefik refused the file, so the previous one was put back on every host: " + failed)
	}
	out, err := s.userMiddlewares(ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	return gen.PutUserMiddlewares200JSONResponse{File: out, Plans: plans, CertificatePending: pending}, nil
}

// storeUserMiddlewares saves the file and the setting in one transaction, after checking that nothing in use
// disappears, with an audit entry.
func (s *Server) storeUserMiddlewares(ctx context.Context, text string, every settings.TraefikMiddlewares, f usermw.File, action string) error {
	ri := info(ctx)
	return s.Store.Tx(ctx, func(q *db.Queries) error {
		svcs, err := services.List(ctx, q)
		if err != nil {
			return err
		}
		if err := stillUsed(f, services.MiddlewareUses(svcs), every.EveryService); err != nil {
			return err
		}
		if err := q.PutUserMiddlewares(ctx, db.PutUserMiddlewaresParams{Yaml: text, UpdatedAt: s.now().UnixMilli()}); err != nil {
			return err
		}
		if err := settings.Put(ctx, q, every); err != nil {
			return err
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: action, IP: ri.ipString(), Target: "traefik:" + usermw.FileName,
			Detail: map[string]any{"middlewares": f.Names(), "every_service": every.EveryService}})
	})
}

// failedTraefik describes the Traefik plans of a sync that failed; "" when none did.
func failedTraefik(plans []gen.SyncPlan) string {
	var out []string
	for _, p := range plans {
		if string(p.Subsystem) == traefik.Subsystem && p.Status == gen.SyncPlanStatus("failed") {
			out = append(out, p.TargetName+": "+p.Error)
		}
	}
	return strings.Join(out, "; ")
}
