package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/auth"
	"git.juzo.io/juzo/hashi/internal/clientip"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/setup"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

func ptr[T any](v T) *T { return &v }

func (s *Server) sessionBody(ctx context.Context, sess *auth.Session) (gen.AuthSession, error) {
	done, err := s.Setup.Completed(ctx)
	if err != nil {
		return gen.AuthSession{}, err
	}
	out := gen.AuthSession{SetupCompleted: done}
	if sess == nil {
		return out, nil
	}
	scopes := make([]string, len(sess.Scopes))
	for i, sc := range sess.Scopes {
		scopes[i] = string(sc)
	}
	out.Authenticated = true
	out.Id = ptr(sess.Correlation)
	out.Method = ptr(gen.AuthSessionMethod(sess.Method))
	out.Scopes = &scopes
	out.BoundIp = ptr(sess.BoundIP.String())
	out.CreatedAt = ptr(sess.CreatedAt)
	out.IdleExpiresAt = ptr(sess.IdleExpiresAt)
	out.AbsoluteExpiresAt = ptr(sess.AbsoluteAt)
	out.CsrfToken = ptr(s.Sessions.CSRFToken(sess))
	if sess.ReauthValid(s.now()) {
		out.ReauthValidUntil = ptr(sess.ReauthAt.Add(auth.ReauthValidity))
	}
	return out, nil
}

func (s *Server) GetAuthSession(ctx context.Context, _ gen.GetAuthSessionRequestObject) (gen.GetAuthSessionResponseObject, error) {
	body, err := s.sessionBody(ctx, info(ctx).session)
	if err != nil {
		return nil, err
	}
	return gen.GetAuthSession200JSONResponse(body), nil
}

func (s *Server) BootstrapLogin(ctx context.Context, req gen.BootstrapLoginRequestObject) (gen.BootstrapLoginResponseObject, error) {
	ri := info(ctx)
	done, err := s.Setup.Completed(ctx)
	if err != nil {
		return nil, err
	}
	if done {
		return nil, forbidden("Setup is complete; sign in with your passkey.")
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
		s.recordAnon(ctx, ri, "auth.bootstrap.login", audit.Denied, map[string]any{"reason": "network not allowed"})
		return nil, forbidden("Bootstrap sign-in works only from a private network.")
	}
	if err := auth.CheckBootstrap(ctx, s.Store.R, req.Body.Username, req.Body.Password); err != nil {
		if errors.Is(err, auth.ErrBadBootstrapLogin) {
			s.recordAnon(ctx, ri, "auth.bootstrap.login", audit.Failure, nil)
			return nil, &Problem{Type: "/problems/bad-credentials", Title: "Wrong username or password", Status: http.StatusUnauthorized,
				Detail: "Use the bootstrap credentials from the container log, or run `hashi bootstrap reset`."}
		}
		return nil, err
	}
	var token string
	var sess *auth.Session
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		token, sess, err = s.Sessions.Issue(ctx, q, auth.MethodBootstrap, "", ri.ip, auth.BootstrapScopes, ri.ua)
		if err != nil {
			return err
		}
		if err := s.Setup.MarkDone(ctx, q, setup.StepBootstrap, nil); err != nil {
			return err
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.Audit.SessionActor(sess.IDHash), Action: "auth.bootstrap.login", IP: ri.ipString()})
	})
	if err != nil {
		return nil, err
	}
	body, err := s.sessionBody(ctx, sess)
	if err != nil {
		return nil, err
	}
	return gen.BootstrapLogin200JSONResponse{Body: body, Headers: gen.BootstrapLogin200ResponseHeaders{SetCookie: ptr(issueCookie(ri, token, sess))}}, nil
}

func (s *Server) recordAnon(ctx context.Context, ri *request, action string, outcome audit.Outcome, detail map[string]any) {
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Action: action, Outcome: outcome, IP: ri.ipString(), Detail: detail}); err != nil {
		s.Log.Warn("audit write failed", "err", err)
	}
}

