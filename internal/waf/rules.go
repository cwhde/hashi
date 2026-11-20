package waf

import (
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"

	coreruleset "github.com/corazawaf/coraza-coreruleset/v4"
)

// Rule is one CRS request rule as the exclusions picker shows it (bug 18): its number, its own message as the name,
// the rule file's attack group and its paranoia level.
type Rule struct {
	ID       int
	Name     string
	Group    string
	Paranoia int
}

// groups names the CRS request rule files by their number (REQUEST-<n>-…conf). Response rules never run: forward
// auth sees requests only. Files without a group here (initialisation, exceptions, scoring) carry no attack rules.
var groups = map[string]string{
	"911": "Method enforcement",
	"913": "Scanner detection",
	"920": "Protocol enforcement",
	"921": "Protocol attack",
	"922": "Multipart attack",
	"930": "Local file inclusion",
	"931": "Remote file inclusion",
	"932": "Remote command execution",
	"933": "PHP injection",
	"934": "Generic attack",
	"941": "Cross-site scripting",
	"942": "SQL injection",
	"943": "Session fixation",
	"944": "Java attack",
}

var (
	ruleID    = regexp.MustCompile(`"id:(\d+)`)
	ruleMsg   = regexp.MustCompile(`msg:'((?:[^'\\]|\\.)*)'`)
	ruleLevel = regexp.MustCompile(`tag:'paranoia-level/(\d)'`)
	rulesOnce sync.Once
	rulesAll  []Rule
)

// Rules lists every CRS request rule that has a message, read once from the rule files Coraza runs, so the names
// always match the rules in use and nothing is kept by hand.
func Rules() []Rule {
	rulesOnce.Do(func() { rulesAll = parseRules(coreruleset.FS) })
	return rulesAll
}

func parseRules(fsys fs.FS) []Rule {
	files, _ := fs.Glob(fsys, "@owasp_crs/REQUEST-*.conf")
	var out []Rule
	for _, f := range files {
		group := groups[strings.SplitN(strings.TrimPrefix(path.Base(f), "REQUEST-"), "-", 2)[0]]
		if group == "" {
			continue
		}
		data, err := fs.ReadFile(fsys, f)
		if err != nil {
			continue
		}
		out = append(out, parseFile(string(data), group)...)
	}
	slices.SortFunc(out, func(a, b Rule) int { return a.ID - b.ID })
	return out
}

// parseFile reads the SecRule directives of one file. A directive continues over lines ending in a backslash; a
// chained rule's later parts have no id and are skipped, and a rule without a message has nothing to show.
func parseFile(src, group string) []Rule {
	src = strings.ReplaceAll(src, "\\\n", " ")
	var out []Rule
	for _, line := range strings.Split(src, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "SecRule ") {
			continue
		}
		id, msg := ruleID.FindStringSubmatch(line), ruleMsg.FindStringSubmatch(line)
		if id == nil || msg == nil {
			continue
		}
		n, _ := strconv.Atoi(id[1])
		r := Rule{ID: n, Name: strings.ReplaceAll(msg[1], `\'`, "'"), Group: group, Paranoia: 1}
		if l := ruleLevel.FindStringSubmatch(line); l != nil {
			r.Paranoia, _ = strconv.Atoi(l[1])
		}
		out = append(out, r)
	}
	return out
}
