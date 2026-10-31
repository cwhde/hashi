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
	"net"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
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
)

// Candidate is one address an agent reports.
type Candidate struct {
	Interface string `json:"interface"`
	Address   string `json:"address"`
}

// Beat is a heartbeat's content.
type Beat struct {
	Version, Hostname, SelectedInterface string
	// InDocker: the agent runs in a container, so its install type follows (08 §1).
	InDocker bool
	// IntervalSeconds is how often the agent reports; staleness is measured against it (08 §2).
	IntervalSeconds int
	// ReportedPublic is what the agent says its public address is. Used only for an agent the admin trusts for it,
	// and never over the address its connection came from.
	ReportedPublic string
	Candidates     []Candidate
}

// Reply is what an agent is told after a heartbeat.
type Reply struct {
	CheckPublicIP   bool
	IntervalSeconds int // the admin's default; an agent with its own interval keeps it
}

// Service keeps agents.
type Service struct {
	st     *store.Store
	pepper []byte
	al     *audit.Log
	now    func() time.Time
	mu     sync.Mutex
	timer  *time.Timer
	// debounce groups address changes before consumers hear of them (05a §4.2).
	debounce time.Duration
	// changed runs after a heartbeat with new addresses: it recompiles the sets, which reach their consumers (08 §4).
	changed func(ctx context.Context)
}

// New returns the service.
func New(st *store.Store, v *vault.Vault, al *audit.Log) *Service {
	return &Service{st: st, pepper: v.Operational("pulse-token-pepper"), al: al, now: time.Now, debounce: 2 * time.Second}
}

// OnChange sets what runs after an address change.
func (s *Service) OnChange(f func(ctx context.Context)) { s.changed = f }

func hashToken(pepper []byte, token string) []byte {
	m := hmac.New(sha256.New, pepper)
	m.Write([]byte(token))
	return m.Sum(nil)
}

func (s *Service) newToken(ctx context.Context) (secrets.Value, []byte, error) {
	raw := make([]byte, 32) // 256 bits (08 §3)
	if _, err := rand.Read(raw); err != nil {
		return secrets.Value{}, nil, err
	}
	token := "hp_" + hex.EncodeToString(raw)
	return secrets.New(token), hashToken(s.pepper, token), nil
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
	sets, err := s.st.R.ListIPSetsUsingAgent(ctx, sql.NullString{String: id, Valid: true})
	if err != nil {
		return err
	}
	if len(sets) > 0 {
		names := make([]string, 0, len(sets))
		for _, x := range sets {
			names = append(names, x.Name)
		}
		return fmt.Errorf("%w: IP sets %s use it", ErrInUse, strings.Join(names, ", "))
	}
	return s.st.Q.DeletePulseAgent(ctx, id)
}

// SetDNS changes an agent's name and its internal DNS entry.
func (s *Service) SetDNS(ctx context.Context, id, name string, enabled bool, dnsName string, trustReported bool, reachPort int) (db.PulseAgent, error) {
	if reachPort < 0 || reachPort > 65535 {
		return db.PulseAgent{}, fmt.Errorf("%w: port must be between 0 and 65535", ErrInvalid)
	}
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
	var trust int64
	if trustReported {
		trust = 1
	}
	before, err := s.get(ctx, id)
	if err != nil {
		return db.PulseAgent{}, err
	}
	if err := s.st.Q.SetPulseDNS(ctx, db.SetPulseDNSParams{Name: name, DnsEnabled: on, DnsName: dnsName, TrustReportedIp: trust, ReachPort: int64(reachPort), UpdatedAt: s.now().UnixMilli(), ID: id}); err != nil {
		return db.PulseAgent{}, err
	}
	// Taking trust away also takes away what the agent said: its address must leave the sets using it.
	if before.TrustReportedIp == 1 && !trustReported && before.PublicIpSource == SourceReported {
		if err := s.st.Q.ClearPulseReportedIP(ctx, id); err != nil {
			return db.PulseAgent{}, err
		}
		s.notify(ctx)
	}
	return s.get(ctx, id)
}

