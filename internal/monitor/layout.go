package monitor

import (
	"cmp"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// The public layout (07 §7): one order of groups and entries for both public pages. It holds order and groups only;
// the Apps switch is the service's dashboard_visible or the DNS entry's dashboard, the Status switch the entry's
// endpoint's public flag.

// Entry kinds: the subject an entry stands for.
const (
	EntryService  = "service"
	EntryDNS      = "dns"
	EntryEndpoint = "endpoint"
)

// OtherGroup is the name shown for entries without a group; it is never stored.
const OtherGroup = "Other"

// ErrLayoutStale: the layout changed since the editor loaded it (07 §7, 409).
var ErrLayoutStale = errors.New("the public layout changed since it was loaded")

// LayoutEntry is one entry with what the editor and the public pages need.
type LayoutEntry struct {
	Key  string // kind ":" id, the entry's identity in the API
	Kind string // EntryService, EntryDNS or EntryEndpoint
	ID   string
	Name string
	// Type is what it is to a visitor: web, link (a DNS entry), tcp, udp or check (a host or a user monitor).
	Type               string
	Apps, Status       bool
	CanApps, CanStatus bool
	URL                string // the address the apps page links to; "" when Apps is not possible
	Slug               string // the service page's address part, unique within the layout
	// IconType and IconID key the app icon (07 §9.1): a service or a DNS entry; empty for other entries.
	IconType, IconID string

	endpoint *db.MonitorEndpoint
}

// Endpoint is the entry's check, if it has one.
func (e LayoutEntry) Endpoint() *db.MonitorEndpoint { return e.endpoint }

// LayoutGroup is a named group, or "Other" (ID "") last.
type LayoutGroup struct {
	ID      string
	Name    string
	Entries []LayoutEntry
}

// Layout is the whole order with its version.
type Layout struct {
	Version int64
	Groups  []LayoutGroup
}

// subjects lists every entry the layout must hold, unordered (07 §7 Entries).
func subjects(ctx context.Context, q *db.Queries) (map[string]LayoutEntry, error) {
	eps, err := q.ListMonitorEndpoints(ctx)
	if err != nil {
		return nil, err
	}
	bySource := map[string]*db.MonitorEndpoint{}
	out := map[string]LayoutEntry{}
	for i := range eps {
		e := &eps[i]
		switch e.SourceType {
		case SourceService, SourceDNS:
			bySource[e.SourceType+"|"+e.SourceID] = e
		default:
			k := EntryEndpoint + ":" + e.ID
			out[k] = LayoutEntry{Key: k, Kind: EntryEndpoint, ID: e.ID, Name: e.Name, Type: "check", CanStatus: true,
				Status: e.Public == 1, endpoint: e}
		}
	}
	svcs, err := services.List(ctx, q)
	if err != nil {
		return nil, err
	}
	for _, v := range svcs {
		if v.System() {
			continue
		}
		k := EntryService + ":" + v.ID
		e := LayoutEntry{Key: k, Kind: EntryService, ID: v.ID, Name: v.Name, Type: "web", Slug: v.Slug,
			IconType: SourceService, IconID: v.ID, endpoint: bySource[SourceService+"|"+v.ID]}
		switch v.Kind {
		case services.KindTCP:
			e.Type = "tcp"
		case services.KindUDP:
			e.Type = "udp"
		}
		if e.Type == "web" && v.HostName != "" {
			e.CanApps, e.Apps = true, v.DashboardVisible
			e.URL = "https://" + strings.TrimSuffix(v.HostName, ".") + "/"
		}
		if e.endpoint != nil {
			e.CanStatus, e.Status = true, e.endpoint.Public == 1
		}
		out[k] = e
	}
	recs, err := q.ListLayoutDNSRecords(ctx)
	if err != nil {
		return nil, err
	}
	for _, r := range recs {
		host := r.ZoneName
		if r.Name != "@" {
			host = r.Name + "." + r.ZoneName
		}
		k := EntryDNS + ":" + r.ID
		e := LayoutEntry{Key: k, Kind: EntryDNS, ID: r.ID, Name: cmp.Or(r.DisplayName, host), Type: "link",
			CanApps: true, Apps: r.Dashboard == 1, URL: "https://" + host + "/", IconType: SourceDNS, IconID: r.ID,
			endpoint: bySource[SourceDNS+"|"+r.ID]}
		if e.endpoint != nil {
			e.CanStatus, e.Status = true, e.endpoint.Public == 1
		}
		out[k] = e
	}
	return out, nil
}

// readLayout puts the subjects into the stored order. Entries not stored yet are left out; syncLayout adds them.
func readLayout(ctx context.Context, q *db.Queries) (Layout, error) {
	meta, err := q.GetPublicLayout(ctx)
	if err != nil {
		return Layout{}, fmt.Errorf("read public layout: %w", err)
	}
	all, err := subjects(ctx, q)
	if err != nil {
		return Layout{}, err
	}
	groups, err := q.ListPublicGroups(ctx)
	if err != nil {
		return Layout{}, err
	}
	rows, err := q.ListPublicEntries(ctx)
	if err != nil {
		return Layout{}, err
	}
	out := Layout{Version: meta.Version}
	at := map[string]int{}
	for _, g := range groups {
		at[g.ID] = len(out.Groups)
		out.Groups = append(out.Groups, LayoutGroup{ID: g.ID, Name: g.Name, Entries: []LayoutEntry{}})
	}
	out.Groups = append(out.Groups, LayoutGroup{Name: OtherGroup, Entries: []LayoutEntry{}})
	slugs := map[string]int{}
	for _, r := range rows {
		e, ok := all[r.SubjectType+":"+r.SubjectID]
		if !ok {
			continue
		}
		e.Slug = uniqueSlug(slugs, cmp.Or(e.Slug, Slugify(e.Name)))
		i := len(out.Groups) - 1
		if j, ok := at[r.GroupID.String]; r.GroupID.Valid && ok {
			i = j
		}
		out.Groups[i].Entries = append(out.Groups[i].Entries, e)
	}
	return out, nil
}

// Slugify makes a service page's address part from a name: lowercase letters and digits joined by single dashes.
func Slugify(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			dash = false
		} else {
			dash = true
		}
	}
	return cmp.Or(b.String(), "entry")
}

