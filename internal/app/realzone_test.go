//go:build dev && realzone

package app

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/auth/authtest"
	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/dns/hetzner"
	"git.juzo.io/juzo/hashi/internal/secrets"
)

// TestRealZoneRecordPath is S3's record gate against the real DNS zone (11-delivery.md §3 row S3): the same build,
// the real Hetzner adapter, the whole admin API over HTTP, a virtual authenticator for the passkey (no HTTPS before
// S5). It writes only the probe name `_hashi-test` with documentation addresses and removes it again.
//
//	HASHI_REAL_TOKEN=… HASHI_REAL_ROOT=<root domain> go test -tags 'dev realzone' -run RealZone -v ./internal/app/
//
// Nothing here names the real zone; the evidence lines print plan results and hashes only.
func TestRealZoneRecordPath(t *testing.T) {
	token, root := os.Getenv("HASHI_REAL_TOKEN"), os.Getenv("HASHI_REAL_ROOT")
	if token == "" || root == "" {
		t.Skip("HASHI_REAL_TOKEN and HASHI_REAL_ROOT not set")
	}
	ctx := context.Background()
	direct := hetzner.New(secrets.New(token))
	zone, err := direct.ResolveZone(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	read := func() map[string]dns.RRSet {
		t.Helper()
		all, err := direct.ListRRSets(ctx, zone)
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]dns.RRSet{}
		for _, r := range all {
			out[r.Name+" "+r.Type] = r
		}
		return out
	}
	before := read()
	if _, ok := before[dns.ProbeName+" A"]; ok {
		t.Fatal("a _hashi-test A record already exists; remove it first")
	}
	nsHash, soaHash := before["@ NS"].Hash(), soaShape(before["@ SOA"])
	t.Logf("evidence: zone has %d RRSets; NS hash %s…, SOA (without serial) hash %s…", len(before), nsHash[:12], soaHash[:12])

	cfg := newConfig(t) // -tags dev without HASHI_DEV_DNS_FIXTURES: the real adapter
	in := start(t, cfg)
	c := in.client()
	c.bootstrapLogin()
	admin := "hashi-s3-gate." + root
	c.mustStatus(c.do(http.MethodPut, "/api/setup/base-settings", map[string]any{"root_domain": root, "admin_domain": admin,
		"internal_url": "http://192.0.2.10:3340", "sync_interval_seconds": 3600, "theme": "graphite",
		"public_dashboard_enabled": true, "public_status_enabled": true}), 200)
	c.mustStatus(c.do(http.MethodPut, "/api/setup/dns/provider", map[string]any{"type": "hetzner", "token": token, "default_ttl": 3600}), 200)
	r := c.mustStatus(c.do(http.MethodGet, "/api/setup/dns/import", nil), 200)
	var leave []map[string]string
	for _, row := range r.body["rows"].([]any) {
		rr := row.(map[string]any)["rrset"].(map[string]any)
		leave = append(leave, map[string]string{"name": rr["name"].(string), "type": rr["type"].(string), "decision": "leave"})
	}
	c.mustStatus(c.do(http.MethodPut, "/api/setup/dns/import", map[string]any{"decisions": leave}), 200)

	// Passkey (virtual authenticator), recovery key, finish.
	a := authtest.New(t)
	origin := "https://" + admin
	r = c.mustStatus(c.do(http.MethodPost, "/api/setup/passkey/options", nil), 200)
	pub := r.body["options"].(map[string]any)["publicKey"].(map[string]any)
	cred := mustJSONMap(t, a.Register(root, origin, decodeB64(t, pub["challenge"].(string)),
		decodeB64(t, pub["user"].(map[string]any)["id"].(string))))
	c.mustStatus(c.do(http.MethodPost, "/api/setup/passkey/verify", map[string]any{"ceremony_id": r.body["ceremony_id"],
		"name": "gate", "credential": cred}), 200)
	r = c.mustStatus(c.do(http.MethodPost, "/api/setup/recovery-key", nil), 200)
	groups := strings.Split(r.body["recovery_key"].(string), "-")
	c.mustStatus(c.do(http.MethodPost, "/api/setup/recovery-key/confirm", map[string]string{"checksum": groups[len(groups)-1]}), 200)
	c.mustStatus(c.do(http.MethodPost, "/api/setup/complete", nil), 200)

	assert := func(path string) {
		t.Helper()
		r := c.mustStatus(c.do(http.MethodPost, path+"/options", nil), 200)
		ch := decodeB64(t, r.body["options"].(map[string]any)["publicKey"].(map[string]any)["challenge"].(string))
		verify := path + "/verify"
		if path == "/api/auth/reauth" {
			verify = path
		}
		c.mustStatus(c.do(http.MethodPost, verify, map[string]any{"ceremony_id": r.body["ceremony_id"],
			"credential": mustJSONMap(t, a.Assert(root, origin, ch))}), 200)
	}
	assert("/api/auth/passkeys/login")
	zoneID := c.mustStatus(c.do(http.MethodGet, "/api/dns/zones", nil), 200).body["zones"].([]any)[0].(map[string]any)["id"].(string)

	// A Hashi-owned record: saved → low-risk plan applied and verified.
	r = c.mustStatus(c.do(http.MethodPost, "/api/dns/records", map[string]any{"zone_id": zoneID, "name": dns.ProbeName, "type": "A",
		"ttl": 300, "values": []string{"192.0.2.10"}}), 200)
	plan := r.body["plan"].(map[string]any)
	if plan["status"] != "applied" || !strings.Contains(plan["result"].(string), "verified") {
		t.Fatalf("create: %s", r.raw)
	}
	recID := r.body["record"].(map[string]any)["id"].(string)
	t.Logf("evidence: create plan %s %s: %s", plan["id"], plan["status"], plan["result"])
	got, ok := read()[dns.ProbeName+" A"]
	if !ok || got.Values[0] != "192.0.2.10" || *got.TTL != 300 || got.Owner == "" {
		t.Fatalf("provider has %+v", got)
	}
	t.Logf("evidence: direct read: %s A %v TTL %d, owner label set", dns.ProbeName, got.Values, *got.TTL)

	// Deleting it is high risk: pending until approved with a fresh passkey assertion.
	r = c.mustStatus(c.do(http.MethodDelete, "/api/dns/records/"+recID, nil), 200)
	plan = r.body["plan"].(map[string]any)
	if plan["status"] != "pending" || plan["risk"] != "high" {
		t.Fatalf("delete: %s", r.raw)
	}
	if _, ok := read()[dns.ProbeName+" A"]; !ok {
		t.Fatal("deleted before approval")
	}
	pid := plan["id"].(string)
	r = c.do(http.MethodPost, "/api/sync/plans/"+pid+"/approve", nil)
	if r.status < 400 || !strings.Contains(r.raw, "reauth") {
		t.Fatalf("approve without reauth: %d %s", r.status, r.raw)
	}
	assert("/api/auth/reauth")
	c.mustStatus(c.do(http.MethodPost, "/api/sync/plans/"+pid+"/approve", nil), 200)
	r = c.mustStatus(c.do(http.MethodPost, "/api/sync/plans/"+pid+"/apply", nil), 200)
	if r.body["status"] != "applied" {
		t.Fatalf("apply: %s", r.raw)
	}
	t.Logf("evidence: delete plan %s pending → approved with reauth → %s: %s", pid, r.body["status"], r.body["result"])

	after := read()
	if _, ok := after[dns.ProbeName+" A"]; ok {
		t.Fatal("still at the provider")
	}
	if after["@ NS"].Hash() != nsHash || soaShape(after["@ SOA"]) != soaHash || len(after) != len(before) {
		t.Fatalf("zone changed: NS %v SOA %v, %d → %d RRSets", after["@ NS"].Hash() == nsHash, soaShape(after["@ SOA"]) == soaHash,
			len(before), len(after))
	}
	t.Logf("evidence: after: %d RRSets, NS hash and SOA (without the provider's serial) unchanged, no _hashi-test left", len(after))
	if strings.Contains(in.logs.String(), token) {
		t.Fatal("token in the log")
	}
}

// soaShape identifies the SOA without its serial: the provider bumps the serial on every zone change, which is not a
// change to the SOA by Hashi (invariant 2 is about Hashi's writes).
func soaShape(r dns.RRSet) string {
	f := strings.Fields(strings.Join(r.Values, " "))
	if len(f) > 2 {
		f = append(f[:2:2], f[3:]...)
	}
	return dns.Normalize(dns.RRSet{Name: r.Name, Type: r.Type, TTL: r.TTL, Values: []string{strings.Join(f, " ")}}).Hash()
}