func (s *Server) Logout(ctx context.Context, _ gen.LogoutRequestObject) (gen.LogoutResponseObject, error) {
	ri := info(ctx)
	// Server-side revocation first; the cookie is cleared only after it succeeded (02 §3.4).
	err := s.Store.Tx(ctx, func(q *db.Queries) error {
		_, err := s.Sessions.Revoke(ctx, q, ri.session.IDHash, auth.ReasonLogout, s.actor(ri), ri.ip)
		return err
	})
	if err != nil {
		return nil, err
	}
	name := cookieSession
	if ri.session.Method == auth.MethodBootstrap && !ri.tls {
		name = cookieBootstrap
	}
	return gen.Logout204Response{Headers: gen.Logout204ResponseHeaders{SetCookie: ptr(newCookie(name, "", -1).String())}}, nil
}

func (s *Server) ListSessions(ctx context.Context, _ gen.ListSessionsRequestObject) (gen.ListSessionsResponseObject, error) {
	cur := info(ctx).session
	list, err := s.Sessions.Active(ctx)
	if err != nil {
		return nil, err
	}
	out := gen.SessionList{Sessions: make([]gen.SessionInfo, len(list))}
	for i, x := range list {
		out.Sessions[i] = gen.SessionInfo{
			Id: x.Correlation, Method: string(x.Method), BoundIp: x.BoundIP.String(), CreatedAt: x.CreatedAt,
			LastSeenAt: x.LastSeenAt, AbsoluteExpiresAt: x.AbsoluteAt, Current: bytes.Equal(x.IDHash, cur.IDHash),
		}
	}
	return gen.ListSessions200JSONResponse(out), nil
}

func (s *Server) RevokeSession(ctx context.Context, req gen.RevokeSessionRequestObject) (gen.RevokeSessionResponseObject, error) {
	ri := info(ctx)
	list, err := s.Sessions.Active(ctx)
	if err != nil {
		return nil, err
	}
	for _, x := range list {
		if x.Correlation != req.Id {
			continue
		}
		err := s.Store.Tx(ctx, func(q *db.Queries) error {
			_, err := s.Sessions.Revoke(ctx, q, x.IDHash, auth.ReasonManual, s.actor(ri), ri.ip)
			return err
		})
		if err != nil {
			return nil, err
		}
		return gen.RevokeSession204Response{}, nil
	}
	return nil, notFound("No active session with that ID.")
}

func (s *Server) RevokeOtherSessions(ctx context.Context, _ gen.RevokeOtherSessionsRequestObject) (gen.RevokeOtherSessionsResponseObject, error) {
	ri := info(ctx)
	list, err := s.Sessions.Active(ctx)
	if err != nil {
		return nil, err
	}
	n := 0
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		for _, x := range list {
			if bytes.Equal(x.IDHash, ri.session.IDHash) {
				continue
			}
			ok, err := s.Sessions.Revoke(ctx, q, x.IDHash, auth.ReasonBulk, s.actor(ri), ri.ip)
			if err != nil {
				return err
			}
			if ok {
				n++
			}
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "session.revoke_others", IP: ri.ipString(), Detail: map[string]any{"count": n}})
	})
	if err != nil {
		return nil, err
	}
	return gen.RevokeOtherSessions200JSONResponse{Revoked: n}, nil
}

// ---- passkey ceremonies ----

func optionsMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	return m, json.Unmarshal(b, &m)
}

func ceremonyError(err error) error {
	switch {
	case errors.Is(err, auth.ErrCeremony):
		return &Problem{Type: "/problems/passkey-failed", Title: "Passkey check failed", Status: http.StatusBadRequest, Detail: auth.ErrCeremony.Error()}
	case errors.Is(err, auth.ErrPasskeyUnavailable):
		return conflict(err.Error())
	}
	return err
}

func credentialBytes(c map[string]any) ([]byte, error) {
	if len(c) == 0 {
		return nil, badRequest("credential is empty")
	}
	return json.Marshal(c)
}

func passkeyBody(p auth.Passkey) gen.Passkey {
	out := gen.Passkey{Id: p.ID, Name: p.Name, Label: p.Label, CreatedAt: p.CreatedAt}
	if !p.LastUsedAt.IsZero() {
		out.LastUsedAt = ptr(p.LastUsedAt)
	}
	return out
}

func (s *Server) ReauthOptions(ctx context.Context, _ gen.ReauthOptionsRequestObject) (gen.ReauthOptionsResponseObject, error) {
	id, opts, err := s.Passkeys.BeginReauth(ctx, info(ctx).session.IDHash)
	if err != nil {
		return nil, ceremonyError(err)
	}
	m, err := optionsMap(opts)
	if err != nil {
		return nil, err
	}
	return gen.ReauthOptions200JSONResponse{CeremonyId: id, Options: m}, nil
}

