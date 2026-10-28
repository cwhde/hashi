package dns

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Errors for manual entries.
var (
	// ErrConflict: the RRSet exists at the provider without being Hashi's, or as another entry (03 §3).
	ErrConflict = errors.New("conflict")
	// ErrNoRecord: no manual entry with that id.
	ErrNoRecord = errors.New("no such DNS record")
)

// RecordInput is a manual entry (03 §7). Name is relative to the zone ("@" = apex; an absolute name inside the zone
// is accepted). TTL nil = zone default. Values are what the record holds: TXT as plain text (Hashi quotes and splits),
// CNAME/MX/SRV targets with or without the trailing dot.
type RecordInput struct {
	Name        string
	Type        string
	TTL         *int
	Values      []string
	DisplayName string
	Dashboard   bool
	Monitoring  bool
}

// Record is a desired RRSet Hashi owns.
type Record struct {
	ID          string
	ZoneID      string
	RRSet       RRSet
	Origin      string
	DisplayName string
	Dashboard   bool
	Monitoring  bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// instanceKey is the app_settings key of this installation's id, the value of the provider ownership marker.
const instanceKey = "dns.instance_id"

// InstanceID returns this installation's id, created on first use.
func (s *Service) InstanceID(ctx context.Context) (string, error) {
	row, err := s.st.R.GetSetting(ctx, instanceKey)
	if err == nil {
		var id string
		if err := json.Unmarshal([]byte(row.ValueJson), &id); err == nil && id != "" {
			return id, nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	id := strings.ToLower(ulid.Make().String())
	b, _ := json.Marshal(id)
	err = s.st.Tx(ctx, func(q *db.Queries) error {
		if row, err := q.GetSetting(ctx, instanceKey); err == nil { // another request won
			return json.Unmarshal([]byte(row.ValueJson), &id)
		}
		return q.UpsertSetting(ctx, db.UpsertSettingParams{Key: instanceKey, ValueJson: string(b), UpdatedAt: s.now().UnixMilli()})
	})
	return id, err
}

// normalizeInput validates in for zone and returns the RRSet it describes (03 §3 value formats).
func normalizeInput(in RecordInput, zone string, caps Capabilities) (RRSet, error) {
	typ := strings.ToUpper(strings.TrimSpace(in.Type))
	if Protected(typ) {
		return RRSet{}, fmt.Errorf("%w: %s records are never changed by Hashi", ErrInvalid, typ)
	}
	if !slices.Contains(caps.ManagedTypes, typ) {
		return RRSet{}, fmt.Errorf("%w: Hashi does not manage %s records with this provider", ErrInvalid, typ)
	}
	name, err := relativeName(in.Name, zone)
	if err != nil {
		return RRSet{}, err
	}
	if IsChallenge(name) {
		return RRSet{}, fmt.Errorf("%w: %s names are reserved for certificate challenges", ErrInvalid, ChallengeLabel)
	}
	if in.TTL != nil && (*in.TTL < 60 || *in.TTL > 86400) {
		return RRSet{}, fmt.Errorf("%w: TTL must be between 60 and 86400 seconds", ErrInvalid)
	}
	if len(in.Values) == 0 {
		return RRSet{}, fmt.Errorf("%w: a record needs at least one value", ErrInvalid)
	}
	vals := make([]string, 0, len(in.Values))
	for _, v := range in.Values {
		nv, err := normalizeValue(typ, name, v)
		if err != nil {
			return RRSet{}, fmt.Errorf("%w: %w", ErrInvalid, err)
		}
		if slices.Contains(vals, nv) {
			return RRSet{}, fmt.Errorf("%w: value %q appears twice", ErrInvalid, nv)
		}
		vals = append(vals, nv)
	}
	if typ == "CNAME" {
		if len(vals) != 1 {
			return RRSet{}, fmt.Errorf("%w: a CNAME has exactly one target", ErrInvalid)
		}
		if name == Apex {
			return RRSet{}, fmt.Errorf("%w: the zone apex cannot be a CNAME", ErrInvalid)
		}
	}
	if typ == "SRV" && !srvName(name) {
		return RRSet{}, fmt.Errorf("%w: an SRV name starts with _service._protocol (e.g. _sip._tcp)", ErrInvalid)
	}
	return Normalize(RRSet{Name: name, Type: typ, TTL: in.TTL, Values: vals}), nil
}

// relativeName turns a name typed by the admin into the zone-relative form.
func relativeName(n, zone string) (string, error) {
	n = CanonicalName(n)
	zone = CanonicalName(zone)
	switch {
	case n == "" || n == Apex || n == zone:
		return Apex, nil
	case strings.HasSuffix(n, "."+zone):
		n = strings.TrimSuffix(n, "."+zone)
	}
	labels := strings.Split(n, ".")
	for i, l := range labels {
		if l == "*" && i == 0 {
			continue
		}
		if !validLabel(l) {
			return "", fmt.Errorf("%w: %q is not a valid DNS name inside %s", ErrInvalid, n, zone)
		}
	}
	if len(n)+len(zone)+1 > 253 {
		return "", fmt.Errorf("%w: the name is longer than 253 characters", ErrInvalid)
	}
	return n, nil
}

// validLabel accepts host labels plus a leading underscore (service labels such as _dmarc, _sip).
func validLabel(l string) bool {
	if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
		return false
	}
	for i, c := range l {
		ok := c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_' && i == 0
		if !ok {
			return false
		}
	}
	return true
}

func srvName(n string) bool {
	l := strings.Split(n, ".")
	return len(l) >= 2 && strings.HasPrefix(l[0], "_") && (l[1] == "_tcp" || l[1] == "_udp" || l[1] == "_tls")
}

// normalizeValue checks one value against the stored format of its type (03 §3) and returns it in that format.
func normalizeValue(typ, name, v string) (string, error) {
	v = strings.TrimSpace(v)
	switch typ {
	case "A", "AAAA":
		a, err := netip.ParseAddr(v)
		if err != nil || a.Zone() != "" || (typ == "A") != a.Is4() {
			return "", fmt.Errorf("%q is not an IPv%s address", v, map[bool]string{true: "4", false: "6"}[typ == "A"])
		}
		return a.String(), nil
	case "CNAME":
		return target(v)
	case "MX":
		f := strings.Fields(v)
		if len(f) != 2 || !uint16Field(f[0]) {
			return "", fmt.Errorf("%q: an MX value is \"priority target\", e.g. \"10 mail.example.com\"", v)
		}
		t, err := target(f[1])
		return f[0] + " " + t, err
	case "SRV":
		f := strings.Fields(v)
		if len(f) != 4 || !uint16Field(f[0]) || !uint16Field(f[1]) || !uint16Field(f[2]) {
			return "", fmt.Errorf("%q: an SRV value is \"priority weight port target\"", v)
		}
		t, err := target(f[3])
		return strings.Join(f[:3], " ") + " " + t, err
	case "CAA":
		f := strings.SplitN(v, " ", 3)
		if len(f) != 3 {
			return "", fmt.Errorf("%q: a CAA value is `flags tag \"value\"`", v)
		}
		if n, err := strconv.Atoi(f[0]); err != nil || n < 0 || n > 255 {
			return "", fmt.Errorf("%q: CAA flags are 0–255", v)
		}
		if f[1] != "issue" && f[1] != "issuewild" && f[1] != "iodef" {
			return "", fmt.Errorf("%q: the CAA tag is issue, issuewild or iodef", v)
		}
		val := strings.TrimSpace(f[2])
		if t, ok := DecodeTXT(val); ok {
			val = t
		}
		if strings.ContainsAny(val, "\"\\") {
			return "", fmt.Errorf("%q: the CAA value cannot contain quotes or backslashes", v)
		}
		return f[0] + " " + f[1] + ` "` + val + `"`, nil
	case "TXT":
		if len(v) > 4000 {
			return "", fmt.Errorf("a TXT value is limited to 4000 characters here")
		}
		return v, nil
	}
	return "", fmt.Errorf("type %s has no value format", typ)
}

// target is an absolute host name with its trailing dot, as the provider stores it (03 §3).
func target(t string) (string, error) {
	t = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(t)), ".")
	if t == "" {
		return "", fmt.Errorf("the target is empty")
	}
	for _, l := range strings.Split(t, ".") {
		if !validLabel(l) {
			return "", fmt.Errorf("%q is not a valid host name", t)
		}
	}
	return t + ".", nil
}

