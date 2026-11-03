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
			slug, ok := traefik.ServiceSlug(l.Router)
			if !ok {
				continue
			}
			if id, ok := ids[slug]; ok {
				abuse.RecordResponse(security.Response{At: l.Start, Addr: l.Client, ServiceID: id, Path: l.Path, Status: l.Status})
			}
		}
	}
}