func uniqueSlug(seen map[string]int, s string) string {
	seen[s]++
	if n := seen[s]; n > 1 {
		return fmt.Sprintf("%s-%d", s, n)
	}
	return s
}

// syncLayout keeps the layout complete (07 §7): the first run fills it from the endpoints' groups and the old category
// order; later runs drop entries whose subject is gone, add new ones at the end of "Other" (a user monitor joins the
// group it was created with), follow a Group changed in an endpoint's settings, and write every entry's group to its
// endpoint. It bumps the version only when the order changed.
func syncLayout(ctx context.Context, q *db.Queries, now int64) error {
	meta, err := q.GetPublicLayout(ctx)
	if err != nil {
		return fmt.Errorf("read public layout: %w", err)
	}
	all, err := subjects(ctx, q)
	if err != nil {
		return err
	}
	if meta.Seeded == 0 {
		return seedLayout(ctx, q, all, now)
	}
	cur, err := readLayout(ctx, q)
	if err != nil {
		return err
	}
	changed := false
	held := map[string]bool{}
	for gi := range cur.Groups {
		for _, e := range cur.Groups[gi].Entries {
			held[e.Key] = true
		}
	}
	rows, err := q.ListPublicEntries(ctx)
	if err != nil {
		return err
	}
	if len(rows) != len(held) {
		changed = true // some subject is gone
	}
	// An endpoint whose group differs from its entry's was moved in the endpoint's settings: follow it. Moves are
	// collected first: placing one may add a group.
	var moves []LayoutEntry
	for gi := range cur.Groups {
		g := &cur.Groups[gi]
		keep := g.Entries[:0:0]
		for _, e := range g.Entries {
			if ep := e.endpoint; ep != nil && ep.GroupName != g.Name {
				moves = append(moves, e)
				continue
			}
			keep = append(keep, e)
		}
		g.Entries = keep
	}
	for _, e := range moves {
		cur.place(e, e.endpoint.GroupName)
		changed = true
	}
	news := []LayoutEntry{}
	for k, e := range all {
		if !held[k] {
			news = append(news, e)
		}
	}
	slices.SortFunc(news, func(a, b LayoutEntry) int { return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	for _, e := range news {
		group := OtherGroup
		if ep := e.endpoint; ep != nil && ep.SourceType == SourceUser {
			group = ep.GroupName
		}
		cur.place(e, group)
		changed = true
	}
	if !changed {
		return writeEndpointGroups(ctx, q, cur, now)
	}
	return writeLayout(ctx, q, cur, meta.Version+1, now)
}

// place appends an entry to the named group, creating the group before "Other" when it does not exist.
func (l *Layout) place(e LayoutEntry, group string) {
	group = strings.TrimSpace(group)
	if group == "" {
		group = OtherGroup
	}
	for i := range l.Groups {
		if l.Groups[i].Name == group {
			l.Groups[i].Entries = append(l.Groups[i].Entries, e)
			return
		}
	}
	n := len(l.Groups) - 1
	l.Groups = slices.Insert(l.Groups, n, LayoutGroup{Name: group, Entries: []LayoutEntry{e}})
}

// seedLayout fills the layout once from what came before (07 §7 Migration).
func seedLayout(ctx context.Context, q *db.Queries, all map[string]LayoutEntry, now int64) error {
	page, err := settings.Get[settings.StatusPage](ctx, q)
	if err != nil {
		return err
	}
	l := Layout{Groups: []LayoutGroup{{Name: OtherGroup}}}
	list := make([]LayoutEntry, 0, len(all))
	for _, e := range all {
		list = append(list, e)
	}
	group := func(e LayoutEntry) string {
		if e.endpoint != nil {
			return e.endpoint.GroupName
		}
		if e.Kind == EntryDNS {
			return "DNS entries"
		}
		return "Services"
	}
	rank := func(g string) int {
		if i := slices.Index(page.Order, g); i >= 0 {
			return i
		}
		return len(page.Order)
	}
	slices.SortFunc(list, func(a, b LayoutEntry) int {
		ga, gb := group(a), group(b)
		return cmp.Or(cmp.Compare(rank(ga), rank(gb)), cmp.Compare(ga, gb),
			cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)), cmp.Compare(a.Key, b.Key))
	})
	for _, e := range list {
		l.place(e, group(e))
		// A hidden category's entries leave the status page (the hidden list goes, 07 §7 Migration).
		if ep := e.endpoint; ep != nil && ep.Public == 1 && slices.Contains(page.Hidden, ep.GroupName) {
			if err := q.SetMonitorEndpointPublic(ctx, db.SetMonitorEndpointPublicParams{Public: 0, UpdatedAt: now, ID: ep.ID}); err != nil {
				return err
			}
		}
	}
	return writeLayout(ctx, q, l, 1, now)
}

