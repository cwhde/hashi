package dns

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// tokenPurpose names the vault secret holding a DNS provider token.
const tokenPurpose = "dns.provider.token" //nolint:gosec // a vault purpose name, not a credential

// Provider types Hashi has an adapter for.
var ProviderTypes = []string{"hetzner"}

// Factory opens a Provider for a provider type and token (wired in internal/app; tests pass a replay-backed one).
type Factory func(typ string, token secrets.Value) (Provider, error)

// Service errors mapped by the API.
var (
	ErrInvalid       = errors.New("invalid input")
	ErrValidation    = errors.New("read-only validation failed")
	ErrNotConfigured = errors.New("no DNS provider is configured yet")
	ErrNoZone        = errors.New("no such DNS zone")
)

// Service keeps the DNS provider connection, the managed zone, its observed snapshot, ownership, import decisions and
// desired entries. It writes to providers only through its sync subsystems (plans.go, invariant 7).
type Service struct {
	st    *store.Store
	vault *vault.Vault
	audit *audit.Log
	open  Factory
	now   func() time.Time
}

// NewService returns the DNS service.
func NewService(st *store.Store, v *vault.Vault, a *audit.Log, open Factory) *Service {
	return &Service{st: st, vault: v, audit: a, open: open, now: time.Now}
}

// ProviderInfo is a provider connection without its token.
type ProviderInfo struct {
	ID              string
	Type            string
	DefaultTTL      int
	Health          string
	LastValidatedAt *time.Time
	LastError       string
}

// ZoneInfo is a managed zone with its refresh state.
type ZoneInfo struct {
	ID               string
	Name             string
	Provider         ProviderInfo
	ZoneTTL          int
	LastRefreshedAt  *time.Time
	LastRefreshError string
	RRSetCount       int
}

// ConnectInput is setup step 3's form (02 §1.3). A zero Token keeps the stored one; an empty ZoneID picks the zone
// containing RootDomain.
type ConnectInput struct {
	Type       string
	Token      secrets.Value
	DefaultTTL int
	RootDomain string
	ZoneID     string
}

// ConnectResult lists the zones the token sees and the zone now managed. Provider and Zone are nil when no zone
// contains the root domain: nothing was stored and the admin picks one (sending the token again).
type ConnectResult struct {
	Provider *ProviderInfo
	Zones    []Zone
	Zone     *ZoneInfo
}

// Connect validates a token read-only (list zones, read the zone's RRSets), then stores the provider with its token
// sealed in the vault, the managed zone and its first snapshot. Nothing is stored when validation fails or no zone
// was chosen, so a stored token can always read the stored zone.
func (s *Service) Connect(ctx context.Context, actor, ip string, in ConnectInput) (ConnectResult, error) {
	if !slices.Contains(ProviderTypes, in.Type) {
		return ConnectResult{}, fmt.Errorf("%w: unknown provider type %q", ErrInvalid, in.Type)
	}
	if in.DefaultTTL < 60 || in.DefaultTTL > 86400 {
		return ConnectResult{}, fmt.Errorf("%w: default TTL must be between 60 and 86400 seconds", ErrInvalid)
	}
	existing, err := s.provider(ctx, s.st.R)
	if err != nil && !errors.Is(err, ErrNotConfigured) {
		return ConnectResult{}, err
	}
	token := in.Token
	if token.IsZero() {
		if existing == nil {
			return ConnectResult{}, fmt.Errorf("%w: an API token is required", ErrInvalid)
		}
		if token, err = s.token(ctx, *existing); err != nil {
			return ConnectResult{}, err
		}
	}
	r, err := s.open(in.Type, token)
	if err != nil {
		return ConnectResult{}, err
	}
	zones, err := r.ListZones(ctx)
	if err != nil {
		return ConnectResult{}, fmt.Errorf("%w: %w", ErrValidation, err)
	}
	if len(zones) == 0 {
		return ConnectResult{}, fmt.Errorf("%w: the token sees no DNS zones", ErrValidation)
	}
	zone, err := s.pickZone(ctx, zones, in, existing)
	if err != nil {
		return ConnectResult{}, err
	}
	if zone == nil {
		// Validation is incomplete until a zone's RRSets were read with this token: store nothing, let the admin pick.
		return ConnectResult{Zones: zones}, nil
	}
	rrsets, err := r.ListRRSets(ctx, *zone)
	if err != nil {
		return ConnectResult{}, fmt.Errorf("%w: read RRSets of %s: %w", ErrValidation, zone.Name, err)
	}

	now := s.now()
	var providerID, zoneID string
	err = s.st.Tx(ctx, func(q *db.Queries) error {
		var err error
		if providerID, err = s.saveProvider(ctx, q, existing, in, now); err != nil {
			return err
		}
		if zoneID, err = s.saveZone(ctx, q, providerID, *zone, now); err != nil {
			return err
		}
		if _, err := s.replaceSnapshot(ctx, q, zoneID, *zone, rrsets, now); err != nil {
			return err
		}
		return s.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "dns.provider.connected", IP: ip,
			Target: "dns_zone:" + zoneID, Detail: map[string]any{"type": in.Type, "zone": zone.Name, "rrsets": len(rrsets)}})
	})
	if err != nil {
		return ConnectResult{}, err
	}
	zi, err := s.Zone(ctx, zoneID)
	if err != nil {
		return ConnectResult{}, err
	}
	return ConnectResult{Provider: &zi.Provider, Zones: zones, Zone: &zi}, nil
}

