package rules

import (
	"errors"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

// Bug 57: a rule item that applies to a set makes the set used, named by its rule set.
func TestRulesAreSetConsumers(t *testing.T) {
	ctx := t.Context()
	st := storetest.New(t)
	set, err := ipsets.CreateSet(ctx, st.Q, ipsets.SetInput{Slug: "office", Name: "office"}, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Consumers(ctx, st.Q, set.ID); err != nil || len(got) != 0 {
		t.Fatalf("unused set: %v %v", got, err)
	}
	if err := st.Q.InsertRuleSet(ctx, db.InsertRuleSetParams{ID: "R1", Slug: "staff", Name: "Staff", Kind: "user", Origin: "user"}); err != nil {
		t.Fatal(err)
	}
	if err := insertEntry(ctx, st.Q, "R1", Entry{ID: "E1", Priority: 10, Type: EntryItem, AppliesTo: set.ID, Action: "none", Origin: "user", Enabled: true}, 1); err != nil {
		t.Fatal(err)
	}
	got, err := Consumers(ctx, st.Q, set.ID)
	if err != nil || len(got) != 1 || got[0].Type != "rule_set" || got[0].ID != "R1" || got[0].Name != "Rule set Staff" || got[0].Position != ipsets.PositionMatch {
		t.Fatalf("consumers %+v %v", got, err)
	}
}

// An attached rule set can't be deleted, and says where it is attached (06 §13). Moved here from the edge flow test,
// whose delete now stops at the passkey check first (bug 51).
func TestDeleteAttachedRuleSet(t *testing.T) {
	ctx := t.Context()
	st := storetest.New(t)
	now := time.Unix(1, 0)
	parent, err := Create(ctx, st.Q, "outer", "Outer", "", now)
	if err != nil {
		t.Fatal(err)
	}
	child, err := Create(ctx, st.Q, "inner", "Inner", "", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := PutEntries(ctx, st.Q, parent, []Entry{{Type: EntrySet, ChildID: child, Enabled: true}}, "", false, now); err != nil {
		t.Fatal(err)
	}
	if err := Delete(ctx, st.Q, child); !errors.Is(err, ErrInUse) || !strings.Contains(err.Error(), "Outer") {
		t.Fatalf("attached: %v", err)
	}
	if err := Delete(ctx, st.Q, parent); err != nil {
		t.Fatalf("unattached: %v", err)
	}
}
