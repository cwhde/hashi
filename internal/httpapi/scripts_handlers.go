package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/scripts"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store/db"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
)

func scriptError(err error) error {
	switch {
	case errors.Is(err, scripts.ErrInvalid):
		return invalid(strings.TrimPrefix(err.Error(), scripts.ErrInvalid.Error()+": "))
	case errors.Is(err, scripts.ErrNotFound):
		return notFound("No such script.")
	case errors.Is(err, scripts.ErrNotReady):
		return conflict(strings.TrimPrefix(err.Error(), scripts.ErrNotReady.Error()+": "))
	}
	return syncError(err)
}

func scriptInput(b gen.ScriptInput) scripts.Input {
	in := scripts.Input{Name: strings.TrimSpace(b.Name), Description: str(b.Description), Body: b.Body, Enabled: b.Enabled,
		Schedule: b.Schedule, TimeoutSeconds: b.TimeoutSeconds, AllHosts: b.AllHosts, Hosts: b.Hosts}
	for _, e := range b.Env {
		in.Env = append(in.Env, scripts.EnvVar{Name: e.Name, Value: secrets.New(str(e.Value)), Secret: e.Secret})
	}
	return in
}

// openScriptPlans are the open scripts plans by host id.
func (s *Server) openScriptPlans(ctx context.Context) (map[string]*hsync.Plan, error) {
	open, err := s.Sync.Open(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]*hsync.Plan{}
	for _, p := range open {
		if p.Subsystem == scripts.Subsystem {
			out[p.Target] = p
		}
	}
	return out, nil
}

func (s *Server) scriptBody(ctx context.Context, sc scripts.Script, open map[string]*hsync.Plan) (gen.Script, error) {
	out := gen.Script{Id: sc.ID, Slug: sc.Slug, Name: sc.Name, Description: sc.Description, Body: sc.Body, Enabled: sc.Enabled == 1,
		Schedule: sc.Schedule, TimeoutSeconds: int(sc.TimeoutSeconds), AllHosts: sc.AllHosts == 1, Hosts: sc.Hosts,
		Env: []gen.ScriptEnvVar{}, Targets: []gen.ScriptHost{}, LastRuns: []gen.ScriptRunStatus{},
		CreatedAt: time.UnixMilli(sc.CreatedAt), UpdatedAt: time.UnixMilli(sc.UpdatedAt)}
	if out.Hosts == nil {
		out.Hosts = []string{}
	}
	for _, e := range sc.Env {
		inVault := e.SecretID.Valid
		v := gen.ScriptEnvVar{Name: e.Name, Secret: inVault}
		if !inVault {
			v.Value = ptr(e.Value)
		}
		out.Env = append(out.Env, v)
	}
	targets, err := s.Scripts.Targets(ctx, sc)
	if err != nil {
		return out, err
	}
	for _, id := range targets {
		h, err := s.Store.R.GetFirewallHost(ctx, id)
		if err != nil {
			return out, err
		}
		_, pending := open[id]
		out.Targets = append(out.Targets, gen.ScriptHost{Id: id, Name: h.Name, Pending: pending})
	}
	st, err := s.Scripts.LastStatuses(ctx, sc.ID)
	if err != nil {
		return out, err
	}
	for _, x := range st {
		out.LastRuns = append(out.LastRuns, gen.ScriptRunStatus(x))
	}
	return out, nil
}

func (s *Server) ListScripts(ctx context.Context, _ gen.ListScriptsRequestObject) (gen.ListScriptsResponseObject, error) {
	all, err := s.Scripts.List(ctx)
	if err != nil {
		return nil, err
	}
	open, err := s.openScriptPlans(ctx)
	if err != nil {
		return nil, err
	}
	out := gen.ScriptList{Scripts: []gen.Script{}}
	for _, sc := range all {
		b, err := s.scriptBody(ctx, sc, open)
		if err != nil {
			return nil, err
		}
		out.Scripts = append(out.Scripts, b)
	}
	return gen.ListScripts200JSONResponse(out), nil
}

func (s *Server) oneScript(ctx context.Context, sc scripts.Script) (gen.Script, error) {
	open, err := s.openScriptPlans(ctx)
	if err != nil {
		return gen.Script{}, err
	}
	return s.scriptBody(ctx, sc, open)
}

