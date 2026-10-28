package traefik

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// TLSPort is websecure's port (render.go), where Verify checks the served certificates.
const TLSPort = 443

// Dialer opens a TCP connection to the edge's websecure port.
type Dialer func(ctx context.Context) (net.Conn, error)

// Fingerprint is the hex SHA-256 of the certificate's leaf (the first PEM block), as certificates.fingerprint stores it.
func (c Cert) Fingerprint() (string, error) {
	b, _ := pem.Decode(c.CertPEM)
	if b == nil || b.Type != "CERTIFICATE" {
		return "", fmt.Errorf("certificate %s: no PEM certificate", c.Name)
	}
	if _, err := x509.ParseCertificate(b.Bytes); err != nil {
		return "", fmt.Errorf("certificate %s: %w", c.Name, err)
	}
	sum := sha256.Sum256(b.Bytes)
	return hex.EncodeToString(sum[:]), nil
}

// SNI is the name Verify asks for: the certificate's name (the root domain, a SAN); a wildcard name becomes a label
// under it.
func (c Cert) SNI() string {
	return strings.Replace(c.Name, "*.", "hashi-verify.", 1)
}

// Served does one TLS handshake with SNI name and returns the fingerprint of the leaf the edge presents. The chain is
// not checked: what matters is that it is exactly the certificate Hashi distributed.
func Served(ctx context.Context, dial Dialer, name string) (string, error) {
	conn, err := dial(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = conn.Close() }()
	if d, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(d)
	}
	tc := tls.Client(conn, &tls.Config{ServerName: name, InsecureSkipVerify: true, MinVersion: tls.VersionTLS12}) //nolint:gosec // compared by fingerprint
	if err := tc.HandshakeContext(ctx); err != nil {
		return "", fmt.Errorf("TLS handshake for %s: %w", name, err)
	}
	certs := tc.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return "", fmt.Errorf("TLS handshake for %s: no certificate", name)
	}
	sum := sha256.Sum256(certs[0].Raw)
	return hex.EncodeToString(sum[:]), nil
}

// ErrNotServed means the edge does not present the distributed certificate.
var ErrNotServed = errors.New("the edge does not serve the distributed certificate")

// CertCheck is the outcome of checking one certificate on an edge.
type CertCheck struct {
	Name, Want, Got string
	Err             error
}

// VerifyServed checks every certificate by TLS handshake (03 §5.3 Verify), retrying each until timeout while Traefik's
// file provider picks up the new files.
func VerifyServed(ctx context.Context, dial Dialer, certs []Cert, timeout time.Duration) []CertCheck {
	out := make([]CertCheck, 0, len(certs))
	for _, c := range certs {
		chk := CertCheck{Name: c.Name}
		chk.Want, chk.Err = c.Fingerprint()
		if chk.Err != nil {
			out = append(out, chk)
			continue
		}
		deadline := time.Now().Add(timeout)
		for {
			hctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			chk.Got, chk.Err = Served(hctx, dial, c.SNI())
			cancel()
			if chk.Err == nil && chk.Got != chk.Want {
				chk.Err = fmt.Errorf("%w: %s presents %.12s…, expected %.12s…", ErrNotServed, c.SNI(), chk.Got, chk.Want)
			}
			if chk.Err == nil || time.Now().After(deadline) || ctx.Err() != nil {
				break
			}
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
			}
		}
		out = append(out, chk)
	}
	return out
}
