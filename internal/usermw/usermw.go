// Package usermw is the user middleware file, 30-user-middlewares.yml (04 §4): the YAML the admin writes in the
// editor, checked strictly before anything reaches a Traefik host. Traefik v3.7.16 loads none of its files when one of
// them has an unknown key, and silently skips a middleware name another file already defines
// (testdata/contracts/traefik/traffic/), so a file that fails here is never stored.
package usermw

import (
	"bytes"
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"go.yaml.in/yaml/v3"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"git.juzo.io/juzo/hashi/internal/store/db"
)

// FileName is the dynamic file the user middlewares are written to.
const FileName = "30-user-middlewares.yml"

// MaxSize bounds the stored text.
const MaxSize = 64 << 10

// Middleware is one middleware the file defines.
type Middleware struct {
	Name string
	// Type is the Traefik middleware type, e.g. "headers" or "ipAllowList".
	Type string
	Line int
}

// File is a parsed user file.
type File struct {
	Middlewares []Middleware // sorted by name
	// Content is http.middlewares marshalled back (anchors resolved, comments dropped); nil when the file defines none.
	Content map[string]any
}

// Has reports whether the file defines name.
func (f File) Has(name string) bool {
	return slices.ContainsFunc(f.Middlewares, func(m Middleware) bool { return m.Name == name })
}

// Names lists the defined names, sorted.
func (f File) Names() []string {
	out := make([]string, 0, len(f.Middlewares))
	for _, m := range f.Middlewares {
		out = append(out, m.Name)
	}
	return out
}

// Error is a problem in the file; Line is 1-based, 0 when the problem has no single place.
type Error struct {
	Line int
	Msg  string
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return "line " + strconv.Itoa(e.Line) + ": " + e.Msg
	}
	return e.Msg
}

func fail(line int, format string, a ...any) error {
	return &Error{Line: line, Msg: fmt.Sprintf(format, a...)}
}

var nameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// CheckName refuses names that are not lowercase letters, digits and '-' or that Hashi uses itself: every object Hashi
// renders is named hashi-… or svc-… (a second definition of a name is skipped by Traefik without an error).
func CheckName(n string) error {
	if !nameRe.MatchString(n) {
		return fmt.Errorf("%q: a middleware name has lowercase letters, digits and '-' (at most 63, no '-' at either end)", n)
	}
	if strings.HasPrefix(n, "hashi") || strings.HasPrefix(n, "svc-") {
		return fmt.Errorf("%q: names starting with hashi or svc- are Hashi's own", n)
	}
	return nil
}

