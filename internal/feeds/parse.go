// Package feeds turns external blocklists into IP sets of kind feed (06 §8): an SSRF-safe download, parsers for the
// formats in testdata/contracts/feeds/, a filter that never takes private or reserved ranges, and the refresh that
// replaces a set's entries only when a whole download parsed.
package feeds

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strconv"
	"strings"
)

// Formats (06 §8).
const (
	FormatLines      = "lines"       // one address or CIDR per line, # and ; comments (FireHOL .netset/.ipset, Spamhaus .txt)
	FormatCSV        = "csv"         // a separated column, optional prefix-length column (DShield)
	FormatJSONArray  = "json_array"  // ["192.0.2.1", …]
	FormatJSONObject = "json_object" // {"<field>": ["…"]}
	FormatJSONLines  = "json_lines"  // one object per line with the address in <field>
)

// Options configure the column and field formats. Columns count from 1.
type Options struct {
	Separator    string `json:"separator,omitempty"` // "tab" (default), "comma", "semicolon", "space"
	Column       int    `json:"column,omitempty"`
	PrefixColumn int    `json:"prefix_column,omitempty"`
	Field        string `json:"field,omitempty"`
}

// Reject is a line Hashi did not take, with its line number (1-based; 0 for JSON arrays and objects: the index+1).
type Reject struct {
	Line   int    `json:"line"`
	Text   string `json:"text"`
	Reason string `json:"reason"`
}

// Parsed is a whole download, before the reserved-range filter.
type Parsed struct {
	Prefixes []netip.Prefix // normalized (masked, IPv4-mapped unmapped), unique, sorted
	Rejects  []Reject       // the first MaxRejectsKept
	Rejected int            // all of them
	Ignored  int            // comment, blank and metadata lines
}

// MaxRejectsKept bounds the rejects kept for the preview and the run record.
const MaxRejectsKept = 200

// ErrFormat means the download is not in the configured format at all.
var ErrFormat = errors.New("the download is not in the configured format")

// Validate checks a format's options.
func Validate(format string, o Options) error {
	switch format {
	case FormatLines, FormatJSONArray:
		return nil
	case FormatCSV:
		if o.Column < 1 || o.PrefixColumn < 0 || o.PrefixColumn == o.Column {
			return errors.New("a CSV feed needs the address column (from 1) and, if any, another prefix-length column")
		}
		if _, ok := separators[o.Separator]; !ok && o.Separator != "" {
			return errors.New("the separator is tab, comma, semicolon or space")
		}
		return nil
	case FormatJSONObject, FormatJSONLines:
		if o.Field == "" {
			return errors.New("a JSON feed needs the field that holds the addresses")
		}
		return nil
	}
	return fmt.Errorf("unknown feed format %q", format)
}

var separators = map[string]string{"tab": "\t", "comma": ",", "semicolon": ";", "space": " "}

// Parse reads body in format.
func Parse(format string, o Options, body []byte) (Parsed, error) {
	if err := Validate(format, o); err != nil {
		return Parsed{}, err
	}
	p := &parser{seen: map[netip.Prefix]bool{}}
	var err error
	switch format {
	case FormatLines:
		p.lines(body)
	case FormatCSV:
		p.csv(body, o)
	case FormatJSONArray:
		var a []json.RawMessage
		if err = json.Unmarshal(body, &a); err == nil {
			p.values(a)
		}
	case FormatJSONObject:
		var m map[string]json.RawMessage
		if err = json.Unmarshal(body, &m); err == nil {
			var a []json.RawMessage
			if err = json.Unmarshal(m[o.Field], &a); err == nil {
				p.values(a)
			}
		}
	case FormatJSONLines:
		err = p.jsonLines(body, o.Field)
	}
	if err != nil {
		return Parsed{}, fmt.Errorf("%w: %w", ErrFormat, err)
	}
	slices.SortFunc(p.out.Prefixes, comparePrefix)
	return p.out, nil
}

func comparePrefix(a, b netip.Prefix) int {
	if c := a.Addr().Compare(b.Addr()); c != 0 {
		return c
	}
	return a.Bits() - b.Bits()
}

type parser struct {
	out  Parsed
	seen map[netip.Prefix]bool
}

func (p *parser) reject(line int, text, reason string) {
	p.out.Rejected++
	if len(p.out.Rejects) < MaxRejectsKept {
		if len(text) > 120 {
			text = text[:120]
		}
		p.out.Rejects = append(p.out.Rejects, Reject{Line: line, Text: text, Reason: reason})
	}
}

