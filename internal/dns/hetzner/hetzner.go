// Package hetzner is the DNS provider adapter for the Hetzner Cloud API (03-dns.md §2). Everything here follows the
// recorded responses in testdata/contracts/hetzner (captured 2026-10-28): request paths, pagination via
// meta.pagination.next_page, RateLimit-* headers with a Unix reset time, and the {"error": {"code", "message"}} body.
// The write side (write.go) follows the probe capture in testdata/contracts/hetzner/write.
package hetzner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.juzo.io/juzo/hashi/internal/dns"
	"git.juzo.io/juzo/hashi/internal/secrets"
)

// BaseURL is the Hetzner Cloud API.
const BaseURL = "https://api.hetzner.cloud/v1"

const (
	perPage     = 25 // the page size used in the capture
	maxAttempts = 3
	// maxWait bounds a single rate-limit wait; a longer one fails with dns.ErrRateLimited instead of hanging a request.
	maxWait = 30 * time.Second
)

// ErrLegacyOrInvalidToken is what a 401 becomes: the legacy DNS console token and a wrong or revoked project token
// look the same to the Cloud API, so the message names the fix for both (03 §2).
var ErrLegacyOrInvalidToken = fmt.Errorf("%w: create a Hetzner Cloud project API token with read/write access "+
	"(Cloud Console → project → Security → API tokens); tokens from the old DNS Console no longer work", dns.ErrUnauthorized)

// APIError is an error response the adapter has no more specific meaning for.
type APIError struct {
	Status  int
	Code    string
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("hetzner api: %d %s: %s", e.Status, e.Code, e.Message)
}

// Client reads zones and RRSets. It is safe for concurrent use.
type Client struct {
	base  string
	token secrets.Value
	http  *http.Client
	now   func() time.Time
	sleep func(context.Context, time.Duration) error

	mu        sync.Mutex
	exhausted time.Time // RateLimit-Remaining hit 0; no request before this time
}

// Option configures a Client (tests point it at a replay server and a fake clock).
type Option func(*Client)

// WithBaseURL replaces BaseURL.
func WithBaseURL(u string) Option { return func(c *Client) { c.base = strings.TrimSuffix(u, "/") } }

// WithHTTPClient replaces the default HTTP client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithClock replaces time.Now and the context-aware sleep.
func WithClock(now func() time.Time, sleep func(context.Context, time.Duration) error) Option {
	return func(c *Client) { c.now, c.sleep = now, sleep }
}

// New returns a client for a Cloud project token.
func New(token secrets.Value, opts ...Option) *Client {
	c := &Client{base: BaseURL, token: token, http: &http.Client{Timeout: 30 * time.Second}, now: time.Now, sleep: sleepCtx}
	for _, o := range opts {
		o(c)
	}
	return c
}

var _ dns.Provider = (*Client)(nil)

// Capabilities are static for Hetzner. Every managed type was created, changed and deleted in the probe capture
// (testdata/contracts/hetzner/write).
func (c *Client) Capabilities(context.Context) (dns.Capabilities, error) {
	return dns.Capabilities{
		ManagedTypes:     []string{"A", "AAAA", "CNAME", "MX", "TXT", "SRV", "CAA"},
		ReadOnlyTypes:    []string{"NS", "SOA"},
		MultiValue:       true,
		ZoneDefaultTTL:   true,
		OwnershipMarker:  true,
		ChangeProtection: true,
	}, nil
}

type pagination struct {
	NextPage *int `json:"next_page"`
}

type meta struct {
	Pagination pagination `json:"pagination"`
}

type zoneJSON struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	TTL  int    `json:"ttl"`
}

func (z zoneJSON) model() dns.Zone {
	return dns.Zone{ID: strconv.FormatInt(z.ID, 10), Name: dns.CanonicalName(z.Name), TTL: z.TTL}
}

type rrsetJSON struct {
	Name       string            `json:"name"`
	Type       string            `json:"type"`
	TTL        *int              `json:"ttl"`
	Labels     map[string]string `json:"labels"`
	Protection struct {
		Change bool `json:"change"`
	} `json:"protection"`
	Records []struct {
		Value string `json:"value"`
	} `json:"records"`
}

