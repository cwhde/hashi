// Package certs is Hashi's ACME client (03-dns.md §5.3): one account, DNS-01 through the DNS provider adapter,
// certificate keys in the vault, issue and renewal as `certificates` plans run by the certificates.renew job, and
// distribution through each Traefik host's `traefik` plan.
package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"git.juzo.io/juzo/hashi/internal/secrets"
)

// ACME directories Hashi offers (testdata/contracts/acme/). "custom" takes any URL (tests, other CAs).
var Directories = map[string]string{
	"gts":         "https://dv.acme-v02.api.pki.goog/directory",
	"letsencrypt": "https://acme-v02.api.letsencrypt.org/directory",
}

// Renewal windows (03 §5.3): renew 30 days before expiry; warn at 21 days left, critical at 7. After a failed attempt
// the hourly job waits RetryAfter before trying again on its own (Renew now always tries).
const (
	RenewBefore    = 30 * 24 * time.Hour
	WarnBefore     = 21 * 24 * time.Hour
	CriticalBefore = 7 * 24 * time.Hour
	RetryAfter     = 4 * time.Hour
)

// JobInterval is how often certificates.renew runs. It plans nothing while every certificate is valid and not due.
const JobInterval = time.Hour

// Want is one certificate Hashi needs: its stable name and the SANs it covers.
type Want struct {
	Name  string
	Names []string
}

// Desired is the certificate set for the root domain: the root and `*.<root>` in one certificate. Deeper wildcards
// (one per parent name in use) arrive with services in S5.
func Desired(root string) []Want {
	root = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(root)), ".")
	if root == "" {
		return nil
	}
	return []Want{{Name: root, Names: []string{root, "*." + root}}}
}

// SameNames reports whether two SAN lists cover the same names, in any order.
func SameNames(a, b []string) bool {
	x, y := slices.Clone(a), slices.Clone(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}

// Level is how urgent a certificate's expiry is.
type Level string

// Levels (Overview and notifications use them; "" = fine).
const (
	LevelOK       Level = ""
	LevelWarning  Level = "warning"
	LevelCritical Level = "critical"
	LevelExpired  Level = "expired"
)

// LevelOf is the expiry level of a certificate that is valid until notAfter.
func LevelOf(now, notAfter time.Time) Level {
	left := notAfter.Sub(now)
	switch {
	case left <= 0:
		return LevelExpired
	case left <= CriticalBefore:
		return LevelCritical
	case left <= WarnBefore:
		return LevelWarning
	}
	return LevelOK
}

// Due reports whether a certificate valid until notAfter is inside the renewal window.
func Due(now, notAfter time.Time) bool { return notAfter.Sub(now) <= RenewBefore }

// Issued is a certificate as Hashi stores it.
type Issued struct {
	Names       []string
	CertPEM     []byte // leaf first, then the chain the CA sent
	KeyPEM      secrets.Value
	Issuer      string
	NotBefore   time.Time
	NotAfter    time.Time
	Fingerprint string // sha256 of the leaf DER, hex
}

// Fingerprint is the hex SHA-256 of a DER certificate, the value edges are verified against.
func Fingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

// FromChain builds Issued from the DER chain an ACME server returned (leaf first) and the key the CSR was made with.
func FromChain(chain [][]byte, key secrets.Value) (Issued, error) {
	if len(chain) == 0 {
		return Issued{}, errors.New("the CA returned no certificate")
	}
	leaf, err := x509.ParseCertificate(chain[0])
	if err != nil {
		return Issued{}, fmt.Errorf("parse the issued certificate: %w", err)
	}
	var b []byte
	for _, der := range chain {
		b = append(b, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})...)
	}
	issuer := leaf.Issuer.CommonName
	if len(leaf.Issuer.Organization) > 0 {
		issuer = leaf.Issuer.Organization[0] + " " + issuer
	}
	return Issued{Names: leaf.DNSNames, CertPEM: b, KeyPEM: key, Issuer: strings.TrimSpace(issuer),
		NotBefore: leaf.NotBefore, NotAfter: leaf.NotAfter, Fingerprint: Fingerprint(chain[0])}, nil
}

// NewKey creates an ECDSA P-256 key (account and certificate keys) as a PKCS#8 PEM.
func NewKey() (*ecdsa.PrivateKey, secrets.Value, error) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, secrets.Value{}, err
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		return nil, secrets.Value{}, err
	}
	return k, secrets.New(string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))), nil
}

// ParseKey reads a PKCS#8 PEM ECDSA key made by NewKey.
func ParseKey(v secrets.Value) (*ecdsa.PrivateKey, error) {
	b, _ := pem.Decode([]byte(v.Reveal()))
	if b == nil || b.Type != "PRIVATE KEY" {
		return nil, errors.New("not a PKCS#8 PEM key")
	}
	k, err := x509.ParsePKCS8PrivateKey(b.Bytes)
	if err != nil {
		return nil, err
	}
	ec, ok := k.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("not an ECDSA key")
	}
	return ec, nil
}
