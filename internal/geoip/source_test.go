package geoip

import (
	"context"
	"net/http"
	"testing"
	"time"
)

type capture struct{ req *http.Request }

func (c *capture) RoundTrip(r *http.Request) (*http.Response, error) {
	c.req = r
	return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody, Request: r}, nil
}

// The GeoIP account (09 §2 Security): with an account the download goes to MaxMind and logs in with the account ID;
// with a URL the account is not sent anywhere.
func TestGeoIPAccountSetting(t *testing.T) {
	c := &capture{}
	s := New(t.TempDir(), &http.Client{Transport: c}, time.Now)
	_, _ = s.request(context.Background(), Source{AccountID: "123456", LicenseKey: "key"}, maxmindURL, Country)
	if u, _, ok := c.req.BasicAuth(); !ok || u != "123456" || c.req.URL.Host != "download.maxmind.com" {
		t.Fatalf("maxmind request %v %v", c.req.URL, ok)
	}
	if (Source{AccountID: "123456"}).Configured() {
		t.Fatal("an account without its key counts as configured")
	}
	_, _ = s.request(context.Background(), Source{AccountID: "123456", LicenseKey: "key", URL: "https://mirror.example.com/{edition}"},
		"https://mirror.example.com/{edition}", Country)
	if _, _, ok := c.req.BasicAuth(); ok {
		t.Fatal("the account was sent to a mirror")
	}
}
