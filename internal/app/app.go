// Package app wires Hashi together: config, logging, store, jobs and the three listeners.
// It is the only package allowed to hold process-wide state.
package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/auth"
	"git.juzo.io/juzo/hashi/internal/certs"
	"git.juzo.io/juzo/hashi/internal/clientip"
	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/dns/hetzner"
	"git.juzo.io/juzo/hashi/internal/firewall"
	"git.juzo.io/juzo/hashi/internal/httpapi"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/jobs"
	"git.juzo.io/juzo/hashi/internal/logring"
	"git.juzo.io/juzo/hashi/internal/monitor"
	"git.juzo.io/juzo/hashi/internal/notify"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/setup"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/traefik"
	"git.juzo.io/juzo/hashi/internal/vault"
	"git.juzo.io/juzo/hashi/internal/web"
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
	if cfg.Stdout == nil {
		cfg.Stdout = os.Stdout
	}
	if cfg.Logs == nil {
		cfg.Logs = logring.New(1)
	}

	st, err := store.Open(ctx, cfg.DBPath())
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()
	if err := st.Migrate(ctx, log.With("component", "store")); err != nil {
		return err
	}

	// Service mode: no master key, no start (02 §5.2).
	master, err := vault.LoadMasterKey(cfg.MasterKeyFile)
	if err != nil {
		return err
	}
	v, err := vault.New(master, 1)
	if err != nil {
		return err
	}
	if err := v.CheckKey(ctx, st.Q); err != nil {
		return err
	}

	al := audit.New(v.Operational("audit-correlation"))
	ip := clientip.New(nil)
	sessions := auth.NewManager(st, al, v.Operational("csrf"))
	cfgSettings := settings.New(st)
	cfgSettings.Subscribe(func(c settings.Category) {
		switch c := c.(type) {
		case settings.General:
			p, err := clientip.ParsePrefixes(c.TrustedProxies)
			if err == nil { // stored values were validated; a parse error here keeps the previous list
				ip.SetTrusted(p)
			}
		case settings.Security:
			sessions.SetIdle(c.AdminIdle())
		}
	})
	public := newPublicToggles()
	cfgSettings.Subscribe(public.update)
	if err := cfgSettings.Load(ctx); err != nil {
		return err
	}
	passkeys := auth.NewPasskeys(st, relyingParty(st, cfg))
	setupSvc := setup.New(st, v, al)
	setupSvc.SkipUnbuilt = devBuild
	rp := relyingParty(st, cfg)
	setupSvc.RPID = func(ctx context.Context) (string, error) {
		r, err := rp(ctx)
		return r.ID, err
	}
	sessions.OnRevoke(passkeys.DropSession)
	sessions.OnRevoke(setupSvc.DropSession)

	if err := ensureBootstrap(ctx, st, setupSvc, al, cfg.Stdout, log); err != nil {
		return err
	}

	openDNS := cfg.dnsProviders
	if openDNS == nil {
		if openDNS, err = devDNSProviders(cfg); err != nil {
			return err
		}
	}
	if openDNS == nil {
		openDNS = openDNSProvider
	}
	dnsSvc := dns.NewService(st, v, al, openDNS)

	runner := jobs.New(st.Q, log)
	if err := runner.Register(ctx, jobs.Job{Kind: "session_cleanup", Interval: time.Hour, Run: sessions.Cleanup}); err != nil {
		return err
	}
	// The observed snapshot follows the sync interval from the base settings (read at start; a change applies after a restart).
	general, err := settings.Get[settings.General](ctx, st.R)
	if err != nil {
		return err
	}
	// Traefik hosts over SSH (04) and Hashi-issued certificates (03 §5.3); both write only through their sync plans.
	downloads := &http.Client{Timeout: 5 * time.Minute}
	traefikMgr := traefik.NewManager(st, v, al,
		func(ctx context.Context, c sshx.Config) (traefik.Conn, error) { return sshx.Dial(ctx, c) },
		func(ctx context.Context, arch string) ([]byte, error) {
			return traefik.Download(ctx, downloads, "", arch)
		})
	certMgr := certs.NewManager(st, v, al, certs.DNSSolver(st, dnsSvc), &http.Client{Timeout: time.Minute}, log)
	if err := devEdge(cfg, traefikMgr, certMgr); err != nil {
		return err
	}
	// IP sets (05a) and firewall hosts (05): host items resolve through the firewall manager.
	ipSets := ipsets.NewService(st, al, log.With("component", "ipsets"), nil)
	fwMgr := firewall.NewManager(st, v, al, ipSets,
		func(ctx context.Context, c sshx.Config) (firewall.Conn, error) { return sshx.Dial(ctx, c) })
	// Address sets in target positions (05a §5.2): upstreams, DNS values; generated host and service records (03 §5).
	ipSets.AddConsumers(services.Consumers)
	ipSets.AddConsumers(dns.Consumers)
	traefikMgr.UseTargets(ipSets.ResolveTarget)
	dnsSvc.UseTargets(ipSets.ResolveTarget)
	dnsSvc.UseGenerator(fwMgr.Records(ipSets.ResolveTarget))
	if err := devFirewall(cfg, fwMgr); err != nil {
		return err
	}
	subs := append(dnsSvc.Subsystems(), traefikMgr.Subsystems()...)
	subs = append(subs, fwMgr.Subsystems()...)
	syncEngine := hsync.New(st, al, append(subs, certMgr.Subsystems()...)...)
	if err := syncEngine.Start(ctx); err != nil {
		return err
	}
	// Each run reads the zone fresh and reconciles Hashi's entries (01 §5.4 periodic reconcile).
	if err := runner.Register(ctx, jobs.Job{Kind: "dns.refresh", Interval: time.Duration(general.SyncIntervalSeconds) * time.Second, Run: dnsSvc.Reconcile(syncEngine)}); err != nil {
		return err
	}

	// Monitoring (07): checks, closed-bucket rollups, retention and notification delivery all run without a browser.
	notifier := notify.New(st, v)
	monitorSvc := monitor.New(st, st.Ready, ipSets.ResolveTarget, notifier)
	for _, j := range []jobs.Job{
		{Kind: monitor.JobChecks, Interval: monitor.CheckTick, Run: monitorSvc.RunChecks},
		{Kind: monitor.JobRollup, Interval: monitor.RollupTick, Run: monitorSvc.Rollup},
		{Kind: monitor.JobRetain, Interval: monitor.RetainTick, Run: monitorSvc.Retain},
		{Kind: notify.JobKind, Interval: notify.JobInterval, Run: notifier.Deliver},
	} {
		if err := runner.Register(ctx, j); err != nil {
			return err
		}
	}

	// dns answers refresh and expired elements drop out every minute; a changed compiled set re-plans every managed
	// firewall host (low-risk element changes apply, the rest waits for approval, 05 §4.4).
	if err := runner.Register(ctx, jobs.Job{Kind: ipsets.JobKind, Interval: ipsets.JobInterval, Run: ipSets.Refresh}); err != nil {
		return err
	}
	// Hourly: every managed firewall host is read and planned again; a lost table (a reboot without the boot unit, a
	// flush by hand) comes back through a plan (invariants 15, 16).
	if err := runner.Register(ctx, jobs.Job{Kind: "firewall.reconcile", Interval: time.Hour, Run: func(ctx context.Context) (string, error) {
		plans, err := fwMgr.PlanAll(ctx, syncEngine, audit.System)
		return fmt.Sprintf("%d firewall plan(s)", len(plans)), err
	}}); err != nil {
		return err
	}
	ipSets.Subscribe(func(ctx context.Context, changed []string) {
		if _, err := fwMgr.PlanAll(ctx, syncEngine, audit.System); err != nil {
			log.Warn("firewall plans after an IP set change", "err", err)
		}
		// Upstreams and DNS values that follow a changed set (05a §4.2): every Traefik host and zone is planned again.
		// A set no target uses touches neither; a plan here would apply other waiting changes unreviewed (setup step 7).
		used, err := targetsUse(ctx, st.R, changed)
		if err != nil {
			log.Warn("IP set consumers", "err", err)
			return
		}
		if !used {
			return
		}
		if err := traefikMgr.PlanAll(ctx, syncEngine, audit.System); err != nil {
			log.Warn("Traefik plans after an IP set change", "err", err)
		}
		if _, err := dnsSvc.Reconcile(syncEngine)(ctx); err != nil {
			log.Warn("DNS plans after an IP set change", "err", err)
		}
	})

	// Hourly: issue what is missing, renew 30 days ahead (a failure waits certs.RetryAfter), then bring every Traefik
	// host's certificate files up to date.
	if err := runner.Register(ctx, jobs.Job{Kind: certs.JobKind, Interval: certs.JobInterval,
		Run: certMgr.Renew(syncEngine, traefikMgr.Distribute(syncEngine))}); err != nil {
		return err
	}

	// Requests through Hashi-managed Traefik carry the edge token; the resolver trusts their forwarding headers (01 §8).
	edgeToken, err := traefik.EnsureEdgeToken(ctx, st, v)
	if err != nil {
		return fmt.Errorf("edge token: %w", err)
	}
	ip.SetEdgeToken(edgeToken.Reveal())
	if t := devEdgeToken(cfg); t != "" {
		ip.SetEdgeToken(t)
	}

	health := httpapi.NewHealth(map[string]httpapi.Check{"database": st.Ready})
	api := &httpapi.Server{
		Health: health, Version: gen.Version{Version: Version, Commit: Commit}, Store: st, Sessions: sessions,
		Passkeys: passkeys, Setup: setupSvc, Settings: cfgSettings, DNS: dnsSvc, Traefik: traefikMgr, Certs: certMgr, IPSets: ipSets, Monitor: monitorSvc, Notify: notifier, Firewall: fwMgr, Sync: syncEngine, Vault: v, Audit: al, IP: ip, Logs: cfg.Logs,
		CertsSoon: func(ctx context.Context) error { return runner.Soon(ctx, certs.JobKind) },
		Log:       log.With("component", "api"),
	}
	servers := []*server{{name: "admin", addr: cfg.AdminAddr, handler: adminMux(api)}}
	if cfg.PublicAppsAddr != "" {
		servers = append(servers, &server{name: "public-apps", addr: cfg.PublicAppsAddr, handler: publicMux(api, "apps", &public.apps)})
	}
	if cfg.PublicStatusAddr != "" {
		servers = append(servers, &server{name: "public-status", addr: cfg.PublicStatusAddr, handler: publicMux(api, "status", &public.status)})
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

// openDNSProvider opens the adapter for a provider type (03 §1: adding a provider is a new module plus a case here).
func openDNSProvider(typ string, token secrets.Value) (dns.Provider, error) {
	switch typ {
	case "hetzner":
		return hetzner.New(token), nil
	}
	return nil, fmt.Errorf("no adapter for DNS provider type %q", typ)
}

// relyingParty resolves where passkeys are bound: the base settings, or a localhost origin in dev builds.
func relyingParty(st *store.Store, cfg Config) func(context.Context) (auth.RelyingParty, error) {
	return func(ctx context.Context) (auth.RelyingParty, error) {
		if devBuild && cfg.DevWebAuthnOrigin != "" {
			return auth.RelyingParty{ID: "localhost", Origin: cfg.DevWebAuthnOrigin}, nil
		}
		g, err := settings.Get[settings.General](ctx, st.R) // read pool: this runs inside ceremony transactions
		if err != nil || !g.Configured() {
			return auth.RelyingParty{}, err
		}
		return auth.RPFromDomains(g.RootDomain, g.AdminDomain), nil
	}
}

// ensureBootstrap creates and prints bootstrap credentials once, on the first start before setup completes
// (02 §1.1). Later starts keep the stored hash and print nothing.
func ensureBootstrap(ctx context.Context, st *store.Store, s *setup.Service, al *audit.Log, out io.Writer, log *slog.Logger) error {
	done, err := s.Completed(ctx)
	if err != nil || done {
		return err
	}
	if _, err := st.Q.GetBootstrapCredentials(ctx); err == nil {
		log.Info("setup not complete; bootstrap credentials were printed on first start (hashi bootstrap reset prints new ones)")
		return nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var c auth.BootstrapCredentials
	err = st.Tx(ctx, func(q *db.Queries) error {
		var err error
		if c, err = auth.NewBootstrapCredentials(ctx, q, time.Now()); err != nil {
			return err
		}
		return al.Record(ctx, q, audit.Event{Actor: audit.System, Action: "setup.bootstrap_credentials.created"})
	})
	if err != nil {
		return err
	}
	auth.PrintBootstrap(out, c)
	return nil
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

func adminMux(api *httpapi.Server) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /", web.Handler())
	return api.Handler(mux)
}

// publicToggles mirrors the Appearance settings that switch the public pages on and off.
type publicToggles struct {
	apps, status atomic.Bool
}

func newPublicToggles() *publicToggles {
	p := &publicToggles{}
	p.apps.Store(true)
	p.status.Store(true)
	return p
}

func (p *publicToggles) update(c settings.Category) {
	if a, ok := c.(settings.Appearance); ok {
		p.apps.Store(a.PublicDashboardEnabled)
		p.status.Store(a.PublicStatusEnabled)
	}
}

// publicMux serves one public page on its own listener (07 §7, §8): the page, its API and health, nothing of the admin
// API. A disabled page answers 404.
func publicMux(api *httpapi.Server, page string, enabled *atomic.Bool) http.Handler {
	mux := http.NewServeMux()
	// Anonymous visitors never see an error's text (07 §7): it is logged, and they get a plain 500.
	fail := func(w http.ResponseWriter, _ *http.Request, err error) {
		api.Log.Error("public page", "err", err)
		http.Error(w, "Temporarily unavailable", http.StatusInternalServerError)
	}
	h := gen.NewStrictHandlerWithOptions(api, nil, gen.StrictHTTPServerOptions{RequestErrorHandlerFunc: fail, ResponseErrorHandlerFunc: fail})
	mux.HandleFunc("GET /healthz", h.GetHealthz)
	if page == "status" {
		mux.HandleFunc("GET /api/public/status", onlyIf(enabled, h.GetPublicStatus))
	} else {
		mux.HandleFunc("GET /api/public/apps", onlyIf(enabled, h.GetPublicApps))
	}
	mux.HandleFunc("GET /", onlyIf(enabled, web.Public(page).ServeHTTP))
	return mux
}

func onlyIf(enabled *atomic.Bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !enabled.Load() {
			http.NotFound(w, r)
			return
		}
		next(w, r)
	}
}

// targetsUse reports whether a service upstream or DNS value follows one of the sets.
func targetsUse(ctx context.Context, q *db.Queries, sets []string) (bool, error) {
	for _, id := range sets {
		for _, src := range []ipsets.ConsumerSource{services.Consumers, dns.Consumers} {
			c, err := src(ctx, q, id)
			if err != nil {
				return false, err
			}
			if len(c) > 0 {
				return true, nil
			}
		}
	}
	return false, nil
}