func (s *Server) CreateScript(ctx context.Context, req gen.CreateScriptRequestObject) (gen.CreateScriptResponseObject, error) {
	ri := info(ctx)
	sc, err := s.Scripts.Create(ctx, s.actor(ri), ri.ipString(), scriptInput(*req.Body))
	if err != nil {
		return nil, scriptError(err)
	}
	b, err := s.oneScript(ctx, sc)
	if err != nil {
		return nil, err
	}
	return gen.CreateScript201JSONResponse(b), nil
}

func (s *Server) GetScript(ctx context.Context, req gen.GetScriptRequestObject) (gen.GetScriptResponseObject, error) {
	sc, err := s.Scripts.Get(ctx, req.Id)
	if err != nil {
		return nil, scriptError(err)
	}
	b, err := s.oneScript(ctx, sc)
	if err != nil {
		return nil, err
	}
	return gen.GetScript200JSONResponse(b), nil
}

func (s *Server) UpdateScript(ctx context.Context, req gen.UpdateScriptRequestObject) (gen.UpdateScriptResponseObject, error) {
	ri := info(ctx)
	sc, err := s.Scripts.Update(ctx, s.actor(ri), ri.ipString(), req.Id, scriptInput(*req.Body))
	if err != nil {
		return nil, scriptError(err)
	}
	b, err := s.oneScript(ctx, sc)
	if err != nil {
		return nil, err
	}
	return gen.UpdateScript200JSONResponse(b), nil
}

func (s *Server) DeleteScript(ctx context.Context, req gen.DeleteScriptRequestObject) (gen.DeleteScriptResponseObject, error) {
	ri := info(ctx)
	if err := s.Scripts.Delete(ctx, s.actor(ri), ri.ipString(), req.Id); err != nil {
		return nil, scriptError(err)
	}
	return gen.DeleteScript204Response{}, nil
}

// PlanScript plans each host the script runs on. A scripts plan covers every script on its host.
func (s *Server) PlanScript(ctx context.Context, req gen.PlanScriptRequestObject) (gen.PlanScriptResponseObject, error) {
	sc, err := s.Scripts.Get(ctx, req.Id)
	if err != nil {
		return nil, scriptError(err)
	}
	targets, err := s.Scripts.Targets(ctx, sc)
	if err != nil {
		return nil, err
	}
	ri := info(ctx)
	out := gen.ScriptPlans{Plans: []gen.SyncPlan{}}
	var errs []error
	for _, h := range targets {
		p, err := s.Sync.Propose(ctx, scripts.Subsystem, h, hsync.TriggerManual, s.actor(ri))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if p != nil {
			out.Plans = append(out.Plans, s.planBody(ctx, p))
		}
	}
	if len(out.Plans) == 0 && len(errs) > 0 {
		return nil, scriptError(errors.Join(errs...))
	}
	return gen.PlanScript200JSONResponse(out), nil
}

// ApplyScript approves and applies the open scripts plans of the script's hosts (the passkey was just confirmed).
func (s *Server) ApplyScript(ctx context.Context, req gen.ApplyScriptRequestObject) (gen.ApplyScriptResponseObject, error) {
	sc, err := s.Scripts.Get(ctx, req.Id)
	if err != nil {
		return nil, scriptError(err)
	}
	targets, err := s.Scripts.Targets(ctx, sc)
	if err != nil {
		return nil, err
	}
	open, err := s.openScriptPlans(ctx)
	if err != nil {
		return nil, err
	}
	ri := info(ctx)
	out := gen.ScriptPlans{Plans: []gen.SyncPlan{}}
	for _, h := range targets {
		p, ok := open[h]
		if !ok {
			continue
		}
		if p.Status == hsync.StatusPending {
			if err := s.Sync.Approve(ctx, p.ID, s.actor(ri), ri.ipString()); err != nil {
				return nil, syncError(err)
			}
		}
		done, err := s.Sync.Apply(ctx, p.ID, s.actor(ri), ri.ipString())
		if err != nil && !errors.Is(err, hsync.ErrReplanned) {
			if done, err = s.Sync.Get(ctx, p.ID); err != nil {
				return nil, syncError(err)
			}
		}
		if done != nil {
			out.Plans = append(out.Plans, s.planBody(ctx, done))
		}
	}
	return gen.ApplyScript200JSONResponse(out), nil
}

func (s *Server) RunScript(ctx context.Context, req gen.RunScriptRequestObject) (gen.RunScriptResponseObject, error) {
	ri := info(ctx)
	id, err := s.Scripts.Start(ctx, s.actor(ri), ri.ipString(), req.Id, req.Body.HostId)
	if err != nil {
		return nil, scriptError(err)
	}
	return gen.RunScript202JSONResponse{RunId: id}, nil
}

