package httpapi

import (
	"os"
	"slices"
	"testing"

	"go.yaml.in/yaml/v3"

	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
)

// 01-platform.md §7: an endpoint without a declared scope fails CI.
func TestEveryOperationDeclaresScopes(t *testing.T) {
	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	methods := map[string]bool{"get": true, "put": true, "post": true, "delete": true, "patch": true, "head": true, "options": true}
	n := 0
	for path, ops := range doc.Paths {
		for m, op := range ops {
			if !methods[m] {
				continue
			}
			n++
			scopes, ok := op["x-hashi-scopes"].([]any)
			if !ok {
				t.Errorf("%s %s: missing x-hashi-scopes list", m, path)
				continue
			}
			for _, s := range scopes {
				if _, ok := s.(string); !ok {
					t.Errorf("%s %s: scope %v is not a string", m, path, s)
				}
			}
		}
	}
	if n == 0 {
		t.Fatal("no operations found; is the path right?")
	}
}

// 01 §5.1 step 2: approving a plan needs sync.apply and a fresh passkey; every plan action needs sync.apply.
func TestPlanApprovalNeedsReauth(t *testing.T) {
	if r := gen.Requirements["ApproveSyncPlan"]; !r.Reauth || !slices.Contains(r.Scopes, "sync.apply") {
		t.Errorf("approve: %+v", r)
	}
	for _, op := range []string{"ApplySyncPlan", "DiscardSyncPlan"} {
		if r := gen.Requirements[op]; !slices.Contains(r.Scopes, "sync.apply") {
			t.Errorf("%s: %+v", op, r)
		}
	}
}
