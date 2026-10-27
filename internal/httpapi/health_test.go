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