// pickZone: an explicit choice must be one the token sees; otherwise the zone containing the root domain, else the
// zone already stored if this token sees it (resuming setup), else none.
func (s *Service) pickZone(ctx context.Context, zones []Zone, in ConnectInput, existing *ProviderInfo) (*Zone, error) {
	if in.ZoneID != "" {
		for _, z := range zones {
			if z.ID == in.ZoneID {
				return &z, nil
			}
		}
		return nil, fmt.Errorf("%w: the token does not see zone %q", ErrInvalid, in.ZoneID)
	}
	if z, err := ResolveFromList(zones, in.RootDomain); err == nil {
		return &z, nil
	}
	if existing != nil {
		rows, err := s.st.R.ListDNSZones(ctx)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			for _, z := range zones {
				if row.ProviderID == existing.ID && z.ID == row.ProviderZoneID {
					return &z, nil
				}
			}
		}
	}
	return nil, nil
}

func (s *Service) saveProvider(ctx context.Context, q *db.Queries, existing *ProviderInfo, in ConnectInput, now time.Time) (string, error) {
	ms := now.UnixMilli()
	validated := sql.NullInt64{Int64: ms, Valid: true}
	if existing == nil {
		sec, err := s.seal(ctx, q, in.Token)
		if err != nil {
			return "", err
		}
		id := ulid.Make().String()
		return id, q.InsertDNSProvider(ctx, db.InsertDNSProviderParams{ID: id, Name: "Hetzner DNS", Type: in.Type,
			DefaultTtl: int64(in.DefaultTTL), TokenSecretID: sec, Health: "healthy", LastValidatedAt: validated, CreatedAt: ms, UpdatedAt: ms})
	}
	row, err := q.GetDNSProvider(ctx, existing.ID)
	if err != nil {
		return "", err
	}
	if row.Type != in.Type {
		return "", fmt.Errorf("%w: the provider type cannot change (%s configured)", ErrInvalid, row.Type)
	}
	secretID := row.TokenSecretID
	if !in.Token.IsZero() {
		if secretID, err = s.seal(ctx, q, in.Token); err != nil {
			return "", err
		}
	}
	if err := q.UpdateDNSProvider(ctx, db.UpdateDNSProviderParams{DefaultTtl: int64(in.DefaultTTL), TokenSecretID: secretID,
		Health: "healthy", LastValidatedAt: validated, LastValidationError: "", UpdatedAt: ms, ID: row.ID}); err != nil {
		return "", err
	}
	if secretID != row.TokenSecretID {
		if err := q.DeleteSecret(ctx, row.TokenSecretID); err != nil {
			return "", err
		}
	}
	return row.ID, nil
}

