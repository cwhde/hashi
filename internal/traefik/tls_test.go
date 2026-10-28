package traefik

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"
)

// testCert is a self-signed certificate for name and *.name.
func testCert(t *testing.T, name string) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: name},
		DNSNames: []string{name, "*." + name}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(90 * 24 * time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb})
}

// tlsEdge serves the given certificate for every SNI and records the names asked for.
func tlsEdge(t *testing.T, certPEM, keyPEM []byte) (Dialer, func() []string) {
	t.Helper()
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var snis []string
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{MinVersion: tls.VersionTLS12,
		GetCertificate: func(h *tls.ClientHelloInfo) (*tls.Certificate, error) {
			mu.Lock()
			defer mu.Unlock()
			snis = append(snis, h.ServerName)
			return &pair, nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { _ = c.(*tls.Conn).Handshake(); _ = c.Close() }()
		}
	}()
	return func(ctx context.Context) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", ln.Addr().String())
	}, func() []string { mu.Lock(); defer mu.Unlock(); return append([]string(nil), snis...) }
}

func TestVerifyServedComparesTheLeafFingerprint(t *testing.T) {
	ctx := context.Background()
	crt, key := testCert(t, "example.com")
	other, otherKey := testCert(t, "example.com")
	dial, snis := tlsEdge(t, crt, key)
	c := Cert{Name: "example.com", CertPEM: crt}
	checks := VerifyServed(ctx, dial, []Cert{c}, time.Second)
	if len(checks) != 1 || checks[0].Err != nil || checks[0].Got != checks[0].Want {
		t.Fatalf("matching edge: %+v", checks)
	}
	if got := snis(); len(got) == 0 || got[0] != "example.com" {
		t.Errorf("SNI %q", got)
	}
	wrong, _ := tlsEdge(t, other, otherKey)
	checks = VerifyServed(ctx, wrong, []Cert{c}, 1500*time.Millisecond)
	if !errors.Is(checks[0].Err, ErrNotServed) {
		t.Errorf("other certificate served: %+v", checks[0])
	}
}
