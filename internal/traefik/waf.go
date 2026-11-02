package traefik

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strconv"
	"strings"

	"git.juzo.io/juzo/hashi/internal/services"
)

// CRS request policy (06 §9). CRS refuses other methods and content types from the headers alone (911100, 920420), so
// the base policy adds the methods ordinary APIs use, and uploads adds WebDAV and lifts the content type check.
const (
	wafMethods       = "GET HEAD POST OPTIONS PUT PATCH DELETE"
	wafUploadMethods = wafMethods + " PROPFIND PROPPATCH MKCOL COPY MOVE LOCK UNLOCK REPORT SEARCH"
	ruleContentType  = 920420
)

// WAFMiddlewarePrefix starts every WAF middleware's name; the rest is a hash of its directives, so one middleware
// serves every router with the same configuration.
const WAFMiddlewarePrefix = "hashi-waf-"

// wafDirectives is the Coraza configuration for one effective WAF, in the order the capture used
// (testdata/contracts/coraza/dynamic.yaml): recommended base, engine on, no body access, CRS setup, request policy,
// CRS rules, then removals (they must follow the rules they remove).
func wafDirectives(e services.EffectiveWAF) []string {
	methods := wafMethods
	if e.Uploads {
		methods = wafUploadMethods
	}
	d := []string{
		"Include @coraza.conf-recommended",
		"SecRuleEngine On",
		"SecRequestBodyAccess Off",
		"Include @crs-setup.conf.example",
		`SecAction "id:900200,phase:1,pass,t:none,nolog,setvar:'tx.allowed_methods=` + methods + `'"`,
		"Include @owasp_crs/*.conf",
	}
	remove := slices.Clone(e.Exclusions)
	if e.Uploads {
		remove = append(remove, ruleContentType)
	}
	slices.Sort(remove)
	for _, id := range slices.Compact(remove) {
		d = append(d, "SecRuleRemoveById "+strconv.Itoa(id))
	}
	return d
}

// withWAF appends e's middleware to chain (none when the WAF is off) and records it in wafs. The WAF runs last: forward
// auth's Stage A rejects blocked and challenged addresses before any rule is evaluated.
func withWAF(chain []string, e services.EffectiveWAF, wafs map[string]middleware) []string {
	out := slices.Clone(chain)
	if e.Off() {
		return out
	}
	d := wafDirectives(e)
	sum := sha256.Sum256([]byte(strings.Join(d, "\n")))
	name := WAFMiddlewarePrefix + hex.EncodeToString(sum[:])[:12]
	wafs[name] = middleware{Plugin: map[string]pluginConfig{PluginName: {Directives: d}}}
	return append(out, name)
}

// wafCount is 1 when chain ends with a WAF middleware.
func wafCount(chain []string, wafs map[string]middleware) int {
	if n := len(chain); n > 0 {
		if _, ok := wafs[chain[n-1]]; ok {
			return 1
		}
	}
	return 0
}

// byPathSpecificity orders WAF paths so a more specific one gets the higher priority: prefixes by length, then exact
// paths.
func byPathSpecificity(paths []services.WAFPath) []services.WAFPath {
	out := slices.Clone(paths)
	slices.SortStableFunc(out, func(a, b services.WAFPath) int {
		if (a.Match == services.MatchExact) != (b.Match == services.MatchExact) {
			if a.Match == services.MatchExact {
				return 1
			}
			return -1
		}
		if len(a.Path) != len(b.Path) {
			return len(a.Path) - len(b.Path)
		}
		return strings.Compare(a.Path, b.Path)
	})
	return out
}

func wafPathRule(p services.WAFPath) string {
	if p.Match == services.MatchExact {
		return "Path(`" + p.Path + "`)"
	}
	return "PathPrefix(`" + p.Path + "`)"
}