// Parse checks text strictly (04 §4) and returns what it defines. Empty text or comments only define nothing.
func Parse(text string) (File, error) {
	if len(text) > MaxSize {
		return File{}, fail(0, "the file is larger than %d KB", MaxSize>>10)
	}
	dec := yaml.NewDecoder(strings.NewReader(text))
	var doc yaml.Node
	if err := dec.Decode(&doc); errors.Is(err, io.EOF) {
		return File{}, nil
	} else if err != nil {
		return File{}, yamlError(err)
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err == nil {
		return File{}, fail(extra.Line, "only one YAML document is allowed (remove the ---)")
	} else if !errors.Is(err, io.EOF) {
		return File{}, yamlError(err)
	}
	if len(doc.Content) == 0 || (doc.Content[0].Kind == yaml.ScalarNode && doc.Content[0].Tag == "!!null") {
		return File{}, nil
	}
	// Decoding into a map is what reports a repeated key.
	var v any
	if err := doc.Decode(&v); err != nil {
		return File{}, yamlError(err)
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return File{}, fail(root.Line, "the file starts with http: and middlewares: under it")
	}
	httpNode, err := only(root, "http", "the top level holds only http:")
	if err != nil {
		return File{}, err
	}
	if httpNode == nil {
		return File{}, nil
	}
	mwsNode, err := only(httpNode, "middlewares", "under http: only middlewares: is allowed (routers and services are Hashi's)")
	if err != nil {
		return File{}, err
	}
	if mwsNode == nil {
		return File{}, nil
	}
	s, err := schemas()
	if err != nil {
		return File{}, err
	}
	var out File
	content := map[string]any{}
	for i := 0; i+1 < len(mwsNode.Content); i += 2 {
		k, body := mwsNode.Content[i], mwsNode.Content[i+1]
		name := k.Value
		if err := CheckName(name); err != nil {
			return File{}, fail(k.Line, "%s", err.Error())
		}
		if body.Kind != yaml.MappingNode || len(body.Content) != 2 {
			return File{}, fail(k.Line, "%s: a middleware has exactly one type, e.g. headers: or ipAllowList:", name)
		}
		typ := body.Content[0].Value
		def, ok := s.types[typ]
		if !ok {
			return File{}, fail(body.Content[0].Line, "%s: %q is not a Traefik middleware type (%s)", name, typ, strings.Join(s.names, ", "))
		}
		if typ == "plugin" {
			return File{}, fail(body.Content[0].Line, "%s: Hashi installs no Traefik plugins", name)
		}
		var val any
		if err := body.Content[1].Decode(&val); err != nil {
			return File{}, yamlError(err)
		}
		inst, err := jsonValue(val)
		if err != nil {
			return File{}, fail(body.Content[1].Line, "%s: %v", name, err)
		}
		if err := def.Validate(inst); err != nil {
			return File{}, schemaError(name, typ, body.Content[1], err)
		}
		if typ == "headers" {
			if err := noHashiHeaders(name, body.Content[1]); err != nil {
				return File{}, err
			}
		}
		content[name] = map[string]any{typ: val} // the YAML values: numbers stay numbers when marshalled back
		out.Middlewares = append(out.Middlewares, Middleware{Name: name, Type: typ, Line: k.Line})
	}
	sort.Slice(out.Middlewares, func(i, j int) bool { return out.Middlewares[i].Name < out.Middlewares[j].Name })
	if len(content) > 0 {
		out.Content = content
	}
	return out, nil
}

// only returns the value of the one allowed key of a mapping; nil when the mapping is empty or the value is null.
func only(n *yaml.Node, key, msg string) (*yaml.Node, error) {
	if n.Kind == yaml.ScalarNode && n.Tag == "!!null" {
		return nil, nil
	}
	if n.Kind != yaml.MappingNode {
		return nil, fail(n.Line, "%s", msg)
	}
	var val *yaml.Node
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value != key {
			return nil, fail(n.Content[i].Line, "%s, not %s:", msg, n.Content[i].Value)
		}
		val = n.Content[i+1]
	}
	if val != nil && val.Kind == yaml.ScalarNode && val.Tag == "!!null" {
		return nil, nil
	}
	if val != nil && val.Kind != yaml.MappingNode {
		return nil, fail(val.Line, "%s: holds a list of names, each with its middleware", key)
	}
	return val, nil
}

// noHashiHeaders refuses request headers forward auth owns: a user middleware runs after forward auth and could
// otherwise name a signed-in user to the upstream (06 §2).
func noHashiHeaders(name string, body *yaml.Node) error {
	for i := 0; i+1 < len(body.Content); i += 2 {
		if body.Content[i].Value != "customRequestHeaders" {
			continue
		}
		h := body.Content[i+1]
		for j := 0; j+1 < len(h.Content); j += 2 {
			if strings.HasPrefix(strings.ToLower(h.Content[j].Value), "x-hashi-") {
				return fail(h.Content[j].Line, "%s: X-Hashi-* request headers belong to Hashi's forward auth", name)
			}
		}
	}
	return nil
}

var printer = message.NewPrinter(language.English)

var lineRe = regexp.MustCompile(`^yaml: (?:line (\d+): )?(.*)$`)

func yamlError(err error) error {
	var te *yaml.TypeError
	if errors.As(err, &te) && len(te.Errors) > 0 {
		err = errors.New(te.Errors[0])
	}
	msg := err.Error()
	if m := lineRe.FindStringSubmatch(msg); m != nil {
		line, _ := strconv.Atoi(m[1])
		return fail(line, "%s", m[2])
	}
	if m := regexp.MustCompile(`line (\d+)`).FindStringSubmatch(msg); m != nil {
		line, _ := strconv.Atoi(m[1])
		return fail(line, "%s", strings.TrimPrefix(msg, "yaml: "))
	}
	return fail(0, "%s", strings.TrimPrefix(msg, "yaml: "))
}

// jsonValue turns a decoded YAML value into the plain JSON value the schema validator takes.
func jsonValue(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, errors.New("keys must be text")
	}
	return jsonschema.UnmarshalJSON(bytes.NewReader(b))
}

