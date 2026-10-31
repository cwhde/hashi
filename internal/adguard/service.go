package adguard

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// Connection modes (03 §8). Read-only exists so an instance a sync tool still overwrites can be watched, not written.
const (
	ModeActive   = "active"
	ModeReadOnly = "read_only"
	ModeDisabled = "disabled"
)

// JobKind and JobInterval are the reconcile job's.
const (
	JobKind     = "adguard.reconcile"
	JobInterval = 5 * time.Minute
)

// Errors the API maps.
var (
	ErrNotFound = errors.New("no such AdGuard connection")
	ErrBadInput = errors.New("invalid")
	// ErrUnreachable: the connection does not work yet; saving it anyway is the admin's explicit choice (08 §5).
	ErrUnreachable = errors.New("could not connect")
)

// Input is a connection's editable fields. A zero Password keeps the stored one.
type Input struct {
	Name, TargetSetID, Scheme, PathPrefix, ServerName, Username, Mode string
	Port                                                              int
	TLSVerify                                                         bool
	Password                                                          secrets.Value
	// SaveInactive saves a connection that does not answer yet.
	SaveInactive bool
}

func (in Input) check() error {
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("%w: a name is required", ErrBadInput)
	}
	if in.Scheme != "http" && in.Scheme != "https" {
		return fmt.Errorf("%w: scheme must be http or https", ErrBadInput)
	}
	if in.Port < 1 || in.Port > 65535 {
		return fmt.Errorf("%w: port must be between 1 and 65535", ErrBadInput)
	}
	if in.Mode != ModeActive && in.Mode != ModeReadOnly && in.Mode != ModeDisabled {
		return fmt.Errorf("%w: unknown mode %q", ErrBadInput, in.Mode)
	}
	if in.PathPrefix != "" && !strings.HasPrefix(in.PathPrefix, "/") {
		return fmt.Errorf("%w: the path prefix must start with /", ErrBadInput)
	}
	return nil
}

// Resolver resolves a target position (ipsets.Service.ResolveTarget).
type Resolver func(ctx context.Context, consumer, setID string) (ipsets.Target, error)

// Service keeps connections and owned rewrites.
type Service struct {
	st         *store.Store
	vault      *vault.Vault
	now        func() time.Time
	resolve    Resolver
	wanted     func(ctx context.Context) ([]Desired, error)
	originGone func(ctx context.Context, origin string) bool
}

// UseOriginGone says which rewrites lost their owner on purpose (03 §9).
func (s *Service) UseOriginGone(f func(ctx context.Context, origin string) bool) { s.originGone = f }

func (s *Service) gone(ctx context.Context) func(string) bool {
	return func(origin string) bool { return s.originGone != nil && s.originGone(ctx, origin) }
}

// ErrAgentTarget refuses sending a password to an address an agent reported unless the instance proves who it is.
var ErrAgentTarget = fmt.Errorf("%w: a target using a Pulse agent needs HTTPS, certificate checks and an expected hostname, because the agent's reported address decides where the password goes", ErrBadInput)

// agentSafe holds whenever the password may follow an agent's address (08 §3: a token can do nothing but its heartbeat).
func (s *Service) agentSafe(ctx context.Context, setID, scheme, serverName string, verify, hasPassword bool) error {
	if !hasPassword {
		return nil
	}
	uses, err := ipsets.UsesPulse(ctx, s.st.R, setID)
	if err != nil {
		return err
	}
	if uses && (scheme != "https" || !verify || serverName == "") {
		return ErrAgentTarget
	}
	return nil
}

