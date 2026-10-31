// Package pulse keeps Pulse agents (08): tokens, heartbeats, and the addresses `pulse` IP items resolve to. An agent's
// token can do one thing, send that agent's heartbeat.
package pulse

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	_ "embed" // the install script
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/adguard"
	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// InstallScript is served to hosts installing an agent. It holds nothing secret.
//
//go:embed install.sh
var InstallScript []byte

// Errors the API maps.
var (
	ErrNotFound = errors.New("no such Pulse agent")
	ErrInvalid  = errors.New("invalid")
	ErrToken    = errors.New("invalid agent token")
	ErrRate     = errors.New("too many heartbeats")
	ErrInUse    = errors.New("the agent is in use")
)

// Limits on a heartbeat (08 §3).
const (
	maxCandidates = 64
	minBeatGap    = 2 * time.Second
	StaleFactor   = 3 // stale after this many missed intervals (08 §2)
)

// Candidate is one address an agent reports.
type Candidate struct {
	Interface string `json:"interface"`
	Address   string `json:"address"`
}

// Beat is a heartbeat's content.
type Beat struct {
	Version, Hostname, SelectedInterface string
	Candidates                           []Candidate
}

// Service keeps agents.
type Service struct {
	st    *store.Store
	vault *vault.Vault
	al    *audit.Log
	now   func() time.Time
	mu    sync.Mutex
	// changed runs after a heartbeat with new addresses: it recompiles the sets, which reach their consumers (08 §4).
	changed func(ctx context.Context)
}

// New returns the service.
func New(st *store.Store, v *vault.Vault, al *audit.Log) *Service {
	return &Service{st: st, vault: v, al: al, now: time.Now}
}

// OnChange sets what runs after an address change.
func (s *Service) OnChange(f func(ctx context.Context)) { s.changed = f }

// pepper returns the HMAC key, creating it in the vault on first use.
func (s *Service) pepper(ctx context.Context) (secrets.Value, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, err := s.st.R.GetPulseConfig(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return secrets.Value{}, err
		}
		v := secrets.New(hex.EncodeToString(raw))
		p, err := s.vault.Seal(vault.Service, "pulse:pepper", v)
		if err != nil {
			return secrets.Value{}, err
		}
		return v, s.st.Tx(ctx, func(q *db.Queries) error {
			if err := q.InsertSecret(ctx, p); err != nil {
				return err
			}
			return q.InsertPulseConfig(ctx, p.ID)
		})
	}
	if err != nil {
		return secrets.Value{}, err
	}
	rec, err := s.st.R.GetSecret(ctx, cfg.PepperSecretID)
	if err != nil {
		return secrets.Value{}, err
	}
	return s.vault.Open(rec)
}

func hashToken(pepper secrets.Value, token string) []byte {
	m := hmac.New(sha256.New, []byte(pepper.Reveal()))
	m.Write([]byte(token))
	return m.Sum(nil)
}

func (s *Service) newToken(ctx context.Context) (secrets.Value, []byte, error) {
	raw := make([]byte, 32) // 256 bits (08 §3)
	if _, err := rand.Read(raw); err != nil {
		return secrets.Value{}, nil, err
	}
	token := "hp_" + hex.EncodeToString(raw)
	pepper, err := s.pepper(ctx)
	if err != nil {
		return secrets.Value{}, nil, err
	}
	return secrets.New(token), hashToken(pepper, token), nil
}

// Create adds an agent and returns its token, shown once.
func (s *Service) Create(ctx context.Context, name, install string) (db.PulseAgent, secrets.Value, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return db.PulseAgent{}, secrets.Value{}, fmt.Errorf("%w: a name is required", ErrInvalid)
	}
	if install != "docker" {
		install = "linux"
	}
	token, hash, err := s.newToken(ctx)
	if err != nil {
		return db.PulseAgent{}, secrets.Value{}, err
	}
	id, now := ulid.Make().String(), s.now().UnixMilli()
	if err := s.st.Q.InsertPulseAgent(ctx, db.InsertPulseAgentParams{ID: id, Name: name, TokenHash: hash, InstallType: install, CreatedAt: now, UpdatedAt: now}); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return db.PulseAgent{}, secrets.Value{}, fmt.Errorf("%w: an agent named %s exists", ErrInvalid, name)
		}
		return db.PulseAgent{}, secrets.Value{}, err
	}
	row, err := s.st.R.GetPulseAgent(ctx, id)
	return row, token, err
}

func (s *Service) get(ctx context.Context, id string) (db.PulseAgent, error) {
	row, err := s.st.R.GetPulseAgent(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return row, ErrNotFound
	}
	return row, err
}

// Rotate replaces the token; the old one stops working. A revoked agent becomes usable again.
func (s *Service) Rotate(ctx context.Context, id string) (db.PulseAgent, secrets.Value, error) {
	if _, err := s.get(ctx, id); err != nil {
		return db.PulseAgent{}, secrets.Value{}, err
	}
	token, hash, err := s.newToken(ctx)
	if err != nil {
		return db.PulseAgent{}, secrets.Value{}, err
	}
	if err := s.st.Q.SetPulseToken(ctx, db.SetPulseTokenParams{TokenHash: hash, UpdatedAt: s.now().UnixMilli(), ID: id}); err != nil {
		return db.PulseAgent{}, secrets.Value{}, err
	}
	s.notify(ctx)
	row, err := s.get(ctx, id)
	return row, token, err
}

