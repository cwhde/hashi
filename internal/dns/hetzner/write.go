package hetzner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/dns"
)

// OwnerLabel is the RRSet label that marks an RRSet as Hashi's in the Hetzner console (03 §3). The probe capture
// confirmed the key is accepted (write/put_labels_a.json).
const OwnerLabel = "hashi.io/owner"

const (
	actionPollEvery = 500 * time.Millisecond
	actionTimeout   = 2 * time.Minute
)

// ErrActionFailed: an asynchronous Action ended in "error" or did not finish in time.
var ErrActionFailed = errors.New("hetzner: action failed")

type recordJSON struct {
	Value string `json:"value"`
}

// actionJSON is the Action every write except a label change answers with (201, status "running" at first).
type actionJSON struct {
	ID      int64           `json:"id"`
	Status  string          `json:"status"`
	Command string          `json:"command"`
	Error   json.RawMessage `json:"error"`
}

// Plan diffs desired against observed and refuses anything the adapter may never send (NS/SOA, protected RRSets).
func (c *Client) Plan(_ context.Context, _ dns.Zone, desired, observed []dns.RRSet) (dns.ProviderPlan, error) {
	changes := dns.Diff(desired, observed)
	for _, ch := range changes {
		if err := dns.CheckWritable(ch); err != nil {
			return dns.ProviderPlan{}, err
		}
	}
	return dns.ProviderPlan{Changes: changes}, nil
}

// Apply sends the plan's changes in order and waits for each Action. Every change is checked before the first
// request, so a forbidden change sends nothing. A delete of an RRSet that is already gone is success.
func (c *Client) Apply(ctx context.Context, zone dns.Zone, plan dns.ProviderPlan) error {
	for _, ch := range plan.Changes {
		if err := dns.CheckWritable(ch); err != nil {
			return err
		}
	}
	for _, ch := range plan.Changes {
		name, typ := ch.Key()
		if err := c.apply(ctx, zone, ch); err != nil {
			return fmt.Errorf("%s %s %s: %w", ch.Op, name, typ, err)
		}
	}
	return nil
}

func (c *Client) apply(ctx context.Context, zone dns.Zone, ch dns.Change) error {
	base := "/zones/" + url.PathEscape(zone.ID) + "/rrsets"
	switch ch.Op {
	case dns.OpCreate:
		r := ch.After
		body := map[string]any{"name": r.Name, "type": r.Type, "ttl": r.TTL, "records": records(*r)}
		raw, _, err := c.do(ctx, http.MethodPost, base, nil, body, false)
		if err != nil {
			return err
		}
		if err := c.waitAction(ctx, raw); err != nil {
			return err
		}
		if r.Owner != "" {
			return c.putLabels(ctx, base, *r, map[string]string{})
		}
		return nil
	case dns.OpUpdate:
		r, o := ch.After, ch.Before
		path := base + "/" + rrsetPath(*r)
		if !slices.Equal(r.Values, o.Values) {
			raw, _, err := c.do(ctx, http.MethodPost, path+"/actions/set_records", nil, map[string]any{"records": records(*r)}, true)
			if err != nil {
				return err
			}
			if err := c.waitAction(ctx, raw); err != nil {
				return err
			}
		}
		if !equalTTL(r.TTL, o.TTL) {
			raw, _, err := c.do(ctx, http.MethodPost, path+"/actions/change_ttl", nil, map[string]any{"ttl": r.TTL}, true)
			if err != nil {
				return err
			}
			if err := c.waitAction(ctx, raw); err != nil {
				return err
			}
		}
		if r.Owner != "" && r.Owner != o.Owner {
			return c.putLabels(ctx, base, *r, o.Labels)
		}
		return nil
	case dns.OpLabel:
		return c.putLabels(ctx, base, *ch.After, ch.Before.Labels)
	case dns.OpDelete:
		raw, _, err := c.do(ctx, http.MethodDelete, base+"/"+rrsetPath(*ch.Before), nil, nil, true)
		var ae *APIError
		if errors.As(err, &ae) && ae.Status == http.StatusNotFound {
			return nil // already gone (write/error_delete_missing.json)
		}
		if err != nil {
			return err
		}
		return c.waitAction(ctx, raw)
	}
	return fmt.Errorf("unknown change %q", ch.Op)
}

// putLabels replaces the RRSet's labels (synchronous, 200) with the observed ones plus the owner marker, so labels
// set by others in the console survive.
func (c *Client) putLabels(ctx context.Context, base string, r dns.RRSet, observed map[string]string) error {
	labels := maps.Clone(observed)
	if labels == nil {
		labels = map[string]string{}
	}
	labels[OwnerLabel] = r.Owner
	_, _, err := c.do(ctx, http.MethodPut, base+"/"+rrsetPath(r), nil, map[string]any{"labels": labels}, true)
	return err
}

