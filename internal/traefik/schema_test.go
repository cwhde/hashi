package traefik

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"go.yaml.in/yaml/v3"

	"git.juzo.io/juzo/hashi/internal/secrets"
)

// 04 §9: every rendered dynamic file, parsed back, validates against the pinned Traefik v3 file-provider schema
// (testdata/contracts/traefik/schema_v3_file_provider.json, from SchemaStore).
func TestDynamicFilesMatchTraefikSchema(t *testing.T) {
	f, err := os.Open(filepath.Join("..", "..", "testdata", "contracts", "traefik", "schema_v3_file_provider.json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	doc, err := jsonschema.UnmarshalJSON(f)
	if err != nil {
		t.Fatal(err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("traefik-v3-file.json", doc); err != nil {
		t.Fatal(err)
	}
	schema, err := c.Compile("traefik-v3-file.json")
	if err != nil {
		t.Fatal(err)
	}
	o := testOptions()
	o.TrustedIPs = []string{"192.0.2.1"}
	o.Services = testServices()
	o.EdgeToken = secrets.New("edge-token-for-tests")
	files, err := Render(o)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, file := range files {
		if file.Kind != KindDynamic {
			continue
		}
		var v any
		if err := yaml.Unmarshal(file.Data, &v); err != nil {
			t.Fatalf("%s: %v", file.Path, err)
		}
		// YAML maps decode as map[string]any; a JSON round trip gives the validator plain JSON values.
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(inst); err != nil {
			t.Errorf("%s does not match the schema: %v", file.Path, err)
		}
		n++
	}
	if n != 6 {
		t.Errorf("validated %d dynamic files, want 6", n)
	}
	// The schema is strict enough to catch a misspelt key.
	bad, _ := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(`{"http":{"middlewares":{"x":{"forwardAuht":{"address":"http://a"}}}}}`)))
	if schema.Validate(bad) == nil {
		t.Error("schema accepted an unknown middleware type")
	}
}
