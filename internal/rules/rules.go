// Package rules owns rule sets (06 §4): the types, the cycle check, flattening and first-match evaluation. Global, service
// root and reusable sets are one type; forward auth and the trace both evaluate through this package.
package rules

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Kinds and the fixed ID of the global set.
const (
	KindUser        = "user"
	KindGlobal      = "global"
	KindServiceRoot = "service_root"
	GlobalID        = "global"
)

// Entry types, actions and path matches (06 §4.1).
const (
	EntryItem = "item"
	EntrySet  = "set"

	ActionNone     = "none"
	ActionDeny     = "deny"
	ActionCaptcha  = "pass_captcha"
	ActionAuth     = "pass_authentication"
	MatchExact     = "exact"
	MatchPrefix    = "prefix"
	MatchRegex     = "regex"
	OriginSystem   = "system"
	OriginUser     = "user"
	rootPathPrefix = "/"
)

// Errors the API maps.
var (
	ErrInvalid  = errors.New("invalid rule")
	ErrNotFound = errors.New("rule set not found")
	ErrLocked   = errors.New("this rule set cannot be changed here")
	ErrInUse    = errors.New("rule set is in use")
	ErrTaken    = errors.New("another rule set already uses this slug")
)

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// Entry is a rule item or a reference to another rule set, at a priority unique in its set.
type Entry struct {
	ID       string
	Priority int
	Type     string
	ChildID  string // EntrySet
	// Item fields.
	AppliesTo string
	MatchType string // empty = prefix "/"
	Pattern   string
	Action    string
	Provider  string
	Groups    []string
	Origin    string
	Locked    bool
	Fallback  bool
	Enabled   bool
	Note      string
}

// Set is one rule set with its entries in priority order.
type Set struct {
	ID          string
	Slug        string
	Name        string
	Description string
	Kind        string
	Origin      string
	ServiceID   string
	Entries     []Entry
}

// Graph is every stored rule set by ID.
type Graph map[string]*Set

// CycleError is a reference that would close a loop (invariant 22).
type CycleError struct{ Path []string }

func (e *CycleError) Error() string {
	return fmt.Sprintf("%v: rule set references form a cycle: %s", ErrInvalid, strings.Join(e.Path, " → "))
}

func (e *CycleError) Unwrap() error { return ErrInvalid }

// CheckReference reports a CycleError when parent attaching child would close a loop.
func (g Graph) CheckReference(parentID, childID string) error {
	slug := func(id string) string {
		if s, ok := g[id]; ok {
			return s.Slug
		}
		return id
	}
	var walk func(id string, path []string) []string
	seen := map[string]bool{}
	walk = func(id string, path []string) []string {
		if id == parentID {
			return path
		}
		if seen[id] {
			return nil
		}
		seen[id] = true
		if s, ok := g[id]; ok {
			for _, e := range s.Entries {
				if e.Type == EntrySet {
					if p := walk(e.ChildID, append(slices.Clone(path), slug(e.ChildID))); p != nil {
						return p
					}
				}
			}
		}
		return nil
	}
	if p := walk(childID, []string{slug(parentID), slug(childID)}); p != nil {
		return &CycleError{Path: p}
	}
	return nil
}

// Flat is one item of a flattened list with the rule-set path it came from.
type Flat struct {
	Entry Entry
	Path  []string
	re    *regexp.Regexp
}

// Matches reports whether a normalized path meets the item's condition.
func (f Flat) Matches(path string) bool {
	switch f.Entry.MatchType {
	case MatchExact:
		return path == f.Entry.Pattern
	case MatchRegex:
		return f.re != nil && f.re.MatchString(path)
	case MatchPrefix:
		return strings.HasPrefix(path, f.Entry.Pattern) // the same string prefix Traefik's PathPrefix uses for routes
	}
	return true
}

func rootPathName(s *Set) string {
	if s.Kind == KindServiceRoot {
		return "service root"
	}
	return s.Slug
}

// Flatten is 06 §4.3: the root's items, the global set's, then the root's fallback; pre-order at every level, disabled
// items left out. A system service runs its fallback alone (06 §4.5), so no rule can lock the admin out.
func (g Graph) Flatten(rootID string, system bool) []Flat {
	root, ok := g[rootID]
	if !ok {
		return nil
	}
	var out []Flat
	var fallback *Flat
	active := map[string]bool{}
	var walk func(s *Set, path []string)
	walk = func(s *Set, path []string) {
		if active[s.ID] { // stored data cannot hold a cycle; a bad row must not hang forward auth
			return
		}
		active[s.ID] = true
		defer delete(active, s.ID)
		path = append(slices.Clone(path), rootPathName(s))
		for _, e := range s.Entries {
			switch {
			case e.Type == EntrySet:
				if c, ok := g[e.ChildID]; ok {
					walk(c, path)
				}
			case e.Fallback:
				if s.ID == rootID {
					f := Flat{Entry: e, Path: path}
					fallback = &f
				}
			case e.Enabled:
				f := Flat{Entry: e, Path: path}
				if e.MatchType == MatchRegex {
					f.re, _ = regexp.Compile(e.Pattern) // compiled at save; a bad stored pattern matches nothing
				}
				out = append(out, f)
			}
		}
	}
	if !system {
		walk(root, nil)
		if gs, ok := g[GlobalID]; ok && rootID != GlobalID {
			walk(gs, nil)
		}
	} else {
		for _, e := range root.Entries {
			if e.Fallback {
				f := Flat{Entry: e, Path: []string{rootPathName(root)}}
				fallback = &f
			}
		}
	}
	if fallback != nil {
		out = append(out, *fallback)
	}
	return out
}

// Evaluate returns the index of the first item that decides, or -1. in reports whether the address is in a set.
func Evaluate(list []Flat, path string, in func(setID string) bool) int {
	for i, f := range list {
		if in(f.Entry.AppliesTo) && f.Matches(path) {
			return i
		}
	}
	return -1
}

// FailClosed is 06 §2.1: rules that can deny or ask for sign-in must not open up while Hashi is away.
func FailClosed(list []Flat) bool {
	return slices.ContainsFunc(list, func(f Flat) bool {
		return f.Entry.Action == ActionDeny || f.Entry.Action == ActionAuth
	})
}

// Warnings are the save-time warnings of 06 §4.3 for one set's flattened items. allID is the system set everyone is in;
// geoMissing names sets used by the list whose countries cannot be looked up.
func Warnings(list []Flat, allID string, geoMissing func(setID string) bool) []string {
	var out []string
	covering := false
	for i, f := range list {
		if covering && !f.Entry.Fallback {
			out = append(out, fmt.Sprintf("Rule %d can never match: an earlier rule already covers everyone on every path.", i+1))
		}
		everyone := f.Entry.AppliesTo == allID && (f.Entry.MatchType == "" || (f.Entry.MatchType == MatchPrefix && f.Entry.Pattern == rootPathPrefix))
		if everyone && !f.Entry.Fallback {
			if i == 0 && f.Entry.Action == ActionNone {
				out = append(out, "The first rule lets everyone through, so every later rule on this service does nothing.")
			}
			covering = true
		}
		if geoMissing != nil && geoMissing(f.Entry.AppliesTo) {
			out = append(out, fmt.Sprintf("Rule %d uses a country or network, but no GeoIP database is available, so it matches nobody.", i+1))
		}
	}
	return out
}
