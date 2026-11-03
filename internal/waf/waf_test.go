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

func with(r Request, k, v string) Request {
	r.Header[k] = []string{v}
	return r
}

// Plain pages, sync clients, WebDAV and git pass; attacks in the query and unknown methods are refused.
func TestCheck(t *testing.T) {
	e := New()
	base := Config{}
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
		{"PUT octet-stream (sync clients)", base, req("PUT", "/remote.php/dav/files/a/x.bin", "application/octet-stream"), false},
		{"PROPFIND (WebDAV)", base, req("PROPFIND", "/remote.php/dav/", ""), false},
		{"git push over HTTPS", base, req("POST", "/owner/repo.git/git-receive-pack", "application/x-git-receive-pack-request"), false},
		{"git fetch refs", base, req("GET", "/owner/repo.git/info/refs?service=git-upload-pack", ""), false},
		{"git LFS", base, req("POST", "/owner/repo.git/info/lfs/objects/batch", "application/vnd.git-lfs+json"), false},
		{"git fetch without .git (seen in CI, 920450)", base, with(req("POST", "/owner/repo/git-upload-pack", "application/x-git-upload-pack-request"), "Content-Encoding", "gzip"), false},
		{"large git push", base, with(req("POST", "/owner/repo/git-receive-pack", "application/x-git-receive-pack-request"), "Expect", "100-continue"), false},
		{"WebDAV LOCK", base, with(with(req("LOCK", "/remote.php/dav/files/a/x.txt", "application/xml"), "Lock-Token", "<opaquelocktoken:1>"), "If", "(<opaquelocktoken:1>)"), false},
		{"resumed upload", base, with(req("PUT", "/remote.php/dav/files/a/x.bin", "application/octet-stream"), "Content-Range", "bytes 0-99/200"), false},
		{"httpoxy", base, with(req("GET", "/", ""), "Proxy", "http://198.51.100.9"), true},
		{"method override", base, with(req("POST", "/api/items", "application/json"), "X-HTTP-Method-Override", "DELETE"), true},
		{"probe for a site's .git folder", base, req("GET", "/.git/config", ""), true},
		{"probe dressed as git", base, req("GET", "/.git/info/refs?service=git-upload-pack", ""), true},
		{"unknown method", base, req("TRACE", "/", ""), true},
		{"xss in the query", base, req("GET", "/?q=%3Cscript%3Ealert(1)%3C/script%3E", ""), true},
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
		t.Errorf("%d compiled WAFs, want one per configuration (2: plain, and git over HTTPS)", len(e.wafs))
	}
}

func TestExclusionRemovesARule(t *testing.T) {
	e := New()
	r := req("GET", "/?q=%3Cscript%3Ealert(1)%3C/script%3E", "")
	v, _ := e.Check(Config{}, r)
	if !v.Blocked {
		t.Fatal("not blocked without exclusions")
	}
	// Excluding the anomaly evaluation lets it pass: exclusions are how a false positive is silenced.
	if v, _ := e.Check(Config{Exclusions: []int{949110}}, r); v.Blocked {
		t.Fatalf("blocked by %d with the deciding rule excluded", v.RuleID)
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
