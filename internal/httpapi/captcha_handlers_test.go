package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"git.juzo.io/juzo/hashi/internal/audit"
	"git.juzo.io/juzo/hashi/internal/auth"
	"git.juzo.io/juzo/hashi/internal/httpapi/gen"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	"git.juzo.io/juzo/hashi/internal/vault"
)

// Hashi creates its own key in the Cap next to it; the form shows where Cap is without anything saved yet.
func TestCreateCaptchaKey(t *testing.T) {
	ctx := context.Background()
	st := storetest.New(t)
	al := audit.New([]byte("k"))
	v, _ := vault.New(bytes.Repeat([]byte{7}, vault.KeySize), 1)
	_ = settings.Put(ctx, st.Q, settings.General{RootDomain: "example.com", AdminDomain: "hashi.example.com",
		InternalURL: "http://192.0.2.5:3340", SyncIntervalSeconds: 3600, TrustedProxies: []string{}})
	const admin = "0123456789abcdef0123"
	capSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in map[string]any
		_ = json.NewDecoder(r.Body).Decode(&in)
		switch {
		case r.URL.Path == "/auth/login" && in["admin_key"] == admin:
			_, _ = io.WriteString(w, `{"success":true,"session_token":"tok","hashed_token":"h","expires":1}`)
		case r.URL.Path == "/server/keys":
			_, _ = io.WriteString(w, `{"siteKey":"0a1b2c3d4e","secretKey":"sk-made-by-cap"}`)
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer capSrv.Close()
	_ = settings.Put(ctx, st.Q, settings.Captcha{VerifyURL: capSrv.URL, CapPort: 3343, TimeoutSeconds: 5})
	s := &Server{Store: st, Audit: al, Vault: v, Sessions: auth.NewManager(st, al, []byte("csrf")), Captcha: &CaptchaCache{},
		CapPort: 3350, Log: slog.Default()}

	got, err := s.GetCaptchaSettings(ctx, gen.GetCaptchaSettingsRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	if b := got.(gen.GetCaptchaSettings200JSONResponse); b.CapAddress != "192.0.2.5" || b.CapPort != 3350 || *b.CanCreateKey {
		t.Fatalf("defaults %+v", b)
	}
	if _, err := s.CreateCaptchaKey(ctx, gen.CreateCaptchaKeyRequestObject{}); status(err) != 409 {
		t.Fatalf("without cap.env: %v", err)
	}
	s.CapEnvFile = t.TempDir() + "/cap.env"
	_ = os.WriteFile(s.CapEnvFile, []byte("ADMIN_KEY="+admin+"\n"), 0o600)
	res, err := s.CreateCaptchaKey(ctx, gen.CreateCaptchaKeyRequestObject{})
	if err != nil {
		t.Fatal(err)
	}
	b := res.(gen.CreateCaptchaKey200JSONResponse)
	sec, err := s.captchaSecret(ctx)
	if b.SiteKey != "0a1b2c3d4e" || !b.HasSecret || err != nil || sec.Reveal() != "sk-made-by-cap" {
		t.Fatalf("key %+v %v", b, err)
	}
}