func (r rrsetJSON) model() dns.RRSet {
	vals := make([]string, len(r.Records))
	for i, rec := range r.Records {
		vals[i] = rec.Value
	}
	return dns.Normalize(dns.RRSet{Name: r.Name, Type: r.Type, TTL: r.TTL, Values: vals, Labels: r.Labels,
		Protected: r.Protection.Change, Owner: r.Labels[OwnerLabel]})
}

// ListZones returns every zone the token can see (token validation, 03 §2).
func (c *Client) ListZones(ctx context.Context) ([]dns.Zone, error) {
	var out []dns.Zone
	err := c.pages(ctx, "/zones", func(raw []byte) error {
		var p struct {
			Zones []zoneJSON `json:"zones"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return err
		}
		for _, z := range p.Zones {
			out = append(out, z.model())
		}
		return nil
	})
	return out, err
}

// ResolveZone lists the zones and picks the longest suffix match.
func (c *Client) ResolveZone(ctx context.Context, fqdn string) (dns.Zone, error) {
	zones, err := c.ListZones(ctx)
	if err != nil {
		return dns.Zone{}, err
	}
	z, err := dns.ResolveFromList(zones, fqdn)
	if err != nil {
		return dns.Zone{}, fmt.Errorf("%w for %s among the %d zones this token can see", err, fqdn, len(zones))
	}
	return z, nil
}

// GetZone reads one zone by ID; an unknown ID is dns.ErrZoneNotFound.
func (c *Client) GetZone(ctx context.Context, id string) (dns.Zone, error) {
	raw, _, err := c.get(ctx, "/zones/"+url.PathEscape(id), nil)
	if err != nil {
		return dns.Zone{}, err
	}
	var p struct {
		Zone zoneJSON `json:"zone"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return dns.Zone{}, fmt.Errorf("hetzner: decode zone: %w", err)
	}
	return p.Zone.model(), nil
}

// ListRRSets returns every RRSet of zone, normalized.
func (c *Client) ListRRSets(ctx context.Context, zone dns.Zone) ([]dns.RRSet, error) {
	var out []dns.RRSet
	err := c.pages(ctx, "/zones/"+url.PathEscape(zone.ID)+"/rrsets", func(raw []byte) error {
		var p struct {
			RRSets []rrsetJSON `json:"rrsets"`
		}
		if err := json.Unmarshal(raw, &p); err != nil {
			return err
		}
		for _, r := range p.RRSets {
			out = append(out, r.model())
		}
		return nil
	})
	return out, err
}

// pages follows meta.pagination.next_page until it is null.
func (c *Client) pages(ctx context.Context, path string, each func([]byte) error) error {
	page := 1
	for seen := 0; ; seen++ {
		if seen > 1000 {
			return fmt.Errorf("hetzner: %s: more than 1000 pages, giving up", path)
		}
		q := url.Values{"page": {strconv.Itoa(page)}, "per_page": {strconv.Itoa(perPage)}}
		raw, _, err := c.get(ctx, path, q)
		if err != nil {
			return err
		}
		if err := each(raw); err != nil {
			return fmt.Errorf("hetzner: decode %s page %d: %w", path, page, err)
		}
		var m struct {
			Meta meta `json:"meta"`
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			return fmt.Errorf("hetzner: decode %s pagination: %w", path, err)
		}
		next := m.Meta.Pagination.NextPage
		if next == nil {
			return nil
		}
		if *next <= page {
			return fmt.Errorf("hetzner: %s: next_page %d does not advance past %d", path, *next, page)
		}
		page = *next
	}
}

// get performs an idempotent GET; a 404 is dns.ErrZoneNotFound.
func (c *Client) get(ctx context.Context, path string, q url.Values) ([]byte, http.Header, error) {
	body, h, err := c.do(ctx, http.MethodGet, path, q, nil, true)
	var ae *APIError
	if errors.As(err, &ae) && ae.Status == http.StatusNotFound {
		return nil, nil, fmt.Errorf("%w: %s", dns.ErrZoneNotFound, ae.Message)
	}
	return body, h, err
}

// do sends one request with rate-limit backoff. A 429 is retried for every request (it was not processed);
// 5xx and transport errors only when idempotent, because a lost response to a create may still have created it.
func (c *Client) do(ctx context.Context, method, path string, q url.Values, payload any, idempotent bool) ([]byte, http.Header, error) {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	var reqBody []byte
	if payload != nil {
		var err error
		if reqBody, err = json.Marshal(payload); err != nil {
			return nil, nil, fmt.Errorf("hetzner: encode %s %s: %w", method, path, err)
		}
	}
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if err := c.waitForBudget(ctx); err != nil {
			return nil, nil, err
		}
		var rd io.Reader
		if reqBody != nil {
			rd = bytes.NewReader(reqBody)
		}
		req, err := http.NewRequestWithContext(ctx, method, u, rd)
		if err != nil {
			return nil, nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.token.Reveal())
		req.Header.Set("Accept", "application/json")
		if reqBody != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			lastErr = fmt.Errorf("hetzner: %s %s: %w", method, path, err)
			if !idempotent {
				return nil, nil, lastErr
			}
			if err := c.sleep(ctx, backoff(attempt)); err != nil {
				return nil, nil, err
			}
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
		_ = resp.Body.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("hetzner: read %s: %w", path, err)
		}
		c.noteRateLimit(resp.Header)
		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			return body, resp.Header, nil
		case resp.StatusCode == http.StatusUnauthorized:
			return nil, nil, ErrLegacyOrInvalidToken
		case resp.StatusCode == http.StatusTooManyRequests:
			// No 429 was captured; RateLimit-Reset (if sent) decides the wait in waitForBudget, else backoff.
			lastErr = fmt.Errorf("%w: %w", dns.ErrRateLimited, apiError(resp.StatusCode, body))
			c.markExhausted(resp.Header, attempt)
		case resp.StatusCode >= 500:
			lastErr = apiError(resp.StatusCode, body)
			if !idempotent {
				return nil, nil, lastErr
			}
			if err := c.sleep(ctx, backoff(attempt)); err != nil {
				return nil, nil, err
			}
		default:
			return nil, nil, apiError(resp.StatusCode, body)
		}
	}
	return nil, nil, lastErr
}

