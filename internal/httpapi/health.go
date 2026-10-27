// Package httpapi implements the generated OpenAPI strict server (api/openapi.yaml).
package httpapi

import (
	"context"
	"sync/atomic"
	"time"

	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
)

// Check is one readiness dependency. It returns nil when healthy.
type Check func(ctx context.Context) error

// Health backs /healthz (process alive) and /readyz (dependencies OK and not shutting down).
type Health struct {
	checks   map[string]Check
	draining atomic.Bool
}

// NewHealth returns a Health with the named readiness checks.
func NewHealth(checks map[string]Check) *Health { return &Health{checks: checks} }

// Drain makes /readyz fail so load balancers stop sending traffic before shutdown.
func (h *Health) Drain() { h.draining.Store(true) }

func (h *Health) ready(ctx context.Context) (gen.Health, bool) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	checks := map[string]string{}
	ok := true
	if h.draining.Load() {
		checks["shutdown"] = "draining"
		ok = false
	}
	for name, c := range h.checks {
		// Check errors are our own messages (schema version, ping); they carry no secrets.
		if err := c(ctx); err != nil {
			checks[name] = err.Error()
			ok = false
		} else {
			checks[name] = "ok"
		}
	}
	status := gen.Ok
	if !ok {
		status = gen.Unavailable
	}
	return gen.Health{Status: status, Checks: &checks}, ok
}

func (s *Server) GetHealthz(context.Context, gen.GetHealthzRequestObject) (gen.GetHealthzResponseObject, error) {
	return gen.GetHealthz200JSONResponse{Status: gen.Ok}, nil
}

func (s *Server) GetReadyz(ctx context.Context, _ gen.GetReadyzRequestObject) (gen.GetReadyzResponseObject, error) {
	body, ok := s.Health.ready(ctx)
	if !ok {
		return gen.GetReadyz503JSONResponse(body), nil
	}
	return gen.GetReadyz200JSONResponse(body), nil
}

func (s *Server) GetVersion(context.Context, gen.GetVersionRequestObject) (gen.GetVersionResponseObject, error) {
	return gen.GetVersion200JSONResponse(s.Version), nil
}
