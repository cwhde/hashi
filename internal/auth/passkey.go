package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// RelyingParty says which RP ID and origin passkeys are bound to (02 §2): RP ID = root domain, accepted origin =
// exactly https://<admin domain>.
type RelyingParty struct {
	ID     string
	Origin string
}

// RPFromDomains builds the production relying party from the base settings.
func RPFromDomains(rootDomain, adminDomain string) RelyingParty {
	return RelyingParty{ID: rootDomain, Origin: "https://" + adminDomain}
}

// ErrPasskeyUnavailable means base settings are missing, so there is no relying party yet.
var ErrPasskeyUnavailable = errors.New("passkeys need the root and admin domain (setup step 2) first")

// ErrCeremony means the ceremony is unknown, expired, or its response did not verify.
var ErrCeremony = errors.New("passkey check failed or expired; try again")

// ErrLastPasskey forbids deleting the only passkey (02 §2).
var ErrLastPasskey = errors.New("the last passkey cannot be deleted")

const ceremonyTTL = 5 * time.Minute

type ceremonyKind string

const (
	kindRegister ceremonyKind = "register"
	kindLogin    ceremonyKind = "login"
	kindReauth   ceremonyKind = "reauth"
)

type ceremony struct {
	kind    ceremonyKind
	owner   string // session id hash (register/reauth); "" for login
	data    webauthn.SessionData
	expires time.Time
}

// Passkeys runs WebAuthn ceremonies. Ceremony state is in memory only (a restart simply asks the user to retry).
type Passkeys struct {
	st  *store.Store
	rp  func(ctx context.Context) (RelyingParty, error)
	now func() time.Time

	mu         sync.Mutex
	ceremonies map[string]ceremony
}

// NewPasskeys returns the passkey service. rp resolves the relying party from current settings.
func NewPasskeys(st *store.Store, rp func(ctx context.Context) (RelyingParty, error)) *Passkeys {
	return &Passkeys{st: st, rp: rp, now: time.Now, ceremonies: map[string]ceremony{}}
}

// DropSession forgets ceremonies owned by a revoked session (Manager.OnRevoke).
func (p *Passkeys) DropSession(idHash []byte) {
	owner := string(idHash)
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, c := range p.ceremonies {
		if c.owner == owner {
			delete(p.ceremonies, k)
		}
	}
}

func (p *Passkeys) put(c ceremony) string {
	id := ulid.Make().String()
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	for k, old := range p.ceremonies {
		if now.After(old.expires) {
			delete(p.ceremonies, k)
		}
	}
	c.expires = now.Add(ceremonyTTL)
	p.ceremonies[id] = c
	return id
}

// take returns and removes a ceremony: each one can be completed once.
func (p *Passkeys) take(id string, kind ceremonyKind, owner string) (webauthn.SessionData, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	c, ok := p.ceremonies[id]
	delete(p.ceremonies, id)
	if !ok || c.kind != kind || c.owner != owner || p.now().After(c.expires) {
		return webauthn.SessionData{}, ErrCeremony
	}
	return c.data, nil
}

func (p *Passkeys) webauthn(ctx context.Context) (*webauthn.WebAuthn, error) {
	rp, err := p.rp(ctx)
	if err != nil {
		return nil, err
	}
	if rp.ID == "" || rp.Origin == "" {
		return nil, ErrPasskeyUnavailable
	}
	return webauthn.New(&webauthn.Config{
		RPID: rp.ID, RPDisplayName: "Hashi", RPOrigins: []string{rp.Origin},
		AuthenticatorSelection: protocol.AuthenticatorSelection{
			ResidentKey: protocol.ResidentKeyRequirementPreferred, UserVerification: protocol.VerificationRequired,
		},
	})
}

// adminUser adapts the single admin to webauthn.User.
type adminUser struct {
	handle []byte
	creds  []webauthn.Credential
}