// writeLayout replaces the stored order with l (group IDs kept, new groups get one) and sets the version.
func writeLayout(ctx context.Context, q *db.Queries, l Layout, version, now int64) error {
	if err := q.DeletePublicEntries(ctx); err != nil {
		return err
	}
	if err := q.DeletePublicGroups(ctx); err != nil {
		return err
	}
	pos := int64(0)
	for gi, g := range l.Groups {
		var gid sql.NullString
		if g.Name != OtherGroup {
			if g.ID == "" {
				g.ID = ulid.Make().String()
			}
			gid = sql.NullString{String: g.ID, Valid: true}
			if err := q.InsertPublicGroup(ctx, db.InsertPublicGroupParams{ID: g.ID, Name: g.Name, Position: int64(gi)}); err != nil {
				return fmt.Errorf("store group %q: %w", g.Name, err)
			}
		}
		for _, e := range g.Entries {
			pos++
			if err := q.InsertPublicEntry(ctx, db.InsertPublicEntryParams{SubjectType: e.Kind, SubjectID: e.ID, GroupID: gid, Position: pos}); err != nil {
				return fmt.Errorf("store entry %s: %w", e.Key, err)
			}
		}
	}
	if err := q.SetPublicLayoutVersion(ctx, version); err != nil {
		return err
	}
	return writeEndpointGroups(ctx, q, l, now)
}

