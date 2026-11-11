package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strings"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/rules"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

func generalBody(g settings.General) gen.GeneralSettings {
	td := gen.GeneralSettingsTimeDisplay(settings.TimeLocal)
	if g.TimeDisplay == settings.TimeUTC {
		td = gen.GeneralSettingsTimeDisplay(settings.TimeUTC)
	}
	return gen.GeneralSettings{RootDomain: g.RootDomain, AdminDomain: g.AdminDomain, InternalUrl: g.InternalURL,
		SyncIntervalSeconds: g.SyncIntervalSeconds, TrustedProxies: g.TrustedProxies, TimeDisplay: &td}
}

func securityBody(v settings.Security) gen.SecuritySettings {
	return gen.SecuritySettings{AdminIdleTimeoutMinutes: v.AdminIdleTimeoutMinutes, BootstrapAllowedCidrs: v.BootstrapAllowedCIDRs}
}

func appearanceBody(a settings.Appearance) gen.AppearanceSettings {
	out := gen.AppearanceSettings{Theme: a.Theme, PublicDashboardEnabled: a.PublicDashboardEnabled, PublicStatusEnabled: a.PublicStatusEnabled,
		ReduceTransparency: ptr(a.ReduceTransparency)}
	if c := a.CustomTheme; c != nil {
		out.CustomTheme = &gen.CustomTheme{Canvas: c.Canvas, Surface: c.Surface, Text: c.Text, Muted: c.Muted, Border: c.Border,
			Accent: c.Accent, Roundness: gen.CustomThemeRoundness(c.Roundness)}
		if len(c.Charts) > 0 {
			out.CustomTheme.Charts = ptr(c.Charts)
		}
	}
	return out
}

func appearanceOf(b gen.AppearanceSettings) settings.Appearance {
	v := settings.Appearance{Theme: b.Theme, PublicDashboardEnabled: b.PublicDashboardEnabled, PublicStatusEnabled: b.PublicStatusEnabled,
		ReduceTransparency: deref(b.ReduceTransparency)}
	if c := b.CustomTheme; c != nil {
		v.CustomTheme = &settings.CustomTheme{Canvas: strings.ToLower(c.Canvas), Surface: strings.ToLower(c.Surface),
			Text: strings.ToLower(c.Text), Muted: strings.ToLower(c.Muted), Border: strings.ToLower(c.Border),
			Accent: strings.ToLower(c.Accent), Roundness: string(c.Roundness)}
		if c.Charts != nil {
			v.CustomTheme.Charts = *c.Charts
		}
	}
	return v
}

