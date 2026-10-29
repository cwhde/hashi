package ipsets

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/store/db"
)

// Errors the API maps.
var (
	ErrNotFound = errors.New("IP set not found")
	// ErrLocked: system, feed and dynamic sets change only through their owners (05a §3, invariant 10).
	ErrLocked = errors.New("this IP set is not editable")
	ErrTaken  = errors.New("another IP set already uses this slug")
	// ErrInUse: a set (or host) is still referenced; the message lists the referencing objects.
	ErrInUse = errors.New("still in use")
)

// Consumer is something that references a set: a parent set, a firewall binding, a DNS record, a service target.
type Consumer struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Position string `json:"position"` // "match" or "target" (05a §5)
}

// Position names.
const (
	PositionMatch  = "match"
	PositionTarget = "target"
)

// ConsumerSource lists the consumers a feature keeps for a set. Features register one each (firewall, DNS,
// services); parent sets are known to the package itself.
type ConsumerSource func(ctx context.Context, q *db.Queries, setID string) ([]Consumer, error)

// HostLookup reports whether a firewall/edge host exists (host items reference it). Nil until hosts exist.
type HostLookup func(ctx context.Context, q *db.Queries, hostID string) (name string, err error)

var slugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// LoadGraph reads every set and element.
func LoadGraph(ctx context.Context, q *db.Queries) (Graph, error) {
	sets, err := q.ListIPSets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list IP sets: %w", err)
	}
	els, err := q.ListAllIPSetElements(ctx)
	if err != nil {
		return nil, fmt.Errorf("list IP set elements: %w", err)
	}
	g := make(Graph, len(sets))
	for _, r := range sets {
		s := setFromRow(r)
		g[s.ID] = &s
	}
	for _, r := range els {
		e, err := elementFromRow(r)
		if err != nil {
			return nil, err
		}
		if s, ok := g[e.SetID]; ok {
			s.Elements = append(s.Elements, e)
		}
	}
	return g, nil
}

// SetInput is what the user edits on a set.
type SetInput struct {
	Slug        string
	Name        string
	Description string
}

func (in SetInput) validate() (SetInput, error) {
	in.Slug = strings.TrimSpace(strings.ToLower(in.Slug))
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if !slugRe.MatchString(in.Slug) {
		return in, invalid("slug must be lowercase letters, digits and dashes (1–63 characters)")
	}
	if in.Name == "" || len(in.Name) > 120 {
		return in, invalid("name is required (at most 120 characters)")
	}
	if len(in.Description) > 1000 {
		return in, invalid("description is at most 1000 characters")
	}
	return in, nil
}

// CreateSet stores a new user set.
func CreateSet(ctx context.Context, q *db.Queries, in SetInput, now time.Time) (Set, error) {
	in, err := in.validate()
	if err != nil {
		return Set{}, err
	}
	if err := slugFree(ctx, q, in.Slug, ""); err != nil {
		return Set{}, err
	}
	id, ms := ulid.Make().String(), now.UnixMilli()
	if err := q.InsertIPSet(ctx, db.InsertIPSetParams{ID: id, Slug: in.Slug, Name: in.Name, Description: in.Description,
		Kind: KindUser, Origin: "user", CreatedAt: ms, UpdatedAt: ms}); err != nil {
		return Set{}, fmt.Errorf("store IP set: %w", err)
	}
	return GetSet(ctx, q, id)
}

// UpdateSet changes a user set's slug, name and description.
func UpdateSet(ctx context.Context, q *db.Queries, id string, in SetInput, now time.Time) (Set, error) {
	cur, err := GetSet(ctx, q, id)
	if err != nil {
		return Set{}, err
	}
	if !cur.Editable() {
		return Set{}, ErrLocked
	}
	if in, err = in.validate(); err != nil {
		return Set{}, err
	}
	if err := slugFree(ctx, q, in.Slug, id); err != nil {
		return Set{}, err
	}
	if err := q.UpdateIPSet(ctx, db.UpdateIPSetParams{Slug: in.Slug, Name: in.Name, Description: in.Description,
		UpdatedAt: now.UnixMilli(), ID: id}); err != nil {
		return Set{}, fmt.Errorf("update IP set: %w", err)
	}
	return GetSet(ctx, q, id)
}

