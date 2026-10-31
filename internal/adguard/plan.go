package adguard

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"git.juzo.io/juzo/hashi/internal/store/db"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
)

// Subsystem is the plan's subsystem name (01 §5.3).
const Subsystem = "adguard"

// Desired is one rewrite Hashi wants on every active connection.
type Desired struct {
	Domain, Answer string
	Origin         string // what asks for it, e.g. "service <id>"
}

// itemValue is a plan item's before/after. Duplicate marks an extra copy of an owned rewrite: deleting it keeps the rewrite owned.
type itemValue struct {
	Domain    string `json:"domain"`
	Answer    string `json:"answer"`
	Origin    string `json:"origin,omitempty"`
	Duplicate bool   `json:"duplicate,omitempty"`
}

func value(v itemValue) json.RawMessage {
	b, _ := json.Marshal(v) // plain strings
	return b
}

type key struct{ domain, answer string }

// Diff compares what Hashi wants with what it owns and what the instance holds. Only owned rewrites and their
// duplicates are ever changed. A rewrite of someone else's on a wanted name is a conflict: shown, never applied (invariant: owned objects only).
func Diff(desired []Desired, owned []db.AdguardRewritesOwned, remote []Rewrite) []hsync.Item {
	return diff(desired, owned, remote, func(string) bool { return false })
}

// diff is Diff; gone reports an origin whose owner was explicitly deleted, whose rewrite needs no second approval (03 §9).
func diff(desired []Desired, owned []db.AdguardRewritesOwned, remote []Rewrite, gone func(origin string) bool) []hsync.Item {
	count := map[key]int{}
	answers := map[string][]string{}
	for _, r := range remote {
		k := key{r.Domain, r.Answer}
		if count[k] == 0 {
			answers[r.Domain] = append(answers[r.Domain], r.Answer)
		}
		count[k]++
	}
	own := map[key]db.AdguardRewritesOwned{}
	ownByDomain := map[string][]db.AdguardRewritesOwned{}
	for _, o := range owned {
		own[key{o.Domain, o.Answer}] = o
		ownByDomain[o.Domain] = append(ownByDomain[o.Domain], o)
	}
	want := map[key]bool{}
	for _, d := range desired {
		want[key{d.Domain, d.Answer}] = true
	}
	duplicates := func(o db.AdguardRewritesOwned) []hsync.Item {
		var items []hsync.Item
		for range max(count[key{o.Domain, o.Answer}]-1, 0) {
			items = append(items, hsync.Item{Action: hsync.ActionDelete, Object: o.Domain, Risk: hsync.High, Note: "duplicate of a rewrite Hashi owns",
				Before: value(itemValue{Domain: o.Domain, Answer: o.Answer, Duplicate: true})})
		}
		return items
	}
	var items []hsync.Item
	replaced := map[key]bool{}
	for _, d := range desired {
		k := key{d.Domain, d.Answer}
		after := value(itemValue{Domain: d.Domain, Answer: d.Answer, Origin: d.Origin})
		if o, ok := own[k]; ok {
			if count[k] == 0 {
				items = append(items, hsync.Item{Action: hsync.ActionCreate, Object: d.Domain, After: after, Risk: hsync.Low, Note: "was removed from AdGuard"})
			}
			items = append(items, duplicates(o)...)
			if d.Origin != o.Origin && count[k] > 0 {
				items = append(items, hsync.Item{Action: hsync.ActionLabel, Object: d.Domain, After: after, Risk: hsync.Low, Note: "same rewrite, new owner"})
			}
			continue
		}
		var stale *db.AdguardRewritesOwned
		for i, o := range ownByDomain[d.Domain] {
			if !want[key{o.Domain, o.Answer}] && !replaced[key{o.Domain, o.Answer}] {
				stale = &ownByDomain[d.Domain][i]
				break
			}
		}
		foreign := false
		for _, a := range answers[d.Domain] {
			if _, mine := own[key{d.Domain, a}]; !mine {
				foreign = true
			}
		}
		switch {
		case foreign:
			items = append(items, hsync.Item{Action: hsync.ActionConflict, Object: d.Domain, After: after, Risk: hsync.Low, Note: "a rewrite Hashi does not own exists for this name; left alone"})
		case stale != nil && count[key{stale.Domain, stale.Answer}] > 0:
			replaced[key{stale.Domain, stale.Answer}] = true
			items = append(items, hsync.Item{Action: hsync.ActionUpdate, Object: d.Domain, Risk: hsync.Low,
				Before: value(itemValue{Domain: stale.Domain, Answer: stale.Answer}), After: after})
			items = append(items, duplicates(*stale)...)
		default:
			items = append(items, hsync.Item{Action: hsync.ActionCreate, Object: d.Domain, After: after, Risk: hsync.Low})
		}
	}
	for _, o := range owned {
		k := key{o.Domain, o.Answer}
		if want[k] || replaced[k] {
			continue
		}
		it := hsync.Item{Action: hsync.ActionDelete, Object: o.Domain, Risk: hsync.High, Before: value(itemValue{Domain: o.Domain, Answer: o.Answer})}
		if count[k] == 0 {
			it.Risk, it.Note = hsync.Low, "already gone from AdGuard"
		} else if gone(o.Origin) {
			it.Risk, it.Note = hsync.Low, "its agent was deleted"
		}
		items = append(items, duplicates(o)...) // a delete removes one copy: the extras go first, then ownership
		items = append(items, it)
	}
	slices.SortStableFunc(items, func(a, b hsync.Item) int { return cmp.Compare(a.Object, b.Object) })
	return items
}

