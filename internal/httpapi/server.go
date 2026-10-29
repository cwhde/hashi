// Package httpapi implements the generated OpenAPI strict server (api/openapi.yaml).
package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/auth"
	"git.juzo.io/juzo/hashi/internal/certs"
	"git.juzo.io/juzo/hashi/internal/clientip"
	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/logring"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/setup"
	"git.juzo.io/juzo/hashi/internal/store"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/traefik"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// Server implements gen.StrictServerInterface. Fields are wired in internal/app.
type Server struct {
	Health   *Health
	Version  gen.Version
	Store    *store.Store
	Sessions *auth.Manager
	Passkeys *auth.Passkeys
	Setup    *setup.Service
	Settings *settings.Service
	DNS      *dns.Service
	Traefik  *traefik.Manager
	Certs    *certs.Manager
	IPSets   *ipsets.Service
	// CertsSoon runs the certificate job now (a new service name needs a certificate); nil in tests without a runner.
	CertsSoon func(ctx context.Context) error
	Sync      *hsync.Engine
	Vault     *vault.Vault
	Audit     *audit.Log
	IP        *clientip.Resolver
	Logs      *logring.Ring
	Log       *slog.Logger
}

var _ gen.StrictServerInterface = (*Server)(nil)

// Handler returns the admin API: request context → strict handlers with the scope middleware → problem errors.
func (s *Server) Handler(mux *http.ServeMux) http.Handler {
	strict := gen.NewStrictHandlerWithOptions(s, []gen.StrictMiddlewareFunc{s.requireScopes}, gen.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeProblem(w, badRequest(err.Error()))
		},
		ResponseErrorHandlerFunc: s.writeError,
	})
	gen.HandlerFromMux(strict, mux)
	return s.withRequest(mux)
}

// writeError renders handler errors: *Problem as-is, anything else as an opaque 500 (details only in the log).
func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var p *Problem
	if errors.As(err, &p) {
		writeProblem(w, p)
		return
	}
	s.Log.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	writeProblem(w, &Problem{Type: "/problems/internal", Title: "Internal error", Status: http.StatusInternalServerError,
		Detail: "Hashi could not complete the request; nothing was changed. See the log for details."})
}

func (s *Server) now() time.Time { return s.Sessions.Now() }
