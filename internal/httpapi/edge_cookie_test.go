package httpapi

import (
	"log/slog"

	"git.juzo.io/juzo/hashi/internal/security"
	"git.juzo.io/juzo/hashi/internal/services"
	"git.juzo.io/juzo/hashi/internal/settings"
	"git.juzo.io/juzo/hashi/internal/store/storetest"
	"testing"
	"time"
)

// Remember device (09 §2 Security): on, the sign-in cookie lasts until the session expires; off, it is a browser-session
// cookie.
func TestEdgeSessionCookieRememberDevice(t *testing.T) {
	exp := time.Now().Add(48 * time.Hour)
	if c := edgeSessionCookie("example.com", "v", exp, true); c.MaxAge < 47*3600 || c.MaxAge > 48*3600 {
		t.Fatalf("remembered cookie max-age %d", c.MaxAge)
	}
	if c := edgeSessionCookie("example.com", "v", exp, false); c.MaxAge != 0 || !c.Expires.IsZero() {
		t.Fatalf("session cookie %+v", c)
	}
}

// The default WAF mode (09 §2 Security): a service without its own mode follows it.
func TestWAFDefaultModeSetting(t *testing.T) {
	sec := security.NewEngine(storetest.New(t), slog.New(slog.DiscardHandler), time.Now)
	s := &Server{Security: sec}
	cfg := sec.Settings()
	for _, mode := range []string{settings.WAFDisabled, settings.WAFHeadersOnly} {
		cfg.WAFDefaultMode = mode
		sec.UseSettings(cfg)
		if got := s.wafFor(services.WAF{}, false, "/x"); got.Mode != mode {
			t.Errorf("default %s: service gets %s", mode, got.Mode)
		}
	}
}
