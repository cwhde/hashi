package scripts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/sshx"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// Errors the API maps.
var (
	ErrInvalid  = errors.New("invalid script")
	ErrNotFound = errors.New("script not found")
	ErrNotReady = errors.New("the script is not on that host yet")
)

// Host is an open SSH connection to a firewall host (firewall.Conn satisfies it).
type Host interface {
	Run(ctx context.Context, cmd string, stdin io.Reader) (sshx.Result, error)
	ReadFile(p string) ([]byte, error)
	WriteFileAtomic(p string, data []byte, mode fs.FileMode) (bool, error)
	MkdirAll(p string, mode fs.FileMode) error
	Close() error
}

// Dialer opens SSH to a firewall host by id.
type Dialer func(ctx context.Context, hostID string) (Host, error)

// Service keeps scripts and their runs (09 §1).
type Service struct {
	st    *store.Store
	vault *vault.Vault
	audit *audit.Log
	dial  Dialer
	log   *slog.Logger
	now   func() time.Time
	// planHosts proposes a scripts plan for each host after a save (the sync engine; nil in some tests).
	planHosts func(ctx context.Context, hostIDs []string, actor string)
}

// New returns the service.
func New(st *store.Store, v *vault.Vault, a *audit.Log, log *slog.Logger, dial Dialer) *Service {
	return &Service{st: st, vault: v, audit: a, dial: dial, log: log, now: time.Now}
}

// OnSave sets what runs after a save changes what hosts should hold.
func (s *Service) OnSave(fn func(ctx context.Context, hostIDs []string, actor string)) {
	s.planHosts = fn
}

// EnvVar is one variable as the admin enters it. A secret with an empty Value keeps the stored one.
type EnvVar struct {
	Name   string
	Value  secrets.Value
	Secret bool
}

// Input is a script as the admin saves it.
type Input struct {
	Name, Description, Body string
	Enabled                 bool
	Schedule                string // cron; "" = manual only
	TimeoutSeconds          int
	AllHosts                bool
	Hosts                   []string
	Env                     []EnvVar
}

// Script is a stored script with its targets and variable names (secret values never leave the vault here).
type Script struct {
	db.Script
	Hosts []string
	Env   []db.ScriptEnv
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// slugOf makes the file name on the host from the name: lower case, a-z 0-9 and dashes.
func slugOf(name string) string {
	s := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(s) > 48 {
		s = strings.TrimRight(s[:48], "-")
	}
	return s
}

func (s *Service) validate(in Input) error {
	switch {
	case strings.TrimSpace(in.Name) == "" || slugOf(in.Name) == "":
		return fmt.Errorf("%w: a name with at least one letter or digit", ErrInvalid)
	case strings.TrimSpace(in.Body) == "":
		return fmt.Errorf("%w: the script is empty", ErrInvalid)
	case !strings.HasPrefix(in.Body, "#!"):
		return fmt.Errorf("%w: the script must start with a #! line, e.g. #!/bin/sh", ErrInvalid)
	case in.TimeoutSeconds < 1 || in.TimeoutSeconds > 86400:
		return fmt.Errorf("%w: the timeout is 1 second to 24 hours", ErrInvalid)
	}
	if in.Schedule != "" {
		if _, err := ParseCron(in.Schedule); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalid, err)
		}
	}
	seen := map[string]bool{}
	for _, e := range in.Env {
		if !envRe.MatchString(e.Name) {
			return fmt.Errorf("%w: variable name %q (letters, digits, _; not starting with a digit)", ErrInvalid, e.Name)
		}
		if seen[e.Name] {
			return fmt.Errorf("%w: variable %s twice", ErrInvalid, e.Name)
		}
		seen[e.Name] = true
	}
	return nil
}

// Create stores a new script.
func (s *Service) Create(ctx context.Context, actor, ip string, in Input) (Script, error) {
	return s.save(ctx, actor, ip, "", in)
}

