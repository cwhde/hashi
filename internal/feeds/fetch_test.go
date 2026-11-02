package feeds

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFetchRefusesInternalAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("192.0.2.1\n")) }))
	defer srv.Close()
	f := &Fetcher{UserAgent: "t", MaxBytes: 1 << 20, Timeout: 5 * time.Second}
	ctx := context.Background()
	port := srv.URL[strings.LastIndex(srv.URL, ":")+1:]
	for _, raw := range []string{
		srv.URL,                          // literal loopback, refused before connecting
		"http://localhost:" + port + "/", // a name that resolves to loopback: refused by the dialer
		"http://[::ffff:127.0.0.1]:" + port + "/",
		"https://169.254.169.254/latest/meta-data/", // cloud metadata
		"https://10.20.30.40/x", "https://100.64.0.1/x", "https://[fd00::1]/x",
		"ftp://example.com/x", "https://user:pw@example.com/x",
	} {
		if _, err := f.Get(ctx, raw, true, "", ""); !errors.Is(err, ErrDenied) {
			t.Errorf("%s: %v", raw, err)
		}
	}
	if err := f.CheckURL("http://example.com/list.txt", false); !errors.Is(err, ErrDenied) {
		t.Error("plain HTTP without the feed allowing it")
	}
	// Redirects are checked like the first URL.
	u, _ := url.Parse("https://192.168.1.1/")
	if err := f.checkURL(u, false); !errors.Is(err, ErrDenied) {
		t.Error("redirect into the LAN")
	}
}

func TestFetchValidatorsLimitsAndTLS(t *testing.T) {
	var gotUA, gotINM, gotIMS string
	big := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA, gotINM, gotIMS = r.UserAgent(), r.Header.Get("If-None-Match"), r.Header.Get("If-Modified-Since")
		switch {
		case r.URL.Path == "/missing":
			http.NotFound(w, r)
		case big:
			_, _ = w.Write([]byte(strings.Repeat("x", 2<<20)))
		case gotINM == `"v1"`:
			w.WriteHeader(http.StatusNotModified)
		default:
			w.Header().Set("ETag", `"v1"`)
			w.Header().Set("Last-Modified", "Mon, 02 Nov 2026 10:00:00 GMT")
			_, _ = w.Write([]byte("192.0.2.1\n"))
		}
	}))
	defer srv.Close()
	f := &Fetcher{UserAgent: "Hashi/test", MaxBytes: 1 << 20, Timeout: 5 * time.Second, AllowInternal: true}
	ctx := context.Background()
	d, err := f.Get(ctx, srv.URL+"/list", true, "", "")
	if err != nil || string(d.Body) != "192.0.2.1\n" || d.ETag != `"v1"` || len(d.Hash) != 64 || gotUA != "Hashi/test" {
		t.Fatalf("first: %+v %v (UA %q)", d, err, gotUA)
	}
	d, err = f.Get(ctx, srv.URL+"/list", true, d.ETag, d.LastModified)
	if err != nil || !d.NotModified || gotIMS == "" {
		t.Fatalf("second: %+v %v", d, err)
	}
	if _, err := f.Get(ctx, srv.URL+"/missing", true, "", ""); err == nil || !strings.Contains(err.Error(), strconv.Itoa(404)) {
		t.Errorf("404: %v", err)
	}
	big = true
	if _, err := f.Get(ctx, srv.URL+"/list", true, "", ""); err == nil || !strings.Contains(err.Error(), "larger") {
		t.Errorf("too large: %v", err)
	}
	// TLS is verified: a self-signed server is refused.
	tls := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer tls.Close()
	if _, err := f.Get(ctx, tls.URL, false, "", ""); err == nil || !strings.Contains(err.Error(), "certificate") {
		t.Errorf("self-signed: %v", err)
	}
}
