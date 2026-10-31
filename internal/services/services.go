// Package services is the service model (04 §1): what Hashi publishes through every Traefik host. It stores and
// validates services; the traefik package renders them and the sync engine applies the result.
package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Kinds: the HTTP family (served on websecure) and the L4 streams (one entry point per public port, 04 §1.2).
const (
	KindTCP   = "tcp"
	KindUDP   = "udp"
	KindHTTP  = "http"
	KindHTTPS = "https"
	KindH2C   = "h2c"
)

// Domain modes.
const (
	ModeRoot      = "root"
	ModeSubdomain = "subdomain"
	ModeCustom    = "custom"
)

// Origins (invariant 9).
const (
	OriginSystem = "system"
	OriginUser   = "user"
)

// Path match types and rewrite modes (04 §1.1).
const (
	MatchPrefix = "prefix"
	MatchExact  = "exact"
	MatchRegex  = "regex"

	RewriteNone          = "none"
	RewriteReplacePrefix = "replace_prefix"
	RewriteExact         = "exact"
	RewriteRegex         = "regex"
	RewriteStripPrefix   = "strip_prefix"
)

// Minimum TLS versions served to clients.
const (
	TLS12 = "VersionTLS12"
	TLS13 = "VersionTLS13"
)

// OwnerSetup owns the Hashi admin system service (02 §1.6).
const OwnerSetup = "setup"

// Errors the API maps.
var (
	ErrNotFound = errors.New("service not found")
	// ErrSystem: system services change only through their owning workflow (invariant 10).
	ErrSystem = errors.New("this is a system service: change it through its owning setting")
	ErrTaken  = errors.New("another service already uses this slug, host name or public port")
)

// Target is one upstream: a literal address (an IP or a host name) or an address set (05a §5.2) whose resolved
// addresses are load-balanced. Exactly one of Address and SetID is set.
type Target struct {
	Address string `json:"address,omitempty"`
	SetID   string `json:"set_id,omitempty"`
	Port    int    `json:"port"`
	Weight  int    `json:"weight,omitempty"`
}

// Route is an advanced route; its identity is ID, never Priority.
type Route struct {
	ID             string
	Enabled        bool
	Targets        []Target
	PathMatch      string
	PathValue      string
	Priority       int
	RewriteMode    string
	RewritePattern string
	RewriteValue   string
}

// Service is one HTTP-family service with its routes.
type Service struct {
	ID                string
	Name              string
	Slug              string
	Kind              string
	DomainMode        string
	HostName          string
	Enabled           bool
	Origin            string
	OwnerFeature      string
	Target            Target
	TLSSkipVerify     bool
	TLSServerName     string
	TLSMinVersion     string
	AdvancedRoutes    bool
	DashboardVisible  bool
	MonitoringVisible bool
	AdGuardRewrite    bool
	// EntryHostID is the service routing override (03 §5.1): the firewall host public traffic enters through.
	EntryHostID string
	// PublicPort is a stream's public port; Traefik's entry point tcp-<port> / udp-<port> listens on it (04 §3).
	PublicPort int
	// ProxyProtocol sends PROXY protocol v2 to a TCP upstream (testdata/contracts/traefik/streams).
	ProxyProtocol bool
	Routes        []Route
	// Targets is the simple target with its address set resolved (set by Resolve for rendering; nil otherwise).
	Targets   []Target
	CreatedAt time.Time
	UpdatedAt time.Time
	// InvalidReason is set when the service no longer validates against the current root domain and zone; such a
	// service is left out of every render and never stops the others (04 §1.5).
	InvalidReason string
}

// Renderable reports whether the service goes into the rendered Traefik files (04 §7).
func (s Service) Renderable() bool { return s.Enabled && s.InvalidReason == "" }

// System reports whether the service is a system service (invariant 10).
func (s Service) System() bool { return s.Origin == OriginSystem }

// CurrentNames reads the root domain and managed zone that host names are checked against.
func CurrentNames(ctx context.Context, q *db.Queries) (Names, error) {
	g, err := settings.Get[settings.General](ctx, q)
	if err != nil {
		return Names{}, err
	}
	n := Names{Root: g.RootDomain}
	zones, err := q.ListDNSZones(ctx)
	if err != nil {
		return n, err
	}
	if len(zones) > 0 {
		n.Zone = zones[0].Name
	}
	return n, nil
}

