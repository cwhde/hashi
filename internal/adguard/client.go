// Package adguard manages DNS rewrites on AdGuard Home instances (03 §8) through its control API
// (testdata/contracts/adguard). AdGuard takes almost anything: it stores exact duplicates and empty domains, and
// deleting a missing rule still answers 200. So input is checked here and callers verify by reading the list back.
package adguard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"git.juzo.io/juzo/hashi/internal/secrets"
)

// Rewrite is one DNS rewrite. Enabled is a pointer on the wire: older instances may leave it out, which means on.
type Rewrite struct {
	Domain  string `json:"domain"`
	Answer  string `json:"answer"`
	Enabled *bool  `json:"enabled,omitempty"`
}

// IsEnabled reports whether the rewrite answers queries.
func (r Rewrite) IsEnabled() bool { return r.Enabled == nil || *r.Enabled }

// Status is the part of /control/status Hashi reads.
type Status struct {
	Version string `json:"version"`
	Running bool   `json:"running"`
}

// Errors callers map.
var (
	ErrAuth    = errors.New("AdGuard refused the username or password")
	ErrInvalid = errors.New("invalid rewrite")
)

// Client talks to one instance. Base holds scheme, host and an optional path prefix.
type Client struct {
	Base     string
	Username string
	Password secrets.Value
	HTTP     *http.Client
	// Host overrides the Host header when Base names an address (the connection's expected hostname).
	Host string
}

// Check refuses a rewrite AdGuard would store but could never answer with.
func (r Rewrite) Check() error {
	if d := strings.TrimSpace(r.Domain); d == "" || d != r.Domain || strings.ContainsAny(d, " /") {
		return fmt.Errorf("%w: domain %q", ErrInvalid, r.Domain)
	}
	if _, err := netip.ParseAddr(r.Answer); err != nil {
		return fmt.Errorf("%w: answer %q is not an address", ErrInvalid, r.Answer)
	}
	return nil
}

func (c Client) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.Base, "/")+"/control/"+path, rd)
	if err != nil {
		return fmt.Errorf("adguard: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Host != "" {
		req.Host = c.Host
	}
	if c.Username != "" {
		req.SetBasicAuth(c.Username, c.Password.Reveal())
	}
	h := c.HTTP
	if h == nil {
		h = http.DefaultClient
	}
	res, err := h.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err
		}
		return fmt.Errorf("adguard: %s: %w", path, err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("adguard: %s: %w", path, err)
	}
	switch {
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return ErrAuth
	case res.StatusCode != http.StatusOK:
		return fmt.Errorf("adguard: %s: HTTP %d: %s", path, res.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("adguard: %s: unexpected answer: %w", path, err)
		}
	}
	return nil
}

// Status reads the instance's version and state; it also proves the credentials.
func (c Client) Status(ctx context.Context) (Status, error) {
	var s Status
	return s, c.do(ctx, http.MethodGet, "status", nil, &s)
}

// List returns every rewrite, manual ones and duplicates included.
func (c Client) List(ctx context.Context) ([]Rewrite, error) {
	var l []Rewrite
	return l, c.do(ctx, http.MethodGet, "rewrite/list", nil, &l)
}

// Add creates a rewrite. AdGuard does not refuse an exact duplicate: the caller compares with List first.
func (c Client) Add(ctx context.Context, r Rewrite) error {
	if err := r.Check(); err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, "rewrite/add", r, nil)
}

// Update replaces target with update. A missing target is an error from AdGuard.
func (c Client) Update(ctx context.Context, target, update Rewrite) error {
	if err := update.Check(); err != nil {
		return err
	}
	return c.do(ctx, http.MethodPut, "rewrite/update", map[string]Rewrite{"target": target, "update": update}, nil)
}

// Delete removes one rule matching r. AdGuard answers 200 when nothing matched, and leaves any exact duplicate.
func (c Client) Delete(ctx context.Context, r Rewrite) error {
	return c.do(ctx, http.MethodPost, "rewrite/delete", Rewrite{Domain: r.Domain, Answer: r.Answer}, nil)
}
