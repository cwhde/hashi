package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/pulse"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

func pulseError(err error) error {
	switch {
	case errors.Is(err, pulse.ErrNotFound):
		return notFound(err.Error())
	case errors.Is(err, pulse.ErrInUse):
		return conflict(err.Error())
	case errors.Is(err, pulse.ErrInvalid):
		return invalid(err.Error())
	case errors.Is(err, pulse.ErrToken):
		return &Problem{Type: "/problems/unauthenticated", Title: "Invalid token", Status: http.StatusUnauthorized, Detail: "The agent token is not valid."}
	case errors.Is(err, pulse.ErrRate):
		return &Problem{Type: "/problems/rate-limited", Title: "Too many heartbeats", Status: http.StatusTooManyRequests, Detail: "Wait before sending another heartbeat."}
	}
	return err
}

func reachable(r db.PulseAgent) *bool {
	if r.ReachPort == 0 || !r.ReachOk.Valid {
		return nil
	}
	v := r.ReachOk.Int64 == 1
	return &v
}

func (s *Server) pulseBody(ctx context.Context, r db.PulseAgent) gen.PulseAgent {
	c, err := settings.Get[settings.Pulse](ctx, s.Store.R)
	if err != nil {
		c = settings.Pulse{StaleFactor: 3}
	}
	cands := []string{}
	for _, c := range pulse.Candidates(r) {
		cands = append(cands, c.Address)
	}
	return gen.PulseAgent{Id: r.ID, Name: r.Name, Status: gen.PulseAgentStatus(pulse.Status(r, time.Now(), c.StaleFactor)), InstallType: r.InstallType, PublicIp: r.PublicIp, SelectedIp: r.SelectedIp,
		Candidates: cands, Hostname: r.Hostname, Version: r.Version, IntervalSeconds: int(r.IntervalSeconds), LastSeenAt: millis(r.LastSeenAt), DnsEnabled: r.DnsEnabled == 1, DnsName: r.DnsName, TrustReportedIp: r.TrustReportedIp == 1, PublicIpSource: gen.PulseAgentPublicIpSource(r.PublicIpSource), ReachPort: int(r.ReachPort), Reachable: reachable(r)}
}

// PulseHeartbeat takes an agent's report. It needs the agent's token and nothing else (08 §3).
func (s *Server) PulseHeartbeat(ctx context.Context, req gen.PulseHeartbeatRequestObject) (gen.PulseHeartbeatResponseObject, error) {
	token, ok := strings.CutPrefix(req.Params.Authorization, "Bearer ")
	if !ok || token == "" {
		return nil, pulseError(pulse.ErrToken)
	}
	b := pulse.Beat{Version: req.Body.Version, Hostname: req.Body.Hostname}
	if req.Body.SelectedInterface != nil {
		b.SelectedInterface = *req.Body.SelectedInterface
	}
	for _, c := range req.Body.Candidates {
		b.Candidates = append(b.Candidates, pulse.Candidate{Interface: c.Interface, Address: c.Address})
	}
	b.InDocker = req.Body.InDocker != nil && *req.Body.InDocker
	if req.Body.IntervalSeconds != nil {
		b.IntervalSeconds = *req.Body.IntervalSeconds
	}
	if req.Body.ReportedPublicIp != nil {
		b.ReportedPublic = *req.Body.ReportedPublicIp
	}
	r, err := s.Pulse.Heartbeat(ctx, req.AgentId, token, b, info(ctx).ip)
	if err != nil {
		return nil, pulseError(err)
	}
	return gen.PulseHeartbeat200JSONResponse{CheckPublicIp: r.CheckPublicIP, IntervalSeconds: r.IntervalSeconds}, nil
}

func (s *Server) ListPulseAgents(ctx context.Context, _ gen.ListPulseAgentsRequestObject) (gen.ListPulseAgentsResponseObject, error) {
	rows, err := s.Store.R.ListPulseAgents(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]gen.PulseAgent, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.pulseBody(ctx, r))
	}
	g, err := settings.Get[settings.General](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	url := ""
	if g.AdminDomain != "" {
		url = "https://" + g.AdminDomain
	}
	return gen.ListPulseAgents200JSONResponse{Agents: out, HashiUrl: url}, nil
}