func (s *Service) seal(ctx context.Context, q *db.Queries, token secrets.Value) (string, error) {
	p, err := s.vault.Seal(vault.Service, tokenPurpose, token)
	if err != nil {
		return "", err
	}
	return p.ID, q.InsertSecret(ctx, p)
}

// saveZone keeps the zone row (and with it ownership and decisions) when the same zone is chosen again, and drops any
// other zone of the provider: Hashi manages exactly one.
func (s *Service) saveZone(ctx context.Context, q *db.Queries, providerID string, z Zone, now time.Time) (string, error) {
	row, err := q.GetDNSZoneByProviderZone(ctx, db.GetDNSZoneByProviderZoneParams{ProviderID: providerID, ProviderZoneID: z.ID})
	id := row.ID
	switch {
	case errors.Is(err, sql.ErrNoRows):
		id = ulid.Make().String()
		if _, err := q.DeleteDNSZonesExcept(ctx, db.DeleteDNSZonesExceptParams{ProviderID: providerID, ID: id}); err != nil {
			return "", err
		}
		if err := q.InsertDNSZone(ctx, db.InsertDNSZoneParams{ID: id, ProviderID: providerID, ProviderZoneID: z.ID,
			Name: z.Name, ZoneTtl: int64(z.TTL), CreatedAt: now.UnixMilli()}); err != nil {
			return "", fmt.Errorf("store zone %s: %w", z.Name, err)
		}
	case err != nil:
		return "", err
	default:
		if _, err := q.DeleteDNSZonesExcept(ctx, db.DeleteDNSZonesExceptParams{ProviderID: providerID, ID: id}); err != nil {
			return "", err
		}
	}
	return id, nil
}

type rrsetKey struct{ name, typ string }

// replaceSnapshot swaps the zone's observed RRSets for rrsets and returns a diff summary against the old snapshot.
func (s *Service) replaceSnapshot(ctx context.Context, q *db.Queries, zoneID string, z Zone, rrsets []RRSet, now time.Time) (string, error) {
	old, err := q.ListObservedRRSets(ctx, zoneID)
	if err != nil {
		return "", err
	}
	before := make(map[rrsetKey]string, len(old))
	for _, o := range old {
		before[rrsetKey{o.Name, o.Type}] = o.Hash
	}
	if err := q.DeleteObservedRRSets(ctx, zoneID); err != nil {
		return "", err
	}
	var added, changed int
	for _, r := range rrsets {
		vals, _ := json.Marshal(r.Values)
		labels, _ := json.Marshal(r.Labels)
		h := r.Hash()
		k := rrsetKey{r.Name, r.Type}
		if prev, ok := before[k]; !ok {
			added++
		} else if prev != h {
			changed++
		}
		delete(before, k)
		var ttl sql.NullInt64
		if r.TTL != nil {
			ttl = sql.NullInt64{Int64: int64(*r.TTL), Valid: true}
		}
		if err := q.InsertObservedRRSet(ctx, db.InsertObservedRRSetParams{ZoneID: zoneID, Name: r.Name, Type: r.Type, Ttl: ttl,
			ValuesJson: string(vals), LabelsJson: string(labels), Protected: boolInt(r.Protected), Hash: h, ObservedAt: now.UnixMilli(), Owner: r.Owner}); err != nil {
			return "", fmt.Errorf("store RRSet %s %s: %w", r.Name, r.Type, err)
		}
	}
	if err := q.SetDNSZoneRefreshed(ctx, db.SetDNSZoneRefreshedParams{ZoneTtl: int64(z.TTL), LastRefreshedAt: sql.NullInt64{Int64: now.UnixMilli(), Valid: true},
		RrsetCount: int64(len(rrsets)), ID: zoneID}); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s: %d RRSets (+%d −%d ~%d)", z.Name, len(rrsets), added, len(before), changed), nil
}

