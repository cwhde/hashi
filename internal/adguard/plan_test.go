package adguard

import (
	"testing"

	"git.juzo.io/juzo/hashi/internal/store/db"
	hsync "git.juzo.io/juzo/hashi/internal/sync"
)

func own(domain, answer string) db.AdguardRewritesOwned {
	return db.AdguardRewritesOwned{Domain: domain, Answer: answer}
}

func actions(items []hsync.Item) []string {
	out := []string{}
	for _, i := range items {
		out = append(out, i.Action+" "+i.Object)
	}
	return out
}

func same(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestDiff(t *testing.T) {
	want := []Desired{{Domain: "a.example.com", Answer: "192.0.2.1"}}
	for name, c := range map[string]struct {
		desired []Desired
		owned   []db.AdguardRewritesOwned
		remote  []Rewrite
		expect  []string
	}{
		"new":                 {want, nil, nil, []string{"create a.example.com"}},
		"in place":            {want, []db.AdguardRewritesOwned{own("a.example.com", "192.0.2.1")}, []Rewrite{{Domain: "a.example.com", Answer: "192.0.2.1"}}, []string{}},
		"removed by hand":     {want, []db.AdguardRewritesOwned{own("a.example.com", "192.0.2.1")}, nil, []string{"create a.example.com"}},
		"owned duplicate":     {want, []db.AdguardRewritesOwned{own("a.example.com", "192.0.2.1")}, []Rewrite{{Domain: "a.example.com", Answer: "192.0.2.1"}, {Domain: "a.example.com", Answer: "192.0.2.1"}}, []string{"delete a.example.com"}},
		"address changed":     {want, []db.AdguardRewritesOwned{own("a.example.com", "192.0.2.9")}, []Rewrite{{Domain: "a.example.com", Answer: "192.0.2.9"}}, []string{"update a.example.com"}},
		"no longer wanted":    {nil, []db.AdguardRewritesOwned{own("a.example.com", "192.0.2.1")}, []Rewrite{{Domain: "a.example.com", Answer: "192.0.2.1"}}, []string{"delete a.example.com"}},
		"retired with a copy": {nil, []db.AdguardRewritesOwned{own("a.example.com", "192.0.2.1")}, []Rewrite{{Domain: "a.example.com", Answer: "192.0.2.1"}, {Domain: "a.example.com", Answer: "192.0.2.1"}}, []string{"delete a.example.com", "delete a.example.com"}},
		"changed with a copy": {want, []db.AdguardRewritesOwned{own("a.example.com", "192.0.2.9")}, []Rewrite{{Domain: "a.example.com", Answer: "192.0.2.9"}, {Domain: "a.example.com", Answer: "192.0.2.9"}}, []string{"update a.example.com", "delete a.example.com"}},
		"manual same value":   {want, nil, []Rewrite{{Domain: "a.example.com", Answer: "192.0.2.1"}}, []string{"conflict a.example.com"}},
		"manual other value":  {want, nil, []Rewrite{{Domain: "a.example.com", Answer: "192.0.2.7"}}, []string{"conflict a.example.com"}},
		"unrelated manual":    {nil, nil, []Rewrite{{Domain: "m.example.com", Answer: "192.0.2.7"}}, []string{}},
	} {
		t.Run(name, func(t *testing.T) {
			if got := actions(Diff(c.desired, c.owned, c.remote)); !same(got, c.expect) {
				t.Fatalf("got %v, want %v", got, c.expect)
			}
		})
	}
}

// Owned objects only: no item but a conflict ever names a rewrite Hashi does not own, whatever else is there.
func TestInvariant_AdGuardOwnedOnly(t *testing.T) {
	remote := []Rewrite{{Domain: "m.example.com", Answer: "192.0.2.7"}, {Domain: "m.example.com", Answer: "192.0.2.7"}, {Domain: "n.example.com", Answer: "192.0.2.8"}}
	items := Diff([]Desired{{Domain: "n.example.com", Answer: "192.0.2.1"}}, nil, remote)
	for _, i := range items {
		if i.Action != hsync.ActionConflict {
			t.Fatalf("touches an unowned rewrite: %+v", i)
		}
	}
	// A deletion of something owned is high-risk: it waits for approval.
	del := Diff(nil, []db.AdguardRewritesOwned{own("a.example.com", "192.0.2.1")}, []Rewrite{{Domain: "a.example.com", Answer: "192.0.2.1"}})
	if len(del) != 1 || del[0].Risk != hsync.High {
		t.Fatalf("%+v", del)
	}
}
