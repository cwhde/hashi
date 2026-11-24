package httpapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"git.juzo.io/juzo/hashi/internal/appicons"
	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/monitor"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// The public pages and their one layout (07 §7). The public handlers send public fields only: names, states,
// history, the service page's address part and icons; never targets, IDs or check details.

// iconSet looks up app icons; empty when the icon service is not wired (tests).
type iconSet map[string]db.AppIcon

func (s *Server) icons(ctx context.Context) (iconSet, error) {
	if s.AppIcons == nil {
		return nil, nil
	}
	return s.AppIcons.Icons(ctx)
}

// urls gives an entry's icon addresses, nil when it has none.
func (i iconSet) urls(kind, id string) (icon, light *string) {
	if kind == "" {
		return nil, nil
	}
	r, ok := i[appicons.Key(kind, id)]
	if !ok {
		return nil, nil
	}
	a, b := appicons.URLs(r)
	return emptyNil(a), emptyNil(b)
}

func spellBody(v []monitor.PublicSpell) []gen.PublicSpell {
	out := make([]gen.PublicSpell, 0, len(v))
	for _, sp := range v {
		b := gen.PublicSpell{State: gen.PublicSpellState(sp.State), Start: sp.Start.UTC()}
		if !sp.End.IsZero() {
			end := sp.End.UTC()
			b.End = &end
		}
		out = append(out, b)
	}
	return out
}

func dayBody(v []monitor.PublicDay) []gen.PublicDay {
	out := make([]gen.PublicDay, 0, len(v))
	for _, d := range v {
		day, err := time.Parse(time.DateOnly, d.Date)
		if err != nil {
			continue // our own format; cannot happen
		}
		out = append(out, gen.PublicDay{Date: openapi_types.Date{Time: day}, State: gen.PublicDayState(d.State), Uptime: d.Uptime, Spells: spellBody(d.Spells)})
	}
	return out
}

func (i iconSet) entryBody(e monitor.PublicEntry) gen.PublicStatusEntry {
	minutes := make([]gen.PublicMinute, 0, len(e.Minutes))
	for _, m := range e.Minutes {
		minutes = append(minutes, gen.PublicMinute{At: m.At.UTC(), State: gen.PublicMinuteState(m.State), LatencyMs: m.LatencyMs})
	}
	b := gen.PublicStatusEntry{Name: e.Name, Slug: e.Slug, State: gen.MonitorState(e.State), Since: e.Since.UTC(),
		Minutes: minutes, Days: dayBody(e.Days), Uptime24h: e.Uptime24, Uptime90d: e.Uptime90}
	b.Icon, b.IconLight = i.urls(e.IconType, e.IconID)
	b.Mark = boolIf(e.IconType == monitor.IconHashi)
	return b
}

func (s *Server) GetPublicStatus(ctx context.Context, _ gen.GetPublicStatusRequestObject) (gen.GetPublicStatusResponseObject, error) {
	title, look, shown, err := s.publicLook(ctx)
	if err != nil {
		return nil, err
	}
	v, err := s.Monitor.PublicStatus(ctx)
	if err != nil {
		return nil, err
	}
	icons, err := s.icons(ctx)
	if err != nil {
		return nil, err
	}
	out := gen.GetPublicStatus200JSONResponse{Title: title, Assets: shown, Theme: look.Theme, CustomTheme: look.CustomTheme, UpdatedAt: nonZero(v.UpdatedAt),
		Groups: make([]gen.PublicStatusGroup, 0, len(v.Groups))}
	for _, g := range v.Groups {
		entries := make([]gen.PublicStatusEntry, 0, len(g.Entries))
		for _, e := range g.Entries {
			entries = append(entries, icons.entryBody(e))
		}
		out.Groups = append(out.Groups, gen.PublicStatusGroup{Name: g.Name, Entries: entries})
	}
	return out, nil
}

func (s *Server) GetPublicService(ctx context.Context, req gen.GetPublicServiceRequestObject) (gen.GetPublicServiceResponseObject, error) {
	rng := "24h"
	if req.Params.Range != nil {
		rng = string(*req.Params.Range)
	}
	v, err := s.Monitor.PublicService(ctx, req.Slug, rng)
	if err != nil {
		return nil, monitorError(err)
	}
	title, look, shown, err := s.publicLook(ctx)
	if err != nil {
		return nil, err
	}
	icons, err := s.icons(ctx)
	if err != nil {
		return nil, err
	}
	series := make([]gen.PublicPoint, 0, len(v.Series))
	for _, p := range v.Series {
		series = append(series, gen.PublicPoint{At: p.At, AvgMs: p.AvgMs, P95Ms: p.P95Ms, Availability: p.Availability})
	}
	badges := []string{}
	for _, k := range monitor.BadgeKinds {
		if _, ok := monitor.Badge(v, k); ok {
			badges = append(badges, k)
		}
	}
	return gen.GetPublicService200JSONResponse{Title: title, Assets: shown, Theme: look.Theme, CustomTheme: look.CustomTheme,
		Entry: icons.entryBody(v.PublicEntry), Group: v.Group, Uptime7d: v.Uptime7, Uptime30d: v.Uptime30, AvgMs24h: v.AvgMs24,
		Series: series, Downtimes: spellBody(v.Downtimes), Badges: badges}, nil
}

