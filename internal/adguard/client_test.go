package adguard

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.juzo.io/juzo/hashi/internal/secrets"
)

type call struct {
	Method string          `json:"method"`
	Path   string          `json:"path"`
	Status int             `json:"status"`
	Body   json.RawMessage `json:"body"`
}

// Replays what both running versions answered (testdata/contracts/adguard).
func TestClientAgainstContract(t *testing.T) {
	files, err := filepath.Glob("../../testdata/contracts/adguard/rewrites-*.json")
	if err != nil || len(files) == 0 {
		t.Fatalf("no fixtures: %v", err)
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			var fx struct {
				Calls map[string]call `json:"calls"`
			}
			if err := json.Unmarshal(raw, &fx); err != nil {
				t.Fatal(err)
			}
			var answer call
			var gotAuth, gotBody string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if u, p, _ := r.BasicAuth(); u+":"+p != "admin:pw" {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				gotAuth = r.Method + " " + r.URL.Path
				b, _ := io.ReadAll(r.Body)
				gotBody = string(b)
				w.WriteHeader(answer.Status)
				var s string
				if json.Unmarshal(answer.Body, &s) == nil {
					_, _ = w.Write([]byte(s))
					return
				}
				_, _ = w.Write(answer.Body)
			}))
			defer srv.Close()
			c := Client{Base: srv.URL + "/", Username: "admin", Password: secrets.New("pw")} // hashi:allow-secret-pattern
			ctx := t.Context()

			answer = fx.Calls["status"]
			st, err := c.Status(ctx)
			if err != nil || !st.Running || st.Version == "" {
				t.Fatalf("status %+v %v", st, err)
			}
			answer = fx.Calls["list"]
			l, err := c.List(ctx)
			if err != nil || len(l) != 3 || !l[0].IsEnabled() || l[2].IsEnabled() {
				t.Fatalf("list %+v %v", l, err)
			}
			if gotAuth != answer.Method+" "+answer.Path {
				t.Fatalf("called %s", gotAuth)
			}
			answer = fx.Calls["add"]
			if err := c.Add(ctx, Rewrite{Domain: "svc.example.com", Answer: "192.0.2.10"}); err != nil {
				t.Fatal(err)
			}
			if gotBody != `{"domain":"svc.example.com","answer":"192.0.2.10"}` {
				t.Fatalf("add body %s", gotBody)
			}
			answer = fx.Calls["update-missing"]
			err = c.Update(ctx, Rewrite{Domain: "a.example.com", Answer: "192.0.2.1"}, Rewrite{Domain: "a.example.com", Answer: "192.0.2.2"})
			if err == nil || !strings.Contains(err.Error(), "target rule not found") {
				t.Fatalf("update missing: %v", err)
			}
			answer = fx.Calls["delete-missing"]
			if err := c.Delete(ctx, Rewrite{Domain: "a.example.com", Answer: "192.0.2.1"}); err != nil {
				t.Fatal(err)
			}
			bad := Client{Base: srv.URL, Username: "nope", Password: secrets.New("nope")} // hashi:allow-secret-pattern
			if _, err := bad.Status(ctx); !errors.Is(err, ErrAuth) {
				t.Fatalf("bad auth: %v", err)
			}
		})
	}
}

// AdGuard stores these without complaint; Hashi must not send them.
func TestClientRefusesWhatAdGuardWouldAccept(t *testing.T) {
	c := Client{Base: "http://127.0.0.1:1"}
	for _, r := range []Rewrite{{Domain: "", Answer: "192.0.2.1"}, {Domain: "a.example.com", Answer: "x"}, {Domain: " a.example.com", Answer: "192.0.2.1"}} {
		if err := c.Add(t.Context(), r); !errors.Is(err, ErrInvalid) {
			t.Errorf("%+v: %v", r, err)
		}
	}
}
