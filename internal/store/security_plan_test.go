package store_test

import (
	"context"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/store/storetest"
)

// Bug 99: the Security dashboard took 2.5 s even for one hour, because two of its queries read the whole table. Each
// dashboard query must reach its rows through the period, not by scanning.
func TestSecurityDashboardQueriesUseThePeriod(t *testing.T) {
	st := storetest.New(t)
	for name, q := range map[string]string{
		"recent events": `SELECT e.occurred_at FROM security_events e LEFT JOIN services s ON s.id = e.service_id
			LEFT JOIN firewall_hosts h ON h.id = e.firewall_host_id WHERE e.occurred_at >= 1
			ORDER BY e.occurred_at DESC LIMIT 30`,
		"top addresses": `SELECT address, SUM(request_count) FROM security_ip_top_1m WHERE bucket_start >= 1
			GROUP BY +address HAVING SUM(denied_count) + SUM(challenged_count) > 0 ORDER BY 2 DESC LIMIT 10`,
	} {
		rows, err := st.Reader.QueryContext(context.Background(), "EXPLAIN QUERY PLAN "+q)
		if err != nil {
			t.Fatal(name, err)
		}
		var plan []string
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				t.Fatal(name, err)
			}
			plan = append(plan, detail)
		}
		_ = rows.Close()
		got := strings.Join(plan, " | ")
		if strings.Contains(got, "SCAN security_") {
			t.Errorf("%s scans a whole table: %s", name, got)
		}
	}
}
