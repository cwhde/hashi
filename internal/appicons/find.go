// Package appicons gives each dashboard app its icon (07 §9.1, bug 149): picked by the owner (an upload or a
// selfh.st icon) or found by Hashi on the app itself, stored under <data>/assets/apps and served only by Hashi.
package appicons

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/assets"
)

// Candidate is one icon a page names, with the size it claims.
type Candidate struct {
	URL  string
	Size int  // pixels on the long side; 0 when unknown
	SVG  bool // scales to any size
	// Kind orders equal sizes: 0 manifest, 1 apple-touch-icon, 2 icon link, 3 /favicon.ico guess.
	Kind int
}

var (
	linkTag  = regexp.MustCompile(`(?is)<link\b[^>]*>`)
	attrPair = regexp.MustCompile(`(?is)([a-z-]+)\s*=\s*("([^"]*)"|'([^']*)'|([^\s"'>]+))`)
	sizeDim  = regexp.MustCompile(`(?i)^(\d+)x(\d+)$`)
)

func attrs(tag string) map[string]string {
	out := map[string]string{}
	for _, m := range attrPair.FindAllStringSubmatch(tag, -1) {
		out[strings.ToLower(m[1])] = m[3] + m[4] + m[5]
	}
	return out
}

// largest reads a sizes value ("180x180", "16x16 32x32", "any") as its largest side; "any" counts as scalable.
func largest(sizes string) (int, bool) {
	best, any := 0, false
	for _, f := range strings.Fields(sizes) {
		if strings.EqualFold(f, "any") {
			any = true
			continue
		}
		if m := sizeDim.FindStringSubmatch(f); m != nil {
			w, _ := strconv.Atoi(m[1])
			h, _ := strconv.Atoi(m[2])
			best = max(best, w, h)
		}
	}
	return best, any
}

func isSVGPath(u string) bool {
	return strings.EqualFold(path.Ext(strings.SplitN(u, "?", 2)[0]), ".svg")
}

// ParseHead reads a page's icon links and its manifest address, resolved against base. Monochrome mask icons and
// inline data: icons are left out.
func ParseHead(page []byte, base *url.URL) (cands []Candidate, manifest string) {
	for _, tag := range linkTag.FindAllString(string(page), -1) {
		a := attrs(tag)
		rel, href := " "+strings.ToLower(a["rel"])+" ", strings.TrimSpace(a["href"])
		if href == "" || strings.HasPrefix(strings.ToLower(href), "data:") {
			continue
		}
		u, err := base.Parse(href)
		if err != nil {
			continue
		}
		size, scalable := largest(a["sizes"])
		svg := scalable || isSVGPath(u.Path) || strings.Contains(a["type"], "svg")
		switch {
		case strings.Contains(rel, " manifest "):
			if manifest == "" {
				manifest = u.String()
			}
		case strings.Contains(rel, " mask-icon "):
		case strings.Contains(rel, " apple-touch-icon ") || strings.Contains(rel, " apple-touch-icon-precomposed "):
			if size == 0 {
				size = 180 // Apple's documented size when none is given
			}
			cands = append(cands, Candidate{URL: u.String(), Size: size, SVG: svg, Kind: 1})
		case strings.Contains(rel, " icon "):
			cands = append(cands, Candidate{URL: u.String(), Size: size, SVG: svg, Kind: 2})
		}
	}
	return cands, manifest
}

// ParseManifest reads a web app manifest's icons, resolved against the manifest's own address. Icons only for
// monochrome use are left out.
func ParseManifest(b []byte, base *url.URL) []Candidate {
	var m struct {
		Icons []struct {
			Src     string `json:"src"`
			Sizes   string `json:"sizes"`
			Type    string `json:"type"`
			Purpose string `json:"purpose"`
		} `json:"icons"`
	}
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	var out []Candidate
	for _, ic := range m.Icons {
		p := strings.Fields(strings.ToLower(ic.Purpose))
		if len(p) > 0 && !slices.Contains(p, "any") && !slices.Contains(p, "maskable") {
			continue // monochrome only
		}
		u, err := base.Parse(strings.TrimSpace(ic.Src))
		if err != nil || ic.Src == "" || strings.HasPrefix(ic.Src, "data:") {
			continue
		}
		size, scalable := largest(ic.Sizes)
		out = append(out, Candidate{URL: u.String(), Size: size, SVG: scalable || isSVGPath(u.Path) || strings.Contains(ic.Type, "svg"), Kind: 0})
	}
	return out
}

// want is the size an icon should have: the card shows it at 40 px, twice that on dense screens, with room to spare.
const want = 96

// Order puts the best icon first: a scalable one, then the smallest that is at least `want`, then the largest of the
// smaller ones; equal sizes go manifest, apple-touch-icon, icon link, guess. Duplicates are dropped.
func Order(c []Candidate) []Candidate {
	seen := map[string]bool{}
	out := make([]Candidate, 0, len(c))
	for _, x := range c {
		if !seen[x.URL] {
			seen[x.URL] = true
			out = append(out, x)
		}
	}
	score := func(x Candidate) int {
		switch {
		case x.SVG:
			return 0
		case x.Size >= want:
			return 1
		case x.Size == 0:
			return 3 // unknown: worth a try after the ones that say they fit
		}
		return 2
	}
	slices.SortStableFunc(out, func(a, b Candidate) int {
		if c := score(a) - score(b); c != 0 {
			return c
		}
		switch score(a) {
		case 1:
			if a.Size != b.Size {
				return a.Size - b.Size
			}
		case 2:
			if a.Size != b.Size {
				return b.Size - a.Size
			}
		}
		return a.Kind - b.Kind
	})
	return out
}

