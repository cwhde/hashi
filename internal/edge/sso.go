package edge

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/oklog/ulid/v2"
	"golang.org/x/oauth2"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// CookieSession is the edge session cookie, on the root domain so one login covers every subdomain (06 §7). CookieState
// ties a login's callback to the browser that started it.
const (
	CookieSession = "__Secure-hashi.edge"
	CookieState   = "__Host-hashi.edge-state"

	keyPurpose = "edge:session-key"
	loginTTL   = 10 * time.Minute
	maxPending = 10000
	touchEvery = time.Minute
	metaTTL    = 10 * time.Minute
)

// Errors the login flow reports.
var (
	ErrNoProvider = errors.New("no sign-in provider is set up")
	ErrBadLogin   = errors.New("the sign-in could not be completed")
	ErrNoSession  = errors.New("no valid edge session")
)

// Session is a validated edge session.
type Session struct {
	IDHash     string
	ProviderID string
	Subject    string
	Username   string
	Email      string
	Groups     []string
	IDToken    string
}

// HasAnyGroup reports whether the session is in one of the groups; no groups means anyone signed in.
func (s Session) HasAnyGroup(groups []string) bool {
	return len(groups) == 0 || slices.ContainsFunc(groups, func(g string) bool { return slices.Contains(s.Groups, g) })
}

type pending struct {
	provider, verifier, nonce, rd string
	expires                       time.Time
}

type metadata struct {
	provider           *oidc.Provider
	endSessionEndpoint string
	fetched            time.Time
}

// SSO runs the sign-in flow and validates sessions.
type SSO struct {
	st     *store.Store
	vault  *vault.Vault
	client *http.Client
	now    func() time.Time

	mu    sync.Mutex
	state map[string]pending
	meta  map[string]*metadata
	key   []byte
}