func (s *Server) hostName(ctx context.Context, id string, cache map[string]string) string {
	if n, ok := cache[id]; ok {
		return n
	}
	n := id
	if h, err := s.Store.R.GetFirewallHost(ctx, id); err == nil {
		n = h.Name
	}
	cache[id] = n
	return n
}

func scriptRunBody(r db.ScriptRun, host string) gen.ScriptRun {
	out := gen.ScriptRun{Id: r.ID, ScriptId: r.ScriptID, HostId: r.HostID, HostName: host, Trigger: gen.ScriptRunTrigger(r.Trigger),
		Status: gen.ScriptRunStatus(r.Status), StartedAt: time.UnixMilli(r.StartedAt), Output: r.Output, Truncated: r.Truncated == 1,
		Error: r.Error, Actor: r.Actor, FinishedAt: msTime(r.FinishedAt)}
	if r.ExitCode.Valid {
		out.ExitCode = ptr(int(r.ExitCode.Int64))
	}
	return out
}

func (s *Server) ListScriptRuns(ctx context.Context, req gen.ListScriptRunsRequestObject) (gen.ListScriptRunsResponseObject, error) {
	rows, err := s.Scripts.Runs(ctx, req.Id)
	if err != nil {
		return nil, scriptError(err)
	}
	names := map[string]string{}
	out := gen.ScriptRunList{Runs: make([]gen.ScriptRun, 0, len(rows))}
	for _, r := range rows {
		out.Runs = append(out.Runs, scriptRunBody(db.ScriptRun{ID: r.ID, ScriptID: r.ScriptID, HostID: r.HostID, Trigger: r.Trigger,
			Status: r.Status, ExitCode: r.ExitCode, StartedAt: r.StartedAt, FinishedAt: r.FinishedAt, Truncated: r.Truncated,
			Error: r.Error, Actor: r.Actor}, s.hostName(ctx, r.HostID, names)))
	}
	return gen.ListScriptRuns200JSONResponse(out), nil
}

func (s *Server) GetScriptRun(ctx context.Context, req gen.GetScriptRunRequestObject) (gen.GetScriptRunResponseObject, error) {
	r, err := s.Scripts.GetRun(ctx, req.RunId)
	if err != nil {
		if errors.Is(err, scripts.ErrNotFound) {
			return nil, notFound("No such run.")
		}
		return nil, err
	}
	return gen.GetScriptRun200JSONResponse(scriptRunBody(r, s.hostName(ctx, r.HostID, map[string]string{}))), nil
}

func (s *Server) PreviewScriptSchedule(_ context.Context, req gen.PreviewScriptScheduleRequestObject) (gen.PreviewScriptScheduleResponseObject, error) {
	sc, err := scripts.ParseCron(req.Params.Schedule)
	if err != nil {
		return nil, invalid(err.Error())
	}
	return gen.PreviewScriptSchedule200JSONResponse{Next: sc.Next(time.Now().UTC(), 5), TimerLines: sc.OnCalendar()}, nil
}

// scriptItem fills a scripts plan item: the script's text before and after for a line diff, and one line with the
// schedule, timeout and variable names.
func scriptItem(it hsync.Item, out *gen.SyncPlanItem) {
	type state struct {
		Body     string   `json:"body"`
		Schedule []string `json:"schedule"`
		Timeout  int      `json:"timeout_seconds"`
		Env      []string `json:"env"`
		Hash     string   `json:"hash"`
	}
	text := func(raw json.RawMessage, content **string) *string {
		var v state
		if len(raw) == 0 || json.Unmarshal(raw, &v) != nil {
			return nil
		}
		if v.Body != "" {
			*content = ptr(v.Body)
		}
		var parts []string
		if v.Schedule != nil {
			parts = append(parts, "schedule "+strings.Join(v.Schedule, " or "))
		}
		if v.Timeout > 0 {
			parts = append(parts, fmt.Sprintf("timeout %d s", v.Timeout))
		}
		if len(v.Env) > 0 {
			parts = append(parts, "variables "+strings.Join(slices.Clone(v.Env), ", "))
		}
		if v.Hash != "" && len(v.Hash) > 12 {
			parts = append(parts, "sha256 "+v.Hash[:12]+"…")
		}
		t := strings.Join(parts, " · ")
		return &t
	}
	out.BeforeText = text(it.Before, &out.BeforeContent)
	out.AfterText = text(it.After, &out.AfterContent)
}
