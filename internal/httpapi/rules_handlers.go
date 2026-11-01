package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/netip"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/edge"
	"git.juzo.io/juzo/hashi/internal/geoip"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/rules"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/traefik"
	"git.juzo.io/juzo/hashi/internal/vault"
)

func ruleError(err error) error {
	var ce *rules.CycleError
	switch {
	case errors.As(err, &ce):
		return badRequest(strings.TrimPrefix(err.Error(), rules.ErrInvalid.Error()+": "))
	case errors.Is(err, rules.ErrNotFound):
		return notFound("No such rule set.")
	case errors.Is(err, rules.ErrInvalid):
		return invalid(strings.TrimPrefix(err.Error(), rules.ErrInvalid.Error()+": "))
	case errors.Is(err, rules.ErrLocked), errors.Is(err, rules.ErrInUse), errors.Is(err, rules.ErrTaken):
		return conflict(err.Error())
	}
	return err
}

func entryBody(g rules.Graph, e rules.Entry) gen.RuleEntry {
	groups := e.Groups
	if groups == nil {
		groups = []string{}
	}
	out := gen.RuleEntry{Id: e.ID, Priority: e.Priority, Type: gen.RuleEntryType(e.Type), ChildId: e.ChildID, AppliesTo: e.AppliesTo,
		MatchType: gen.RuleEntryMatchType(e.MatchType), Pattern: e.Pattern, Action: gen.RuleEntryAction(e.Action), ProviderId: e.Provider, Groups: groups,
		Origin: e.Origin, Locked: e.Locked, Fallback: e.Fallback, Enabled: e.Enabled, Note: e.Note}
	if c, ok := g[e.ChildID]; ok {
		out.ChildSlug = c.Slug
	}
	return out
}

func entryFromInput(in gen.RuleEntryInput) rules.Entry {
	e := rules.Entry{Type: string(in.Type), Enabled: true}
	if in.ChildId != nil {
		e.ChildID = *in.ChildId
	}
	if in.AppliesTo != nil {
		e.AppliesTo = *in.AppliesTo
	}
	if in.MatchType != nil {
		e.MatchType = string(*in.MatchType)
	}
	if in.Pattern != nil {
		e.Pattern = *in.Pattern
	}
	if in.Action != nil {
		e.Action = string(*in.Action)
	}
	if in.ProviderId != nil {
		e.Provider = *in.ProviderId
	}
	if in.Groups != nil {
		e.Groups = *in.Groups
	}
	if in.Enabled != nil {
		e.Enabled = *in.Enabled
	}
	if in.Note != nil {
		e.Note = *in.Note
	}
	return e
}

// geoMissing reports whether a set holds a geo item whose database is not on hand (06 §4.3).
func (s *Server) geoMissing(ctx context.Context) func(string) bool {
	return func(id string) bool {
		res, err := s.IPSets.Compile(ctx, s.Store.R, id)
		if err != nil {
			return false
		}
		for _, g := range res.Geo {
			if s.GeoIP == nil || !s.GeoIP.Available(g) {
				return true
			}
		}
		return false
	}
}

func (s *Server) allSetID(ctx context.Context) (string, error) {
	all, err := s.Store.R.GetIPSetBySystemKey(ctx, sql.NullString{String: ipsets.SystemAll, Valid: true})
	return all.ID, err
}

func (s *Server) ruleSetBody(ctx context.Context, g rules.Graph, id string) (gen.RuleSet, error) {
	rs, ok := g[id]
	if !ok {
		return gen.RuleSet{}, rules.ErrNotFound
	}
	out := gen.RuleSet{Id: rs.ID, Slug: rs.Slug, Name: rs.Name, Description: rs.Description, Kind: gen.RuleSetKind(rs.Kind), Origin: rs.Origin,
		Entries: []gen.RuleEntry{}, UsedBy: []gen.RuleSetUse{}, Warnings: []string{}}
	for _, e := range rs.Entries {
		out.Entries = append(out.Entries, entryBody(g, e))
	}
	for _, p := range g.Parents(id) {
		switch p.Kind {
		case rules.KindServiceRoot:
			out.UsedBy = append(out.UsedBy, gen.RuleSetUse{Label: strings.TrimPrefix(p.Name, "Rules of "), Href: "/services/" + p.ServiceID})
		default:
			out.UsedBy = append(out.UsedBy, gen.RuleSetUse{Label: p.Slug, Href: "/rule-sets/" + p.ID})
		}
	}
	all, err := s.allSetID(ctx)
	if err != nil {
		return out, err
	}
	out.Warnings = append(out.Warnings, rules.Warnings(g.Flatten(id, false), all, s.geoMissing(ctx))...)
	return out, nil
}