// NewSSO returns the flow; client carries the timeouts for calls to providers.
func NewSSO(st *store.Store, v *vault.Vault, client *http.Client, now func() time.Time) *SSO {
	return &SSO{st: st, vault: v, client: client, now: now, state: map[string]pending{}, meta: map[string]*metadata{}}
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// sessionKey loads or creates the HMAC key for session cookies; it lives in the vault.
func (s *SSO) sessionKey(ctx context.Context) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.key != nil {
		return s.key, nil
	}
	rec, err := s.st.R.GetSecretByPurpose(ctx, keyPurpose)
	if err == nil {
		v, err := s.vault.Open(rec)
		if err != nil {
			return nil, err
		}
		s.key = []byte(v.Reveal())
		return s.key, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	tok, err := randomToken(32)
	if err != nil {
		return nil, err
	}
	ins, err := s.vault.Seal(vault.Service, keyPurpose, secrets.New(tok))
	if err != nil {
		return nil, err
	}
	if err := s.st.Tx(ctx, func(q *db.Queries) error { return q.InsertSecret(ctx, ins) }); err != nil {
		return nil, err
	}
	s.key = []byte(tok)
	return s.key, nil
}

func sign(key []byte, id string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(id))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func hashID(id string) string {
	h := sha256.Sum256([]byte(id))
	return hex.EncodeToString(h[:])
}

// discover returns the provider's metadata through the OIDC library, read at most every metaTTL. The library insists the
// document names exactly the configured issuer and handles signing keys and their rotation.
func (s *SSO) discover(ctx context.Context, issuer string, force bool) (*metadata, error) {
	s.mu.Lock()
	m := s.meta[issuer]
	s.mu.Unlock()
	if m != nil && !force && s.now().Sub(m.fetched) < metaTTL {
		return m, nil
	}
	// The key set keeps this context for later fetches, so it must outlive the request.
	p, err := oidc.NewProvider(oidc.ClientContext(context.WithoutCancel(ctx), s.client), issuer)
	if err != nil {
		return nil, fmt.Errorf("discovery: %w", err)
	}
	var extra struct {
		EndSession string `json:"end_session_endpoint"`
	}
	if err := p.Claims(&extra); err != nil {
		return nil, fmt.Errorf("discovery: %w", err)
	}
	fresh := &metadata{provider: p, endSessionEndpoint: extra.EndSession, fetched: s.now()}
	s.mu.Lock()
	s.meta[issuer] = fresh
	s.mu.Unlock()
	return fresh, nil
}

// Check reads a provider's discovery document, for the settings page.
func (s *SSO) Check(ctx context.Context, issuer string) error {
	_, err := s.discover(ctx, issuer, true)
	return err
}

func oauthConfig(p db.OidcProvider, m *metadata, secret, callback string) oauth2.Config {
	return oauth2.Config{ClientID: p.ClientID, ClientSecret: secret, Endpoint: m.provider.Endpoint(), RedirectURL: callback, Scopes: strings.Fields(p.Scopes)}
}

// provider returns the chosen provider, or the default for an empty ID (06 §7).
func (s *SSO) provider(ctx context.Context, id string) (db.OidcProvider, error) {
	if id != "" {
		p, err := s.st.R.GetOIDCProvider(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return p, ErrNoProvider
		}
		return p, err
	}
	all, err := s.st.R.ListOIDCProviders(ctx)
	if err != nil {
		return db.OidcProvider{}, err
	}
	for _, p := range all {
		if p.IsDefault == 1 {
			return p, nil
		}
	}
	return db.OidcProvider{}, ErrNoProvider
}

// DefaultProviderID is the default provider's ID, or "" without one.
func (s *SSO) DefaultProviderID(ctx context.Context) string {
	p, err := s.provider(ctx, "")
	if err != nil {
		return ""
	}
	return p.ID
}

// Begin starts a login and returns the provider's URL and the state for the browser's cookie.
func (s *SSO) Begin(ctx context.Context, providerID, rd, callback string) (authURL, state string, err error) {
	p, err := s.provider(ctx, providerID)
	if err != nil {
		return "", "", err
	}
	m, err := s.discover(ctx, p.Issuer, false)
	if err != nil {
		return "", "", err
	}
	verifier, err := randomToken(48)
	if err != nil {
		return "", "", err
	}
	if state, err = randomToken(24); err != nil {
		return "", "", err
	}
	nonce, err := randomToken(24)
	if err != nil {
		return "", "", err
	}
	now := s.now()
	s.mu.Lock()
	for k, v := range s.state {
		if now.After(v.expires) {
			delete(s.state, k)
		}
	}
	// A flood of unfinished logins may not lock everyone out: the oldest goes, never the new one.
	for len(s.state) >= maxPending {
		var oldest string
		for k, v := range s.state {
			if oldest == "" || v.expires.Before(s.state[oldest].expires) {
				oldest = k
			}
		}
		delete(s.state, oldest)
	}
	s.state[state] = pending{provider: p.ID, verifier: verifier, nonce: nonce, rd: rd, expires: now.Add(loginTTL)}
	s.mu.Unlock()

	cfg := oauthConfig(p, m, "", callback)
	return cfg.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), state, nil
}

// Identity is what the ID token says, after verification.
type Identity struct {
	Subject, Username, Email string
	Groups                   []string
	IDToken                  string
}