// DeleteSet removes a user set that nothing references (05a §3).
func DeleteSet(ctx context.Context, q *db.Queries, id string, sources []ConsumerSource) (Set, error) {
	cur, err := GetSet(ctx, q, id)
	if err != nil {
		return Set{}, err
	}
	if !cur.Editable() {
		return Set{}, ErrLocked
	}
	cons, err := Consumers(ctx, q, id, sources)
	if err != nil {
		return Set{}, err
	}
	if len(cons) > 0 {
		names := make([]string, len(cons))
		for i, c := range cons {
			names[i] = c.Name
		}
		return Set{}, fmt.Errorf("%w: used by %s", ErrInUse, strings.Join(names, ", "))
	}
	return cur, q.DeleteIPSet(ctx, id)
}

// GetSet returns one set with its elements.
func GetSet(ctx context.Context, q *db.Queries, id string) (Set, error) {
	r, err := q.GetIPSet(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Set{}, ErrNotFound
	}
	if err != nil {
		return Set{}, err
	}
	s := setFromRow(r)
	els, err := q.ListIPSetElements(ctx, id)
	if err != nil {
		return Set{}, err
	}
	for _, er := range els {
		e, err := elementFromRow(er)
		if err != nil {
			return Set{}, err
		}
		s.Elements = append(s.Elements, e)
	}
	return s, nil
}

// Consumers lists parent sets plus what every registered source reports.
func Consumers(ctx context.Context, q *db.Queries, id string, sources []ConsumerSource) ([]Consumer, error) {
	parents, err := q.ListIPSetParents(ctx, sql.NullString{String: id, Valid: true})
	if err != nil {
		return nil, err
	}
	out := make([]Consumer, 0, len(parents))
	for _, p := range parents {
		out = append(out, Consumer{Type: "ip_set", ID: p.ID, Name: p.Name, Position: PositionMatch})
	}
	for _, src := range sources {
		c, err := src(ctx, q, id)
		if err != nil {
			return nil, err
		}
		out = append(out, c...)
	}
	return out, nil
}

// ElementInput is a new or changed element.
type ElementInput struct {
	Item      Item
	Note      string
	ExpiresAt *time.Time
}

// Editor checks element changes against the whole graph: references, cycles and target capability.
type Editor struct {
	Sources []ConsumerSource
	Hosts   HostLookup
}

// AddElement appends an element to a user set.
func (ed Editor) AddElement(ctx context.Context, q *db.Queries, setID string, in ElementInput, by string, now time.Time) (Element, error) {
	g, s, err := ed.editable(ctx, q, setID)
	if err != nil {
		return Element{}, err
	}
	e := Element{ID: ulid.Make().String(), SetID: setID, Origin: "user", CreatedBy: by, CreatedAt: now}
	if e, err = ed.prepare(ctx, q, g, s, e, in, now); err != nil {
		return Element{}, err
	}
	pos, err := q.NextIPSetElementPosition(ctx, setID)
	if err != nil {
		return Element{}, err
	}
	e.Position = int(pos)
	p := elementParams(e)
	ms := now.UnixMilli()
	if err := q.InsertIPSetElement(ctx, db.InsertIPSetElementParams{ID: e.ID, IpSetID: setID, Position: pos,
		Type: e.Item.Type(), Origin: e.Origin, Locked: 0, Address: p.address, Prefix: p.prefix, Family: p.family,
		HostID: p.hostID, Scope: p.scope, Fqdn: p.fqdn, ChildSetID: p.child, Note: e.Note, ExpiresAt: p.expires,
		CreatedBy: by, CreatedAt: ms, UpdatedAt: ms}); err != nil {
		return Element{}, fmt.Errorf("store element: %w", err)
	}
	return e, touch(ctx, q, setID, now)
}

