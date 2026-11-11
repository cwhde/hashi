package usermw

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAccepts(t *testing.T) {
	f, err := Parse(`# office and uploads
http:
  middlewares:
    office-only:
      ipAllowList:
        sourceRange: [192.0.2.0/24]
    big-uploads: &b
      buffering:
        maxRequestBodyBytes: 2147483648
    cors:
      headers:
        accessControlAllowOriginList: ["https://app.example.test"]
        customResponseHeaders:
          X-Frame-Options: ""
`)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.Names(), ","); got != "big-uploads,cors,office-only" {
		t.Fatalf("names %s", got)
	}
	if f.Middlewares[0].Type != "buffering" || f.Middlewares[0].Line != 7 {
		t.Errorf("first middleware %+v", f.Middlewares[0])
	}
	if !f.Has("cors") || f.Has("nope") {
		t.Error("Has")
	}
	b := f.Content["big-uploads"].(map[string]any)["buffering"].(map[string]any)
	if n, ok := b["maxRequestBodyBytes"]; !ok || n == nil {
		t.Errorf("content lost a value: %#v", b)
	}
}

func TestParseEmpty(t *testing.T) {
	for _, text := range []string{"", "   \n", "# only a comment\n", "http:\n", "http:\n  middlewares:\n", "http:\n  middlewares: {}\n"} {
		f, err := Parse(text)
		if err != nil {
			t.Errorf("%q: %v", text, err)
		}
		if len(f.Middlewares) != 0 || f.Content != nil {
			t.Errorf("%q defines %v", text, f.Names())
		}
	}
}

// The captured user file Traefik loaded parses; the captured broken ones do not (testdata/contracts/traefik/traffic).
func TestParseCapturedFiles(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "contracts", "traefik", "traffic")
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	if _, err := Parse(read("user_middlewares_capture.yml")); err != nil {
		t.Errorf("the file Traefik loaded: %v", err)
	}
	if _, err := Parse(read("variant_ipwhitelist.yml")); err != nil {
		t.Errorf("ipWhiteList, which Traefik v3.7.16 loads: %v", err)
	}
	for _, name := range []string{"variant_two_types.yml", "variant_basicauth_unknown_key.yml"} {
		if _, err := Parse(read(name)); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

func TestParseRefuses(t *testing.T) {
	cases := []struct {
		name, text, want string
		line             int
	}{
		{"not yaml", "http:\n  middlewares:\n    a: [\n", "", 0},
		{"repeated key", "http:\n  middlewares:\n    a:\n      compress: {}\n    a:\n      compress: {}\n", "already defined", 5},
		{"two documents", "http: {}\n---\nhttp: {}\n", "one YAML document", 2},
		{"routers", "http:\n  routers:\n    r: {}\n", "only middlewares", 2},
		{"tcp", "tcp:\n  routers: {}\n", "only http", 1},
		{"a list", "- http\n", "starts with http", 1},
		{"middlewares list", "http:\n  middlewares: [a]\n", "list of names", 2},
		{"unknown type", "http:\n  middlewares:\n    a:\n      forwardAuht:\n        address: http://x\n", "not a Traefik middleware type", 4},
		{"two types", "http:\n  middlewares:\n    a:\n      compress: {}\n      buffering: {}\n", "exactly one type", 3},
		{"plugin", "http:\n  middlewares:\n    a:\n      plugin:\n        x: {}\n", "no Traefik plugins", 4},
		{"unknown field", "http:\n  middlewares:\n    a:\n      buffering:\n        maxRequestBodyByte: 1\n", "maxRequestBodyByte", 5},
		{"open schema object", "http:\n  middlewares:\n    a:\n      basicAuth:\n        users: [\"u:x\"]\n        notAField: true\n", "notAField", 6},
		{"wrong value type", "http:\n  middlewares:\n    a:\n      ipAllowList:\n        sourceRange: 7\n", "sourceRange", 5},
		{"reserved hashi", "http:\n  middlewares:\n    hashi-headers:\n      compress: {}\n", "Hashi's own", 3},
		{"reserved svc", "http:\n  middlewares:\n    svc-photos:\n      compress: {}\n", "Hashi's own", 3},
		{"upper case", "http:\n  middlewares:\n    Office:\n      compress: {}\n", "lowercase", 3},
		{"provider suffix", "http:\n  middlewares:\n    a@file:\n      compress: {}\n", "lowercase", 3},
		{"hashi header", "http:\n  middlewares:\n    a:\n      headers:\n        customRequestHeaders:\n          X-Hashi-User: admin\n", "X-Hashi-", 6},
		{"too large", strings.Repeat("#", MaxSize+1), "larger", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := Parse(c.text)
			var e *Error
			if !errors.As(err, &e) {
				t.Fatalf("got %v, want an *Error", err)
			}
			if !strings.Contains(e.Msg, c.want) {
				t.Errorf("message %q does not name %q", e.Msg, c.want)
			}
			if c.line != 0 && e.Line != c.line {
				t.Errorf("line %d, want %d (%s)", e.Line, c.line, e.Msg)
			}
		})
	}
}

// A hashi-named header on the response side is the admin's business: only request headers reach the upstream.
func TestParseAllowsHashiResponseHeader(t *testing.T) {
	if _, err := Parse("http:\n  middlewares:\n    a:\n      headers:\n        customResponseHeaders:\n          X-Hashi-Seen: \"1\"\n"); err != nil {
		t.Fatal(err)
	}
}

// The embedded schema is the pinned contract copy.
func TestEmbeddedSchemaIsTheContract(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "contracts", "traefik", "schema_v3_file_provider.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, schemaJSON) {
		t.Fatal("internal/usermw/schema_v3_file_provider.json differs from testdata/contracts/traefik/: copy it again")
	}
}

func TestCheckName(t *testing.T) {
	for _, n := range []string{"a", "office-only", "x1", strings.Repeat("a", 63)} {
		if err := CheckName(n); err != nil {
			t.Errorf("%s: %v", n, err)
		}
	}
	for _, n := range []string{"", "-a", "a-", "A", "a_b", strings.Repeat("a", 64), "hashi", "hashicorp", "svc-x"} {
		if CheckName(n) == nil {
			t.Errorf("%q accepted", n)
		}
	}
}
