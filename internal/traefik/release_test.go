package traefik

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Bug 9: one failed download of the release failed the whole install plan. Temporary failures are retried; a
// missing file is not.
func TestDownloadRetriesTemporaryFailures(t *testing.T) {
	for _, c := range []struct {
		name     string
		statuses []int // per request; the last repeats
		requests int32
		errHas   string
	}{
		{"two outages, then served", []int{503, 502, 200}, 3, "checksum"},
		{"rate limited once", []int{429, 200}, 2, "checksum"},
		{"down for good", []int{503}, 3, "after 3 attempts"},
		{"missing file", []int{404}, 1, "HTTP 404"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var n atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				i := int(n.Add(1)) - 1
				code := c.statuses[min(i, len(c.statuses)-1)]
				w.WriteHeader(code)
				if code == 200 {
					_, _ = w.Write([]byte("not the pinned release"))
				}
			}))
			defer srv.Close()
			_, err := download(context.Background(), srv.Client(), srv.URL, "amd64", func(int) time.Duration { return 0 })
			if err == nil || !strings.Contains(err.Error(), c.errHas) || n.Load() != c.requests {
				t.Fatalf("err %v after %d requests; want %q after %d", err, n.Load(), c.errHas, c.requests)
			}
		})
	}
}