func uint16Field(s string) bool {
	n, err := strconv.Atoi(s)
	return err == nil && n >= 0 && n <= 65535
}

// CreateRecord stores a manual entry as a desired RRSet owned by Hashi (origin user). The caller then plans
// dns-records. An RRSet with the same name and type that exists at the provider but is not Hashi's is a conflict
// (import it instead); so is a CNAME beside other types at the same name.
func (s *Service) CreateRecord(ctx context.Context, actor, ip, zoneID string, in RecordInput) (Record, error) {
	z, err := s.Zone(ctx, zoneID)
	if err != nil {
		return Record{}, err
	}
	caps, err := s.capabilities(ctx, z.Provider)
	if err != nil {
		return Record{}, err
	}
	r, err := normalizeInput(in, z.Name, caps)
	if err != nil {
		return Record{}, err
	}
	id := ulid.Make().String()
	err = s.st.Tx(ctx, func(q *db.Queries) error {
		obs, err := s.observed(ctx, q, zoneID)
		if err != nil {
			return err
		}
		recs, err := q.ListDNSRecords(ctx, zoneID)
		if err != nil {
			return err
		}
		for _, rec := range recs {
			if rec.Name == r.Name && rec.Type == r.Type {
				return fmt.Errorf("%w: %s %s is already a Hashi entry; edit it instead", ErrConflict, r.Name, r.Type)
			}
		}
		owned, err := ownership(ctx, q, zoneID)
		if err != nil {
			return err
		}
		for _, o := range obs {
			if o.Name != r.Name {
				continue
			}
			if o.Type == r.Type && !owned[rrsetKey{o.Name, o.Type}].adopted() {
				return fmt.Errorf("%w: %s %s already exists at the provider; import it on the Import tab to manage it", ErrConflict, r.Name, r.Type)
			}
			if (r.Type == "CNAME") != (o.Type == "CNAME") && o.Type != r.Type {
				return fmt.Errorf("%w: %s has a %s record; a CNAME cannot share its name with other types", ErrConflict, r.Name, o.Type)
			}
		}
		for _, rec := range recs {
			if rec.Name == r.Name && (r.Type == "CNAME") != (rec.Type == "CNAME") {
				return fmt.Errorf("%w: %s has a %s entry; a CNAME cannot share its name with other types", ErrConflict, r.Name, rec.Type)
			}
		}
		now := s.now().UnixMilli()
		if err := q.InsertDNSRecord(ctx, recordParams(id, zoneID, r, OriginUser, in, now)); err != nil {
			return err
		}
		if err := q.UpsertDNSOwnership(ctx, db.UpsertDNSOwnershipParams{ZoneID: zoneID, Name: r.Name, Type: r.Type,
			Origin: OriginUser, CreatedAt: now, UpdatedAt: now}); err != nil {
			return err
		}
		return s.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "dns.record.created", IP: ip, Target: "dns_record:" + id,
			Detail: map[string]any{"name": r.Name, "type": r.Type}})
	})
	if err != nil {
		return Record{}, err
	}
	return s.Record(ctx, id)
}

