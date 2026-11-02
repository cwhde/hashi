package rules

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// HTTP-family service kinds own a root rule set (06 §4.2).
func httpKind(k string) bool { return k == "http" || k == "https" || k == "h2c" }

// Load reads every rule set with its entries.
func Load(ctx context.Context, q *db.Queries) (Graph, error) {
	sets, err := q.ListRuleSets(ctx)
	if err != nil {
		return nil, fmt.Errorf("list rule sets: %w", err)
	}
	g := make(Graph, len(sets))
	for _, r := range sets {
		g[r.ID] = &Set{ID: r.ID, Slug: r.Slug, Name: r.Name, Description: r.Description, Kind: r.Kind, Origin: r.Origin, ServiceID: r.ServiceID.String}
	}
	entries, err := q.ListRuleEntries(ctx)
	if err != nil {
		return nil, fmt.Errorf("list rule entries: %w", err)
	}
	for _, r := range entries {
		s, ok := g[r.RuleSetID]
		if !ok {
			continue
		}
		e := Entry{ID: r.ID, Priority: int(r.Priority), Type: r.EntryType, ChildID: r.ChildRuleSetID.String, AppliesTo: r.AppliesToSetID.String,
			MatchType: r.MatchType.String, Pattern: r.Pattern.String, Action: r.Action.String, Provider: r.AuthProviderID.String,
			Origin: r.Origin, Locked: r.Locked == 1, Fallback: r.IsFallback == 1, Enabled: r.Enabled == 1, Note: r.Note}
		if err := json.Unmarshal([]byte(r.AuthGroupsJson), &e.Groups); err != nil {
			return nil, fmt.Errorf("rule entry %s: %w", r.ID, err)
		}
		s.Entries = append(s.Entries, e)
	}
	return g, nil
}

// EnsureRoots gives every HTTP-family service its root rule set with an open fallback. Idempotent: it runs at start for
// services older than rule sets and after a service is created.
func EnsureRoots(ctx context.Context, q *db.Queries, now time.Time) error {
	svcs, err := q.ListServices(ctx)
	if err != nil {
		return fmt.Errorf("list services: %w", err)
	}
	sets, err := q.ListRuleSets(ctx)
	if err != nil {
		return fmt.Errorf("list rule sets: %w", err)
	}
	all, err := q.GetIPSetBySystemKey(ctx, sql.NullString{String: "all", Valid: true})
	if err != nil {
		return fmt.Errorf("the all set: %w", err)
	}
	ms := now.UnixMilli()
	for _, s := range svcs {
		if !httpKind(s.Kind) || slices.ContainsFunc(sets, func(r db.RuleSet) bool { return r.ServiceID.String == s.ID }) {
			continue
		}
		id := ulid.Make().String()
		if err := q.InsertRuleSet(ctx, db.InsertRuleSetParams{ID: id, Slug: "root-" + strings.ToLower(s.ID), Name: "Rules of " + s.Name,
			Kind: KindServiceRoot, Origin: OriginSystem, ServiceID: nullStr(s.ID), CreatedAt: ms, UpdatedAt: ms}); err != nil {
			return fmt.Errorf("root rule set of %s: %w", s.Slug, err)
		}
		if err := insertEntry(ctx, q, id, Entry{ID: ulid.Make().String(), Priority: 1, Type: EntryItem, AppliesTo: all.ID, Action: ActionNone,
			Origin: OriginSystem, Locked: true, Fallback: true, Enabled: true}, ms); err != nil {
			return fmt.Errorf("fallback of %s: %w", s.Slug, err)
		}
	}
	return nil
}

