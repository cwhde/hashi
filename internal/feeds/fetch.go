package feeds

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"syscall"
	"time"
)

// Fetcher downloads feeds without reaching inside the network (06 §8): HTTPS only unless a feed allows HTTP, TLS
// verified, a few redirects, a size and time limit, and every address it would connect to checked against the denied
// ranges, the final one in the dialer itself, so DNS rebinding cannot slip through.
type Fetcher struct {
	UserAgent string
	MaxBytes  int64
	Timeout   time.Duration
	// AllowInternal lets tests and the dev build fetch from loopback servers. Never set in production.
	AllowInternal bool
	// RootCAs replaces the system roots (tests with a local TLS server); nil in production.
	RootCAs *x509.CertPool
}

// NewFetcher is the production fetcher.
func NewFetcher(version string) *Fetcher {
	return &Fetcher{UserAgent: "Hashi/" + version + " (feed refresh)", MaxBytes: 32 << 20, Timeout: time.Minute}
}

// ErrDenied means the URL or an address it leads to is not allowed.
var ErrDenied = errors.New("feed address not allowed")

const maxRedirects = 5

// denied are the ranges a feed is never fetched from: Hashi's own networks and the cloud metadata address.
var denied = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8", "169.254.0.0/16",
		"172.16.0.0/12", "192.0.0.0/24", "192.168.0.0/16", "198.18.0.0/15", "224.0.0.0/3",
		"::/128", "::1/128", "::ffff:0:0/96", "64:ff9b::/96", "fc00::/7", "fe80::/10", "ff00::/8"} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()

func (f *Fetcher) allowed(a netip.Addr) bool {
	if f.AllowInternal {
		return true
	}
	a = a.Unmap()
	for _, p := range denied {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

// checkURL validates a feed URL and every redirect target.
func (f *Fetcher) checkURL(u *url.URL, allowHTTP bool) error {
	switch {
	case u.Scheme != "https" && (u.Scheme != "http" || !allowHTTP):
		return fmt.Errorf("%w: %s:// (feeds use https)", ErrDenied, u.Scheme)
	case u.User != nil || u.Hostname() == "":
		return fmt.Errorf("%w: no host or credentials in the URL", ErrDenied)
	}
	if a, err := netip.ParseAddr(u.Hostname()); err == nil && !f.allowed(a) {
		return fmt.Errorf("%w: %s is an internal address", ErrDenied, a)
	}
	return nil
}

// CheckURL is the check for a URL a user typed, before anything is fetched.
func (f *Fetcher) CheckURL(raw string, allowHTTP bool) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrDenied, err)
	}
	return f.checkURL(u, allowHTTP)
}

func (f *Fetcher) client(allowHTTP bool) *http.Client {
	d := &net.Dialer{Timeout: 15 * time.Second, Control: func(_, address string, _ syscall.RawConn) error {
		ap, err := netip.ParseAddrPort(address)
		if err != nil {
			return fmt.Errorf("%w: %s", ErrDenied, address)
		}
		if !f.allowed(ap.Addr()) {
			return fmt.Errorf("%w: resolves to the internal address %s", ErrDenied, ap.Addr())
		}
		return nil
	}}
	tr := &http.Transport{DialContext: d.DialContext, Proxy: nil, TLSHandshakeTimeout: 15 * time.Second,
		TLSClientConfig:       &tls.Config{RootCAs: f.RootCAs, MinVersion: tls.VersionTLS12},
		ResponseHeaderTimeout: 30 * time.Second, MaxIdleConns: 1, IdleConnTimeout: time.Second}
	return &http.Client{Transport: tr, Timeout: f.Timeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("more than %d redirects", maxRedirects)
		}
		return f.checkURL(req.URL, allowHTTP)
	}}
}

// Download is one fetch. NotModified means the validators matched (304) and Body is empty.
type Download struct {
	Body         []byte
	Hash         string
	ETag         string
	LastModified string
	NotModified  bool
}

// Get downloads raw, sending the previous validators.
func (f *Fetcher) Get(ctx context.Context, raw string, allowHTTP bool, etag, lastModified string) (Download, error) {
	if err := f.CheckURL(raw, allowHTTP); err != nil {
		return Download{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return Download{}, err
	}
	req.Header.Set("User-Agent", f.UserAgent)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}
	c := f.client(allowHTTP)
	defer c.CloseIdleConnections()
	resp, err := c.Do(req)
	if err != nil {
		return Download{}, fmt.Errorf("download: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	d := Download{ETag: resp.Header.Get("ETag"), LastModified: resp.Header.Get("Last-Modified")}
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotModified:
		d.NotModified = true
		return d, nil
	default:
		return Download{}, fmt.Errorf("download: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, f.MaxBytes+1))
	if err != nil {
		return Download{}, fmt.Errorf("download: %w", err)
	}
	if int64(len(b)) > f.MaxBytes {
		return Download{}, fmt.Errorf("download: larger than %d MB", f.MaxBytes>>20)
	}
	sum := sha256.Sum256(b)
	d.Body, d.Hash = b, hex.EncodeToString(sum[:])
	return d, nil
}