// Revoke stops the token. A revoked agent resolves to nothing (05a §2), so its sets change.
func (s *Service) Revoke(ctx context.Context, id string) (db.PulseAgent, error) {
	if _, err := s.get(ctx, id); err != nil {
		return db.PulseAgent{}, err
	}
	now := s.now().UnixMilli()
	if err := s.st.Q.RevokePulseAgent(ctx, db.RevokePulseAgentParams{RevokedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: id}); err != nil {
		return db.PulseAgent{}, err
	}
	s.notify(ctx)
	return s.get(ctx, id)
}

// Delete removes an agent no set references (05a §2).
func (s *Service) Delete(ctx context.Context, id string) error {
	if _, err := s.get(ctx, id); err != nil {
		return err
	}
	names, err := s.st.R.ListIPSetsUsingAgent(ctx, sql.NullString{String: id, Valid: true})
	if err != nil {
		return err
	}
	if len(names) > 0 {
		return fmt.Errorf("%w: IP sets %s use it", ErrInUse, strings.Join(names, ", "))
	}
	return s.st.Q.DeletePulseAgent(ctx, id)
}

// SetDNS changes an agent's name and its internal DNS entry.
func (s *Service) SetDNS(ctx context.Context, id, name string, enabled bool, dnsName string) (db.PulseAgent, error) {
	if _, err := s.get(ctx, id); err != nil {
		return db.PulseAgent{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return db.PulseAgent{}, fmt.Errorf("%w: a name is required", ErrInvalid)
	}
	var on int64
	if enabled {
		on = 1
	}
	if err := s.st.Q.SetPulseDNS(ctx, db.SetPulseDNSParams{Name: name, DnsEnabled: on, DnsName: dnsName, UpdatedAt: s.now().UnixMilli(), ID: id}); err != nil {
		return db.PulseAgent{}, err
	}
	return s.get(ctx, id)
}

// Heartbeat checks the token and records what the agent reports. source is the address it came from, via the client-IP resolver.
func (s *Service) Heartbeat(ctx context.Context, id, token string, b Beat, source netip.Addr) error {
	row, err := s.st.R.GetPulseAgent(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrToken // the same answer as a wrong token: an unknown ID tells a caller nothing
	}
	if err != nil {
		return err
	}
	pepper, err := s.pepper(ctx)
	if err != nil {
		return err
	}
	if !hmac.Equal(hashToken(pepper, token), row.TokenHash) || row.RevokedAt.Valid {
		return ErrToken
	}
	now := s.now()
	if row.LastSeenAt.Valid && now.Sub(time.UnixMilli(row.LastSeenAt.Int64)) < minBeatGap {
		return ErrRate
	}
	cands, err := clean(b.Candidates)
	if err != nil {
		return err
	}
	selected := selectIP(cands, b.SelectedInterface, row.SelectedIp)
	public := ""
	if source.IsValid() && source.Unmap().IsGlobalUnicast() && !source.Unmap().IsPrivate() {
		public = source.Unmap().String()
	} else if row.PublicIp != "" {
		public = row.PublicIp // reached over a private path: the last public address stays true enough
	}
	raw, err := json.Marshal(cands)
	if err != nil {
		return err
	}
	changed := public != row.PublicIp || selected != row.SelectedIp || string(raw) != row.CandidatesJson
	if err := s.st.Q.SetPulseHeartbeat(ctx, db.SetPulseHeartbeatParams{LastSeenAt: sql.NullInt64{Int64: now.UnixMilli(), Valid: true}, PublicIp: public, CandidatesJson: string(raw),
		SelectedIp: selected, Hostname: trim(b.Hostname, 253), Version: trim(b.Version, 64), UpdatedAt: now.UnixMilli(), ID: id}); err != nil {
		return err
	}
	if changed {
		if s.al != nil {
			if err := s.al.Record(ctx, s.st.Q, audit.Event{Actor: audit.System, Action: "pulse.address_changed", Target: "pulse_agent:" + id, Outcome: audit.Success,
				Detail: map[string]any{"public": public, "selected": selected}}); err != nil {
				return err
			}
		}
		s.notify(ctx)
	}
	return nil
}

// notify runs in the background: a heartbeat must not wait for firewall and proxy plans.
func (s *Service) notify(ctx context.Context) {
	if s.changed == nil {
		return
	}
	go func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		s.changed(ctx)
	}(context.WithoutCancel(ctx))
}