func apiError(status int, body []byte) *APIError {
	var e struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &e) != nil || e.Error.Code == "" {
		return &APIError{Status: status, Code: "unknown", Message: http.StatusText(status)}
	}
	return &APIError{Status: status, Code: e.Error.Code, Message: e.Error.Message}
}

// noteRateLimit remembers when the hourly budget is used up (RateLimit-Remaining 0 until RateLimit-Reset).
func (c *Client) noteRateLimit(h http.Header) {
	if h.Get("RateLimit-Remaining") != "0" {
		return
	}
	if reset, ok := resetTime(h); ok {
		c.mu.Lock()
		c.exhausted = reset
		c.mu.Unlock()
	}
}

func (c *Client) markExhausted(h http.Header, attempt int) {
	until, ok := resetTime(h)
	if !ok {
		until = c.now().Add(backoff(attempt))
	}
	c.mu.Lock()
	c.exhausted = until
	c.mu.Unlock()
}

func resetTime(h http.Header) (time.Time, bool) {
	s, err := strconv.ParseInt(h.Get("RateLimit-Reset"), 10, 64)
	if err != nil || s <= 0 {
		return time.Time{}, false
	}
	return time.Unix(s, 0), true
}

// waitForBudget sleeps until the rate-limit reset, or fails if that is further away than maxWait.
func (c *Client) waitForBudget(ctx context.Context) error {
	c.mu.Lock()
	until := c.exhausted
	c.mu.Unlock()
	d := until.Sub(c.now())
	if d <= 0 {
		return nil
	}
	if d > maxWait {
		return fmt.Errorf("%w until %s", dns.ErrRateLimited, until.UTC().Format(time.RFC3339))
	}
	if err := c.sleep(ctx, d); err != nil {
		return err
	}
	c.mu.Lock()
	if !c.exhausted.After(until) {
		c.exhausted = time.Time{}
	}
	c.mu.Unlock()
	return nil
}

func backoff(attempt int) time.Duration { return time.Duration(1<<(attempt-1)) * time.Second }

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
