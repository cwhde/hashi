package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/monitor"
	"git.juzo.io/juzo/hashi/internal/notify"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

func monitorError(err error) error {
	switch {
	case errors.Is(err, monitor.ErrNotFound), errors.Is(err, notify.ErrNotFound), errors.Is(err, services.ErrNotFound):
		return notFound(err.Error())
	case errors.Is(err, monitor.ErrSourced):
		return conflict(err.Error())
	case errors.Is(err, monitor.ErrInvalid), errors.Is(err, notify.ErrInvalid):
		return invalid(err.Error())
	}
	return err
}

func millis(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := time.UnixMilli(v.Int64).UTC()
	return &t
}

func nullInt(v sql.NullInt64) *int64 {
	if !v.Valid {
		return nil
	}
	return &v.Int64
}

func emptyNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func stripBody(s monitor.Strip) []gen.MonitorStripSample {
	out := make([]gen.MonitorStripSample, 0, len(s))
	for _, v := range s {
		out = append(out, gen.MonitorStripSample{At: v.At, Verdict: gen.MonitorStripSampleVerdict(v.Verdict)})
	}
	return out
}

func endpointBody(r monitor.Row, lastErr map[string]string) gen.MonitorEndpoint {
	e := r.Endpoint
	checks := make([]gen.MonitorCheck, 0, len(r.Checks))
	for _, c := range r.Checks {
		checks = append(checks, gen.MonitorCheck{Id: c.ID, Type: c.Type, Role: c.Role, Target: c.Target, Required: c.Required == 1, LastError: emptyNil(lastErr[c.ID])})
	}
	since := time.UnixMilli(e.StateSince).UTC()
	return gen.MonitorEndpoint{Id: e.ID, Name: e.Name, Group: e.GroupName, SourceType: e.SourceType, State: gen.MonitorState(r.State), StateSince: &since,
		Paused: e.Paused == 1, Public: e.Public == 1, IntervalSeconds: e.IntervalSeconds, TimeoutSeconds: e.TimeoutSeconds, DegradedMs: e.DegradedMs,
		RouteProviderId: emptyNil(e.RouteProviderID.String), LastCheckAt: millis(e.LastCheckAt), LastLatencyMs: nullInt(e.LastLatencyMs), Uptime24h: r.Uptime24,
		Checks: checks, Strip: stripBody(r.Strip)}
}

func (s *Server) ListMonitorEndpoints(ctx context.Context, _ gen.ListMonitorEndpointsRequestObject) (gen.ListMonitorEndpointsResponseObject, error) {
	rows, err := s.Monitor.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]gen.MonitorEndpoint, 0, len(rows))
	for _, r := range rows {
		out = append(out, endpointBody(r, nil))
	}
	return gen.ListMonitorEndpoints200JSONResponse{Endpoints: out}, nil
}

func (s *Server) CreateMonitorEndpoint(ctx context.Context, req gen.CreateMonitorEndpointRequestObject) (gen.CreateMonitorEndpointResponseObject, error) {
	ri := info(ctx)
	b := req.Body
	group := ""
	if b.Group != nil {
		group = *b.Group
	}
	spec := monitor.CheckSpec{Type: b.Type, Target: b.Target}
	if b.Probe != nil {
		spec.Probe = *b.Probe
	}
	if b.Expect != nil {
		spec.Expect = *b.Expect
	}
	id, err := s.Monitor.CreateUser(ctx, b.Name, group, spec)
	if err != nil {
		return nil, monitorError(err)
	}
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: "monitor.endpoint.created", Target: id, IP: ri.ipString()}); err != nil {
		return nil, err
	}
	d, err := s.Monitor.Get(ctx, id, "1h")
	if err != nil {
		return nil, monitorError(err)
	}
	return gen.CreateMonitorEndpoint201JSONResponse(endpointBody(d.Row, nil)), nil
}

func (s *Server) GetMonitorEndpoint(ctx context.Context, req gen.GetMonitorEndpointRequestObject) (gen.GetMonitorEndpointResponseObject, error) {
	rng := "24h"
	if req.Params.Range != nil {
		rng = string(*req.Params.Range)
	}
	d, err := s.Monitor.Get(ctx, req.Id, rng)
	if err != nil {
		return nil, monitorError(err)
	}
	series := make([]gen.MonitorPoint, 0, len(d.Series))
	for _, p := range d.Series {
		series = append(series, gen.MonitorPoint{At: p.At, LatencyMs: p.LatencyMs, Availability: p.Availability, GapMs: p.GapMs})
	}
	events := make([]gen.MonitorEvent, 0, len(d.Events))
	for _, e := range d.Events {
		events = append(events, gen.MonitorEvent{Id: e.ID, At: time.UnixMilli(e.At).UTC(), From: e.FromState, To: gen.MonitorState(e.ToState), Message: e.Message})
	}
	return gen.GetMonitorEndpoint200JSONResponse{Endpoint: endpointBody(d.Row, d.CheckLastError), Uptime1h: d.Stats["1h"], Uptime7d: d.Stats["7d"], Uptime30d: d.Stats["30d"],
		MinMs: d.MinMs, AvgMs: d.AvgMs, MaxMs: d.MaxMs, Series: series, Events: events}, nil
}