// List returns every service with its routes, each checked against the current names.
func List(ctx context.Context, q *db.Queries) ([]Service, error) {
	n, err := CurrentNames(ctx, q)
	if err != nil {
		return nil, err
	}
	rows, err := q.ListServices(ctx)
	if err != nil {
		return nil, err
	}
	routes, err := q.ListAllServiceRoutes(ctx)
	if err != nil {
		return nil, err
	}
	byService := map[string][]db.ServiceRoute{}
	for _, r := range routes {
		byService[r.ServiceID] = append(byService[r.ServiceID], r)
	}
	out := make([]Service, 0, len(rows))
	for _, r := range rows {
		s, err := fromRow(r, byService[r.ID])
		if err != nil {
			return nil, err
		}
		if err := Validate(s, n); err != nil {
			s.InvalidReason = err.Error()
		}
		out = append(out, s)
	}
	return out, nil
}

// Get returns one service.
func Get(ctx context.Context, q *db.Queries, id string) (Service, error) {
	all, err := List(ctx, q)
	if err != nil {
		return Service{}, err
	}
	for _, s := range all {
		if s.ID == id {
			return s, nil
		}
	}
	return Service{}, ErrNotFound
}

// Create validates and stores a new user service.
func Create(ctx context.Context, q *db.Queries, s Service, now time.Time) (Service, error) {
	s.ID, s.Origin, s.OwnerFeature = ulid.Make().String(), OriginUser, ""
	return s, insert(ctx, q, s, now)
}

func insert(ctx context.Context, q *db.Queries, s Service, now time.Time) error {
	n, err := CurrentNames(ctx, q)
	if err != nil {
		return err
	}
	s = normalized(s)
	if err := Validate(s, n); err != nil {
		return err
	}
	if err := taken(ctx, q, s); err != nil {
		return err
	}
	if err := references(ctx, q, s); err != nil {
		return err
	}
	ms := now.UnixMilli()
	if err := q.InsertService(ctx, db.InsertServiceParams{ID: s.ID, Name: s.Name, Slug: s.Slug, Kind: s.Kind,
		DomainMode: s.DomainMode, HostName: s.HostName, Enabled: b2i(s.Enabled), Origin: s.Origin, OwnerFeature: s.OwnerFeature,
		TargetAddress: s.Target.Address, TargetPort: int64(s.Target.Port), TlsSkipVerify: b2i(s.TLSSkipVerify),
		TlsServerName: s.TLSServerName, TlsMinVersion: s.TLSMinVersion, AdvancedRoutes: b2i(s.AdvancedRoutes),
		DashboardVisible: b2i(s.DashboardVisible), MonitoringVisible: b2i(s.MonitoringVisible),
		AdguardRewrite: b2i(s.AdGuardRewrite), TargetSetID: nullStr(s.Target.SetID), EntryHostID: nullStr(s.EntryHostID),
		PublicPort: nullPort(s.PublicPort), ProxyProtocol: b2i(s.ProxyProtocol), CreatedAt: ms, UpdatedAt: ms}); err != nil {
		return fmt.Errorf("store service: %w", err)
	}
	return putRoutes(ctx, q, s, ms)
}

// Update replaces a user service's fields and routes. System services are refused (invariant 10).
func Update(ctx context.Context, q *db.Queries, s Service, now time.Time) (Service, error) {
	cur, err := Get(ctx, q, s.ID)
	if err != nil {
		return Service{}, err
	}
	if cur.System() {
		return Service{}, ErrSystem
	}
	s.Origin, s.OwnerFeature, s.Enabled = cur.Origin, cur.OwnerFeature, cur.Enabled
	return s, update(ctx, q, s, now)
}

