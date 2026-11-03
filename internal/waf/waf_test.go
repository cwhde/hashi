package waf

import (
	"net/netip"
	"runtime"
	"testing"
	"time"
)

func req(method, uri, contentType string) Request {
	h := map[string][]string{"User-Agent": {"Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0"},
		"Accept": {"text/html,application/xhtml+xml"}, "Accept-Language": {"en"}}
	if contentType != "" {
		h["Content-Type"] = []string{contentType}
	}
	return Request{Client: netip.MustParseAddr("198.51.100.7"), Method: method, URI: uri, Host: "files.example.com", Header: h}
}

// The same requests as the Traefik plugin's integration test (internal/traefik/integration_waf_test.go, removed with
// the plugin): plain pages pass; SQL injection in the query, a PUT of octet-stream and PROPFIND are refused; uploads
// lets PUT and WebDAV through; an exclusion removes a rule.
func TestCheck(t *testing.T) {
	e := New()
	base, up := Config{}, Config{Uploads: true}
	cases := []struct {
		name    string
		c       Config
		r       Request
		blocked bool
	}{
		{"plain page", base, req("GET", "/index.html", ""), false},
		{"query", base, req("GET", "/search?q=hello+world", ""), false},
		{"json api", base, req("POST", "/api/items", "application/json"), false},
		{"sql injection in the query", base, req("GET", "/?id=1%27%20OR%201=1--", ""), true},
		{"path traversal", base, req("GET", "/files?name=../../etc/passwd", ""), true},
		{"PUT octet-stream", base, req("PUT", "/file", "application/octet-stream"), true},
		{"PROPFIND", base, req("PROPFIND", "/", ""), true},
		{"PUT octet-stream with uploads", up, req("PUT", "/up/file", "application/octet-stream"), false},
		{"PROPFIND with uploads", up, req("PROPFIND", "/up/", ""), false},
		{"sql injection with uploads", up, req("GET", "/?id=1%27%20OR%201=1--", ""), true},
	}
	for _, c := range cases {
		v, err := e.Check(c.c, c.r)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if v.Blocked != c.blocked {
			t.Errorf("%s: blocked=%v (rule %d), want %v", c.name, v.Blocked, v.RuleID, c.blocked)
		}
	}
	if len(e.wafs) != 2 {
		t.Errorf("%d compiled WAFs, want one per configuration (2)", len(e.wafs))
	}
}

func TestExclusionRemovesARule(t *testing.T) {
	e := New()
	r := req("PUT", "/file", "application/octet-stream")
	if v, _ := e.Check(Config{}, r); !v.Blocked {
		t.Fatal("not blocked without the exclusion")
	}
	if v, _ := e.Check(Config{Exclusions: []int{920420}}, r); v.Blocked {
		t.Fatalf("blocked by %d with 920420 excluded", v.RuleID)
	}
}

// What one compiled WAF costs, for the record: the plugin needed ~0.7 GB for one route and 2.5 GB for 60.
func TestCost(t *testing.T) {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	e := New()
	if err := e.Warm(Config{}); err != nil {
		t.Fatal(err)
	}
	built := time.Since(start)
	runtime.GC()
	runtime.ReadMemStats(&after)
	start = time.Now()
	const n = 200
	for range n {
		_, _ = e.Check(Config{}, req("GET", "/index.html?page=2", ""))
	}
	t.Logf("build %v, heap %+d MB, check %v per request", built, (int64(after.HeapInuse)-int64(before.HeapInuse))>>20, time.Since(start)/n)
	runtime.KeepAlive(e)
}