func (s *Server) CreatePulseAgent(ctx context.Context, req gen.CreatePulseAgentRequestObject) (gen.CreatePulseAgentResponseObject, error) {
	ri := info(ctx)
	install := ""
	if req.Body.InstallType != nil {
		install = string(*req.Body.InstallType)
	}
	row, token, err := s.Pulse.Create(ctx, req.Body.Name, install)
	if err != nil {
		return nil, pulseError(err)
	}
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: "pulse.agent.created", Target: row.ID, IP: ri.ipString()}); err != nil {
		return nil, err
	}
	return gen.CreatePulseAgent201JSONResponse{Agent: s.pulseBody(ctx, row), Token: token.Reveal()}, nil
}

func (s *Server) UpdatePulseAgent(ctx context.Context, req gen.UpdatePulseAgentRequestObject) (gen.UpdatePulseAgentResponseObject, error) {
	name := ""
	if req.Body.DnsName != nil {
		name = *req.Body.DnsName
	}
	port := 0
	if req.Body.ReachPort != nil {
		port = *req.Body.ReachPort
	}
	row, err := s.Pulse.SetDNS(ctx, req.Id, req.Body.Name, req.Body.DnsEnabled, name, req.Body.TrustReportedIp, port)
	if err != nil {
		return nil, pulseError(err)
	}
	ri := info(ctx)
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: "pulse.agent.updated", Target: row.ID, IP: ri.ipString(),
		Detail: map[string]any{"trust_reported_ip": req.Body.TrustReportedIp, "dns_enabled": req.Body.DnsEnabled}}); err != nil {
		return nil, err
	}
	return gen.UpdatePulseAgent200JSONResponse(s.pulseBody(ctx, row)), nil
}

func (s *Server) DeletePulseAgent(ctx context.Context, req gen.DeletePulseAgentRequestObject) (gen.DeletePulseAgentResponseObject, error) {
	ri := info(ctx)
	if err := s.Pulse.Delete(ctx, req.Id); err != nil {
		return nil, pulseError(err)
	}
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: "pulse.agent.deleted", Target: req.Id, IP: ri.ipString()}); err != nil {
		return nil, err
	}
	return gen.DeletePulseAgent204Response{}, nil
}

func (s *Server) RotatePulseToken(ctx context.Context, req gen.RotatePulseTokenRequestObject) (gen.RotatePulseTokenResponseObject, error) {
	ri := info(ctx)
	row, token, err := s.Pulse.Rotate(ctx, req.Id)
	if err != nil {
		return nil, pulseError(err)
	}
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: "pulse.agent.token_rotated", Target: row.ID, IP: ri.ipString()}); err != nil {
		return nil, err
	}
	return gen.RotatePulseToken200JSONResponse{Agent: s.pulseBody(ctx, row), Token: token.Reveal()}, nil
}

func (s *Server) RevokePulseAgent(ctx context.Context, req gen.RevokePulseAgentRequestObject) (gen.RevokePulseAgentResponseObject, error) {
	ri := info(ctx)
	row, err := s.Pulse.Revoke(ctx, req.Id)
	if err != nil {
		return nil, pulseError(err)
	}
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: "pulse.agent.revoked", Target: row.ID, IP: ri.ipString()}); err != nil {
		return nil, err
	}
	return gen.RevokePulseAgent200JSONResponse(s.pulseBody(ctx, row)), nil
}