// Heartbeat checks the token and records what the agent reports. source is the address it came from, via the client-IP
// resolver. The answer tells the agent what Hashi wants from it.
func (s *Service) Heartbeat(ctx context.Context, id, token string, b Beat, source netip.Addr) (Reply, error) {
	row, err := s.st.R.GetPulseAgent(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Reply{}, ErrToken // the same answer as a wrong token: an unknown ID tells a caller nothing
	}
	if err != nil {
		return Reply{}, err
	}
	if !hmac.Equal(hashToken(s.pepper, token), row.TokenHash) || row.RevokedAt.Valid {
		return Reply{}, ErrToken
	}
	now := s.now()
	if row.LastSeenAt.Valid && now.Sub(time.UnixMilli(row.LastSeenAt.Int64)) < minBeatGap {
		return Reply{}, ErrRate
	}
	cands, err := clean(b.Candidates)
	if err != nil {
		return Reply{}, err
	}
	selected := selectIP(cands, b.SelectedInterface, row.SelectedIp)
	public, origin := publicAddress(row, source, b.ReportedPublic)
	install := row.InstallType
	if b.InDocker {
		install = "docker"
	}
	interval := row.IntervalSeconds
	if b.IntervalSeconds >= 10 && b.IntervalSeconds <= 86400 {
		interval = int64(b.IntervalSeconds)
	}
	raw, err := json.Marshal(cands)
	if err != nil {
		return Reply{}, err
	}
	changed := public != row.PublicIp || selected != row.SelectedIp || string(raw) != row.CandidatesJson
	if err := s.st.Q.SetPulseHeartbeat(ctx, db.SetPulseHeartbeatParams{LastSeenAt: sql.NullInt64{Int64: now.UnixMilli(), Valid: true}, PublicIp: public, PublicIpSource: origin, CandidatesJson: string(raw),
		SelectedIp: selected, Hostname: trim(b.Hostname, 253), Version: trim(b.Version, 64), IntervalSeconds: interval, InstallType: install, UpdatedAt: now.UnixMilli(), ID: id}); err != nil {
		return Reply{}, err
	}
	if changed {
		if s.al != nil {
			if err := s.al.Record(ctx, s.st.Q, audit.Event{Actor: audit.System, Action: "pulse.address_changed", Target: "pulse_agent:" + id, Outcome: audit.Success,
				Detail: map[string]any{"public": public, "public_source": origin, "selected": selected}}); err != nil {
				return Reply{}, err
			}
		}
		s.notify(ctx)
	}
	// Outside services hear from an agent only when it is trusted and this connection was not public: an address learned
	// from an earlier connection says nothing about where the agent is now.
	cfg, err := settings.Get[settings.Pulse](ctx, s.st.R)
	if err != nil {
		return Reply{}, err
	}
	return Reply{CheckPublicIP: row.TrustReportedIp == 1 && !publicRoutable(source.Unmap()), IntervalSeconds: cfg.HeartbeatIntervalSeconds}, nil
}

// Where a public address came from.
const (
	SourceConnection = "connection"
	SourceReported   = "reported"
)

// carrierShared is shared carrier space (NetBird, CGNAT): private in use, though not by RFC 1918.
var carrierShared = netip.MustParsePrefix("100.64.0.0/10")

// publicRoutable is an address that can be someone's public one.
func publicRoutable(a netip.Addr) bool {
	return a.IsValid() && a.IsGlobalUnicast() && !a.IsPrivate() && !carrierShared.Contains(a)
}

// publicAddress picks an agent's public address. The connection's source is proof and always wins. An agent's own report
// counts only if the admin trusts this agent for it. Otherwise the last address stays: a private path says nothing new.
func publicAddress(row db.PulseAgent, source netip.Addr, reported string) (addr, origin string) {
	if s := source.Unmap(); publicRoutable(s) {
		return s.String(), SourceConnection
	}
	if row.TrustReportedIp == 1 {
		if a, err := netip.ParseAddr(reported); err == nil && publicRoutable(a.Unmap()) {
			return a.Unmap().String(), SourceReported
		}
	}
	return row.PublicIp, row.PublicIpSource
}