func (s *Server) Reauth(ctx context.Context, req gen.ReauthRequestObject) (gen.ReauthResponseObject, error) {
	ri := info(ctx)
	cred, err := credentialBytes(req.Body.Credential)
	if err != nil {
		return nil, err
	}
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		pk, err := s.Passkeys.FinishReauth(ctx, q, ri.session.IDHash, req.Body.CeremonyId, cred)
		if err != nil {
			return err
		}
		if err := s.Sessions.MarkReauth(ctx, q, ri.session); err != nil {
			return err
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "auth.reauth", Target: "passkey:" + pk, IP: ri.ipString()})
	})
	if err != nil {
		s.recordSessionFailure(ctx, ri, "auth.reauth")
		return nil, ceremonyError(err)
	}
	body, err := s.sessionBody(ctx, ri.session)
	if err != nil {
		return nil, err
	}
	return gen.Reauth200JSONResponse(body), nil
}

func (s *Server) recordSessionFailure(ctx context.Context, ri *request, action string) {
	if err := s.Audit.Record(ctx, s.Store.Q, audit.Event{Actor: s.actor(ri), Action: action, Outcome: audit.Failure, IP: ri.ipString()}); err != nil {
		s.Log.Warn("audit write failed", "err", err)
	}
}

func (s *Server) ListPasskeys(ctx context.Context, _ gen.ListPasskeysRequestObject) (gen.ListPasskeysResponseObject, error) {
	list, err := s.Passkeys.List(ctx)
	if err != nil {
		return nil, err
	}
	out := gen.PasskeyList{Passkeys: make([]gen.Passkey, len(list))}
	for i, p := range list {
		out.Passkeys[i] = passkeyBody(p)
	}
	return gen.ListPasskeys200JSONResponse(out), nil
}

func (s *Server) PasskeyRegisterOptions(ctx context.Context, _ gen.PasskeyRegisterOptionsRequestObject) (gen.PasskeyRegisterOptionsResponseObject, error) {
	id, opts, err := s.Passkeys.BeginRegistration(ctx, info(ctx).session.IDHash)
	if err != nil {
		return nil, ceremonyError(err)
	}
	m, err := optionsMap(opts)
	if err != nil {
		return nil, err
	}
	return gen.PasskeyRegisterOptions200JSONResponse{CeremonyId: id, Options: m}, nil
}

func (s *Server) PasskeyRegisterVerify(ctx context.Context, req gen.PasskeyRegisterVerifyRequestObject) (gen.PasskeyRegisterVerifyResponseObject, error) {
	pk, err := s.registerPasskey(ctx, req.Body)
	if err != nil {
		return nil, err
	}
	return gen.PasskeyRegisterVerify200JSONResponse(passkeyBody(pk)), nil
}

// registerPasskey finishes a registration ceremony for the current session (setup step 9 or Settings → Account).
func (s *Server) registerPasskey(ctx context.Context, body *gen.CeremonyResponse) (auth.Passkey, error) {
	ri := info(ctx)
	cred, err := credentialBytes(body.Credential)
	if err != nil {
		return auth.Passkey{}, err
	}
	name := ""
	if body.Name != nil {
		name = *body.Name
	}
	var pk auth.Passkey
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		pk, err = s.Passkeys.FinishRegistration(ctx, q, ri.session.IDHash, body.CeremonyId, name, cred)
		if err != nil {
			return err
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "auth.passkey.registered", Target: "passkey:" + pk.ID,
			IP: ri.ipString(), Detail: map[string]any{"name": pk.Name}})
	})
	if err != nil {
		s.recordSessionFailure(ctx, ri, "auth.passkey.registered")
		return auth.Passkey{}, ceremonyError(err)
	}
	return pk, nil
}