func update(ctx context.Context, q *db.Queries, s Service, now time.Time) error {
	n, err := CurrentNames(ctx, q)
	if err != nil {
		return err
	}
	s = normalized(s)
	if err := Validate(s, n); err != nil {
		return err
	}
	if err := taken(ctx, q, s); err != nil {
		return err
	}
	if err := references(ctx, q, s); err != nil {
		return err
	}
	ms := now.UnixMilli()
	if err := q.UpdateService(ctx, db.UpdateServiceParams{Name: s.Name, Slug: s.Slug, Kind: s.Kind, DomainMode: s.DomainMode,
		HostName: s.HostName, TargetAddress: s.Target.Address, TargetPort: int64(s.Target.Port),
		TlsSkipVerify: b2i(s.TLSSkipVerify), TlsServerName: s.TLSServerName, TlsMinVersion: s.TLSMinVersion,
		AdvancedRoutes: b2i(s.AdvancedRoutes), DashboardVisible: b2i(s.DashboardVisible),
		MonitoringVisible: b2i(s.MonitoringVisible), AdguardRewrite: b2i(s.AdGuardRewrite),
		TargetSetID: nullStr(s.Target.SetID), EntryHostID: nullStr(s.EntryHostID), PublicPort: nullPort(s.PublicPort),
		ProxyProtocol: b2i(s.ProxyProtocol), UpdatedAt: ms, ID: s.ID}); err != nil {
		return fmt.Errorf("update service: %w", err)
	}
	if err := q.DeleteServiceRoutes(ctx, s.ID); err != nil {
		return err
	}
	return putRoutes(ctx, q, s, ms)
}

// SetEnabled turns a user service on or off; disabled services stay stored but are not rendered (04 §7).
func SetEnabled(ctx context.Context, q *db.Queries, id string, enabled bool, now time.Time) (Service, error) {
	cur, err := Get(ctx, q, id)
	if err != nil {
		return Service{}, err
	}
	if cur.System() {
		return Service{}, ErrSystem
	}
	if err := q.SetServiceEnabled(ctx, db.SetServiceEnabledParams{Enabled: b2i(enabled), UpdatedAt: now.UnixMilli(), ID: id}); err != nil {
		return Service{}, err
	}
	cur.Enabled = enabled
	return cur, nil
}

// Delete removes a user service; system services cannot be deleted (invariant 10).
func Delete(ctx context.Context, q *db.Queries, id string) (Service, error) {
	cur, err := Get(ctx, q, id)
	if err != nil {
		return Service{}, err
	}
	if cur.System() {
		return Service{}, ErrSystem
	}
	return cur, q.DeleteService(ctx, id)
}

// EnsureSystem creates or updates the Hashi admin service from General settings (02 §1.6): admin domain → internal
// Hashi URL, origin system, owned by setup. It reports whether anything changed.
func EnsureSystem(ctx context.Context, q *db.Queries, now time.Time) (Service, bool, error) {
	g, err := settings.Get[settings.General](ctx, q)
	if err != nil {
		return Service{}, false, err
	}
	want, err := adminService(g)
	if err != nil {
		return Service{}, false, err
	}
	row, err := q.GetServiceByOwner(ctx, OwnerSetup)
	if errors.Is(err, sql.ErrNoRows) {
		want.ID = ulid.Make().String()
		if err := insert(ctx, q, want, now); err != nil {
			return Service{}, false, err
		}
		s, err := Get(ctx, q, want.ID)
		return s, true, err
	}
	if err != nil {
		return Service{}, false, err
	}
	cur, err := Get(ctx, q, row.ID)
	if err != nil {
		return Service{}, false, err
	}
	if cur.HostName == want.HostName && cur.Kind == want.Kind && cur.Target == want.Target && cur.DomainMode == want.DomainMode {
		return cur, false, nil
	}
	want.ID = cur.ID
	if err := update(ctx, q, want, now); err != nil {
		return Service{}, false, err
	}
	s, err := Get(ctx, q, cur.ID)
	return s, true, err
}

// adminService is the desired Hashi admin service for the settings.
func adminService(g settings.General) (Service, error) {
	if g.AdminDomain == "" || g.InternalURL == "" {
		return Service{}, errors.New("the admin domain and internal Hashi URL are not set (setup step 2)")
	}
	u, err := url.Parse(g.InternalURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return Service{}, fmt.Errorf("the internal Hashi URL %q is not an http(s) URL", g.InternalURL)
	}
	port := 80
	if u.Scheme == "https" {
		port = 443
	}
	if p := u.Port(); p != "" {
		if port, err = strconv.Atoi(p); err != nil {
			return Service{}, fmt.Errorf("the internal Hashi URL %q has a bad port", g.InternalURL)
		}
	}
	mode := ModeSubdomain
	if g.AdminDomain == g.RootDomain {
		mode = ModeRoot
	}
	return Service{Name: "Hashi", Slug: "hashi-admin", Kind: u.Scheme, DomainMode: mode, HostName: g.AdminDomain,
		Enabled: true, Origin: OriginSystem, OwnerFeature: OwnerSetup, Target: Target{Address: u.Hostname(), Port: port}}, nil
}

