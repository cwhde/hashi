package httpapi

import (
	"context"
	"errors"
	"slices"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

func generalBody(g settings.General) gen.GeneralSettings {
	return gen.GeneralSettings{RootDomain: g.RootDomain, AdminDomain: g.AdminDomain, InternalUrl: g.InternalURL,
		SyncIntervalSeconds: g.SyncIntervalSeconds, TrustedProxies: g.TrustedProxies}
}

func securityBody(v settings.Security) gen.SecuritySettings {
	return gen.SecuritySettings{AdminIdleTimeoutMinutes: v.AdminIdleTimeoutMinutes, BootstrapAllowedCidrs: v.BootstrapAllowedCIDRs}
}

func appearanceBody(a settings.Appearance) gen.AppearanceSettings {
	return gen.AppearanceSettings{Theme: a.Theme, PublicDashboardEnabled: a.PublicDashboardEnabled, PublicStatusEnabled: a.PublicStatusEnabled}
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
func saveSetting[T settings.Category](ctx context.Context, s *Server, v T, check func(cur T) error) error {
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
		SyncIntervalSeconds: b.SyncIntervalSeconds, TrustedProxies: proxies}
	sess := info(ctx).session
	err := saveSetting(ctx, s, v, func(cur settings.General) error {
		if cur.Configured() && cur.RootDomain != v.RootDomain {
			// Passkeys are bound to the root domain (02 §2); changing it would lock the admin out.
			return conflict("The root domain is fixed after setup because passkeys are bound to it.")
		}
		if !slices.Equal(cur.TrustedProxies, v.TrustedProxies) && !sess.ReauthValid(s.now()) {
			return reauthRequired() // trusted-proxy change is high-risk (02 §2, 01 §8)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// The Hashi system service follows the admin domain and internal URL (02 §1.6: edited only here).
	if row, err := s.Store.R.GetServiceByOwner(ctx, services.OwnerSetup); err == nil {
		changed := false
		ri := info(ctx)
		if err := s.Store.Tx(ctx, func(q *db.Queries) error {
			sv, ch, err := services.EnsureSystem(ctx, q, s.now())
			if err != nil || !ch {
				return err
			}
			changed = true
			return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "service.system.saved", IP: ri.ipString(),
				Target: "service:" + row.ID, Detail: map[string]any{"host_name": sv.HostName}})
		}); err != nil {
			return nil, serviceError(err)
		}
		if changed {
			if _, _, err := s.syncServices(ctx); err != nil {
				return nil, err
			}
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
	v := settings.Appearance{Theme: req.Body.Theme, PublicDashboardEnabled: req.Body.PublicDashboardEnabled, PublicStatusEnabled: req.Body.PublicStatusEnabled}
	if err := saveSetting(ctx, s, v, nil); err != nil {
		return nil, err
	}
	return gen.PutAppearanceSettings200JSONResponse(appearanceBody(v)), nil
}
