package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/captcha"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/rules"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/vault"
)

func (s *Server) captchaBody(ctx context.Context, c settings.Captcha) (gen.CaptchaSettings, error) {
	out := gen.CaptchaSettings{Enabled: c.Enabled, ChallengeDomain: c.ChallengeDomain, CapAddress: c.CapAddress,
		CapPort: c.CapPort, VerifyUrl: c.VerifyURL, SiteKey: c.SiteKey, TimeoutSeconds: c.TimeoutSeconds,
		AdminServiceId: strPtr(c.AdminServiceID)}
	if _, err := s.Store.R.GetSecretByPurpose(ctx, CaptchaSecretPurpose); err == nil {
		out.HasSecret = true
	} else if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	if row, err := s.Store.R.GetServiceByOwner(ctx, services.OwnerCaptcha); err == nil {
		out.ChallengeServiceId = &row.ID
	} else if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	return out, nil
}

func (s *Server) GetCaptchaSettings(ctx context.Context, _ gen.GetCaptchaSettingsRequestObject) (gen.GetCaptchaSettingsResponseObject, error) {
	c, err := settings.Get[settings.Captcha](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	b, err := s.captchaBody(ctx, c)
	if err != nil {
		return nil, err
	}
	return gen.GetCaptchaSettings200JSONResponse(b), nil
}

// PutCaptchaSettings saves the settings and the secret, and keeps the challenge service in step in the same
// transaction: if the name is taken, nothing is saved (06 §11).
func (s *Server) PutCaptchaSettings(ctx context.Context, req gen.PutCaptchaSettingsRequestObject) (gen.PutCaptchaSettingsResponseObject, error) {
	ri := info(ctx)
	in := req.Body
	c := settings.Captcha{Enabled: in.Enabled, ChallengeDomain: strings.ToLower(strings.TrimSuffix(in.ChallengeDomain, ".")),
		CapAddress: in.CapAddress, CapPort: in.CapPort, VerifyURL: strings.TrimSuffix(in.VerifyUrl, "/"), SiteKey: in.SiteKey,
		TimeoutSeconds: in.TimeoutSeconds, AdminServiceID: deref(in.AdminServiceId)}
	newSecret := deref(in.SecretKey)
	if newSecret != "" && !strings.HasPrefix(newSecret, "sk-") {
		return nil, invalid("A Cap secret key starts with sk-.")
	}
	err := s.Store.Tx(ctx, func(q *db.Queries) error {
		if c.AdminServiceID != "" {
			sv, err := services.Get(ctx, q, c.AdminServiceID)
			if err != nil {
				return invalid("The Cap dashboard's service does not exist.")
			}
			if sv.System() {
				return invalid("The Cap dashboard's service is a service of its own, not a system service.")
			}
		}
		if err := settings.Put(ctx, q, c); err != nil {
			return err
		}
		if newSecret != "" {
			p, err := s.Vault.Seal(vault.Service, CaptchaSecretPurpose, secrets.New(newSecret))
			if err != nil {
				return err
			}
			if err := q.DeleteSecretsByPurpose(ctx, CaptchaSecretPurpose); err != nil {
				return err
			}
			if err := q.InsertSecret(ctx, p); err != nil {
				return err
			}
		}
		if c.Enabled {
			if _, err := q.GetSecretByPurpose(ctx, CaptchaSecretPurpose); errors.Is(err, sql.ErrNoRows) {
				return invalid("CAPTCHA needs Cap's secret key.")
			} else if err != nil {
				return err
			}
		}
		if _, _, err := services.EnsureChallenge(ctx, q, s.now()); err != nil {
			return err
		}
		if err := rules.EnsureRoots(ctx, q, s.now()); err != nil {
			return err
		}
		detail := map[string]any{"enabled": c.Enabled, "challenge_domain": c.ChallengeDomain, "site_key": c.SiteKey}
		if newSecret != "" {
			detail["secret_rotated"] = true
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "captcha.settings_changed", IP: ri.ipString(),
			Target: "captcha", Outcome: audit.Success, Detail: detail})
	})
	if errors.Is(err, settings.ErrInvalid) {
		return nil, invalid(strings.TrimPrefix(err.Error(), settings.ErrInvalid.Error()+": "))
	}
	if err != nil {
		return nil, serviceError(err)
	}
	if s.Captcha != nil {
		s.Captcha.Set(c)
	}
	s.edgeChanged(ctx)
	plans, pending, err := s.syncServices(ctx)
	if err != nil {
		return nil, err
	}
	b, err := s.captchaBody(ctx, c)
	if err != nil {
		return nil, err
	}
	return gen.PutCaptchaSettings200JSONResponse{Settings: b, Plans: plans, CertificatePending: pending}, nil
}

func (s *Server) TestCaptcha(ctx context.Context, _ gen.TestCaptchaRequestObject) (gen.TestCaptchaResponseObject, error) {
	c, err := settings.Get[settings.Captcha](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	secret, err := s.captchaSecret(ctx)
	if errors.Is(err, sql.ErrNoRows) || c.SiteKey == "" || c.VerifyURL == "" {
		return gen.TestCaptcha200JSONResponse{Ok: false, Message: "Set the site key, the secret key and the verify address first."}, nil
	}
	if err != nil {
		return nil, err
	}
	tctx, cancel := context.WithTimeout(ctx, time.Duration(c.TimeoutSeconds)*time.Second)
	defer cancel()
	if err := captcha.Test(tctx, http.DefaultClient, c.VerifyURL, c.SiteKey, secret); err != nil {
		return gen.TestCaptcha200JSONResponse{Ok: false, Message: err.Error()}, nil
	}
	return gen.TestCaptcha200JSONResponse{Ok: true, Message: "Cap accepts the site key and secret key."}, nil
}
