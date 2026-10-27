package secrets

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

const canary = "s3cr3t-canary-value"

func TestInvariant_5_ValueNeverRenders(t *testing.T) {
	v := New(canary)
	outputs := []string{
		v.String(),
		fmt.Sprintf("%v %+v %#v %s %q %x", v, v, v, v, v, v),
		fmt.Sprint(struct{ T Value }{v}),
	}
	j, err := json.Marshal(map[string]any{"token": v})
	if err != nil {
		t.Fatal(err)
	}
	outputs = append(outputs, string(j))
	for _, o := range outputs {
		if strings.Contains(o, canary) {
			t.Fatalf("secret leaked: %q", o)
		}
	}
	if v.Reveal() != canary {
		t.Fatal("Reveal must return the secret")
	}
}

func TestInvariant_5_LogsRedactSecrets(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(NewRedactingHandler(slog.NewJSONHandler(&buf, nil)))

	log.Info("typed value under an allowed key", "summary", New(canary))
	log.Info("unknown keys", "api_token", canary, slog.Group("ssh", slog.String("private_key", canary)))
	log.With("Authorization", canary).Info("via With")

	out := buf.String()
	if strings.Contains(out, canary) {
		t.Fatalf("secret leaked into log:\n%s", out)
	}
	if strings.Count(out, redacted) != 4 {
		t.Fatalf("expected 4 redactions, got:\n%s", out)
	}
}

func TestRedactKeepsAllowListedAttrs(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(NewRedactingHandler(slog.NewJSONHandler(&buf, nil)))
	log.Info("job", "kind", "noop", "key_id", "k1", "duration_ms", 12)
	for _, want := range []string{`"kind":"noop"`, `"key_id":"k1"`, `"duration_ms":12`} {
		if !strings.Contains(buf.String(), want) {
			t.Fatalf("missing %s in %s", want, buf.String())
		}
	}
}