// Refresh reads the zone's RRSets from the provider and replaces the observed snapshot. A failure keeps the previous
// snapshot and is recorded on the zone and the provider's health.
func (s *Service) Refresh(ctx context.Context, zoneID string) (string, error) {
	zrow, err := s.st.R.GetDNSZone(ctx, zoneID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNoZone
	}
	if err != nil {
		return "", err
	}
	summary, rerr := s.refresh(ctx, zrow)
	now := s.now().UnixMilli()
	health, msg := "healthy", ""
	if rerr != nil {
		health, msg = "failed", rerr.Error()
	}
	if err := s.st.Tx(ctx, func(q *db.Queries) error {
		if rerr != nil {
			if err := q.SetDNSZoneRefreshError(ctx, db.SetDNSZoneRefreshErrorParams{LastRefreshError: msg, ID: zoneID}); err != nil {
				return err
			}
		}
		return q.SetDNSProviderHealth(ctx, db.SetDNSProviderHealthParams{Health: health, LastValidatedAt: sql.NullInt64{Int64: now, Valid: true},
			LastValidationError: msg, UpdatedAt: now, ID: zrow.ProviderID})
	}); err != nil {
		return "", errors.Join(rerr, err)
	}
	return summary, rerr
}

func (s *Service) refresh(ctx context.Context, zrow db.DnsZone) (string, error) {
	prow, err := s.st.R.GetDNSProvider(ctx, zrow.ProviderID)
	if err != nil {
		return "", err
	}
	token, err := s.token(ctx, providerInfo(prow))
	if err != nil {
		return "", err
	}
	r, err := s.open(prow.Type, token)
	if err != nil {
		return "", err
	}
	zones, err := r.ListZones(ctx)
	if err != nil {
		return "", err
	}
	idx := slices.IndexFunc(zones, func(z Zone) bool { return z.ID == zrow.ProviderZoneID })
	if idx < 0 {
		return "", fmt.Errorf("%w: %s is no longer visible to the token", ErrZoneNotFound, zrow.Name)
	}
	rrsets, err := r.ListRRSets(ctx, zones[idx])
	if err != nil {
		return "", err
	}
	var summary string
	err = s.st.Tx(ctx, func(q *db.Queries) error {
		summary, err = s.replaceSnapshot(ctx, q, zrow.ID, zones[idx], rrsets, s.now())
		return err
	})
	return summary, err
}

// RefreshAll refreshes every managed zone; it is the zone refresh job.
func (s *Service) RefreshAll(ctx context.Context) (string, error) {
	rows, err := s.st.R.ListDNSZones(ctx)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "no DNS zone configured", nil
	}
	var sums []string
	var errs []error
	for _, z := range rows {
		sum, err := s.Refresh(ctx, z.ID)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", z.Name, err))
			continue
		}
		sums = append(sums, sum)
	}
	return strings.Join(sums, "; "), errors.Join(errs...)
}

func (s *Service) token(ctx context.Context, p ProviderInfo) (secrets.Value, error) {
	row, err := s.st.R.GetDNSProvider(ctx, p.ID)
	if err != nil {
		return secrets.Value{}, err
	}
	rec, err := s.st.R.GetSecret(ctx, row.TokenSecretID)
	if err != nil {
		return secrets.Value{}, fmt.Errorf("read DNS token: %w", err)
	}
	return s.vault.Open(rec)
}

// provider returns the configured provider (the first; one exists after setup step 3).
func (s *Service) provider(ctx context.Context, q *db.Queries) (*ProviderInfo, error) {
	rows, err := q.ListDNSProviders(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, ErrNotConfigured
	}
	p := providerInfo(rows[0])
	return &p, nil
}

// Current returns the configured provider and its managed zone (nil when none was picked yet).
func (s *Service) Current(ctx context.Context) (*ProviderInfo, *ZoneInfo, error) {
	p, err := s.provider(ctx, s.st.R)
	if err != nil {
		return nil, nil, err
	}
	zones, err := s.Zones(ctx)
	if err != nil || len(zones) == 0 {
		return p, nil, err
	}
	return p, &zones[0], nil
}

func providerInfo(r db.DnsProvider) ProviderInfo {
	return ProviderInfo{ID: r.ID, Type: r.Type, DefaultTTL: int(r.DefaultTtl), Health: r.Health,
		LastValidatedAt: msPtr(r.LastValidatedAt), LastError: r.LastValidationError}
}

