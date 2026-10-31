package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"

	"git.juzo.io/juzo/hashi/internal/firewall"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/setup"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/store/db"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
)

func firewallError(err error) error {
	switch {
	case errors.Is(err, firewall.ErrInvalid):
		return invalid(strings.TrimPrefix(err.Error(), firewall.ErrInvalid.Error()+": "))
	case errors.Is(err, firewall.ErrNotFound):
		return notFound("No such firewall host.")
	case errors.Is(err, firewall.ErrNotReady), errors.Is(err, firewall.ErrInUse):
		return conflict(err.Error())
	}
	return syncError(err)
}

func firewallInput(b gen.FirewallHostInput) firewall.HostInput {
	in := firewall.HostInput{Name: b.Name, EdgeMode: string(b.EdgeMode), Host: str(b.Host), User: str(b.User),
		TraefikConnectionID: str(b.TraefikConnectionId), InternalTraefikIP: str(b.InternalTraefikIp),
		RouteViaHostID: str(b.RouteViaHostId), PublicIPOverride: str(b.PublicIpOverride),
		PublicIP6Override: str(b.PublicIp6Override), WANOverride: str(b.WanIfaceOverride), NetBirdIface: str(b.NetbirdIface),
		DropInbound: deref(b.DropOtherInbound)}
	if b.Port != nil {
		in.Port = *b.Port
	}
	if b.Auth != nil {
		in.Auth = string(*b.Auth)
	}
	if b.Credential != nil {
		in.Credential = secrets.New(*b.Credential)
	}
	if b.Passphrase != nil {
		in.Passphrase = secrets.New(*b.Passphrase)
	}
	if b.HostKey != nil {
		in.HostKey = *b.HostKey
	}
	if b.Subnets != nil {
		in.Subnets = *b.Subnets
	}
	if b.NetbirdMode != nil {
		in.NetBirdMode = string(*b.NetbirdMode)
	}
	if b.NetbirdPort != nil {
		in.NetBirdPort = *b.NetbirdPort
	}
	return in
}