func insertEntry(ctx context.Context, q *db.Queries, setID string, e Entry, ms int64) error {
	groups := e.Groups
	if groups == nil {
		groups = []string{}
	}
	gj, err := json.Marshal(groups)
	if err != nil {
		return err
	}
	return q.InsertRuleEntry(ctx, db.InsertRuleEntryParams{ID: e.ID, RuleSetID: setID, Priority: int64(e.Priority), EntryType: e.Type,
		ChildRuleSetID: optStr(e.ChildID), AppliesToSetID: optStr(e.AppliesTo), MatchType: optStr(e.MatchType), Pattern: optStr(e.Pattern),
		Action: optStr(e.Action), AuthProviderID: optStr(e.Provider), AuthGroupsJson: string(gj), Origin: e.Origin, Locked: b2i(e.Locked),
		IsFallback: b2i(e.Fallback), Enabled: b2i(e.Enabled), Note: e.Note, CreatedAt: ms, UpdatedAt: ms})
}

// normalize validates one user entry against the stored graph.
func normalize(ctx context.Context, q *db.Queries, g Graph, parent string, e Entry) (Entry, error) {
	if e.Type == EntrySet {
		c, ok := g[e.ChildID]
		if !ok {
			return e, invalid("the attached rule set does not exist")
		}
		if c.Kind != KindUser {
			return e, invalid("only reusable rule sets can be attached")
		}
		if err := g.CheckReference(parent, e.ChildID); err != nil {
			return e, err
		}
		return Entry{ID: e.ID, Type: EntrySet, ChildID: e.ChildID, Origin: OriginUser, Enabled: true, Note: e.Note}, nil
	}
	if e.Type != EntryItem {
		return e, invalid("unknown entry type %q", e.Type)
	}
	if _, err := q.GetIPSet(ctx, e.AppliesTo); err != nil {
		return e, invalid("the IP set of a rule does not exist")
	}
	switch e.MatchType {
	case "":
		e.Pattern = ""
	case MatchExact, MatchPrefix, MatchRegex:
		if !strings.HasPrefix(e.Pattern, "/") {
			return e, invalid("a path must start with /")
		}
		if e.MatchType == MatchRegex {
			if _, err := regexp.Compile(e.Pattern); err != nil {
				return e, invalid("the path pattern is not a valid regular expression: %v", err)
			}
		}
	default:
		return e, invalid("path match must be exact, prefix or regex")
	}
	switch e.Action {
	case ActionNone, ActionDeny:
		e.Provider, e.Groups = "", nil
	case ActionCaptcha:
		e.Provider, e.Groups = "", nil
		// No CAPTCHA in front of CAPTCHA (06 §11): not in the challenge service's or the Cap dashboard's own rules.
		if p, ok := g[parent]; ok && p.ServiceID != "" {
			c, err := settings.Get[settings.Captcha](ctx, q)
			if err != nil {
				return e, err
			}
			sv, err := q.GetService(ctx, p.ServiceID)
			if err != nil {
				return e, err
			}
			if p.ServiceID == c.AdminServiceID || (sv.Origin == "system" && sv.OwnerFeature == "captcha") {
				return e, invalid("Pass CAPTCHA cannot protect Cap itself: the CAPTCHA would need itself to load")
			}
		}
	case ActionAuth:
		if e.Provider != "" {
			if _, err := q.GetOIDCProvider(ctx, e.Provider); err != nil {
				return e, invalid("the sign-in provider of a rule does not exist")
			}
		}
	default:
		return e, invalid("unknown action %q", e.Action)
	}
	e.Origin, e.Locked, e.Fallback, e.ChildID = OriginUser, false, false, ""
	return e, nil
}

