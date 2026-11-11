package httpapi

import (
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
