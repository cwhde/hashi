package httpapi

import (
	"context"
	"errors"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/auth"
	"git.juzo.io/juzo/hashi/internal/clientip"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Console recovery (02 §2): `hashi admin recover` prints a one-time link; it opens a session that can register one
// new passkey and nothing else.

func (s *Server) RecoverLogin(ctx context.Context, req gen.RecoverLoginRequestObject) (gen.RecoverLoginResponseObject, error) {
	ri := info(ctx)
	sec, err := settings.Get[settings.Security](ctx, s.Store.R)
	if err != nil {
		return nil, err
	}
	private, err := clientip.ParsePrefixes(sec.BootstrapAllowedCIDRs)
	if err != nil {
		return nil, err
	}
	if !ri.ip.IsValid() || !clientip.InAny(ri.ip, private) {
		s.recordAnon(ctx, ri, "auth.recover.login", audit.Denied, map[string]any{"reason": "network not allowed"})
		return nil, forbidden("Recovery links work only from a private network.")
	}
	var token string
	var sess *auth.Session
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		if err := auth.UseRecoveryLink(ctx, q, req.Body.Token, s.now()); err != nil {
			return err
		}
		token, sess, err = s.Sessions.Issue(ctx, q, auth.MethodRecovery, "", ri.ip, auth.RecoveryScopes, ri.ua)
		if err != nil {
			return err
		}
		return s.Audit.Record(ctx, q, audit.Event{Actor: s.Audit.SessionActor(sess.IDHash), Action: "auth.recover.login", IP: ri.ipString()})
	})
	if errors.Is(err, auth.ErrRecoveryLink) {
		s.recordAnon(ctx, ri, "auth.recover.login", audit.Failure, nil)
		return nil, forbidden(err.Error())
	}
	if err != nil {
		return nil, err
	}
	body, err := s.sessionBody(ctx, sess)
	if err != nil {
		return nil, err
	}
	return gen.RecoverLogin200JSONResponse{Body: body, Headers: gen.RecoverLogin200ResponseHeaders{SetCookie: ptr(issueCookie(ri, token, sess))}}, nil
}

func (s *Server) RecoverPasskeyOptions(ctx context.Context, _ gen.RecoverPasskeyOptionsRequestObject) (gen.RecoverPasskeyOptionsResponseObject, error) {
	id, opts, err := s.Passkeys.BeginRegistration(ctx, info(ctx).session.IDHash)
	if err != nil {
		return nil, ceremonyError(err)
	}
	m, err := optionsMap(opts)
	if err != nil {
		return nil, err
	}
	return gen.RecoverPasskeyOptions200JSONResponse{CeremonyId: id, Options: m}, nil
}

func (s *Server) RecoverPasskeyVerify(ctx context.Context, req gen.RecoverPasskeyVerifyRequestObject) (gen.RecoverPasskeyVerifyResponseObject, error) {
	ri := info(ctx)
	pk, err := s.registerPasskey(ctx, req.Body)
	if err != nil {
		return nil, err
	}
	// One passkey per link: the recovery session ends here; the admin signs in with the new passkey.
	err = s.Store.Tx(ctx, func(q *db.Queries) error {
		_, err := s.Sessions.Revoke(ctx, q, ri.session.IDHash, auth.ReasonLogout, s.actor(ri), ri.ip)
		return err
	})
	if err != nil {
		return nil, err
	}
	return gen.RecoverPasskeyVerify200JSONResponse{Body: passkeyBody(pk),
		Headers: gen.RecoverPasskeyVerify200ResponseHeaders{SetCookie: ptr(newCookie(cookieSession, "", -1).String())}}, nil
}
