package httpapi

import (
	"context"
	"errors"
	"testing"

	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
)

func TestReadyz(t *testing.T) {
	ctx := context.Background()
	ok := func(context.Context) error { return nil }
	behind := func(context.Context) error { return errors.New("schema at version 1, want 2") }

	cases := []struct {
		name   string
		checks map[string]Check
		drain  bool
		want   string // "200" or "503"
		detail map[string]string
	}{
		{"healthy", map[string]Check{"database": ok}, false, "200", map[string]string{"database": "ok"}},
		{"schema behind", map[string]Check{"database": behind}, false, "503", map[string]string{"database": "schema at version 1, want 2"}},
		{"draining", map[string]Check{"database": ok}, true, "503", map[string]string{"database": "ok", "shutdown": "draining"}},
	}
	for _, c := range cases {
		h := NewHealth(c.checks)
		if c.drain {
			h.Drain()
		}
		res, err := (&Server{Health: h}).GetReadyz(ctx, gen.GetReadyzRequestObject{})
		if err != nil {
			t.Fatal(err)
		}
		var body gen.Health
		switch r := res.(type) {
		case gen.GetReadyz200JSONResponse:
			body = gen.Health(r)
			if c.want != "200" {
				t.Errorf("%s: got 200", c.name)
			}
		case gen.GetReadyz503JSONResponse:
			body = gen.Health(r)
			if c.want != "503" || body.Status != gen.Unavailable {
				t.Errorf("%s: got 503 status=%s", c.name, body.Status)
			}
		}
		for k, v := range c.detail {
			if (*body.Checks)[k] != v {
				t.Errorf("%s: check %s = %q, want %q", c.name, k, (*body.Checks)[k], v)
			}
		}
	}
}

func TestItemTextSummarizesNonDNSItems(t *testing.T) {
	for raw, want := range map[string]string{
		`{"path":"/etc/hashi/traefik/traefik.yaml","kind":"static","mode":"644","sha256":"0123456789abcdef0123"}`: "mode 644 · sha256 0123456789ab…",
		`{"version":"v3.7.16"}`: "v3.7.16",
		`{"name":"example.com","names":["example.com","*.example.com"],"not_after":"2027-01-26T15:01:41Z"}`: "example.com, *.example.com · valid until 2027-01-26",
	} {
		if got := itemText([]byte(raw)); got == nil || *got != want {
			t.Errorf("%s: %v, want %q", raw, got, want)
		}
	}
}
