package hetzner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/dns/hetzner/hetznertest"
	"git.juzo.io/juzo/hashi/internal/secrets"
)

// writeFixture is a recorded write exchange with its request body.
type writeFixture struct {
	hetznertest.Fixture
	RequestBody json.RawMessage `json:"request_body"`
}

func loadWrite(t *testing.T, name string) writeFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(hetznertest.Dir(), "write", name+".json")) //nolint:gosec // test fixture
	if err != nil {
		t.Fatal(err)
	}
	var f writeFixture
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

// writeReplay answers exactly the given fixtures, in any order, and fails the test on any other request or on a
// request body that differs from the recorded one. Action polls answer the recorded poll first and the captured
// success shape (write/set_records_a_unchanged_action_poll.json) after that, so a "running" Action finishes.
type writeReplay struct {
	t        *testing.T
	mu       sync.Mutex
	byReq    map[string]writeFixture
	polls    map[string]int
	success  writeFixture
	seen     []string
	bodyEq   func(got, want json.RawMessage) bool
	override func(w http.ResponseWriter, r *http.Request) bool
}

func newWriteReplay(t *testing.T, names ...string) (*writeReplay, *Client) {
	t.Helper()
	wr := &writeReplay{t: t, byReq: map[string]writeFixture{}, polls: map[string]int{},
		success: loadWrite(t, "set_records_a_unchanged_action_poll"), bodyEq: jsonEqual}
	for _, n := range names {
		f := loadWrite(t, n)
		wr.byReq[f.Request] = f
	}
	srv := httptest.NewServer(http.HandlerFunc(wr.serve))
	t.Cleanup(srv.Close)
	now := time.Unix(1_793_000_000, 0)
	c := New(secrets.New(hetznertest.Token), WithBaseURL(srv.URL+"/v1"),
		WithClock(func() time.Time { return now }, func(_ context.Context, d time.Duration) error { now = now.Add(d); return nil }))
	return wr, c
}

