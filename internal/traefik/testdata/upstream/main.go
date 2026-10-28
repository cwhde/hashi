// Command upstream is the integration tests' stand-in for services behind Traefik and for Hashi itself. Every answer
// names the port, protocol, path and edge header it saw, so a test can check routing, rewrites and the edge token.
package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "get" {
		get(os.Args[2], os.Args[3], os.Args[4:])
		return
	}
	var mu sync.Mutex
	lastFA := "none"
	handler := func(port string) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/readyz":
				return
			case "/api/edge-auth/forward": // Hashi's forward auth: allow, and remember the edge header it carried
				mu.Lock()
				lastFA = r.Header.Get("X-Hashi-Edge")
				mu.Unlock()
				return
			case "/last-fa":
				mu.Lock()
				defer mu.Unlock()
				_, _ = fmt.Fprintf(w, "fa-edge=%s\n", lastFA)
				return
			}
			_, _ = fmt.Fprintf(w, "port=%s proto=%s path=%s edge=%s\n", port, r.Proto, r.URL.Path, r.Header.Get("X-Hashi-Edge"))
		})
	}
	h2c := new(http.Protocols)
	h2c.SetUnencryptedHTTP2(true)
	go serve(&http.Server{Addr: "127.0.0.1:9001", Handler: handler("9001"), ReadHeaderTimeout: 5 * time.Second}, false)
	go serve(&http.Server{Addr: "127.0.0.1:9002", Handler: handler("9002"), Protocols: h2c, ReadHeaderTimeout: 5 * time.Second}, false)
	go serve(&http.Server{Addr: "127.0.0.1:9003", Handler: handler("9003"), TLSConfig: selfSigned(), ReadHeaderTimeout: 5 * time.Second}, true)
	serve(&http.Server{Addr: "127.0.0.1:9000", Handler: handler("9000"), ReadHeaderTimeout: 5 * time.Second}, false)
}

func serve(s *http.Server, tlsOn bool) {
	var err error
	if tlsOn {
		err = s.ListenAndServeTLS("", "")
	} else {
		err = s.ListenAndServe()
	}
	log.Fatal(err)
}

func selfSigned() *tls.Config {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "upstream.internal"},
		DNSNames: []string{"upstream.internal"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		log.Fatal(err)
	}
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12}
}

// get requests https://<host><path> from the local Traefik (127.0.0.1:443) with that SNI and Host, plus Name=value
// headers, and prints the status and body.
func get(host, path string, headers []string) {
	tr := &http.Transport{TLSClientConfig: &tls.Config{ServerName: host, InsecureSkipVerify: true}, //nolint:gosec // test client
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, network, "127.0.0.1:443")
		}}
	req, err := http.NewRequest(http.MethodGet, "https://"+host+path, nil)
	if err != nil {
		log.Fatal(err)
	}
	for _, h := range headers {
		k, v, _ := strings.Cut(h, "=")
		req.Header.Set(k, v)
	}
	resp, err := (&http.Client{Transport: tr, Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	fmt.Printf("status=%d %s", resp.StatusCode, b)
}
