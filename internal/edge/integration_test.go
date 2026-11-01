//go:build integration

package edge

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os/exec"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/secrets"
	"git.juzo.io/juzo/hashi/internal/store/db"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// The whole sign-in against a real Pocket ID (the version the real server runs): real discovery, keys, PKCE, ID token and
// group claim. A signed-in browser is stood in for by Pocket ID's one-time access token.
func TestIntegrationPocketID(t *testing.T) {
	const name, port, issuer = "hashi-test-pocketid", "18412", "http://localhost:18412"
	const callback = "http://localhost:9/cb"
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	apiKey := hex.EncodeToString(b[:16])
	_ = exec.Command("docker", "rm", "-f", name).Run()
	out, err := exec.Command("docker", "run", "-d", "--name", name, "-p", "127.0.0.1:"+port+":1411", "-e", "APP_URL="+issuer,
		"-e", "ENCRYPTION_KEY="+hex.EncodeToString(b), "-e", "STATIC_API_KEY="+apiKey, "ghcr.io/pocket-id/pocket-id:v2.19.1").CombinedOutput()
	if err != nil {
		t.Fatalf("start: %v: %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", name).Run() })
	for i := 0; ; i++ {
		resp, err := http.Get(issuer + "/.well-known/openid-configuration")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				break
			}
		}
		if i > 60 {
			t.Fatal("Pocket ID did not start")
		}
		time.Sleep(time.Second)
	}
	admin := func(method, path string, body any) map[string]any {
		t.Helper()
		raw, _ := json.Marshal(body)
		req, _ := http.NewRequest(method, issuer+path, bytes.NewReader(raw))
		req.Header.Set("X-API-Key", apiKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		data, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 300 {
			t.Fatalf("%s %s: %d %s", method, path, resp.StatusCode, data)
		}
		out := map[string]any{}
		_ = json.Unmarshal(data, &out)
		return out
	}
	user := admin("POST", "/api/users", map[string]any{"username": "alice", "email": "alice@example.com", "firstName": "Alice", "lastName": "T", "emailVerified": true})["id"].(string)
	group := admin("POST", "/api/user-groups", map[string]any{"name": "admins", "friendlyName": "Admins"})["id"].(string)
	admin("PUT", "/api/user-groups/"+group+"/users", map[string]any{"userIds": []string{user}})
	client := admin("POST", "/api/oidc/clients", map[string]any{"name": "hashi", "callbackURLs": []string{callback}, "logoutCallbackURLs": []string{}, "pkceEnabled": true, "skipConsent": true})
	clientID, clientSecret := client["id"].(string), client["createdSecret"].(map[string]any)["secret"].(string)
	token := admin("POST", "/api/users/"+user+"/one-time-access-token", map[string]any{"ttl": "1h"})["token"].(string)

	ctx := context.Background()
	st := storetest.New(t)
	v, err := vault.New(bytes.Repeat([]byte{7}, vault.KeySize), 1)
	if err != nil {
		t.Fatal(err)
	}
	p, err := v.Seal(vault.Service, "oidc:p", secrets.New(clientSecret))
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Tx(ctx, func(q *db.Queries) error {
		if err := q.InsertSecret(ctx, p); err != nil {
			return err
		}
		return q.InsertOIDCProvider(ctx, db.InsertOIDCProviderParams{ID: "p", Name: "Pocket ID", Issuer: issuer, ClientID: clientID, SecretID: p.ID,
			Scopes: "openid profile email groups", UsernameClaim: "preferred_username", EmailClaim: "email", GroupsClaim: "groups", IsDefault: 1})
	}); err != nil {
		t.Fatal(err)
	}
	sso := NewSSO(st, v, &http.Client{Timeout: 10 * time.Second}, time.Now)
	authURL, state, err := sso.Begin(ctx, "", "https://a.example.com/x", callback)
	if err != nil {
		t.Fatal(err)
	}

	// The browser: sign in, then follow the authorize address until it lands on the callback.
	jar, _ := cookiejar.New(nil)
	browser := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := browser.Post(issuer+"/api/one-time-access-token/"+token, "application/json", nil)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("browser login: %v %v", resp, err)
	}
	_ = resp.Body.Close()
	next := authURL
	var code string
	for i := 0; i < 5 && code == ""; i++ {
		resp, err := browser.Get(next)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		next = resp.Header.Get("Location")
		if !strings.HasPrefix(next, callback) {
			continue
		}
		u, err := url.Parse(next)
		if err != nil {
			t.Fatal(err)
		}
		if u.Query().Get("state") != state {
			t.Fatalf("state: %s", next)
		}
		code = u.Query().Get("code")
	}
	if code == "" {
		t.Fatalf("no code; last redirect %q", next)
	}
	cookie, rd, _, err := sso.Finish(ctx, code, state, state, callback, "192.0.2.1", "ua")
	if err != nil {
		t.Fatal(err)
	}
	sess, err := sso.Validate(ctx, cookie)
	if err != nil {
		t.Fatal(err)
	}
	if rd != "https://a.example.com/x" || sess.Username != "alice" || sess.Email != "alice@example.com" || !sess.HasAnyGroup([]string{"admins"}) {
		t.Fatalf("rd %q, session %+v", rd, sess)
	}
	end, err := sso.Logout(ctx, cookie, "")
	if err != nil || !strings.Contains(end, "/api/oidc/end-session") {
		t.Fatalf("logout: %q %v", end, err)
	}
}