// Zones lists the managed zones.
func (s *Service) Zones(ctx context.Context) ([]ZoneInfo, error) {
	rows, err := s.st.R.ListDNSZones(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ZoneInfo, 0, len(rows))
	for _, r := range rows {
		zi, err := s.zoneInfo(ctx, r)
		if err != nil {
			return nil, err
		}
		out = append(out, zi)
	}
	return out, nil
}

// Zone returns one managed zone.
func (s *Service) Zone(ctx context.Context, id string) (ZoneInfo, error) {
	r, err := s.st.R.GetDNSZone(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ZoneInfo{}, ErrNoZone
	}
	if err != nil {
		return ZoneInfo{}, err
	}
	return s.zoneInfo(ctx, r)
}

func (s *Service) zoneInfo(ctx context.Context, r db.DnsZone) (ZoneInfo, error) {
	p, err := s.st.R.GetDNSProvider(ctx, r.ProviderID)
	if err != nil {
		return ZoneInfo{}, err
	}
	return ZoneInfo{ID: r.ID, Name: r.Name, Provider: providerInfo(p), ZoneTTL: int(r.ZoneTtl), LastRefreshedAt: msPtr(r.LastRefreshedAt),
		LastRefreshError: r.LastRefreshError, RRSetCount: int(r.RrsetCount)}, nil
}

// Observed is an observed RRSet with Hashi's ownership ("" = not Hashi's) and import decision (nil = undecided).
// Entry is Hashi's desired entry for it, if any. Pending marks an entry Hashi has not created at the provider yet
// (RRSets lists it with the entry's values).
type Observed struct {
	RRSet
	Origin     string
	Decision   *Decision
	ObservedAt time.Time
	Entry      *Record
	Pending    bool
}

// Origins (invariant 9). Manual entries are user, imports imported; system and managed arrive with generated records.
const (
	OriginSystem   = "system"
	OriginManaged  = "managed"
	OriginImported = "imported"
	OriginUser     = "user"
)

// Unmanaged is the origin filter value for RRSets Hashi does not own.
const Unmanaged = "unmanaged"

// RRSets lists the zone's observed RRSets. origin filters by ownership (an origin or Unmanaged); q matches the name,
// type or a value, case-insensitively.
func (s *Service) RRSets(ctx context.Context, zoneID, origin, q string) ([]Observed, error) {
	if _, err := s.Zone(ctx, zoneID); err != nil {
		return nil, err
	}
	all, err := s.observed(ctx, s.st.R, zoneID)
	if err != nil {
		return nil, err
	}
	recs, err := s.Records(ctx, s.st.R, zoneID)
	if err != nil {
		return nil, err
	}
	idx := make(map[rrsetKey]int, len(all))
	for i, o := range all {
		idx[rrsetKey{o.Name, o.Type}] = i
	}
	for _, rec := range recs {
		if i, ok := idx[rrsetKey{rec.RRSet.Name, rec.RRSet.Type}]; ok {
			all[i].Entry = &rec
			continue
		}
		all = append(all, Observed{RRSet: rec.RRSet, Origin: rec.Origin, Entry: &rec, Pending: true})
	}
	slices.SortFunc(all, func(a, b Observed) int { return strings.Compare(a.Name+" "+a.Type, b.Name+" "+b.Type) })
	q = strings.ToLower(strings.TrimSpace(q))
	out := all[:0]
	for _, o := range all {
		have := o.Origin
		if have == "" {
			have = Unmanaged
		}
		if origin != "" && have != origin {
			continue
		}
		if q != "" && !matches(o.RRSet, q) {
			continue
		}
		out = append(out, o)
	}
	return out, nil
}

func matches(r RRSet, q string) bool {
	if strings.Contains(r.Name, q) || strings.ToLower(r.Type) == q {
		return true
	}
	for _, v := range r.Values {
		if strings.Contains(strings.ToLower(v), q) {
			return true
		}
	}
	return false
}

