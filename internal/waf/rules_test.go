package waf

import "testing"

// Bug 18: exclusions are picked by name, and the names come from the rule files Coraza runs, not a list kept by hand.
func TestRulesHaveTheirOwnNames(t *testing.T) {
	rules := Rules()
	if len(rules) < 100 {
		t.Fatalf("only %d rules read from the CRS files", len(rules))
	}
	byID := map[int]Rule{}
	for _, r := range rules {
		if r.Name == "" || r.Group == "" || r.Paranoia < 1 || r.Paranoia > 4 {
			t.Errorf("incomplete rule %+v", r)
		}
		if _, dup := byID[r.ID]; dup {
			t.Errorf("rule %d listed twice", r.ID)
		}
		byID[r.ID] = r
	}
	want := map[int]Rule{
		920440: {ID: 920440, Name: "URL file extension is restricted by policy", Group: "Protocol enforcement", Paranoia: 1},
	}
	for id, w := range want {
		if byID[id] != w {
			t.Errorf("rule %d = %+v, want %+v", id, byID[id], w)
		}
	}
	for _, r := range rules {
		if r.ID >= 949000 {
			t.Errorf("rule %d is not an attack rule a request can trip", r.ID)
		}
	}
}