func rewriteHash(domain, answer string) string {
	sum := sha256.Sum256([]byte(domain + "\x00" + answer))
	return hex.EncodeToString(sum[:])
}

// sub plans and applies one connection's rewrites (01 §5).
type sub struct{ s *Service }

// Subsystems returns what the sync engine drives.
func (s *Service) Subsystems() []hsync.Subsystem { return []hsync.Subsystem{sub{s}} }

func (sub) Name() string                 { return Subsystem }
func (sub) LockKey(target string) string { return "adguard:" + target }

func (x sub) Plan(ctx context.Context, id string) (hsync.Draft, error) {
	row, err := x.s.st.R.GetAdGuardConnection(ctx, id)
	if err != nil {
		return hsync.Draft{}, fmt.Errorf("adguard connection %s: %w", id, err)
	}
	if row.Mode != ModeActive { // read-only and disabled connections are never written
		return hsync.Draft{}, nil
	}
	items, err := x.s.items(ctx, row)
	if err != nil {
		return hsync.Draft{}, err
	}
	return hsync.Draft{Title: "AdGuard rewrites on " + row.Name, Items: lowFirst(items)}, nil
}

// lowFirst plans low-risk changes alone while there are any, so a removal waiting for approval never holds back an address
// update: the removal is planned once nothing else is left to do.
func lowFirst(items []hsync.Item) []hsync.Item {
	var low []hsync.Item
	work := false
	for _, it := range items {
		if it.Risk == hsync.Low {
			low = append(low, it)
			work = work || it.Action != hsync.ActionConflict
		}
	}
	if work {
		return low
	}
	return items
}

func (s *Service) items(ctx context.Context, row db.AdguardConnection) ([]hsync.Item, error) {
	c, err := s.connect(ctx, row)
	if err != nil {
		return nil, err
	}
	remote, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	desired, err := s.wanted(ctx)
	if err != nil {
		return nil, err
	}
	owned, err := s.st.R.ListAdGuardOwned(ctx, row.ID)
	if err != nil {
		return nil, err
	}
	return diff(desired, owned, remote, s.gone(ctx)), nil
}