// UpdateElement replaces an element's item, note and expiry.
func (ed Editor) UpdateElement(ctx context.Context, q *db.Queries, setID, elementID string, in ElementInput, now time.Time) (Element, error) {
	g, s, err := ed.editable(ctx, q, setID)
	if err != nil {
		return Element{}, err
	}
	var cur *Element
	for i := range s.Elements {
		if s.Elements[i].ID == elementID {
			cur = &s.Elements[i]
		}
	}
	if cur == nil {
		return Element{}, ErrNotFound
	}
	e, err := ed.prepare(ctx, q, g, s, *cur, in, now)
	if err != nil {
		return Element{}, err
	}
	p := elementParams(e)
	if err := q.UpdateIPSetElement(ctx, db.UpdateIPSetElementParams{Type: e.Item.Type(), Address: p.address,
		Prefix: p.prefix, Family: p.family, HostID: p.hostID, Scope: p.scope, Fqdn: p.fqdn, ChildSetID: p.child,
		Note: e.Note, ExpiresAt: p.expires, UpdatedAt: now.UnixMilli(), ID: e.ID}); err != nil {
		return Element{}, fmt.Errorf("update element: %w", err)
	}
	return e, touch(ctx, q, setID, now)
}

// DeleteElement removes an element from a user set. Removing never makes a set stop being an address set.
func (ed Editor) DeleteElement(ctx context.Context, q *db.Queries, setID, elementID string, now time.Time) (Element, error) {
	_, s, err := ed.editable(ctx, q, setID)
	if err != nil {
		return Element{}, err
	}
	for _, e := range s.Elements {
		if e.ID == elementID {
			if err := q.DeleteIPSetElement(ctx, elementID); err != nil {
				return Element{}, err
			}
			return e, touch(ctx, q, setID, now)
		}
	}
	return Element{}, ErrNotFound
}

func (ed Editor) editable(ctx context.Context, q *db.Queries, setID string) (Graph, *Set, error) {
	g, err := LoadGraph(ctx, q)
	if err != nil {
		return nil, nil, err
	}
	s, ok := g[setID]
	if !ok {
		return nil, nil, ErrNotFound
	}
	if !s.Editable() {
		return nil, nil, ErrLocked
	}
	return g, s, nil
}

// prepare normalizes the input into e and checks it against the graph with e in place.
func (ed Editor) prepare(ctx context.Context, q *db.Queries, g Graph, s *Set, e Element, in ElementInput, now time.Time) (Element, error) {
	it, err := Normalize(in.Item)
	if err != nil {
		return e, err
	}
	if len(in.Note) > 500 {
		return e, invalid("note is at most 500 characters")
	}
	if in.ExpiresAt != nil && !in.ExpiresAt.After(now) {
		return e, invalid("expiry must lie in the future")
	}
	switch v := it.(type) {
	case SetRef:
		if _, ok := g[v.SetID]; !ok {
			return e, invalid("referenced IP set does not exist")
		}
		if err := g.CheckReference(s.ID, v.SetID); err != nil {
			return e, err
		}
	case Host:
		if ed.Hosts == nil {
			return e, invalid("no firewall or edge host is registered")
		}
		if _, err := ed.Hosts(ctx, q, v.HostID); err != nil {
			return e, invalid("referenced host does not exist")
		}
	}
	e.Item, e.Note, e.ExpiresAt = it, strings.TrimSpace(in.Note), in.ExpiresAt
	if err := ed.keepsTargets(ctx, q, g, s, e); err != nil {
		return e, err
	}
	return e, nil
}

// keepsTargets refuses a change that makes a set used in a target position (or one of its ancestors that is) stop
// being an address set (05a §5.2).
func (ed Editor) keepsTargets(ctx context.Context, q *db.Queries, g Graph, s *Set, e Element) error {
	if Targetable(e.Item) {
		return nil
	}
	// The graph with the change applied.
	next := make(Graph, len(g))
	for id, v := range g {
		next[id] = v
	}
	changed := *s
	changed.Elements = nil
	replaced := false
	for _, x := range s.Elements {
		if x.ID == e.ID {
			x, replaced = e, true
		}
		changed.Elements = append(changed.Elements, x)
	}
	if !replaced {
		changed.Elements = append(changed.Elements, e)
	}
	next[s.ID] = &changed
	for id := range Ancestors(g, s.ID) {
		if !g.Compile(id, nil, time.Time{}).AddressSet || next.Compile(id, nil, time.Time{}).AddressSet {
			continue
		}
		cons, err := Consumers(ctx, q, id, ed.Sources)
		if err != nil {
			return err
		}
		var names []string
		for _, c := range cons {
			if c.Position == PositionTarget {
				names = append(names, c.Name)
			}
		}
		if len(names) > 0 {
			return fmt.Errorf("%w: %s is used as a target by %s, and targets take only single addresses (ip, host, dns)",
				ErrInvalid, g[id].Name, strings.Join(names, ", "))
		}
	}
	return nil
}