// validate runs before a save (08 §5): the set can be a target, the password is safe, and the instance answers.
func (s *Service) validate(ctx context.Context, in Input, id string, current sql.NullString) error {
	if err := s.checkTarget(ctx, in.TargetSetID); err != nil {
		return err
	}
	if err := s.agentSafe(ctx, in.TargetSetID, in.Scheme, in.ServerName, in.TLSVerify, !in.Password.IsZero() || current.Valid); err != nil {
		return err
	}
	if in.SaveInactive || in.Mode == ModeDisabled { // a disabled connection is not contacted, not even to test it
		return nil
	}
	silent, err := ipsets.SilentAgents(ctx, s.st.R, in.TargetSetID)
	if err != nil {
		return err
	}
	if len(silent) > 0 {
		return fmt.Errorf("%w: Pulse agent %s has not sent a heartbeat yet", ErrUnreachable, strings.Join(silent, ", "))
	}
	password := in.Password
	if password.IsZero() && current.Valid {
		rec, err := s.st.R.GetSecret(ctx, current.String)
		if err != nil {
			return err
		}
		if password, err = s.vault.Open(rec); err != nil {
			return err
		}
	}
	row := db.AdguardConnection{ID: id, TargetSetID: in.TargetSetID, Port: int64(in.Port), Scheme: in.Scheme, PathPrefix: in.PathPrefix, TlsVerify: b2i(in.TLSVerify), ServerName: in.ServerName, Username: in.Username}
	if _, _, err := s.dialWith(ctx, row, password); err != nil {
		return fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
	return nil
}

// New returns the service. wanted lists the rewrites Hashi wants; resolve turns a target set into addresses.
func New(st *store.Store, v *vault.Vault, resolve Resolver, wanted func(ctx context.Context) ([]Desired, error)) *Service {
	return &Service{st: st, vault: v, now: time.Now, resolve: resolve, wanted: wanted}
}

// Consumers lists the connections reaching their instance through set id (05a §5.2).
func Consumers(ctx context.Context, q *db.Queries, id string) ([]ipsets.Consumer, error) {
	rows, err := q.ListAdGuardConnectionsBySet(ctx, id)
	if err != nil {
		return nil, err
	}
	out := make([]ipsets.Consumer, 0, len(rows))
	for _, r := range rows {
		out = append(out, ipsets.Consumer{Type: "adguard", ID: r.ID, Name: "AdGuard " + r.Name, Position: ipsets.PositionTarget})
	}
	return out, nil
}

func (s *Service) checkTarget(ctx context.Context, id string) error {
	if err := ipsets.CheckTarget(ctx, s.st.R, id); err != nil {
		return fmt.Errorf("%w: %w", ErrBadInput, err)
	}
	return nil
}

// Create stores a connection. It starts read-only unless the input says otherwise.
func (s *Service) Create(ctx context.Context, in Input) (db.AdguardConnection, error) {
	if in.Mode == "" {
		in.Mode = ModeReadOnly
	}
	if err := in.check(); err != nil {
		return db.AdguardConnection{}, err
	}
	id, now := ulid.Make().String(), s.now().UnixMilli()
	if err := s.validate(ctx, in, id, sql.NullString{}); err != nil {
		return db.AdguardConnection{}, err
	}
	err := s.st.Tx(ctx, func(q *db.Queries) error {
		var secretID sql.NullString
		if !in.Password.IsZero() {
			p, err := s.vault.Seal(vault.Service, "adguard:"+id, in.Password)
			if err != nil {
				return err
			}
			if err := q.InsertSecret(ctx, p); err != nil {
				return err
			}
			secretID = sql.NullString{String: p.ID, Valid: true}
		}
		return q.InsertAdGuardConnection(ctx, db.InsertAdGuardConnectionParams{ID: id, Name: strings.TrimSpace(in.Name), TargetSetID: in.TargetSetID, Port: int64(in.Port),
			Scheme: in.Scheme, PathPrefix: in.PathPrefix, TlsVerify: b2i(in.TLSVerify), ServerName: in.ServerName, Username: in.Username, SecretID: secretID, Mode: in.Mode, CreatedAt: now, UpdatedAt: now})
	})
	if err != nil {
		return db.AdguardConnection{}, err
	}
	return s.st.R.GetAdGuardConnection(ctx, id)
}

// Update changes a connection; a zero password keeps the stored one.
func (s *Service) Update(ctx context.Context, id string, in Input) (db.AdguardConnection, error) {
	if err := in.check(); err != nil {
		return db.AdguardConnection{}, err
	}
	cur, err := s.st.R.GetAdGuardConnection(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return db.AdguardConnection{}, ErrNotFound
	}
	if err != nil {
		return db.AdguardConnection{}, err
	}
	if err := s.validate(ctx, in, id, cur.SecretID); err != nil {
		return db.AdguardConnection{}, err
	}
	err = s.st.Tx(ctx, func(q *db.Queries) error {
		row, err := q.GetAdGuardConnection(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		secretID := row.SecretID
		if !in.Password.IsZero() {
			p, err := s.vault.Seal(vault.Service, "adguard:"+id, in.Password)
			if err != nil {
				return err
			}
			if err := q.InsertSecret(ctx, p); err != nil {
				return err
			}
			secretID = sql.NullString{String: p.ID, Valid: true}
		}
		if err := q.UpdateAdGuardConnection(ctx, db.UpdateAdGuardConnectionParams{Name: strings.TrimSpace(in.Name), TargetSetID: in.TargetSetID, Port: int64(in.Port), Scheme: in.Scheme,
			PathPrefix: in.PathPrefix, TlsVerify: b2i(in.TLSVerify), ServerName: in.ServerName, Username: in.Username, SecretID: secretID, Mode: in.Mode, UpdatedAt: s.now().UnixMilli(), ID: id}); err != nil {
			return err
		}
		if row.SecretID.Valid && row.SecretID != secretID {
			return q.DeleteSecret(ctx, row.SecretID.String)
		}
		return nil
	})
	if err != nil {
		return db.AdguardConnection{}, err
	}
	return s.st.R.GetAdGuardConnection(ctx, id)
}

// Delete forgets a connection and its secret. Rewrites stay on the instance: removing them is a plan, not a side effect.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.st.Tx(ctx, func(q *db.Queries) error {
		row, err := q.GetAdGuardConnection(ctx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := q.DeleteAdGuardConnection(ctx, id); err != nil {
			return err
		}
		if row.SecretID.Valid {
			return q.DeleteSecret(ctx, row.SecretID.String)
		}
		return nil
	})
}

