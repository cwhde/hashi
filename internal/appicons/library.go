package appicons

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// LibraryBase is the selfh.st icon collection through jsDelivr (testdata/contracts/selfhst): index.json, and files at
// /{svg,png,webp}/<reference>[-light|-dark].<ext>. Icons by selfh.st, licensed CC BY 4.0.
const LibraryBase = "https://cdn.jsdelivr.net/gh/selfhst/icons@main"

const (
	indexTTL   = 24 * time.Hour
	maxResults = 30
	maxIndex   = 5 << 20
)

// reference is the form of selfh.st's file names, so a reference never leaves its folder in a file address.
var reference = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

var notAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func norm(s string) string { return notAlnum.ReplaceAllString(strings.ToLower(s), "") }

// entry is one icon of index.json; the availability fields are "Yes" or "No".
type entry struct {
	Name      string `json:"Name"`
	Reference string `json:"Reference"`
	SVG       string `json:"SVG"`
	PNG       string `json:"PNG"`
	Light     string `json:"Light"`
	Tags      string `json:"Tags"`

	name, ref, tags string
}

// Hit is one search result.
type Hit struct {
	Name      string
	Reference string
	Format    string // svg or png
	Light     bool   // a white variant exists for dark themes
}

// Library searches the collection; the index is fetched at most once a day, and a stale one is used while the CDN is
// away.
type Library struct {
	Base   string
	Client *http.Client
	now    func() time.Time

	mu    sync.Mutex
	index []entry
	at    time.Time
}

// NewLibrary uses the collection at base (LibraryBase in production).
func NewLibrary(base string, c *http.Client) *Library {
	return &Library{Base: strings.TrimSuffix(base, "/"), Client: c, now: time.Now}
}

func (l *Library) load(ctx context.Context) ([]entry, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.index != nil && l.now().Sub(l.at) < indexTTL {
		return l.index, nil
	}
	p, err := get(ctx, l.Client, l.Base+"/index.json", "", maxIndex)
	if err == nil && p.status != http.StatusOK {
		err = fmt.Errorf("icon index answered %d", p.status)
	}
	var raw []entry
	if err == nil {
		err = json.Unmarshal(p.body, &raw)
	}
	if err != nil {
		if l.index != nil {
			return l.index, nil
		}
		return nil, fmt.Errorf("icon library: %w", err)
	}
	idx := make([]entry, 0, len(raw))
	for _, e := range raw {
		if !reference.MatchString(e.Reference) || (e.SVG != "Yes" && e.PNG != "Yes") {
			continue
		}
		e.name, e.ref, e.tags = norm(e.Name), norm(e.Reference), strings.ToLower(e.Tags)
		idx = append(idx, e)
	}
	l.index, l.at = idx, l.now()
	return idx, nil
}

func (e entry) hit() Hit {
	f := "png"
	if e.SVG == "Yes" {
		f = "svg"
	}
	return Hit{Name: e.Name, Reference: e.Reference, Format: f, Light: e.Light == "Yes"}
}

// Search returns up to 30 icons for q: exact name or reference first, then prefix, then contains, then a tag match;
// ties by name.
func (l *Library) Search(ctx context.Context, q string) ([]Hit, error) {
	idx, err := l.load(ctx)
	if err != nil {
		return nil, err
	}
	nq, tq := norm(q), strings.ToLower(strings.TrimSpace(q))
	type ranked struct {
		e    entry
		rank int
	}
	var found []ranked
	for _, e := range idx {
		r := -1
		switch {
		case nq == "" || e.name == nq || e.ref == nq:
			r = 0
		case strings.HasPrefix(e.name, nq) || strings.HasPrefix(e.ref, nq):
			r = 1
		case strings.Contains(e.name, nq) || strings.Contains(e.ref, nq):
			r = 2
		case tq != "" && strings.Contains(e.tags, tq):
			r = 3
		}
		if r >= 0 {
			found = append(found, ranked{e, r})
		}
	}
	slices.SortStableFunc(found, func(a, b ranked) int { return cmp.Or(a.rank-b.rank, strings.Compare(a.e.name, b.e.name)) })
	out := make([]Hit, 0, min(maxResults, len(found)))
	for _, f := range found[:min(maxResults, len(found))] {
		out = append(out, f.e.hit())
	}
	return out, nil
}

// Lookup finds one icon by its exact reference.
func (l *Library) Lookup(ctx context.Context, ref string) (Hit, error) {
	if !reference.MatchString(ref) {
		return Hit{}, fmt.Errorf("%w: unknown icon %q", ErrInvalid, ref)
	}
	idx, err := l.load(ctx)
	if err != nil {
		return Hit{}, err
	}
	for _, e := range idx {
		if e.Reference == ref {
			return e.hit(), nil
		}
	}
	return Hit{}, fmt.Errorf("%w: unknown icon %q", ErrInvalid, ref)
}

// URL is where the collection serves h (light: its white variant).
func (l *Library) URL(h Hit, light bool) string {
	name := h.Reference
	if light {
		name += "-light"
	}
	return l.Base + "/" + h.Format + "/" + name + "." + h.Format
}

// Fetch downloads h, and its white variant when there is one.
func (l *Library) Fetch(ctx context.Context, h Hit) (dark Found, light *Found, err error) {
	one := func(u string) (Found, error) {
		p, err := get(ctx, l.Client, u, "", 1<<20)
		if err != nil {
			return Found{}, fmt.Errorf("icon library: %w", err)
		}
		if p.status != http.StatusOK {
			return Found{}, fmt.Errorf("icon library answered %d for %s", p.status, u)
		}
		return found(p.body, "selfh.st "+h.Reference)
	}
	if dark, err = one(l.URL(h, false)); err != nil {
		return Found{}, nil, err
	}
	if h.Light {
		if f, err := one(l.URL(h, true)); err == nil {
			light = &f
		}
	}
	return dark, light, nil
}
