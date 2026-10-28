package httpapi

import (
	"context"
	"errors"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/auth"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/setup"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

func (s *Server) setupBody(ctx context.Context) (gen.SetupState, error) {
	st, err := s.Setup.Load(ctx, s.Store.R)
	if err != nil {
		return gen.SetupState{}, err
	}
	out := gen.SetupState{Completed: st.Completed, Steps: make([]gen.SetupStep, len(st.Steps))}
	for i, x := range st.Steps {
		out.Steps[i] = gen.SetupStep{
			N: x.N, Key: x.Key, Title: x.Title, Required: x.Required, Built: x.Built, Slice: x.Slice,
			Status: gen.SetupStepStatus(x.Status), LastError: x.LastError,
		}
	}
	// Inputs only for the setup session itself; anyone may see which steps are done (the SPA routes on it).
	if sess := info(ctx).session; sess != nil && sess.Has(auth.SetupManage) {
		g, err := settings.Get[settings.General](ctx, s.Store.R)
		if err != nil {
			return out, err
		}
		a, err := settings.Get[settings.Appearance](ctx, s.Store.R)
		if err != nil {
			return out, err
		}
		if g.Configured() {
			out.BaseSettings = &gen.BaseSettings{
				RootDomain: g.RootDomain, AdminDomain: g.AdminDomain, InternalUrl: g.InternalURL,
				SyncIntervalSeconds: g.SyncIntervalSeconds, Theme: a.Theme,
				PublicDashboardEnabled: a.PublicDashboardEnabled, PublicStatusEnabled: a.PublicStatusEnabled,
			}
		}
		n, err := s.Store.R.CountPasskeys(ctx)
		if err != nil {
			return out, err
		}
		out.Passkeys = ptr(int(n))
		if out.Dns, err = s.setupDNSBody(ctx); err != nil {
			return out, err
		}
	}
	return out, nil
}

func setupError(err error) error {
	switch {
	case errors.Is(err, setup.ErrCompleted), errors.Is(err, setup.ErrNotReady), errors.Is(err, setup.ErrNoPasskey),
		errors.Is(err, setup.ErrNoPendingKey):
		return conflict(err.Error())
	case errors.Is(err, setup.ErrChecksumMismatch):
		return invalid(err.Error())
	}
	return err
}

func (s *Server) GetSetup(ctx context.Context, _ gen.GetSetupRequestObject) (gen.GetSetupResponseObject, error) {
	body, err := s.setupBody(ctx)
	if err != nil {
		return nil, err
	}
	return gen.GetSetup200JSONResponse(body), nil
}

func (s *Server) PutSetupBaseSettings(ctx context.Context, req gen.PutSetupBaseSettingsRequestObject) (gen.PutSetupBaseSettingsResponseObject, error) {
	ri := info(ctx)
	if done, err := s.Setup.Completed(ctx); err != nil {
		return nil, err
	} else if done {
		return nil, conflict(setup.ErrCompleted.Error())
	}
	b := req.Body
	var g settings.General
	var a settings.Appearance
	err := s.Store.Tx(ctx, func(q *db.Queries) error {
		cur, err := settings.Get[settings.General](ctx, q)
		if err != nil {
			return err
		}
		g = settings.General{
			RootDomain: b.RootDomain, AdminDomain: b.AdminDomain, InternalURL: b.InternalUrl,
			SyncIntervalSeconds: b.SyncIntervalSeconds, TrustedProxies: cur.TrustedProxies,
		}
		a = settings.Appearance{Theme: b.Theme, PublicDashboardEnabled: b.PublicDashboardEnabled, PublicStatusEnabled: b.PublicStatusEnabled}
		for _, c := range []settings.Category{g, a} {
			if err := settings.Put(ctx, q, c); err != nil {
				return err
			}
		}
		if cur.Configured() && cur.RootDomain != g.RootDomain {
			if err := s.Setup.RootDomainChanged(ctx, q, g.RootDomain); err != nil {
				return err
			}
		}
		if err := s.Setup.MarkDone(ctx, q, setup.StepBase, nil); err != nil {
			return err
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "setup.base_settings", IP: ri.ipString(),
			Detail: map[string]any{"root_domain": g.RootDomain, "admin_domain": g.AdminDomain}})
	})
	if errors.Is(err, settings.ErrInvalid) {
		// Validation failed: the step does not advance; record why.
		_ = s.Setup.MarkFailed(ctx, s.Store.Q, setup.StepBase, err.Error())
		return nil, invalid(err.Error())
	}
	if err != nil {
		return nil, err
	}
	s.Settings.Notify(g)
	s.Settings.Notify(a)
	body, err := s.setupBody(ctx)
	if err != nil {
		return nil, err
	}
	return gen.PutSetupBaseSettings200JSONResponse(body), nil
}