func (p *parser) add(line int, raw string, prefix netip.Prefix, err error) {
	if err != nil {
		p.reject(line, raw, "not an IP address or CIDR")
		return
	}
	if !p.seen[prefix] {
		p.seen[prefix] = true
		p.out.Prefixes = append(p.out.Prefixes, prefix)
	}
}

// ParsePrefix reads an address or CIDR, normalized: IPv4-mapped IPv6 becomes IPv4, host bits are cleared, zones are
// refused.
func ParsePrefix(s string) (netip.Prefix, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		if err != nil || p.Addr().Zone() != "" {
			return netip.Prefix{}, errors.New("bad prefix")
		}
		a, bits := p.Addr(), p.Bits()
		if a.Is4In6() {
			if bits < 96 {
				return netip.Prefix{}, errors.New("bad prefix")
			}
			a, bits = a.Unmap(), bits-96
		}
		return netip.PrefixFrom(a, bits).Masked(), nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil || a.Zone() != "" {
		return netip.Prefix{}, errors.New("bad address")
	}
	a = a.Unmap()
	return netip.PrefixFrom(a, a.BitLen()), nil
}

func scan(body []byte, f func(n int, line string)) {
	sc := bufio.NewScanner(bytes.NewReader(body))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for n := 1; sc.Scan(); n++ {
		f(n, sc.Text())
	}
}

// lines: "#" and ";" start a comment anywhere on the line (Spamhaus: "192.0.2.0/24 ; SBL1").
func (p *parser) lines(body []byte) {
	scan(body, func(n int, line string) {
		if i := strings.IndexAny(line, "#;"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			p.out.Ignored++
			return
		}
		pr, err := ParsePrefix(line)
		p.add(n, line, pr, err)
	})
}

func (p *parser) csv(body []byte, o Options) {
	sep := separators[o.Separator]
	if sep == "" {
		sep = "\t"
	}
	scan(body, func(n int, line string) {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, ";") {
			p.out.Ignored++
			return
		}
		var cols []string
		if sep == " " {
			cols = strings.Fields(line)
		} else {
			cols = strings.Split(line, sep)
		}
		if len(cols) < max(o.Column, o.PrefixColumn) {
			p.reject(n, line, "too few columns")
			return
		}
		addr := strings.Trim(strings.TrimSpace(cols[o.Column-1]), `"`)
		if o.PrefixColumn > 0 {
			bits, err := strconv.Atoi(strings.Trim(strings.TrimSpace(cols[o.PrefixColumn-1]), `"`))
			if err != nil {
				// A header row ("Start", "Prefix", …) is not an entry.
				if _, aerr := netip.ParseAddr(addr); aerr != nil {
					p.out.Ignored++
					return
				}
				p.reject(n, line, "the prefix length is not a number")
				return
			}
			addr += "/" + strconv.Itoa(bits)
		}
		if _, err := ParsePrefix(addr); err != nil && n == 1 {
			p.out.Ignored++ // header row
			return
		}
		pr, err := ParsePrefix(addr)
		p.add(n, addr, pr, err)
	})
}

func (p *parser) values(a []json.RawMessage) {
	for i, raw := range a {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			p.reject(i+1, string(raw), "not a string")
			continue
		}
		pr, err := ParsePrefix(s)
		p.add(i+1, s, pr, err)
	}
}

// jsonLines: one object per line; lines without the field (Spamhaus's closing metadata line) are ignored.
func (p *parser) jsonLines(body []byte, field string) error {
	objects := 0
	scan(body, func(n int, line string) {
		if strings.TrimSpace(line) == "" {
			p.out.Ignored++
			return
		}
		var m map[string]json.RawMessage
		if json.Unmarshal([]byte(line), &m) != nil {
			p.reject(n, line, "not a JSON object")
			return
		}
		objects++
		raw, ok := m[field]
		if !ok {
			p.out.Ignored++
			return
		}
		var s string
		if json.Unmarshal(raw, &s) != nil {
			p.reject(n, line, "the field is not a string")
			return
		}
		pr, err := ParsePrefix(s)
		p.add(n, s, pr, err)
	})
	if objects == 0 && len(p.out.Rejects) > 0 {
		return errors.New("no line is a JSON object")
	}
	return nil
}