// UpdateRecord changes a manual or imported entry's TTL, values and display fields. Name and type identify the RRSet
// and cannot change (delete and create instead).
func (s *Service) UpdateRecord(ctx context.Context, actor, ip, id string, in RecordInput) (Record, error) {
	cur, err := s.Record(ctx, id)
	if err != nil {
		return Record{}, err
	}
	if err := editable(cur); err != nil {
		return Record{}, err
	}
	z, err := s.Zone(ctx, cur.ZoneID)
	if err != nil {
		return Record{}, err
	}
	caps, err := s.capabilities(ctx, z.Provider)
	if err != nil {
		return Record{}, err
	}
	in.Name, in.Type = cur.RRSet.Name, cur.RRSet.Type
	r, err := normalizeInput(in, z.Name, caps)
	if err != nil {
		return Record{}, err
	}
	err = s.st.Tx(ctx, func(q *db.Queries) error {
		p := recordParams(id, cur.ZoneID, r, cur.Origin, in, s.now().UnixMilli())
		if err := q.UpdateDNSRecord(ctx, db.UpdateDNSRecordParams{Ttl: p.Ttl, ValuesJson: p.ValuesJson, DisplayName: p.DisplayName,
			Dashboard: p.Dashboard, Monitoring: p.Monitoring, UpdatedAt: p.UpdatedAt, ID: id}); err != nil {
			return err
		}
		return s.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "dns.record.updated", IP: ip, Target: "dns_record:" + id,
			Detail: map[string]any{"name": r.Name, "type": r.Type}})
	})
	if err != nil {
		return Record{}, err
	}
	return s.Record(ctx, id)
}