func firewallBody(row db.FirewallHost) gen.FirewallHost {
	e := firewall.EffectiveOf(row)
	addr := func(a netip.Addr) string {
		if a.IsValid() {
			return a.String()
		}
		return ""
	}
	subnets := make([]string, 0, len(e.Subnets))
	for _, p := range e.Subnets {
		subnets = append(subnets, p.String())
	}
	out := gen.FirewallHost{Id: row.ID, Name: row.Name, EdgeMode: gen.FirewallHostEdgeMode(row.EdgeMode), Host: row.SshHost,
		Port: int(row.SshPort), User: row.SshUser, Auth: gen.FirewallHostAuth(row.Auth), HostKeyFingerprint: row.HostKeyFingerprint,
		TraefikConnectionId: row.TraefikConnectionID.String, InternalTraefikIp: row.InternalTraefikIp,
		RouteViaHostId: row.RouteViaHostID.String, PublicIp: addr(e.PublicIP), PublicIp6: addr(e.PublicIP6),
		PublicIpOverride: row.PublicIpOverride, PublicIp6Override: row.PublicIp6Override, WanIface: e.WAN,
		WanIfaceOverride: row.WanIfaceOverride, Subnets: subnets, NetbirdMode: gen.FirewallHostNetbirdMode(row.NetbirdMode),
		NetbirdIface: row.NetbirdIface, NetbirdPort: int(row.NetbirdPort), DropOtherInbound: row.DropInboundAcceptedAt.Valid,
		OwnershipConfirmed: row.OwnershipConfirmedAt.Valid, Health: gen.FirewallHostHealth(row.Health),
		LastError: row.LastValidationError, LastValidatedAt: msTime(row.LastValidatedAt), LastAppliedAt: msTime(row.LastAppliedAt)}
	if f := e.Facts; f.NftVersion != "" || f.Kernel != "" {
		facts := gen.FirewallFacts{Kernel: f.Kernel, Os: f.OSPretty, NftVersion: f.NftVersion, IptablesBackend: f.IptablesBackend,
			Tables: nonNil(f.Tables), Wan: f.WAN, Candidates: []string{}, SshFrom: addr(f.SSHFrom), Warnings: nonNil(f.Warnings())}
		for _, c := range f.Candidates() {
			facts.Candidates = append(facts.Candidates, c.String())
		}
		if nb := f.NetBird; nb != nil {
			routed := []string{}
			for _, r := range nb.Routed {
				routed = append(routed, r.String())
			}
			facts.Netbird = &gen.FirewallNetBird{Version: nb.Version, Connected: nb.Connected, Overlay: nb.Overlay.Masked().String(),
				Port: nb.Port, KernelInterface: nb.Kernel, Iface: nb.Iface, Routed: routed}
		}
		out.Facts = &facts
	}
	return out
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// saveFirewall saves and, for a managed host, validates right away; a failed validation is reported in the body.
func (s *Server) saveFirewall(ctx context.Context, id string, b gen.FirewallHostInput) (gen.FirewallHost, error) {
	ri := info(ctx)
	row, err := s.Firewall.Save(ctx, s.actor(ri), ri.ipString(), id, firewallInput(b))
	if err != nil {
		return gen.FirewallHost{}, firewallError(err)
	}
	if row.EdgeMode == firewall.ModeManaged {
		_, _ = s.Firewall.Validate(ctx, row.ID) // stored as health + last_error
		if row, err = s.Store.R.GetFirewallHost(ctx, row.ID); err != nil {
			return gen.FirewallHost{}, err
		}
	}
	s.syncRecords(ctx) // host records follow the name, public address and host override (03 §5.1)
	return firewallBody(row), nil
}

// ---- setup step 6 ----

// setupFirewallHost is the first host: the one setup step 6 creates and plans.
func (s *Server) setupFirewallHost(ctx context.Context) (db.FirewallHost, bool, error) {
	rows, err := s.Store.R.ListFirewallHosts(ctx)
	if err != nil || len(rows) == 0 {
		return db.FirewallHost{}, false, err
	}
	first := rows[0]
	for _, r := range rows {
		if r.CreatedAt < first.CreatedAt {
			first = r
		}
	}
	return first, true, nil
}

func (s *Server) ScanSetupFirewallHost(ctx context.Context, req gen.ScanSetupFirewallHostRequestObject) (gen.ScanSetupFirewallHostResponseObject, error) {
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	hk, err := s.Firewall.ScanHostKey(ctx, req.Body.Host, req.Body.Port)
	if err != nil {
		return nil, providerFailed("Could not read the SSH host key: " + err.Error())
	}
	return gen.ScanSetupFirewallHost200JSONResponse{Type: hk.Type, Fingerprint: hk.Fingerprint, Key: hk.Key}, nil
}

func (s *Server) PutSetupFirewallHost(ctx context.Context, req gen.PutSetupFirewallHostRequestObject) (gen.PutSetupFirewallHostResponseObject, error) {
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	cur, exists, err := s.setupFirewallHost(ctx)
	if err != nil {
		return nil, err
	}
	id := ""
	if exists {
		id = cur.ID
	}
	body, err := s.saveFirewall(ctx, id, *req.Body)
	if err != nil {
		return nil, err
	}
	return gen.PutSetupFirewallHost200JSONResponse(body), nil
}

// DeleteSetupFirewallHost lets the wizard start over: a mistyped address or login must not need a database reset.
func (s *Server) DeleteSetupFirewallHost(ctx context.Context, _ gen.DeleteSetupFirewallHostRequestObject) (gen.DeleteSetupFirewallHostResponseObject, error) {
	ri := info(ctx)
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	row, ok, err := s.setupFirewallHost(ctx)
	if err != nil {
		return nil, err
	}
	if ok {
		if row.OwnershipConfirmedAt.Valid {
			return nil, conflict("This host is confirmed already; delete it from the Firewall page after setup.")
		}
		if err := s.Firewall.Delete(ctx, s.actor(ri), ri.ipString(), row.ID); err != nil {
			return nil, firewallError(err)
		}
		s.syncRecords(ctx)
	}
	body, err := s.setupBody(ctx)
	if err != nil {
		return nil, err
	}
	return gen.DeleteSetupFirewallHost200JSONResponse(body), nil
}

func (s *Server) ConfirmSetupFirewall(ctx context.Context, _ gen.ConfirmSetupFirewallRequestObject) (gen.ConfirmSetupFirewallResponseObject, error) {
	ri := info(ctx)
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	row, ok, err := s.setupFirewallHost(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, conflict("Save the firewall host first.")
	}
	// Step 6 (02 §1): SSH validated, tools detected, subnets valid, linked to a Traefik connection. An external_nat
	// host has nothing to validate: its name and public address are what DNS and hashi-hosts need.
	if row.EdgeMode == firewall.ModeManaged {
		if row.Health == "failed" || !row.LastValidatedAt.Valid || !row.TraefikConnectionID.Valid {
			return nil, conflict("Validate the firewall host first (it must log in, read the host and be linked to a Traefik connection).")
		}
		if err := s.Firewall.ConfirmOwnership(ctx, s.actor(ri), ri.ipString(), row.ID); err != nil {
			return nil, firewallError(err)
		}
	}
	if err := s.Setup.MarkDone(ctx, s.Store.Q, setup.StepFirewall, nil); err != nil {
		return nil, err
	}
	body, err := s.setupBody(ctx)
	if err != nil {
		return nil, err
	}
	return gen.ConfirmSetupFirewall200JSONResponse(body), nil
}

func (s *Server) PlanSetupFirewall(ctx context.Context, _ gen.PlanSetupFirewallRequestObject) (gen.PlanSetupFirewallResponseObject, error) {
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	row, ok, err := s.setupFirewallHost(ctx)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, conflict("Save and confirm the firewall host first.")
	}
	p, err := s.Sync.Propose(ctx, firewall.Subsystem, row.ID, hsync.TriggerManual, s.actor(info(ctx)))
	if err != nil {
		return nil, firewallError(err)
	}
	return gen.PlanSetupFirewall200JSONResponse(s.planResult(ctx, p)), nil
}

