package httpapi

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

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