// writeEndpointGroups gives every entry's endpoint its group's name, so notifications and older readers agree.
func writeEndpointGroups(ctx context.Context, q *db.Queries, l Layout, now int64) error {
	for _, g := range l.Groups {
		for _, e := range g.Entries {
			if ep := e.endpoint; ep != nil && ep.GroupName != g.Name {
				if err := q.SetMonitorEndpointGroup(ctx, db.SetMonitorEndpointGroupParams{GroupName: g.Name, UpdatedAt: now, ID: ep.ID}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// Layout returns the public layout for the editor.
func (s *Service) Layout(ctx context.Context) (Layout, error) {
	return readLayout(ctx, s.st.R)
}

// LayoutSave is one save from the editor (07 §8.1): the whole order and any switch changes.
type LayoutSave struct {
	Version  int64
	Groups   []LayoutSaveGroup // named groups in order; "Other" is not among them
	Other    []string          // entry keys in "Other", in order
	Switches []LayoutSwitch
}

// LayoutSaveGroup is one named group with its entry keys in order. ID is empty for a new group.
type LayoutSaveGroup struct {
	ID      string
	Name    string
	Entries []string
}

// LayoutSwitch turns an entry's Apps or Status switch; nil leaves it.
type LayoutSwitch struct {
	Entry        string
	Apps, Status *bool
}

// SaveLayout stores a new order and switch changes in one transaction. A stale version is refused; an entry the
// request leaves out keeps existing at the end of "Other"; a switch that is not possible is refused.
func (s *Service) SaveLayout(ctx context.Context, in LayoutSave) (Layout, error) {
	now := s.now().UnixMilli()
	var out Layout
	err := s.st.Tx(ctx, func(q *db.Queries) error {
		cur, err := readLayout(ctx, q)
		if err != nil {
			return err
		}
		if in.Version != cur.Version {
			return ErrLayoutStale
		}
		byKey := map[string]LayoutEntry{}
		for _, g := range cur.Groups {
			for _, e := range g.Entries {
				byKey[e.Key] = e
			}
		}
		used := map[string]bool{}
		take := func(keys []string) ([]LayoutEntry, error) {
			list := make([]LayoutEntry, 0, len(keys))
			for _, k := range keys {
				e, ok := byKey[k]
				if !ok {
					return nil, errInvalid(fmt.Sprintf("unknown entry %q", k))
				}
				if used[k] {
					return nil, errInvalid(fmt.Sprintf("entry %q appears twice", k))
				}
				used[k] = true
				list = append(list, e)
			}
			return list, nil
		}
		next := Layout{}
		names := map[string]bool{}
		known := map[string]bool{}
		for _, g := range cur.Groups {
			known[g.ID] = true
		}
		for _, g := range in.Groups {
			name := strings.TrimSpace(g.Name)
			switch {
			case name == "":
				return errInvalid("a group needs a name")
			case strings.EqualFold(name, OtherGroup):
				return errInvalid(`"Other" is kept for entries without a group`)
			case utf8.RuneCountInString(name) > 60:
				return errInvalid("a group name has at most 60 characters")
			case names[strings.ToLower(name)]:
				return errInvalid(fmt.Sprintf("two groups are called %q", name))
			case g.ID != "" && !known[g.ID]:
				return errInvalid(fmt.Sprintf("unknown group %q", g.ID))
			}
			names[strings.ToLower(name)] = true
			list, err := take(g.Entries)
			if err != nil {
				return err
			}
			next.Groups = append(next.Groups, LayoutGroup{ID: g.ID, Name: name, Entries: list})
		}
		other, err := take(in.Other)
		if err != nil {
			return err
		}
		for _, g := range cur.Groups { // left out: kept, at the end of "Other"
			for _, e := range g.Entries {
				if !used[e.Key] {
					other = append(other, e)
				}
			}
		}
		next.Groups = append(next.Groups, LayoutGroup{Name: OtherGroup, Entries: other})
		for _, sw := range in.Switches {
			e, ok := byKey[sw.Entry]
			if !ok {
				return errInvalid(fmt.Sprintf("unknown entry %q", sw.Entry))
			}
			if err := setSwitches(ctx, q, e, sw, now); err != nil {
				return err
			}
		}
		if err := writeLayout(ctx, q, next, cur.Version+1, now); err != nil {
			return err
		}
		out, err = readLayout(ctx, q)
		return err
	})
	if err != nil {
		return Layout{}, err
	}
	s.statusCache.reset()
	s.appsCache.reset()
	return out, nil
}

func setSwitches(ctx context.Context, q *db.Queries, e LayoutEntry, sw LayoutSwitch, now int64) error {
	if sw.Apps != nil && *sw.Apps != e.Apps {
		if !e.CanApps {
			return errInvalid(fmt.Sprintf("%s has no web address for the apps page", e.Name))
		}
		var err error
		if e.Kind == EntryDNS {
			err = q.SetDNSRecordDashboard(ctx, db.SetDNSRecordDashboardParams{Dashboard: flag(*sw.Apps), UpdatedAt: now, ID: e.ID})
		} else {
			_, err = q.SetServiceDashboard(ctx, db.SetServiceDashboardParams{DashboardVisible: flag(*sw.Apps), UpdatedAt: now, ID: e.ID})
		}
		if err != nil {
			return err
		}
	}
	if sw.Status != nil && *sw.Status != e.Status {
		if !e.CanStatus {
			return errInvalid(fmt.Sprintf("%s has no check for the status page", e.Name))
		}
		if err := q.SetMonitorEndpointPublic(ctx, db.SetMonitorEndpointPublicParams{Public: flag(*sw.Status), UpdatedAt: now, ID: e.endpoint.ID}); err != nil {
			return err
		}
	}
	return nil
}
