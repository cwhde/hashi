package httpapi

import (
	"context"
	"errors"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/auth"
	"git.juzo.io/juzo/hashi/internal/certs"
	"git.juzo.io/juzo/hashi/internal/clientip"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/setup"
	"git.juzo.io/juzo/hashi/internal/store/db"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/traefik"
)

// ---- step 7: Hashi system service ----

// systemServiceBody is the step 7 response: the service, the plans shown, and the state after any step change.
func (s *Server) systemServiceBody(ctx context.Context, sv services.Service, certPlan, traefikPlan *hsync.Plan) (gen.SetupSystemService, error) {
	out := gen.SetupSystemService{Service: serviceBody(sv)}
	if certPlan != nil {
		out.CertificatePlan = ptr(s.planBody(ctx, certPlan))
	}
	if traefikPlan != nil {
		out.TraefikPlan = ptr(s.planBody(ctx, traefikPlan))
	}
	st, err := s.setupBody(ctx)
	out.State = st
	return out, err
}

// systemServiceServed reports whether the setup host serves the admin domain: a certificate covering it is issued and
// verified on the host with its current fingerprint. Routers are verified by every applied Traefik plan.
func (s *Server) systemServiceServed(ctx context.Context, connID, host string) (bool, string, error) {
	rows, err := s.Store.R.ListCertificates(ctx)
	if err != nil {
		return false, "", err
	}
	dists, err := s.Store.R.ListCertificateDistributions(ctx)
	if err != nil {
		return false, "", err
	}
	for _, r := range rows {
		if r.CertPem == "" || !certs.Serves(certs.NamesOf(r), host) {
			continue
		}
		for _, d := range dists {
			if d.CertificateID == r.ID && d.ConnectionID == connID && d.State == "verified" && d.Fingerprint == r.Fingerprint {
				return true, "", nil
			}
		}
		return false, "The Traefik host does not serve the certificate for " + host + " yet.", nil
	}
	return false, "No certificate covers " + host + " yet.", nil
}

// finishSystemService marks step 7 done once the host serves the admin domain, or records why not.
func (s *Server) finishSystemService(ctx context.Context, connID, host string) error {
	ok, why, err := s.systemServiceServed(ctx, connID, host)
	if err != nil {
		return err
	}
	if !ok {
		return s.Setup.MarkFailed(ctx, s.Store.Q, setup.StepSystemSvc, why)
	}
	return s.Setup.MarkDone(ctx, s.Store.Q, setup.StepSystemSvc, map[string]any{"host_name": host})
}

// CreateSetupSystemService is step 7 (02 §1.6): the admin service from the base settings, a certificate that covers
// the admin domain (issued now when a deeper name needs a new wildcard, about a minute) and the Traefik plan for the
// setup host. Firewall and DNS parts of this step arrive with S6.
func (s *Server) CreateSetupSystemService(ctx context.Context, _ gen.CreateSetupSystemServiceRequestObject) (gen.CreateSetupSystemServiceResponseObject, error) {
	ri := info(ctx)
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	row, ok, err := s.setupConnection(ctx)
	if err != nil {
		return nil, err
	}
	if !ok || !row.OwnershipConfirmedAt.Valid {
		return nil, conflict("Finish the Traefik connection (step 5) first.")
	}
	var sv services.Service
	if err := s.Store.Tx(ctx, func(q *db.Queries) error {
		var changed bool
		if sv, changed, err = services.EnsureSystem(ctx, q, s.now()); err != nil || !changed {
			return err
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "service.system.saved", IP: ri.ipString(),
			Target: "service:" + sv.ID, Detail: map[string]any{"host_name": sv.HostName}})
	}); err != nil {
		return nil, serviceError(err)
	}
	var certPlan *hsync.Plan
	if pending, err := s.Certs.Pending(ctx); err != nil {
		return nil, err
	} else if pending {
		target, err := s.Certs.IssueTarget(ctx)
		if err != nil {
			return nil, traefikError(err)
		}
		if certPlan, err = s.autoRun(ctx, certs.Subsystem, target); err != nil {
			return nil, traefikError(err)
		}
		if certPlan != nil && certPlan.Status != hsync.StatusApplied {
			body, err := s.systemServiceBody(ctx, sv, certPlan, nil)
			if err != nil {
				return nil, err
			}
			return gen.CreateSetupSystemService200JSONResponse(body), nil
		}
	}
	p, err := s.Sync.Propose(ctx, traefik.Subsystem, row.ID, hsync.TriggerManual, s.actor(ri))
	if err != nil {
		return nil, traefikError(err)
	}
	if p == nil {
		if err := s.finishSystemService(ctx, row.ID, sv.HostName); err != nil {
			return nil, err
		}
	}
	body, err := s.systemServiceBody(ctx, sv, certPlan, p)
	if err != nil {
		return nil, err
	}
	return gen.CreateSetupSystemService200JSONResponse(body), nil
}