func (s *Server) PutMonitorEndpoint(ctx context.Context, req gen.PutMonitorEndpointRequestObject) (gen.PutMonitorEndpointResponseObject, error) {
	ri := info(ctx)
	b := req.Body
	v := monitor.Settings{Group: b.Group, Paused: b.Paused, Public: b.Public, IntervalSeconds: b.IntervalSeconds, TimeoutSeconds: b.TimeoutSeconds, DegradedMs: b.DegradedMs}
	if b.RouteProviderId != nil {
		v.RouteProviderID = *b.RouteProviderId
	}
	if err := s.Monitor.Update(ctx, req.Id, v); err != nil {
		return nil, monitorError(err)
	}
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: "monitor.endpoint.updated", Target: req.Id, IP: ri.ipString()}); err != nil {
		return nil, err
	}
	return gen.PutMonitorEndpoint204Response{}, nil
}

func (s *Server) DeleteMonitorEndpoint(ctx context.Context, req gen.DeleteMonitorEndpointRequestObject) (gen.DeleteMonitorEndpointResponseObject, error) {
	ri := info(ctx)
	if err := s.Monitor.DeleteUser(ctx, req.Id); err != nil {
		return nil, monitorError(err)
	}
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: "monitor.endpoint.deleted", Target: req.Id, IP: ri.ipString()}); err != nil {
		return nil, err
	}
	return gen.DeleteMonitorEndpoint204Response{}, nil
}

func notifyProviderBody(p db.NotificationProvider) (gen.NotificationProvider, error) {
	out := gen.NotificationProvider{Id: p.ID, Type: p.Type, Name: p.Name, HasSecret: p.SecretID.Valid}
	if p.Type == notify.TypeTelegram {
		var c notify.TelegramConfig
		if err := json.Unmarshal([]byte(p.ConfigJson), &c); err != nil {
			return out, err
		}
		out.ChatId = &c.ChatID
		return out, nil
	}
	var c notify.SMTPConfig
	if err := json.Unmarshal([]byte(p.ConfigJson), &c); err != nil {
		return out, err
	}
	out.Host, out.Port, out.Tls, out.Username, out.From, out.To = &c.Host, &c.Port, &c.TLS, emptyNil(c.Username), &c.From, &c.To
	return out, nil
}

func (s *Server) ListNotificationProviders(ctx context.Context, _ gen.ListNotificationProvidersRequestObject) (gen.ListNotificationProvidersResponseObject, error) {
	rows, err := s.Store.R.ListNotificationProviders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]gen.NotificationProvider, 0, len(rows))
	for _, r := range rows {
		b, err := notifyProviderBody(r)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return gen.ListNotificationProviders200JSONResponse{Providers: out}, nil
}

func (s *Server) CreateNotificationProvider(ctx context.Context, req gen.CreateNotificationProviderRequestObject) (gen.CreateNotificationProviderResponseObject, error) {
	ri := info(ctx)
	b := req.Body
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	var row db.NotificationProvider
	var err error
	if b.Type == gen.Telegram {
		row, err = s.Notify.CreateTelegram(ctx, b.Name, notify.TelegramConfig{ChatID: str(b.ChatId)}, secrets.New(str(b.BotToken)))
	} else {
		c := notify.SMTPConfig{Host: str(b.Host), TLS: str(b.Tls), Username: str(b.Username), From: str(b.From), To: str(b.To)}
		if b.Port != nil {
			c.Port = *b.Port
		}
		row, err = s.Notify.CreateSMTP(ctx, b.Name, c, secrets.New(str(b.Password)))
	}
	if err != nil {
		return nil, monitorError(err)
	}
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: "notification.provider.created", Target: row.ID, IP: ri.ipString()}); err != nil {
		return nil, err
	}
	body, err := notifyProviderBody(row)
	if err != nil {
		return nil, err
	}
	return gen.CreateNotificationProvider201JSONResponse(body), nil
}