// PutEntries replaces a set's entries in the given order; priorities are renumbered from 1. A service root keeps its
// locked fallback last with the given action. isSystemService refuses a system service's root (invariant 10).
func PutEntries(ctx context.Context, q *db.Queries, setID string, entries []Entry, fallback string, isSystemService bool, now time.Time) error {
	g, err := Load(ctx, q)
	if err != nil {
		return err
	}
	s, ok := g[setID]
	if !ok {
		return ErrNotFound
	}
	if isSystemService {
		return ErrLocked
	}
	var keep *Entry
	for _, e := range s.Entries {
		if e.Fallback {
			kept := e
			keep = &kept
		}
	}
	if s.Kind == KindServiceRoot {
		if keep == nil {
			return fmt.Errorf("root rule set %s has no fallback", setID)
		}
		if fallback != ActionNone && fallback != ActionDeny {
			return invalid("the fallback must be none or deny")
		}
		keep.Action = fallback
	}
	// The graph the cycle check sees must not contain this set's old references.
	s.Entries = nil
	out := make([]Entry, 0, len(entries)+1)
	for _, in := range entries {
		e, err := normalize(ctx, q, g, setID, in)
		if err != nil {
			return err
		}
		e.ID = ulid.Make().String()
		e.Priority = len(out) + 1
		out = append(out, e)
		s.Entries = append(s.Entries, e)
	}
	if keep != nil {
		keep.ID, keep.Priority = ulid.Make().String(), len(out)+1
		out = append(out, *keep)
	}
	if err := q.DeleteRuleEntries(ctx, setID); err != nil {
		return fmt.Errorf("replace entries: %w", err)
	}
	ms := now.UnixMilli()
	for _, e := range out {
		if err := insertEntry(ctx, q, setID, e, ms); err != nil {
			return fmt.Errorf("store entry: %w", err)
		}
	}
	return nil
}

var slugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// Create stores a reusable rule set.
func Create(ctx context.Context, q *db.Queries, slug, name, description string, now time.Time) (string, error) {
	if !slugRe.MatchString(slug) || strings.HasPrefix(slug, "root-") || slug == GlobalID {
		return "", invalid("the slug must be lowercase letters, digits and dashes")
	}
	if strings.TrimSpace(name) == "" {
		name = slug
	}
	id, ms := ulid.Make().String(), now.UnixMilli()
	err := q.InsertRuleSet(ctx, db.InsertRuleSetParams{ID: id, Slug: slug, Name: name, Description: description, Kind: KindUser, Origin: OriginUser, CreatedAt: ms, UpdatedAt: ms})
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return "", ErrTaken
	}
	return id, err
}

// Update changes a reusable set's slug, name and description.
func Update(ctx context.Context, q *db.Queries, id, slug, name, description string, now time.Time) error {
	r, err := q.GetRuleSet(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if r.Kind != KindUser {
		return ErrLocked
	}
	if !slugRe.MatchString(slug) || strings.HasPrefix(slug, "root-") || slug == GlobalID {
		return invalid("the slug must be lowercase letters, digits and dashes")
	}
	err = q.UpdateRuleSet(ctx, db.UpdateRuleSetParams{Slug: slug, Name: name, Description: description, UpdatedAt: now.UnixMilli(), ID: id})
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		return ErrTaken
	}
	return err
}

// Parents lists the sets that attach id.
func (g Graph) Parents(id string) []*Set {
	var out []*Set
	for _, s := range g {
		if slices.ContainsFunc(s.Entries, func(e Entry) bool { return e.Type == EntrySet && e.ChildID == id }) {
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(a, b *Set) int { return strings.Compare(a.Slug, b.Slug) })
	return out
}

// Delete removes a reusable set that nothing attaches; the error names where it is attached.
func Delete(ctx context.Context, q *db.Queries, id string) error {
	g, err := Load(ctx, q)
	if err != nil {
		return err
	}
	s, ok := g[id]
	if !ok {
		return ErrNotFound
	}
	if s.Kind != KindUser {
		return ErrLocked
	}
	if ps := g.Parents(id); len(ps) > 0 {
		names := make([]string, 0, len(ps))
		for _, p := range ps {
			names = append(names, p.Name)
		}
		return fmt.Errorf("%w: attached to %s", ErrInUse, strings.Join(names, ", "))
	}
	return q.DeleteRuleSet(ctx, id)
}

func nullStr(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }

func optStr(s string) sql.NullString { return sql.NullString{String: s, Valid: s != ""} }

func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
