package monitor

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"git.juzo.io/juzo/hashi/internal/store/db"
)

// 07 §2: healthy codes are a list of codes and ranges.
func TestParseCodes(t *testing.T) {
	got, err := ParseCodes(" 200-399, 401 ,")
	if err != nil || len(got) != 2 || got[0] != (CodeRange{200, 399}) || got[1] != (CodeRange{401, 401}) {
		t.Fatalf("%v %v", got, err)
	}
	if got, err := ParseCodes(""); err != nil || got != nil {
		t.Fatalf("empty: %v %v", got, err)
	}
	for _, bad := range []string{"abc", "99", "600", "300-200", "200-"} {
		if _, err := ParseCodes(bad); err == nil {
			t.Errorf("%q passed", bad)
		}
	}
}

// 07 §2: method, path, codes and the text the answer must contain apply to an HTTP check; the public check sends the
// monitor session.
func TestHTTPCheckSettings(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		if r.URL.Path == "/health" {
			_, _ = io.WriteString(w, strings.Repeat("x", 5000)+"all good")
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	e := Executor{Session: func() string { return "monitor-abc" }}
	ok := func(c Check) Result {
		t.Helper()
		return e.Run(context.Background(), c, 5*time.Second)
	}
	if r := ok(Check{Type: TypeHTTP, Target: srv.URL + "/", Path: "/health?x=1", Method: http.MethodPost, Contains: "all good"}); !r.OK {
		t.Fatalf("settings: %+v", r)
	}
	if got.Method != http.MethodPost || got.URL.Path != "/health" || got.URL.RawQuery != "x=1" || got.Header.Get("Cookie") != "" {
		t.Fatalf("request %s %s cookie %q", got.Method, got.URL, got.Header.Get("Cookie"))
	}
	if r := ok(Check{Type: TypeHTTP, Target: srv.URL + "/health", Contains: "missing"}); r.OK || !strings.Contains(r.Detail, "does not contain") {
		t.Fatalf("contains: %+v", r)
	}
	if r := ok(Check{Type: TypeHTTP, Target: srv.URL + "/", Codes: []CodeRange{{200, 299}, {404, 404}}}); !r.OK {
		t.Fatalf("404 in the list: %+v", r)
	}
	if r := ok(Check{Type: TypeHTTP, Target: srv.URL + "/"}); r.OK {
		t.Fatalf("404 passed by default: %+v", r)
	}
	if r := ok(Check{Type: TypeHTTP, Target: srv.URL + "/health", Public: true}); !r.OK || got.Header.Get("Cookie") != sessionCookie+"=monitor-abc" {
		t.Fatalf("public check without the monitor session: %+v %q", r, got.Header.Get("Cookie"))
	}
}

// 07 §2: Addresses limits a check to one family; a name with only the other family fails with a plain reason.
func TestCheckFamily(t *testing.T) {
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	names := newNames(func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("::1"), netip.MustParseAddr("127.0.0.1")}, nil
	}, 0)
	e := Executor{Names: names}
	if r := e.Run(context.Background(), Check{Type: TypeTCP, Target: "both.test:" + port, Family: "4"}, 5*time.Second); !r.OK {
		t.Fatalf("IPv4 only: %+v", r)
	}
	if r := e.Run(context.Background(), Check{Type: TypeTCP, Target: "127.0.0.1:" + port, Family: "6"}, 5*time.Second); r.OK || !strings.Contains(r.Detail, "not an IPv6") {
		t.Fatalf("literal of the other family: %+v", r)
	}
	v4 := newNames(func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
	}, 0)
	if r := (Executor{Names: v4}).Run(context.Background(), Check{Type: TypeTCP, Target: "v4.test:" + port, Family: "6"}, 5*time.Second); r.OK || !strings.Contains(r.Detail, "no IPv6") {
		t.Fatalf("name without IPv6: %+v", r)
	}
}

// 07 §2: the public check and the certificate of a service's name take the visitors' path; the internal check takes
// the endpoint's certificate choice; request settings stay off TCP checks.
func TestCheckFor(t *testing.T) {
	e := db.MonitorEndpoint{SourceType: SourceService}
	v := CheckSettings{Method: "HEAD", Path: "/x", Codes: "401", Family: "4", SkipVerify: true}
	in := checkFor(e, db.MonitorCheck{Type: TypeHTTPS, Role: "internal"}, v)
	if !in.SkipVerify || in.Public || in.Method != "HEAD" || in.Path != "/x" || len(in.Codes) != 1 || in.Family != "4" {
		t.Fatalf("internal %+v", in)
	}
	pub := checkFor(e, db.MonitorCheck{Type: TypeHTTPS, Role: "public"}, v)
	if pub.SkipVerify || !pub.Public {
		t.Fatalf("public %+v", pub)
	}
	if c := checkFor(e, db.MonitorCheck{Type: TypeTLS}, v); !c.Public || c.Path != "" {
		t.Fatalf("certificate %+v", c)
	}
	if c := checkFor(db.MonitorEndpoint{SourceType: SourceAdGuard}, db.MonitorCheck{Type: TypeTCP}, v); c.Public || c.Method != "" || c.Family != "4" {
		t.Fatalf("tcp %+v", c)
	}
}

func TestCheckSettingsValidate(t *testing.T) {
	for _, v := range []CheckSettings{{Method: "PUT"}, {Family: "5"}, {Path: "health"}, {Codes: "abc"}, {Contains: strings.Repeat("x", 201)}} {
		if err := v.Validate(); err == nil {
			t.Errorf("%+v passed", v)
		}
	}
	v := CheckSettings{Method: "GET", Path: " /health ", Codes: " 200-399 ", Family: "6"}
	if err := v.Validate(); err != nil || v.Path != "/health" || v.Codes != "200-399" {
		t.Fatalf("%+v %v", v, err)
	}
}