func (s *Server) GetPublicApps(ctx context.Context, _ gen.GetPublicAppsRequestObject) (gen.GetPublicAppsResponseObject, error) {
	title, look, shown, err := s.publicLook(ctx)
	if err != nil {
		return nil, err
	}
	d, err := settings.Get[settings.Dashboard](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	v, err := s.Monitor.PublicApps(ctx)
	if err != nil {
		return nil, err
	}
	icons, err := s.icons(ctx)
	if err != nil {
		return nil, err
	}
	out := gen.GetPublicApps200JSONResponse{Title: title, Assets: shown, Theme: look.Theme, CustomTheme: look.CustomTheme, DefaultSort: gen.PublicAppsDefaultSort(d.DefaultSort),
		UpdatedAt: nonZero(v.UpdatedAt), Groups: make([]gen.PublicAppGroup, 0, len(v.Groups))}
	for _, g := range v.Groups {
		apps := make([]gen.PublicApp, 0, len(g.Apps))
		for _, a := range g.Apps {
			app := gen.PublicApp{Name: a.Name, Url: a.URL, State: gen.MonitorState(a.State), HasCheck: a.HasCheck}
			app.Icon, app.IconLight = icons.urls(a.IconType, a.IconID)
			apps = append(apps, app)
		}
		out.Groups = append(out.Groups, gen.PublicAppGroup{Name: g.Name, Apps: apps})
	}
	return out, nil
}

// layoutBody is the editor's view: every entry, its switches and what is possible, with the status page's history for
// the entries shown there (the preview).
func (s *Server) layoutBody(ctx context.Context, l monitor.Layout) (gen.PublicLayout, error) {
	icons, err := s.icons(ctx)
	if err != nil {
		return gen.PublicLayout{}, err
	}
	status, err := s.Monitor.PublicStatus(ctx)
	if err != nil {
		return gen.PublicLayout{}, err
	}
	days := map[string][]monitor.PublicDay{}
	for _, g := range status.Groups {
		for _, e := range g.Entries {
			days[e.Slug] = e.Days
		}
	}
	out := gen.PublicLayout{Version: l.Version, Groups: make([]gen.PublicLayoutGroup, 0, len(l.Groups))}
	for _, g := range l.Groups {
		entries := make([]gen.PublicLayoutEntry, 0, len(g.Entries))
		for _, e := range g.Entries {
			b := gen.PublicLayoutEntry{Key: e.Key, Name: e.Name, Type: gen.PublicLayoutEntryType(e.Type), Apps: e.Apps, Status: e.Status,
				CanApps: e.CanApps, CanStatus: e.CanStatus, Url: emptyNil(e.URL), Slug: e.Slug}
			if e.IconType != "" && e.IconType != monitor.IconHashi { // Hashi's mark is not an app icon to pick
				k := appicons.Key(e.IconType, e.IconID)
				b.IconApp = &k
			}
			b.Icon, b.IconLight = icons.urls(e.IconType, e.IconID)
			if ep := e.Endpoint(); ep != nil {
				st := gen.MonitorState(ep.State)
				b.State = &st
			}
			if d, ok := days[e.Slug]; ok && e.Status {
				body := dayBody(d)
				b.Days = &body
			}
			entries = append(entries, b)
		}
		out.Groups = append(out.Groups, gen.PublicLayoutGroup{Id: emptyNil(g.ID), Name: g.Name, Entries: entries})
	}
	return out, nil
}

func (s *Server) GetPublicLayout(ctx context.Context, _ gen.GetPublicLayoutRequestObject) (gen.GetPublicLayoutResponseObject, error) {
	l, err := s.Monitor.Layout(ctx)
	if err != nil {
		return nil, err
	}
	b, err := s.layoutBody(ctx, l)
	if err != nil {
		return nil, err
	}
	return gen.GetPublicLayout200JSONResponse(b), nil
}

func (s *Server) PutPublicLayout(ctx context.Context, req gen.PutPublicLayoutRequestObject) (gen.PutPublicLayoutResponseObject, error) {
	ri := info(ctx)
	in := monitor.LayoutSave{Version: req.Body.Version, Other: req.Body.Other}
	for _, g := range req.Body.Groups {
		sg := monitor.LayoutSaveGroup{Name: g.Name, Entries: g.Entries}
		if g.Id != nil {
			sg.ID = *g.Id
		}
		in.Groups = append(in.Groups, sg)
	}
	if req.Body.Switches != nil {
		for _, sw := range *req.Body.Switches {
			in.Switches = append(in.Switches, monitor.LayoutSwitch{Entry: sw.Entry, Apps: sw.Apps, Status: sw.Status})
		}
	}
	l, err := s.Monitor.SaveLayout(ctx, in)
	if err != nil {
		return nil, monitorError(err)
	}
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: "public_layout.saved", Target: "public-layout", IP: ri.ipString()}); err != nil {
		return nil, err
	}
	b, err := s.layoutBody(ctx, l)
	if err != nil {
		return nil, err
	}
	return gen.PutPublicLayout200JSONResponse(b), nil
}

// ServeBadge answers /badge/{slug}/{kind}.svg on the status listener (07 §7.3): 404 for an entry with Status off, an
// unknown kind, or a kind the entry does not have.
func (s *Server) ServeBadge(w http.ResponseWriter, r *http.Request) {
	kind, ok := strings.CutSuffix(r.PathValue("file"), ".svg")
	if !ok {
		http.NotFound(w, r)
		return
	}
	v, err := s.Monitor.PublicService(r.Context(), r.PathValue("slug"), "24h")
	if errors.Is(err, monitor.ErrNoPublicService) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		s.Log.Error("badge", "err", err)
		http.Error(w, "Temporarily unavailable", http.StatusInternalServerError)
		return
	}
	svg, ok := monitor.Badge(v, kind)
	if !ok {
		http.NotFound(w, r)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "image/svg+xml")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'")
	h.Set("Cache-Control", "public, max-age=60")
	// Every text in the image is escaped by BadgeSVG; the slug from the address only picks the entry.
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(svg))
}