// schemaError names the deepest place the schema refused, with its line.
func schemaError(name, typ string, body *yaml.Node, err error) error {
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return fail(body.Line, "%s (%s): %v", name, typ, err)
	}
	leaf := ve
	for len(leaf.Causes) > 0 {
		next := leaf.Causes[0]
		for _, c := range leaf.Causes {
			if len(c.InstanceLocation) > len(next.InstanceLocation) {
				next = c
			}
		}
		leaf = next
	}
	path := strings.Join(leaf.InstanceLocation, ".")
	msg := leaf.ErrorKind.LocalizedString(printer)
	if path != "" {
		msg = path + ": " + msg
	}
	loc := leaf.InstanceLocation
	if ap, ok := leaf.ErrorKind.(*kind.AdditionalProperties); ok && len(ap.Properties) > 0 {
		loc = append(slices.Clone(loc), ap.Properties[0]) // point at the unknown key itself
	}
	return fail(lineOf(body, loc), "%s (%s): %s", name, typ, msg)
}

// lineOf follows a JSON pointer through the YAML nodes as far as it goes.
func lineOf(n *yaml.Node, path []string) int {
	line := n.Line
	for _, p := range path {
		switch n.Kind {
		case yaml.MappingNode:
			found := false
			for i := 0; i+1 < len(n.Content); i += 2 {
				if n.Content[i].Value == p {
					line, n, found = n.Content[i].Line, n.Content[i+1], true
					break
				}
			}
			if !found {
				return line
			}
		case yaml.SequenceNode:
			i, err := strconv.Atoi(p)
			if err != nil || i < 0 || i >= len(n.Content) {
				return line
			}
			n = n.Content[i]
			line = n.Line
		default:
			return line
		}
	}
	return line
}

//go:embed schema_v3_file_provider.json
var schemaJSON []byte

type compiled struct {
	types map[string]*jsonschema.Schema // middleware type → its definition
	names []string
}

// closeObjects marks every schema object that lists properties but says nothing about others as closed. The pinned
// schema leaves a few open (basicAuth among them), and Traefik refuses an unknown key by loading no file at all
// (testdata/contracts/traefik/traffic/ finding 9). The schema combines no sub-schemas (no allOf/anyOf), so closing
// each object on its own cannot refuse a key a sibling defines.
func closeObjects(n any) {
	switch v := n.(type) {
	case map[string]any:
		if _, ok := v["properties"]; ok {
			if _, set := v["additionalProperties"]; !set {
				v["additionalProperties"] = false
			}
		}
		for _, c := range v {
			closeObjects(c)
		}
	case []any:
		for _, c := range v {
			closeObjects(c)
		}
	}
}

var schemas = sync.OnceValues(func() (compiled, error) {
	var tree any
	if err := json.Unmarshal(schemaJSON, &tree); err != nil {
		return compiled{}, fmt.Errorf("traefik schema: %w", err)
	}
	closeObjects(tree)
	closed, err := json.Marshal(tree)
	if err != nil {
		return compiled{}, fmt.Errorf("traefik schema: %w", err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(closed))
	if err != nil {
		return compiled{}, fmt.Errorf("traefik schema: %w", err)
	}
	c := jsonschema.NewCompiler()
	const url = "traefik-v3-file.json"
	if err := c.AddResource(url, doc); err != nil {
		return compiled{}, fmt.Errorf("traefik schema: %w", err)
	}
	// The middleware types are the one-property alternatives of definitions.httpMiddleware.oneOf.
	var raw struct {
		Definitions struct {
			HTTPMiddleware struct {
				OneOf []struct {
					Properties map[string]struct {
						Ref string `json:"$ref"`
					} `json:"properties"`
				} `json:"oneOf"`
			} `json:"httpMiddleware"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(schemaJSON, &raw); err != nil {
		return compiled{}, fmt.Errorf("traefik schema: %w", err)
	}
	out := compiled{types: map[string]*jsonschema.Schema{}}
	for _, alt := range raw.Definitions.HTTPMiddleware.OneOf {
		for typ, p := range alt.Properties {
			s, err := c.Compile(url + p.Ref)
			if err != nil {
				return compiled{}, fmt.Errorf("traefik schema %s: %w", typ, err)
			}
			out.types[typ] = s
			if typ != "plugin" {
				out.names = append(out.names, typ)
			}
		}
	}
	if len(out.types) == 0 {
		return compiled{}, errors.New("traefik schema lists no middleware types")
	}
	sort.Strings(out.names)
	return out, nil
})

// Load parses the stored file; no row is an empty file. A stored file always parsed when it was saved, so an error
// here means the checks became stricter since.
func Load(ctx context.Context, q *db.Queries) (string, File, error) {
	row, err := q.GetUserMiddlewares(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return "", File{}, nil
	}
	if err != nil {
		return "", File{}, fmt.Errorf("read the user middlewares: %w", err)
	}
	f, err := Parse(row.Yaml)
	if err != nil {
		return row.Yaml, File{}, fmt.Errorf("the stored user middleware file: %w", err)
	}
	return row.Yaml, f, nil
}