func (s *Server) ListRuleSets(ctx context.Context, _ gen.ListRuleSetsRequestObject) (gen.ListRuleSetsResponseObject, error) {
	g, err := rules.Load(ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	rows, err := s.Store.R.ListRuleSets(ctx)
	if err != nil {
		return nil, err
	}
	out := gen.ListRuleSets200JSONResponse{}
	for _, r := range rows {
		if r.Kind == rules.KindServiceRoot {
			continue
		}
		b, err := s.ruleSetBody(ctx, g, r.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
}

func (s *Server) GetRuleSet(ctx context.Context, req gen.GetRuleSetRequestObject) (gen.GetRuleSetResponseObject, error) {
	g, err := rules.Load(ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	b, err := s.ruleSetBody(ctx, g, req.Id)
	if err != nil {
		return nil, ruleError(err)
	}
	return gen.GetRuleSet200JSONResponse(b), nil
}

// changeRules runs one change with its audit entry; decisions follow at once, and Traefik is planned only when a service's
// outage behaviour changed (a plan here would otherwise apply other waiting changes unreviewed).
func (s *Server) changeRules(ctx context.Context, action, target string, f func(q *db.Queries) error) ([]gen.SyncPlan, error) {
	ri := info(ctx)
	before, err := s.failModes(ctx)
	if err != nil {
		return nil, err
	}
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		if err := f(q); err != nil {
			return err
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: action, IP: ri.ipString(), Target: target, Outcome: audit.Success})
	})
	if err != nil {
		return nil, ruleError(err)
	}
	if err := s.Edge.Rebuild(ctx); err != nil {
		s.Log.Warn("forward auth after a rule change", "err", err)
	}
	after, err := s.failModes(ctx)
	if err != nil {
		return nil, err
	}
	plans := []gen.SyncPlan{}
	if before != after {
		if plans, _, err = s.syncServices(ctx); err != nil {
			return nil, err
		}
	}
	return plans, nil
}

func (s *Server) failModes(ctx context.Context) (string, error) {
	svcs, err := services.List(ctx, s.Store.R)
	if err != nil {
		return "", err
	}
	if err := traefik.MarkFailClosed(ctx, s.Store.R, svcs); err != nil {
		return "", err
	}
	var b strings.Builder
	for _, sv := range svcs {
		if sv.FailClosed {
			b.WriteString(sv.ID + ",")
		}
	}
	return b.String(), nil
}

func (s *Server) CreateRuleSet(ctx context.Context, req gen.CreateRuleSetRequestObject) (gen.CreateRuleSetResponseObject, error) {
	var id string
	desc := ""
	if req.Body.Description != nil {
		desc = *req.Body.Description
	}
	if _, err := s.changeRules(ctx, "rule_set.created", "rule_set:"+req.Body.Slug, func(q *db.Queries) error {
		var err error
		id, err = rules.Create(ctx, q, req.Body.Slug, req.Body.Name, desc, s.now())
		return err
	}); err != nil {
		return nil, err
	}
	g, err := rules.Load(ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	b, err := s.ruleSetBody(ctx, g, id)
	return gen.CreateRuleSet200JSONResponse(b), err
}

func (s *Server) UpdateRuleSet(ctx context.Context, req gen.UpdateRuleSetRequestObject) (gen.UpdateRuleSetResponseObject, error) {
	desc := ""
	if req.Body.Description != nil {
		desc = *req.Body.Description
	}
	if _, err := s.changeRules(ctx, "rule_set.updated", "rule_set:"+req.Id, func(q *db.Queries) error {
		return rules.Update(ctx, q, req.Id, req.Body.Slug, req.Body.Name, desc, s.now())
	}); err != nil {
		return nil, err
	}
	g, err := rules.Load(ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	b, err := s.ruleSetBody(ctx, g, req.Id)
	return gen.UpdateRuleSet200JSONResponse(b), err
}

func (s *Server) DeleteRuleSet(ctx context.Context, req gen.DeleteRuleSetRequestObject) (gen.DeleteRuleSetResponseObject, error) {
	if _, err := s.changeRules(ctx, "rule_set.deleted", "rule_set:"+req.Id, func(q *db.Queries) error { return rules.Delete(ctx, q, req.Id) }); err != nil {
		return nil, err
	}
	return gen.DeleteRuleSet204Response{}, nil
}

func (s *Server) PutRuleSetEntries(ctx context.Context, req gen.PutRuleSetEntriesRequestObject) (gen.PutRuleSetEntriesResponseObject, error) {
	g, err := rules.Load(ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	if rs, ok := g[req.Id]; ok && rs.Kind == rules.KindServiceRoot {
		return nil, conflict("A service's own rules are edited on its Access tab.")
	}
	entries := make([]rules.Entry, 0, len(req.Body.Entries))
	for _, in := range req.Body.Entries {
		entries = append(entries, entryFromInput(in))
	}
	plans, err := s.changeRules(ctx, "rule_set.entries_changed", "rule_set:"+req.Id, func(q *db.Queries) error {
		return rules.PutEntries(ctx, q, req.Id, entries, "", false, s.now())
	})
	if err != nil {
		return nil, err
	}
	if g, err = rules.Load(ctx, s.Store.R); err != nil {
		return nil, err
	}
	b, err := s.ruleSetBody(ctx, g, req.Id)
	return gen.PutRuleSetEntries200JSONResponse{RuleSet: b, Plans: plans}, err
}

func flatBody(g rules.Graph, list []rules.Flat) []gen.FlatRule {
	out := make([]gen.FlatRule, 0, len(list))
	for _, f := range list {
		out = append(out, gen.FlatRule{Entry: entryBody(g, f.Entry), Path: f.Path})
	}
	return out
}

func (s *Server) serviceAccess(ctx context.Context, id string) (gen.ServiceAccess, error) {
	sv, err := services.Get(ctx, s.Store.R, id)
	if err != nil {
		return gen.ServiceAccess{}, serviceError(err)
	}
	g, err := rules.Load(ctx, s.Store.R)
	if err != nil {
		return gen.ServiceAccess{}, err
	}
	var root *rules.Set
	for _, rs := range g {
		if rs.ServiceID == id {
			root = rs
		}
	}
	if root == nil {
		return gen.ServiceAccess{}, conflict("This service has no rules: TCP and UDP services are protected at the firewall.")
	}
	body, err := s.ruleSetBody(ctx, g, root.ID)
	if err != nil {
		return gen.ServiceAccess{}, err
	}
	out := gen.ServiceAccess{RuleSet: body, Editable: !sv.System(), Fallback: gen.ServiceAccessFallback(rules.ActionNone)}
	list := g.Flatten(root.ID, sv.System())
	for _, f := range list {
		if f.Entry.Fallback {
			out.Fallback = gen.ServiceAccessFallback(f.Entry.Action)
		}
	}
	all, err := s.allSetID(ctx)
	if err != nil {
		return out, err
	}
	out.FailClosed, out.Flattened = rules.FailClosed(list), flatBody(g, list)
	out.Warnings = rules.Warnings(list, all, s.geoMissing(ctx))
	if out.Warnings == nil {
		out.Warnings = []string{}
	}
	return out, nil
}

func (s *Server) GetServiceAccess(ctx context.Context, req gen.GetServiceAccessRequestObject) (gen.GetServiceAccessResponseObject, error) {
	a, err := s.serviceAccess(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return gen.GetServiceAccess200JSONResponse(a), nil
}

func (s *Server) PutServiceAccess(ctx context.Context, req gen.PutServiceAccessRequestObject) (gen.PutServiceAccessResponseObject, error) {
	cur, err := s.serviceAccess(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	sv, err := services.Get(ctx, s.Store.R, req.Id)
	if err != nil {
		return nil, serviceError(err)
	}
	fallback := string(cur.Fallback)
	if req.Body.Fallback != nil {
		fallback = string(*req.Body.Fallback)
	}
	entries := make([]rules.Entry, 0, len(req.Body.Entries))
	for _, in := range req.Body.Entries {
		entries = append(entries, entryFromInput(in))
	}
	plans, err := s.changeRules(ctx, "service.access_changed", "service:"+req.Id, func(q *db.Queries) error {
		return rules.PutEntries(ctx, q, cur.RuleSet.Id, entries, fallback, sv.System(), s.now())
	})
	if err != nil {
		return nil, err
	}
	a, err := s.serviceAccess(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return gen.PutServiceAccess200JSONResponse{Access: a, Plans: plans}, nil
}

func (s *Server) TraceAddress(ctx context.Context, req gen.TraceAddressRequestObject) (gen.TraceAddressResponseObject, error) {
	a, err := netip.ParseAddr(strings.TrimSpace(req.Address))
	if err != nil {
		return nil, invalid("That is not an IP address.")
	}
	d, ok := s.Edge.DecideService(req.Body.ServiceId, a.Unmap(), req.Body.Path)
	if !ok {
		return nil, notFound("No such HTTP service.")
	}
	g, err := rules.Load(ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	return gen.TraceAddress200JSONResponse{Action: d.Action, Decided: d.Index, NormalizedPath: edge.NormalizePath(req.Body.Path), Flattened: flatBody(g, d.List)}, nil
}

func (s *Server) providerBody(ctx context.Context, p db.OidcProvider) gen.SSOProvider {
	out := gen.SSOProvider{Id: p.ID, Name: p.Name, Issuer: p.Issuer, ClientId: p.ClientID, Scopes: p.Scopes, UsernameClaim: p.UsernameClaim,
		EmailClaim: p.EmailClaim, GroupsClaim: p.GroupsClaim, IsDefault: p.IsDefault == 1}
	if err := s.SSO.Check(ctx, p.Issuer); err != nil {
		out.DiscoveryError = err.Error()
	}
	return out
}

func (s *Server) ListSSOProviders(ctx context.Context, _ gen.ListSSOProvidersRequestObject) (gen.ListSSOProvidersResponseObject, error) {
	rows, err := s.Store.R.ListOIDCProviders(ctx)
	if err != nil {
		return nil, err
	}
	g, err := settings.Get[settings.General](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	out := gen.ListSSOProviders200JSONResponse{Providers: []gen.SSOProvider{}, CallbackUrl: "https://" + g.AdminDomain + edgeCallbackPath, CookieDomain: g.RootDomain}
	for _, p := range rows {
		out.Providers = append(out.Providers, s.providerBody(ctx, p))
	}
	return out, nil
}

func orDefault(p *string, def string) string {
	if p == nil || strings.TrimSpace(*p) == "" {
		return def
	}
	return strings.TrimSpace(*p)
}

// saveProvider stores a provider (id empty: new). It reads the discovery document first, so a mistyped issuer is refused.
func (s *Server) saveProvider(ctx context.Context, id string, in gen.SSOProviderInput) (db.OidcProvider, error) {
	issuer := strings.TrimSpace(in.Issuer) // exactly as the provider names itself, or its tokens fail the issuer check
	if !strings.HasPrefix(issuer, "https://") {
		return db.OidcProvider{}, invalid("The issuer must be an https address.")
	}
	if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.ClientId) == "" {
		return db.OidcProvider{}, invalid("A provider needs a name and a client ID.")
	}
	if err := s.SSO.Check(ctx, issuer); err != nil {
		return db.OidcProvider{}, invalid("Could not read that provider: " + err.Error())
	}
	secret := ""
	if in.ClientSecret != nil {
		secret = *in.ClientSecret
	}
	if id == "" && secret == "" {
		return db.OidcProvider{}, invalid("A new provider needs its client secret.")
	}
	ri := info(ctx)
	var out db.OidcProvider
	err := s.Store.Tx(ctx, func(q *db.Queries) error {
		now := s.now().UnixMilli()
		var cur db.OidcProvider
		isNew := id == ""
		if isNew {
			id = edge.NewProviderID()
		} else {
			var err error
			if cur, err = q.GetOIDCProvider(ctx, id); errors.Is(err, sql.ErrNoRows) {
				return notFound("No such provider.")
			} else if err != nil {
				return err
			}
		}
		secretID := cur.SecretID
		if secret != "" {
			p, err := s.Vault.Seal(vault.Service, "oidc:"+id, secrets.New(secret))
			if err != nil {
				return err
			}
			if err := q.InsertSecret(ctx, p); err != nil {
				return err
			}
			secretID = p.ID
		}
		def := int64(0)
		if in.IsDefault {
			def = 1
			if err := q.ClearOIDCDefault(ctx); err != nil {
				return err
			}
		}
		out = db.OidcProvider{ID: id, Name: strings.TrimSpace(in.Name), Issuer: issuer, ClientID: strings.TrimSpace(in.ClientId), SecretID: secretID,
			Scopes: orDefault(in.Scopes, "openid profile email groups"), UsernameClaim: orDefault(in.UsernameClaim, "preferred_username"),
			EmailClaim: orDefault(in.EmailClaim, "email"), GroupsClaim: orDefault(in.GroupsClaim, "groups"), IsDefault: def}
		var err error
		if isNew {
			err = q.InsertOIDCProvider(ctx, db.InsertOIDCProviderParams{ID: out.ID, Name: out.Name, Issuer: out.Issuer, ClientID: out.ClientID, SecretID: out.SecretID,
				Scopes: out.Scopes, UsernameClaim: out.UsernameClaim, EmailClaim: out.EmailClaim, GroupsClaim: out.GroupsClaim, IsDefault: def, CreatedAt: now, UpdatedAt: now})
		} else {
			err = q.UpdateOIDCProvider(ctx, db.UpdateOIDCProviderParams{Name: out.Name, Issuer: out.Issuer, ClientID: out.ClientID, SecretID: out.SecretID,
				Scopes: out.Scopes, UsernameClaim: out.UsernameClaim, EmailClaim: out.EmailClaim, GroupsClaim: out.GroupsClaim, IsDefault: def, UpdatedAt: now, ID: id})
		}
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE") {
				return conflict("Another provider already uses this name.")
			}
			return err
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "sso.provider_saved", IP: ri.ipString(), Target: "oidc_provider:" + id, Outcome: audit.Success,
			Detail: map[string]any{"name": out.Name, "issuer": out.Issuer}})
	})
	return out, err
}

func (s *Server) CreateSSOProvider(ctx context.Context, req gen.CreateSSOProviderRequestObject) (gen.CreateSSOProviderResponseObject, error) {
	p, err := s.saveProvider(ctx, "", *req.Body)
	if err != nil {
		return nil, err
	}
	return gen.CreateSSOProvider200JSONResponse(s.providerBody(ctx, p)), nil
}

func (s *Server) UpdateSSOProvider(ctx context.Context, req gen.UpdateSSOProviderRequestObject) (gen.UpdateSSOProviderResponseObject, error) {
	p, err := s.saveProvider(ctx, req.Id, *req.Body)
	if err != nil {
		return nil, err
	}
	return gen.UpdateSSOProvider200JSONResponse(s.providerBody(ctx, p)), nil
}

func (s *Server) DeleteSSOProvider(ctx context.Context, req gen.DeleteSSOProviderRequestObject) (gen.DeleteSSOProviderResponseObject, error) {
	ri := info(ctx)
	err := s.Store.Tx(ctx, func(q *db.Queries) error {
		n, err := q.CountRuleItemsUsingProvider(ctx, sql.NullString{String: req.Id, Valid: true})
		if err != nil {
			return err
		}
		if n > 0 {
			return conflict("Rules still name this provider.")
		}
		if err := q.DeleteOIDCProvider(ctx, req.Id); err != nil {
			return err
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "sso.provider_deleted", IP: ri.ipString(), Target: "oidc_provider:" + req.Id, Outcome: audit.Success})
	})
	if err != nil {
		return nil, err
	}
	return gen.DeleteSSOProvider204Response{}, nil
}

const geoKeyPurpose = "geoip:license-key"

// GeoSource reads the download source.
func GeoSource(ctx context.Context, q *db.Queries, v *vault.Vault) (geoip.Source, error) {
	cfg, err := settings.Get[settings.GeoIP](ctx, q)
	if err != nil {
		return geoip.Source{}, err
	}
	src := geoip.Source{AccountID: cfg.AccountID, URL: cfg.URL}
	rec, err := q.GetSecretByPurpose(ctx, geoKeyPurpose)
	if errors.Is(err, sql.ErrNoRows) {
		return src, nil
	}
	if err != nil {
		return src, err
	}
	key, err := v.Open(rec)
	if err != nil {
		return src, err
	}
	src.LicenseKey = key.Reveal()
	return src, nil
}

func (s *Server) geoStatus(ctx context.Context, problem string) (gen.GeoIPStatus, error) {
	cfg, err := settings.Get[settings.GeoIP](ctx, s.Store.R)
	if err != nil {
		return gen.GeoIPStatus{}, err
	}
	out := gen.GeoIPStatus{Databases: []gen.GeoIPDatabase{}, AccountId: cfg.AccountID, Url: cfg.URL, Error: problem}
	if _, err := s.Store.R.GetSecretByPurpose(ctx, geoKeyPurpose); err == nil {
		out.HasKey = true
	}
	if s.GeoIP != nil {
		for _, d := range s.GeoIP.Statuses() {
			out.Databases = append(out.Databases, gen.GeoIPDatabase{Edition: d.Edition, BuiltAt: d.Build, FetchedAt: d.Fetched, Stale: s.now().Sub(d.Build) > geoip.StaleAfter})
		}
	}
	return out, nil
}

func (s *Server) GetGeoIP(ctx context.Context, _ gen.GetGeoIPRequestObject) (gen.GetGeoIPResponseObject, error) {
	st, err := s.geoStatus(ctx, "")
	if err != nil {
		return nil, err
	}
	return gen.GetGeoIP200JSONResponse(st), nil
}

func (s *Server) PutGeoIP(ctx context.Context, req gen.PutGeoIPRequestObject) (gen.PutGeoIPResponseObject, error) {
	ri := info(ctx)
	cfg := settings.GeoIP{AccountID: orDefault(req.Body.AccountId, ""), URL: orDefault(req.Body.Url, "")}
	err := s.Store.Tx(ctx, func(q *db.Queries) error {
		if err := settings.Put(ctx, q, cfg); err != nil {
			return err
		}
		if req.Body.LicenseKey != nil && *req.Body.LicenseKey != "" {
			p, err := s.Vault.Seal(vault.Service, geoKeyPurpose, secrets.New(*req.Body.LicenseKey))
			if err != nil {
				return err
			}
			if err := q.DeleteSecretsByPurpose(ctx, geoKeyPurpose); err != nil {
				return err
			}
			if err := q.InsertSecret(ctx, p); err != nil {
				return err
			}
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "geoip.source_changed", IP: ri.ipString(), Target: "geoip", Outcome: audit.Success})
	})
	if errors.Is(err, settings.ErrInvalid) {
		return nil, invalid(strings.TrimPrefix(err.Error(), settings.ErrInvalid.Error()+": "))
	}
	if err != nil {
		return nil, err
	}
	src, err := GeoSource(ctx, s.Store.R, s.Vault)
	if err != nil {
		return nil, err
	}
	problem := ""
	dl, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if s.GeoIP != nil {
		changed, err := s.GeoIP.Update(dl, src, true)
		if err != nil {
			problem = err.Error()
		}
		if changed {
			s.IPSets.Changed(ctx)
			if err := s.Edge.Rebuild(ctx); err != nil {
				s.Log.Warn("forward auth after a GeoIP update", "err", err)
			}
		}
	}
	st, err := s.geoStatus(ctx, problem)
	if err != nil {
		return nil, err
	}
	return gen.PutGeoIP200JSONResponse(st), nil
}

func edgeSSOBody(v settings.EdgeSSO) gen.EdgeSSOSettings {
	return gen.EdgeSSOSettings{MaxSessionHours: v.MaxSessionHours, IdleTimeoutHours: v.IdleTimeoutHours, RememberDevice: v.RememberDevice}
}

func (s *Server) GetEdgeSSOSettings(ctx context.Context, _ gen.GetEdgeSSOSettingsRequestObject) (gen.GetEdgeSSOSettingsResponseObject, error) {
	v, err := settings.Get[settings.EdgeSSO](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	return gen.GetEdgeSSOSettings200JSONResponse(edgeSSOBody(v)), nil
}

func (s *Server) PutEdgeSSOSettings(ctx context.Context, req gen.PutEdgeSSOSettingsRequestObject) (gen.PutEdgeSSOSettingsResponseObject, error) {
	v := settings.EdgeSSO{MaxSessionHours: req.Body.MaxSessionHours, IdleTimeoutHours: req.Body.IdleTimeoutHours, RememberDevice: req.Body.RememberDevice}
	if err := saveSetting(ctx, s, v, nil); err != nil {
		return nil, err
	}
	return gen.PutEdgeSSOSettings200JSONResponse(edgeSSOBody(v)), nil
}

// refuseGeoWithoutDatabase is 05a §4.3: a firewall set of countries with no database would silently match nothing.
func (s *Server) refuseGeoWithoutDatabase(ctx context.Context, sets ...[]string) error {
	missing := s.geoMissing(ctx)
	for _, list := range sets {
		for _, id := range list {
			if missing(id) {
				return conflict("That set names countries or networks, but the GeoIP database is missing, so it would match nobody. Add the database under Security, GeoIP first.")
			}
		}
	}
	return nil
}