// RunSetupSystemServicePlan approves and runs the shown Traefik plan (the click is the approval while setup has no
// passkey, as in step 5), then completes step 7 when the host serves the admin domain.
func (s *Server) RunSetupSystemServicePlan(ctx context.Context, req gen.RunSetupSystemServicePlanRequestObject) (gen.RunSetupSystemServicePlanResponseObject, error) {
	ri := info(ctx)
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	row, ok, err := s.setupConnection(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.Sync.Get(ctx, req.Id)
	if err != nil {
		return nil, syncError(err)
	}
	if !ok || p.Subsystem != traefik.Subsystem || p.Target != row.ID {
		return nil, notFound("No Traefik plan with that ID.")
	}
	sysRow, err := s.Store.R.GetServiceByOwner(ctx, services.OwnerSetup)
	if err != nil {
		return nil, conflict("Create the Hashi system service first.")
	}
	if p.Status == hsync.StatusPending {
		if err := s.Sync.Approve(ctx, p.ID, s.actor(ri), ri.ipString()); err != nil {
			return nil, syncError(err)
		}
	}
	ran, err := s.Sync.Apply(ctx, p.ID, s.actor(ri), ri.ipString())
	if err != nil {
		if errors.Is(err, hsync.ErrState) || errors.Is(err, hsync.ErrLocked) {
			return nil, syncError(err)
		}
		if ran, err = s.Sync.Get(ctx, p.ID); err != nil {
			return nil, syncError(err)
		}
	}
	if ran.Status == hsync.StatusApplied {
		if err := s.finishSystemService(ctx, row.ID, sysRow.HostName); err != nil {
			return nil, err
		}
	} else {
		_ = s.Setup.MarkFailed(ctx, s.Store.Q, setup.StepSystemSvc, ran.Error)
	}
	sv, err := services.Get(ctx, s.Store.R, sysRow.ID)
	if err != nil {
		return nil, err
	}
	body, err := s.systemServiceBody(ctx, sv, nil, ran)
	if err != nil {
		return nil, err
	}
	return gen.RunSetupSystemServicePlan200JSONResponse(body), nil
}

// ---- step 8: HTTPS reachability ----

func (s *Server) StartSetupHTTPS(ctx context.Context, _ gen.StartSetupHTTPSRequestObject) (gen.StartSetupHTTPSResponseObject, error) {
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	st, err := s.Setup.Load(ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	if st.Steps[setup.StepSystemSvc-1].Status != setup.Done {
		return nil, conflict("Publish the Hashi system service (step 7) first.")
	}
	g, err := settings.Get[settings.General](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	nonce, exp, err := s.Setup.NewHTTPSNonce()
	if err != nil {
		return nil, err
	}
	// The nonce travels in the fragment: browsers never send it, so it stays out of every access log.
	return gen.StartSetupHTTPS200JSONResponse{Url: "https://" + g.AdminDomain + "/setup#https=" + nonce, ExpiresAt: exp.UTC()}, nil
}

// ProveSetupHTTPS completes step 8 (02 §1 step 8): the request must come through Hashi-managed Traefik (edge token),
// for the admin domain, over HTTPS, from a bootstrap network, with the link's nonce. The browser is then signed in on
// the HTTPS origin with a new bootstrap session bound to its address there, because passkeys (step 9) need that origin.
func (s *Server) ProveSetupHTTPS(ctx context.Context, req gen.ProveSetupHTTPSRequestObject) (gen.ProveSetupHTTPSResponseObject, error) {
	ri := info(ctx)
	done, err := s.Setup.Completed(ctx)
	if err != nil {
		return nil, err
	}
	if done {
		return nil, forbidden("Setup is complete.")
	}
	g, err := settings.Get[settings.General](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	deny := func(reason, detail string) (gen.ProveSetupHTTPSResponseObject, error) {
		s.recordAnon(ctx, ri, "setup.https.proof", audit.Denied, map[string]any{"reason": reason, "host": ri.host})
		return nil, forbidden(detail)
	}
	switch {
	case !ri.edge:
		return deny("not through Hashi's Traefik", "This request did not come through a Traefik host Hashi manages.")
	case ri.host != g.AdminDomain:
		return deny("wrong host", "Open the link on https://"+g.AdminDomain+".")
	case !ri.tls:
		return deny("not https", "The request did not arrive over HTTPS.")
	}
	sec, err := settings.Get[settings.Security](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	allowed, err := clientip.ParsePrefixes(sec.BootstrapAllowedCIDRs)
	if err != nil {
		return nil, err
	}
	if !ri.ip.IsValid() || !clientip.InAny(ri.ip, allowed) {
		return deny("network not allowed", "Setup works only from a private network.")
	}
	if err := s.Setup.ConsumeHTTPSNonce(req.Body.Nonce); err != nil {
		return deny("nonce", err.Error())
	}
	var token string
	var sess *auth.Session
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		token, sess, err = s.Sessions.Issue(ctx, q, auth.MethodBootstrap, "", ri.ip, auth.BootstrapScopes, ri.ua)
		if err != nil {
			return err
		}
		if err := s.Setup.MarkDone(ctx, q, setup.StepHTTPS, map[string]any{"admin_domain": g.AdminDomain}); err != nil {
			return err
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.Audit.SessionActor(sess.IDHash), Action: "setup.https.proof",
			IP: ri.ipString(), Detail: map[string]any{"admin_domain": g.AdminDomain}})
	})
	if err != nil {
		return nil, err
	}
	body, err := s.sessionBody(ctx, sess)
	if err != nil {
		return nil, err
	}
	return gen.ProveSetupHTTPS200JSONResponse{Body: body, Headers: gen.ProveSetupHTTPS200ResponseHeaders{SetCookie: ptr(issueCookie(ri, token, sess))}}, nil
}