// Ancestors returns id and every set that includes it, directly or through nesting.
func Ancestors(g Graph, id string) map[string]bool {
	out := map[string]bool{id: true}
	for changed := true; changed; {
		changed = false
		for pid, p := range g {
			if out[pid] {
				continue
			}
			for _, e := range p.Elements {
				if ref, ok := e.Item.(SetRef); ok && out[ref.SetID] {
					out[pid], changed = true, true
					break
				}
			}
		}
	}
	return out
}

func slugFree(ctx context.Context, q *db.Queries, slug, self string) error {
	sets, err := q.ListIPSets(ctx)
	if err != nil {
		return err
	}
	for _, s := range sets {
		if s.Slug == slug && s.ID != self {
			return ErrTaken
		}
	}
	return nil
}

func touch(ctx context.Context, q *db.Queries, setID string, now time.Time) error {
	s, err := q.GetIPSet(ctx, setID)
	if err != nil {
		return err
	}
	return q.UpdateIPSet(ctx, db.UpdateIPSetParams{Slug: s.Slug, Name: s.Name, Description: s.Description,
		UpdatedAt: now.UnixMilli(), ID: setID})
}

type elementColumns struct {
	address, prefix, hostID, scope, fqdn, child sql.NullString
	family, expires                             sql.NullInt64
}

func elementParams(e Element) elementColumns {
	var c elementColumns
	str := func(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }
	switch v := e.Item.(type) {
	case IP:
		c.address, c.family = str(v.Addr.String()), family(v.Addr)
	case CIDR:
		c.prefix, c.family = str(v.Prefix.String()), family(v.Prefix.Addr())
	case Host:
		c.hostID, c.scope = str(v.HostID), str(v.Scope)
	case DNS:
		c.fqdn = str(v.FQDN)
	case SetRef:
		c.child = str(v.SetID)
	}
	if e.ExpiresAt != nil {
		c.expires = sql.NullInt64{Int64: e.ExpiresAt.UnixMilli(), Valid: true}
	}
	return c
}

func family(a netip.Addr) sql.NullInt64 {
	if a.Is4() {
		return sql.NullInt64{Int64: 4, Valid: true}
	}
	return sql.NullInt64{Int64: 6, Valid: true}
}

func setFromRow(r db.IpSet) Set {
	return Set{ID: r.ID, Slug: r.Slug, Name: r.Name, Description: r.Description, Kind: r.Kind, Origin: r.Origin,
		SystemKey: r.SystemKey.String, CreatedAt: time.UnixMilli(r.CreatedAt).UTC(), UpdatedAt: time.UnixMilli(r.UpdatedAt).UTC()}
}

func elementFromRow(r db.IpSetElement) (Element, error) {
	e := Element{ID: r.ID, SetID: r.IpSetID, Position: int(r.Position), Origin: r.Origin, Locked: r.Locked == 1,
		Note: r.Note, CreatedBy: r.CreatedBy, CreatedAt: time.UnixMilli(r.CreatedAt).UTC()}
	if r.ExpiresAt.Valid {
		t := time.UnixMilli(r.ExpiresAt.Int64).UTC()
		e.ExpiresAt = &t
	}
	var err error
	switch r.Type {
	case TypeIP:
		var a netip.Addr
		if a, err = netip.ParseAddr(r.Address.String); err == nil {
			e.Item = IP{Addr: a}
		}
	case TypeCIDR:
		var p netip.Prefix
		if p, err = netip.ParsePrefix(r.Prefix.String); err == nil {
			e.Item = CIDR{Prefix: p}
		}
	case TypeHost:
		e.Item = Host{HostID: r.HostID.String, Scope: r.Scope.String}
	case TypeDNS:
		e.Item = DNS{FQDN: r.Fqdn.String}
	case TypeSet:
		e.Item = SetRef{SetID: r.ChildSetID.String}
	default:
		// pulse and geo arrive with later slices; their rows cannot exist yet.
		err = fmt.Errorf("item type %q is not supported yet", r.Type)
	}
	if err != nil {
		return e, fmt.Errorf("element %s: %w", r.ID, err)
	}
	return e, nil
}

func nullStr(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }
