// Command upstream is the integration tests' stand-in for services behind Traefik and for Hashi itself. Every answer
// names the port, protocol, path and edge header it saw, so a test can check routing, rewrites and the edge token.
// TCP 9010 and UDP 9012 echo with their port; TCP 9010 also reports a PROXY protocol v2 header and its source.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
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
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "get":
			get(os.Args[2], os.Args[3], os.Args[4:])
			return
		case "tcp", "udp":
			send(os.Args[1], os.Args[2], os.Args[3])
			return
		}
	}
	go tcpEcho("127.0.0.1:9010")
	go udpEcho("127.0.0.1:9012")
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

// proxyV2 is the PROXY protocol v2 signature.
var proxyV2 = []byte("\r\n\r\n\x00\r\nQUIT\n")

// tcpEcho answers each connection's first line with "tcp port=… proxy=… got=…".
func tcpEcho(addr string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	for {
		c, err := ln.Accept()
		if err != nil {
			log.Fatal(err)
		}
		go func() {
			defer func() { _ = c.Close() }()
			_ = c.SetDeadline(time.Now().Add(10 * time.Second))
			r := bufio.NewReader(c)
			proxy := "none"
			// Only a header starts with "\r"; a plain client line may be shorter than the 16 header bytes.
			if first, err := r.Peek(1); err != nil || first[0] != '\r' {
				proxy = "none"
			} else if head, err := r.Peek(16); err == nil && bytes.Equal(head[:12], proxyV2) {
				n := int(binary.BigEndian.Uint16(head[14:16]))
				hdr := make([]byte, 16+n)
				if _, err := io.ReadFull(r, hdr); err != nil {
					return
				}
				proxy = "v2"
				if hdr[13] == 0x11 && n >= 12 { // TCP over IPv4: source address first
					proxy += " src=" + net.IP(hdr[16:20]).String()
				}
			}
			line, _ := r.ReadString('\n')
			_, _ = fmt.Fprintf(c, "tcp port=%s proxy=%s got=%s\n", addr[strings.LastIndex(addr, ":")+1:], proxy, strings.TrimSpace(line))
		}()
	}
}

// udpEcho answers every datagram with "udp got=…".
func udpEcho(addr string) {
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		log.Fatal(err)
	}
	buf := make([]byte, 2048)
	for {
		n, from, err := pc.ReadFrom(buf)
		if err != nil {
			log.Fatal(err)
		}
		_, _ = pc.WriteTo([]byte("udp got="+string(buf[:n])+"\n"), from)
	}
}

// send writes msg to 127.0.0.1:<port> over tcp or udp and prints the first reply line. The TCP side keeps its
// connection open until it has read the reply (testdata/contracts/traefik/streams: a half-closed client got none).
func send(network, port, msg string) {
	c, err := net.DialTimeout(network, "127.0.0.1:"+port, 5*time.Second)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	_ = c.SetDeadline(time.Now().Add(5 * time.Second))
	if network == "udp" {
		_, err = c.Write([]byte(msg))
	} else {
		_, err = c.Write([]byte(msg + "\n"))
	}
	if err != nil {
		log.Fatal(err)
	}
	line, err := bufio.NewReader(c).ReadString('\n')
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(line)
}