func (s *Service) observed(ctx context.Context, q *db.Queries, zoneID string) ([]Observed, error) {
	rows, err := q.ListObservedRRSets(ctx, zoneID)
	if err != nil {
		return nil, err
	}
	owned, err := q.ListDNSOwnership(ctx, zoneID)
	if err != nil {
		return nil, err
	}
	decisions, err := q.ListImportDecisions(ctx, zoneID)
	if err != nil {
		return nil, err
	}
	origin := make(map[rrsetKey]string, len(owned))
	for _, o := range owned {
		origin[rrsetKey{o.Name, o.Type}] = o.Origin
	}
	decided := make(map[rrsetKey]Decision, len(decisions))
	for _, d := range decisions {
		decided[rrsetKey{d.Name, d.Type}] = Decision(d.Decision)
	}
	out := make([]Observed, 0, len(rows))
	for _, r := range rows {
		o := Observed{RRSet: RRSet{Name: r.Name, Type: r.Type, Protected: r.Protected == 1, Labels: map[string]string{}, Owner: r.Owner},
			ObservedAt: time.UnixMilli(r.ObservedAt).UTC()}
		if r.Ttl.Valid {
			ttl := int(r.Ttl.Int64)
			o.TTL = &ttl
		}
		if err := json.Unmarshal([]byte(r.ValuesJson), &o.Values); err != nil {
			return nil, fmt.Errorf("decode values of %s %s: %w", r.Name, r.Type, err)
		}
		_ = json.Unmarshal([]byte(r.LabelsJson), &o.Labels)
		k := rrsetKey{r.Name, r.Type}
		o.Origin = origin[k]
		if d, ok := decided[k]; ok {
			o.Decision = &d
		}
		out = append(out, o)
	}
	return out, nil
}

// ImportRow is an observed RRSet with the decisions the import table may offer for it.
type ImportRow struct {
	Observed
	Allowed []Decision
}

// ImportTable returns the zone's observed RRSets that Hashi did not create, with their decision; undecided rows are
// the new ones (03 §4).
func (s *Service) ImportTable(ctx context.Context, zoneID string) (ZoneInfo, []ImportRow, error) {
	z, err := s.Zone(ctx, zoneID)
	if err != nil {
		return ZoneInfo{}, nil, err
	}
	caps, err := s.capabilities(ctx, z.Provider)
	if err != nil {
		return ZoneInfo{}, nil, err
	}
	obs, err := s.observed(ctx, s.st.R, zoneID)
	if err != nil {
		return ZoneInfo{}, nil, err
	}
	rows := make([]ImportRow, 0, len(obs))
	for _, o := range obs {
		if o.Origin != "" && o.Origin != OriginImported {
			continue // created by Hashi: nothing to import
		}
		if IsChallenge(o.Name) {
			continue // DNS-01 values come and go with certificate orders (03 §5.3)
		}
		rows = append(rows, ImportRow{Observed: o, Allowed: AllowedDecisions(o.RRSet, caps)})
	}
	return z, rows, nil
}

// capabilities asks the provider's adapter (static for Hetzner; no request is made).
func (s *Service) capabilities(ctx context.Context, p ProviderInfo) (Capabilities, error) {
	token, err := s.token(ctx, p)
	if err != nil {
		return Capabilities{}, err
	}
	r, err := s.open(p.Type, token)
	if err != nil {
		return Capabilities{}, err
	}
	return r.Capabilities(ctx)
}

// DecisionInput is one row of a decisions save.
type DecisionInput struct {
	Name     string
	Type     string
	Decision Decision
}

