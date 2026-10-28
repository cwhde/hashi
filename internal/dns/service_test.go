package dns_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/dns/hetzner"
	"git.juzo.io/juzo/hashi/internal/dns/hetzner/hetznertest"
	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	"git.juzo.io/juzo/hashi/internal/vault"
)

type fixture struct {
	svc *dns.Service
	st  *store.Store
	srv *hetznertest.Server
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	st := storetest.New(t)
	v, err := vault.New(bytes.Repeat([]byte{7}, vault.KeySize), 1)
	if err != nil {
		t.Fatal(err)
	}
	srv := hetznertest.NewServer(t)
	open := func(typ string, token secrets.Value) (dns.Reader, error) {
		return hetzner.New(token, hetzner.WithBaseURL(srv.BaseURL())), nil
	}
	return fixture{svc: dns.NewService(st, v, audit.New([]byte("k")), open), st: st, srv: srv}
}

func connect(t *testing.T, f fixture) dns.ConnectResult {
	t.Helper()
	res, err := f.svc.Connect(context.Background(), "test", "", dns.ConnectInput{Type: "hetzner", Token: secrets.New(hetznertest.Token),
		DefaultTTL: 3600, RootDomain: "example.com"})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestConnectPicksRootZoneAndStoresSnapshot(t *testing.T) {
	f := newFixture(t)
	res := connect(t, f)
	if len(res.Zones) != 2 || res.Zone == nil || res.Zone.Name != "example.com" || res.Zone.RRSetCount != 58 {
		t.Fatalf("result = %+v zone=%+v", res, res.Zone)
	}
	if res.Provider.Health != "healthy" || res.Provider.DefaultTTL != 3600 {
		t.Errorf("provider = %+v", res.Provider)
	}
	rrs, err := f.svc.RRSets(context.Background(), res.Zone.ID, "", "")
	if err != nil || len(rrs) != 58 {
		t.Fatalf("rrsets = %d, %v", len(rrs), err)
	}
	// The token is only in the vault, sealed.
	rows, err := f.st.R.ListSecretMeta(context.Background())
	if err != nil || len(rows) != 1 || rows[0].Purpose != "dns.provider.token" {
		t.Fatalf("secrets = %+v, %v", rows, err)
	}
	rec, _ := f.st.R.GetSecret(context.Background(), rows[0].ID)
	if bytes.Contains(rec.Ciphertext, []byte(hetznertest.Token)) {
		t.Error("token stored in plain text")
	}
}

func TestInvariant_3_FailedValidationStoresNothing(t *testing.T) {
	f := newFixture(t)
	_, err := f.svc.Connect(context.Background(), "test", "", dns.ConnectInput{Type: "hetzner", Token: secrets.New("wrong"),
		DefaultTTL: 3600, RootDomain: "example.com"})
	if !errors.Is(err, dns.ErrValidation) || !errors.Is(err, dns.ErrUnauthorized) {
		t.Fatalf("err = %v", err)
	}
	if _, _, err := f.svc.Current(context.Background()); !errors.Is(err, dns.ErrNotConfigured) {
		t.Errorf("provider stored after failed validation: %v", err)
	}
	if rows, _ := f.st.R.ListSecretMeta(context.Background()); len(rows) != 0 {
		t.Errorf("token stored after failed validation: %+v", rows)
	}
	for _, r := range f.srv.Requests() {
		if !strings.HasPrefix(r, "GET ") {
			t.Errorf("non-GET request during validation: %s", r)
		}
	}
}

func TestConnectWithoutMatchingZoneAsksForChoice(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	res, err := f.svc.Connect(ctx, "test", "", dns.ConnectInput{Type: "hetzner", Token: secrets.New(hetznertest.Token),
		DefaultTTL: 300, RootDomain: "example.net"})
	if err != nil || res.Zone != nil || len(res.Zones) != 2 {
		t.Fatalf("res = %+v, %v", res, err)
	}
	if _, _, err := f.svc.Current(ctx); !errors.Is(err, dns.ErrNotConfigured) {
		t.Errorf("stored without a zone: %v", err)
	}
	res, err = f.svc.Connect(ctx, "test", "", dns.ConnectInput{Type: "hetzner", Token: secrets.New(hetznertest.Token), DefaultTTL: 300,
		RootDomain: "example.net", ZoneID: res.Zones[0].ID})
	if err != nil || res.Zone == nil || res.Zone.Name != "example.com" {
		t.Fatalf("pick: %+v, %v", res, err)
	}
	if _, err := f.svc.Connect(ctx, "test", "", dns.ConnectInput{Type: "hetzner", DefaultTTL: 300, ZoneID: "123"}); !errors.Is(err, dns.ErrInvalid) { // stored token reused
		t.Errorf("unknown zone id: %v", err)
	}
}

func TestDecisionsPersistAndOwnership(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	z := connect(t, f).Zone
	err := f.svc.SaveDecisions(ctx, "test", "", z.ID, []dns.DecisionInput{
		{Name: "svc1", Type: "A", Decision: dns.Import},
		{Name: "svc2", Type: "CNAME", Decision: dns.Prune},
		{Name: "@", Type: "MX", Decision: dns.Leave},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, rows, err := f.svc.ImportTable(ctx, z.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	undecided := 0
	for _, r := range rows {
		if r.Decision == nil {
			undecided++
			continue
		}
		got[r.Name+"/"+r.Type] = string(*r.Decision) + ":" + r.Origin
	}
	want := map[string]string{"svc1/A": "import:imported", "svc2/CNAME": "prune:", "@/MX": "leave:"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if undecided != 55 {
		t.Errorf("undecided = %d, want 55 (re-import shows only new RRSets)", undecided)
	}
	imported, _ := f.svc.RRSets(ctx, z.ID, dns.OriginImported, "")
	unmanaged, _ := f.svc.RRSets(ctx, z.ID, dns.Unmanaged, "")
	if len(imported) != 1 || len(unmanaged) != 57 {
		t.Errorf("imported=%d unmanaged=%d", len(imported), len(unmanaged))
	}
	// Taking the import back removes the imported ownership.
	if err := f.svc.SaveDecisions(ctx, "test", "", z.ID, []dns.DecisionInput{{Name: "svc1", Type: "A", Decision: dns.Leave}}); err != nil {
		t.Fatal(err)
	}
	if imported, _ := f.svc.RRSets(ctx, z.ID, dns.OriginImported, ""); len(imported) != 0 {
		t.Errorf("ownership kept after leave: %+v", imported)
	}
	// A refresh keeps the decisions.
	if _, err := f.svc.Refresh(ctx, z.ID); err != nil {
		t.Fatal(err)
	}
	_, rows, _ = f.svc.ImportTable(ctx, z.ID)
	if i := slices.IndexFunc(rows, func(o dns.ImportRow) bool { return o.Name == "svc2" && o.Type == "CNAME" }); i < 0 || rows[i].Decision == nil || *rows[i].Decision != dns.Prune {
		t.Error("decision lost on refresh")
	}
}

func TestInvariant_2_ImportRejectsNSSOAPrune(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	z := connect(t, f).Zone
	for _, typ := range []string{"NS", "SOA"} {
		for _, d := range []dns.Decision{dns.Prune, dns.Import} {
			err := f.svc.SaveDecisions(ctx, "test", "", z.ID, []dns.DecisionInput{{Name: "@", Type: typ, Decision: d}})
			if !errors.Is(err, dns.ErrInvalid) {
				t.Errorf("%s %s: err = %v", typ, d, err)
			}
		}
	}
	_, rows, _ := f.svc.ImportTable(ctx, z.ID)
	for _, r := range rows {
		if dns.Protected(r.Type) {
			if r.Decision != nil || r.Origin != "" || !slices.Equal(r.Allowed, []dns.Decision{dns.Leave}) {
				t.Errorf("%s %s: decision=%v origin=%q allowed=%v", r.Name, r.Type, r.Decision, r.Origin, r.Allowed)
			}
		}
	}
	// A batch with one invalid row changes nothing.
	err := f.svc.SaveDecisions(ctx, "test", "", z.ID, []dns.DecisionInput{{Name: "svc1", Type: "A", Decision: dns.Import}, {Name: "@", Type: "NS", Decision: dns.Prune}})
	if err == nil {
		t.Fatal("batch accepted")
	}
	if imported, _ := f.svc.RRSets(ctx, z.ID, dns.OriginImported, ""); len(imported) != 0 {
		t.Error("partial batch applied")
	}
}

func TestRefreshFailureKeepsSnapshot(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	z := connect(t, f).Zone
	f.srv.Override = func(w http.ResponseWriter, r *http.Request) bool {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"unauthorized","message":"the token you have provided is invalid"}}`))
		return true
	}
	if _, err := f.svc.Refresh(ctx, z.ID); !errors.Is(err, dns.ErrUnauthorized) {
		t.Fatalf("err = %v", err)
	}
	zi, _ := f.svc.Zone(ctx, z.ID)
	if zi.RRSetCount != 58 || zi.LastRefreshError == "" || zi.Provider.Health != "failed" {
		t.Errorf("zone after failure = %+v", zi)
	}
	if rrs, _ := f.svc.RRSets(ctx, z.ID, "", ""); len(rrs) != 58 {
		t.Errorf("snapshot lost: %d", len(rrs))
	}
	f.srv.Override = nil
	sum, err := f.svc.RefreshAll(ctx)
	if err != nil || !strings.Contains(sum, "58 RRSets (+0 −0 ~0)") {
		t.Errorf("RefreshAll = %q, %v", sum, err)
	}
	if zi, _ := f.svc.Zone(ctx, z.ID); zi.LastRefreshError != "" || zi.Provider.Health != "healthy" {
		t.Errorf("error not cleared: %+v", zi)
	}
}

func TestRRSetsFilter(t *testing.T) {
	f := newFixture(t)
	z := connect(t, f).Zone
	rrs, err := f.svc.RRSets(context.Background(), z.ID, "", "edge-a")
	if err != nil || len(rrs) == 0 {
		t.Fatalf("%d, %v", len(rrs), err)
	}
	for _, r := range rrs {
		if !strings.Contains(r.Name, "edge-a") && !slices.ContainsFunc(r.Values, func(v string) bool { return strings.Contains(v, "edge-a") }) {
			t.Errorf("%s %s does not match", r.Name, r.Type)
		}
	}
	if _, err := f.svc.RRSets(context.Background(), "nope", "", ""); !errors.Is(err, dns.ErrNoZone) {
		t.Errorf("unknown zone: %v", err)
	}
}

// Review finding (#28): a new token that cannot see the managed zone must not replace the stored token, and the old
// zone must not stay as if validated.
func TestConnectTokenWithoutManagedZoneStoresNothing(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	first := connect(t, f)
	before, _ := f.st.R.ListSecretMeta(ctx)
	f.srv.Override = func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == "/v1/zones" { // this token sees only the other zone
			_, _ = w.Write([]byte(`{"zones":[{"id":100002,"name":"example.org","ttl":3600}],"meta":{"pagination":{"next_page":null}}}`))
			return true
		}
		return false
	}
	res, err := f.svc.Connect(ctx, "test", "", dns.ConnectInput{Type: "hetzner", Token: secrets.New("other-" + hetznertest.Token),
		DefaultTTL: 600, RootDomain: "example.com"})
	if err != nil || res.Zone != nil || len(res.Zones) != 1 {
		t.Fatalf("res = %+v, %v; want the zone list and no zone", res, err)
	}
	after, _ := f.st.R.ListSecretMeta(ctx)
	if len(after) != 1 || after[0].ID != before[0].ID {
		t.Errorf("stored token replaced: %+v → %+v", before, after)
	}
	p, z, err := f.svc.Current(ctx)
	if err != nil || z == nil || z.ID != first.Zone.ID || p.DefaultTTL != 3600 {
		t.Errorf("stored state changed: %+v %+v %v", p, z, err)
	}
	f.srv.Override = nil
	if _, err := f.svc.Refresh(ctx, first.Zone.ID); err != nil {
		t.Errorf("stored token no longer reads the zone: %v", err)
	}
}