func (s *Server) SetupPasskeyOptions(ctx context.Context, _ gen.SetupPasskeyOptionsRequestObject) (gen.SetupPasskeyOptionsResponseObject, error) {
	if done, err := s.Setup.Completed(ctx); err != nil {
		return nil, err
	} else if done {
		return nil, conflict(setup.ErrCompleted.Error())
	}
	id, opts, err := s.Passkeys.BeginRegistration(ctx, info(ctx).session.IDHash)
	if err != nil {
		return nil, ceremonyError(err)
	}
	m, err := optionsMap(opts)
	if err != nil {
		return nil, err
	}
	return gen.SetupPasskeyOptions200JSONResponse{CeremonyId: id, Options: m}, nil
}

func (s *Server) SetupPasskeyVerify(ctx context.Context, req gen.SetupPasskeyVerifyRequestObject) (gen.SetupPasskeyVerifyResponseObject, error) {
	if done, err := s.Setup.Completed(ctx); err != nil {
		return nil, err
	} else if done {
		return nil, conflict(setup.ErrCompleted.Error())
	}
	pk, err := s.registerPasskey(ctx, req.Body)
	if err != nil {
		return nil, err
	}
	return gen.SetupPasskeyVerify200JSONResponse(passkeyBody(pk)), nil
}

func (s *Server) SetupRecoveryKey(ctx context.Context, _ gen.SetupRecoveryKeyRequestObject) (gen.SetupRecoveryKeyResponseObject, error) {
	key, err := s.Setup.GenerateRecoveryKey(ctx, info(ctx).session.IDHash)
	if err != nil {
		return nil, setupError(err)
	}
	// The one place the recovery key leaves the server, once (02 §5.3).
	return gen.SetupRecoveryKey200JSONResponse{RecoveryKey: key.Reveal()}, nil
}

func (s *Server) ConfirmRecoveryKey(ctx context.Context, req gen.ConfirmRecoveryKeyRequestObject) (gen.ConfirmRecoveryKeyResponseObject, error) {
	if err := s.Setup.ConfirmRecoveryKey(ctx, info(ctx).session.IDHash, req.Body.Checksum); err != nil {
		return nil, setupError(err)
	}
	body, err := s.setupBody(ctx)
	if err != nil {
		return nil, err
	}
	return gen.ConfirmRecoveryKey200JSONResponse(body), nil
}

func (s *Server) CompleteSetup(ctx context.Context, _ gen.CompleteSetupRequestObject) (gen.CompleteSetupResponseObject, error) {
	ri := info(ctx)
	if err := s.Setup.Complete(ctx, s.actor(ri), ri.ipString()); err != nil {
		return nil, setupError(err)
	}
	// The bootstrap session that made this call is revoked now; drop its in-memory material too.
	s.Passkeys.DropSession(ri.session.IDHash)
	s.Setup.DropSession(ri.session.IDHash)
	body, err := s.setupBody(ctx)
	if err != nil {
		return nil, err
	}
	return gen.CompleteSetup200JSONResponse(body), nil
}