// Finish validates the callback, exchanges the code and creates the session. It returns the cookie value and the
// original address. A state is good for one try.
func (s *SSO) Finish(ctx context.Context, code, state, cookieState, callback, ip, ua string) (cookie, rd string, expires time.Time, err error) {
	if state == "" || subtle.ConstantTimeCompare([]byte(state), []byte(cookieState)) != 1 {
		return "", "", expires, fmt.Errorf("%w: the browser did not start this sign-in", ErrBadLogin)
	}
	s.mu.Lock()
	pend, ok := s.state[state]
	delete(s.state, state)
	s.mu.Unlock()
	if !ok || s.now().After(pend.expires) {
		return "", "", expires, fmt.Errorf("%w: the sign-in took too long", ErrBadLogin)
	}
	p, err := s.provider(ctx, pend.provider)
	if err != nil {
		return "", "", expires, err
	}
	id, err := s.exchange(ctx, p, code, pend, callback)
	if err != nil {
		return "", "", expires, err
	}
	key, err := s.sessionKey(ctx)
	if err != nil {
		return "", "", expires, err
	}
	sessionID, err := randomToken(32)
	if err != nil {
		return "", "", expires, err
	}
	cfg, err := settings.Get[settings.EdgeSSO](ctx, s.st.R)
	if err != nil {
		return "", "", expires, err
	}
	groups, err := json.Marshal(append([]string{}, id.Groups...))
	if err != nil {
		return "", "", expires, err
	}
	now := s.now()
	expires = now.Add(time.Duration(cfg.MaxSessionHours) * time.Hour)
	err = s.st.Tx(ctx, func(q *db.Queries) error {
		return q.InsertEdgeSession(ctx, db.InsertEdgeSessionParams{IDHash: hashID(sessionID), ProviderID: p.ID, Subject: id.Subject,
			Username: id.Username, Email: id.Email, GroupsJson: string(groups), IDToken: id.IDToken, Ip: ip, UserAgent: ua,
			CreatedAt: now.UnixMilli(), LastSeenAt: now.UnixMilli(), ExpiresAt: expires.UnixMilli()})
	})
	if err != nil {
		return "", "", expires, fmt.Errorf("store session: %w", err)
	}
	return sessionID + "." + sign(key, sessionID), pend.rd, expires, nil
}

func (s *SSO) exchange(ctx context.Context, p db.OidcProvider, code string, pend pending, callback string) (Identity, error) {
	m, err := s.discover(ctx, p.Issuer, false)
	if err != nil {
		return Identity{}, err
	}
	rec, err := s.st.R.GetSecret(ctx, p.SecretID)
	if err != nil {
		return Identity{}, err
	}
	secret, err := s.vault.Open(rec)
	if err != nil {
		return Identity{}, err
	}
	cfg := oauthConfig(p, m, secret.Reveal(), callback)
	tok, err := cfg.Exchange(oidc.ClientContext(ctx, s.client), code, oauth2.VerifierOption(pend.verifier))
	if err != nil {
		var re *oauth2.RetrieveError
		if errors.As(err, &re) { // its text can quote the response; the code is enough
			return Identity{}, fmt.Errorf("%w: the provider refused the code (%s)", ErrBadLogin, re.ErrorCode)
		}
		return Identity{}, fmt.Errorf("token request: %w", err)
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return Identity{}, fmt.Errorf("%w: the provider sent no ID token", ErrBadLogin)
	}
	return s.verify(ctx, p, m, raw, pend.nonce)
}

// verify checks the ID token through the OIDC library (signature, issuer, audience, expiry), then the nonce, and maps the
// claims (06 §7).
func (s *SSO) verify(ctx context.Context, p db.OidcProvider, m *metadata, raw, nonce string) (Identity, error) {
	tok, err := m.provider.VerifierContext(oidc.ClientContext(context.WithoutCancel(ctx), s.client), &oidc.Config{ClientID: p.ClientID, Now: s.now}).Verify(ctx, raw)
	if err != nil {
		return Identity{}, fmt.Errorf("%w: the ID token is not valid: %w", ErrBadLogin, err)
	}
	if subtle.ConstantTimeCompare([]byte(tok.Nonce), []byte(nonce)) != 1 {
		return Identity{}, fmt.Errorf("%w: the ID token answers another sign-in", ErrBadLogin)
	}
	if tok.Subject == "" {
		return Identity{}, fmt.Errorf("%w: the ID token names nobody", ErrBadLogin)
	}
	claims := map[string]any{}
	if err := tok.Claims(&claims); err != nil {
		return Identity{}, fmt.Errorf("%w: %w", ErrBadLogin, err)
	}
	id := Identity{IDToken: raw, Subject: tok.Subject}
	id.Username, _ = claims[p.UsernameClaim].(string)
	id.Email, _ = claims[p.EmailClaim].(string)
	switch g := claims[p.GroupsClaim].(type) {
	case []any:
		for _, v := range g {
			if name, ok := v.(string); ok {
				id.Groups = append(id.Groups, name)
			}
		}
	case string:
		id.Groups = []string{g}
	}
	return id, nil
}

