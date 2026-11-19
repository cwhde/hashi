package security

import (
	"context"
	"fmt"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

func hourRows(t *testing.T, q *db.Queries) map[string]int64 {
	t.Helper()
	rows, err := q.SecurityHourRefusedByCountry(context.Background(), db.SecurityHourRefusedByCountryParams{Since: 0, Until: 1 << 62})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int64{}
	for _, r := range rows {
		out[r.Country] = r.Refused
	}
	return out
}

// Closed hours are rolled from the minutes; the open hour waits; a late minute in a recent hour is picked up on the
// next run; each hour keeps at most 500 addresses, the most refused ones (01 §4.3, bug 99).
func TestTelemetryRollup(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	c := &clock{t: time.Date(2026, 11, 19, 12, 30, 0, 0, time.UTC)}
	tel := NewTelemetry(st, c.now)
	hour := func(h int) int64 { return time.Date(2026, 11, 19, h, 0, 0, 0, time.UTC).UnixMilli() }
	add := func(at int64, country string, denied int64) {
		t.Helper()
		if err := st.Q.AddSecurityBucket(ctx, db.AddSecurityBucketParams{BucketStart: at, Country: country, Decision: DecisionDeny,
			PathPrefix: fmt.Sprint("/p", at%7), RequestCount: denied, DeniedCount: denied}); err != nil {
			t.Fatal(err)
		}
	}
	for m := range 60 {
		add(hour(10)+int64(m)*60_000, "CN", 1) // one whole hour, many path prefixes
	}
	add(hour(11)+5*60_000, "US", 4)
	add(hour(12)+60_000, "DE", 9) // the open hour
	for i := range 600 {
		if err := st.Q.AddSecurityTop(ctx, db.AddSecurityTopParams{BucketStart: hour(10), Address: fmt.Sprintf("198.51.%d.%d", i/250, i%250),
			RequestCount: 1, DeniedCount: int64(i)}); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := tel.Rollup(ctx); err != nil || n != 2 {
		t.Fatalf("rolled %d hours, %v", n, err)
	}
	if got := hourRows(t, st.R); got["CN"] != 60 || got["US"] != 4 || got["DE"] != 0 {
		t.Fatalf("hours %v: 10:00 and 11:00 rolled, 12:00 still open", got)
	}
	top, err := st.R.SecurityHourTopRefused(ctx, db.SecurityHourTopRefusedParams{Since: 0, Until: 1 << 62})
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 100 || top[0].Denied != 599 {
		t.Fatalf("top: %d rows (the query keeps 100), first %+v", len(top), top[0])
	}

	// A late access-log line for 11:00, and the clock moves on: 11:00 is recomputed, 12:00 now closed.
	add(hour(11)+10*60_000, "US", 1)
	c.add(time.Hour)
	if _, err := tel.Rollup(ctx); err != nil {
		t.Fatal(err)
	}
	if got := hourRows(t, st.R); got["US"] != 5 || got["DE"] != 9 || got["CN"] != 60 {
		t.Fatalf("after the late line %v", got)
	}
	if until, err := RolledUntil(ctx, st.R); err != nil || until != hour(13) {
		t.Fatalf("rolled until %d, %v", until, err)
	}
}

// The hour's address list is capped at 500, the most refused first.
func TestTelemetryRollupTopCap(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	start := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	// Every minute with address rows also has bucket rows; the rollup starts from those.
	if err := st.Q.AddSecurityBucket(ctx, db.AddSecurityBucketParams{BucketStart: start.UnixMilli(), Decision: DecisionDeny, RequestCount: 1}); err != nil {
		t.Fatal(err)
	}
	for i := range 600 {
		if err := st.Q.AddSecurityTop(ctx, db.AddSecurityTopParams{BucketStart: start.UnixMilli() + int64(i%60)*60_000,
			Address: fmt.Sprintf("198.51.%d.%d", i/250, i%250), RequestCount: 1, DeniedCount: int64(i)}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := NewTelemetry(st, func() time.Time { return start.Add(2 * time.Hour) }).Rollup(ctx); err != nil {
		t.Fatal(err)
	}
	var n, least int64
	if err := st.Reader.QueryRowContext(ctx, `SELECT COUNT(*), MIN(denied_count) FROM security_ip_top_1h`).Scan(&n, &least); err != nil {
		t.Fatal(err)
	}
	if n != 500 || least != 100 {
		t.Fatalf("%d addresses kept, least refused %d: want the 500 most refused", n, least)
	}
}

// Retention deletes minutes after 30 days but only once an hour holds them, hours after a year and events after 90
// days, in batches, also when more than a batch shares one minute.
func TestTelemetryRetain(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	now := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	old := now.Add(-31 * 24 * time.Hour).Truncate(time.Hour).UnixMilli()
	for i := range 6000 { // one minute, more rows than a batch
		if err := st.Q.AddSecurityBucket(ctx, db.AddSecurityBucketParams{BucketStart: old, Country: "CN", Asn: int64(i),
			Decision: DecisionDeny, RequestCount: 1, DeniedCount: 1}); err != nil {
			t.Fatal(err)
		}
	}
	recent := now.Add(-time.Hour).UnixMilli()
	if err := st.Q.AddSecurityBucket(ctx, db.AddSecurityBucketParams{BucketStart: recent, Decision: DecisionAllow, RequestCount: 1}); err != nil {
		t.Fatal(err)
	}
	for i, at := range []time.Time{now.Add(-91 * 24 * time.Hour), now.Add(-time.Hour)} {
		if err := st.Q.InsertSecurityEvent(ctx, db.InsertSecurityEventParams{ID: fmt.Sprint(i), OccurredAt: at.UnixMilli(), Address: "198.51.100.1",
			EventType: "waf_match", Severity: "warning", MetadataJson: "{}"}); err != nil {
			t.Fatal(err)
		}
	}
	count := func(table string) (n int) {
		t.Helper()
		if err := st.Reader.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	tel := NewTelemetry(st, func() time.Time { return now })
	// Nothing rolled yet: the old minutes stay.
	if _, err := tel.Retain(ctx); err != nil {
		t.Fatal(err)
	}
	if n := count("security_buckets_1m"); n != 6001 {
		t.Fatalf("%d minute rows: none may go before an hour holds them", n)
	}
	if n := count("security_events"); n != 1 {
		t.Fatalf("%d events: the one past 90 days goes", n)
	}
	if _, err := tel.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if n := count("security_buckets_1m"); n != 1 {
		t.Fatalf("%d minute rows after the rollup", n)
	}
	if hourRows(t, st.R)["CN"] != 6000 {
		t.Fatal("the rolled hour must keep the old minute's counts")
	}
	// A year on, the hour goes too.
	tel.now = func() time.Time { return now.Add(366 * 24 * time.Hour) }
	if _, err := tel.Retain(ctx); err != nil {
		t.Fatal(err)
	}
	if hourRows(t, st.R)["CN"] != 0 {
		t.Fatal("hours past a year must go")
	}
}