// RunSetupFirewallPlan approves and runs the firewall plan the wizard showed (the click on the shown plan is the
// approval while setup has no passkey yet). Only plans for the setup host qualify; the rescue timer guards the run.
func (s *Server) RunSetupFirewallPlan(ctx context.Context, req gen.RunSetupFirewallPlanRequestObject) (gen.RunSetupFirewallPlanResponseObject, error) {
	ri := info(ctx)
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	row, ok, err := s.setupFirewallHost(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.Sync.Get(ctx, req.Id)
	if err != nil {
		return nil, syncError(err)
	}
	if !ok || p.Subsystem != firewall.Subsystem || p.Target != row.ID {
		return nil, notFound("No firewall plan with that ID.")
	}
	ran, err := s.approveAndRun(ctx, p, ri)
	if err != nil {
		return nil, err
	}
	return gen.RunSetupFirewallPlan200JSONResponse(s.planBody(ctx, ran)), nil
}

func (s *Server) approveAndRun(ctx context.Context, p *hsync.Plan, ri *request) (*hsync.Plan, error) {
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
	return ran, nil
}

// ---- hosts ----

func (s *Server) ListFirewallHosts(ctx context.Context, _ gen.ListFirewallHostsRequestObject) (gen.ListFirewallHostsResponseObject, error) {
	rows, err := s.Store.R.ListFirewallHosts(ctx)
	if err != nil {
		return nil, err
	}
	out := gen.FirewallHostList{Hosts: make([]gen.FirewallHost, 0, len(rows))}
	for _, r := range rows {
		out.Hosts = append(out.Hosts, firewallBody(r))
	}
	return gen.ListFirewallHosts200JSONResponse(out), nil
}

func (s *Server) CreateFirewallHost(ctx context.Context, req gen.CreateFirewallHostRequestObject) (gen.CreateFirewallHostResponseObject, error) {
	body, err := s.saveFirewall(ctx, "", *req.Body)
	if err != nil {
		return nil, err
	}
	return gen.CreateFirewallHost200JSONResponse(body), nil
}

func (s *Server) ScanFirewallHost(ctx context.Context, req gen.ScanFirewallHostRequestObject) (gen.ScanFirewallHostResponseObject, error) {
	hk, err := s.Firewall.ScanHostKey(ctx, req.Body.Host, req.Body.Port)
	if err != nil {
		return nil, providerFailed("Could not read the SSH host key: " + err.Error())
	}
	return gen.ScanFirewallHost200JSONResponse{Type: hk.Type, Fingerprint: hk.Fingerprint, Key: hk.Key}, nil
}

func (s *Server) GetFirewallHost(ctx context.Context, req gen.GetFirewallHostRequestObject) (gen.GetFirewallHostResponseObject, error) {
	row, err := s.Store.R.GetFirewallHost(ctx, req.Id)
	if err != nil {
		return nil, firewallError(firewall.ErrNotFound)
	}
	return gen.GetFirewallHost200JSONResponse(firewallBody(row)), nil
}

func (s *Server) UpdateFirewallHost(ctx context.Context, req gen.UpdateFirewallHostRequestObject) (gen.UpdateFirewallHostResponseObject, error) {
	body, err := s.saveFirewall(ctx, req.Id, *req.Body)
	if err != nil {
		return nil, err
	}
	return gen.UpdateFirewallHost200JSONResponse(body), nil
}

func (s *Server) DeleteFirewallHost(ctx context.Context, req gen.DeleteFirewallHostRequestObject) (gen.DeleteFirewallHostResponseObject, error) {
	ri := info(ctx)
	if err := s.Firewall.Delete(ctx, s.actor(ri), ri.ipString(), req.Id); err != nil {
		return nil, firewallError(err)
	}
	s.syncRecords(ctx)
	return gen.DeleteFirewallHost204Response{}, nil
}

func (s *Server) ValidateFirewallHost(ctx context.Context, req gen.ValidateFirewallHostRequestObject) (gen.ValidateFirewallHostResponseObject, error) {
	if _, err := s.Firewall.Validate(ctx, req.Id); errors.Is(err, firewall.ErrNotFound) || errors.Is(err, firewall.ErrInvalid) {
		return nil, firewallError(err)
	}
	row, err := s.Store.R.GetFirewallHost(ctx, req.Id)
	if err != nil {
		return nil, firewallError(firewall.ErrNotFound)
	}
	return gen.ValidateFirewallHost200JSONResponse(firewallBody(row)), nil
}

func (s *Server) ConfirmFirewallHost(ctx context.Context, req gen.ConfirmFirewallHostRequestObject) (gen.ConfirmFirewallHostResponseObject, error) {
	ri := info(ctx)
	if err := s.Firewall.ConfirmOwnership(ctx, s.actor(ri), ri.ipString(), req.Id); err != nil {
		return nil, firewallError(err)
	}
	row, err := s.Store.R.GetFirewallHost(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return gen.ConfirmFirewallHost200JSONResponse(firewallBody(row)), nil
}

func (s *Server) PlanFirewallHost(ctx context.Context, req gen.PlanFirewallHostRequestObject) (gen.PlanFirewallHostResponseObject, error) {
	p, err := s.autoRun(ctx, firewall.Subsystem, req.Id)
	if err != nil {
		return nil, firewallError(err)
	}
	return gen.PlanFirewallHost200JSONResponse(s.planResult(ctx, p)), nil
}

func (s *Server) GetFirewallRendered(ctx context.Context, req gen.GetFirewallRenderedRequestObject) (gen.GetFirewallRenderedResponseObject, error) {
	d, err := s.Firewall.Rendered(ctx, req.Id)
	if err != nil {
		return nil, firewallError(err)
	}
	f := d.Spec.Folded
	out := gen.FirewallRendered{Table: string(d.Table), Allow: len(f.Allow4) + len(f.Allow6), Block: len(f.Block4) + len(f.Block6),
		HostAccess: len(d.Spec.HostAccess), Ports: []string{}}
	for _, p := range d.Spec.Ports {
		out.Ports = append(out.Ports, fmt.Sprintf("%s/%d → %d", p.Proto, p.Port, p.TargetPort))
	}
	return gen.GetFirewallRendered200JSONResponse(out), nil
}

func (s *Server) GetFirewallRuleset(ctx context.Context, req gen.GetFirewallRulesetRequestObject) (gen.GetFirewallRulesetResponseObject, error) {
	now, atSetup, err := s.Firewall.Ruleset(ctx, req.Id)
	if errors.Is(err, firewall.ErrNotFound) {
		return nil, firewallError(err)
	}
	if err != nil {
		return nil, providerFailed("Could not read the host: " + err.Error())
	}
	return gen.GetFirewallRuleset200JSONResponse{Now: now, AtSetup: atSetup}, nil
}

// ---- bindings and host access ----

func (s *Server) GetFirewallBindings(ctx context.Context, _ gen.GetFirewallBindingsRequestObject) (gen.GetFirewallBindingsResponseObject, error) {
	b, err := s.Firewall.GetBindings(ctx)
	if err != nil {
		return nil, err
	}
	return gen.GetFirewallBindings200JSONResponse{Allow: b.Allow, Block: b.Block}, nil
}

func (s *Server) PutFirewallBindings(ctx context.Context, req gen.PutFirewallBindingsRequestObject) (gen.PutFirewallBindingsResponseObject, error) {
	ri := info(ctx)
	if err := s.Firewall.PutBindings(ctx, s.actor(ri), ri.ipString(), firewall.Bindings{Allow: req.Body.Allow, Block: req.Body.Block}); err != nil {
		return nil, firewallError(err)
	}
	plans, err := s.firewallPlans(ctx)
	if err != nil {
		return nil, err
	}
	return gen.PutFirewallBindings200JSONResponse(plans), nil
}

func (s *Server) GetFirewallHostAccess(ctx context.Context, req gen.GetFirewallHostAccessRequestObject) (gen.GetFirewallHostAccessResponseObject, error) {
	sets, err := s.Firewall.GetHostAccess(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return gen.GetFirewallHostAccess200JSONResponse{Sets: sets}, nil
}

func (s *Server) PutFirewallHostAccess(ctx context.Context, req gen.PutFirewallHostAccessRequestObject) (gen.PutFirewallHostAccessResponseObject, error) {
	ri := info(ctx)
	if err := s.Firewall.PutHostAccess(ctx, s.actor(ri), ri.ipString(), req.Id, req.Body.Sets); err != nil {
		return nil, firewallError(err)
	}
	plans, err := s.firewallPlans(ctx)
	if err != nil {
		return nil, err
	}
	return gen.PutFirewallHostAccess200JSONResponse(plans), nil
}

// firewallPlans plans every managed host after a binding or access change (05 §4.4: element changes apply, the rest
// waits for approval).
func (s *Server) firewallPlans(ctx context.Context) (gen.FirewallPlans, error) {
	plans, err := s.Firewall.PlanAll(ctx, s.Sync, s.actor(info(ctx)))
	out := gen.FirewallPlans{Plans: make([]gen.SyncPlan, 0, len(plans))}
	for _, p := range plans {
		out.Plans = append(out.Plans, s.planBody(ctx, p))
	}
	if err != nil {
		s.Log.Warn("firewall plans", "err", err)
	}
	return out, nil
}

func (s *Server) GetFirewallTakeover(ctx context.Context, req gen.GetFirewallTakeoverRequestObject) (gen.GetFirewallTakeoverResponseObject, error) {
	row, err := s.Store.R.GetFirewallHost(ctx, req.Id)
	if err != nil {
		return nil, notFound("No such firewall host.")
	}
	t, err := firewall.ParseTakeover(row.TakeoverJson)
	if err != nil {
		return nil, err
	}
	return gen.GetFirewallTakeover200JSONResponse{Chains: list(t.Chains), Rules: list(t.Rules), Ipsets: list(t.IPSets),
		CronMatch: list(t.CronMatch), CronFiles: list(t.CronFiles)}, nil
}

func (s *Server) PutFirewallTakeover(ctx context.Context, req gen.PutFirewallTakeoverRequestObject) (gen.PutFirewallTakeoverResponseObject, error) {
	ri, b := info(ctx), req.Body
	t := firewall.Takeover{Chains: b.Chains, Rules: b.Rules, IPSets: b.Ipsets, CronMatch: b.CronMatch, CronFiles: b.CronFiles}
	if err := s.Firewall.PutTakeover(ctx, s.actor(ri), ri.ipString(), req.Id, t); err != nil {
		return nil, firewallError(err)
	}
	plans, err := s.firewallPlans(ctx)
	if err != nil {
		return nil, err
	}
	return gen.PutFirewallTakeover200JSONResponse(plans), nil
}

func list(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// keyInstallConfig is the one-time login of a key install; nothing of it is stored.
func keyInstallConfig(b *gen.KeyInstallInput) (sshx.Config, error) {
	hk, err := sshx.ParseHostKey(b.HostKey)
	if err != nil {
		return sshx.Config{}, invalid("Read and confirm the host key first.")
	}
	a := sshx.Auth{User: b.User}
	if b.Password != nil {
		a.Password = secrets.New(*b.Password) // hashi:allow-secret-pattern
	}
	if b.PrivateKey != nil {
		a.PrivateKey = secrets.New(*b.PrivateKey)
	}
	if b.Passphrase != nil {
		a.Passphrase = secrets.New(*b.Passphrase)
	}
	if a.Password.IsZero() == a.PrivateKey.IsZero() {
		return sshx.Config{}, invalid("Give a password or a private key.")
	}
	return sshx.Config{Addr: sshx.Addr(b.Host, b.Port), Pinned: hk, Auth: a}, nil
}

func keyInstallError(err error) error {
	switch {
	case errors.Is(err, sshx.ErrInstall):
		return invalid(strings.TrimPrefix(err.Error(), sshx.ErrInstall.Error()+": "))
	case errors.Is(err, sshx.ErrHostKeyMismatch):
		return conflict("The host presented another key than the one you confirmed. Check the address and read the key again.")
	}
	return providerFailed("Could not reach the host: " + err.Error())
}

func (s *Server) installKey(ctx context.Context, b *gen.KeyInstallInput, onFirewall bool) (string, error) {
	ri := info(ctx)
	c, err := keyInstallConfig(b)
	if err != nil {
		return "", err
	}
	line, err := s.Traefik.Identity(ctx, s.actor(ri), ri.ipString())
	if err != nil {
		return "", err
	}
	if onFirewall {
		err = s.Firewall.InstallKey(ctx, s.actor(ri), ri.ipString(), c, line)
	} else {
		err = s.Traefik.InstallKey(ctx, s.actor(ri), ri.ipString(), c, line)
	}
	if err != nil {
		return "", keyInstallError(err)
	}
	return line, nil
}

func (s *Server) InstallKeySetupEdge(ctx context.Context, req gen.InstallKeySetupEdgeRequestObject) (gen.InstallKeySetupEdgeResponseObject, error) {
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	line, err := s.installKey(ctx, req.Body, false)
	if err != nil {
		return nil, err
	}
	return gen.InstallKeySetupEdge200JSONResponse{PublicKey: line}, nil
}

func (s *Server) InstallKeySetupFw(ctx context.Context, req gen.InstallKeySetupFwRequestObject) (gen.InstallKeySetupFwResponseObject, error) {
	if err := s.requireSetupOpen(ctx); err != nil {
		return nil, err
	}
	line, err := s.installKey(ctx, req.Body, true)
	if err != nil {
		return nil, err
	}
	return gen.InstallKeySetupFw200JSONResponse{PublicKey: line}, nil
}

func (s *Server) InstallKeyFirewall(ctx context.Context, req gen.InstallKeyFirewallRequestObject) (gen.InstallKeyFirewallResponseObject, error) {
	line, err := s.installKey(ctx, req.Body, true)
	if err != nil {
		return nil, err
	}
	return gen.InstallKeyFirewall200JSONResponse{PublicKey: line}, nil
}

func (s *Server) FirewallIdentity(ctx context.Context, _ gen.FirewallIdentityRequestObject) (gen.FirewallIdentityResponseObject, error) {
	ri := info(ctx)
	line, err := s.Traefik.Identity(ctx, s.actor(ri), ri.ipString())
	if err != nil {
		return nil, err
	}
	return gen.FirewallIdentity200JSONResponse{PublicKey: line}, nil
}