func (u adminUser) WebAuthnID() []byte                         { return u.handle }
func (adminUser) WebAuthnName() string                         { return "admin" }
func (adminUser) WebAuthnDisplayName() string                  { return "Hashi admin" }
func (u adminUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

// loadUser returns the admin, creating the user handle on first use.
func (p *Passkeys) loadUser(ctx context.Context, q *db.Queries) (adminUser, error) {
	u, err := q.GetAdminUser(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		h := make([]byte, 64)
		if _, err := rand.Read(h); err != nil {
			return adminUser{}, err
		}
		if err := q.InsertAdminUser(ctx, db.InsertAdminUserParams{UserHandle: h, CreatedAt: p.now().UnixMilli()}); err != nil {
			return adminUser{}, err
		}
		u, err = q.GetAdminUser(ctx)
	}
	if err != nil {
		return adminUser{}, fmt.Errorf("read admin user: %w", err)
	}
	rows, err := q.ListPasskeys(ctx)
	if err != nil {
		return adminUser{}, err
	}
	au := adminUser{handle: u.UserHandle}
	for _, r := range rows {
		var c webauthn.Credential
		if err := json.Unmarshal([]byte(r.CredentialJson), &c); err != nil {
			return adminUser{}, fmt.Errorf("passkey %s: %w", r.ID, err)
		}
		au.creds = append(au.creds, c)
	}
	return au, nil
}

// BeginRegistration starts adding a passkey for the session identified by owner (its id hash).
func (p *Passkeys) BeginRegistration(ctx context.Context, owner []byte) (string, *protocol.CredentialCreation, error) {
	wa, err := p.webauthn(ctx)
	if err != nil {
		return "", nil, err
	}
	u, err := p.loadUser(ctx, p.st.Q)
	if err != nil {
		return "", nil, err
	}
	excl := make([]protocol.CredentialDescriptor, len(u.creds))
	for i, c := range u.creds {
		excl[i] = c.Descriptor()
	}
	opts, sd, err := wa.BeginRegistration(u, webauthn.WithExclusions(excl))
	if err != nil {
		return "", nil, fmt.Errorf("begin registration: %w", err)
	}
	return p.put(ceremony{kind: kindRegister, owner: string(owner), data: *sd}), opts, nil
}

// Passkey is a stored credential's display data.
type Passkey struct {
	ID         string
	Name       string
	Label      string
	CreatedAt  time.Time
	LastUsedAt time.Time
}

// FinishRegistration verifies the browser's response and stores the credential with q.
func (p *Passkeys) FinishRegistration(ctx context.Context, q *db.Queries, owner []byte, ceremonyID, name string, response []byte) (Passkey, error) {
	sd, err := p.take(ceremonyID, kindRegister, string(owner))
	if err != nil {
		return Passkey{}, err
	}
	wa, err := p.webauthn(ctx)
	if err != nil {
		return Passkey{}, err
	}
	u, err := p.loadUser(ctx, q)
	if err != nil {
		return Passkey{}, err
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(response)
	if err != nil {
		return Passkey{}, fmt.Errorf("%w: %v", ErrCeremony, err) //nolint:errorlint // library detail is context only
	}
	cred, err := wa.CreateCredential(u, sd, parsed)
	if err != nil {
		return Passkey{}, fmt.Errorf("%w: %v", ErrCeremony, err) //nolint:errorlint // see above
	}
	raw, err := json.Marshal(cred)
	if err != nil {
		return Passkey{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Passkey"
	}
	pk := Passkey{ID: ulid.Make().String(), Name: name, Label: aaguidLabel(cred.Authenticator.AAGUID), CreatedAt: p.now()}
	if err := q.InsertPasskey(ctx, db.InsertPasskeyParams{
		ID: pk.ID, CredentialID: cred.ID, CredentialJson: string(raw), Name: pk.Name, AaguidLabel: pk.Label,
		CreatedAt: pk.CreatedAt.UnixMilli(),
	}); err != nil {
		return Passkey{}, fmt.Errorf("store passkey: %w", err)
	}
	return pk, nil
}

// aaguidLabel names the authenticator model. Without a metadata service the AAGUID itself is the label; an all-zero
// AAGUID (no attestation) says nothing.
func aaguidLabel(aaguid []byte) string {
	if len(aaguid) != 16 || bytes.Equal(aaguid, make([]byte, 16)) {
		return ""
	}
	id, err := uuid.FromBytes(aaguid)
	if err != nil {
		return ""
	}
	return id.String()
}

// BeginLogin starts a discoverable (usernameless) login.
func (p *Passkeys) BeginLogin(ctx context.Context) (string, *protocol.CredentialAssertion, error) {
	wa, err := p.webauthn(ctx)
	if err != nil {
		return "", nil, err
	}
	opts, sd, err := wa.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return "", nil, fmt.Errorf("begin login: %w", err)
	}
	return p.put(ceremony{kind: kindLogin, data: *sd}), opts, nil
}

// FinishLogin verifies an assertion and returns the matching stored passkey's id, updating its sign count with q.
func (p *Passkeys) FinishLogin(ctx context.Context, q *db.Queries, ceremonyID string, response []byte) (string, error) {
	sd, err := p.take(ceremonyID, kindLogin, "")
	if err != nil {
		return "", err
	}
	return p.finishAssertion(ctx, q, sd, response, true)
}

// BeginReauth starts a passkey assertion for an existing session.
func (p *Passkeys) BeginReauth(ctx context.Context, owner []byte) (string, *protocol.CredentialAssertion, error) {
	wa, err := p.webauthn(ctx)
	if err != nil {
		return "", nil, err
	}
	u, err := p.loadUser(ctx, p.st.Q)
	if err != nil {
		return "", nil, err
	}
	if len(u.creds) == 0 {
		return "", nil, ErrCeremony
	}
	opts, sd, err := wa.BeginLogin(u, webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return "", nil, fmt.Errorf("begin reauth: %w", err)
	}
	return p.put(ceremony{kind: kindReauth, owner: string(owner), data: *sd}), opts, nil
}

// FinishReauth verifies the session owner's assertion.
func (p *Passkeys) FinishReauth(ctx context.Context, q *db.Queries, owner []byte, ceremonyID string, response []byte) (string, error) {
	sd, err := p.take(ceremonyID, kindReauth, string(owner))
	if err != nil {
		return "", err
	}
	return p.finishAssertion(ctx, q, sd, response, false)
}

func (p *Passkeys) finishAssertion(ctx context.Context, q *db.Queries, sd webauthn.SessionData, response []byte, discoverable bool) (string, error) {
	wa, err := p.webauthn(ctx)
	if err != nil {
		return "", err
	}
	u, err := p.loadUser(ctx, q)
	if err != nil {
		return "", err
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(response)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrCeremony, err) //nolint:errorlint // library detail is context only
	}
	var cred *webauthn.Credential
	if discoverable {
		cred, err = wa.ValidateDiscoverableLogin(func(_, userHandle []byte) (webauthn.User, error) {
			if !bytes.Equal(userHandle, u.handle) {
				return nil, errors.New("unknown user handle")
			}
			return u, nil
		}, sd, parsed)
	} else {
		cred, err = wa.ValidateLogin(u, sd, parsed)
	}
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrCeremony, err) //nolint:errorlint // see above
	}
	row, err := q.GetPasskeyByCredentialID(ctx, cred.ID)
	if err != nil {
		return "", fmt.Errorf("%w: credential not registered", ErrCeremony)
	}
	raw, err := json.Marshal(cred)
	if err != nil {
		return "", err
	}
	if err := q.UpdatePasskeyUse(ctx, db.UpdatePasskeyUseParams{
		CredentialJson: string(raw), LastUsedAt: sql.NullInt64{Int64: p.now().UnixMilli(), Valid: true}, ID: row.ID,
	}); err != nil {
		return "", err
	}
	return row.ID, nil
}

// List returns all passkeys.
func (p *Passkeys) List(ctx context.Context) ([]Passkey, error) {
	rows, err := p.st.R.ListPasskeys(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Passkey, len(rows))
	for i, r := range rows {
		out[i] = Passkey{ID: r.ID, Name: r.Name, Label: r.AaguidLabel, CreatedAt: ms(r.CreatedAt)}
		if r.LastUsedAt.Valid {
			out[i].LastUsedAt = ms(r.LastUsedAt.Int64)
		}
	}
	return out, nil
}

// Delete removes a passkey with q unless it is the last one. The caller revokes its sessions in the same transaction.
func Delete(ctx context.Context, q *db.Queries, id string) error {
	n, err := q.CountPasskeys(ctx)
	if err != nil {
		return err
	}
	if n <= 1 {
		return ErrLastPasskey
	}
	if _, err := q.DeletePasskey(ctx, id); err != nil {
		return fmt.Errorf("delete passkey: %w", err)
	}
	return nil
}

// EncodeID is the URL-safe form used in API responses for credential ids.
func EncodeID(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