func (s *Server) UpdateNotificationProvider(ctx context.Context, req gen.UpdateNotificationProviderRequestObject) (gen.UpdateNotificationProviderResponseObject, error) {
	ri := info(ctx)
	b := req.Body
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	secret := str(b.Password) + str(b.BotToken)
	row, err := s.Notify.Update(ctx, req.Id, b.Name, func(typ string) (any, error) {
		if typ == notify.TypeTelegram {
			c := notify.TelegramConfig{ChatID: str(b.ChatId)}
			return c, c.Validate()
		}
		c := notify.SMTPConfig{Host: str(b.Host), TLS: str(b.Tls), Username: str(b.Username), From: str(b.From), To: str(b.To)}
		if b.Port != nil {
			c.Port = *b.Port
		}
		return c, c.Validate()
	}, secrets.New(secret))
	if err != nil {
		return nil, monitorError(err)
	}
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: "notification.provider.updated", Target: row.ID, IP: ri.ipString()}); err != nil {
		return nil, err
	}
	body, err := notifyProviderBody(row)
	if err != nil {
		return nil, err
	}
	return gen.UpdateNotificationProvider200JSONResponse(body), nil
}

func (s *Server) DiscoverTelegramChats(ctx context.Context, req gen.DiscoverTelegramChatsRequestObject) (gen.DiscoverTelegramChatsResponseObject, error) {
	chats, err := notify.Telegram{Token: secrets.New(req.Body.BotToken)}.DiscoverChats(ctx)
	if err != nil {
		return nil, invalid(err.Error())
	}
	out := gen.DiscoverTelegramChats200JSONResponse{Chats: []struct {
		Id    string `json:"id"`
		Title string `json:"title"`
	}{}}
	for _, c := range chats {
		out.Chats = append(out.Chats, struct {
			Id    string `json:"id"`
			Title string `json:"title"`
		}{Id: c.ID, Title: c.Title})
	}
	return out, nil
}

func (s *Server) DeleteNotificationProvider(ctx context.Context, req gen.DeleteNotificationProviderRequestObject) (gen.DeleteNotificationProviderResponseObject, error) {
	ri := info(ctx)
	if err := s.Notify.Delete(ctx, req.Id); err != nil {
		return nil, monitorError(err)
	}
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: "notification.provider.deleted", Target: req.Id, IP: ri.ipString()}); err != nil {
		return nil, err
	}
	return gen.DeleteNotificationProvider204Response{}, nil
}

func (s *Server) deliveries(ctx context.Context) (gen.NotificationDeliveryList, error) {
	rows, err := s.Store.R.ListNotificationDeliveries(ctx, 30)
	if err != nil {
		return gen.NotificationDeliveryList{}, err
	}
	provs, err := s.Store.R.ListNotificationProviders(ctx)
	if err != nil {
		return gen.NotificationDeliveryList{}, err
	}
	names := map[string]string{}
	for _, p := range provs {
		names[p.ID] = p.Name
	}
	out := make([]gen.NotificationDelivery, 0, len(rows))
	for _, d := range rows {
		b := gen.NotificationDelivery{Id: d.ID, Provider: names[d.ProviderID], Kind: d.Kind, Subject: d.Subject, Status: gen.NotificationDeliveryStatus(d.Status), Attempts: d.Attempts,
			LastError: emptyNil(d.LastError), CreatedAt: time.UnixMilli(d.CreatedAt).UTC(), DeliveredAt: millis(d.DeliveredAt)}
		if d.Status == "pending" {
			t := time.UnixMilli(d.NextAttemptAt).UTC()
			b.NextAttemptAt = &t
		}
		out = append(out, b)
	}
	return gen.NotificationDeliveryList{Deliveries: out}, nil
}

func (s *Server) TestNotificationProvider(ctx context.Context, req gen.TestNotificationProviderRequestObject) (gen.TestNotificationProviderResponseObject, error) {
	if err := s.Notify.EnqueueTest(ctx, req.Id); err != nil {
		return nil, monitorError(err)
	}
	l, err := s.deliveries(ctx)
	if err != nil {
		return nil, err
	}
	return gen.TestNotificationProvider200JSONResponse(l), nil
}

func (s *Server) ListNotificationDeliveries(ctx context.Context, _ gen.ListNotificationDeliveriesRequestObject) (gen.ListNotificationDeliveriesResponseObject, error) {
	l, err := s.deliveries(ctx)
	if err != nil {
		return nil, err
	}
	return gen.ListNotificationDeliveries200JSONResponse(l), nil
}

