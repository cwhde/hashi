package ipsets

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"slices"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

func newService(t *testing.T, lookup LookupFunc) (*store.Store, *Service) {
	t.Helper()
	st := storetest.New(t)
	s := NewService(st, audit.New([]byte("salt")), slog.New(slog.NewTextHandler(io.Discard, nil)), lookup)
	s.now = func() time.Time { return now }
	return st, s
}

func TestSystemSetsSeeded(t *testing.T) {
	st, svc := newService(t, nil)
	ctx := context.Background()
	for key, want := range map[string]string{
		SystemAll:             "[0.0.0.0/0] [::/0]",
		SystemPrivateNetworks: "[10.0.0.0/8 100.64.0.0/10 127.0.0.0/8 169.254.0.0/16 172.16.0.0/12 192.168.0.0/16] [::1/128 fc00::/7 fe80::/10]",
		SystemHashiHosts:      "[] []",
	} {
		row, err := st.Q.GetIPSetBySystemKey(ctx, nullStr(key))
		if err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		res, err := svc.Compile(ctx, st.Q, row.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprint(res.V4) + " " + fmt.Sprint(res.V6); got != want {
			t.Errorf("%s: %s, want %s", key, got, want)
		}
		if _, err := UpdateSet(ctx, st.Q, row.ID, SetInput{Slug: "x", Name: "x"}, now); !errors.Is(err, ErrLocked) {
			t.Errorf("%s editable: %v", key, err)
		}
		if _, err := DeleteSet(ctx, st.Q, row.ID, nil); !errors.Is(err, ErrLocked) {
			t.Errorf("%s deletable: %v", key, err)
		}
		if _, err := (Editor{}).AddElement(ctx, st.Q, row.ID, ElementInput{Item: mustItem(t, "192.0.2.1")}, "t", now); !errors.Is(err, ErrLocked) {
			t.Errorf("%s elements editable: %v", key, err)
		}
	}
}

func TestSetCRUDAndElements(t *testing.T) {
	st, _ := newService(t, nil)
	ctx, q := context.Background(), st.Q
	s, err := CreateSet(ctx, q, SetInput{Slug: " Trusted-Devices ", Name: "Trusted devices"}, now)
	if err != nil || s.Slug != "trusted-devices" || s.Kind != KindUser {
		t.Fatalf("%+v %v", s, err)
	}
	if _, err := CreateSet(ctx, q, SetInput{Slug: "trusted-devices", Name: "again"}, now); !errors.Is(err, ErrTaken) {
		t.Fatalf("duplicate slug: %v", err)
	}
	if _, err := CreateSet(ctx, q, SetInput{Slug: "all", Name: "x"}, now); !errors.Is(err, ErrTaken) {
		t.Fatalf("system slug: %v", err)
	}
	for _, bad := range []SetInput{{Slug: "", Name: "x"}, {Slug: "a_b", Name: "x"}, {Slug: "ok", Name: " "}} {
		if _, err := CreateSet(ctx, q, bad, now); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: %v", bad, err)
		}
	}
	ed := Editor{}
	e1, err := ed.AddElement(ctx, q, s.ID, ElementInput{Item: mustItem(t, "192.0.2.10/24"), Note: " phone "}, "session:x", now)
	if err != nil || e1.Note != "phone" || e1.Position != 1 {
		t.Fatalf("%+v %v", e1, err)
	}
	e2, err := ed.AddElement(ctx, q, s.ID, ElementInput{Item: DNS{FQDN: "Home.Example.com."}}, "session:x", now)
	if err != nil || e2.Position != 2 || e2.Item.(DNS).FQDN != "home.example.com" {
		t.Fatalf("%+v %v", e2, err)
	}
	if _, err := ed.AddElement(ctx, q, s.ID, ElementInput{Item: Host{HostID: "nope", Scope: ScopePublic}}, "t", now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("host without hosts: %v", err)
	}
	past := now.Add(-time.Minute)
	if _, err := ed.AddElement(ctx, q, s.ID, ElementInput{Item: mustItem(t, "192.0.2.1"), ExpiresAt: &past}, "t", now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("past expiry: %v", err)
	}
	if _, err := ed.UpdateElement(ctx, q, s.ID, e1.ID, ElementInput{Item: mustItem(t, "198.51.100.0/24")}, now); err != nil {
		t.Fatal(err)
	}
	got, err := GetSet(ctx, q, s.ID)
	if err != nil || len(got.Elements) != 2 || got.Elements[0].Item.(CIDR).Prefix.String() != "198.51.100.0/24" || got.Elements[0].Note != "" {
		t.Fatalf("%+v %v", got, err)
	}
	if _, err := ed.DeleteElement(ctx, q, s.ID, e2.ID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := ed.DeleteElement(ctx, q, s.ID, e2.ID, now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	if _, err := DeleteSet(ctx, q, s.ID, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := GetSet(ctx, q, s.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestReferencesCyclesAndDelete(t *testing.T) {
	st, _ := newService(t, nil)
	ctx, q, ed := context.Background(), st.Q, Editor{}
	a := mustSet(t, q, "a")
	b := mustSet(t, q, "b")
	if _, err := ed.AddElement(ctx, q, a.ID, ElementInput{Item: SetRef{SetID: b.ID}}, "t", now); err != nil {
		t.Fatal(err)
	}
	_, err := ed.AddElement(ctx, q, b.ID, ElementInput{Item: SetRef{SetID: a.ID}}, "t", now)
	var ce *CycleError
	if !errors.As(err, &ce) || strings.Join(ce.Path, " → ") != "b → a → b" {
		t.Fatalf("cycle: %v", err)
	}
	if _, err := ed.AddElement(ctx, q, b.ID, ElementInput{Item: SetRef{SetID: "missing"}}, "t", now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing ref: %v", err)
	}
	if _, err := DeleteSet(ctx, q, b.ID, nil); !errors.Is(err, ErrInUse) || !strings.Contains(err.Error(), "a") {
		t.Fatalf("delete referenced: %v", err)
	}
	cons, err := Consumers(ctx, q, b.ID, nil)
	if err != nil || len(cons) != 1 || cons[0].ID != a.ID || cons[0].Type != "ip_set" {
		t.Fatalf("consumers %+v %v", cons, err)
	}
	// The database refuses it too (ON DELETE RESTRICT).
	if err := q.DeleteIPSet(ctx, b.ID); err == nil {
		t.Fatal("RESTRICT foreign key did not hold")
	}
}

func TestTargetSetRefusesMatchOnlyItems(t *testing.T) {
	st, _ := newService(t, nil)
	ctx, q := context.Background(), st.Q
	inner := mustSet(t, q, "inner")
	outer := mustSet(t, q, "outer")
	free := mustSet(t, q, "free")
	targets := func(_ context.Context, _ *db.Queries, id string) ([]Consumer, error) {
		if id == outer.ID {
			return []Consumer{{Type: "service", ID: "svc", Name: "service searxng", Position: PositionTarget}}, nil
		}
		return nil, nil
	}
	ed := Editor{Sources: []ConsumerSource{targets}}
	if _, err := ed.AddElement(ctx, q, outer.ID, ElementInput{Item: SetRef{SetID: inner.ID}}, "t", now); err != nil {
		t.Fatal(err)
	}
	if _, err := ed.AddElement(ctx, q, inner.ID, ElementInput{Item: mustItem(t, "192.0.2.1")}, "t", now); err != nil {
		t.Fatal(err)
	}
	_, err := ed.AddElement(ctx, q, inner.ID, ElementInput{Item: mustItem(t, "192.0.2.0/24")}, "t", now)
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), "service searxng") {
		t.Fatalf("cidr inside a target's nested set: %v", err)
	}
	all, err := q.GetIPSetBySystemKey(ctx, nullStr(SystemAll))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ed.AddElement(ctx, q, outer.ID, ElementInput{Item: SetRef{SetID: all.ID}}, "t", now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("the all set as a target: %v", err)
	}
	if _, err := ed.AddElement(ctx, q, free.ID, ElementInput{Item: mustItem(t, "192.0.2.0/24")}, "t", now); err != nil {
		t.Fatalf("a set without target consumers takes a cidr: %v", err)
	}
}

func TestRecompileNotifiesOnlyChangedSets(t *testing.T) {
	answers := map[string][]netip.Addr{"home.example.com": {netip.MustParseAddr("198.51.100.1")}}
	st, svc := newService(t, func(_ context.Context, n string) ([]netip.Addr, error) { return answers[n], nil })
	ctx, q, ed := context.Background(), st.Q, Editor{}
	home := mustSet(t, q, "home")
	other := mustSet(t, q, "other")
	parent := mustSet(t, q, "parent")
	mustAdd(t, ed, q, home.ID, DNS{FQDN: "home.example.com"})
	mustAdd(t, ed, q, other.ID, mustItem(t, "203.0.113.1"))
	mustAdd(t, ed, q, parent.ID, SetRef{SetID: home.ID})

	var got [][]string
	svc.Subscribe(func(_ context.Context, changed []string) { got = append(got, changed) })
	first, err := svc.Recompile(ctx)
	if err != nil || len(first) != 6 { // three system sets + three user sets, all compiled for the first time
		t.Fatalf("first: %v %v", first, err)
	}
	if again, _ := svc.Recompile(ctx); len(again) != 0 {
		t.Fatalf("nothing changed, got %v", again)
	}
	// The home WAN address changes: the dns item's set and its parent change, nothing else.
	answers["home.example.com"] = []netip.Addr{netip.MustParseAddr("198.51.100.2")}
	svc.now = func() time.Time { return now.Add(DNSRefresh) }
	sum, err := svc.Refresh(ctx)
	if err != nil {
		t.Fatal(err)
	}
	last := got[len(got)-1]
	slices.Sort(last)
	want := []string{home.ID, parent.ID}
	slices.Sort(want)
	if !slices.Equal(last, want) || !strings.Contains(sum, "1 dns name(s) refreshed") {
		t.Fatalf("changed %v want %v (%s)", last, want, sum)
	}
	row, err := q.GetIPSet(ctx, parent.ID)
	if err != nil || row.ResolutionStatus != StatusOK || row.CompiledHash == "" {
		t.Fatalf("%+v %v", row, err)
	}
}

func TestRefreshDropsExpiredWithAudit(t *testing.T) {
	st, svc := newService(t, nil)
	ctx, q, ed := context.Background(), st.Q, Editor{}
	s := mustSet(t, q, "guests")
	soon := now.Add(time.Hour)
	if _, err := ed.AddElement(ctx, q, s.ID, ElementInput{Item: mustItem(t, "192.0.2.1"), ExpiresAt: &soon, Note: "guest"}, "t", now); err != nil {
		t.Fatal(err)
	}
	mustAdd(t, ed, q, s.ID, mustItem(t, "192.0.2.2"))
	svc.now = func() time.Time { return soon }
	sum, err := svc.Refresh(ctx)
	if err != nil || !strings.Contains(sum, "1 element(s) expired") {
		t.Fatalf("%s %v", sum, err)
	}
	got, _ := GetSet(ctx, q, s.ID)
	if len(got.Elements) != 1 {
		t.Fatalf("elements %+v", got.Elements)
	}
	events, err := q.ListAuditEvents(ctx, db.ListAuditEventsParams{Lim: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 || events[0].Action != "ip_set.element.expired" {
		t.Fatalf("events %+v", events)
	}
}

func mustSet(t *testing.T, q *db.Queries, slug string) Set {
	t.Helper()
	s, err := CreateSet(context.Background(), q, SetInput{Slug: slug, Name: slug}, now)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func mustAdd(t *testing.T, ed Editor, q *db.Queries, setID string, it Item) {
	t.Helper()
	if _, err := ed.AddElement(context.Background(), q, setID, ElementInput{Item: it}, "t", now); err != nil {
		t.Fatal(err)
	}
}
