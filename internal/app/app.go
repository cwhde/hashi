// Package app wires Hashi together: config, logging, store, jobs and the three listeners.
// It is the only package allowed to hold process-wide state.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"golang.org/x/sync/errgroup"

	"git.juzo.io/juzo/hashi/internal/httpapi"
	"git.juzo.io/juzo/hashi/internal/jobs"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store"
)

// Build information, set with -ldflags "-X git.juzo.io/juzo/hashi/internal/app.Version=…".
var (
	Version = "dev"
	Commit  = "unknown"
)

const shutdownGrace = 15 * time.Second

// NewLogger returns the JSON logger every component uses, wrapped by the redacting handler.
func NewLogger(w io.Writer, level slog.Level) *slog.Logger {
	return slog.New(secrets.NewRedactingHandler(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				a.Value = slog.TimeValue(a.Value.Time().UTC()) // invariant 6
			}
			return a
		},
	})))
}

// Run starts Hashi and blocks until ctx is cancelled or a listener fails.
// ready, when non-nil, receives the bound admin address once all listeners accept connections.
func Run(ctx context.Context, cfg Config, log *slog.Logger, ready func(adminAddr string)) error {
	log.Info("starting", "version", Version, "commit", Commit, "db_path", cfg.DBPath())

	st, err := store.Open(ctx, cfg.DBPath())
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	if err := st.Migrate(ctx, log.With("component", "store")); err != nil {
		return err
	}

	runner := jobs.New(st.Q, log)
	if err := runner.Register(ctx, jobs.Noop()); err != nil {
		return err
	}

	health := httpapi.NewHealth(map[string]httpapi.Check{"database": st.Ready})
	servers := []*server{{name: "admin", addr: cfg.AdminAddr, handler: adminMux(health)}}
	if cfg.PublicAppsAddr != "" {
		servers = append(servers, &server{name: "public-apps", addr: cfg.PublicAppsAddr, handler: publicMux(health, "Hashi apps")})
	}
	if cfg.PublicStatusAddr != "" {
		servers = append(servers, &server{name: "public-status", addr: cfg.PublicStatusAddr, handler: publicMux(health, "Hashi status")})
	}
	for _, s := range servers {
		if err := s.listen(ctx); err != nil {
			return err
		}
		log.Info("listening", "listener", s.name, "addr", s.ln.Addr().String())
	}
	if ready != nil {
		ready(servers[0].ln.Addr().String())
	}

	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error { return runner.Run(gctx) })
	for _, s := range servers {
		g.Go(func() error { return s.serve(log) })
	}
	g.Go(func() error {
		<-gctx.Done()
		health.Drain()
		log.Info("shutting down")
		sctx, cancel := context.WithTimeout(context.WithoutCancel(gctx), shutdownGrace)
		defer cancel()
		var errs []error
		for _, s := range servers {
			errs = append(errs, s.srv.Shutdown(sctx))
		}
		return errors.Join(errs...)
	})
	return g.Wait()
}

type server struct {
	name    string
	addr    string
	handler http.Handler
	ln      net.Listener
	srv     *http.Server
}

func (s *server) listen(ctx context.Context) error {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.addr)
	if err != nil {
		return fmt.Errorf("%s listener on %s: %w", s.name, s.addr, err)
	}
	s.ln = ln
	s.srv = &http.Server{
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	return nil
}

func (s *server) serve(log *slog.Logger) error {
	s.srv.ErrorLog = slog.NewLogLogger(log.With("listener", s.name).Handler(), slog.LevelWarn)
	if err := s.srv.Serve(s.ln); !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("%s listener: %w", s.name, err)
	}
	return nil
}

func adminMux(h *httpapi.Health) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.Healthz)
	mux.HandleFunc("GET /readyz", h.Readyz)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "Hashi "+Version+"\n")
	})
	return mux
}

// publicMux is the S0 placeholder for the public dashboard and status listeners (07-monitoring-public.md, S7).
func publicMux(h *httpapi.Health, title string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.Healthz)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, title+": not configured yet\n")
	})
	return mux
}