// notify tells the sets after the debounce, in the background: a heartbeat must not wait for firewall and proxy plans,
// and several agents changing at once are one recompile.
func (s *Service) notify(ctx context.Context) {
	if s.changed == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	base := context.WithoutCancel(ctx)
	s.timer = time.AfterFunc(s.debounce, func() {
		ctx, cancel := context.WithTimeout(base, 5*time.Minute)
		defer cancel()
		s.changed(ctx)
	})
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
		// Candidates are private by definition (08 §1): a public one would enter sets through the `internal` scope, past
		// the opt-in that guards a self-reported public address.
		if !a.IsPrivate() && !carrierShared.Contains(a) {
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
func Status(r db.PulseAgent, now time.Time, staleFactor int) string {
	switch {
	case r.RevokedAt.Valid:
		return "revoked"
	case !r.LastSeenAt.Valid:
		return "waiting"
	case now.Sub(time.UnixMilli(r.LastSeenAt.Int64)) > time.Duration(r.IntervalSeconds*int64(staleFactor))*time.Second:
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
	addrs := addrsOf(r, scope, selector)
	c, err := settings.Get[settings.Pulse](ctx, q)
	if err != nil {
		return nil, err
	}
	// A stale agent keeps resolving to its last addresses; its set shows why they may be old (05a §2).
	if Status(r, s.now(), c.StaleFactor) == "stale" {
		return addrs, ipsets.Stale{What: "the Pulse agent " + r.Name + " is stale"}
	}
	return addrs, nil
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

// Reachability job (08 §2).
const (
	ReachJobKind  = "pulse.reachability"
	ReachInterval = time.Minute
)

// CheckReach tries each agent's selected address on its configured port, so the page can tell "Hashi can reach this"
// from "the agent only reports it". Agents without a port are left alone.
func (s *Service) CheckReach(ctx context.Context) (string, error) {
	rows, err := s.st.R.ListPulseAgents(ctx)
	if err != nil {
		return "", err
	}
	checked := 0
	for _, r := range rows {
		if r.ReachPort == 0 || r.SelectedIp == "" || r.RevokedAt.Valid {
			continue
		}
		dctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		conn, err := (&net.Dialer{}).DialContext(dctx, "tcp", net.JoinHostPort(r.SelectedIp, strconv.FormatInt(r.ReachPort, 10)))
		cancel()
		ok := int64(0)
		if err == nil {
			_ = conn.Close()
			ok = 1
		}
		if err := s.st.Q.SetPulseReach(ctx, db.SetPulseReachParams{ReachOk: sql.NullInt64{Int64: ok, Valid: true}, ID: r.ID}); err != nil {
			return "", err
		}
		checked++
	}
	return strconv.Itoa(checked) + " agents checked", nil
}

var slugBad = regexp.MustCompile(`[^a-z0-9-]+`)
var slugDashes = regexp.MustCompile(`-+`)

// Slug is an agent's DNS label (03 §9).
func Slug(name string) string {
	s := slugBad.ReplaceAllString(strings.ToLower(name), "-")
	s = strings.Trim(slugDashes.ReplaceAllString(s, "-"), "-")
	return trim(s, 63)
}

// AgentName is one agent's internal DNS name and its state, for the DNS page (03 §9).
type AgentName struct {
	AgentID, Agent, Domain, Answer string
	Status                         string
	Collision                      bool   // another agent has this name: this one needs an override
	Problem                        string // why it has no rewrite, in words
	Wanted                         bool
}

// Names decides every agent's name. Names that collide go to the first agent only; the others are shown, not hidden.
// A stale agent keeps its rewrite unless the admin chose to remove it after a while (03 §9).
func (s *Service) Names(ctx context.Context) ([]AgentName, error) {
	c, err := settings.Get[settings.AgentDNS](ctx, s.st.R)
	if err != nil {
		return nil, err
	}
	p, err := settings.Get[settings.Pulse](ctx, s.st.R)
	if err != nil {
		return nil, err
	}
	rows, err := s.st.R.ListPulseAgents(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now()
	seen := map[string]bool{}
	out := make([]AgentName, 0, len(rows))
	for _, r := range rows {
		n := AgentName{AgentID: r.ID, Agent: r.Name, Status: Status(r, now, p.StaleFactor)}
		label := r.DnsName
		if label == "" {
			label = r.Name
		}
		label = Slug(label)
		switch {
		case r.DnsEnabled == 0:
			n.Problem = "turned off for this agent"
		case r.RevokedAt.Valid:
			n.Problem = "the agent is revoked"
		case label == "":
			n.Problem = "its name has no usable letters; set a DNS name"
		case seen[label]:
			n.Collision, n.Problem = true, "another agent has this name; set a DNS name"
		}
		if label != "" {
			n.Domain = label + "." + c.Domain
		}
		n.Answer = answerFor(r, c.IPMode)
		if n.Problem == "" && n.Answer == "" {
			n.Problem = "no address reported yet"
		}
		if n.Problem == "" && n.Status == "stale" && c.RemoveStaleAfterHours > 0 && r.LastSeenAt.Valid &&
			now.Sub(time.UnixMilli(r.LastSeenAt.Int64)) > time.Duration(c.RemoveStaleAfterHours)*time.Hour {
			n.Problem = "removed: stale for too long"
		}
		if n.Problem == "" {
			seen[label], n.Wanted = true, c.Enabled
		}
		out = append(out, n)
	}
	return out, nil
}

// answerFor picks the address a name answers with (03 §9): selected, the first private candidate, or public.
func answerFor(r db.PulseAgent, mode string) string {
	switch mode {
	case settings.AgentIPPublic:
		return r.PublicIp
	case settings.AgentIPPrivate:
		cands := Candidates(r)
		if a, err := netip.ParseAddr(r.SelectedIp); err == nil && a.IsPrivate() {
			return r.SelectedIp
		}
		for _, x := range cands {
			if a, err := netip.ParseAddr(x.Address); err == nil && a.IsPrivate() {
				return x.Address
			}
		}
		return ""
	}
	return r.SelectedIp
}

// Rewrites lists the rewrites agent DNS wants.
func (s *Service) Rewrites(ctx context.Context) ([]adguard.Desired, error) {
	names, err := s.Names(ctx)
	if err != nil {
		return nil, err
	}
	var out []adguard.Desired
	for _, n := range names {
		if n.Wanted {
			out = append(out, adguard.Desired{Domain: n.Domain, Answer: n.Answer, Origin: "agent " + n.AgentID})
		}
	}
	return out, nil
}

// OriginGone reports whether a rewrite came from an agent that was explicitly deleted: removing it needs no second
// approval (03 §9).
func (s *Service) OriginGone(ctx context.Context, origin string) bool {
	id, ok := strings.CutPrefix(origin, "agent ")
	if !ok {
		return false
	}
	_, err := s.st.R.GetPulseAgent(ctx, id)
	return errors.Is(err, sql.ErrNoRows)
}