// connect finds the first of the target's addresses that answers (08 §5: addresses are tried in item order) and records the result.
func (s *Service) connect(ctx context.Context, row db.AdguardConnection) (Client, error) {
	c, version, err := s.dial(ctx, row)
	msg := ""
	if err != nil {
		msg = err.Error()
	}
	if e := s.st.Q.SetAdGuardConnectionCheck(ctx, db.SetAdGuardConnectionCheckParams{LastCheckedAt: sql.NullInt64{Int64: s.now().UnixMilli(), Valid: true},
		LastVersion: version, LastError: msg, ID: row.ID}); e != nil {
		return c, e
	}
	return c, err
}

func (s *Service) dial(ctx context.Context, row db.AdguardConnection) (Client, string, error) {
	var password secrets.Value
	if row.SecretID.Valid {
		rec, err := s.st.R.GetSecret(ctx, row.SecretID.String)
		if err != nil {
			return Client{}, "", err
		}
		if password, err = s.vault.Open(rec); err != nil {
			return Client{}, "", err
		}
	}
	return s.dialWith(ctx, row, password)
}

func (s *Service) dialWith(ctx context.Context, row db.AdguardConnection, password secrets.Value) (Client, string, error) {
	// Checked on every connection, not only on save: the set may have gained a Pulse item since.
	if err := s.agentSafe(ctx, row.TargetSetID, row.Scheme, row.ServerName, row.TlsVerify == 1, !password.IsZero()); err != nil {
		return Client{}, "", err
	}
	t, err := s.resolve(ctx, "adguard:"+row.ID, row.TargetSetID)
	if err != nil {
		return Client{}, "", err
	}
	if len(t.Addresses) == 0 {
		return Client{}, "", fmt.Errorf("the target has no address: %s", t.Unresolved)
	}
	h := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{
		MinVersion: tls.VersionTLS12, ServerName: row.ServerName, InsecureSkipVerify: row.TlsVerify == 0, //nolint:gosec // the admin's explicit per-connection choice (03 §8)
	}}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	var last error
	for _, a := range t.Addresses {
		c := Client{Base: row.Scheme + "://" + net.JoinHostPort(a.String(), strconv.FormatInt(row.Port, 10)) + row.PathPrefix, Username: row.Username, Password: password, HTTP: h, Host: row.ServerName}
		st, err := c.Status(ctx)
		if err == nil {
			return c, st.Version, nil
		}
		if errors.Is(err, ErrAuth) {
			return c, "", err
		}
		last = err
	}
	return Client{}, "", last
}