// Apply does exactly the approved items, one at a time, recording ownership as each lands, then reads the list back:
// AdGuard answers 200 even when a delete matched nothing.
func (x sub) Apply(ctx context.Context, id string, items []hsync.Item) (string, error) {
	s := x.s
	row, err := s.st.R.GetAdGuardConnection(ctx, id)
	if err != nil {
		return "", err
	}
	if row.Mode != ModeActive {
		return "", fmt.Errorf("%w: %s is %s", ErrReadOnly, row.Name, row.Mode)
	}
	c, err := s.connect(ctx, row)
	if err != nil {
		return "", err
	}
	owned, err := s.st.R.ListAdGuardOwned(ctx, id)
	if err != nil {
		return "", err
	}
	isOwned := func(v itemValue) bool {
		return slices.ContainsFunc(owned, func(o db.AdguardRewritesOwned) bool { return o.Domain == v.Domain && o.Answer == v.Answer })
	}
	now := s.now().UnixMilli()
	put := func(v itemValue) error {
		return s.st.Q.PutAdGuardOwned(ctx, db.PutAdGuardOwnedParams{ConnectionID: id, Domain: v.Domain, Answer: v.Answer, Origin: v.Origin, Hash: rewriteHash(v.Domain, v.Answer), CreatedAt: now})
	}
	forget := func(v itemValue) error {
		return s.st.Q.DeleteAdGuardOwned(ctx, db.DeleteAdGuardOwnedParams{ConnectionID: id, Domain: v.Domain, Answer: v.Answer})
	}
	expectGone, expectThere, done := map[key]int{}, map[key]bool{}, 0
	before, err := c.List(ctx)
	if err != nil {
		return "", err
	}
	counts := map[key]int{}
	for _, r := range before {
		counts[key{r.Domain, r.Answer}]++
	}
	for _, it := range items {
		var b, a itemValue
		if len(it.Before) > 0 {
			if err := json.Unmarshal(it.Before, &b); err != nil {
				return "", err
			}
		}
		if len(it.After) > 0 {
			if err := json.Unmarshal(it.After, &a); err != nil {
				return "", err
			}
		}
		switch it.Action {
		case hsync.ActionLabel: // ownership only: nothing changes on the instance
			if err := put(a); err != nil {
				return "", err
			}
		case hsync.ActionCreate:
			if err := c.Add(ctx, Rewrite{Domain: a.Domain, Answer: a.Answer}); err != nil {
				return "", err
			}
			if err := put(a); err != nil {
				return "", err
			}
			counts[key{a.Domain, a.Answer}]++
			expectThere[key{a.Domain, a.Answer}] = true
		case hsync.ActionUpdate:
			if !isOwned(b) {
				return "", fmt.Errorf("%w: %s", ErrNotOwned, b.Domain)
			}
			if err := c.Update(ctx, Rewrite{Domain: b.Domain, Answer: b.Answer}, Rewrite{Domain: a.Domain, Answer: a.Answer}); err != nil {
				return "", err
			}
			if err := put(a); err != nil {
				return "", err
			}
			if err := forget(b); err != nil {
				return "", err
			}
			counts[key{b.Domain, b.Answer}]--
			counts[key{a.Domain, a.Answer}]++
			expectThere[key{a.Domain, a.Answer}] = true
		case hsync.ActionDelete:
			if !isOwned(b) { // the last guard: nothing Hashi does not own is ever deleted
				return "", fmt.Errorf("%w: %s", ErrNotOwned, b.Domain)
			}
			if counts[key{b.Domain, b.Answer}] > 0 {
				if err := c.Delete(ctx, Rewrite{Domain: b.Domain, Answer: b.Answer}); err != nil {
					return "", err
				}
				counts[key{b.Domain, b.Answer}]--
			}
			if !b.Duplicate {
				if err := forget(b); err != nil {
					return "", err
				}
			}
			expectGone[key{b.Domain, b.Answer}] = counts[key{b.Domain, b.Answer}]
		default:
			continue
		}
		done++
	}
	after, err := c.List(ctx)
	if err != nil {
		return "", err
	}
	got := map[key]int{}
	for _, r := range after {
		got[key{r.Domain, r.Answer}]++
	}
	for k := range expectThere {
		if got[k] == 0 {
			return "", fmt.Errorf("%w: %s → %s is missing", ErrVerify, k.domain, k.answer)
		}
	}
	for k, n := range expectGone {
		if got[k] != n {
			return "", fmt.Errorf("%w: %s → %s is there %d times, expected %d", ErrVerify, k.domain, k.answer, got[k], n)
		}
	}
	return strconv.Itoa(done) + " rewrites changed", nil
}

// Errors of an apply.
var (
	ErrReadOnly = errors.New("connection does not allow writes")
	ErrNotOwned = errors.New("refusing to change a rewrite Hashi does not own")
	ErrVerify   = errors.New("AdGuard does not show the change")
)
