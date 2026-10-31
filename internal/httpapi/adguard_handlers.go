package httpapi

import (
	"context"
	"encoding/json"
	"errors"

	"git.juzo.io/juzo/hashi/internal/adguard"
	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

func adguardError(err error) error {
	switch {
	case errors.Is(err, adguard.ErrNotFound):
		return notFound(err.Error())
	case errors.Is(err, adguard.ErrBadInput):
		return invalid(err.Error())
	}
	return err
}

func adguardBody(r db.AdguardConnection) gen.AdGuardConnection {
	return gen.AdGuardConnection{Id: r.ID, Name: r.Name, TargetSetId: r.TargetSetID, Port: int(r.Port), Scheme: r.Scheme, PathPrefix: r.PathPrefix, TlsVerify: r.TlsVerify == 1,
		ServerName: r.ServerName, Username: r.Username, HasPassword: r.SecretID.Valid, Mode: gen.AdGuardConnectionMode(r.Mode), LastCheckedAt: millis(r.LastCheckedAt), LastVersion: r.LastVersion, LastError: r.LastError}
}

func adguardInput(b gen.AdGuardConnectionInput, mode string) adguard.Input {
	str := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	if b.Mode != nil {
		mode = string(*b.Mode)
	}
	return adguard.Input{Name: b.Name, TargetSetID: b.TargetSetId, Port: b.Port, Scheme: b.Scheme, PathPrefix: str(b.PathPrefix), TLSVerify: b.TlsVerify,
		ServerName: str(b.ServerName), Username: str(b.Username), Mode: mode, Password: secrets.New(str(b.Password))}
}

func (s *Server) ListAdGuardConnections(ctx context.Context, _ gen.ListAdGuardConnectionsRequestObject) (gen.ListAdGuardConnectionsResponseObject, error) {
	rows, err := s.Store.R.ListAdGuardConnections(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]gen.AdGuardConnection, 0, len(rows))
	for _, r := range rows {
		out = append(out, adguardBody(r))
	}
	return gen.ListAdGuardConnections200JSONResponse{Connections: out}, nil
}

func (s *Server) CreateAdGuardConnection(ctx context.Context, req gen.CreateAdGuardConnectionRequestObject) (gen.CreateAdGuardConnectionResponseObject, error) {
	ri := info(ctx)
	row, err := s.AdGuard.Create(ctx, adguardInput(*req.Body, ""))
	if err != nil {
		return nil, adguardError(err)
	}
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: "adguard.connection.created", Target: row.ID, IP: ri.ipString()}); err != nil {
		return nil, err
	}
	return gen.CreateAdGuardConnection201JSONResponse(adguardBody(row)), nil
}

func (s *Server) UpdateAdGuardConnection(ctx context.Context, req gen.UpdateAdGuardConnectionRequestObject) (gen.UpdateAdGuardConnectionResponseObject, error) {
	ri := info(ctx)
	mode := adguard.ModeReadOnly
	if cur, err := s.Store.R.GetAdGuardConnection(ctx, req.Id); err == nil {
		mode = cur.Mode
	}
	row, err := s.AdGuard.Update(ctx, req.Id, adguardInput(*req.Body, mode))
	if err != nil {
		return nil, adguardError(err)
	}
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: "adguard.connection.updated", Target: row.ID, IP: ri.ipString(), Detail: map[string]any{"mode": row.Mode}}); err != nil {
		return nil, err
	}
	return gen.UpdateAdGuardConnection200JSONResponse(adguardBody(row)), nil
}

func (s *Server) DeleteAdGuardConnection(ctx context.Context, req gen.DeleteAdGuardConnectionRequestObject) (gen.DeleteAdGuardConnectionResponseObject, error) {
	ri := info(ctx)
	if err := s.AdGuard.Delete(ctx, req.Id); err != nil {
		return nil, adguardError(err)
	}
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: "adguard.connection.deleted", Target: req.Id, IP: ri.ipString()}); err != nil {
		return nil, err
	}
	return gen.DeleteAdGuardConnection204Response{}, nil
}

func (s *Server) TestAdGuardConnection(ctx context.Context, req gen.TestAdGuardConnectionRequestObject) (gen.TestAdGuardConnectionResponseObject, error) {
	row, err := s.AdGuard.Test(ctx, req.Id)
	if err != nil {
		return nil, adguardError(err)
	}
	return gen.TestAdGuardConnection200JSONResponse(adguardBody(row)), nil
}

func (s *Server) GetAdGuardRewrites(ctx context.Context, req gen.GetAdGuardRewritesRequestObject) (gen.GetAdGuardRewritesResponseObject, error) {
	v, err := s.AdGuard.Rewrites(ctx, req.Id)
	if err != nil {
		if errors.Is(err, adguard.ErrNotFound) {
			return nil, adguardError(err)
		}
		return nil, invalid(err.Error()) // the instance could not be read; its error text holds no secret
	}
	out := gen.GetAdGuardRewrites200JSONResponse{}
	out.Rewrites = make([]struct {
		Answer  string `json:"answer"`
		Domain  string `json:"domain"`
		Enabled bool   `json:"enabled"`
		Owned   bool   `json:"owned"`
	}, 0, len(v.Rewrites))
	for _, r := range v.Rewrites {
		out.Rewrites = append(out.Rewrites, struct {
			Answer  string `json:"answer"`
			Domain  string `json:"domain"`
			Enabled bool   `json:"enabled"`
			Owned   bool   `json:"owned"`
		}{r.Answer, r.Domain, r.IsEnabled(), r.Owned})
	}
	out.Planned = make([]struct {
		Action string  `json:"action"`
		Answer *string `json:"answer,omitempty"`
		Domain string  `json:"domain"`
		Note   *string `json:"note,omitempty"`
		Risk   string  `json:"risk"`
	}, 0, len(v.Planned))
	for _, it := range v.Planned {
		raw := it.After
		if len(raw) == 0 {
			raw = it.Before
		}
		var val struct {
			Answer string `json:"answer"`
		}
		_ = json.Unmarshal(raw, &val)
		out.Planned = append(out.Planned, struct {
			Action string  `json:"action"`
			Answer *string `json:"answer,omitempty"`
			Domain string  `json:"domain"`
			Note   *string `json:"note,omitempty"`
			Risk   string  `json:"risk"`
		}{it.Action, emptyNil(val.Answer), it.Object, emptyNil(it.Note), string(it.Risk)})
	}
	return out, nil
}