// Test connects now and records the result.
func (s *Service) Test(ctx context.Context, id string) (db.AdguardConnection, error) {
	row, err := s.st.R.GetAdGuardConnection(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, ErrNotFound
	}
	if err != nil {
		return row, err
	}
	if row.Mode == ModeDisabled {
		return row, nil
	}
	_, _ = s.connect(ctx, row) // the outcome is in the row
	return s.st.R.GetAdGuardConnection(ctx, id)
}

// View is what an instance holds and what Hashi would do there, for read-only connections too.
type View struct {
	Rewrites []ViewRewrite
	Planned  []hsync.Item
}

// ViewRewrite is one rewrite on the instance.
type ViewRewrite struct {
	Rewrite
	Owned bool
}

// Rewrites reads the instance. It changes nothing there.
func (s *Service) Rewrites(ctx context.Context, id string) (View, error) {
	row, err := s.st.R.GetAdGuardConnection(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return View{}, ErrNotFound
	}
	if err != nil {
		return View{}, err
	}
	if row.Mode == ModeDisabled { // neither contacted nor planned (03 §8)
		return View{}, nil
	}
	c, err := s.connect(ctx, row)
	if err != nil {
		return View{}, err
	}
	remote, err := c.List(ctx)
	if err != nil {
		return View{}, err
	}
	owned, err := s.st.R.ListAdGuardOwned(ctx, id)
	if err != nil {
		return View{}, err
	}
	desired, err := s.wanted(ctx)
	if err != nil {
		return View{}, err
	}
	mine := map[key]bool{}
	for _, o := range owned {
		mine[key{o.Domain, o.Answer}] = true
	}
	v := View{Planned: diff(desired, owned, remote, s.gone(ctx))}
	for _, r := range remote {
		v.Rewrites = append(v.Rewrites, ViewRewrite{Rewrite: r, Owned: mine[key{r.Domain, r.Answer}]})
	}
	return v, nil
}

// ReconnectUsing retests the connections reaching their instance through changed sets (08 §4 step 5).
func (s *Service) ReconnectUsing(ctx context.Context, changed []string) {
	for _, id := range changed {
		rows, err := s.st.R.ListAdGuardConnectionsBySet(ctx, id)
		if err != nil {
			continue
		}
		for _, r := range rows {
			if r.Mode != ModeDisabled {
				_ = s.st.Q.SetAdGuardConnectionCheck(ctx, db.SetAdGuardConnectionCheckParams{LastCheckedAt: r.LastCheckedAt, LastVersion: r.LastVersion,
					LastError: "reconnecting after an address change", ID: r.ID})
				_, _ = s.Test(ctx, r.ID)
			}
		}
	}
}

// Reconcile is the job: every active connection gets its plan; low-risk plans apply, high-risk ones wait (invariant 16).
// One failing instance does not stop the others.
func (s *Service) Reconcile(eng *hsync.Engine) func(ctx context.Context) (string, error) {
	return func(ctx context.Context) (string, error) {
		rows, err := s.st.R.ListAdGuardConnections(ctx)
		if err != nil {
			return "", err
		}
		active, failed := 0, 0
		var errs []error
		for _, r := range rows {
			if r.Mode == ModeDisabled {
				continue
			}
			if r.Mode == ModeReadOnly {
				_, _ = s.Test(ctx, r.ID)
				continue
			}
			active++
			if _, err := eng.Auto(ctx, Subsystem, r.ID, hsync.TriggerReconcile, audit.System); err != nil {
				failed++
				errs = append(errs, fmt.Errorf("%s: %w", r.Name, err))
			}
		}
		return fmt.Sprintf("%d active, %d failed", active, failed), errors.Join(errs...)
	}
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
