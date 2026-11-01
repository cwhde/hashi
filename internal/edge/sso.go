package edge

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/oklog/ulid/v2"

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
	maxPending = 1000
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
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	EndSessionEndpoint    string `json:"end_session_endpoint"`
	fetched               time.Time
	keys                  map[string]any
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

// discover returns the provider's metadata, fetched at most every metaTTL. The issuer must be the configured one.
func (s *SSO) discover(ctx context.Context, issuer string, force bool) (*metadata, error) {
	issuer = strings.TrimRight(issuer, "/")
	s.mu.Lock()
	m := s.meta[issuer]
	s.mu.Unlock()
	if m != nil && !force && s.now().Sub(m.fetched) < metaTTL {
		return m, nil
	}
	var fresh metadata
	if err := s.getJSON(ctx, issuer+"/.well-known/openid-configuration", &fresh); err != nil {
		return nil, fmt.Errorf("discovery: %w", err)
	}
	if strings.TrimRight(fresh.Issuer, "/") != issuer {
		return nil, fmt.Errorf("discovery names issuer %q, expected %q", fresh.Issuer, issuer)
	}
	if fresh.AuthorizationEndpoint == "" || fresh.TokenEndpoint == "" || fresh.JWKSURI == "" {
		return nil, errors.New("discovery is missing an endpoint")
	}
	var set struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
			Crv string `json:"crv"`
			X   string `json:"x"`
			Y   string `json:"y"`
		} `json:"keys"`
	}
	if err := s.getJSON(ctx, fresh.JWKSURI, &set); err != nil {
		return nil, fmt.Errorf("signing keys: %w", err)
	}
	fresh.keys = map[string]any{}
	dec := base64.RawURLEncoding.DecodeString
	for _, k := range set.Keys {
		switch k.Kty {
		case "RSA":
			n, e1 := dec(k.N)
			e, e2 := dec(k.E)
			if e1 == nil && e2 == nil {
				fresh.keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
			}
		case "EC":
			x, e1 := dec(k.X)
			y, e2 := dec(k.Y)
			if e1 == nil && e2 == nil && k.Crv == "P-256" {
				fresh.keys[k.Kid] = &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)} //nolint:staticcheck // the JWT library takes this form
			}
		}
	}
	fresh.fetched = s.now()
	s.mu.Lock()
	s.meta[issuer] = &fresh
	s.mu.Unlock()
	return &fresh, nil
}

// Check reads a provider's discovery document, for the settings page.
func (s *SSO) Check(ctx context.Context, issuer string) error {
	_, err := s.discover(ctx, issuer, true)
	return err
}

func (s *SSO) getJSON(ctx context.Context, u string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %d", u, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(v)
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
	if len(s.state) >= maxPending {
		s.mu.Unlock()
		return "", "", errors.New("too many sign-ins in progress")
	}
	s.state[state] = pending{provider: p.ID, verifier: verifier, nonce: nonce, rd: rd, expires: now.Add(loginTTL)}
	s.mu.Unlock()

	ch := sha256.Sum256([]byte(verifier))
	q := url.Values{"client_id": {p.ClientID}, "response_type": {"code"}, "scope": {p.Scopes}, "redirect_uri": {callback},
		"state": {state}, "nonce": {nonce}, "code_challenge": {base64.RawURLEncoding.EncodeToString(ch[:])}, "code_challenge_method": {"S256"}}
	sep := "?"
	if strings.Contains(m.AuthorizationEndpoint, "?") {
		sep = "&"
	}
	return m.AuthorizationEndpoint + sep + q.Encode(), state, nil
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
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {callback}, "code_verifier": {pend.verifier}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Identity{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(url.QueryEscape(p.ClientID), url.QueryEscape(secret.Reveal())) // RFC 6749 §2.3.1
	resp, err := s.client.Do(req)
	if err != nil {
		return Identity{}, fmt.Errorf("token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		IDToken string `json:"id_token"`
		Error   string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return Identity{}, fmt.Errorf("token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK || body.IDToken == "" {
		return Identity{}, fmt.Errorf("%w: the provider refused the code (%d %s)", ErrBadLogin, resp.StatusCode, body.Error)
	}
	return s.verify(ctx, p, body.IDToken, pend.nonce)
}

// verify checks the ID token's signature, issuer, audience, expiry and nonce, then maps the claims (06 §7).
func (s *SSO) verify(ctx context.Context, p db.OidcProvider, raw, nonce string) (Identity, error) {
	keyFor := func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		m, err := s.discover(ctx, p.Issuer, false)
		if err != nil {
			return nil, err
		}
		if k, ok := m.keys[kid]; ok {
			return k, nil
		}
		// A rotated key: read the provider once more.
		if m, err = s.discover(ctx, p.Issuer, true); err != nil {
			return nil, err
		}
		if k, ok := m.keys[kid]; ok {
			return k, nil
		}
		return nil, fmt.Errorf("unknown signing key %q", kid)
	}
	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, keyFor, jwt.WithValidMethods([]string{"RS256", "ES256"}),
		jwt.WithIssuer(strings.TrimRight(p.Issuer, "/")), jwt.WithAudience(p.ClientID), jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(s.now), jwt.WithLeeway(time.Minute))
	if err != nil {
		return Identity{}, fmt.Errorf("%w: the ID token is not valid: %w", ErrBadLogin, err)
	}
	got, _ := claims["nonce"].(string)
	if subtle.ConstantTimeCompare([]byte(got), []byte(nonce)) != 1 {
		return Identity{}, fmt.Errorf("%w: the ID token answers another sign-in", ErrBadLogin)
	}
	id := Identity{IDToken: raw}
	id.Subject, _ = claims["sub"].(string)
	if id.Subject == "" {
		return Identity{}, fmt.Errorf("%w: the ID token names nobody", ErrBadLogin)
	}
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
	if err != nil || m.EndSessionEndpoint == "" {
		return "", nil //nolint:nilerr // same
	}
	q := url.Values{"id_token_hint": {sess.IDToken}}
	if postLogout != "" {
		q.Set("post_logout_redirect_uri", postLogout)
	}
	return m.EndSessionEndpoint + "?" + q.Encode(), nil
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
