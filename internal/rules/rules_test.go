package rules

import (
	"errors"
	"testing"
)

func item(p int, set, match, pattern, action string) Entry {
	return Entry{Priority: p, Type: EntryItem, AppliesTo: set, MatchType: match, Pattern: pattern, Action: action, Enabled: true}
}

func graph() Graph {
	return Graph{
		"root": {ID: "root", Slug: "root-x", Kind: KindServiceRoot, Entries: []Entry{
			{Priority: 1, Type: EntrySet, ChildID: "a", Enabled: true},
			{Priority: 2, Type: EntrySet, ChildID: "b", Enabled: true},
			{Priority: 3, Type: EntryItem, AppliesTo: "all", Action: ActionDeny, Enabled: true, Fallback: true},
		}},
		"a":      {ID: "a", Slug: "a", Kind: KindUser, Entries: []Entry{item(1, "all", MatchPrefix, "/admin", ActionAuth)}},
		"b":      {ID: "b", Slug: "b", Kind: KindUser, Entries: []Entry{item(1, "all", MatchPrefix, "/admin", ActionNone)}},
		GlobalID: {ID: GlobalID, Slug: GlobalID, Kind: KindGlobal, Entries: []Entry{item(1, "devices", "", "", ActionNone)}},
	}
}

func everyone(sets ...string) func(string) bool {
	return func(id string) bool {
		for _, s := range sets {
			if s == id {
				return true
			}
		}
		return false
	}
}

// 06 §17 scenario 5 and §4.3: pre-order by priority, global after the root, the fallback last.
func TestFlattenOrderAndConflict(t *testing.T) {
	list := graph().Flatten("root", false)
	var got []string
	for _, f := range list {
		got = append(got, f.Path[len(f.Path)-1]+":"+f.Entry.Action)
	}
	want := []string{"a:pass_authentication", "b:none", "global:none", "service root:deny"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	i := Evaluate(list, "/admin/x", everyone("all"))
	if list[i].Entry.Action != ActionAuth || list[i].Path[1] != "a" {
		t.Fatalf("conflict decided by %+v", list[i])
	}
}

// A system service runs its fallback alone: global rules can't lock the admin out (06 §4.5).
func TestSystemServiceIgnoresRules(t *testing.T) {
	list := graph().Flatten("root", true)
	if len(list) != 1 || !list[0].Entry.Fallback {
		t.Fatalf("got %+v", list)
	}
}

func TestDisabledSkippedAndConditions(t *testing.T) {
	g := Graph{"r": {ID: "r", Slug: "r", Kind: KindServiceRoot, Entries: []Entry{
		{Priority: 1, Type: EntryItem, AppliesTo: "all", Action: ActionDeny, Enabled: false},
		item(2, "all", MatchExact, "/a", ActionDeny),
		item(3, "all", MatchRegex, "^/r[0-9]+$", ActionAuth),
		item(4, "devices", MatchPrefix, "/p", ActionNone),
		{Priority: 5, Type: EntryItem, AppliesTo: "all", Action: ActionNone, Enabled: true, Fallback: true},
	}}}
	list := g.Flatten("r", false)
	for _, c := range []struct {
		path, want string
		sets       []string
	}{
		{"/a", ActionDeny, []string{"all"}},
		{"/a/b", ActionNone, []string{"all"}},
		{"/r12", ActionAuth, []string{"all"}},
		{"/rx", ActionNone, []string{"all"}},
		{"/p", ActionNone, []string{"all", "devices"}},
	} {
		if i := Evaluate(list, c.path, everyone(c.sets...)); list[i].Entry.Action != c.want {
			t.Errorf("%s: %s, want %s", c.path, list[i].Entry.Action, c.want)
		}
	}
}

// 06 §2.1.
func TestFailClosedByContent(t *testing.T) {
	if !FailClosed(graph().Flatten("root", false)) {
		t.Error("deny fallback and sign-in rule must fail closed")
	}
	open := Graph{"r": {ID: "r", Entries: []Entry{item(1, "x", "", "", ActionNone), {Priority: 2, Type: EntryItem, AppliesTo: "all", Action: ActionNone, Enabled: true, Fallback: true}}}}
	if FailClosed(open.Flatten("r", false)) {
		t.Error("only none must fail open")
	}
}

func TestInvariant_22_SetGraphsAcyclic_RuleSets(t *testing.T) {
	g := Graph{"a": {ID: "a", Slug: "a", Entries: []Entry{{Type: EntrySet, ChildID: "b"}}}, "b": {ID: "b", Slug: "b"}}
	var ce *CycleError
	if err := g.CheckReference("b", "a"); !errors.As(err, &ce) || !errors.Is(err, ErrInvalid) {
		t.Fatalf("got %v", err)
	}
	if err := g.CheckReference("a", "a"); !errors.As(err, &ce) {
		t.Fatalf("self reference: %v", err)
	}
	if err := g.CheckReference("a", "b"); err != nil {
		t.Fatal(err)
	}
}

func TestWarnings(t *testing.T) {
	list := Graph{"r": {ID: "r", Entries: []Entry{
		item(1, "all", "", "", ActionNone),
		item(2, "geo", MatchPrefix, "/x", ActionDeny),
		{Priority: 3, Type: EntryItem, AppliesTo: "all", Action: ActionNone, Enabled: true, Fallback: true},
	}}}.Flatten("r", false)
	w := Warnings(list, "all", func(id string) bool { return id == "geo" })
	if len(w) != 3 {
		t.Fatalf("got %q", w)
	}
}
