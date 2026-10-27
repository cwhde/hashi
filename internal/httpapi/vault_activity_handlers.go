package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Secrets whose purpose starts with this prefix belong to the vault itself (key check) and are never listed or revealed.
const internalSecretPrefix = "vault."

func (s *Server) GetVault(ctx context.Context, _ gen.GetVaultRequestObject) (gen.GetVaultResponseObject, error) {
	metas, err := s.Store.R.ListSecretMeta(ctx)
	if err != nil {
		return nil, err
	}
	n := 0
	for _, m := range metas {
		if !strings.HasPrefix(m.Purpose, internalSecretPrefix) {
			n++
		}
	}
	rk, err := s.Store.R.GetRecoveryKey(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return gen.GetVault200JSONResponse{
		Mode: gen.VaultStateModeService, Locked: false, KeyVersion: int(s.Vault.KeyVersion()), Secrets: n,
		RecoveryKeyConfirmed: err == nil && rk.ConfirmedAt.Valid,
	}, nil
}

func (s *Server) ListSecrets(ctx context.Context, _ gen.ListSecretsRequestObject) (gen.ListSecretsResponseObject, error) {
	metas, err := s.Store.R.ListSecretMeta(ctx)
	if err != nil {
		return nil, err
	}
	out := gen.SecretList{Secrets: []gen.SecretMeta{}}
	for _, m := range metas {
		if strings.HasPrefix(m.Purpose, internalSecretPrefix) {
			continue
		}
		out.Secrets = append(out.Secrets, gen.SecretMeta{Id: m.ID, Class: gen.SecretMetaClass(m.Class), Purpose: m.Purpose,
			CreatedAt: time.UnixMilli(m.CreatedAt).UTC(), UpdatedAt: time.UnixMilli(m.UpdatedAt).UTC()})
	}
	return gen.ListSecrets200JSONResponse(out), nil
}

func (s *Server) RevealSecret(ctx context.Context, req gen.RevealSecretRequestObject) (gen.RevealSecretResponseObject, error) {
	ri := info(ctx)
	rec, err := s.Store.R.GetSecret(ctx, req.Id)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && strings.HasPrefix(rec.Purpose, internalSecretPrefix)) {
		return nil, notFound("No secret with that ID.")
	}
	if err != nil {
		return nil, err
	}
	v, err := s.Vault.Open(rec)
	if err != nil {
		return nil, err
	}
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: "vault.secret.revealed", Target: "secret:" + rec.ID,
		IP: ri.ipString(), Detail: map[string]any{"purpose": rec.Purpose}}); err != nil {
		return nil, err // never reveal without the audit entry
	}
	return gen.RevealSecret200JSONResponse{Id: rec.ID, Value: v.Reveal()}, nil
}

func (s *Server) ListAuditEvents(ctx context.Context, req gen.ListAuditEventsRequestObject) (gen.ListAuditEventsResponseObject, error) {
	limit := 100
	if req.Params.Limit != nil {
		limit = min(max(*req.Params.Limit, 1), 500)
	}
	cursor := ""
	if req.Params.Cursor != nil {
		cursor = *req.Params.Cursor
	}
	rows, err := s.Store.R.ListAuditEvents(ctx, db.ListAuditEventsParams{Cursor: cursor, Lim: int64(limit)})
	if err != nil {
		return nil, err
	}
	out := gen.AuditPage{Events: make([]gen.AuditEvent, len(rows))}
	for i, r := range rows {
		detail := map[string]any{}
		_ = json.Unmarshal([]byte(r.DetailJson), &detail)
		out.Events[i] = gen.AuditEvent{Id: r.ID, At: time.UnixMilli(r.At).UTC(), Actor: r.Actor, Action: r.Action, Target: r.Target,
			Outcome: gen.AuditEventOutcome(r.Outcome), Ip: r.Ip, Detail: detail}
	}
	if len(rows) == limit {
		out.NextCursor = ptr(rows[len(rows)-1].ID)
	}
	return gen.ListAuditEvents200JSONResponse(out), nil
}

func (s *Server) ListJobs(ctx context.Context, _ gen.ListJobsRequestObject) (gen.ListJobsResponseObject, error) {
	rows, err := s.Store.R.ListJobsWithLastRun(ctx)
	if err != nil {
		return nil, err
	}
	out := gen.JobList{Jobs: make([]gen.Job, len(rows))}
	for i, r := range rows {
		j := gen.Job{Kind: r.Kind, IntervalSeconds: r.IntervalSeconds, Enabled: r.Enabled == 1, NextRunAt: time.UnixMilli(r.NextRunAt).UTC()}
		if r.RunID.Valid {
			run := gen.JobRun{Id: r.RunID.String, StartedAt: time.UnixMilli(r.StartedAt.Int64).UTC(), Status: gen.JobRunStatus(r.Status.String),
				Summary: r.Summary.String, Error: r.Error.String}
			if r.FinishedAt.Valid {
				run.FinishedAt = ptr(time.UnixMilli(r.FinishedAt.Int64).UTC())
			}
			if r.DurationMs.Valid {
				run.DurationMs = ptr(r.DurationMs.Int64)
			}
			j.LastRun = &run
		}
		out.Jobs[i] = j
	}
	return gen.ListJobs200JSONResponse(out), nil
}

func (s *Server) ListLogLines(_ context.Context, req gen.ListLogLinesRequestObject) (gen.ListLogLinesResponseObject, error) {
	var after int64
	if req.Params.After != nil {
		after = *req.Params.After
	}
	lines, last := s.Logs.After(after)
	out := gen.LogPage{Lines: make([]gen.LogLine, 0, len(lines)), LastSeq: last}
	for _, l := range lines {
		m := map[string]any{}
		if json.Unmarshal(l.JSON, &m) == nil {
			out.Lines = append(out.Lines, gen.LogLine{Seq: l.Seq, Line: m})
		}
	}
	return gen.ListLogLines200JSONResponse(out), nil
}