// Validate checks a cookie value and returns its live session. Idle and maximum limits come from settings at creation
// (expires_at) and now (idle).
func (s *SSO) Validate(ctx context.Context, cookie string) (Session, error) {
	id, mac, ok := strings.Cut(cookie, ".")
	if !ok || id == "" {
		return Session{}, ErrNoSession
	}
	key, err := s.sessionKey(ctx)
	if err != nil {
		return Session{}, err
	}
	if !hmac.Equal([]byte(mac), []byte(sign(key, id))) {
		return Session{}, ErrNoSession
	}
	row, err := s.st.R.GetEdgeSession(ctx, hashID(id))
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, ErrNoSession
	}
	if err != nil {
		return Session{}, err
	}
	cfg, err := settings.Get[settings.EdgeSSO](ctx, s.st.R)
	if err != nil {
		return Session{}, err
	}
	now := s.now()
	if now.UnixMilli() >= row.ExpiresAt || now.Sub(time.UnixMilli(row.LastSeenAt)) >= time.Duration(cfg.IdleTimeoutHours)*time.Hour {
		return Session{}, ErrNoSession
	}
	out := Session{IDHash: row.IDHash, ProviderID: row.ProviderID, Subject: row.Subject, Username: row.Username, Email: row.Email, IDToken: row.IDToken}
	if err := json.Unmarshal([]byte(row.GroupsJson), &out.Groups); err != nil {
		return Session{}, err
	}
	if now.Sub(time.UnixMilli(row.LastSeenAt)) >= touchEvery { // the decision is never delayed by this write
		go func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = s.st.Tx(ctx, func(q *db.Queries) error {
				return q.TouchEdgeSession(ctx, db.TouchEdgeSessionParams{LastSeenAt: now.UnixMilli(), IDHash: row.IDHash})
			})
		}()
	}
	return out, nil
}

// Logout ends the session and returns the provider's end-session address, when it has one.
func (s *SSO) Logout(ctx context.Context, cookie, postLogout string) (string, error) {
	sess, err := s.Validate(ctx, cookie)
	if errors.Is(err, ErrNoSession) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if err := s.st.Tx(ctx, func(q *db.Queries) error { return q.DeleteEdgeSession(ctx, sess.IDHash) }); err != nil {
		return "", err
	}
	p, err := s.st.R.GetOIDCProvider(ctx, sess.ProviderID)
	if err != nil {
		return "", nil //nolint:nilerr // the session is gone; a missing provider only skips the provider's logout
	}
	m, err := s.discover(ctx, p.Issuer, false)
	if err != nil || m.endSessionEndpoint == "" {
		return "", nil //nolint:nilerr // same
	}
	q := url.Values{"id_token_hint": {sess.IDToken}}
	if postLogout != "" {
		q.Set("post_logout_redirect_uri", postLogout)
	}
	return m.endSessionEndpoint + "?" + q.Encode(), nil
}

// Cleanup drops ended sessions.
func (s *SSO) Cleanup(ctx context.Context) error {
	cfg, err := settings.Get[settings.EdgeSSO](ctx, s.st.R)
	if err != nil {
		return err
	}
	now := s.now()
	return s.st.Tx(ctx, func(q *db.Queries) error {
		return q.DeleteExpiredEdgeSessions(ctx, db.DeleteExpiredEdgeSessionsParams{ExpiresAt: now.UnixMilli(),
			LastSeenAt: now.Add(-time.Duration(cfg.IdleTimeoutHours) * time.Hour).UnixMilli()})
	})
}

// NewProviderID names a new provider.
func NewProviderID() string { return ulid.Make().String() }
