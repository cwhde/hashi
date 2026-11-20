package app

import (
	"context"
	"log/slog"

	"git.juzo.io/juzo/hashi/internal/security"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/traefik"
)

// accessSink turns access-log lines into response rows of the telemetry (06 §6.1): the line's router names the
// service; Hashi's own routers and services deleted since are skipped.
func accessSink(st *store.Store, abuse *security.Engine, log *slog.Logger) traefik.LineSink {
	return func(ctx context.Context, _ string, lines []traefik.AccessLine) {
		svcs, err := services.List(ctx, st.R)
		if err != nil {
			log.Warn("access log: list services", "err", err)
			return
		}
		ids := make(map[string]string, len(svcs))
		for _, s := range svcs {
			ids[s.Slug] = s.ID
		}
		for _, l := range lines {
			// An answer to a name no route matches has no router: scanners probe such names, so its errors count
			// towards the error trigger (06 §3.2, bug 26). Hashi's own routers (forward auth) never count.
			if l.Router == "" {
				answer(ctx, abuse, log, security.Response{At: l.Start, Addr: l.Client, Path: l.Path, Status: l.Status})
				continue
			}
			slug, ok := traefik.ServiceSlug(l.Router)
			if !ok {
				continue
			}
			if id, ok := ids[slug]; ok {
				r := security.Response{At: l.Start, Addr: l.Client, ServiceID: id, Path: l.Path, Status: l.Status}
				abuse.RecordResponse(r)
				answer(ctx, abuse, log, r)
			}
		}
	}
}

func answer(ctx context.Context, abuse *security.Engine, log *slog.Logger, r security.Response) {
	if err := abuse.Answer(ctx, r); err != nil {
		log.Warn("access log: error trigger", "err", err)
	}
}
