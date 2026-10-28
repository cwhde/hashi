package certs

import (
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

func TestLevelsAndRenewalWindow(t *testing.T) {
	now := time.Date(2026, 10, 28, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	for _, c := range []struct {
		left  time.Duration
		level Level
		due   bool
	}{
		{90 * day, LevelOK, false}, {30*day + time.Minute, LevelOK, false}, {30 * day, LevelOK, true},
		{21 * day, LevelWarning, true}, {7*day + time.Minute, LevelWarning, true}, {7 * day, LevelCritical, true},
		{time.Minute, LevelCritical, true}, {0, LevelExpired, true}, {-day, LevelExpired, true},
	} {
		if got := LevelOf(now, now.Add(c.left)); got != c.level {
			t.Errorf("%s left: level %q, want %q", c.left, got, c.level)
		}
		if got := Due(now, now.Add(c.left)); got != c.due {
			t.Errorf("%s left: due %v", c.left, got)
		}
	}
	if d := Desired(" Example.COM. "); len(d) != 1 || d[0].Name != "example.com" || !SameNames(d[0].Names, []string{"*.example.com", "example.com"}) {
		t.Errorf("desired: %+v", d)
	}
	if Desired("") != nil {
		t.Error("no root, still a certificate")
	}
}

func TestFromChainAndKeys(t *testing.T) {
	key, pemKey, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseKey(pemKey)
	if err != nil || !back.Equal(key) {
		t.Fatalf("key round trip: %v", err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "example.com"},
		Issuer: pkix.Name{CommonName: "Test CA"}, DNSNames: []string{"example.com", "*.example.com"},
		NotBefore: time.Unix(1_800_000_000, 0), NotAfter: time.Unix(1_807_776_000, 0)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	iss, err := FromChain([][]byte{der, der}, pemKey)
	if err != nil {
		t.Fatal(err)
	}
	if iss.Fingerprint != Fingerprint(der) || len(iss.Names) != 2 || !iss.NotAfter.Equal(tpl.NotAfter) ||
		iss.Issuer != "example.com" || len(iss.CertPEM) == 0 {
		t.Errorf("issued: %+v", iss)
	}
	if _, err := FromChain(nil, pemKey); err == nil {
		t.Error("empty chain accepted")
	}
	for _, s := range []string{"zWNDZM6eQGHWpSRTPal5eIUY", "zWNDZM6eQGHWpSRTPal5eIUY==", " zWNDZM6eQGHWpSRTPal5eIUY\n"} {
		if b, err := decodeHMAC(s); err != nil || len(b) != 18 {
			t.Errorf("%q: %d bytes, %v", s, len(b), err)
		}
	}
	if _, err := decodeHMAC("not base64!"); err == nil {
		t.Error("bad HMAC accepted")
	}
}
