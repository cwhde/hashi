package httpapi

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/url"
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

// captchaDefaults fills what Hashi knows when nothing is saved yet: Cap runs on the Docker host Hashi runs on, at the
// compose port (06 §11).
func (s *Server) captchaDefaults(ctx context.Context, c settings.Captcha) settings.Captcha {
	if c.CapAddress == "" {
		if g, err := settings.Get[settings.General](ctx, s.Store.R); err == nil {
			if u, err := url.Parse(g.InternalURL); err == nil {
				c.CapAddress = u.Hostname()
			}
		}
	}
	if s.CapPort > 0 && c.CapPort == settings.DefaultOf[settings.Captcha]().CapPort {
		c.CapPort = s.CapPort
	}
	return c
}

func (s *Server) captchaBody(ctx context.Context, c settings.Captcha) (gen.CaptchaSettings, error) {
	c = s.captchaDefaults(ctx, c)
	out := gen.CaptchaSettings{CanCreateKey: ptr(s.CapEnvFile != ""), Enabled: c.Enabled, ChallengeDomain: c.ChallengeDomain, CapAddress: c.CapAddress,
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

// CreateCaptchaKey asks the Cap next to Hashi for a site key (Hashi reads Cap's password from cap.env) and stores the
// key in the settings and its secret in the vault, so the admin never copies either.
func (s *Server) CreateCaptchaKey(ctx context.Context, _ gen.CreateCaptchaKeyRequestObject) (gen.CreateCaptchaKeyResponseObject, error) {
	ri := info(ctx)
	if s.CapEnvFile == "" {
		return nil, conflict("Hashi has no access to Cap's password (HASHI_CAP_ENV_FILE): enter a site key and secret instead.")
	}
	admin, err := captcha.AdminKey(s.CapEnvFile)
	if err != nil {
		return nil, conflict("Cap's password file cannot be read: " + err.Error())
	}
	c, err := settings.Get[settings.Captcha](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	c = s.captchaDefaults(ctx, c)
	kctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	siteKey, secret, err := captcha.CreateKey(kctx, http.DefaultClient, c.VerifyURL, admin, "hashi")
	if err != nil {
		return nil, conflict(err.Error())
	}
	c.SiteKey = siteKey
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		if err := settings.Put(ctx, q, c); err != nil {
			return err
		}
		p, err := s.Vault.Seal(vault.Service, CaptchaSecretPurpose, secret)
		if err != nil {
			return err
		}
		if err := q.DeleteSecretsByPurpose(ctx, CaptchaSecretPurpose); err != nil {
			return err
		}
		if err := q.InsertSecret(ctx, p); err != nil {
			return err
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "captcha.key_created", IP: ri.ipString(),
			Target: "captcha", Outcome: audit.Success, Detail: map[string]any{"site_key": siteKey}})
	})
	if err != nil {
		return nil, err
	}
	if s.Captcha != nil {
		s.Captcha.Set(c)
	}
	b, err := s.captchaBody(ctx, c)
	if err != nil {
		return nil, err
	}
	return gen.CreateCaptchaKey200JSONResponse(b), nil
}

func engineBody(e settings.Engine) gen.EngineSettings {
	return gen.EngineSettings{TcpNewPerMinute: e.TCPNewPerMinute, TcpBurst: e.TCPBurst, TcpMaxConcurrent: e.TCPMaxConcurrent,
		WafDefaultMode: e.WAFDefaultMode, WafViolations: e.WAFViolations,
		WafWindowSeconds: e.WAFWindowSeconds, RateRequests: e.RateRequests, RateWindowSeconds: e.RateWindowSeconds,
		ChallengeExpirySeconds: e.ChallengeExpirySecs, ChallengeMaxFailed: e.ChallengeMaxFailed,
		CaptchaClearanceHours: e.ClearanceHours, MetersEnabled: e.MetersEnabled,
		BanFunction: gen.EngineSettingsBanFunction(e.BanFunction), BanBaseSeconds: e.BanBaseSeconds, BanK: float32(e.BanK),
		BanMaxSeconds: e.BanMaxSeconds, BanPermanentAfter: e.BanPermanentAfter, CounterDecaySeconds: e.DecaySeconds,
		FeedRefreshSeconds: e.FeedRefreshSeconds, FeedStaleSeconds: e.FeedStaleSeconds}
}

func (s *Server) GetEngineSettings(ctx context.Context, _ gen.GetEngineSettingsRequestObject) (gen.GetEngineSettingsResponseObject, error) {
	e, err := settings.Get[settings.Engine](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	return gen.GetEngineSettings200JSONResponse(engineBody(e)), nil
}

// PutEngineSettings saves what the Engine tab shows; fields it does not show (the UDP meters, timers) stay as they are.
// The firewall replans through the settings subscription; the Traefik hosts are planned here, since the default WAF
// mode is part of their files.
func (s *Server) PutEngineSettings(ctx context.Context, req gen.PutEngineSettingsRequestObject) (gen.PutEngineSettingsResponseObject, error) {
	b := req.Body
	e, err := settings.Get[settings.Engine](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	e.WAFDefaultMode, e.WAFViolations, e.WAFWindowSeconds = b.WafDefaultMode, b.WafViolations, b.WafWindowSeconds
	e.RateRequests, e.RateWindowSeconds = b.RateRequests, b.RateWindowSeconds
	e.ChallengeExpirySecs, e.ChallengeMaxFailed, e.ClearanceHours = b.ChallengeExpirySeconds, b.ChallengeMaxFailed, b.CaptchaClearanceHours
	e.MetersEnabled, e.TCPNewPerMinute, e.TCPBurst, e.TCPMaxConcurrent = b.MetersEnabled, b.TcpNewPerMinute, b.TcpBurst, b.TcpMaxConcurrent
	e.BanFunction, e.BanBaseSeconds, e.BanK = string(b.BanFunction), b.BanBaseSeconds, float64(b.BanK)
	e.BanMaxSeconds, e.BanPermanentAfter, e.DecaySeconds = b.BanMaxSeconds, b.BanPermanentAfter, b.CounterDecaySeconds
	e.FeedRefreshSeconds, e.FeedStaleSeconds = b.FeedRefreshSeconds, b.FeedStaleSeconds
	if err := saveSetting(ctx, s, e, nil); err != nil {
		return nil, err
	}
	if _, _, err := s.syncServices(ctx); err != nil {
		s.Log.Warn("Traefik plans after engine settings", "err", err)
	}
	return gen.PutEngineSettings200JSONResponse(engineBody(e)), nil
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