// Target is where an app can be asked: its public address and, as a fallback, its upstream with the app's Host.
type Target struct {
	Public   string // https://app.example.com/
	Upstream string // http://192.0.2.10:8080/ or https://…; empty when unknown
}

// Found is an icon Hashi took, and where it came from (shown to the owner).
type Found struct {
	Data        []byte
	ContentType string
	From        string
}

// ErrNone means the app named no usable icon on either path.
var ErrNone = errors.New("no icon found")

// Finder fetches pages and icons. Client asks public addresses; Upstream asks upstreams, which often have
// certificates for other names (self-signed, internal), so it does not verify them: it only ever reads images.
type Finder struct {
	Client   *http.Client
	Upstream *http.Client
}

// NewFinder returns a Finder with its own clients. Redirects are followed on the same host only: an app that sends
// visitors to another host (a sign-in page) shows that host's icon, not its own.
func NewFinder() Finder {
	sameHost := func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Host != via[0].URL.Host {
			return http.ErrUseLastResponse
		}
		return nil
	}
	up := http.DefaultTransport.(*http.Transport).Clone()
	up.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // reads an image from the owner's own upstream; see Finder
	return Finder{
		Client:   &http.Client{Timeout: 15 * time.Second, CheckRedirect: sameHost},
		Upstream: &http.Client{Timeout: 15 * time.Second, CheckRedirect: sameHost, Transport: up},
	}
}

const maxPage = 2 << 20

// page is what get read: the status, the address after redirects, and the body.
type page struct {
	status int
	at     *url.URL
	body   []byte
}

// get fetches u. host, when set, is sent as the Host header (the upstream path).
func get(ctx context.Context, c *http.Client, u, host string, limit int64) (page, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return page{}, err
	}
	if host != "" {
		req.Host = host
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; hashi-icons)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,image/*,*/*;q=0.5")
	resp, err := c.Do(req)
	if err != nil {
		return page{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return page{}, err
	}
	if int64(len(b)) > limit {
		return page{}, fmt.Errorf("%s: larger than %d bytes", u, limit)
	}
	return page{status: resp.StatusCode, at: resp.Request.URL, body: b}, nil
}

// Find asks the public address first. When it answers 401/403, redirects to another host, fails, or names no usable
// icon, it asks the upstream with the app's Host header. ErrNone when neither gives an image.
func (f Finder) Find(ctx context.Context, t Target) (Found, error) {
	pub, err := url.Parse(t.Public)
	if err != nil || pub.Host == "" {
		return Found{}, fmt.Errorf("public address %q", t.Public)
	}
	if got, ok := f.on(ctx, f.Client, pub, ""); ok {
		return got, nil
	}
	if t.Upstream == "" {
		return Found{}, ErrNone
	}
	up, err := url.Parse(t.Upstream)
	if err != nil || up.Host == "" {
		return Found{}, ErrNone
	}
	if got, ok := f.on(ctx, f.Upstream, up, pub.Host); ok {
		got.From = "upstream " + got.From
		return got, nil
	}
	return Found{}, ErrNone
}

// on reads the page at base and tries its icons in order. With host set, base is an upstream: icon addresses on the
// public host are asked there too, since the public host just refused.
func (f Finder) on(ctx context.Context, c *http.Client, base *url.URL, host string) (Found, bool) {
	p, err := get(ctx, c, base.String(), host, maxPage)
	if err != nil || p.status != http.StatusOK {
		return Found{}, false
	}
	at := p.at // after same-host redirects: relative links resolve against the page they are on
	cands, manifest := ParseHead(p.body, at)
	if manifest != "" {
		mu, _ := url.Parse(manifest)
		if m, err := get(ctx, c, f.local(mu, base, host), host, maxPage); err == nil && m.status == http.StatusOK {
			cands = append(cands, ParseManifest(m.body, mu)...)
		}
	}
	cands = append(cands, Candidate{URL: at.ResolveReference(&url.URL{Path: "/favicon.ico"}).String(), Size: 32, Kind: 3})
	for _, cand := range Order(cands) {
		cu, err := url.Parse(cand.URL)
		if err != nil {
			continue
		}
		r, err := get(ctx, c, f.local(cu, base, host), host, 1<<20)
		if err != nil || r.status != http.StatusOK {
			continue
		}
		ct, err := assets.Sniff(r.body)
		if err != nil {
			continue
		}
		return Found{Data: r.body, ContentType: ct, From: cu.Path}, true
	}
	return Found{}, false
}

// local maps an address on the app's public host to the upstream when asking the upstream; other hosts stay.
func (f Finder) local(u, base *url.URL, host string) string {
	if host == "" || !strings.EqualFold(u.Host, host) && !strings.EqualFold(u.Host, base.Host) {
		return u.String()
	}
	v := *u
	v.Scheme, v.Host = base.Scheme, base.Host
	return v.String()
}