func (s *Server) GetAgentDNSSettings(ctx context.Context, _ gen.GetAgentDNSSettingsRequestObject) (gen.GetAgentDNSSettingsResponseObject, error) {
	v, err := settings.Get[settings.AgentDNS](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	return gen.GetAgentDNSSettings200JSONResponse{Enabled: v.Enabled, Domain: v.Domain, IpMode: gen.AgentDNSSettingsIpMode(v.IPMode), RemoveStaleAfterHours: intPtr(v.RemoveStaleAfterHours)}, nil
}

func (s *Server) PutAgentDNSSettings(ctx context.Context, req gen.PutAgentDNSSettingsRequestObject) (gen.PutAgentDNSSettingsResponseObject, error) {
	v := settings.AgentDNS{Enabled: req.Body.Enabled, Domain: req.Body.Domain, IPMode: string(req.Body.IpMode)}
	if req.Body.RemoveStaleAfterHours != nil {
		v.RemoveStaleAfterHours = *req.Body.RemoveStaleAfterHours
	}
	if err := saveSetting(ctx, s, v, nil); err != nil {
		return nil, err
	}
	return gen.PutAgentDNSSettings200JSONResponse{Enabled: v.Enabled, Domain: v.Domain, IpMode: gen.AgentDNSSettingsIpMode(v.IPMode), RemoveStaleAfterHours: intPtr(v.RemoveStaleAfterHours)}, nil
}

func (s *Server) PreviewAgentDNS(ctx context.Context, _ gen.PreviewAgentDNSRequestObject) (gen.PreviewAgentDNSResponseObject, error) {
	names, err := s.Pulse.Names(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(struct {
		Rewrites []map[string]any `json:"rewrites"`
	}{Rewrites: agentNames(names)})
	if err != nil {
		return nil, err
	}
	var out gen.PreviewAgentDNS200JSONResponse
	return out, json.Unmarshal(raw, &out)
}

func agentNames(names []pulse.AgentName) []map[string]any {
	out := make([]map[string]any, 0, len(names))
	for _, n := range names {
		m := map[string]any{"agent_id": n.AgentID, "agent": n.Agent, "domain": n.Domain, "answer": n.Answer, "status": n.Status, "collision": n.Collision, "wanted": n.Wanted}
		if n.Problem != "" {
			m["problem"] = n.Problem
		}
		out = append(out, m)
	}
	return out
}

func (s *Server) GetPulseSettings(ctx context.Context, _ gen.GetPulseSettingsRequestObject) (gen.GetPulseSettingsResponseObject, error) {
	v, err := settings.Get[settings.Pulse](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	return gen.GetPulseSettings200JSONResponse{HeartbeatIntervalSeconds: v.HeartbeatIntervalSeconds, StaleFactor: v.StaleFactor}, nil
}

func (s *Server) PutPulseSettings(ctx context.Context, req gen.PutPulseSettingsRequestObject) (gen.PutPulseSettingsResponseObject, error) {
	v := settings.Pulse{HeartbeatIntervalSeconds: req.Body.HeartbeatIntervalSeconds, StaleFactor: req.Body.StaleFactor}
	if err := saveSetting(ctx, s, v, nil); err != nil {
		return nil, err
	}
	return gen.PutPulseSettings200JSONResponse{HeartbeatIntervalSeconds: v.HeartbeatIntervalSeconds, StaleFactor: v.StaleFactor}, nil
}

func (s *Server) GetPulseConsumers(ctx context.Context, req gen.GetPulseConsumersRequestObject) (gen.GetPulseConsumersResponseObject, error) {
	if _, err := s.Store.R.GetPulseAgent(ctx, req.Id); err != nil {
		return nil, pulseError(pulse.ErrNotFound)
	}
	rows, err := s.Store.R.ListIPSetsUsingAgent(ctx, sql.NullString{String: req.Id, Valid: true})
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	sets, consumers := s.IPSets.Impact(ctx, ids)
	if sets == nil {
		sets = []string{}
	}
	if consumers == nil {
		consumers = []string{}
	}
	return gen.GetPulseConsumers200JSONResponse{Sets: sets, Consumers: consumers}, nil
}

func (s *Server) GetPulseInstall(ctx context.Context, req gen.GetPulseInstallRequestObject) (gen.GetPulseInstallResponseObject, error) {
	if _, err := s.Store.R.GetPulseAgent(ctx, req.Id); err != nil {
		return nil, pulseError(pulse.ErrNotFound)
	}
	g, err := settings.Get[settings.General](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	url := "https://" + g.AdminDomain
	return gen.GetPulseInstall200JSONResponse{
		Linux: "curl -fsSL " + url + "/api/pulse/install.sh | sudo sh -s -- --url " + url + " --id " + req.Id + " --token <token>",
		Compose: "services:\n  pulse:\n    image: git.juzo.io/juzo/hashi-pulse:latest\n    restart: unless-stopped\n    network_mode: host\n    environment:\n      HASHI_URL: " + url +
			"\n      HASHI_AGENT_ID: " + req.Id + "\n      HASHI_TOKEN: <token>\n",
	}, nil
}

func (s *Server) ApplyAgentDNS(ctx context.Context, _ gen.ApplyAgentDNSRequestObject) (gen.ApplyAgentDNSResponseObject, error) {
	if _, err := s.AdGuard.Reconcile(s.Sync)(ctx); err != nil {
		return nil, invalid(err.Error())
	}
	return gen.ApplyAgentDNS204Response{}, nil
}

func (s *Server) PutFirewallHostSSHTarget(ctx context.Context, req gen.PutFirewallHostSSHTargetRequestObject) (gen.PutFirewallHostSSHTargetResponseObject, error) {
	ri := info(ctx)
	if err := s.Firewall.SetSSHTarget(ctx, req.Id, req.Body.SetId, req.Body.SaveInactive != nil && *req.Body.SaveInactive); err != nil {
		return nil, sshTargetError(err)
	}
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: "firewall.host.ssh_target", Target: req.Id, IP: ri.ipString(), Detail: map[string]any{"set": req.Body.SetId}}); err != nil {
		return nil, err
	}
	return gen.PutFirewallHostSSHTarget204Response{}, nil
}

func (s *Server) PutTraefikSSHTarget(ctx context.Context, req gen.PutTraefikSSHTargetRequestObject) (gen.PutTraefikSSHTargetResponseObject, error) {
	ri := info(ctx)
	if err := s.Traefik.SetSSHTarget(ctx, req.Id, req.Body.SetId, req.Body.SaveInactive != nil && *req.Body.SaveInactive); err != nil {
		return nil, sshTargetError(err)
	}
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: "traefik.connection.ssh_target", Target: req.Id, IP: ri.ipString(), Detail: map[string]any{"set": req.Body.SetId}}); err != nil {
		return nil, err
	}
	return gen.PutTraefikSSHTarget204Response{}, nil
}