// normalized fills defaults: the routes' IDs and rewrite mode.
func normalized(s Service) Service {
	routes := make([]Route, len(s.Routes))
	for i, r := range s.Routes {
		if r.ID == "" {
			r.ID = ulid.Make().String()
		}
		if r.RewriteMode == "" {
			r.RewriteMode = RewriteNone
		}
		routes[i] = r
	}
	s.Routes = routes
	return s
}

func taken(ctx context.Context, q *db.Queries, s Service) error {
	rows, err := q.ListServices(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.ID != s.ID && (r.Slug == s.Slug || r.HostName == s.HostName) {
			return ErrTaken
		}
		if r.ID != s.ID && s.Stream() && r.Kind == s.Kind && int(r.PublicPort.Int64) == s.PublicPort {
			return fmt.Errorf("%w: %s already uses %d/%s", ErrTaken, r.Name, s.PublicPort, s.Kind)
		}
	}
	return nil
}

func putRoutes(ctx context.Context, q *db.Queries, s Service, ms int64) error {
	for _, r := range s.Routes {
		targets, err := json.Marshal(r.Targets)
		if err != nil {
			return err
		}
		if err := q.InsertServiceRoute(ctx, db.InsertServiceRouteParams{ID: r.ID, ServiceID: s.ID, Enabled: b2i(r.Enabled),
			TargetsJson: string(targets), PathMatch: r.PathMatch, PathValue: r.PathValue, Priority: int64(r.Priority),
			RewriteMode: r.RewriteMode, RewritePattern: r.RewritePattern, RewriteValue: r.RewriteValue,
			CreatedAt: ms, UpdatedAt: ms}); err != nil {
			return fmt.Errorf("store route: %w", err)
		}
	}
	return nil
}

func fromRow(r db.Service, routes []db.ServiceRoute) (Service, error) {
	s := Service{ID: r.ID, Name: r.Name, Slug: r.Slug, Kind: r.Kind, DomainMode: r.DomainMode, HostName: r.HostName,
		Enabled: r.Enabled == 1, Origin: r.Origin, OwnerFeature: r.OwnerFeature,
		Target:      Target{Address: r.TargetAddress, SetID: r.TargetSetID.String, Port: int(r.TargetPort)},
		EntryHostID: r.EntryHostID.String, PublicPort: int(r.PublicPort.Int64), ProxyProtocol: r.ProxyProtocol == 1,
		TLSSkipVerify: r.TlsSkipVerify == 1, TLSServerName: r.TlsServerName, TLSMinVersion: r.TlsMinVersion,
		AdvancedRoutes: r.AdvancedRoutes == 1, DashboardVisible: r.DashboardVisible == 1,
		MonitoringVisible: r.MonitoringVisible == 1, AdGuardRewrite: r.AdguardRewrite == 1,
		CreatedAt: time.UnixMilli(r.CreatedAt).UTC(), UpdatedAt: time.UnixMilli(r.UpdatedAt).UTC()}
	for _, rr := range routes {
		var t []Target
		if err := json.Unmarshal([]byte(rr.TargetsJson), &t); err != nil {
			return s, fmt.Errorf("route %s targets: %w", rr.ID, err)
		}
		s.Routes = append(s.Routes, Route{ID: rr.ID, Enabled: rr.Enabled == 1, Targets: t, PathMatch: rr.PathMatch,
			PathValue: rr.PathValue, Priority: int(rr.Priority), RewriteMode: rr.RewriteMode,
			RewritePattern: rr.RewritePattern, RewriteValue: rr.RewriteValue})
	}
	return s, nil
}

// references checks what a service points at outside its own row: target sets must be address sets (05a §5.2) and
// the entry host must exist.
func references(ctx context.Context, q *db.Queries, s Service) error {
	check := func(field string, t Target) error {
		if t.SetID == "" {
			return nil
		}
		if err := ipsets.CheckTarget(ctx, q, t.SetID); err != nil {
			if errors.Is(err, ipsets.ErrNotFound) {
				return invalid(field+".set_id", "no IP set %s", t.SetID)
			}
			return invalid(field+".set_id", "%s", strings.TrimPrefix(err.Error(), ipsets.ErrInvalid.Error()+": "))
		}
		return nil
	}
	if err := check("target", s.Target); err != nil {
		return err
	}
	for i, r := range s.Routes {
		for j, t := range r.Targets {
			if err := check(fmt.Sprintf("routes[%d].targets[%d]", i, j), t); err != nil {
				return err
			}
		}
	}
	if s.EntryHostID != "" {
		if _, err := q.GetFirewallHost(ctx, s.EntryHostID); err != nil {
			return invalid("entry_host_id", "no firewall host %s", s.EntryHostID)
		}
	}
	return nil
}