func trim(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

var ifaceName = regexp.MustCompile(`^[a-zA-Z0-9._@:-]{0,32}$`)

func clean(in []Candidate) ([]Candidate, error) {
	if len(in) > maxCandidates {
		return nil, fmt.Errorf("%w: at most %d addresses", ErrInvalid, maxCandidates)
	}
	out := make([]Candidate, 0, len(in))
	for _, c := range in {
		a, err := netip.ParseAddr(c.Address)
		if err != nil || a.Zone() != "" {
			return nil, fmt.Errorf("%w: %q is not an address", ErrInvalid, c.Address)
		}
		if !ifaceName.MatchString(c.Interface) {
			return nil, fmt.Errorf("%w: bad interface name", ErrInvalid)
		}
		a = a.Unmap()
		if a.IsLoopback() || a.IsUnspecified() || a.IsLinkLocalUnicast() || a.IsMulticast() {
			continue
		}
		out = append(out, Candidate{Interface: c.Interface, Address: a.String()})
	}
	return out, nil
}

// selectIP keeps the current choice while it is still reported, so an address does not flap between interfaces. An
// interface the admin named wins; otherwise the first private IPv4, then anything.
func selectIP(cands []Candidate, iface, current string) string {
	if iface != "" {
		for _, c := range cands {
			if c.Interface == iface {
				return c.Address
			}
		}
	}
	if slices.ContainsFunc(cands, func(c Candidate) bool { return c.Address == current }) {
		return current
	}
	for _, c := range cands {
		if a := netip.MustParseAddr(c.Address); a.Is4() && a.IsPrivate() {
			return c.Address
		}
	}
	if len(cands) > 0 {
		return cands[0].Address
	}
	return ""
}

// Status of an agent (08 §2).
func Status(r db.PulseAgent, now time.Time) string {
	switch {
	case r.RevokedAt.Valid:
		return "revoked"
	case !r.LastSeenAt.Valid:
		return "waiting"
	case now.Sub(time.UnixMilli(r.LastSeenAt.Int64)) > time.Duration(r.IntervalSeconds*StaleFactor)*time.Second:
		return "stale"
	}
	return "fresh"
}

// Candidates decodes a row's reported addresses.
func Candidates(r db.PulseAgent) []Candidate {
	var c []Candidate
	_ = json.Unmarshal([]byte(r.CandidatesJson), &c) // written by Heartbeat
	return c
}

// Addrs answers a `pulse` IP item (05a §2): a stale agent keeps its last addresses, a revoked or missing one has none.
func (s *Service) Addrs(ctx context.Context, q *db.Queries, id, scope, selector string) ([]netip.Addr, error) {
	r, err := q.GetPulseAgent(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if r.RevokedAt.Valid {
		return nil, errors.New("the Pulse agent is revoked")
	}
	return addrsOf(r, scope, selector), nil
}

func addrsOf(r db.PulseAgent, scope, selector string) []netip.Addr {
	var out []netip.Addr
	if (scope == ipsets.ScopePublic || scope == ipsets.ScopeAll) && r.PublicIp != "" {
		if a, err := netip.ParseAddr(r.PublicIp); err == nil {
			out = append(out, a)
		}
	}
	if scope == ipsets.ScopeInternal || scope == ipsets.ScopeAll {
		match := func(c Candidate) bool {
			if selector == "" {
				return true
			}
			if p, err := netip.ParsePrefix(selector); err == nil {
				a, err := netip.ParseAddr(c.Address)
				return err == nil && p.Contains(a)
			}
			return c.Interface == selector
		}
		cands := Candidates(r)
		var rest []netip.Addr
		for _, c := range cands {
			a, err := netip.ParseAddr(c.Address)
			if err != nil || !match(c) {
				continue
			}
			if c.Address == r.SelectedIp {
				out = append(out, a) // selected one first (05a §2)
			} else {
				rest = append(rest, a)
			}
		}
		out = append(out, rest...)
	}
	return out
}

var slugBad = regexp.MustCompile(`[^a-z0-9-]+`)
var slugDashes = regexp.MustCompile(`-+`)

// Slug is an agent's DNS label (03 §9).
func Slug(name string) string {
	s := slugBad.ReplaceAllString(strings.ToLower(name), "-")
	s = strings.Trim(slugDashes.ReplaceAllString(s, "-"), "-")
	return trim(s, 63)
}

// Rewrites lists agent DNS entries (03 §9): only for fresh or stale agents, never revoked ones. A stale agent keeps its
// last rewrite. Names that collide are left out for all but the first: they need an override.
func (s *Service) Rewrites(ctx context.Context) ([]adguard.Desired, error) {
	c, err := settings.Get[settings.AgentDNS](ctx, s.st.R)
	if err != nil || !c.Enabled {
		return nil, err
	}
	rows, err := s.st.R.ListPulseAgents(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []adguard.Desired
	for _, r := range rows {
		if r.DnsEnabled == 0 || r.RevokedAt.Valid {
			continue
		}
		label := r.DnsName
		if label == "" {
			label = r.Name
		}
		label = Slug(label)
		if label == "" || seen[label] {
			continue
		}
		answer := r.SelectedIp
		if c.IPMode == settings.AgentIPPublic {
			answer = r.PublicIp
		}
		if answer == "" {
			continue
		}
		seen[label] = true
		out = append(out, adguard.Desired{Domain: label + "." + c.Domain, Answer: answer, Origin: "agent " + r.ID})
	}
	return out, nil
}
