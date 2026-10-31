package ipsets

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"
	"time"

	"go4.org/netipx"
)

// Resolution statuses (05a §4.1).
const (
	StatusOK      = "ok"
	StatusPartial = "partial"
	StatusEmpty   = "empty"
)

// Set is one IP set with its elements in position order.
type Set struct {
	ID          string
	Slug        string
	Name        string
	Description string
	Kind        string
	Origin      string
	SystemKey   string
	Elements    []Element
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Editable reports whether the user may change the set's elements and metadata (05a §3).
func (s Set) Editable() bool { return s.Kind == KindUser }

// Graph is every stored set by ID: the input of the cycle check and the compiler.
type Graph map[string]*Set

// Resolver answers the items whose addresses live outside the set graph. Errors and empty answers both make the
// set partial; they never fail the compile.
type Resolver interface {
	Host(hostID, scope string) ([]netip.Addr, error)
	Pulse(agentID, scope, selector string) ([]netip.Addr, error)
	DNS(fqdn string) ([]netip.Addr, error)
}

// Stale is a resolver's answer for addresses that still count but may be out of date (05a §2).
type Stale struct{ What string }

func (s Stale) Error() string { return s.What }

// Problem names one item that could not resolve, with the nesting path from the compiled set to it.
type Problem struct {
	SetID     string   `json:"setId"`
	ElementID string   `json:"elementId"`
	Path      []string `json:"path"`
	Message   string   `json:"message"`
}

// Result is a compiled set (05a §4.1).
type Result struct {
	V4, V6 []netip.Prefix
	// Leaves are the prefixes of the leaves before merging (deduplicated, sorted). Longest-prefix decisions
	// (firewall folding, 05 §4.2) need them: merging absorbs a /32 inside a /16 of the same set.
	Leaves []netip.Prefix
	// Addresses are the single addresses every target-capable leaf resolved to, in item order (first = primary,
	// 05a §5.2), without duplicates. Only meaningful when AddressSet is true.
	Addresses []netip.Addr
	// AddressSet: every leaf, through nesting, is ip, pulse, host or dns (05a §5.2).
	AddressSet bool
	Status     string
	Problems   []Problem
}

// Hash identifies the compiled content: consumers re-plan only when it changes.
func (r Result) Hash() string {
	var b strings.Builder
	for _, p := range r.V4 {
		b.WriteString(p.String() + "\n")
	}
	for _, p := range r.V6 {
		b.WriteString(p.String() + "\n")
	}
	b.WriteString("--\n")
	for _, a := range r.Addresses {
		b.WriteString(a.String() + "\n")
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// Contains reports whether the compiled prefixes contain a.
func (r Result) Contains(a netip.Addr) bool {
	a = a.Unmap()
	list := r.V4
	if a.Is6() {
		list = r.V6
	}
	for _, p := range list {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// CycleError is a reference that would close a loop; Path is the slugs from the set back to itself.
type CycleError struct{ Path []string }

func (e *CycleError) Error() string {
	return fmt.Sprintf("%v: set references form a cycle: %s", ErrInvalid, strings.Join(e.Path, " → "))
}

func (e *CycleError) Unwrap() error { return ErrInvalid }

// CheckReference reports a CycleError when parent including child would close a loop (05a §2, invariant 22).
func (g Graph) CheckReference(parentID, childID string) error {
	slug := func(id string) string {
		if s, ok := g[id]; ok {
			return s.Slug
		}
		return id
	}
	if parentID == childID {
		return &CycleError{Path: []string{slug(parentID), slug(parentID)}}
	}
	// DFS from child: a path back to parent closes the loop parent → child → … → parent.
	seen := map[string]bool{}
	var walk func(id string, path []string) []string
	walk = func(id string, path []string) []string {
		if id == parentID {
			return path
		}
		if seen[id] {
			return nil
		}
		seen[id] = true
		s, ok := g[id]
		if !ok {
			return nil
		}
		for _, e := range s.Elements {
			if ref, ok := e.Item.(SetRef); ok {
				if p := walk(ref.SetID, append(path, slug(ref.SetID))); p != nil {
					return p
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

// Compile resolves one set at now. Expired elements are left out; an unknown or cyclic reference in stored data
// becomes a problem instead of a failure, so one bad set never stops its consumers (invariant 21 keeps targets).
func (g Graph) Compile(id string, r Resolver, now time.Time) Result {
	c := compiler{g: g, r: r, now: now, memo: map[string]*partial{}, active: map[string]bool{}}
	p := c.set(id)
	var b netipx.IPSetBuilder
	for _, pre := range p.prefixes {
		b.AddPrefix(pre)
	}
	ips, _ := b.IPSet() // the builder only errors on invalid input, which normalization excludes
	res := Result{AddressSet: p.addressSet, Problems: p.problems, Addresses: dedup(p.addresses), Leaves: leaves(p.prefixes)}
	for _, pre := range ips.Prefixes() {
		if pre.Addr().Is4() {
			res.V4 = append(res.V4, pre)
		} else {
			res.V6 = append(res.V6, pre)
		}
	}
	switch {
	case len(res.V4)+len(res.V6) == 0:
		res.Status = StatusEmpty
	case len(res.Problems) > 0:
		res.Status = StatusPartial
	default:
		res.Status = StatusOK
	}
	return res
}

type partial struct {
	prefixes   []netip.Prefix
	addresses  []netip.Addr
	addressSet bool
	problems   []Problem
}

type compiler struct {
	g      Graph
	r      Resolver
	now    time.Time
	memo   map[string]*partial
	active map[string]bool
}

// set compiles one set. Problem paths are relative to the set (they start with its slug); a parent prepends its own
// slug, so a memoized child reached through two parents reports the right path for each.
func (c *compiler) set(id string) *partial {
	s, ok := c.g[id]
	if !ok {
		return &partial{problems: []Problem{{SetID: id, Path: []string{id}, Message: "referenced set does not exist"}}}
	}
	if m, ok := c.memo[id]; ok {
		return m
	}
	if c.active[id] {
		return &partial{problems: []Problem{{SetID: id, Path: []string{s.Slug}, Message: "set references form a cycle"}}}
	}
	path := []string{s.Slug}
	c.active[id] = true
	defer delete(c.active, id)

	out := &partial{addressSet: s.Kind == KindUser || s.Kind == KindSystem}
	fail := func(e Element, msg string) {
		out.problems = append(out.problems, Problem{SetID: id, ElementID: e.ID, Path: path, Message: msg})
	}
	for _, e := range s.Elements {
		if e.Expired(c.now) {
			continue
		}
		switch v := e.Item.(type) {
		case IP:
			out.prefixes = append(out.prefixes, netip.PrefixFrom(v.Addr, v.Addr.BitLen()))
			out.addresses = append(out.addresses, v.Addr)
		case CIDR:
			out.prefixes = append(out.prefixes, v.Prefix)
			out.addressSet = false
		case Host:
			c.leaf(out, e, fail, func() ([]netip.Addr, error) { return c.r.Host(v.HostID, v.Scope) }, "host")
		case Pulse:
			c.leaf(out, e, fail, func() ([]netip.Addr, error) { return c.r.Pulse(v.AgentID, v.Scope, v.Selector) }, "Pulse agent")
		case DNS:
			c.leaf(out, e, fail, func() ([]netip.Addr, error) { return c.r.DNS(v.FQDN) }, v.FQDN)
		case SetRef:
			child := c.set(v.SetID)
			out.prefixes = append(out.prefixes, child.prefixes...)
			out.addresses = append(out.addresses, child.addresses...)
			for _, p := range child.problems {
				p.Path = append([]string{s.Slug}, p.Path...)
				out.problems = append(out.problems, p)
			}
			out.addressSet = out.addressSet && child.addressSet
		}
	}
	c.memo[id] = out
	return out
}

func (c *compiler) leaf(out *partial, e Element, fail func(Element, string), resolve func() ([]netip.Addr, error), what string) {
	if c.r == nil {
		fail(e, what+": no resolver")
		return
	}
	addrs, err := resolve()
	var stale Stale
	switch {
	case errors.As(err, &stale): // the addresses still count; the set says they may be old
		fail(e, stale.What)
	case err != nil:
		fail(e, fmt.Sprintf("%s: %v", what, err))
		return
	case len(addrs) == 0:
		fail(e, what+": resolved to no address")
		return
	}
	for _, a := range addrs {
		a = a.Unmap()
		out.prefixes = append(out.prefixes, netip.PrefixFrom(a, a.BitLen()))
		out.addresses = append(out.addresses, a)
	}
}

func leaves(in []netip.Prefix) []netip.Prefix {
	out := slices.Clone(in)
	slices.SortFunc(out, func(a, b netip.Prefix) int {
		if c := a.Addr().Compare(b.Addr()); c != 0 {
			return c
		}
		return a.Bits() - b.Bits()
	})
	return slices.Compact(out)
}

func dedup(in []netip.Addr) []netip.Addr {
	seen := make(map[netip.Addr]bool, len(in))
	out := make([]netip.Addr, 0, len(in))
	for _, a := range in {
		if !seen[a] {
			seen[a] = true
			out = append(out, a)
		}
	}
	return out
}
