package traefik

import (
	"bytes"
	"net/netip"
	"os"
	"testing"
)

// The captured general log has 40 Coraza lines for 10 refused requests; only the "Access denied" line of each counts.
// The capture's middlewares were named by hand; Hashi's start with WAFMiddlewarePrefix.
func TestParseWAFBlocksFromCapture(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/contracts/coraza/traefik_log_matches.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.ReplaceAll(raw, []byte(`"middlewareName":"waf-`), []byte(`"middlewareName":"`+WAFMiddlewarePrefix))
	blocks := ParseWAFBlocks(raw)
	if len(blocks) != 10 {
		t.Fatalf("%d blocks, want 10", len(blocks))
	}
	b := blocks[0]
	if b.Client != netip.MustParseAddr("192.0.2.10") || b.Rule != 949110 || b.Router != "full@file" ||
		b.URI != "/full/hello" || b.UniqueID != "HVbdFqZPCaqWTYrnVih" || b.At.IsZero() {
		t.Errorf("first block %+v", b)
	}
	// Twice the same lines (a re-read after a truncation) still count once per request within one read.
	if got := ParseWAFBlocks(append(append([]byte{}, raw...), raw...)); len(got) != 10 {
		t.Errorf("duplicated lines: %d blocks", len(got))
	}
	// A middleware that is not Hashi's is someone else's WAF.
	if got := ParseWAFBlocks(bytes.ReplaceAll(raw, []byte(WAFMiddlewarePrefix), []byte("other-"))); len(got) != 0 {
		t.Errorf("foreign middleware counted: %d", len(got))
	}
}

func TestRouterService(t *testing.T) {
	slugs := []string{"files", "my-r-app", "my"}
	for router, want := range map[string]string{
		"svc-files@file":                  "files",
		"svc-files-r-01jr000000000@file":  "files",
		"svc-files-w-01jw000000000@file":  "files",
		"svc-my-r-app@file":               "my-r-app",
		"svc-my-r-app-w-01jw0000000@file": "my-r-app",
		"svc-my-r-01jr0000000000000@file": "my",
		"hashi-health-open@file":          "",
		"svc-filesystem@file":             "",
	} {
		if got, _ := RouterService(router, slugs); got != want {
			t.Errorf("%s → %q, want %q", router, got, want)
		}
	}
}