func (s *Server) GetNotificationRoute(ctx context.Context, _ gen.GetNotificationRouteRequestObject) (gen.GetNotificationRouteResponseObject, error) {
	r, err := s.Store.R.GetNotificationRoute(ctx)
	if err != nil {
		return nil, err
	}
	return gen.GetNotificationRoute200JSONResponse{ProviderId: emptyNil(r.ProviderID.String), MinSeverity: gen.NotificationRouteMinSeverity(r.MinSeverity), CooldownSeconds: r.CooldownSeconds, Recovery: r.Recovery == 1}, nil
}

func (s *Server) PutNotificationRoute(ctx context.Context, req gen.PutNotificationRouteRequestObject) (gen.PutNotificationRouteResponseObject, error) {
	ri := info(ctx)
	b := req.Body
	if b.CooldownSeconds < 0 {
		return nil, invalid("The cooldown cannot be negative.")
	}
	p := db.PutNotificationRouteParams{MinSeverity: string(b.MinSeverity), CooldownSeconds: b.CooldownSeconds, UpdatedAt: time.Now().UnixMilli()}
	if b.Recovery {
		p.Recovery = 1
	}
	if b.ProviderId != nil && *b.ProviderId != "" {
		if _, err := s.Store.R.GetNotificationProvider(ctx, *b.ProviderId); err != nil {
			return nil, notFound("No such notification provider.")
		}
		p.ProviderID = sql.NullString{String: *b.ProviderId, Valid: true}
	}
	if err := s.Store.Q.PutNotificationRoute(ctx, p); err != nil {
		return nil, err
	}
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: "notification.route.updated", IP: ri.ipString()}); err != nil {
		return nil, err
	}
	return gen.PutNotificationRoute204Response{}, nil
}

func (s *Server) PutServiceDashboard(ctx context.Context, req gen.PutServiceDashboardRequestObject) (gen.PutServiceDashboardResponseObject, error) {
	ri := info(ctx)
	v := int64(0)
	if req.Body.Visible {
		v = 1
	}
	n, err := s.Store.Q.SetServiceDashboard(ctx, db.SetServiceDashboardParams{DashboardVisible: v, UpdatedAt: time.Now().UnixMilli(), ID: req.Id})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, notFound("No such service.")
	}
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: "service.dashboard.updated", Target: req.Id, IP: ri.ipString()}); err != nil {
		return nil, err
	}
	return gen.PutServiceDashboard204Response{}, nil
}

// publicLook gives the public pages their title and theme.
func (s *Server) publicLook(ctx context.Context) (string, string, error) {
	g, err := settings.Get[settings.General](ctx, s.Store.R)
	if err != nil {
		return "", "", err
	}
	a, err := settings.Get[settings.Appearance](ctx, s.Store.R)
	if err != nil {
		return "", "", err
	}
	return g.RootDomain, a.Theme, nil
}

func nonZero(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (s *Server) GetPublicStatus(ctx context.Context, _ gen.GetPublicStatusRequestObject) (gen.GetPublicStatusResponseObject, error) {
	title, theme, err := s.publicLook(ctx)
	if err != nil {
		return nil, err
	}
	v, err := s.Monitor.PublicStatus(ctx)
	if err != nil {
		return nil, err
	}
	out := gen.GetPublicStatus200JSONResponse{Title: title, Theme: theme, UpdatedAt: nonZero(v.UpdatedAt), Endpoints: make([]gen.PublicEndpoint, 0, len(v.Endpoints))}
	for _, e := range v.Endpoints {
		out.Endpoints = append(out.Endpoints, gen.PublicEndpoint{Name: e.Name, Group: e.Group, State: gen.MonitorState(e.State), Strip: stripBody(e.Strip), Uptime24h: e.Uptime24})
	}
	return out, nil
}

func (s *Server) GetPublicApps(ctx context.Context, _ gen.GetPublicAppsRequestObject) (gen.GetPublicAppsResponseObject, error) {
	title, theme, err := s.publicLook(ctx)
	if err != nil {
		return nil, err
	}
	v, err := s.Monitor.PublicApps(ctx)
	if err != nil {
		return nil, err
	}
	out := gen.GetPublicApps200JSONResponse{Title: title, Theme: theme, UpdatedAt: nonZero(v.UpdatedAt), HostsOnline: v.HostsOnline, HostsTotal: v.HostsTotal,
		FirewallOnline: v.FirewallUp, FirewallTotal: v.FirewallTotal, Apps: make([]gen.PublicApp, 0, len(v.Apps))}
	for _, a := range v.Apps {
		out.Apps = append(out.Apps, gen.PublicApp{Name: a.Name, Group: a.Group, Url: a.URL, State: gen.MonitorState(a.State)})
	}
	return out, nil
}
