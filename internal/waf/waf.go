// Package waf checks requests with Coraza and the OWASP CRS inside Hashi's forward auth (06 §9). Forward auth sees
// the method, the URI and every header but never the body, which is exactly the headers-only mode. One compiled WAF
// per distinct configuration serves every service: the Traefik plugin built one per route and needed 2.5 GB and 16 s
// for 60 routes (measured 2026-11-03), which no edge container can carry.
package waf

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"

	coreruleset "github.com/corazawaf/coraza-coreruleset/v4"
	"github.com/corazawaf/coraza/v3"
)

// Config is one effective WAF: what a request's service and path resolve to (services.EffectiveWAF without the mode).
type Config struct {
	Exclusions []int
}

// CRS request policy for self-hosted apps: every method sync clients and APIs use (WebDAV included), any content
// type (git, sync clients and APIs send their own) and the headers they send. CRS would refuse these from the headers alone (rules 911100,
// 920420) and break Nextcloud, WebDAV and git over HTTPS; the attack rules (injection, XSS, traversal, …) stay.
const (
	methods = "GET HEAD POST OPTIONS PUT PATCH DELETE PROPFIND PROPPATCH MKCOL COPY MOVE LOCK UNLOCK REPORT SEARCH"
	// restrictedHeaders is CRS's basic list (rule 920450) without the headers sync clients, WebDAV and git send:
	// Content-Encoding (git compresses fetch bodies; the rule guards body inspection, which forward auth never does),
	// Lock-Token and If (WebDAV locks), Expect (large git pushes, curl uploads), Content-Range (resumed uploads). The
	// ones that are only ever attacks stay: Proxy (httpoxy), the method overrides, X-Middleware-Subrequest.
	restrictedHeaders = "/proxy/ /x-http-method-override/ /x-http-method/ /x-method-override/ /x-middleware-subrequest/"
	ruleContentType   = 920420
	// ruleNoLength flags a POST without Content-Length: forward auth never receives that header (captured,
	// testdata/contracts/traefik/forward_auth_request.txt), so every POST would score.
	ruleNoLength = 920180
	// ruleRestrictedFile flags ".git/" and similar in the path; git over HTTPS (Gitea, Forgejo) uses exactly such paths.
	ruleRestrictedFile = 930130
)

// Directives is the Coraza configuration for c: recommended base, engine on, no body access (forward auth has no body),
// CRS setup, the request policy, the CRS rules, then removals (they must follow the rules they remove).
func Directives(c Config) []string {
	d := []string{
		"Include @coraza.conf-recommended",
		"SecRuleEngine On",
		"SecRequestBodyAccess Off",
		"SecResponseBodyAccess Off",
		"Include @crs-setup.conf.example",
		`SecAction "id:900200,phase:1,pass,t:none,nolog,setvar:'tx.allowed_methods=` + methods + `'"`,
		`SecAction "id:900250,phase:1,pass,t:none,nolog,setvar:'tx.restricted_headers_basic=` + restrictedHeaders + `'"`,
		"Include @owasp_crs/*.conf",
	}
	remove := append(slices.Clone(c.Exclusions), ruleContentType, ruleNoLength)
	slices.Sort(remove)
	for _, id := range slices.Compact(remove) {
		d = append(d, "SecRuleRemoveById "+strconv.Itoa(id))
	}
	return d
}

func key(c Config) string {
	s := sha256.Sum256([]byte(strings.Join(Directives(c), "\n")))
	return hex.EncodeToString(s[:8])
}

// Request is what forward auth knows of the original request.
type Request struct {
	Client netip.Addr
	Method string
	URI    string // path and query
	Proto  string
	Host   string
	Header map[string][]string
}

// Verdict is the WAF's answer. RuleID is the deciding rule (949110 for the anomaly score) when Blocked.
type Verdict struct {
	Blocked bool
	RuleID  int
	Status  int
}

// Engine keeps one compiled WAF per configuration, built on first use.
type Engine struct {
	mu   sync.Mutex
	wafs map[string]coraza.WAF
}

// New returns an empty engine.
func New() *Engine { return &Engine{wafs: map[string]coraza.WAF{}} }

func (e *Engine) get(c Config) (coraza.WAF, error) {
	k := key(c)
	e.mu.Lock()
	defer e.mu.Unlock()
	if w, ok := e.wafs[k]; ok {
		return w, nil
	}
	w, err := coraza.NewWAF(coraza.NewWAFConfig().WithRootFS(coreruleset.FS).WithDirectives(strings.Join(Directives(c), "\n")))
	if err != nil {
		return nil, fmt.Errorf("build WAF: %w", err)
	}
	e.wafs[k] = w
	return w, nil
}

// Warm builds the WAF for c ahead of the first request (startup), so no visitor waits for the rule compilation.
func (e *Engine) Warm(c Config) error {
	_, err := e.get(c)
	return err
}

// gitSmartHTTP reports whether uri is git's smart HTTP protocol or git LFS on a repository (".git/" in the path), the
// one legitimate use of a restricted-file path; probes for a website's .git folder look different (/.git/config, …).
func gitSmartHTTP(uri string) bool {
	path, query, _ := strings.Cut(uri, "?")
	if !strings.Contains(path, ".git/") || strings.Contains(path, "/.git/") {
		return false
	}
	return strings.HasSuffix(path, "/git-upload-pack") || strings.HasSuffix(path, "/git-receive-pack") ||
		(strings.HasSuffix(path, "/info/refs") && strings.HasPrefix(query, "service=git-")) ||
		strings.Contains(path, "/info/lfs/")
}

// Check runs phase 1 and 2 (headers only) for r.
func (e *Engine) Check(c Config, r Request) (Verdict, error) {
	if gitSmartHTTP(r.URI) {
		c.Exclusions = append(slices.Clone(c.Exclusions), ruleRestrictedFile)
	}
	w, err := e.get(c)
	if err != nil {
		return Verdict{}, err
	}
	tx := w.NewTransaction()
	defer func() {
		tx.ProcessLogging()
		_ = tx.Close()
	}()
	if r.Client.IsValid() {
		tx.ProcessConnection(r.Client.String(), 0, "", 443)
	}
	proto := r.Proto
	if proto == "" {
		proto = "HTTP/1.1"
	}
	tx.ProcessURI(r.URI, r.Method, proto)
	if r.Host != "" {
		tx.AddRequestHeader("Host", r.Host)
		tx.SetServerName(r.Host)
	}
	for k, vs := range r.Header {
		for _, v := range vs {
			tx.AddRequestHeader(k, v)
		}
	}
	if it := tx.ProcessRequestHeaders(); it != nil {
		return Verdict{Blocked: true, RuleID: it.RuleID, Status: it.Status}, nil
	}
	// Phase 2 evaluates the anomaly score (949110) even without a body.
	it, err := tx.ProcessRequestBody()
	if err != nil {
		return Verdict{}, err
	}
	if it != nil {
		return Verdict{Blocked: true, RuleID: it.RuleID, Status: it.Status}, nil
	}
	return Verdict{}, nil
}