// AddValue adds one value with add_records, which creates a missing RRSet with ttl; a value that is already there is
// answered 422 "duplicate value" and is success (write/challenge_add_missing.json, challenge_add_duplicate.json).
func (c *Client) AddValue(ctx context.Context, zone dns.Zone, name, typ string, ttl int, value string) error {
	r := dns.RRSet{Name: name, Type: typ, Values: []string{value}}
	if err := dns.CheckWritable(dns.Change{Op: dns.OpCreate, After: &r}); err != nil {
		return err
	}
	path := "/zones/" + url.PathEscape(zone.ID) + "/rrsets/" + rrsetPath(r) + "/actions/add_records"
	raw, _, err := c.do(ctx, http.MethodPost, path, nil, map[string]any{"ttl": ttl, "records": records(r)}, true) // a retried add answers "duplicate value"
	var ae *APIError
	if errors.As(err, &ae) && ae.Status == http.StatusUnprocessableEntity && strings.HasPrefix(ae.Message, "duplicate value") {
		return nil
	}
	if err != nil {
		return fmt.Errorf("add %s %s value: %w", name, typ, err)
	}
	return c.waitAction(ctx, raw)
}

// RemoveValue removes one value with remove_records. A missing value or RRSet is answered 404 and is success; removing
// the last value deletes the RRSet (write/challenge_remove_*.json, challenge_get_after_last.json).
func (c *Client) RemoveValue(ctx context.Context, zone dns.Zone, name, typ, value string) error {
	r := dns.RRSet{Name: name, Type: typ, Values: []string{value}}
	if err := dns.CheckWritable(dns.Change{Op: dns.OpDelete, Before: &r}); err != nil {
		return err
	}
	path := "/zones/" + url.PathEscape(zone.ID) + "/rrsets/" + rrsetPath(r) + "/actions/remove_records"
	raw, _, err := c.do(ctx, http.MethodPost, path, nil, map[string]any{"records": records(r)}, true)
	var ae *APIError
	if errors.As(err, &ae) && ae.Status == http.StatusNotFound {
		return nil
	}
	if err != nil {
		return fmt.Errorf("remove %s %s value: %w", name, typ, err)
	}
	return c.waitAction(ctx, raw)
}

// waitAction polls GET /actions/{id} until the Action in a write response succeeds or fails.
func (c *Client) waitAction(ctx context.Context, raw []byte) error {
	var resp struct {
		Action actionJSON `json:"action"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil || resp.Action.ID == 0 {
		return fmt.Errorf("hetzner: write response without an action: %w", err)
	}
	a := resp.Action
	deadline := c.now().Add(actionTimeout)
	for {
		switch a.Status {
		case "success":
			return nil
		case "error":
			return fmt.Errorf("%w: %s: %s", ErrActionFailed, a.Command, actionError(a.Error))
		}
		if c.now().After(deadline) {
			return fmt.Errorf("%w: %s (action %d) still %q after %s", ErrActionFailed, a.Command, a.ID, a.Status, actionTimeout)
		}
		if err := c.sleep(ctx, actionPollEvery); err != nil {
			return err
		}
		body, _, err := c.do(ctx, http.MethodGet, "/actions/"+strconv.FormatInt(a.ID, 10), nil, nil, true)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return fmt.Errorf("hetzner: decode action %d: %w", a.ID, err)
		}
		a = resp.Action
	}
}

// actionError renders an Action's error. Only `null` was captured; a non-null error is shown with its message when it
// has the usual {code, message} shape, else as raw JSON.
func actionError(raw json.RawMessage) string {
	var e struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Message != "" {
		return e.Code + ": " + e.Message
	}
	return string(raw)
}

// Verify reads the zone back and reports every change whose RRSet does not match.
func (c *Client) Verify(ctx context.Context, zone dns.Zone, plan dns.ProviderPlan) error {
	rrsets, err := c.ListRRSets(ctx, zone)
	if err != nil {
		return err
	}
	type key struct{ name, typ string }
	got := make(map[key]dns.RRSet, len(rrsets))
	for _, r := range rrsets {
		got[key{r.Name, r.Type}] = r
	}
	var bad []string
	for _, ch := range plan.Changes {
		name, typ := ch.Key()
		r, ok := got[key{name, typ}]
		switch {
		case ch.Op == dns.OpDelete:
			if ok {
				bad = append(bad, name+" "+typ+" still exists")
			}
		case !ok:
			bad = append(bad, name+" "+typ+" is missing")
		case r.Hash() != ch.After.Hash():
			bad = append(bad, name+" "+typ+" has different values or TTL")
		case ch.After.Owner != "" && r.Owner != ch.After.Owner:
			bad = append(bad, name+" "+typ+" lacks the ownership label")
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("%w: %s", dns.ErrVerify, strings.Join(bad, "; "))
	}
	return nil
}

// records renders values as the API stores them: TXT re-quoted and split (EncodeTXT), everything else verbatim.
func records(r dns.RRSet) []recordJSON {
	out := make([]recordJSON, len(r.Values))
	for i, v := range r.Values {
		if r.Type == "TXT" {
			v = dns.EncodeTXT(v)
		}
		out[i] = recordJSON{Value: v}
	}
	return out
}

// rrsetPath is the RRSet's id in URLs: name/type (write/set_records_a.json: /rrsets/_hashi-test/A).
func rrsetPath(r dns.RRSet) string {
	return url.PathEscape(r.Name) + "/" + url.PathEscape(r.Type)
}

func equalTTL(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
