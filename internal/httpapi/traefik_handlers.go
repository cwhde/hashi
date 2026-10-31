package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"git.juzo.io/juzo/hashi/internal/certs"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/setup"
	"git.juzo.io/juzo/hashi/internal/store/db"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/traefik"
)

func traefikError(err error) error {
	switch {
	case errors.Is(err, traefik.ErrInvalid), errors.Is(err, certs.ErrInvalid):
		return invalid(err.Error())
	case errors.Is(err, traefik.ErrNotReady), errors.Is(err, certs.ErrNotConfigured):
		return conflict(err.Error())
	case errors.Is(err, sql.ErrNoRows):
		return notFound("No such Traefik connection or certificate.")
	}
	return syncError(err)
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func msTime(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := time.UnixMilli(v.Int64).UTC()
	return &t
}

// autoRun proposes a plan and runs it right away when it is low risk. A failed run is not an API error: the plan
// carries its error. nil means nothing needed changing.
func (s *Server) autoRun(ctx context.Context, subsystem, target string) (*hsync.Plan, error) {
	ri := info(ctx)
	p, err := s.Sync.Propose(ctx, subsystem, target, hsync.TriggerManual, s.actor(ri))
	if err != nil || p == nil || p.Status != hsync.StatusApproved {
		return p, err
	}
	if _, err := s.Sync.Apply(ctx, p.ID, s.actor(ri), ri.ipString()); err != nil && (errors.Is(err, hsync.ErrState) || errors.Is(err, hsync.ErrLocked)) {
		return nil, err
	}
	return s.Sync.Get(ctx, p.ID)
}

// ---- certificates ----

func providerBody(a db.AcmeAccount) gen.CertificateProvider {
	var resolvers []string
	_ = json.Unmarshal([]byte(a.ResolversJson), &resolvers)
	if resolvers == nil {
		resolvers = []string{}
	}
	return gen.CertificateProvider{Directory: gen.CertificateProviderDirectory(a.Directory), DirectoryUrl: a.DirectoryUrl,
		Email: a.Email, EabKid: a.EabKid, HasEabHmac: a.EabHmacSecretID.Valid, AccountRegistered: a.AccountUri != "",
		PropagationDelayS: int(a.PropagationDelayS), Resolvers: resolvers, Health: gen.CertificateProviderHealth(a.Health),
		LastValidatedAt: msTime(a.LastValidatedAt), LastError: a.LastValidationError}
}

func (s *Server) PutSetupCertificates(ctx context.Context, req gen.PutSetupCertificatesRequestObject) (gen.PutSetupCertificatesResponseObject, error) {
	ri := info(ctx)
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	b := req.Body
	in := certs.ProviderInput{Directory: string(b.Directory), DirectoryURL: str(b.DirectoryUrl), Email: b.Email, EABKID: str(b.EabKid)}
	if b.EabHmac != nil {
		in.EABHMAC = secrets.New(*b.EabHmac)
	}
	if b.PropagationDelayS != nil {
		in.PropagationDelayS = *b.PropagationDelayS
	}
	if b.Resolvers != nil {
		in.Resolvers = *b.Resolvers
	}
	a, err := s.Certs.Configure(ctx, s.actor(ri), ri.ipString(), in)
	if errors.Is(err, certs.ErrInvalid) {
		_ = s.Setup.MarkFailed(ctx, s.Store.Q, setup.StepCert, err.Error())
		return nil, invalid(err.Error())
	}
	if a.ID == "" {
		return nil, err
	}
	if err != nil {
		// Stored, but the CA refused or could not be reached: the step stays open with the reason.
		_ = s.Setup.MarkFailed(ctx, s.Store.Q, setup.StepCert, err.Error())
	} else if err := s.Setup.MarkDone(ctx, s.Store.Q, setup.StepCert, nil); err != nil {
		return nil, err
	}
	return gen.PutSetupCertificates200JSONResponse(providerBody(a)), nil
}

func (s *Server) IssueSetupCertificates(ctx context.Context, _ gen.IssueSetupCertificatesRequestObject) (gen.IssueSetupCertificatesResponseObject, error) {
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	target, err := s.Certs.IssueTarget(ctx)
	if err != nil {
		return nil, traefikError(err)
	}
	p, err := s.autoRun(ctx, certs.Subsystem, target)
	if err != nil {
		return nil, traefikError(err)
	}
	return gen.IssueSetupCertificates200JSONResponse(s.planResult(ctx, p)), nil
}

func (s *Server) ListCertificates(ctx context.Context, _ gen.ListCertificatesRequestObject) (gen.ListCertificatesResponseObject, error) {
	rows, err := s.Store.R.ListCertificates(ctx)
	if err != nil {
		return nil, err
	}
	dists, err := s.Store.R.ListCertificateDistributions(ctx)
	if err != nil {
		return nil, err
	}
	conns, err := s.Store.R.ListTraefikConnections(ctx)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, c := range conns {
		names[c.ID] = c.Name
	}
	out := gen.CertificateList{Certificates: make([]gen.Certificate, 0, len(rows))}
	for _, r := range rows {
		c := gen.Certificate{Id: r.ID, Name: r.Name, Names: []string{}, Issuer: r.Issuer, NotBefore: msTime(r.NotBefore),
			NotAfter: msTime(r.NotAfter), Fingerprint: r.Fingerprint, LastRenewalAt: msTime(r.LastRenewalAt),
			LastRenewalResult: gen.CertificateLastRenewalResult(r.LastRenewalResult), LastRenewalError: r.LastRenewalError,
			Distributions: []gen.CertificateDistribution{}}
		_ = json.Unmarshal([]byte(r.NamesJson), &c.Names)
		if r.NotAfter.Valid {
			c.Level = gen.CertificateLevel(certs.LevelOf(s.now(), time.UnixMilli(r.NotAfter.Int64)))
		}
		for _, d := range dists {
			if d.CertificateID == r.ID {
				c.Distributions = append(c.Distributions, gen.CertificateDistribution{ConnectionId: d.ConnectionID,
					ConnectionName: names[d.ConnectionID], State: gen.CertificateDistributionState(d.State),
					Fingerprint: d.Fingerprint, LastVerifiedAt: msTime(d.LastVerifiedAt), Error: d.Error})
			}
		}
		out.Certificates = append(out.Certificates, c)
	}
	return gen.ListCertificates200JSONResponse(out), nil
}

func (s *Server) RenewCertificate(ctx context.Context, req gen.RenewCertificateRequestObject) (gen.RenewCertificateResponseObject, error) {
	if _, err := s.Store.R.GetCertificate(ctx, req.Id); err != nil {
		return nil, traefikError(err)
	}
	p, err := s.autoRun(ctx, certs.Subsystem, req.Id)
	if err != nil {
		return nil, traefikError(err)
	}
	return gen.RenewCertificate200JSONResponse(s.planResult(ctx, p)), nil
}

// ---- traefik ----

func (s *Server) connectionBody(ctx context.Context, r db.TraefikConnection) (gen.TraefikConnection, error) {
	out := gen.TraefikConnection{SshTargetSetId: emptyNil(r.SshTargetSetID.String), Id: r.ID, Name: r.Name, Host: r.SshHost, Port: int(r.SshPort), User: r.SshUser,
		Auth: gen.TraefikConnectionAuth(r.Auth), HostKeyType: r.HostKeyType, HostKeyFingerprint: r.HostKeyFingerprint,
		InternalIp: r.InternalIp, OsFamily: r.OsFamily, PackageManager: r.PackageManager, InitSystem: r.InitSystem,
		ExistingConfigs: []string{}, TraefikBinary: r.TraefikBinary, ConfigDir: r.ConfigDir, ApiPort: int(r.ApiPort),
		InstalledVersion: r.InstalledVersion, OwnershipConfirmed: r.OwnershipConfirmedAt.Valid,
		Health: gen.TraefikConnectionHealth(r.Health), LastValidatedAt: msTime(r.LastValidatedAt), LastError: r.LastValidationError,
		Files: []gen.TraefikFile{}}
	if r.OsProbe != "" {
		if p, err := traefik.ParseProbe(r.OsProbe); err == nil {
			out.OsName, out.Arch = p.PrettyName, p.Arch
		}
	}
	var disc traefik.Discovery
	if json.Unmarshal([]byte(r.DiscoveryJson), &disc) == nil {
		out.ExistingBinary = disc.ExistingBinary
		if disc.ExistingConfigs != nil {
			out.ExistingConfigs = disc.ExistingConfigs
		}
	}
	files, err := s.Store.R.ListTraefikFiles(ctx, r.ID)
	if err != nil {
		return out, err
	}
	for _, f := range files {
		out.Files = append(out.Files, gen.TraefikFile{Path: f.Path, Sha256: f.LastAppliedHash, AppliedAt: time.UnixMilli(f.AppliedAt).UTC()})
	}
	return out, nil
}

// setupConnection is the wizard's Traefik connection (the first one; setup configures exactly one).
func (s *Server) setupConnection(ctx context.Context) (db.TraefikConnection, bool, error) {
	conns, err := s.Store.R.ListTraefikConnections(ctx)
	if err != nil || len(conns) == 0 {
		return db.TraefikConnection{}, false, err
	}
	return conns[0], true, nil
}

func (s *Server) SetupTraefikIdentity(ctx context.Context, _ gen.SetupTraefikIdentityRequestObject) (gen.SetupTraefikIdentityResponseObject, error) {
	ri := info(ctx)
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	line, err := s.Traefik.Identity(ctx, s.actor(ri), ri.ipString())
	if err != nil {
		return nil, err
	}
	return gen.SetupTraefikIdentity200JSONResponse{PublicKey: line}, nil
}

func (s *Server) ScanSetupTraefikHost(ctx context.Context, req gen.ScanSetupTraefikHostRequestObject) (gen.ScanSetupTraefikHostResponseObject, error) {
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	hk, err := s.Traefik.ScanHostKey(ctx, req.Body.Host, req.Body.Port)
	if err != nil {
		return nil, providerFailed("Could not read the SSH host key: " + err.Error())
	}
	return gen.ScanSetupTraefikHost200JSONResponse{Type: hk.Type, Fingerprint: hk.Fingerprint, Key: hk.Key}, nil
}

func (s *Server) PutSetupTraefikConnection(ctx context.Context, req gen.PutSetupTraefikConnectionRequestObject) (gen.PutSetupTraefikConnectionResponseObject, error) {
	ri := info(ctx)
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	b := req.Body
	in := traefik.ConnectionInput{Name: b.Name, Host: b.Host, Port: b.Port, User: b.User, Auth: string(b.Auth), HostKey: b.HostKey,
		InternalIP: str(b.InternalIp), Binary: str(b.TraefikBinary), ConfigDir: str(b.ConfigDir)}
	if b.Credential != nil {
		in.Credential = secrets.New(*b.Credential)
	}
	if b.Passphrase != nil {
		in.Passphrase = secrets.New(*b.Passphrase)
	}
	if b.ApiPort != nil {
		in.APIPort = *b.ApiPort
	}
	cur, exists, err := s.setupConnection(ctx)
	if err != nil {
		return nil, err
	}
	id := ""
	if exists {
		id = cur.ID
	}
	row, err := s.Traefik.Save(ctx, s.actor(ri), ri.ipString(), id, in)
	if err != nil {
		return nil, traefikError(err)
	}
	if row, _, err = s.Traefik.Validate(ctx, row.ID); err != nil {
		// Failed validation is shown on the connection (health, last error); the step does not advance.
		_ = s.Setup.MarkFailed(ctx, s.Store.Q, setup.StepTraefik, err.Error())
		if row, err = s.Store.R.GetTraefikConnection(ctx, row.ID); err != nil {
			return nil, err
		}
	} else if err := s.Setup.MarkInProgress(ctx, s.Store.Q, setup.StepTraefik, nil); err != nil {
		return nil, err
	}
	body, err := s.connectionBody(ctx, row)
	if err != nil {
		return nil, err
	}
	return gen.PutSetupTraefikConnection200JSONResponse(body), nil
}

func (s *Server) ConfirmSetupTraefik(ctx context.Context, _ gen.ConfirmSetupTraefikRequestObject) (gen.ConfirmSetupTraefikResponseObject, error) {
	ri := info(ctx)
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	row, ok, err := s.setupConnection(ctx)
	if err != nil {
		return nil, err
	}
	if !ok || row.Health != "healthy" {
		return nil, conflict("Save and validate the Traefik host first.")
	}
	if err := s.Traefik.ConfirmOwnership(ctx, s.actor(ri), ri.ipString(), row.ID); err != nil {
		return nil, traefikError(err)
	}
	if err := s.Setup.MarkDone(ctx, s.Store.Q, setup.StepTraefik, nil); err != nil {
		return nil, err
	}
	body, err := s.setupBody(ctx)
	if err != nil {
		return nil, err
	}
	return gen.ConfirmSetupTraefik200JSONResponse(body), nil
}

func (s *Server) PlanSetupTraefik(ctx context.Context, _ gen.PlanSetupTraefikRequestObject) (gen.PlanSetupTraefikResponseObject, error) {
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	row, ok, err := s.setupConnection(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, conflict("Save and validate the Traefik host first.")
	}
	p, err := s.Sync.Propose(ctx, traefik.Subsystem, row.ID, hsync.TriggerManual, s.actor(info(ctx)))
	if err != nil {
		return nil, traefikError(err)
	}
	return gen.PlanSetupTraefik200JSONResponse(s.planResult(ctx, p)), nil
}

// RunSetupTraefikPlan approves and runs the Traefik plan the wizard showed: like the DNS write test, the explicit
// click on the shown plan is the approval while setup has no passkey yet. Only plans for the setup connection qualify.
func (s *Server) RunSetupTraefikPlan(ctx context.Context, req gen.RunSetupTraefikPlanRequestObject) (gen.RunSetupTraefikPlanResponseObject, error) {
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
	return gen.RunSetupTraefikPlan200JSONResponse(s.planBody(ctx, ran)), nil
}

func (s *Server) ListTraefikConnections(ctx context.Context, _ gen.ListTraefikConnectionsRequestObject) (gen.ListTraefikConnectionsResponseObject, error) {
	rows, err := s.Store.R.ListTraefikConnections(ctx)
	if err != nil {
		return nil, err
	}
	out := gen.TraefikConnectionList{Connections: make([]gen.TraefikConnection, 0, len(rows))}
	for _, r := range rows {
		c, err := s.connectionBody(ctx, r)
		if err != nil {
			return nil, err
		}
		out.Connections = append(out.Connections, c)
	}
	return gen.ListTraefikConnections200JSONResponse(out), nil
}

func (s *Server) PlanTraefikConnection(ctx context.Context, req gen.PlanTraefikConnectionRequestObject) (gen.PlanTraefikConnectionResponseObject, error) {
	if _, err := s.Store.R.GetTraefikConnection(ctx, req.Id); err != nil {
		return nil, traefikError(err)
	}
	p, err := s.autoRun(ctx, traefik.Subsystem, req.Id)
	if err != nil {
		return nil, traefikError(err)
	}
	return gen.PlanTraefikConnection200JSONResponse(s.planResult(ctx, p)), nil
}

// strPtr is nil for an empty string (optional response fields).
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