func (wr *writeReplay) serve(w http.ResponseWriter, r *http.Request) {
	wr.mu.Lock()
	defer wr.mu.Unlock()
	line := r.Method + " " + r.URL.Path
	wr.seen = append(wr.seen, line)
	if wr.override != nil && wr.override(w, r) {
		return
	}
	body, _ := io.ReadAll(r.Body)
	f, ok := wr.byReq[line]
	if !ok {
		wr.t.Errorf("unexpected request %s %s", line, body)
		w.WriteHeader(http.StatusTeapot)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/v1/actions/") {
		wr.polls[line]++
		if wr.polls[line] > 1 {
			f = wr.success
		}
	} else if len(f.RequestBody) > 0 && string(f.RequestBody) != "null" && !wr.bodyEq(body, f.RequestBody) {
		wr.t.Errorf("%s body\n got %s\nwant %s", line, body, f.RequestBody)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(f.Status)
	_, _ = w.Write(f.Body)
}

func jsonEqual(a, b json.RawMessage) bool {
	var x, y any
	if json.Unmarshal(a, &x) != nil || json.Unmarshal(b, &y) != nil {
		return false
	}
	ja, _ := json.Marshal(x)
	jb, _ := json.Marshal(y)
	return bytes.Equal(ja, jb)
}

// fromRequest builds the desired RRSet a recorded create request was made for.
func fromRequest(t *testing.T, f writeFixture) dns.RRSet {
	t.Helper()
	var b struct {
		Name    string       `json:"name"`
		Type    string       `json:"type"`
		TTL     *int         `json:"ttl"`
		Records []recordJSON `json:"records"`
	}
	if err := json.Unmarshal(f.RequestBody, &b); err != nil {
		t.Fatal(err)
	}
	r := dns.RRSet{Name: b.Name, Type: b.Type, TTL: b.TTL}
	for _, rec := range b.Records {
		r.Values = append(r.Values, rec.Value)
	}
	return dns.Normalize(r)
}

var zone100001 = dns.Zone{ID: "100001", Name: "example.com", TTL: 3600}

// Every recorded create is reproduced byte for byte (values in the recorded order after sorting) and its Action polled.
func TestContract_CreateEveryManagedType(t *testing.T) {
	for _, name := range []string{"create_a", "create_aaaa", "create_cname", "create_mx", "create_srv", "create_caa"} {
		t.Run(name, func(t *testing.T) {
			f := loadWrite(t, name)
			wr, c := newWriteReplay(t, name, name+"_action_poll")
			r := fromRequest(t, f)
			if err := c.Apply(t.Context(), zone100001, dns.ProviderPlan{Changes: []dns.Change{{Op: dns.OpCreate, After: &r}}}); err != nil {
				t.Fatal(err)
			}
			if n := len(wr.seen); n != 3 { // create, the recorded "running" poll, the success poll
				t.Errorf("requests %v", wr.seen)
			}
		})
	}
}

// TXT: Hashi sends quoted strings; the recorded request split one value into two strings, Hashi joins and re-splits
// at 255 bytes, which the API stores verbatim. Equality is on the decoded text.
func TestContract_CreateTXTQuotesAndSplits(t *testing.T) {
	f := loadWrite(t, "create_txt")
	wr, c := newWriteReplay(t, "create_txt", "create_txt_action_poll")
	wr.bodyEq = func(got, want json.RawMessage) bool {
		var g, w struct {
			Records []recordJSON `json:"records"`
		}
		_ = json.Unmarshal(got, &g)
		_ = json.Unmarshal(want, &w)
		if len(g.Records) != len(w.Records) {
			return false
		}
		for i := range g.Records {
			gt, ok1 := dns.DecodeTXT(g.Records[i].Value)
			wt, ok2 := dns.DecodeTXT(w.Records[i].Value)
			if !ok1 || !ok2 || gt != wt {
				return false
			}
		}
		return true
	}
	r := fromRequest(t, f)
	if err := c.Apply(t.Context(), zone100001, dns.ProviderPlan{Changes: []dns.Change{{Op: dns.OpCreate, After: &r}}}); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("x", 300)
	enc := dns.EncodeTXT(long)
	if got, ok := dns.DecodeTXT(enc); !ok || got != long || !strings.Contains(enc, `" "`) {
		t.Errorf("300 bytes encoded as %q", enc)
	}
	for _, s := range strings.Split(enc, `" "`) {
		if len(strings.Trim(s, `"`)) > 255 {
			t.Errorf("string of %d bytes (write/create_txt_long.json: 422 above 255)", len(s))
		}
	}
	if got, _ := dns.DecodeTXT(dns.EncodeTXT(`a "b" \c`)); got != `a "b" \c` {
		t.Errorf("escapes do not round-trip: %q", got)
	}
}

// An update sends set_records for changed values and change_ttl for a changed TTL, and nothing for what is equal.
func TestContract_UpdateValuesAndTTL(t *testing.T) {
	_, c := newWriteReplay(t, "set_records_a", "set_records_a_action_poll", "change_ttl_a", "change_ttl_a_action_poll")
	ttl := 600
	before := dns.Normalize(dns.RRSet{Name: "_hashi-test", Type: "A", Values: []string{"192.0.2.10", "192.0.2.11"}})
	after := dns.Normalize(dns.RRSet{Name: "_hashi-test", Type: "A", TTL: &ttl, Values: []string{"192.0.2.12"}})
	if err := c.Apply(t.Context(), zone100001, dns.ProviderPlan{Changes: []dns.Change{{Op: dns.OpUpdate, Before: &before, After: &after}}}); err != nil {
		t.Fatal(err)
	}

	// Back to the zone default: change_ttl with ttl null, values untouched.
	_, c = newWriteReplay(t, "change_ttl_a_null", "change_ttl_a_null_action_poll")
	b2, a2 := after, after
	a2.TTL = nil
	if err := c.Apply(t.Context(), zone100001, dns.ProviderPlan{Changes: []dns.Change{{Op: dns.OpUpdate, Before: &b2, After: &a2}}}); err != nil {
		t.Fatal(err)
	}
}

// The owner marker is a synchronous label PUT that keeps labels others set.
func TestContract_OwnerLabel(t *testing.T) {
	wr, c := newWriteReplay(t, "put_labels_a")
	before := dns.Normalize(dns.RRSet{Name: "_hashi-test", Type: "A", Values: []string{"192.0.2.13"}})
	after := before
	after.Owner = "instance-0001"
	if err := c.Apply(t.Context(), zone100001, dns.ProviderPlan{Changes: []dns.Change{{Op: dns.OpLabel, Before: &before, After: &after}}}); err != nil {
		t.Fatal(err)
	}
	wr.bodyEq = func(got, _ json.RawMessage) bool {
		return jsonEqual(got, json.RawMessage(`{"labels":{"env":"x","hashi.io/owner":"instance-0001"}}`))
	}
	before.Labels = map[string]string{"env": "x"}
	if err := c.Apply(t.Context(), zone100001, dns.ProviderPlan{Changes: []dns.Change{{Op: dns.OpLabel, Before: &before, After: &after}}}); err != nil {
		t.Fatal(err)
	}
}

// Deleting waits for the Action; deleting an RRSet that is already gone (404) is success.
func TestContract_Delete(t *testing.T) {
	wr, c := newWriteReplay(t, "delete_aaaa", "delete_aaaa_action_poll", "error_delete_missing")
	aaaa := dns.Normalize(dns.RRSet{Name: "_hashi-test", Type: "AAAA", Values: []string{"2001:db8::10"}})
	missing := dns.Normalize(dns.RRSet{Name: "missing._hashi-test", Type: "A", Values: []string{"192.0.2.1"}})
	if err := c.Apply(t.Context(), zone100001, dns.ProviderPlan{Changes: []dns.Change{
		{Op: dns.OpDelete, Before: &aaaa}, {Op: dns.OpDelete, Before: &missing}}}); err != nil {
		t.Fatal(err)
	}
	if len(wr.seen) != 4 {
		t.Errorf("requests %v", wr.seen)
	}
}

// Recorded errors surface with the API's message; a create is never retried.
func TestContract_WriteErrors(t *testing.T) {
	for _, name := range []string{"error_create_exists", "error_cname_conflict", "error_a_invalid", "error_srv_invalid", "error_caa_invalid", "create_txt_long"} {
		t.Run(name, func(t *testing.T) {
			f := loadWrite(t, name)
			wr, c := newWriteReplay(t, name)
			wr.bodyEq = func(json.RawMessage, json.RawMessage) bool { return true }
			r := fromRequest(t, f)
			err := c.Apply(t.Context(), zone100001, dns.ProviderPlan{Changes: []dns.Change{{Op: dns.OpCreate, After: &r}}})
			var ae *APIError
			if !errors.As(err, &ae) || ae.Status != f.Status || ae.Message == "" {
				t.Fatalf("err = %v", err)
			}
			if len(wr.seen) != 1 {
				t.Errorf("requests %v", wr.seen)
			}
		})
	}
}

// A 5xx on a create is not retried (it may have been created); on an idempotent write it is.
func TestWrite_RetryOnlyIdempotent(t *testing.T) {
	wr, c := newWriteReplay(t, "set_records_a", "set_records_a_action_poll")
	fails := 0
	wr.override = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && fails == 0 {
			fails++
			w.WriteHeader(http.StatusBadGateway)
			return true
		}
		return false
	}
	before := dns.Normalize(dns.RRSet{Name: "_hashi-test", Type: "A", Values: []string{"192.0.2.10"}})
	after := dns.Normalize(dns.RRSet{Name: "_hashi-test", Type: "A", Values: []string{"192.0.2.12"}})
	if err := c.Apply(t.Context(), zone100001, dns.ProviderPlan{Changes: []dns.Change{{Op: dns.OpUpdate, Before: &before, After: &after}}}); err != nil {
		t.Fatalf("set_records after one 502: %v", err)
	}

	wr, c = newWriteReplay(t, "create_a")
	wr.override = func(w http.ResponseWriter, _ *http.Request) bool { w.WriteHeader(http.StatusBadGateway); return true }
	r := fromRequest(t, loadWrite(t, "create_a"))
	if err := c.Apply(t.Context(), zone100001, dns.ProviderPlan{Changes: []dns.Change{{Op: dns.OpCreate, After: &r}}}); err == nil || len(wr.seen) != 1 {
		t.Fatalf("create: err %v after %v", err, wr.seen)
	}
}