// SaveDecisions persists import decisions. Each must name an observed RRSet and be allowed for it (NS/SOA and
// change-protected RRSets: leave only, invariant 2). "import" makes the RRSet Hashi's with origin imported; taking it
// back removes only that imported ownership and entry. Types the provider cannot manage are leave-only too. Prune
// decisions are only recorded; deleting needs the confirmed dns-prune plan.
func (s *Service) SaveDecisions(ctx context.Context, actor, ip, zoneID string, in []DecisionInput) error {
	z, err := s.Zone(ctx, zoneID)
	if err != nil {
		return err
	}
	caps, err := s.capabilities(ctx, z.Provider)
	if err != nil {
		return err
	}
	return s.st.Tx(ctx, func(q *db.Queries) error {
		if _, err := q.GetDNSZone(ctx, zoneID); errors.Is(err, sql.ErrNoRows) {
			return ErrNoZone
		} else if err != nil {
			return err
		}
		obs, err := s.observed(ctx, q, zoneID)
		if err != nil {
			return err
		}
		byKey := make(map[rrsetKey]Observed, len(obs))
		for _, o := range obs {
			byKey[rrsetKey{o.Name, o.Type}] = o
		}
		now := s.now().UnixMilli()
		counts := map[Decision]int{}
		seen := map[rrsetKey]bool{}
		for _, d := range in {
			k := rrsetKey{strings.ToLower(strings.TrimSpace(d.Name)), strings.ToUpper(strings.TrimSpace(d.Type))}
			if seen[k] {
				return fmt.Errorf("%w: %s %s appears twice", ErrInvalid, k.name, k.typ)
			}
			seen[k] = true
			o, ok := byKey[k]
			if !ok {
				return fmt.Errorf("%w: %s %s is not in the zone's last snapshot; refresh and try again", ErrInvalid, k.name, k.typ)
			}
			if IsChallenge(k.name) {
				return fmt.Errorf("%w: %s %s is a certificate challenge name and is not imported", ErrInvalid, k.name, k.typ)
			}
			if err := CheckDecision(o.RRSet, caps, d.Decision); err != nil {
				return fmt.Errorf("%w: %w", ErrInvalid, err)
			}
			if d.Decision == Prune && o.Origin != "" && o.Origin != OriginImported {
				return fmt.Errorf("%w: %s %s is owned by Hashi (origin %s) and cannot be pruned from the import table", ErrInvalid, k.name, k.typ, o.Origin)
			}
			if err := q.UpsertImportDecision(ctx, db.UpsertImportDecisionParams{ZoneID: zoneID, Name: k.name, Type: k.typ,
				Decision: string(d.Decision), DecidedAt: now}); err != nil {
				return err
			}
			if d.Decision == Import && (o.Origin == "" || o.Origin == OriginImported) {
				// The entry starts as the RRSet is now, so importing changes nothing at the provider. An import saved
				// before entries existed (S2) gets its entry here too.
				if _, gerr := q.GetDNSRecordByKey(ctx, db.GetDNSRecordByKeyParams{ZoneID: zoneID, Name: k.name, Type: k.typ}); errors.Is(gerr, sql.ErrNoRows) {
					err = q.InsertDNSRecord(ctx, recordParams(ulid.Make().String(), zoneID, o.RRSet, OriginImported, RecordInput{}, now))
				} else {
					err = gerr
				}
				if err == nil {
					err = q.UpsertDNSOwnership(ctx, db.UpsertDNSOwnershipParams{ZoneID: zoneID, Name: k.name, Type: k.typ,
						Origin: OriginImported, CreatedAt: now, UpdatedAt: now})
				}
			} else if d.Decision != Import && o.Origin == OriginImported {
				err = q.DeleteImportedDNSRecord(ctx, db.DeleteImportedDNSRecordParams{ZoneID: zoneID, Name: k.name, Type: k.typ})
				if err == nil {
					_, err = q.DeleteImportedOwnership(ctx, db.DeleteImportedOwnershipParams{ZoneID: zoneID, Name: k.name, Type: k.typ})
				}
			}
			if err != nil {
				return err
			}
			counts[d.Decision]++
		}
		return s.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "dns.import.decisions_saved", IP: ip, Target: "dns_zone:" + zoneID,
			Detail: map[string]any{"import": counts[Import], "leave": counts[Leave], "prune": counts[Prune]}})
	})
}

func msPtr(v sql.NullInt64) *time.Time {
	if !v.Valid {
		return nil
	}
	t := time.UnixMilli(v.Int64).UTC()
	return &t
}

func boolInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
