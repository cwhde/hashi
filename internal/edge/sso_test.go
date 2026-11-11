package edge

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// fakeIdP answers with the shapes in testdata/contracts/oidc (Pocket ID 2.19.1).
type fakeIdP struct {
	srv                 *httptest.Server
	key                 *rsa.PrivateKey
	nonce, code, secret string
	claims              jwt.MapClaims
	verifier            string
}

func newIdP(t *testing.T) *fakeIdP {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIdP{key: k, code: "the-code", secret: "client-secret"}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		raw, err := os.ReadFile("../../testdata/contracts/oidc/discovery.json")
		if err != nil {
			t.Error(err)
		}
		_, _ = w.Write([]byte(strings.ReplaceAll(string(raw), "https://sso.example.com", f.srv.URL)))
	})
	mux.HandleFunc("GET /.well-known/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		enc := base64.RawURLEncoding.EncodeToString
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{{"kid": "k1", "kty": "RSA", "alg": "RS256", "use": "sig",
			"n": enc(k.N.Bytes()), "e": enc(big.NewInt(int64(k.E)).Bytes())}}})
	})
	mux.HandleFunc("POST /api/oidc/token", func(w http.ResponseWriter, r *http.Request) {
		id, sec, _ := r.BasicAuth()
		if id != "client" || sec != f.secret {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
			return
		}
		if r.FormValue("code") != f.code {
			raw, _ := os.ReadFile("../../testdata/contracts/oidc/token_error_invalid_grant.json")
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(raw)
			return
		}
		f.verifier = r.FormValue("code_verifier")
		c := jwt.MapClaims{"iss": f.srv.URL, "aud": "client", "sub": "user-1", "exp": time.Now().Add(time.Hour).Unix(), "nonce": f.nonce,
			"preferred_username": "alice", "email": "alice@example.com", "groups": []string{"admins"}}
		for k, v := range f.claims {
			c[k] = v
		}
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
		tok.Header["kid"] = "k1"
		s, err := tok.SignedString(k)
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id_token": s, "access_token": "a", "token_type": "Bearer", "expires_in": 3599})
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func newSSO(t *testing.T, f *fakeIdP) (*SSO, *store.Store) {
	t.Helper()
	ctx := context.Background()
	st := storetest.New(t)
	v, err := vault.New(bytes.Repeat([]byte{7}, vault.KeySize), 1)
	if err != nil {
		t.Fatal(err)
	}
	p, err := v.Seal(vault.Service, "oidc:p", secrets.New(f.secret))
	if err != nil {
		t.Fatal(err)
	}
	err = st.Tx(ctx, func(q *db.Queries) error {
		if err := q.InsertSecret(ctx, p); err != nil {
			return err
		}
		return q.InsertOIDCProvider(ctx, db.InsertOIDCProviderParams{ID: "p", Name: "Test", Issuer: f.srv.URL, ClientID: "client", SecretID: p.ID,
			Scopes: "openid profile email groups", UsernameClaim: "preferred_username", EmailClaim: "email", GroupsClaim: "groups", IsDefault: 1})
	})
	if err != nil {
		t.Fatal(err)
	}
	return NewSSO(st, v, f.srv.Client(), time.Now), st
}

func begin(t *testing.T, s *SSO, f *fakeIdP) string {
	t.Helper()
	auth, state, err := s.Begin(context.Background(), "", "https://a.example.com/x", "https://admin.example.com/cb")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(auth)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	if q.Get("code_challenge_method") != "S256" || len(q.Get("state")) < 8 || q.Get("nonce") == "" || q.Get("client_id") != "client" {
		t.Fatalf("authorize query: %v", q)
	}
	f.nonce = q.Get("nonce")
	return state
}

func TestSSOFlow(t *testing.T) {
	ctx := context.Background()
	f := newIdP(t)
	s, _ := newSSO(t, f)
	state := begin(t, s, f)
	cookie, rd, _, err := s.Finish(ctx, f.code, state, state, "https://admin.example.com/cb", "192.0.2.1", "ua")
	if err != nil {
		t.Fatal(err)
	}
	if rd != "https://a.example.com/x" || len(f.verifier) < 43 {
		t.Fatalf("rd %q, verifier %d chars", rd, len(f.verifier))
	}
	sess, err := s.Validate(ctx, cookie)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Username != "alice" || !sess.HasAnyGroup([]string{"admins"}) || sess.HasAnyGroup([]string{"other"}) || !sess.HasAnyGroup(nil) {
		t.Fatalf("session %+v", sess)
	}
	// A state is good for one try.
	if _, _, _, err := s.Finish(ctx, f.code, state, state, "https://admin.example.com/cb", "", ""); err == nil {
		t.Fatal("state reused")
	}
	// The MAC protects the cookie; logout ends the session.
	if _, err := s.Validate(ctx, cookie+"x"); err == nil {
		t.Fatal("tampered cookie accepted")
	}
	if _, err := s.Logout(ctx, cookie, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Validate(ctx, cookie); err == nil {
		t.Fatal("session alive after logout")
	}
}

func TestSSORefusals(t *testing.T) {
	ctx := context.Background()
	for name, c := range map[string]struct {
		claims jwt.MapClaims
		code   string
		cookie string
	}{
		"other browser":  {cookie: "someone-else"},
		"bad code":       {code: "nope"},
		"wrong audience": {claims: jwt.MapClaims{"aud": "another"}},
		"wrong issuer":   {claims: jwt.MapClaims{"iss": "https://evil.example.com"}},
		"expired":        {claims: jwt.MapClaims{"exp": time.Now().Add(-time.Hour).Unix()}},
		"wrong nonce":    {claims: jwt.MapClaims{"nonce": "other"}},
		"no subject":     {claims: jwt.MapClaims{"sub": ""}},
	} {
		t.Run(name, func(t *testing.T) {
			f := newIdP(t)
			f.claims = c.claims
			s, _ := newSSO(t, f)
			state := begin(t, s, f)
			code, cookie := f.code, state
			if c.code != "" {
				code = c.code
			}
			if c.cookie != "" {
				cookie = c.cookie
			}
			if _, _, _, err := s.Finish(ctx, code, state, cookie, "https://admin.example.com/cb", "", ""); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

// The edge sign-in settings (06 §16, 09 §2): a session lasts MaxSessionHours and ends after IdleTimeoutHours without use.
func TestSSOSessionSettings(t *testing.T) {
	ctx := context.Background()
	f := newIdP(t)
	s, st := newSSO(t, f)
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }
	if err := settings.Put(ctx, st.Q, settings.EdgeSSO{MaxSessionHours: 3, IdleTimeoutHours: 1, RememberDevice: true}); err != nil {
		t.Fatal(err)
	}
	used, expires, err := s.OpenSession(ctx, "p", Identity{Subject: "s", Username: "alice"}, "192.0.2.1", "ua")
	if err != nil || !expires.Equal(now.Add(3*time.Hour)) {
		t.Fatalf("expires %v %v", expires, err)
	}
	idle, _, err := s.OpenSession(ctx, "p", Identity{Subject: "s", Username: "alice"}, "192.0.2.1", "ua")
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(59 * time.Minute)
	if _, err := s.Validate(ctx, used); err != nil {
		t.Fatalf("within the idle time: %v", err)
	}
	now = now.Add(2 * time.Minute) // the other session: 61 min without use
	if _, err := s.Validate(ctx, idle); err == nil {
		t.Fatal("session outlived the 1 h idle timeout")
	}
}