// An Action that never finishes fails after the timeout instead of hanging the apply.
func TestWrite_ActionTimeout(t *testing.T) {
	wr, c := newWriteReplay(t, "delete_aaaa", "delete_aaaa_action_poll")
	wr.success = loadWrite(t, "delete_aaaa_action_poll") // keeps answering "running"
	aaaa := dns.Normalize(dns.RRSet{Name: "_hashi-test", Type: "AAAA", Values: []string{"2001:db8::10"}})
	err := c.Apply(t.Context(), zone100001, dns.ProviderPlan{Changes: []dns.Change{{Op: dns.OpDelete, Before: &aaaa}}})
	if !errors.Is(err, ErrActionFailed) {
		t.Fatalf("err = %v", err)
	}
}

// Invariant 2 at the adapter: a plan touching NS/SOA or a change-protected RRSet is refused before any request.
func TestInvariant_2_AdapterRefusesNSSOAAndProtected(t *testing.T) {
	wr, c := newWriteReplay(t)
	ok := dns.Normalize(dns.RRSet{Name: "_hashi-test", Type: "A", Values: []string{"192.0.2.10"}})
	ns := dns.Normalize(dns.RRSet{Name: "@", Type: "NS", Values: []string{"ns1.example.net."}})
	soa := dns.Normalize(dns.RRSet{Name: "@", Type: "SOA", Values: []string{"x"}})
	locked := ok
	locked.Protected = true
	for _, plan := range [][]dns.Change{
		{{Op: dns.OpCreate, After: &ok}, {Op: dns.OpDelete, Before: &ns}},
		{{Op: dns.OpCreate, After: &ok}, {Op: dns.OpUpdate, Before: &soa, After: &soa}},
		{{Op: dns.OpCreate, After: &ns}},
		{{Op: dns.OpCreate, After: &ok}, {Op: dns.OpDelete, Before: &locked}},
	} {
		if err := c.Apply(t.Context(), zone100001, dns.ProviderPlan{Changes: plan}); !errors.Is(err, dns.ErrForbiddenChange) {
			t.Errorf("err = %v", err)
		}
	}
	if _, err := c.Plan(t.Context(), zone100001, nil, []dns.RRSet{ns}); !errors.Is(err, dns.ErrForbiddenChange) {
		t.Errorf("Plan deleting NS: %v", err)
	}
	if len(wr.seen) != 0 {
		t.Errorf("requests sent: %v", wr.seen)
	}
}
