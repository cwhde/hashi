package services

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"

	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/db"
)

// MaxWAFPaths bounds a service's WAF paths; the renderer gives each path its own priority step below the next route
// (traefik.wafSlots).
const MaxWAFPaths = 30

// WAF is a service's WAF configuration (06 §9). Mode "" uses waf.default_mode.
type WAF struct {
	Mode       string    `json:"mode,omitempty"`
	Uploads    bool      `json:"uploads,omitempty"`
	Exclusions []int     `json:"exclusions,omitempty"`
	Paths      []WAFPath `json:"paths,omitempty"`
}

// WAFPath overrides the service's WAF for requests whose path matches. Mode "" uses the service's mode; uploads and
// exclusions add to the service's.
type WAFPath struct {
	ID         string `json:"id"`
	Match      string `json:"match"` // MatchPrefix or MatchExact
	Path       string `json:"path"`
	Mode       string `json:"mode,omitempty"`
	Uploads    bool   `json:"uploads,omitempty"`
	Exclusions []int  `json:"exclusions,omitempty"`
}

// EffectiveWAF is what one router gets after resolution: path → service → default.
type EffectiveWAF struct {
	Mode       string
	Uploads    bool
	Exclusions []int
}

// Off reports whether the router gets no WAF middleware.
func (e EffectiveWAF) Off() bool { return e.Mode == settings.WAFDisabled }

// RootWAF resolves the service's own WAF against the default mode. System services keep the default mode with uploads
// on: Hashi's own API uses PUT and DELETE, and their settings change only through their owning workflow (invariant 10).
func (s Service) RootWAF(defaultMode string) EffectiveWAF {
	if s.System() {
		return EffectiveWAF{Mode: defaultMode, Uploads: true}
	}
	m := s.WAF.Mode
	if m == "" {
		m = defaultMode
	}
	return EffectiveWAF{Mode: m, Uploads: s.WAF.Uploads, Exclusions: s.WAF.Exclusions}
}

// For resolves one path entry on top of the service's root.
func (e EffectiveWAF) For(p WAFPath) EffectiveWAF {
	out := EffectiveWAF{Mode: e.Mode, Uploads: e.Uploads || p.Uploads}
	if p.Mode != "" {
		out.Mode = p.Mode
	}
	out.Exclusions = ruleIDs(append(slices.Clone(e.Exclusions), p.Exclusions...))
	return out
}

func ruleIDs(ids []int) []int {
	slices.Sort(ids)
	return slices.Compact(ids)
}

// errBodyCheck: body inspection breaks request bodies above about 160 KB with the pinned plugin (06 §9).
const errBodyCheck = "checking request bodies is off until the WAF plugin's upload bug is fixed; use headers_only"

func validWAFMode(m string, field string) error {
	switch m {
	case "", settings.WAFHeadersOnly, settings.WAFDisabled:
		return nil
	case settings.WAFEnabled:
		return invalid(field, errBodyCheck)
	}
	return invalid(field, "the WAF mode must be headers_only or disabled")
}

func validExclusions(ids []int, field string) error {
	if len(ids) > 50 {
		return invalid(field, "at most 50 rule exclusions")
	}
	for _, id := range ids {
		if id < 1 || id > 999999 {
			return invalid(field, "%d is not a rule ID", id)
		}
	}
	return nil
}

// NormalizeWAF fills path IDs, sorts exclusions and checks the configuration.
func NormalizeWAF(w WAF) (WAF, error) {
	if err := validWAFMode(w.Mode, "mode"); err != nil {
		return w, err
	}
	if err := validExclusions(w.Exclusions, "exclusions"); err != nil {
		return w, err
	}
	w.Exclusions = ruleIDs(slices.Clone(w.Exclusions))
	if len(w.Paths) > MaxWAFPaths {
		return w, invalid("paths", "at most %d paths", MaxWAFPaths)
	}
	seen := map[string]bool{}
	paths := make([]WAFPath, len(w.Paths))
	for i, p := range w.Paths {
		f := fmt.Sprintf("paths[%d]", i)
		if p.Match != MatchPrefix && p.Match != MatchExact {
			return w, invalid(f+".match", "a WAF path matches by prefix or exactly")
		}
		if !strings.HasPrefix(p.Path, "/") || len(p.Path) > 256 || strings.ContainsAny(p.Path, "`\\ \t\r\n?#") {
			return w, invalid(f+".path", "a path starts with / and has no spaces, backticks, ? or #")
		}
		if seen[p.Match+p.Path] {
			return w, invalid(f+".path", "%s is listed twice", p.Path)
		}
		seen[p.Match+p.Path] = true
		if err := validWAFMode(p.Mode, f+".mode"); err != nil {
			return w, err
		}
		if err := validExclusions(p.Exclusions, f+".exclusions"); err != nil {
			return w, err
		}
		p.Exclusions = ruleIDs(slices.Clone(p.Exclusions))
		if p.ID == "" {
			p.ID = ulid.Make().String()
		}
		paths[i] = p
	}
	w.Paths = paths
	return w, nil
}

// SetWAF replaces a user service's WAF configuration. System services are refused (invariant 10).
func SetWAF(ctx context.Context, q *db.Queries, id string, w WAF, now time.Time) (Service, error) {
	cur, err := Get(ctx, q, id)
	if err != nil {
		return Service{}, err
	}
	if cur.System() {
		return Service{}, ErrSystem
	}
	if cur.Stream() {
		return Service{}, invalid("kind", "TCP and UDP services have no WAF")
	}
	if w, err = NormalizeWAF(w); err != nil {
		return Service{}, err
	}
	b, err := json.Marshal(w)
	if err != nil {
		return Service{}, err
	}
	if err := q.SetServiceWAF(ctx, db.SetServiceWAFParams{WafJson: string(b), UpdatedAt: now.UnixMilli(), ID: id}); err != nil {
		return Service{}, fmt.Errorf("store WAF: %w", err)
	}
	cur.WAF = w
	return cur, nil
}