// Update replaces a script.
func (s *Service) Update(ctx context.Context, actor, ip, id string, in Input) (Script, error) {
	return s.save(ctx, actor, ip, id, in)
}

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func (s *Service) save(ctx context.Context, actor, ip, id string, in Input) (Script, error) {
	in.Schedule = strings.TrimSpace(in.Schedule)
	if err := s.validate(in); err != nil {
		return Script{}, err
	}
	now := s.now().UnixMilli()
	var before []string
	err := s.st.Tx(ctx, func(q *db.Queries) error {
		hosts, err := q.ListFirewallHosts(ctx)
		if err != nil {
			return err
		}
		for _, h := range in.Hosts {
			if !slices.ContainsFunc(hosts, func(x db.FirewallHost) bool { return x.ID == h }) {
				return fmt.Errorf("%w: unknown host %s", ErrInvalid, h)
			}
		}
		old := map[string]db.ScriptEnv{}
		if id == "" {
			id = ulid.Make().String()
			if err := q.InsertScript(ctx, db.InsertScriptParams{ID: id, Slug: slugOf(in.Name), Name: in.Name, Description: in.Description,
				Body: in.Body, Enabled: b2i(in.Enabled), Schedule: in.Schedule, TimeoutSeconds: int64(in.TimeoutSeconds),
				AllHosts: b2i(in.AllHosts), CreatedAt: now, UpdatedAt: now}); err != nil {
				return uniqueName(err)
			}
		} else {
			cur, err := s.load(ctx, q, id)
			if err != nil {
				return err
			}
			before = cur.targets(hosts)
			for _, e := range cur.Env {
				old[e.Name] = e
			}
			// The slug stays: it names the files on the hosts, and renaming would leave the old ones behind.
			if err := q.UpdateScript(ctx, db.UpdateScriptParams{ID: id, Name: in.Name, Description: in.Description, Body: in.Body,
				Enabled: b2i(in.Enabled), Schedule: in.Schedule, TimeoutSeconds: int64(in.TimeoutSeconds),
				AllHosts: b2i(in.AllHosts), UpdatedAt: now}); err != nil {
				return uniqueName(err)
			}
		}
		if err := q.DeleteScriptTargets(ctx, id); err != nil {
			return err
		}
		for _, h := range in.Hosts {
			if err := q.InsertScriptTarget(ctx, db.InsertScriptTargetParams{ScriptID: id, HostID: h}); err != nil {
				return err
			}
		}
		if err := s.putEnv(ctx, q, id, in.Env, old); err != nil {
			return err
		}
		return s.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "script.saved", Target: "script:" + id, IP: ip,
			Outcome: audit.Success, Detail: map[string]any{"name": in.Name, "enabled": in.Enabled, "schedule": in.Schedule,
				"all_hosts": in.AllHosts, "hosts": in.Hosts, "env": envNames(in.Env)}})
	})
	if err != nil {
		return Script{}, err
	}
	sc, err := s.Get(ctx, id)
	if err != nil {
		return Script{}, err
	}
	hosts, err := s.st.R.ListFirewallHosts(ctx)
	if err != nil {
		return sc, err
	}
	s.replan(ctx, union(before, sc.targets(hosts)), actor)
	return sc, nil
}

func uniqueName(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return fmt.Errorf("%w: another script has that name", ErrInvalid)
	}
	return err
}

func envNames(env []EnvVar) []string {
	out := make([]string, len(env))
	for i, e := range env {
		out[i] = e.Name
	}
	return out
}

// putEnv replaces the variables. Secrets go to the vault; a secret sent without a value keeps the stored one.
func (s *Service) putEnv(ctx context.Context, q *db.Queries, id string, env []EnvVar, old map[string]db.ScriptEnv) error {
	if err := q.DeleteScriptEnv(ctx, id); err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, e := range env {
		row := db.InsertScriptEnvParams{ScriptID: id, Name: e.Name}
		switch {
		case !e.Secret:
			row.Value = e.Value.Reveal()
		case e.Value.IsZero():
			o, ok := old[e.Name]
			if !ok || !o.SecretID.Valid {
				return fmt.Errorf("%w: secret %s needs a value", ErrInvalid, e.Name)
			}
			row.SecretID, keep[o.SecretID.String] = o.SecretID, true
		default:
			p, err := s.vault.Seal(vault.Service, "script:"+id+":"+e.Name, e.Value)
			if err != nil {
				return err
			}
			if err := q.InsertSecret(ctx, p); err != nil {
				return err
			}
			row.SecretID = sql.NullString{String: p.ID, Valid: true}
		}
		if err := q.InsertScriptEnv(ctx, row); err != nil {
			return err
		}
	}
	for _, o := range old { // replaced or removed secrets leave the vault
		if o.SecretID.Valid && !keep[o.SecretID.String] {
			if err := q.DeleteSecret(ctx, o.SecretID.String); err != nil {
				return err
			}
		}
	}
	return nil
}

