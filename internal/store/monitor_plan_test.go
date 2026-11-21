package store_test

import (
	"context"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

// The admin's endpoint list took 0.4 s on prod: the last hour's strips (ListRoundsSince) read all 2.2 million samples
// through the primary key, because it sorted by endpoint, instead of the hour through the time index.
func TestMonitorStripsUseTheTimeIndex(t *testing.T) {
	st := storetest.New(t)
	rows, err := st.Reader.QueryContext(context.Background(),
		"EXPLAIN QUERY PLAN SELECT endpoint_id, at, verdict FROM monitor_samples WHERE check_id = '' AND at >= ? ORDER BY at", 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var plan []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		plan = append(plan, detail)
	}
	if got := strings.Join(plan, " | "); !strings.Contains(got, "USING INDEX monitor_samples_at") {
		t.Errorf("the strips read samples without the time index: %s", got)
	}
}
