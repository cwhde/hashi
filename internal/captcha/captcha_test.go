package captcha

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/secrets"
)

type answer struct {
	status int
	body   string
}

// captured reads testdata/contracts/cap/siteverify.txt: one "### name" section per answer, status line and body.
func captured(t *testing.T) map[string]answer {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/contracts/cap/siteverify.txt")
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]answer{}
	for _, sec := range strings.Split(string(raw), "### ")[1:] {
		lines := strings.Split(strings.TrimSpace(sec), "\n")
		f := strings.Fields(lines[1])
		code, _ := strconv.Atoi(f[1])
		out[lines[0]] = answer{status: code, body: lines[len(lines)-1]}
	}
	return out
}

const siteKey = "0123456789"

func TestVerifyAgainstCapture(t *testing.T) {
	ans := captured(t)
	var serve answer
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/"+siteKey+"/siteverify" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("request %s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json;charset=utf-8")
		w.WriteHeader(serve.status)
		_, _ = w.Write([]byte(serve.body))
	}))
	defer srv.Close()
	ctx := context.Background()
	for name, want := range map[string]Result{
		"success":                               Solved,
		"wrong secret":                          NotSolved,
		"same token again":                      NotSolved,
		"token of another site key in the path": NotSolved,
		"malformed token":                       NotSolved,
		"missing response":                      NotSolved,
		"unknown site key, well-formed token":   NotSolved,
	} {
		a, ok := ans[name]
		if !ok {
			t.Fatalf("no captured answer %q", name)
		}
		serve = a
		r, _ := Verify(ctx, srv.Client(), srv.URL+"/", siteKey, secrets.New("sk-test"), "tok")
		if r != want {
			t.Errorf("%s: %v, want %v", name, r, want)
		}
	}
	if got["secret"] != "sk-test" || got["response"] != "tok" {
		t.Errorf("body %v", got)
	}

	// The settings test: the right secret makes Cap say "Token not found" for a made-up token.
	serve = ans["same token again"]
	if err := Test(ctx, srv.Client(), srv.URL, siteKey, secrets.New("sk-test")); err != nil {
		t.Errorf("good settings: %v", err)
	}
	serve = ans["wrong secret"]
	if err := Test(ctx, srv.Client(), srv.URL, siteKey, secrets.New("sk-wrong")); err == nil || !strings.Contains(err.Error(), "Invalid site key or secret") {
		t.Errorf("wrong secret: %v", err)
	}
	srv.Close()
	if r, err := Verify(ctx, srv.Client(), srv.URL, siteKey, secrets.New("sk"), "tok"); r != Unavailable || err == nil {
		t.Errorf("Cap down: %v %v", r, err)
	}
}

func TestClearance(t *testing.T) {
	key := []byte("clearance-key-for-tests")
	now := time.Unix(1_800_000_000, 0)
	c := Clearance(key, "SVC1", now.Add(time.Hour))
	switch {
	case !Cleared(key, "SVC1", c, now):
		t.Error("fresh clearance refused")
	case Cleared(key, "SVC2", c, now):
		t.Error("clearance of one service accepted for another")
	case Cleared(key, "SVC1", c, now.Add(time.Hour)):
		t.Error("expired clearance accepted")
	case Cleared([]byte("other key"), "SVC1", c, now):
		t.Error("clearance signed with another key accepted")
	case Cleared(key, "SVC1", Clearance(key, "SVC1", now.Add(time.Hour))[:10]+"x."+strings.Split(c, ".")[1], now):
		t.Error("tampered clearance accepted")
	case Cleared(key, "SVC1", "garbage", now):
		t.Error("garbage accepted")
	}
	if CookieName("01ABC") != "hashi_cap_01abc" {
		t.Error(CookieName("01ABC"))
	}
}