// Delete removes a script; the next plan of each host it ran on removes its files and timer.
func (s *Service) Delete(ctx context.Context, actor, ip, id string) error {
	hosts, err := s.st.R.ListFirewallHosts(ctx)
	if err != nil {
		return err
	}
	var targets []string
	err = s.st.Tx(ctx, func(q *db.Queries) error {
		cur, err := s.load(ctx, q, id)
		if err != nil {
			return err
		}
		targets = cur.targets(hosts)
		if err := q.DeleteScript(ctx, id); err != nil {
			return err
		}
		for _, e := range cur.Env {
			if e.SecretID.Valid {
				if err := q.DeleteSecret(ctx, e.SecretID.String); err != nil {
					return err
				}
			}
		}
		return s.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "script.deleted", Target: "script:" + id, IP: ip,
			Outcome: audit.Success, Detail: map[string]any{"name": cur.Name}})
	})
	if err != nil {
		return err
	}
	s.replan(ctx, targets, actor)
	return nil
}

func (s *Service) replan(ctx context.Context, hostIDs []string, actor string) {
	if s.planHosts != nil && len(hostIDs) > 0 {
		s.planHosts(ctx, hostIDs, actor)
	}
}

func union(a, b []string) []string {
	out := slices.Concat(a, b)
	slices.Sort(out)
	return slices.Compact(out)
}

// Get loads one script.
func (s *Service) Get(ctx context.Context, id string) (Script, error) {
	return s.load(ctx, s.st.R, id)
}

func (s *Service) load(ctx context.Context, q *db.Queries, id string) (Script, error) {
	row, err := q.GetScript(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Script{}, ErrNotFound
	}
	if err != nil {
		return Script{}, err
	}
	hosts, err := q.ListScriptTargets(ctx, id)
	if err != nil {
		return Script{}, err
	}
	env, err := q.ListScriptEnv(ctx, id)
	if err != nil {
		return Script{}, err
	}
	return Script{Script: row, Hosts: hosts, Env: env}, nil
}

// List loads every script.
func (s *Service) List(ctx context.Context) ([]Script, error) {
	rows, err := s.st.R.ListScripts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Script, 0, len(rows))
	for _, r := range rows {
		sc, err := s.load(ctx, s.st.R, r.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, nil
}

// Eligible reports whether Hashi can reach a firewall host over SSH, so scripts can live there.
func Eligible(h db.FirewallHost) bool { return h.SshHost != "" && len(h.HostKey) > 0 }

// targets are the hosts the script belongs on: every eligible host, or the listed ones (none listed = none, 09 §1).
func (sc Script) targets(hosts []db.FirewallHost) []string {
	var out []string
	for _, h := range hosts {
		if Eligible(h) && (sc.AllHosts == 1 || slices.Contains(sc.Hosts, h.ID)) {
			out = append(out, h.ID)
		}
	}
	return out
}

// Targets are the host ids the script runs on now.
func (s *Service) Targets(ctx context.Context, sc Script) ([]string, error) {
	hosts, err := s.st.R.ListFirewallHosts(ctx)
	if err != nil {
		return nil, err
	}
	return sc.targets(hosts), nil
}

// desired is what one host should hold: every enabled script that targets it, with its variables opened from the vault.
// It also returns the secret values, so run output can be redacted.
func (s *Service) desired(ctx context.Context, hostID string) ([]Desired, []string, error) {
	all, err := s.List(ctx)
	if err != nil {
		return nil, nil, err
	}
	hosts, err := s.st.R.ListFirewallHosts(ctx)
	if err != nil {
		return nil, nil, err
	}
	var out []Desired
	var secretVals []string
	for _, sc := range all {
		if sc.Enabled == 0 || !slices.Contains(sc.targets(hosts), hostID) {
			continue
		}
		d := Desired{Slug: sc.Slug, Body: sc.Body, Timeout: int(sc.TimeoutSeconds), Env: map[string]string{}}
		if sc.Schedule != "" {
			p, err := ParseCron(sc.Schedule)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: stored schedule: %w", sc.Name, err)
			}
			d.Calendar = p.OnCalendar()
		}
		for _, e := range sc.Env {
			if !e.SecretID.Valid {
				d.Env[e.Name] = e.Value
				continue
			}
			rec, err := s.st.R.GetSecret(ctx, e.SecretID.String)
			if err != nil {
				return nil, nil, err
			}
			v, err := s.vault.Open(rec)
			if err != nil { // a locked vault: the script cannot be written or run (09 §1)
				return nil, nil, fmt.Errorf("%s: secret %s is locked: %w", sc.Name, e.Name, err)
			}
			d.Env[e.Name] = v.Reveal()
			secretVals = append(secretVals, v.Reveal())
		}
		out = append(out, d)
	}
	return out, secretVals, nil
}
