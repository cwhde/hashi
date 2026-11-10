package httpapi

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/security"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

// The dashboard sums decisions and events of the period into tiles and a series, keeps response rows out of the
// decision counts, ranks refused addresses and countries, and shows the WAF rules of a refusal (06 §13).
func TestSecurityDashboard(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	now := time.Now()
	s := &Server{Store: st, Log: log, Security: security.NewEngine(st, log, time.Now)}
	minute := now.Add(-10*time.Minute).Unix() / 60 * 60_000
	old := now.Add(-3*time.Hour).Unix() / 60 * 60_000 // outside 1h
	for _, b := range []db.AddSecurityBucketParams{
		{BucketStart: minute, Country: "DE", Decision: security.DecisionAllow, RequestCount: 90},
		{BucketStart: minute, Country: "CN", Decision: security.DecisionDeny, RequestCount: 7, DeniedCount: 7},
		{BucketStart: minute, Country: "CN", Decision: security.DecisionChallenge, RequestCount: 3, ChallengedCount: 3},
		{BucketStart: minute, Decision: security.DecisionResponse, StatusClass: "2xx", RequestCount: 90},
		{BucketStart: old, Country: "US", Decision: security.DecisionDeny, RequestCount: 50, DeniedCount: 50},
	} {
		if err := st.Q.AddSecurityBucket(ctx, b); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.Q.AddSecurityTop(ctx, db.AddSecurityTopParams{BucketStart: minute, Address: "198.51.100.7", RequestCount: 10,
		DeniedCount: 7, ChallengedCount: 3}); err != nil {
		t.Fatal(err)
	}
	at := now.Add(-5 * time.Minute).UnixMilli()
	for i, e := range []db.InsertSecurityEventParams{
		{EventType: "challenge_entered", Severity: "warning", Source: security.SourceRate},
		{EventType: "challenge_entered", Severity: "warning", Source: security.SourceRate},
		{EventType: "challenge_solved", Severity: "info"},
		{EventType: "ban_created", Severity: "critical", Source: security.SourceChallengeFailed},
		{EventType: "waf_match", Severity: "warning", Source: security.SourceWAF, RequestMethod: "GET", RequestPath: "/x",
			MetadataJson: `{"rules":[941100,941110]}`},
	} {
		e.ID, e.OccurredAt, e.Address = fmt.Sprint(i), at+int64(i), "198.51.100.7"
		if e.MetadataJson == "" {
			e.MetadataJson = "{}"
		}
		if err := st.Q.InsertSecurityEvent(ctx, e); err != nil {
			t.Fatal(err)
		}
	}

	h := gen.GetSecurityDashboardParamsRange("1h")
	res, err := s.GetSecurityDashboard(ctx, gen.GetSecurityDashboardRequestObject{Params: gen.GetSecurityDashboardParams{Range: &h}})
	if err != nil {
		t.Fatal(err)
	}
	d := gen.SecurityDashboard(res.(gen.GetSecurityDashboard200JSONResponse))
	if tt := d.Totals; tt.Allowed != 90 || tt.Denied != 7 || tt.Challenged != 3 || tt.WafMatches != 1 || tt.Bans != 1 {
		t.Fatalf("totals %+v (response rows and the old minute must stay out)", tt)
	}
	if len(d.Series) < 60 || d.StepSeconds != 60 {
		t.Fatalf("%d points of %d s: one per minute, empty ones included", len(d.Series), d.StepSeconds)
	}
	if f := d.Funnel; f.Challenged != 2 || f.Solved != 1 || f.Banned != 1 {
		t.Fatalf("funnel %+v", f)
	}
	if len(d.TopAddresses) != 1 || d.TopAddresses[0].Refused != 10 || d.TopAddresses[0].State != gen.SecurityTopAddressStateNone {
		t.Fatalf("top %+v", d.TopAddresses)
	}
	if len(d.Countries) != 1 || d.Countries[0].Country != "CN" || d.Countries[0].Refused != 10 {
		t.Fatalf("countries %+v", d.Countries)
	}
	if len(d.Events) != 5 || d.Events[0].Type != "waf_match" || fmt.Sprint(d.Events[0].Rules) != "[941100 941110]" {
		t.Fatalf("events, newest first with the WAF rules: %+v", d.Events)
	}
}

// The counters endpoints page the view, edit one address and reset by filter (06 §13.1, §14).
func TestSecurityCountersEndpoints(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	eng := security.NewEngine(st, log, time.Now)
	s := &Server{Store: st, Log: log, Security: eng, Audit: audit.New([]byte("k"))}
	for i := range 3 {
		a := netip.MustParseAddr(fmt.Sprintf("198.51.100.%d", 10+i))
		if ok, err := eng.Challenge(ctx, a, security.Cause{Source: security.SourceRate}); !ok || err != nil {
			t.Fatalf("challenge %v %v", ok, err)
		}
	}
	limit := 2
	res, err := s.ListSecurityCounters(ctx, gen.ListSecurityCountersRequestObject{Params: gen.ListSecurityCountersParams{Limit: &limit}})
	if err != nil {
		t.Fatal(err)
	}
	page := gen.CounterList(res.(gen.ListSecurityCounters200JSONResponse))
	if page.Total != 3 || len(page.Rows) != 2 || page.NextCursor == nil {
		t.Fatalf("first page %+v", page)
	}
	res, _ = s.ListSecurityCounters(ctx, gen.ListSecurityCountersRequestObject{Params: gen.ListSecurityCountersParams{Limit: &limit, Cursor: page.NextCursor}})
	if p := gen.CounterList(res.(gen.ListSecurityCounters200JSONResponse)); len(p.Rows) != 1 || p.NextCursor != nil {
		t.Fatalf("second page %+v", p)
	}
	bad := "not-an-address"
	if _, err := s.ListSecurityCounters(ctx, gen.ListSecurityCountersRequestObject{Params: gen.ListSecurityCountersParams{Q: &bad}}); err == nil {
		t.Fatal("a bad q must be refused")
	}

	three := 3
	er, err := s.EditAddressCounters(ctx, gen.EditAddressCountersRequestObject{Address: "198.51.100.10", Body: &gen.EditAddressCountersJSONRequestBody{BanCount: &three}})
	if err != nil {
		t.Fatal(err)
	}
	if r := gen.CounterRow(er.(gen.EditAddressCounters200JSONResponse)); r.BanCount != 3 || r.State != gen.CounterRowStateChallenged {
		t.Fatalf("edited %+v", r)
	}
	if _, err := s.EditAddressCounters(ctx, gen.EditAddressCountersRequestObject{Address: "198.51.100.10", Body: &gen.EditAddressCountersJSONRequestBody{}}); err == nil {
		t.Fatal("an empty edit must be refused")
	}

	q := "198.51.100.0/24"
	minBan := 1
	rr, err := s.ResetSecurityCounters(ctx, gen.ResetSecurityCountersRequestObject{Body: &gen.ResetSecurityCountersJSONRequestBody{
		AllMatching: &gen.CounterFilter{Q: &q, MinBan: &minBan}}})
	if err != nil {
		t.Fatal(err)
	}
	if n := rr.(gen.ResetSecurityCounters200JSONResponse).Count; n != 1 {
		t.Fatalf("reset by filter changed %d records, want the one with a ban counter", n)
	}

	// The audit keeps before and after (06 §16): the edit 0 -> 3, the reset the values it replaced.
	evs, err := st.Q.ListAuditEvents(ctx, db.ListAuditEventsParams{Lim: 10})
	if err != nil {
		t.Fatal(err)
	}
	detail := map[string]string{}
	for _, e := range evs {
		detail[e.Action] = e.DetailJson
	}
	if d := detail["security.counters_edited"]; !strings.Contains(d, `"ban":{"after":3,"before":0}`) || strings.Contains(d, `"challenge"`) {
		t.Errorf("edit detail %s", d)
	}
	if d := detail["security.counters_reset"]; !strings.Contains(d, `"before":[{"address":"198.51.100.10","ban":3,"challenge":1}]`) {
		t.Errorf("reset detail %s", d)
	}
}

// "Ban now" with a duration, "challenge now" on an allowed address (refused, invariant 20) and removal (05a §8).
func TestDynamicEntriesByHand(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	eng := security.NewEngine(st, log, time.Now)
	home := netip.MustParseAddr("192.0.2.50")
	eng.UseAllowed(func(a netip.Addr) bool { return a == home })
	s := &Server{Store: st, Log: log, Security: eng, Audit: audit.New([]byte("k"))}
	two := 120
	res, err := s.AddDynamicEntry(ctx, gen.AddDynamicEntryRequestObject{Id: security.SetBlocked,
		Body: &gen.AddDynamicEntryJSONRequestBody{Address: "198.51.100.7", DurationSeconds: &two}})
	if err != nil {
		t.Fatal(err)
	}
	r := res.(gen.AddDynamicEntry200JSONResponse)
	if !r.Created || r.Counters.State != gen.CounterRowStateBanned || r.Counters.Until == nil ||
		time.Until(*r.Counters.Until) > 2*time.Minute || r.Counters.BanCount != 1 {
		t.Fatalf("ban now %+v", r)
	}
	res, err = s.AddDynamicEntry(ctx, gen.AddDynamicEntryRequestObject{Id: security.SetChallenge,
		Body: &gen.AddDynamicEntryJSONRequestBody{Address: home.String()}})
	if err != nil || res.(gen.AddDynamicEntry200JSONResponse).Created {
		t.Fatalf("an allowed address must not be challenged: %+v %v", res, err)
	}
	if _, err := s.AddDynamicEntry(ctx, gen.AddDynamicEntryRequestObject{Id: "some-set",
		Body: &gen.AddDynamicEntryJSONRequestBody{Address: "198.51.100.8"}}); err == nil {
		t.Fatal("only the dynamic sets take entries")
	}
	if _, err := s.RemoveDynamicEntry(ctx, gen.RemoveDynamicEntryRequestObject{Id: security.SetBlocked, Address: "198.51.100.7"}); err != nil {
		t.Fatal(err)
	}
	if eng.Banned(netip.MustParseAddr("198.51.100.7")) {
		t.Fatal("still banned after removal")
	}
}