func sshTargetError(err error) error {
	if errors.Is(err, sshx.ErrTargetDown) {
		return conflict(err.Error())
	}
	return invalid(err.Error())
}

// GetIPSetSources says where a set's addresses come from, and for an agent how it is doing (08 §5).
func (s *Server) GetIPSetSources(ctx context.Context, req gen.GetIPSetSourcesRequestObject) (gen.GetIPSetSourcesResponseObject, error) {
	src, err := s.IPSets.Sourced(ctx, req.Id)
	if err != nil {
		return nil, invalid(err.Error())
	}
	raw := make([]map[string]any, 0, len(src))
	for _, x := range src {
		addrs := make([]string, 0, len(x.Addresses))
		for _, a := range x.Addresses {
			addrs = append(addrs, a.String())
		}
		m := map[string]any{"source": x.Source, "addresses": addrs}
		if x.AgentID != "" {
			if r, err := s.Store.R.GetPulseAgent(ctx, x.AgentID); err == nil {
				b := s.pulseBody(ctx, r)
				agent := map[string]any{"status": b.Status, "public_ip": b.PublicIp, "selected_ip": b.SelectedIp, "candidates": b.Candidates}
				if b.LastSeenAt != nil {
					agent["last_seen_at"] = b.LastSeenAt
				}
				if b.Reachable != nil {
					agent["reachable"] = *b.Reachable
				}
				m["agent"] = agent
				m["source"] = "Pulse agent " + r.Name
			}
		}
		raw = append(raw, m)
	}
	b, err := json.Marshal(map[string]any{"sources": raw})
	if err != nil {
		return nil, err
	}
	var out gen.GetIPSetSources200JSONResponse
	return out, json.Unmarshal(b, &out)
}