func (s *Server) PasskeyLoginOptions(ctx context.Context, _ gen.PasskeyLoginOptionsRequestObject) (gen.PasskeyLoginOptionsResponseObject, error) {
	if done, err := s.Setup.Completed(ctx); err != nil {
		return nil, err
	} else if !done {
		return nil, conflict("Setup is not complete yet; continue it with the bootstrap login.")
	}
	id, opts, err := s.Passkeys.BeginLogin(ctx)
	if err != nil {
		return nil, ceremonyError(err)
	}
	m, err := optionsMap(opts)
	if err != nil {
		return nil, err
	}
	return gen.PasskeyLoginOptions200JSONResponse{CeremonyId: id, Options: m}, nil
}

func (s *Server) PasskeyLoginVerify(ctx context.Context, req gen.PasskeyLoginVerifyRequestObject) (gen.PasskeyLoginVerifyResponseObject, error) {
	ri := info(ctx)
	if !ri.ip.IsValid() {
		return nil, badRequest("Hashi cannot determine your IP address; check the trusted proxy settings.")
	}
	cred, err := credentialBytes(req.Body.Credential)
	if err != nil {
		return nil, err
	}
	var token string
	var sess *auth.Session
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		pk, err := s.Passkeys.FinishLogin(ctx, q, req.Body.CeremonyId, cred)
		if err != nil {
			return err
		}
		token, sess, err = s.Sessions.Issue(ctx, q, auth.MethodPasskey, pk, ri.ip, auth.PasskeyScopes, ri.ua)
		if err != nil {
			return err
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.Audit.SessionActor(sess.IDHash), Action: "auth.passkey.login",
			Target: "passkey:" + pk, IP: ri.ipString()})
	})
	if err != nil {
		s.recordAnon(ctx, ri, "auth.passkey.login", audit.Failure, nil)
		return nil, ceremonyError(err)
	}
	body, err := s.sessionBody(ctx, sess)
	if err != nil {
		return nil, err
	}
	return gen.PasskeyLoginVerify200JSONResponse{Body: body, Headers: gen.PasskeyLoginVerify200ResponseHeaders{SetCookie: ptr(issueCookie(ri, token, sess))}}, nil
}

func (s *Server) RenamePasskey(ctx context.Context, req gen.RenamePasskeyRequestObject) (gen.RenamePasskeyResponseObject, error) {
	ri := info(ctx)
	err := s.Store.Tx(ctx, func(q *db.Queries) error {
		n, err := q.RenamePasskey(ctx, db.RenamePasskeyParams{Name: req.Body.Name, ID: req.Id})
		if err != nil {
			return err
		}
		if n == 0 {
			return notFound("No passkey with that ID.")
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "auth.passkey.renamed", Target: "passkey:" + req.Id,
			IP: ri.ipString(), Detail: map[string]any{"name": req.Body.Name}})
	})
	if err != nil {
		return nil, err
	}
	row, err := s.Store.R.GetPasskey(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	pk := auth.Passkey{ID: row.ID, Name: row.Name, Label: row.AaguidLabel, CreatedAt: time.UnixMilli(row.CreatedAt).UTC()}
	if row.LastUsedAt.Valid {
		pk.LastUsedAt = time.UnixMilli(row.LastUsedAt.Int64).UTC()
	}
	return gen.RenamePasskey200JSONResponse(passkeyBody(pk)), nil
}

func (s *Server) DeletePasskey(ctx context.Context, req gen.DeletePasskeyRequestObject) (gen.DeletePasskeyResponseObject, error) {
	ri := info(ctx)
	var revoked int64
	err := s.Store.Tx(ctx, func(q *db.Queries) error {
		if _, err := q.GetPasskey(ctx, req.Id); errors.Is(err, sql.ErrNoRows) {
			return notFound("No passkey with that ID.")
		} else if err != nil {
			return err
		}
		// Sessions signed in with this passkey end now (02 §2). Revoke before deleting: the FK is cleared on delete.
		var err error
		if revoked, err = s.Sessions.RevokeByCredential(ctx, q, req.Id, s.actor(ri), ri.ip); err != nil {
			return err
		}
		if err := auth.Delete(ctx, q, req.Id); err != nil {
			if errors.Is(err, auth.ErrLastPasskey) {
				return conflict(err.Error())
			}
			return err
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.actor(ri), Action: "auth.passkey.deleted", Target: "passkey:" + req.Id,
			IP: ri.ipString(), Detail: map[string]any{"sessions_revoked": revoked}})
	})
	if err != nil {
		return nil, err
	}
	return gen.DeletePasskey204Response{}, nil
}