func (s *Server) GetSettings(ctx context.Context, _ gen.GetSettingsRequestObject) (gen.GetSettingsResponseObject, error) {
	g, err := settings.Get[settings.General](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	sec, err := settings.Get[settings.Security](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	a, err := settings.Get[settings.Appearance](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	return gen.GetSettings200JSONResponse{General: generalBody(g), Security: securityBody(sec), Appearance: appearanceBody(a)}, nil
}

// saveSetting validates and stores v with an audit event, then notifies consumers. check may veto the change
// after seeing the current value.
// saveSetting validates and stores one category with its audit entry. then runs in the same transaction after the
// write (a follow-up that must succeed or undo the save).
func saveSetting[T settings.Category](ctx context.Context, s *Server, v T, check func(cur T) error, then ...func(q *db.Queries) error) error {
	ri := info(ctx)
	err := s.Store.Tx(ctx, func(q *db.Queries) error {
		cur, err := settings.Get[T](ctx, q)
		if err != nil {
			return err
		}
		if check != nil {
			if err := check(cur); err != nil {
				return err
			}
		}
		if err := settings.Put(ctx, q, v); err != nil {
			return err
		}
		for _, f := range then {
			if err := f(q); err != nil {
				return err
			}
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "settings." + v.Key() + ".saved", IP: ri.ipString()})
	})
	if errors.Is(err, settings.ErrInvalid) {
		return invalid(err.Error())
	}
	if err != nil {
		return err
	}
	s.Settings.Notify(v)
	return nil
}

func (s *Server) GetGeneralSettings(ctx context.Context, _ gen.GetGeneralSettingsRequestObject) (gen.GetGeneralSettingsResponseObject, error) {
	g, err := settings.Get[settings.General](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	return gen.GetGeneralSettings200JSONResponse(generalBody(g)), nil
}

func (s *Server) PutGeneralSettings(ctx context.Context, req gen.PutGeneralSettingsRequestObject) (gen.PutGeneralSettingsResponseObject, error) {
	b := req.Body
	proxies := b.TrustedProxies
	if proxies == nil {
		proxies = []string{}
	}
	v := settings.General{RootDomain: b.RootDomain, AdminDomain: b.AdminDomain, InternalURL: b.InternalUrl,
		SyncIntervalSeconds: b.SyncIntervalSeconds, TrustedProxies: proxies, TimeDisplay: settings.TimeLocal}
	if b.TimeDisplay != nil {
		v.TimeDisplay = string(*b.TimeDisplay)
	}
	ri := info(ctx)
	sess := ri.session
	moved := false
	err := saveSetting(ctx, s, v, func(cur settings.General) error {
		if cur.Configured() && cur.RootDomain != v.RootDomain {
			// Passkeys are bound to the root domain (02 §2); changing it would lock the admin out.
			return conflict("The root domain is fixed after setup because passkeys are bound to it.")
		}
		if !slices.Equal(cur.TrustedProxies, v.TrustedProxies) && !sess.ReauthValid(s.now()) {
			return reauthRequired() // trusted-proxy change is high-risk (02 §2, 01 §8)
		}
		return nil
	}, func(q *db.Queries) error {
		// The Hashi system service follows the admin domain and internal URL (02 §1.6: edited only here). In the same
		// transaction: if the service cannot move (the name is taken), the settings stay as they were.
		if _, err := q.GetServiceByOwner(ctx, services.OwnerSetup); errors.Is(err, sql.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		sv, changed, err := services.EnsureSystem(ctx, q, s.now())
		if err != nil {
			return err
		}
		// The challenge service's Hashi half follows the internal URL too (06 §11).
		_, capChanged, err := services.EnsureChallenge(ctx, q, s.now())
		if err != nil {
			return err
		}
		if !changed && !capChanged {
			return nil
		}
		if err := rules.EnsureRoots(ctx, q, s.now()); err != nil {
			return err
		}
		moved = true
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "service.system.saved", IP: ri.ipString(),
			Target: "service:" + sv.ID, Detail: map[string]any{"host_name": sv.HostName}})
	})
	if err != nil {
		return nil, serviceError(err)
	}
	if moved {
		s.edgeChanged(ctx)
		if _, _, err := s.syncServices(ctx); err != nil {
			return nil, err
		}
	}
	return gen.PutGeneralSettings200JSONResponse(generalBody(v)), nil
}

func (s *Server) GetSecuritySettings(ctx context.Context, _ gen.GetSecuritySettingsRequestObject) (gen.GetSecuritySettingsResponseObject, error) {
	v, err := settings.Get[settings.Security](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	return gen.GetSecuritySettings200JSONResponse(securityBody(v)), nil
}

func (s *Server) PutSecuritySettings(ctx context.Context, req gen.PutSecuritySettingsRequestObject) (gen.PutSecuritySettingsResponseObject, error) {
	v := settings.Security{AdminIdleTimeoutMinutes: req.Body.AdminIdleTimeoutMinutes, BootstrapAllowedCIDRs: req.Body.BootstrapAllowedCidrs}
	if err := saveSetting(ctx, s, v, nil); err != nil {
		return nil, err
	}
	return gen.PutSecuritySettings200JSONResponse(securityBody(v)), nil
}

func (s *Server) GetAppearanceSettings(ctx context.Context, _ gen.GetAppearanceSettingsRequestObject) (gen.GetAppearanceSettingsResponseObject, error) {
	v, err := settings.Get[settings.Appearance](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	return gen.GetAppearanceSettings200JSONResponse(appearanceBody(v)), nil
}

func (s *Server) PutAppearanceSettings(ctx context.Context, req gen.PutAppearanceSettingsRequestObject) (gen.PutAppearanceSettingsResponseObject, error) {
	v := appearanceOf(*req.Body)
	if err := saveSetting(ctx, s, v, nil); err != nil {
		return nil, err
	}
	return gen.PutAppearanceSettings200JSONResponse(appearanceBody(v)), nil
}

func (s *Server) GetMonitoringSettings(ctx context.Context, _ gen.GetMonitoringSettingsRequestObject) (gen.GetMonitoringSettingsResponseObject, error) {
	v, err := settings.Get[settings.Monitoring](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	return gen.GetMonitoringSettings200JSONResponse(monitoringBody(v)), nil
}

func monitoringBody(v settings.Monitoring) gen.MonitoringSettings {
	return gen.MonitoringSettings{IntervalSeconds: v.IntervalSeconds, TimeoutSeconds: v.TimeoutSeconds, CodeMin: v.CodeMin,
		CodeMax: v.CodeMax, DegradedMs: v.DegradedMs, RetentionDays: v.RetentionDays}
}

func (s *Server) PutMonitoringSettings(ctx context.Context, req gen.PutMonitoringSettingsRequestObject) (gen.PutMonitoringSettingsResponseObject, error) {
	b := req.Body
	v := settings.Monitoring{IntervalSeconds: b.IntervalSeconds, TimeoutSeconds: b.TimeoutSeconds, CodeMin: b.CodeMin, CodeMax: b.CodeMax,
		DegradedMs: b.DegradedMs, RetentionDays: b.RetentionDays}
	if err := saveSetting(ctx, s, v, nil); err != nil {
		return nil, err
	}
	return gen.PutMonitoringSettings200JSONResponse(monitoringBody(v)), nil
}

func (s *Server) GetDashboardSettings(ctx context.Context, _ gen.GetDashboardSettingsRequestObject) (gen.GetDashboardSettingsResponseObject, error) {
	v, err := settings.Get[settings.Dashboard](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	return gen.GetDashboardSettings200JSONResponse{DefaultSort: gen.DashboardSettingsDefaultSort(v.DefaultSort), NewServicesVisible: v.NewServicesVisible}, nil
}

func (s *Server) PutDashboardSettings(ctx context.Context, req gen.PutDashboardSettingsRequestObject) (gen.PutDashboardSettingsResponseObject, error) {
	v := settings.Dashboard{DefaultSort: string(req.Body.DefaultSort), NewServicesVisible: req.Body.NewServicesVisible}
	if err := saveSetting(ctx, s, v, nil); err != nil {
		return nil, err
	}
	return gen.PutDashboardSettings200JSONResponse(*req.Body), nil
}

func (s *Server) GetFirewallDefaultsSettings(ctx context.Context, _ gen.GetFirewallDefaultsSettingsRequestObject) (gen.GetFirewallDefaultsSettingsResponseObject, error) {
	v, err := settings.Get[settings.FirewallDefaults](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	return gen.GetFirewallDefaultsSettings200JSONResponse{NetbirdMode: gen.FirewallDefaultsSettingsNetbirdMode(v.NetBirdMode),
		MssClamp: v.MSSClamp}, nil
}

func (s *Server) PutFirewallDefaultsSettings(ctx context.Context, req gen.PutFirewallDefaultsSettingsRequestObject) (gen.PutFirewallDefaultsSettingsResponseObject, error) {
	v := settings.FirewallDefaults{NetBirdMode: string(req.Body.NetbirdMode), MSSClamp: req.Body.MssClamp}
	if err := saveSetting(ctx, s, v, nil); err != nil {
		return nil, err
	}
	return gen.PutFirewallDefaultsSettings200JSONResponse(*req.Body), nil
}
