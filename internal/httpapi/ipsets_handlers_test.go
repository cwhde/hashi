package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/auth"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/ipsets"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

func status(err error) int {
	var p *Problem
	if errors.As(err, &p) {
		return p.Status
	}
	if err != nil {
		return 500
	}
	return 200
}

func ipItem(t *testing.T, typ, v string) gen.IPItem {
	t.Helper()
	var it gen.IPItem
	var err error
	switch typ {
	case "ip":
		err = it.FromIPItemIP(gen.IPItemIP{Type: gen.Ip, Address: v})
	case "cidr":
		err = it.FromIPItemCIDR(gen.IPItemCIDR{Type: gen.Cidr, Prefix: v})
	case "dns":
		err = it.FromIPItemDNS(gen.IPItemDNS{Type: gen.Dns, Fqdn: v})
	case "set":
		err = it.FromIPItemSet(gen.IPItemSet{Type: gen.Set, SetId: v})
	}
	if err != nil {
		t.Fatal(err)
	}
	return it
}

// The IP set API (05a §8): CRUD, typed items with 422 for bad fields and 400 for a cycle, system sets locked (409),
// search, resolved prefixes, consumers, the address lookup with nesting paths, delete refused while referenced.
func TestIPSetsAPI(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	al := audit.New([]byte("k"))
	lookup := func(_ context.Context, n string) ([]netip.Addr, error) {
		if n == "phone.example.com" {
			return []netip.Addr{netip.MustParseAddr("198.51.100.7")}, nil
		}
		return nil, errors.New("no such host")
	}
	s := &Server{Store: st, Audit: al, Sessions: auth.NewManager(st, al, []byte("csrf")),
		IPSets: ipsets.NewService(st, al, slog.New(slog.NewTextHandler(io.Discard, nil)), lookup), Log: slog.Default()}

	created, err := s.CreateIPSet(ctx, gen.CreateIPSetRequestObject{Body: &gen.IPSetInput{Slug: "trusted-devices", Name: "Trusted devices"}})
	if err != nil {
		t.Fatal(err)
	}
	devices := created.(gen.CreateIPSet200JSONResponse)
	if !devices.Editable || devices.ResolutionStatus != gen.IPSetResolutionStatusEmpty {
		t.Fatalf("%+v", devices)
	}
	if _, err := s.CreateIPSet(ctx, gen.CreateIPSetRequestObject{Body: &gen.IPSetInput{Slug: "trusted-devices", Name: "x"}}); status(err) != http.StatusConflict {
		t.Fatalf("duplicate slug: %v", err)
	}
	add := func(setID, typ, v string) (gen.IPSetElement, error) {
		r, err := s.AddIPSetElement(ctx, gen.AddIPSetElementRequestObject{Id: setID, Body: &gen.IPSetElementInput{Item: ipItem(t, typ, v)}})
		if err != nil {
			return gen.IPSetElement{}, err
		}
		return gen.IPSetElement(r.(gen.AddIPSetElement200JSONResponse)), nil
	}
	if _, err := add(devices.Id, "dns", "phone.example.com"); err != nil {
		t.Fatal(err)
	}
	if e, err := add(devices.Id, "cidr", "203.0.113.77/24"); err != nil || e.Label != "203.0.113.0/24 (cidr)" {
		t.Fatalf("%+v %v", e, err)
	}
	for _, bad := range [][2]string{{"ip", "203.0.113.0/24"}, {"cidr", "203.0.113.1"}, {"dns", "not a name"}, {"ip", "fe80::1%eth0"}} {
		if _, err := add(devices.Id, bad[0], bad[1]); status(err) != http.StatusUnprocessableEntity {
			t.Errorf("%v: %v", bad, err)
		}
	}
	office, err := s.CreateIPSet(ctx, gen.CreateIPSetRequestObject{Body: &gen.IPSetInput{Slug: "office", Name: "Office"}})
	if err != nil {
		t.Fatal(err)
	}
	officeID := office.(gen.CreateIPSet200JSONResponse).Id
	if _, err := add(officeID, "set", devices.Id); err != nil {
		t.Fatal(err)
	}
	if _, err := add(devices.Id, "set", officeID); status(err) != http.StatusBadRequest || !strings.Contains(err.Error(), "trusted-devices → office → trusted-devices") {
		t.Fatalf("cycle: %v", err)
	}

	all, err := s.ListIPSets(ctx, gen.ListIPSetsRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	sets := all.(gen.ListIPSets200JSONResponse).Sets
	if len(sets) != 5 || sets[0].Kind != gen.IPSetKindSystem {
		t.Fatalf("list: %+v", sets)
	}
	var sysAll string
	for _, x := range sets {
		if x.SystemKey == ipsets.SystemAll {
			sysAll = x.Id
		}
	}
	if _, err := add(sysAll, "ip", "192.0.2.1"); status(err) != http.StatusConflict {
		t.Fatalf("system set edit: %v", err)
	}
	if _, err := s.DeleteIPSet(ctx, gen.DeleteIPSetRequestObject{Id: sysAll}); status(err) != http.StatusConflict {
		t.Fatalf("system set delete: %v", err)
	}

	res, err := s.GetIPSetResolved(ctx, gen.GetIPSetResolvedRequestObject{Id: officeID})
	if err != nil {
		t.Fatal(err)
	}
	r := res.(gen.GetIPSetResolved200JSONResponse)
	if strings.Join(r.Prefixes, ",") != "198.51.100.7/32,203.0.113.0/24" || r.AddressSet || r.Status != gen.IPSetResolvedStatusOk {
		t.Fatalf("resolved %+v", r)
	}
	els, err := s.ListIPSetElements(ctx, gen.ListIPSetElementsRequestObject{Id: devices.Id, Params: gen.ListIPSetElementsParams{Q: ptr("203.0.113.5")}})
	if err != nil || len(els.(gen.ListIPSetElements200JSONResponse).Elements) != 1 {
		t.Fatalf("search: %+v %v", els, err)
	}
	cons, err := s.GetIPSetConsumers(ctx, gen.GetIPSetConsumersRequestObject{Id: devices.Id})
	if err != nil || len(cons.(gen.GetIPSetConsumers200JSONResponse).Consumers) != 1 {
		t.Fatalf("consumers: %+v %v", cons, err)
	}

	look, err := s.LookupAddress(ctx, gen.LookupAddressRequestObject{Address: "::ffff:198.51.100.7"})
	if err != nil {
		t.Fatal(err)
	}
	l := look.(gen.LookupAddress200JSONResponse)
	var paths []string
	for _, m := range l.Sets {
		for _, x := range m.Matches {
			paths = append(paths, strings.Join(x.Path, " → ")+" : "+x.Label)
		}
	}
	// all (0.0.0.0/0), office (through trusted-devices) and trusted-devices; sorted by slug.
	want := "all : 0.0.0.0/0 (cidr)|office → trusted-devices : phone.example.com (dns)|trusted-devices : phone.example.com (dns)"
	if l.Address != "198.51.100.7" || strings.Join(paths, "|") != want {
		t.Fatalf("lookup %s: %v", l.Address, paths)
	}
	if _, err := s.LookupAddress(ctx, gen.LookupAddressRequestObject{Address: "nope"}); status(err) != http.StatusUnprocessableEntity {
		t.Fatalf("bad address: %v", err)
	}

	if _, err := s.DeleteIPSet(ctx, gen.DeleteIPSetRequestObject{Id: devices.Id}); status(err) != http.StatusConflict || !strings.Contains(err.Error(), "Office") {
		t.Fatalf("delete referenced: %v", err)
	}
	if _, err := s.DeleteIPSet(ctx, gen.DeleteIPSetRequestObject{Id: officeID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetIPSet(ctx, gen.GetIPSetRequestObject{Id: officeID}); status(err) != http.StatusNotFound {
		t.Fatalf("after delete: %v", err)
	}
	events, err := st.R.ListAuditEvents(ctx, dbAuditParams(20))
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	for _, e := range events {
		actions = append(actions, e.Action)
	}
	if got := strings.Join(actions, ","); !strings.Contains(got, "ip_set.deleted") || !strings.Contains(got, "ip_set.element.added") {
		t.Fatalf("audit: %s", got)
	}
}

func dbAuditParams(n int64) db.ListAuditEventsParams { return db.ListAuditEventsParams{Lim: n} }