// Uses lists the services whose targets reference set id (05a §5.2 consumers).
func Uses(svcs []Service, id string) []Service {
	var out []Service
	for _, s := range svcs {
		if slices.ContainsFunc(s.AllTargets(), func(t Target) bool { return t.SetID == id }) {
			out = append(out, s)
		}
	}
	return out
}

// AllTargets is the simple target followed by every route target.
func (s Service) AllTargets() []Target {
	out := []Target{s.Target}
	for _, r := range s.Routes {
		out = append(out, r.Targets...)
	}
	return out
}

func nullPort(p int) sql.NullInt64 { return sql.NullInt64{Int64: int64(p), Valid: p != 0} }

// Stream reports an L4 service (tcp or udp).
func (s Service) Stream() bool { return s.Kind == KindTCP || s.Kind == KindUDP }

func nullStr(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

// HostPort formats an upstream as host:port, bracketing IPv6 (04 §1.5).
func (t Target) HostPort() string { return net.JoinHostPort(t.Address, strconv.Itoa(t.Port)) }

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// TargetResolver resolves the address set of one target position (ipsets.Service.ResolveTarget).
type TargetResolver func(ctx context.Context, consumer, setID string) (ipsets.Target, error)

// Resolve replaces every address-set target with one literal target per resolved address (same port and weight, in
// the set's order), for rendering. A set that resolves to nothing now keeps its last good addresses (invariant 21); a
// position that never resolved leaves the service out of the render with the reason, like any invalid service.
func Resolve(ctx context.Context, svcs []Service, resolve TargetResolver) ([]Service, error) {
	out := make([]Service, len(svcs))
	for i, s := range svcs {
		// expand returns the literal targets and, when none of the set targets ever resolved, the reason.
		expand := func(consumer string, ts []Target) ([]Target, string, error) {
			var res []Target
			reason := ""
			for j, t := range ts {
				if t.SetID == "" {
					res = append(res, t)
					continue
				}
				got, err := resolve(ctx, fmt.Sprintf("%s:%d", consumer, j), t.SetID)
				if err != nil {
					return nil, "", err
				}
				if len(got.Addresses) == 0 && reason == "" {
					reason = got.Unresolved
				}
				for _, a := range got.Addresses {
					res = append(res, Target{Address: a.String(), Port: t.Port, Weight: t.Weight})
				}
			}
			if len(res) > 0 {
				reason = ""
			}
			return res, reason, nil
		}
		simple, reason, err := expand("service:"+s.ID+":target", []Target{s.Target})
		if err != nil {
			return nil, fmt.Errorf("service %s: %w", s.Slug, err)
		}
		if reason != "" && s.InvalidReason == "" {
			s.InvalidReason = reason // nothing to serve
		}
		s.Targets = simple
		routes := make([]Route, len(s.Routes))
		for k, r := range s.Routes {
			if s.AdvancedRoutes && r.Enabled { // only rendered routes resolve (05a §5.2); the rest stay as stored
				if r.Targets, reason, err = expand("service:"+s.ID+":route:"+r.ID, r.Targets); err != nil {
					return nil, fmt.Errorf("service %s: %w", s.Slug, err)
				}
				// A route that never resolved is left out; its paths fall to the main router (invariant 21: the
				// service itself keeps serving).
				r.Enabled = reason == ""
			}
			routes[k] = r
		}
		s.Routes = routes
		out[i] = s
	}
	return out, nil
}

// Consumers lists the services whose upstreams use set id (05a §5.2: target positions), for the IP set pages and
// the address-set checks on edit and delete.
func Consumers(ctx context.Context, q *db.Queries, id string) ([]ipsets.Consumer, error) {
	svcs, err := List(ctx, q)
	if err != nil {
		return nil, err
	}
	var out []ipsets.Consumer
	for _, s := range Uses(svcs, id) {
		out = append(out, ipsets.Consumer{Type: "service", ID: s.ID, Name: "Service " + s.Name, Position: ipsets.PositionTarget})
	}
	return out, nil
}
