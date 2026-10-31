package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/pulse"
	"git.juzo.io/juzo/hashi/internal/settings"
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

func pulseBody(r db.PulseAgent) gen.PulseAgent {
	cands := []string{}
	for _, c := range pulse.Candidates(r) {
		cands = append(cands, c.Address)
	}
	return gen.PulseAgent{Id: r.ID, Name: r.Name, Status: gen.PulseAgentStatus(pulse.Status(r, time.Now())), InstallType: r.InstallType, PublicIp: r.PublicIp, SelectedIp: r.SelectedIp,
		Candidates: cands, Hostname: r.Hostname, Version: r.Version, IntervalSeconds: int(r.IntervalSeconds), LastSeenAt: millis(r.LastSeenAt), DnsEnabled: r.DnsEnabled == 1, DnsName: r.DnsName}
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
	if err := s.Pulse.Heartbeat(ctx, req.AgentId, token, b, info(ctx).ip); err != nil {
		return nil, pulseError(err)
	}
	return gen.PulseHeartbeat204Response{}, nil
}

func (s *Server) ListPulseAgents(ctx context.Context, _ gen.ListPulseAgentsRequestObject) (gen.ListPulseAgentsResponseObject, error) {
	rows, err := s.Store.R.ListPulseAgents(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]gen.PulseAgent, 0, len(rows))
	for _, r := range rows {
		out = append(out, pulseBody(r))
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
	return gen.CreatePulseAgent201JSONResponse{Agent: pulseBody(row), Token: token.Reveal()}, nil
}

func (s *Server) UpdatePulseAgent(ctx context.Context, req gen.UpdatePulseAgentRequestObject) (gen.UpdatePulseAgentResponseObject, error) {
	name := ""
	if req.Body.DnsName != nil {
		name = *req.Body.DnsName
	}
	row, err := s.Pulse.SetDNS(ctx, req.Id, req.Body.Name, req.Body.DnsEnabled, name)
	if err != nil {
		return nil, pulseError(err)
	}
	return gen.UpdatePulseAgent200JSONResponse(pulseBody(row)), nil
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
	return gen.RotatePulseToken200JSONResponse{Agent: pulseBody(row), Token: token.Reveal()}, nil
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
	return gen.RevokePulseAgent200JSONResponse(pulseBody(row)), nil
}

func (s *Server) GetAgentDNSSettings(ctx context.Context, _ gen.GetAgentDNSSettingsRequestObject) (gen.GetAgentDNSSettingsResponseObject, error) {
	v, err := settings.Get[settings.AgentDNS](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	return gen.GetAgentDNSSettings200JSONResponse{Enabled: v.Enabled, Domain: v.Domain, IpMode: gen.AgentDNSSettingsIpMode(v.IPMode)}, nil
}

func (s *Server) PutAgentDNSSettings(ctx context.Context, req gen.PutAgentDNSSettingsRequestObject) (gen.PutAgentDNSSettingsResponseObject, error) {
	v := settings.AgentDNS{Enabled: req.Body.Enabled, Domain: req.Body.Domain, IPMode: string(req.Body.IpMode)}
	if err := saveSetting(ctx, s, v, nil); err != nil {
		return nil, err
	}
	return gen.PutAgentDNSSettings200JSONResponse{Enabled: v.Enabled, Domain: v.Domain, IpMode: gen.AgentDNSSettingsIpMode(v.IPMode)}, nil
}
