package store_test

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

// query returns a named query from a sqlc query file as SQLite runs it (sqlc's arguments as "?"), so a plan test checks
// the query that ships and not a copy of it.
func query(t *testing.T, file, name string) string {
	t.Helper()
	b, err := os.ReadFile("queries/" + file)
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(b), "-- name: "+name+" ")
	if !ok {
		t.Fatalf("no query %s in %s", name, file)
	}
	_, body, _ := strings.Cut(rest, "\n")
	body, _, _ = strings.Cut(body, ";")
	return regexp.MustCompile(`sqlc\.n?arg\('[a-z_]+'\)`).ReplaceAllString(body, "?")
}

func plan(t *testing.T, sql string, args ...any) string {
	t.Helper()
	st := storetest.New(t)
	rows, err := st.Reader.QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+sql, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		out = append(out, detail)
	}
	return strings.Join(out, " | ")
}

// The public status and service pages took about 150 ms uncached on a copy of prod (2026-11-22): their history read
// every check's hourly rollups through the (resolution, bucket) index (114,024 rows to keep 32,091) and, for a service
// page, all endpoints' history to show one. Naming the endpoints lets SQLite seek through the primary key instead.
func TestPublicHistoryReadsThroughThePrimaryKey(t *testing.T) {
	for _, name := range []string{"ListRoundSamplesSince", "ListHourlyRoundRollupsSince"} {
		sql := query(t, "public_layout.sql", name)
		args := make([]any, strings.Count(sql, "?"))
		for i := range args {
			args[i] = 1
		}
		if got := plan(t, sql, args...); !strings.Contains(got, "USING PRIMARY KEY") {
			t.Errorf("%s does not seek through the primary key: %s", name, got)
		}
	}
}