// DeleteRecord removes a manual or imported entry from the desired state; the next dns-records plan deletes the
// RRSet at the provider (high risk). An entry Hashi never created at the provider just disappears.
func (s *Service) DeleteRecord(ctx context.Context, actor, ip, id string) (Record, error) {
	cur, err := s.Record(ctx, id)
	if err != nil {
		return Record{}, err
	}
	if err := editable(cur); err != nil {
		return Record{}, err
	}
	err = s.st.Tx(ctx, func(q *db.Queries) error {
		if err := q.DeleteDNSRecord(ctx, id); err != nil {
			return err
		}
		owned, err := ownership(ctx, q, cur.ZoneID)
		if err != nil {
			return err
		}
		if o := owned[rrsetKey{cur.RRSet.Name, cur.RRSet.Type}]; o.present && !o.adopted() {
			if err := q.DeleteDNSOwnership(ctx, db.DeleteDNSOwnershipParams{ZoneID: cur.ZoneID, Name: cur.RRSet.Name, Type: cur.RRSet.Type}); err != nil {
				return err
			}
		}
		return s.audit.Record(ctx, q, audit.Event{Actor: actor, Action: "dns.record.deleted", IP: ip, Target: "dns_record:" + id,
			Detail: map[string]any{"name": cur.RRSet.Name, "type": cur.RRSet.Type}})
	})
	return cur, err
}

func editable(r Record) error {
	if r.Origin != OriginUser && r.Origin != OriginImported {
		return fmt.Errorf("%w: %s %s is generated (origin %s) and changes only through what generates it", ErrInvalid,
			r.RRSet.Name, r.RRSet.Type, r.Origin)
	}
	return nil
}

// Record returns one entry.
func (s *Service) Record(ctx context.Context, id string) (Record, error) {
	row, err := s.st.R.GetDNSRecord(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, ErrNoRecord
	}
	if err != nil {
		return Record{}, err
	}
	return recordFromRow(row)
}

// Records lists the zone's desired entries.
func (s *Service) Records(ctx context.Context, q *db.Queries, zoneID string) ([]Record, error) {
	rows, err := q.ListDNSRecords(ctx, zoneID)
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(rows))
	for _, r := range rows {
		rec, err := recordFromRow(r)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, nil
}

func recordFromRow(r db.DnsRecord) (Record, error) {
	rr := RRSet{Name: r.Name, Type: r.Type, Labels: map[string]string{}}
	if r.Ttl.Valid {
		t := int(r.Ttl.Int64)
		rr.TTL = &t
	}
	if err := json.Unmarshal([]byte(r.ValuesJson), &rr.Values); err != nil {
		return Record{}, fmt.Errorf("decode values of %s %s: %w", r.Name, r.Type, err)
	}
	return Record{ID: r.ID, ZoneID: r.ZoneID, RRSet: Normalize(rr), Origin: r.Origin, DisplayName: r.DisplayName,
		Dashboard: r.Dashboard == 1, Monitoring: r.Monitoring == 1, CreatedAt: time.UnixMilli(r.CreatedAt).UTC(),
		UpdatedAt: time.UnixMilli(r.UpdatedAt).UTC()}, nil
}

func recordParams(id, zoneID string, r RRSet, origin string, in RecordInput, now int64) db.InsertDNSRecordParams {
	vals, _ := json.Marshal(r.Values)
	var ttl sql.NullInt64
	if r.TTL != nil {
		ttl = sql.NullInt64{Int64: int64(*r.TTL), Valid: true}
	}
	return db.InsertDNSRecordParams{ID: id, ZoneID: zoneID, Name: r.Name, Type: r.Type, Ttl: ttl, ValuesJson: string(vals), Origin: origin,
		DisplayName: strings.TrimSpace(in.DisplayName), Dashboard: boolInt(in.Dashboard), Monitoring: boolInt(in.Monitoring),
		CreatedAt: now, UpdatedAt: now}
}

// owner is one dns_ownership row as the planner needs it.
type owner struct {
	present     bool
	origin      string
	appliedHash string
}

// adopted reports whether an observed RRSet with this key is Hashi's to change: imported explicitly, or created by
// Hashi (a user entry counts only once Hashi applied it). A user entry whose RRSet appeared at the provider before
// Hashi created it is a conflict, never an overwrite (invariant 1).
func (o owner) adopted() bool {
	return o.present && (o.origin == OriginImported || o.appliedHash != "")
}

func ownership(ctx context.Context, q *db.Queries, zoneID string) (map[rrsetKey]owner, error) {
	rows, err := q.ListDNSOwnership(ctx, zoneID)
	if err != nil {
		return nil, err
	}
	out := make(map[rrsetKey]owner, len(rows))
	for _, r := range rows {
		out[rrsetKey{r.Name, r.Type}] = owner{present: true, origin: r.Origin, appliedHash: r.LastAppliedHash}
	}
	return out, nil
}
